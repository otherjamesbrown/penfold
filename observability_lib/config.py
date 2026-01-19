"""Configuration module for Penfold observability framework.

This module provides configuration management using Pydantic Settings
for all observability framework components including database connections,
metrics collection, dashboard settings, TimescaleDB policies, and performance limits.

Environment Variables:
    All settings can be overridden via environment variables with the
    OBSERVABILITY_ prefix. For example:
    - OBSERVABILITY_DATABASE_URL
    - OBSERVABILITY_LOG_LEVEL
    - OBSERVABILITY_METRICS_BUFFER_SIZE

Configuration Files:
    Settings can be loaded from .env.observability file in the project root.

Example:
    from observability_lib.config import settings

    # Access configuration
    print(settings.database_url)
    print(settings.metrics_buffer_size)
"""

from typing import List, Optional
from pydantic import Field, validator
from pydantic_settings import BaseSettings


class ObservabilitySettings(BaseSettings):
    """Configuration settings for the observability framework.

    This class defines all configuration parameters for the observability
    system including database connections, metrics collection, dashboard
    settings, TimescaleDB optimization, and performance constraints.
    """

    # Database Connection Settings
    database_url: str = Field(
        default="postgresql://localhost/penfold_dev",
        description="PostgreSQL database connection URL with TimescaleDB extension"
    )

    # Logging Configuration
    log_level: str = Field(
        default="INFO",
        description="Log level for observability framework components"
    )

    # Metrics Collection Settings
    metrics_buffer_size: int = Field(
        default=100,
        ge=10,
        le=10000,
        description="Maximum number of metrics to buffer before flushing to database"
    )

    metrics_flush_interval: float = Field(
        default=5.0,
        ge=1.0,
        le=300.0,
        description="Interval in seconds to flush metrics buffer to database"
    )

    # Dashboard Configuration
    dashboard_host: str = Field(
        default="0.0.0.0",
        description="Host address for the observability dashboard server"
    )

    dashboard_port: int = Field(
        default=8001,
        ge=1024,
        le=65535,
        description="Port number for the observability dashboard server"
    )

    dashboard_reload: bool = Field(
        default=True,
        description="Enable auto-reload for dashboard development"
    )

    # TimescaleDB Compression Settings
    enable_compression: bool = Field(
        default=True,
        description="Enable TimescaleDB compression for historical data"
    )

    compression_interval_days: int = Field(
        default=7,
        ge=1,
        le=365,
        description="Number of days after which to compress time-series data"
    )

    # TimescaleDB Retention Policies
    retention_interval_days: int = Field(
        default=90,
        ge=1,
        le=3650,
        description="Number of days to retain time-series data before deletion"
    )

    # Continuous Aggregates Configuration
    enable_continuous_aggregates: bool = Field(
        default=True,
        description="Enable TimescaleDB continuous aggregates for dashboard performance"
    )

    aggregate_refresh_interval_hours: int = Field(
        default=1,
        ge=1,
        le=24,
        description="Refresh interval in hours for continuous aggregates"
    )

    # Performance Limits
    max_decision_trace_size: int = Field(
        default=10000,
        ge=1000,
        le=100000,
        description="Maximum size in characters for decision trace context and reasoning"
    )

    max_workflow_depth: int = Field(
        default=10,
        ge=1,
        le=50,
        description="Maximum depth for nested workflows to prevent infinite recursion"
    )

    max_concurrent_agents: int = Field(
        default=50,
        ge=1,
        le=1000,
        description="Maximum number of concurrent agent connections"
    )

    # Alert Management
    alert_check_interval: int = Field(
        default=60,
        ge=10,
        le=3600,
        description="Interval in seconds for evaluating alert thresholds"
    )

    alert_default_thresholds: str = Field(
        default="performance:30000,error_rate:5.0",
        description="Default alert thresholds as comma-separated key:value pairs"
    )

    # System Resource Monitoring
    monitor_system_resources: bool = Field(
        default=True,
        description="Enable system resource monitoring (CPU, memory, disk)"
    )

    resource_check_interval: int = Field(
        default=30,
        ge=5,
        le=300,
        description="Interval in seconds for system resource checks"
    )

    # Data Export Settings
    enable_metrics_export: bool = Field(
        default=False,
        description="Enable Prometheus metrics export endpoint"
    )

    metrics_export_port: int = Field(
        default=8002,
        ge=1024,
        le=65535,
        description="Port for Prometheus metrics export endpoint"
    )

    # Development Settings
    enable_debug_logging: bool = Field(
        default=False,
        description="Enable verbose debug logging for development"
    )

    simulate_slow_queries: bool = Field(
        default=False,
        description="Add artificial delays to simulate slow database queries (development only)"
    )

    # Tenant Configuration
    default_tenant_id: Optional[str] = Field(
        default=None,
        description="Default tenant ID for single-tenant deployments"
    )

    @validator("log_level")
    def validate_log_level(cls, v):
        """Validate log level is one of the standard Python logging levels."""
        valid_levels = ["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"]
        if v.upper() not in valid_levels:
            raise ValueError(f"log_level must be one of {valid_levels}")
        return v.upper()

    @validator("alert_default_thresholds")
    def validate_alert_thresholds(cls, v):
        """Validate alert thresholds format."""
        try:
            pairs = v.split(",")
            for pair in pairs:
                if ":" not in pair:
                    raise ValueError(f"Invalid threshold format: {pair}")
                key, value = pair.split(":", 1)
                float(value)  # Ensure value is numeric
        except ValueError as e:
            raise ValueError(f"Invalid alert_default_thresholds format: {e}")
        return v

    @validator("compression_interval_days")
    def validate_compression_before_retention(cls, v, values):
        """Ensure compression happens before retention."""
        if "retention_interval_days" in values and v >= values["retention_interval_days"]:
            raise ValueError("compression_interval_days must be less than retention_interval_days")
        return v

    def parse_alert_thresholds(self) -> dict[str, float]:
        """Parse alert thresholds string into dictionary.

        Returns:
            Dictionary mapping threshold names to values
        """
        thresholds = {}
        for pair in self.alert_default_thresholds.split(","):
            key, value = pair.split(":", 1)
            thresholds[key.strip()] = float(value.strip())
        return thresholds

    def get_database_pool_settings(self) -> dict:
        """Get recommended database pool settings for asyncpg.

        Returns:
            Dictionary with pool configuration parameters
        """
        return {
            "min_size": 5,
            "max_size": min(20, self.max_concurrent_agents),
            "max_queries": 50000,
            "max_inactive_connection_lifetime": 300.0,
            "command_timeout": 60.0,
        }

    class Config:
        """Pydantic configuration for settings management."""

        env_prefix = "OBSERVABILITY_"
        env_file = ".env.observability"
        env_file_encoding = "utf-8"
        case_sensitive = False

        # Enable validation on assignment
        validate_assignment = True

        # Schema extra for documentation
        json_schema_extra = {
            "example": {
                "database_url": "postgresql://localhost/penfold_dev",
                "log_level": "INFO",
                "metrics_buffer_size": 100,
                "metrics_flush_interval": 5.0,
                "dashboard_host": "0.0.0.0",
                "dashboard_port": 8001,
                "enable_compression": True,
                "compression_interval_days": 7,
                "retention_interval_days": 90,
                "max_decision_trace_size": 10000,
                "max_workflow_depth": 10,
                "alert_check_interval": 60,
                "alert_default_thresholds": "performance:30000,error_rate:5.0"
            }
        }


# Global settings instance
settings = ObservabilitySettings()


def get_settings() -> ObservabilitySettings:
    """Get the global observability settings instance.

    This function provides a way to access settings that can be easily
    mocked or overridden in tests.

    Returns:
        The global ObservabilitySettings instance
    """
    return settings


def create_test_settings(**overrides) -> ObservabilitySettings:
    """Create settings instance for testing with specific overrides.

    Args:
        **overrides: Configuration values to override

    Returns:
        ObservabilitySettings instance with test configuration

    Example:
        test_settings = create_test_settings(
            database_url="postgresql://localhost/penfold_test",
            metrics_buffer_size=10,
            enable_debug_logging=True
        )
    """
    # Default test configuration
    test_defaults = {
        "database_url": "postgresql://localhost/penfold_test",
        "log_level": "DEBUG",
        "metrics_buffer_size": 10,
        "metrics_flush_interval": 1.0,
        "dashboard_port": 8888,  # Test port
        "enable_compression": False,
        "retention_interval_days": 1,
        "enable_debug_logging": True,
        "simulate_slow_queries": False,
        "enable_continuous_aggregates": False,
    }

    # Merge defaults with overrides
    test_config = {**test_defaults, **overrides}

    return ObservabilitySettings(**test_config)