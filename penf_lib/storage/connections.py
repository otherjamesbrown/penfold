"""Database connection and tenant-aware session management for Penfold storage layer."""

import logging
import uuid
import os
from typing import AsyncGenerator, Optional, Dict, Any
from contextlib import asynccontextmanager

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine, async_sessionmaker
from sqlalchemy import text, select
import redis.asyncio as redis

from .config import config

logger = logging.getLogger(__name__)


# Async database engine
async_engine = create_async_engine(
    config.database.database_url,
    pool_size=config.database.pool_size,
    max_overflow=config.database.max_overflow,
    pool_recycle=config.database.pool_recycle,
    pool_pre_ping=True,
    echo=config.is_development,  # Enable SQL logging in development
)

# Async session factory
AsyncSessionLocal = async_sessionmaker(
    async_engine,
    class_=AsyncSession,
    expire_on_commit=False,
    autoflush=True,
    autocommit=False,
)

# Redis connection pool
redis_pool: Optional[redis.ConnectionPool] = None


async def init_redis() -> redis.Redis:
    """Initialize Redis connection pool."""
    global redis_pool
    if redis_pool is None:
        redis_pool = redis.ConnectionPool.from_url(
            config.redis.redis_url,
            max_connections=config.redis.max_connections,
            retry_on_timeout=config.redis.retry_on_timeout,
            decode_responses=True,
        )
    return redis.Redis(connection_pool=redis_pool)


def get_redis_client() -> redis.Redis:
    """Get a Redis client instance.

    This is a synchronous wrapper around init_redis for compatibility
    with event processing modules.
    """
    # For now, create a new client with the URL
    # In production, this should use connection pooling
    return redis.Redis.from_url(
        config.redis.redis_url,
        max_connections=config.redis.max_connections,
        retry_on_timeout=config.redis.retry_on_timeout,
        socket_keepalive=config.redis.socket_keepalive,
    )


async def close_redis() -> None:
    """Close Redis connection pool."""
    global redis_pool
    if redis_pool is not None:
        await redis_pool.disconnect()
        redis_pool = None


# Global tenant context tracking
_current_tenant_context: Optional[Dict[str, Any]] = None


