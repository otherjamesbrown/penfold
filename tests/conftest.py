"""Global test configuration for Penfold test suite."""

import os
import sys
import pytest
import asyncio
from pathlib import Path

# Add the project root to Python path for imports
project_root = Path(__file__).parent.parent
sys.path.insert(0, str(project_root))

# Set test environment
os.environ["PENFOLD_ENV"] = "testing"

# Import test fixtures
from tests.fixtures.database import *


@pytest.fixture(scope="session", autouse=True)
def setup_test_environment():
    """Setup test environment variables and configuration."""
    # Ensure we're using test database
    os.environ["DB_NAME"] = "penfold_test"
    os.environ["REDIS_DB"] = "15"  # Use dedicated Redis DB for tests

    # Disable SQL echo during tests unless explicitly requested
    if "PYTEST_VERBOSE_SQL" not in os.environ:
        os.environ["SQLALCHEMY_ECHO"] = "false"

    yield

    # Cleanup can go here if needed


@pytest.fixture(autouse=True)
def isolate_tests():
    """Ensure test isolation by resetting any global state."""
    yield
    # Reset any global state that might affect other tests


# Custom markers for test categorization
def pytest_configure(config):
    """Configure custom pytest markers."""
    config.addinivalue_line("markers", "slow: mark test as slow running")
    config.addinivalue_line("markers", "unit: mark test as unit test")
    config.addinivalue_line("markers", "integration: mark test as integration test")
    config.addinivalue_line("markers", "contract: mark test as contract test")
    config.addinivalue_line("markers", "performance: mark test as performance test")
    config.addinivalue_line("markers", "requires_db: mark test as requiring database")
    config.addinivalue_line("markers", "requires_redis: mark test as requiring Redis")


def pytest_collection_modifyitems(config, items):
    """Automatically mark tests based on their location."""
    for item in items:
        # Auto-mark based on test file location
        if "unit" in str(item.fspath):
            item.add_marker(pytest.mark.unit)
        elif "integration" in str(item.fspath):
            item.add_marker(pytest.mark.integration)
            item.add_marker(pytest.mark.requires_db)
        elif "contract" in str(item.fspath):
            item.add_marker(pytest.mark.contract)
            item.add_marker(pytest.mark.requires_db)
        elif "performance" in str(item.fspath):
            item.add_marker(pytest.mark.performance)
            item.add_marker(pytest.mark.slow)

        # Mark database-related tests
        if any(fixture in item.fixturenames for fixture in ["test_session", "test_engine"]):
            item.add_marker(pytest.mark.requires_db)

        # Mark Redis-related tests
        if "redis" in str(item.fspath).lower():
            item.add_marker(pytest.mark.requires_redis)


# Asyncio event loop configuration
@pytest.fixture(scope="session")
def event_loop_policy():
    """Set the event loop policy for the test session."""
    return asyncio.DefaultEventLoopPolicy()


# Performance testing utilities
@pytest.fixture
def benchmark_timer():
    """Simple benchmark timer for performance tests."""
    import time

    class Timer:
        def __init__(self):
            self.start_time = None
            self.end_time = None

        def start(self):
            self.start_time = time.perf_counter()
            return self

        def stop(self):
            self.end_time = time.perf_counter()
            return self

        @property
        def elapsed(self):
            if self.start_time is None or self.end_time is None:
                return None
            return self.end_time - self.start_time

        @property
        def elapsed_ms(self):
            elapsed = self.elapsed
            return elapsed * 1000 if elapsed is not None else None

    return Timer


# Test data generation utilities
@pytest.fixture
def data_generator():
    """Utility for generating test data."""
    import random
    import string
    import uuid

    class DataGenerator:
        @staticmethod
        def random_string(length=10):
            return ''.join(random.choices(string.ascii_letters + string.digits, k=length))

        @staticmethod
        def random_email():
            username = DataGenerator.random_string(8)
            domain = DataGenerator.random_string(6)
            return f"{username}@{domain}.com"

        @staticmethod
        def random_uuid():
            return str(uuid.uuid4())

        @staticmethod
        def random_content_hash():
            return ''.join(random.choices(string.hexdigits.lower(), k=64))

        @staticmethod
        def random_embedding(dimension=768):
            return [random.uniform(-1.0, 1.0) for _ in range(dimension)]

    return DataGenerator


# Skip conditions
def pytest_runtest_setup(item):
    """Setup function run before each test."""
    # Skip database tests if DB_SKIP_TESTS is set
    if "requires_db" in [marker.name for marker in item.iter_markers()]:
        if os.environ.get("DB_SKIP_TESTS"):
            pytest.skip("Database tests skipped (DB_SKIP_TESTS set)")

    # Skip Redis tests if REDIS_SKIP_TESTS is set
    if "requires_redis" in [marker.name for marker in item.iter_markers()]:
        if os.environ.get("REDIS_SKIP_TESTS"):
            pytest.skip("Redis tests skipped (REDIS_SKIP_TESTS set)")

    # Skip slow tests unless explicitly requested
    if "slow" in [marker.name for marker in item.iter_markers()]:
        if not item.config.getoption("--runslow", default=False):
            pytest.skip("Slow test skipped (use --runslow to run)")


def pytest_addoption(parser):
    """Add custom command line options."""
    parser.addoption(
        "--runslow",
        action="store_true",
        default=False,
        help="run slow tests"
    )
    parser.addoption(
        "--skip-db",
        action="store_true",
        default=False,
        help="skip database tests"
    )
    parser.addoption(
        "--skip-redis",
        action="store_true",
        default=False,
        help="skip Redis tests"
    )