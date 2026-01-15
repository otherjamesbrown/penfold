#!/usr/bin/env python3
"""
Validation script for Penfold observability framework.

This script validates that all observability components are working correctly
and can be integrated into existing Penfold agents.
"""

import asyncio
import json
import logging
import sys
import time
from datetime import datetime
from typing import Dict, Any

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def test_imports():
    """Test that all observability components can be imported."""
    try:
        # Core services
        from observability_lib.services import (
            monitor_agent, AgentHealthService, MetricsCollector,
            AlertManagerService, SystemMonitorService,
            BusinessValueTracker, UsageAnalyticsService, ROICalculatorService,
            WorkflowTracker, PerformanceAnalyzer, QualityTracker
        )

        # API endpoints
        from observability_lib.api.dashboard_endpoints import router as dashboard_router
        from observability_lib.api.workflow_endpoints import router as workflow_router
        from observability_lib.api.performance_endpoints import router as performance_router
        from observability_lib.api.alert_endpoints import router as alert_router

        # CLI tools
        import observability_lib.cli.monitor
        import observability_lib.cli.debug

        # Configuration
        from observability_lib.config import ObservabilitySettings

        logger.info("✅ All imports successful")
        return True

    except ImportError as e:
        logger.error(f"❌ Import failed: {e}")
        return False


def test_configuration():
    """Test configuration loading."""
    try:
        from observability_lib.config import ObservabilitySettings

        # Test default configuration
        settings = ObservabilitySettings()
        assert settings.dashboard_host == "0.0.0.0"
        assert settings.dashboard_port == 8888
        assert settings.enable_dashboard is True

        # Test custom configuration
        custom_settings = ObservabilitySettings(
            dashboard_port=9999,
            enable_alerting=False,
            max_events_per_agent_per_hour=5000
        )
        assert custom_settings.dashboard_port == 9999
        assert custom_settings.enable_alerting is False
        assert custom_settings.max_events_per_agent_per_hour == 5000

        logger.info("✅ Configuration validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ Configuration test failed: {str(e)}")
        return False


def test_instrumentation():
    """Test agent instrumentation decorator."""
    try:
        from observability_lib.services import monitor_agent

        @monitor_agent("test_validation_agent")
        class TestAgent:
            def __init__(self):
                self.processed = 0

            def process_item_sync(self, item: Dict[str, Any]):
                """Test synchronous processing method."""
                self.processed += 1
                return {"status": "success", "processed_count": self.processed}

        # Test that decorator doesn't break functionality
        agent = TestAgent()

        # Run sync test
        result = agent.process_item_sync({"test": "data"})
        assert result["status"] == "success"
        assert result["processed_count"] == 1

        logger.info("✅ Instrumentation validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ Instrumentation test failed: {str(e)}")
        return False


def test_data_models():
    """Test observability data models."""
    try:
        from observability_lib.models.agent_metrics import (
            Agent, AgentMetric, WorkflowEvent, AlertThreshold, AlertEvent
        )

        # Test model creation (without database)
        now = datetime.utcnow()

        # These would normally be created via SQLAlchemy sessions
        # but we can validate the model definitions
        assert hasattr(Agent, '__tablename__')
        assert hasattr(AgentMetric, '__tablename__')
        assert hasattr(WorkflowEvent, '__tablename__')

        logger.info("✅ Data models validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ Data models test failed: {e}")
        return False


def test_api_definitions():
    """Test API endpoint definitions."""
    try:
        # Test that API modules can be imported
        import observability_lib.api.dashboard_endpoints
        import observability_lib.api.workflow_endpoints
        import observability_lib.api.performance_endpoints
        import observability_lib.api.alert_endpoints

        # Test that routers are defined
        assert hasattr(observability_lib.api.dashboard_endpoints, 'router')
        assert hasattr(observability_lib.api.workflow_endpoints, 'router')
        assert hasattr(observability_lib.api.performance_endpoints, 'router')
        assert hasattr(observability_lib.api.alert_endpoints, 'router')

        logger.info("✅ API definitions validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ API definitions test failed: {str(e)}")
        return False


def test_cli_tools():
    """Test CLI tools are functional."""
    try:
        # Test monitor CLI
        import observability_lib.cli.monitor as monitor_cli
        assert hasattr(monitor_cli, 'cli')
        assert hasattr(monitor_cli, 'status')
        assert hasattr(monitor_cli, 'health')

        # Test debug CLI
        import observability_lib.cli.debug as debug_cli
        assert hasattr(debug_cli, 'cli')
        assert hasattr(debug_cli, 'list_workflows')
        assert hasattr(debug_cli, 'trace')

        logger.info("✅ CLI tools validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ CLI tools test failed: {e}")
        return False


async def test_service_initialization():
    """Test that services can be initialized (without database connection)."""
    try:
        from observability_lib.config import ObservabilitySettings
        from observability_lib.services import (
            AgentHealthService, MetricsCollector, AlertManagerService,
            SystemMonitorService, BusinessValueTracker, UsageAnalyticsService,
            ROICalculatorService, WorkflowTracker, PerformanceAnalyzer, QualityTracker
        )

        # Use test settings (won't actually connect to database)
        settings = ObservabilitySettings(
            database_url="postgresql://test:test@localhost:5432/test",
            enable_dashboard=False,
            enable_alerting=False
        )

        # Test that services can be instantiated
        services = [
            AgentHealthService(settings),
            MetricsCollector(settings),
            AlertManagerService(settings),
            SystemMonitorService(settings),
            BusinessValueTracker(settings),
            UsageAnalyticsService(settings),
            ROICalculatorService(settings),
            WorkflowTracker(settings),
            PerformanceAnalyzer(settings),
            QualityTracker(settings)
        ]

        # All services should be instantiated without errors
        assert len(services) == 10

        logger.info("✅ Service initialization validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ Service initialization test failed: {e}")
        return False


