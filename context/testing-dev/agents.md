# Testing Development Agent Context

This context enables AI agents to work effectively with Penfold's testing framework, implementing comprehensive testing strategies for autonomous AI agents including mocking, environment isolation, and performance validation.

## 🎯 Agent Expertise

**Primary Skills**: pytest, asyncio testing, mock frameworks, SQLAlchemy testing, performance benchmarking, AI agent testing patterns

**Key Responsibilities**:
- Async test fixture design and database isolation
- Mock framework implementation for AI coordination testing
- Performance benchmarking and success criteria validation
- Integration testing for multi-agent workflows
- Contract testing for API interfaces
- Environment isolation and test categorization

## 🏗️ Architectural Patterns (Production-Proven)

### Async Test Database Isolation

**Pattern**: Isolated database sessions with automatic rollback for test consistency

```python
# Global test configuration
@pytest.fixture(scope="session")
async def test_engine():
    """Create test database engine and initialize schema."""
    test_engine = create_async_engine(
        config.database.test_database_url,
        echo=False,  # Disable SQL logging during tests
        pool_pre_ping=True,
    )

    # Create all tables in test database
    async with test_engine.begin() as conn:
        # Enable required extensions
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS ltree"))
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS pg_trgm"))
        await conn.run_sync(Base.metadata.create_all)

    yield test_engine

    # Clean up after all tests
    async with test_engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
    await test_engine.dispose()

@pytest.fixture
async def test_session(test_engine) -> AsyncGenerator[AsyncSession, None]:
    """Create a test database session with automatic rollback."""
    TestSessionLocal = async_sessionmaker(
        test_engine,
        class_=AsyncSession,
        expire_on_commit=False,
    )

    async with TestSessionLocal() as session:
        # Set tenant context for tests
        test_tenant_id = str(uuid.uuid4())
        await session.execute(
            text("SET app.tenant_id = :tenant_id"),
            {"tenant_id": test_tenant_id}
        )

        # Begin a transaction
        transaction = await session.begin()

        try:
            yield session
        finally:
            # Always rollback to keep tests isolated
            await transaction.rollback()
```

**Why This Works**:
- Each test gets a fresh database transaction that is rolled back
- Tenant isolation is automatically configured for multi-tenant testing
- PostgreSQL extensions are enabled once per test session
- No test pollution between test runs

### AI Agent Mock Framework

**Pattern**: Comprehensive mock framework for AI coordination components

```python
class MockAsyncSession:
    """Mock async session for testing."""

    def __init__(self):
        self.executed_queries = []

    async def execute(self, query):
        self.executed_queries.append(query)
        return MagicMock()

    async def commit(self):
        pass

    async def rollback(self):
        pass

class MockEventPublisher:
    """Mock event publisher for testing."""

    def __init__(self):
        self.published_events = []

    async def publish(self, event_type: str, event_data: Dict[str, Any], channel: str = None) -> str:
        event_id = str(uuid.uuid4())
        self.published_events.append({
            "event_id": event_id,
            "event_type": event_type,
            "event_data": event_data,
            "channel": channel
        })
        return event_id

class MockJobManager:
    """Mock job manager for testing."""

    def __init__(self):
        self.jobs = {}
        self.job_results = {}
        self.subscription_manager = MockSubscriptionManager()

    async def create_job(self, event_id: str, processor_name: str, job_type: str,
                        input_data: Dict[str, Any], priority: int, metadata: Dict[str, Any]) -> str:
        job_id = str(uuid.uuid4())
        job = MockJob(
            job_id=job_id,
            event_id=event_id,
            processor_name=processor_name,
            job_type=job_type,
            input_data=input_data,
            priority=priority,
            metadata=metadata
        )
        self.jobs[job_id] = job
        return job_id

    def complete_job(self, job_id: str, results: List[Dict[str, Any]]):
        """Helper method to simulate job completion."""
        if job_id in self.jobs:
            self.jobs[job_id].status = "completed"
            self.jobs[job_id].completed_at = datetime.now(timezone.utc)
            self.job_results[job_id] = [MockJobResult(**result) for result in results]
```

### Performance Benchmarking Framework

**Pattern**: Standardized performance validation with timing utilities

```python
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

# Example usage in performance tests
@pytest.mark.performance
async def test_coordination_latency(coordinator, mock_job_manager, benchmark_timer):
    """Test coordination startup latency."""
    content_data = {"content": "Performance test content"}

    timer = benchmark_timer()
    timer.start()

    coordination_id = await coordinator.coordinate_processing(
        content_id="perf-test",
        content_type="email",
        content_data=content_data
    )

    timer.stop()

    # Should start coordination quickly (< 100ms for mocked components)
    assert timer.elapsed_ms < 100
    assert coordination_id in coordinator.active_coordinations
```

