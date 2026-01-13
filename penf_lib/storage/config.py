"""Configuration management for Penfold storage layer."""

import os
from typing import Optional
from urllib.parse import quote_plus

try:
    from pydantic_settings import BaseSettings
    from pydantic import Field
except ImportError:
    # Fallback for older pydantic versions
    from pydantic import BaseSettings, Field


class DatabaseConfig(BaseSettings):
    """Database connection configuration."""

    # Database connection
    host: str = Field(default="localhost", env="DB_HOST")
    port: int = Field(default=5432, env="DB_PORT")
    name: str = Field(default="penfold_dev", env="DB_NAME")
    user: str = Field(default_factory=lambda: os.getenv("USER", "postgres"), env="DB_USER")
    password: str = Field(default="", env="DB_PASSWORD")

    # Connection pool settings
    pool_size: int = Field(default=10, env="DB_POOL_SIZE")
    max_overflow: int = Field(default=5, env="DB_MAX_OVERFLOW")
    pool_recycle: int = Field(default=3600, env="DB_POOL_RECYCLE")  # 1 hour

    # Test database settings
    test_name: str = Field(default="penfold_test", env="DB_TEST_NAME")

    class Config:
        env_prefix = ""
        case_sensitive = False

    @property
    def database_url(self) -> str:
        """Generate database URL for SQLAlchemy."""
        password_part = f":{quote_plus(self.password)}" if self.password else ""
        return f"postgresql+asyncpg://{self.user}{password_part}@{self.host}:{self.port}/{self.name}"

    @property
    def test_database_url(self) -> str:
        """Generate test database URL for SQLAlchemy."""
        password_part = f":{quote_plus(self.password)}" if self.password else ""
        return f"postgresql+asyncpg://{self.user}{password_part}@{self.host}:{self.port}/{self.test_name}"

    @property
    def sync_database_url(self) -> str:
        """Generate synchronous database URL for migrations."""
        password_part = f":{quote_plus(self.password)}" if self.password else ""
        return f"postgresql://{self.user}{password_part}@{self.host}:{self.port}/{self.name}"


class RedisConfig(BaseSettings):
    """Redis connection configuration."""

    host: str = Field(default="localhost", env="REDIS_HOST")
    port: int = Field(default=6379, env="REDIS_PORT")
    db: int = Field(default=0, env="REDIS_DB")
    password: Optional[str] = Field(default=None, env="REDIS_PASSWORD")

    # Connection pool settings
    max_connections: int = Field(default=20, env="REDIS_MAX_CONNECTIONS")
    retry_on_timeout: bool = Field(default=True, env="REDIS_RETRY_ON_TIMEOUT")

    class Config:
        env_prefix = ""
        case_sensitive = False

    @property
    def redis_url(self) -> str:
        """Generate Redis URL."""
        password_part = f":{self.password}@" if self.password else ""
        return f"redis://{password_part}{self.host}:{self.port}/{self.db}"


class StorageConfig(BaseSettings):
    """General storage configuration."""

    # Tenant isolation
    default_tenant_id: str = Field(default="default", env="PENFOLD_TENANT_ID")

    # Processing retention
    processing_result_retention_days: int = Field(default=30, env="PROCESSING_RETENTION_DAYS")

    # Vector settings
    embedding_dimension: int = Field(default=768, env="EMBEDDING_DIMENSION")
    vector_index_type: str = Field(default="hnsw", env="VECTOR_INDEX_TYPE")

    # HNSW index parameters (from spec analysis improvements)
    hnsw_m: int = Field(default=16, env="HNSW_M")  # Number of connections per node
    hnsw_ef_construction: int = Field(default=200, env="HNSW_EF_CONSTRUCTION")  # Build-time parameter

    # Performance settings
    query_timeout: int = Field(default=30, env="QUERY_TIMEOUT")  # seconds
    max_batch_size: int = Field(default=1000, env="MAX_BATCH_SIZE")

    class Config:
        env_prefix = ""
        case_sensitive = False


class Config:
    """Main configuration class combining all settings."""

    def __init__(self) -> None:
        """Initialize configuration from environment."""
        self.database = DatabaseConfig()
        self.redis = RedisConfig()
        self.storage = StorageConfig()

    @property
    def is_development(self) -> bool:
        """Check if running in development mode."""
        return os.getenv("PENFOLD_ENV", "development").lower() == "development"

    @property
    def is_testing(self) -> bool:
        """Check if running in testing mode."""
        return os.getenv("PENFOLD_ENV", "development").lower() == "testing"

    @property
    def is_production(self) -> bool:
        """Check if running in production mode."""
        return os.getenv("PENFOLD_ENV", "development").lower() == "production"


# Global configuration instance
config = Config()