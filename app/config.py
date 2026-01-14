"""
Configuration management for Meeting Pipeline

Centralizes all application settings with environment variable support
and validation for required configuration values.
"""
from pydantic import BaseSettings, validator
from typing import List, Optional
import os
from pathlib import Path


class Settings(BaseSettings):
    """Application configuration settings"""

    # Application settings
    HOST: str = "localhost"
    PORT: int = 8000
    DEBUG: bool = True
    ALLOWED_ORIGINS: List[str] = ["http://localhost:3000", "http://localhost:8000"]

    # Database settings
    DATABASE_URL: str = "postgresql://localhost/penfold_dev"
    DATABASE_POOL_SIZE: int = 20
    DATABASE_MAX_OVERFLOW: int = 30

    # File storage settings
    UPLOAD_DIR: Path = Path.home() / "penfold-uploads"
    PROCESSED_DIR: Path = Path.home() / "penfold-processed"
    MAX_UPLOAD_SIZE: int = 2147483648  # 2GB
    CHUNK_SIZE: int = 10 * 1024 * 1024  # 10MB chunks for TUS

    # Speech-to-Text settings
    WHISPER_MODEL_SIZE: str = "large-v3"
    STT_CONFIDENCE_THRESHOLD: float = 0.8
    GOOGLE_CLOUD_CREDENTIALS_PATH: Optional[str] = None
    USE_LOCAL_STT_ONLY: bool = False

    # Privacy and security
    ENCRYPTION_KEY_PATH: Path = Path.home() / ".penfold" / "encryption-keys"
    JWT_SECRET_KEY: str = "dev-secret-key-change-in-production"
    JWT_ALGORITHM: str = "HS256"
    JWT_EXPIRATION_HOURS: int = 24

    # Processing settings
    MAX_CONCURRENT_JOBS: int = 20
    MAX_CPU_INTENSIVE_JOBS: int = 4
    JOB_TIMEOUT_SECONDS: int = 7200  # 2 hours
    FIFO_QUEUE_ENABLED: bool = True

    # Event system (optional Redis for external events)
    REDIS_URL: Optional[str] = None

    # AI/Embeddings settings
    OPENAI_API_KEY: Optional[str] = None
    EMBEDDING_MODEL: str = "text-embedding-3-small"
    EMBEDDING_DIMENSION: int = 1536

    # Performance tuning
    PROCESSING_BATCH_SIZE: int = 10
    SEARCH_RESULTS_LIMIT: int = 50
    VECTOR_SEARCH_LIMIT: int = 100

    @validator("UPLOAD_DIR", "PROCESSED_DIR", "ENCRYPTION_KEY_PATH")
    def ensure_directories_exist(cls, v):
        """Ensure required directories exist"""
        path = Path(v)
        path.mkdir(parents=True, exist_ok=True)
        return path

    @validator("DATABASE_URL")
    def validate_database_url(cls, v):
        """Ensure database URL is valid"""
        if not v or not v.startswith(("postgresql://", "postgresql+asyncpg://")):
            raise ValueError("DATABASE_URL must be a valid PostgreSQL connection string")
        return v

    @validator("MAX_UPLOAD_SIZE")
    def validate_max_upload_size(cls, v):
        """Ensure max upload size is reasonable"""
        if v > 5 * 1024 * 1024 * 1024:  # 5GB limit
            raise ValueError("MAX_UPLOAD_SIZE cannot exceed 5GB")
        return v

    @validator("STT_CONFIDENCE_THRESHOLD")
    def validate_confidence_threshold(cls, v):
        """Ensure confidence threshold is between 0 and 1"""
        if not 0.0 <= v <= 1.0:
            raise ValueError("STT_CONFIDENCE_THRESHOLD must be between 0.0 and 1.0")
        return v

    class Config:
        env_file = ".env"
        case_sensitive = True


# Create global settings instance
settings = Settings()


def get_database_url() -> str:
    """Get database URL with async driver"""
    if settings.DATABASE_URL.startswith("postgresql://"):
        return settings.DATABASE_URL.replace("postgresql://", "postgresql+asyncpg://", 1)
    return settings.DATABASE_URL


def get_upload_path(privacy_level: str) -> Path:
    """Get appropriate upload path based on privacy level"""
    base_path = settings.UPLOAD_DIR
    if privacy_level == "confidential":
        return base_path / "encrypted"
    elif privacy_level in ["team_only", "organization"]:
        return base_path / "secure"
    else:
        return base_path / "general"


def get_encryption_key_path(privacy_level: str) -> Path:
    """Get encryption key path for privacy level"""
    return settings.ENCRYPTION_KEY_PATH / f"{privacy_level}.key"


def is_production() -> bool:
    """Check if running in production environment"""
    return os.getenv("ENVIRONMENT", "development").lower() == "production"