class TenantContext:
    """Tenant context manager for session-scoped tenant isolation."""

    def __init__(self, tenant_id: uuid.UUID, user_email: str, session_id: Optional[str] = None):
        self.tenant_id = tenant_id
        self.user_email = user_email
        self.session_id = session_id or str(uuid.uuid4())

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary representation."""
        return {
            "tenant_id": str(self.tenant_id),
            "user_email": self.user_email,
            "session_id": self.session_id,
        }


def set_current_tenant_context(context: TenantContext) -> None:
    """Set global tenant context for current session."""
    global _current_tenant_context
    _current_tenant_context = context.to_dict()


def get_current_tenant_context() -> Optional[TenantContext]:
    """Get current tenant context."""
    global _current_tenant_context
    if _current_tenant_context:
        return TenantContext(
            tenant_id=uuid.UUID(_current_tenant_context["tenant_id"]),
            user_email=_current_tenant_context["user_email"],
            session_id=_current_tenant_context["session_id"],
        )
    return None


def clear_current_tenant_context() -> None:
    """Clear current tenant context."""
    global _current_tenant_context
    _current_tenant_context = None


@asynccontextmanager
async def get_session(tenant_context: Optional[TenantContext] = None) -> AsyncGenerator[AsyncSession, None]:
    """
    Get database session with proper error handling, cleanup, and tenant context.

    Args:
        tenant_context: Optional tenant context. If not provided, uses current global context.

    Usage:
        async with get_session() as session:
            # Performs operations in current tenant context
            result = await session.execute(query)
            await session.commit()

        # Or with specific tenant context
        context = TenantContext(tenant_id, user_email)
        async with get_session(context) as session:
            # Performs operations in specified tenant context
            result = await session.execute(query)
            await session.commit()
    """
    session = AsyncSessionLocal()
    try:
        # Use provided context or current global context
        context = tenant_context or get_current_tenant_context()

        if context:
            # Set tenant context using RLS helper function
            await session.execute(
                text("SELECT set_tenant_context(:tenant_id)"),
                {"tenant_id": str(context.tenant_id)}
            )

            # Set session ID for tracking
            await session.execute(
                text("SET app.session_id = :session_id"),
                {"session_id": context.session_id}
            )

            logger.debug(f"Set tenant context: {context.tenant_id} for session {context.session_id}")
        else:
            # Clear tenant context for superuser access
            await session.execute(text("SELECT clear_tenant_context()"))
            logger.debug("Using superuser context (no tenant isolation)")

        yield session
        await session.commit()
    except Exception:
        await session.rollback()
        logger.exception("Database session error, rolling back")
        raise
    finally:
        await session.close()


@asynccontextmanager
async def get_session_for_tenant(
    tenant_id: uuid.UUID,
    user_email: str,
    session_id: Optional[str] = None
) -> AsyncGenerator[AsyncSession, None]:
    """
    Get database session for specific tenant with automatic context management.

    Args:
        tenant_id: The tenant UUID for Row-Level Security
        user_email: User email for session tracking
        session_id: Optional session ID for tracking

    Usage:
        async with get_session_for_tenant(tenant_uuid, "user@example.com") as session:
            # Operations will be scoped to this tenant
            pass
    """
    context = TenantContext(tenant_id, user_email, session_id)
    async with get_session(context) as session:
        yield session


async def switch_tenant_context(
    tenant_id: uuid.UUID,
    user_email: str,
    session_id: Optional[str] = None
) -> bool:
    """
    Switch global tenant context safely with validation.

    Args:
        tenant_id: Target tenant UUID
        user_email: User email for session tracking
        session_id: Optional session ID

    Returns:
        True if switch was successful, False otherwise

    Raises:
        ValueError: If tenant doesn't exist or is inactive
    """
    try:
        # Use a temporary session to validate tenant and update session
        async with get_session() as session:  # No context = superuser access for validation
            result = await session.execute(
                text("SELECT switch_tenant_safely(:tenant_id, :user_email)"),
                {"tenant_id": str(tenant_id), "user_email": user_email}
            )
            success = result.scalar()

            if success:
                # Set global context
                context = TenantContext(tenant_id, user_email, session_id)
                set_current_tenant_context(context)
                logger.info(f"Switched to tenant context: {tenant_id} for user: {user_email}")
                return True
            else:
                logger.warning(f"Failed to switch to tenant: {tenant_id}")
                return False

    except Exception as e:
        logger.error(f"Error switching tenant context: {e}")
        raise ValueError(f"Failed to switch to tenant {tenant_id}: {e}")


async def get_available_tenants(user_email: str) -> list[Dict[str, Any]]:
    """
    Get list of available tenants for a user.

    Args:
        user_email: User email to check tenant access

    Returns:
        List of tenant information dictionaries
    """
    async with get_session() as session:  # Superuser access to query all tenants
        result = await session.execute(
            text("""
                SELECT id, name, display_name, description, is_active
                FROM tenants
                WHERE is_deleted = false
                AND (owner_email = :user_email OR :user_email = 'user@example.com')
                ORDER BY name
            """),
            {"user_email": user_email}
        )

        tenants = []
        for row in result:
            tenants.append({
                "id": str(row.id),
                "name": row.name,
                "display_name": row.display_name,
                "description": row.description,
                "is_active": row.is_active,
            })

        return tenants


async def get_current_session_info() -> Optional[Dict[str, Any]]:
    """
    Get current session and tenant information.

    Returns:
        Session information dictionary or None if no active session
    """
    context = get_current_tenant_context()
    if not context:
        return None

    async with get_session() as session:
        result = await session.execute(
            text("""
                SELECT ts.session_id, ts.tenant_id, ts.user_email,
                       ts.last_activity, ts.expires_at,
                       t.name as tenant_name, t.display_name
                FROM tenant_sessions ts
                JOIN tenants t ON ts.tenant_id = t.id
                WHERE ts.session_id = :session_id
                AND ts.expires_at > NOW()
            """),
            {"session_id": context.session_id}
        )

        row = result.first()
        if row:
            return {
                "session_id": row.session_id,
                "tenant_id": str(row.tenant_id),
                "tenant_name": row.tenant_name,
                "tenant_display_name": row.display_name,
                "user_email": row.user_email,
                "last_activity": row.last_activity,
                "expires_at": row.expires_at,
            }

        return None


async def extend_current_session(hours: int = 24) -> bool:
    """
    Extend current session expiration.

    Args:
        hours: Number of hours to extend session

    Returns:
        True if session was extended successfully
    """
    context = get_current_tenant_context()
    if not context:
        return False

    async with get_session() as session:
        result = await session.execute(
            text("""
                UPDATE tenant_sessions
                SET last_activity = NOW(),
                    expires_at = NOW() + INTERVAL ':hours hours',
                    updated_at = NOW()
                WHERE session_id = :session_id
                RETURNING id
            """),
            {"session_id": context.session_id, "hours": hours}
        )

        return result.rowcount > 0


async def cleanup_expired_sessions() -> int:
    """
    Clean up expired tenant sessions.

    Returns:
        Number of sessions cleaned up
    """
    async with get_session() as session:  # Superuser access
        result = await session.execute(
            text("DELETE FROM tenant_sessions WHERE expires_at < NOW()")
        )

        count = result.rowcount
        if count > 0:
            logger.info(f"Cleaned up {count} expired tenant sessions")

        return count


async def init_database() -> None:
    """
    Initialize database schema and extensions.

    This function should be called during application startup to ensure
    the database is properly configured with required extensions.
    """
    from .models import Base  # Import here to avoid circular imports

    async with async_engine.begin() as conn:
        # Enable required PostgreSQL extensions
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS ltree"))
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS pg_trgm"))

        # Create all tables
        await conn.run_sync(Base.metadata.create_all)

        logger.info("Database initialized successfully")


async def check_database_connection() -> bool:
    """
    Check if database connection is working.

    Returns:
        bool: True if connection is successful, False otherwise
    """
    try:
        async with get_session() as session:
            await session.execute(text("SELECT 1"))
            logger.info("Database connection check successful")
            return True
    except Exception as e:
        logger.error(f"Database connection check failed: {e}")
        return False


async def check_redis_connection() -> bool:
    """
    Check if Redis connection is working.

    Returns:
        bool: True if connection is successful, False otherwise
    """
    try:
        redis_client = await init_redis()
        await redis_client.ping()
        logger.info("Redis connection check successful")
        return True
    except Exception as e:
        logger.error(f"Redis connection check failed: {e}")
        return False


async def cleanup_connections() -> None:
    """Clean up all database and Redis connections."""
    await async_engine.dispose()
    await close_redis()
    logger.info("All connections cleaned up")


# Health check function
async def health_check() -> dict:
    """
    Perform health check on all storage connections.

    Returns:
        dict: Status of database and Redis connections
    """
    db_status = await check_database_connection()
    redis_status = await check_redis_connection()

    return {
        "database": "healthy" if db_status else "unhealthy",
        "redis": "healthy" if redis_status else "unhealthy",
        "overall": "healthy" if db_status and redis_status else "unhealthy",
    }