def test_enum_definitions():
    """Test enum definitions for type safety."""
    try:
        from observability_lib.services.agent_health import HealthStatus, HealthTrendDirection
        from observability_lib.services.alert_manager import AlertSeverity, AlertCategory, AlertStatus
        from observability_lib.services.system_monitor import ResourceType
        from observability_lib.services.business_value_tracker import KpiStatus, TrendDirection
        from observability_lib.services.usage_analytics import EngagementLevel, FeatureCategory
        from observability_lib.services.roi_calculator import ValueCategory, CalculationMethod

        # Test that enums have expected values
        assert hasattr(HealthStatus, 'EXCELLENT')
        assert hasattr(AlertSeverity, 'CRITICAL')
        assert hasattr(ResourceType, 'CPU')
        assert hasattr(EngagementLevel, 'HIGH')

        logger.info("✅ Enum definitions validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ Enum definitions test failed: {e}")
        return False


async def test_performance_requirements():
    """Test performance characteristics (without full infrastructure)."""
    try:
        from observability_lib.services import monitor_agent

        @monitor_agent("performance_test_agent")
        class PerformanceAgent:
            async def lightweight_operation(self):
                await asyncio.sleep(0.001)  # 1ms operation
                return "completed"

        agent = PerformanceAgent()

        # Measure overhead of instrumentation
        start_time = time.time()
        for _ in range(100):
            await agent.lightweight_operation()
        total_time = time.time() - start_time

        avg_time_per_operation = (total_time / 100) * 1000  # Convert to ms

        # Should be minimal overhead (just the decorator)
        logger.info(f"Average time per operation: {avg_time_per_operation:.2f}ms")

        # Basic performance validation - should complete quickly
        assert total_time < 5.0  # Less than 5 seconds for 100 operations

        logger.info("✅ Performance requirements validation successful")
        return True

    except Exception as e:
        logger.error(f"❌ Performance requirements test failed: {e}")
        return False


def print_integration_guide():
    """Print integration guide for existing agents."""
    guide = """
🔧 PENFOLD OBSERVABILITY INTEGRATION GUIDE

To add observability to existing Penfold agents:

1. Install observability framework:
   pip install -e observability_lib/

2. Add instrumentation to your agent:

   from observability_lib.services import monitor_agent

   @monitor_agent("your_agent_name")
   class YourAgent:
       async def process_emails(self, emails):
           # Your existing code here
           pass

3. Configure observability settings:

   from observability_lib.config import ObservabilitySettings

   settings = ObservabilitySettings(
       database_url="your_database_url",
       enable_dashboard=True,
       enable_alerting=True
   )

4. Start observability services (in your main application):

   from observability_lib.services import AgentHealthService

   health_service = AgentHealthService(settings)
   await health_service.start()

5. Access monitoring tools:

   # CLI monitoring
   python -m observability_lib.cli.monitor status
   python -m observability_lib.cli.debug list-workflows

   # Dashboard (if enabled)
   Open http://localhost:8888

6. Set up alerts (optional):

   from observability_lib.services import AlertManagerService, AlertRule

   alert_manager = AlertManagerService(settings)
   await alert_manager.start()

   # Add custom alert rules as needed

For more details, see docs/integration_guide.md
"""
    print(guide)


async def main():
    """Run all validation tests."""
    logger.info("🚀 Starting Penfold Observability Framework Validation")

    tests = [
        ("Import Validation", test_imports),
        ("Configuration Validation", test_configuration),
        ("Instrumentation Validation", test_instrumentation),
        ("Data Models Validation", test_data_models),
        ("API Definitions Validation", test_api_definitions),
        ("CLI Tools Validation", test_cli_tools),
        ("Service Initialization", test_service_initialization),
        ("Enum Definitions", test_enum_definitions),
        ("Performance Requirements", test_performance_requirements),
    ]

    results = []

    for test_name, test_func in tests:
        logger.info(f"\n📋 Running {test_name}...")

        if asyncio.iscoroutinefunction(test_func):
            success = await test_func()
        else:
            success = test_func()

        results.append((test_name, success))

    # Summary
    logger.info("\n" + "="*60)
    logger.info("VALIDATION RESULTS SUMMARY")
    logger.info("="*60)

    passed = 0
    failed = 0

    for test_name, success in results:
        status = "✅ PASS" if success else "❌ FAIL"
        logger.info(f"{status}: {test_name}")
        if success:
            passed += 1
        else:
            failed += 1

    logger.info(f"\nTotal: {len(results)} tests")
    logger.info(f"Passed: {passed}")
    logger.info(f"Failed: {failed}")

    if failed == 0:
        logger.info("\n🎉 ALL VALIDATIONS PASSED! Observability framework is ready for integration.")
        print_integration_guide()
        return 0
    else:
        logger.error(f"\n❌ {failed} validation(s) failed. Please check the errors above.")
        return 1


if __name__ == "__main__":
    exit_code = asyncio.run(main())