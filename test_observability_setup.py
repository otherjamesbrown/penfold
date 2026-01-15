#!/usr/bin/env python3
"""
Test script to validate observability framework setup.

This script verifies that all core components can be imported and
basic configuration is working properly.
"""

import sys
import os
from pathlib import Path

def test_observability_setup():
    """Test basic observability framework setup."""
    print("🔍 Testing Penfold Observability Framework Setup")
    print("=" * 50)

    # Test configuration import
    try:
        sys.path.insert(0, str(Path(__file__).parent))
        from observability_lib.config import settings, get_settings
        print("✅ Configuration module imported successfully")
        print(f"   Database URL: {settings.database_url}")
        print(f"   Dashboard Port: {settings.dashboard_port}")
        print(f"   Log Level: {settings.log_level}")
    except ImportError as e:
        print(f"❌ Failed to import configuration: {e}")
        return False

    # Test enum imports
    try:
        from observability_lib.models import (
            AgentType, MetricType, EventType, EventStatus,
            LogLevel, ThresholdType, ComparisonOperator, AlertStatus
        )
        print("✅ Enum types imported successfully")
        print(f"   Agent Types: {[t.value for t in AgentType]}")
        print(f"   Metric Types: {[t.value for t in MetricType]}")
    except ImportError as e:
        print(f"❌ Failed to import enum types: {e}")
        return False

    # Test model imports (without database connection)
    try:
        from observability_lib.models import (
            Agent, AgentMetric, DecisionTrace, WorkflowEvent,
            AgentLog, AlertThreshold, AlertEvent
        )
        print("✅ Database models imported successfully")
    except ImportError as e:
        print(f"❌ Failed to import database models: {e}")
        return False

    # Test required dependencies
    dependencies = [
        ("structlog", "Structured logging"),
        ("prometheus_client", "Prometheus metrics"),
        ("sse_starlette", "Server-Sent Events"),
        ("pydantic", "Data validation"),
        ("sqlalchemy", "Database ORM")
    ]

    for dep_name, description in dependencies:
        try:
            __import__(dep_name)
            print(f"✅ {description} ({dep_name}) available")
        except ImportError:
            print(f"❌ {description} ({dep_name}) not available")
            return False

    # Test configuration validation
    try:
        from observability_lib.config import create_test_settings
        test_settings = create_test_settings()
        print("✅ Test configuration created successfully")
        print(f"   Test Database URL: {test_settings.database_url}")
    except Exception as e:
        print(f"❌ Configuration validation failed: {e}")
        return False

    print("\n🎉 Observability framework setup validation PASSED!")
    print("\nNext steps:")
    print("1. Set up TimescaleDB extension in PostgreSQL")
    print("2. Run observability_schema.sql to create database tables")
    print("3. Configure agent instrumentation")
    print("4. Start dashboard server")

    return True

if __name__ == "__main__":
    success = test_observability_setup()
    sys.exit(0 if success else 1)