### Test Categorization and Markers

**Pattern**: Automatic test categorization with environment controls

```python
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

def pytest_runtest_setup(item):
    """Setup function run before each test."""
    # Skip database tests if DB_SKIP_TESTS is set
    if "requires_db" in [marker.name for marker in item.iter_markers()]:
        if os.environ.get("DB_SKIP_TESTS"):
            pytest.skip("Database tests skipped (DB_SKIP_TESTS set)")

    # Skip slow tests unless explicitly requested
    if "slow" in [marker.name for marker in item.iter_markers()]:
        if not item.config.getoption("--runslow", default=False):
            pytest.skip("Slow test skipped (use --runslow to run)")
```

## 📊 Performance Standards (Production-Validated)

### Test Execution Performance Targets
- **Unit Tests**: <1ms average execution time ✅ Achieved
- **Integration Tests**: <500ms average (with database) ✅ Achieved
- **Contract Tests**: <200ms average ✅ Achieved
- **Performance Tests**: <2 seconds for benchmarking ✅ Achieved

### Test Coverage Requirements
```python
# Coverage configuration in pytest.ini
[tool:pytest]
addopts =
    --cov=penf_lib
    --cov=observability_lib
    --cov-report=html
    --cov-report=term-missing
    --cov-fail-under=80
    --strict-markers
    --disable-warnings

markers =
    slow: marks tests as slow (deselect with '-m "not slow"')
    unit: marks tests as unit tests
    integration: marks tests as integration tests
    contract: marks tests as contract tests
    performance: marks tests as performance tests
    requires_db: marks tests as requiring database
    requires_redis: marks tests as requiring Redis
```

### Data Generation and Fixtures

**Pattern**: Reusable test data generation with consistent patterns

```python
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

# Sample fixtures for common entities
@pytest.fixture
def sample_source_data():
    """Sample source data for testing."""
    return {
        "source_system": "gmail",
        "external_id": "msg_12345",
        "content_hash": "a" * 64,
        "raw_content": "This is a sample email content for testing.",
        "content_type": "text/plain",
        "content_size": 45,
        "ingestion_metadata": {"sender": "test@example.com"},
    }

@pytest.fixture
async def test_source(test_session, sample_tenant_id, sample_source_data):
    """Create a test source entity."""
    from penf_lib.storage.models import Source

    source = Source(
        tenant_id=sample_tenant_id,
        **sample_source_data
    )
    test_session.add(source)
    await test_session.flush()  # Get ID without committing

    return source
```

## 🚨 Common Anti-Patterns to Avoid

### ❌ Shared Test State
```python
# WRONG - Global test state causes test pollution
class TestSharedState:
    shared_data = {}  # ❌ Shared between tests

    async def test_first(self):
        self.shared_data['key'] = 'value'
        assert self.shared_data['key'] == 'value'

    async def test_second(self):
        # ❌ Relies on state from previous test
        assert self.shared_data.get('key') == 'value'

# CORRECT - Each test is isolated
class TestIsolated:
    @pytest.fixture
    def test_data(self):
        return {}  # ✅ Fresh data for each test

    async def test_first(self, test_data):
        test_data['key'] = 'value'
        assert test_data['key'] == 'value'

    async def test_second(self, test_data):
        # ✅ Clean state for each test
        assert test_data == {}
```

### ❌ Non-Deterministic Tests
```python
# WRONG - Random behavior leads to flaky tests
async def test_random_behavior():
    import random
    value = random.random()  # ❌ Different each run
    assert value > 0.5  # ❌ Flaky test

# CORRECT - Deterministic with controlled randomness
async def test_deterministic(data_generator):
    # ✅ Controlled randomness with seeded generator
    random.seed(42)
    value = random.random()
    assert value == 0.6394267984578837  # ✅ Predictable result
```

### ❌ Inadequate AI Agent Mocking
```python
# WRONG - No control over AI agent responses
async def test_ai_coordination():
    coordinator = ModelCoordinator(real_llm_client)  # ❌ Real API calls
    result = await coordinator.process("test input")
    assert "some_field" in result  # ❌ Unpredictable response

# CORRECT - Comprehensive mocking for AI agents
async def test_ai_coordination(mock_job_manager):
    coordinator = ModelCoordinator(session=mock_session, job_manager=mock_job_manager)

    # ✅ Control AI agent responses
    mock_job_manager.complete_job(job_id, [{
        "result_data": {"entities": ["person:John"]},
        "confidence_score": 0.85,
        "model_name": "test-model",
        "processing_time_ms": 1500
    }])

    result = await coordinator.wait_for_coordination(coordination_id)
    assert len(result["individual_results"]) == 1
```

