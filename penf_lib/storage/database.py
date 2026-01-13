"""Database configuration and session management."""

import os
from typing import AsyncGenerator, Optional
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine, async_sessionmaker
from sqlalchemy.orm import DeclarativeBase
from sqlalchemy import MetaData


# SQLAlchemy 2.0 style base
class Base(DeclarativeBase):
    """Base class for all SQLAlchemy models."""
    metadata = MetaData()


class DatabaseManager:
    """Manages async database connections and sessions."""

    def __init__(self, database_url: Optional[str] = None):
        """Initialize database manager.

        Args:
            database_url: PostgreSQL connection URL.
                         If None, uses PENF_DATABASE_URL environment variable.
        """
        self.database_url = database_url or os.getenv(
            'PENF_DATABASE_URL',
            'postgresql+asyncpg://penfold:penfold@localhost/penfold'
        )

        self.engine = create_async_engine(
            self.database_url,
            echo=os.getenv('PENF_DB_ECHO', 'false').lower() == 'true',
            pool_pre_ping=True,
            pool_recycle=3600,
        )

        self.async_session_factory = async_sessionmaker(
            self.engine,
            class_=AsyncSession,
            expire_on_commit=False
        )

    async def get_session(self) -> AsyncGenerator[AsyncSession, None]:
        """Get async database session.

        Yields:
            Database session
        """
        async with self.async_session_factory() as session:
            try:
                yield session
            finally:
                await session.close()

    async def create_tables(self):
        """Create all tables."""
        async with self.engine.begin() as conn:
            await conn.run_sync(Base.metadata.create_all)

    async def drop_tables(self):
        """Drop all tables."""
        async with self.engine.begin() as conn:
            await conn.run_sync(Base.metadata.drop_all)

    async def close(self):
        """Close database engine."""
        await self.engine.dispose()


# Global database manager instance
db_manager = DatabaseManager()