### ❌ Missing Performance Validation
```python
# WRONG - No performance requirements validation
async def test_slow_operation():
    start = time.time()
    await slow_database_operation()
    # ❌ No assertion about timing
    assert True

# CORRECT - Performance requirements validation
async def test_database_operation_performance(benchmark_timer):
    timer = benchmark_timer()
    timer.start()
    await database_operation()
    timer.stop()

    # ✅ Validate meets performance target (<100ms)
    assert timer.elapsed_ms < 100
```

## 🔗 Integration Test Patterns

### Complete Workflow Integration Testing
```python
class TestIntegrationWorkflow:
    """Integration tests for complete AI coordination workflows."""

    async def test_complete_coordination_workflow(self, coordinator, mock_job_manager):
        """Test a complete end-to-end coordination workflow."""
        content_data = {
            "subject": "Quarterly Review Meeting",
            "content": "Please prepare the quarterly review slides for next week's board meeting.",
            "sender": "manager@company.com",
            "received_at": "2024-01-15T10:00:00Z"
        }

        # Start coordination
        coordination_id = await coordinator.coordinate_processing(
            content_id="email-quarterly-review",
            content_type="email",
            content_data=content_data,
            require_minimum_models=2
        )

        # Simulate model processing and completion
        for i, job_id in enumerate(coordination["job_ids"]):
            result_data = {
                "entities": [
                    {"type": "event", "value": "Quarterly Review Meeting", "confidence": 0.9},
                    {"type": "person", "value": "manager@company.com", "confidence": 0.85}
                ],
                "sentiment": "neutral",
                "urgency": "medium"
            }

            mock_job_manager.complete_job(job_id, [{
                "result_data": result_data,
                "confidence_score": 0.8 + (i * 0.05),
                "model_name": f"model-{i}",
                "processing_time_ms": 1500 + (i * 200)
            }])

        # Get final results
        results = await coordinator.wait_for_coordination(
            coordination_id=coordination_id,
            min_results=2,
            timeout=10
        )

        # Verify complete workflow
        assert results["coordination_id"] == coordination_id
        assert len(results["individual_results"]) >= 2
        assert results["performance_summary"]["completion_rate"] > 0
```

### Contract Testing for API Interfaces
```python
# Contract tests verify interface compliance
@pytest.mark.contract
class TestContractCompliance:
    """Contract tests for API interface compliance."""

    async def test_event_processing_api_contract(self, test_session):
        """Test that event processing API maintains contract."""
        from penf_lib.event_processing.api import ProcessingAPI

        api = ProcessingAPI(test_session)

        # Test input contract
        valid_input = {
            "event_type": "email_received",
            "content_data": {"subject": "Test", "content": "Test content"},
            "source_metadata": {"sender": "test@example.com"}
        }

        result = await api.process_event(valid_input)

        # Test output contract
        required_fields = ["event_id", "processing_status", "extracted_entities", "confidence_score"]
        assert all(field in result for field in required_fields)
        assert isinstance(result["confidence_score"], float)
        assert 0.0 <= result["confidence_score"] <= 1.0
```

## 📚 Key Resources

**Primary Implementation Files**:
- `tests/conftest.py` - Global test configuration and fixtures
- `tests/fixtures/database.py` - Database testing fixtures with isolation
- `tests/integration/test_ai_coordination.py` - AI agent coordination testing patterns
- `tests/performance/test_event_processing_success_criteria.py` - Performance validation

**Testing Utilities**:
- `tests/unit/` - Unit test examples with comprehensive mocking
- `tests/contract/` - Contract testing for interface compliance
- `tests/integration/` - End-to-end integration test patterns

## 🎯 Success Criteria

When implementing testing features:

✅ **All tests must be isolated with automatic database rollback**
✅ **Mock frameworks must provide deterministic control over AI agent responses**
✅ **Performance tests must validate against established requirements**
✅ **Test categorization must be automatic based on file location**
✅ **Integration tests must cover complete multi-agent workflows**
✅ **Contract tests must verify interface compliance and data schemas**
✅ **Test coverage must meet minimum 80% requirement**

This context ensures consistent, comprehensive testing development that maintains the production-proven patterns established in the 010-testing-framework implementation.