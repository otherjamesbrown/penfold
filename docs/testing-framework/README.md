# Penfold Testing Framework

The Penfold Testing Framework provides comprehensive testing capabilities for AI-first applications, including environment isolation, AI model mocking, and realistic test data management.

## Quick Start

```bash
# Run unit tests (fast, fully mocked)
pytest tests/unit/ -v

# Run integration tests (with lightweight AI models)
pytest tests/integration/ -v --runslow

# Run performance tests
pytest tests/performance/ -v -m performance --runslow

# Run specific test category
pytest -m "unit and not slow" -v
```

## Testing Tiers

### 1. Unit Tests (<100ms per test)
- **Purpose**: Fast, isolated component testing
- **AI Strategy**: Full mocking with deterministic responses
- **Database**: In-memory SQLite or mocked
- **Location**: `tests/unit/`

```python
@pytest.mark.unit
async def test_email_processing_mock(mock_ai_full):
    """Test email processing with fully mocked AI"""
    processor = EmailProcessor(ai_client=mock_ai_full['ollama'])

    result = await processor.extract_entities("Test email content")

    assert result['people'] == ['James Brown', 'Sarah Chen']
    assert result['projects'] == ['Atlas Integration']
```

### 2. Integration Tests (<10s per test)
- **Purpose**: Multi-component testing with realistic AI behavior
- **AI Strategy**: Lightweight models (Phi-3 Mini, Qwen2.5-7B)
- **Database**: PostgreSQL with automatic rollback
- **Location**: `tests/integration/`

```python
@pytest.mark.integration
async def test_email_to_database_workflow(test_session, lightweight_ai):
    """Test complete email processing workflow"""
    email_data = load_test_email('atlas_project_concern')

    result = await process_email_complete(email_data, test_session)

    # Verify database entities created
    sources = await get_sources(test_session)
    assert len(sources) == 1
    assert sources[0].source_system == 'gmail'
```

### 3. End-to-End Tests (<30s per test)
- **Purpose**: Complete workflow validation
- **AI Strategy**: Recorded real AI responses
- **Database**: Full PostgreSQL with test data
- **Location**: `tests/e2e/`

```python
@pytest.mark.slow
async def test_complete_ingestion_pipeline(recorded_ai_responses):
    """Test full pipeline with recorded AI responses"""
    session = recorded_ai_responses['email_processing']

    result = await run_full_pipeline(test_email_corpus)

    assert result.success_rate > 0.95
    assert result.processing_time < 30000  # 30 seconds
```

## Test Environment Setup

### Local Development
```bash
# Install test dependencies
pip install -e .[test]

# Set up test environment
export PENFOLD_ENV=testing
export DB_NAME=penfold_test
export REDIS_DB=15

# Run test database setup
pytest --setup-only
```

### Docker Environment
```bash
# Start test services
docker-compose -f docker-compose.test.yml up -d

# Run tests against containers
pytest --docker-env

# Clean up
docker-compose -f docker-compose.test.yml down -v
```

### CI/CD Environment
```bash
# Skip expensive tests
export DB_SKIP_TESTS=1
export REDIS_SKIP_TESTS=1

# Run only unit tests
pytest tests/unit/ -v
```

## AI Model Mocking

### Unit Test Mocking
```python
# Use deterministic AI responses
@pytest.fixture
def mock_ai_full():
    with patch('penf_lib.ai.ollama_client') as mock_ollama:
        mock_ollama.generate = AsyncMock(return_value={
            'response': 'Mock response based on prompt patterns'
        })
        yield mock_ollama

# Pattern-based responses
def test_ai_summarization(mock_ai_full):
    # Mock will return consistent summary based on content patterns
    result = ai_client.summarize("Email about Atlas project timeline")
    assert "Atlas project" in result
```

### Integration Test Models
```python
# Use fast, lightweight models
LIGHTWEIGHT_MODELS = {
    'summarization': 'phi-3-mini-3.8b',     # 3.8B params, fast inference
    'entity_extraction': 'qwen2.5-7b',      # Good at structured tasks
    'categorization': 'llama-3.2-3b',       # Fast classification
    'embedding': 'nomic-embed-text'          # Consistent embeddings
}

# Configure for integration tests
@pytest.fixture
def lightweight_ai():
    strategy = LightweightModelStrategy()
    yield strategy
```

### Recorded Response Replay
```python
# Record real AI interactions
async def record_atlas_analysis():
    recorder = AIResponseRecorder()

    # Record real interactions
    summary = await real_ai.summarize(atlas_email)
    entities = await real_ai.extract_entities(atlas_email)

    await recorder.save_session('atlas_analysis', [
        AIInteraction('summarize', atlas_email, summary),
        AIInteraction('extract_entities', atlas_email, entities)
    ])

# Replay in tests
@pytest.fixture
def recorded_responses():
    return AIResponseRecorder().load_session('atlas_analysis')
```

## Test Data Management

### Business Scenarios
```python
# Realistic business email threads
@pytest.fixture
def atlas_project_emails():
    return [
        Email(
            subject="Atlas Timeline Concern",
            sender="marcus.rodriguez@company.com",
            content="Engineering team is reporting potential delays...",
            thread_id="atlas-concern-001"
        ),
        Email(
            subject="Re: Atlas Timeline Concern",
            sender="james.brown@company.com",
            content="Let's schedule a checkpoint meeting...",
            thread_id="atlas-concern-001"
        )
    ]

# Consistent test personas
@pytest.fixture
def test_people():
    return [
        Person(name="James Brown", role="COO", email="james.brown@company.com"),
        Person(name="Sarah Chen", role="VP Engineering", email="sarah.chen@company.com"),
        Person(name="Marcus Rodriguez", role="Head of Sales", email="marcus.rodriguez@company.com")
    ]
```

### Data Generation
```python
# Generate consistent test data
class TestDataGenerator:
    def generate_email_thread(self, scenario: str, participants: List[str]) -> List[Email]:
        scenarios = {
            'project_escalation': self._escalation_thread,
            'budget_approval': self._budget_thread,
            'meeting_coordination': self._meeting_thread
        }
        return scenarios[scenario](participants)

# Usage in tests
def test_email_processing_scenarios(data_generator):
    for scenario in ['project_escalation', 'budget_approval', 'meeting_coordination']:
        emails = data_generator.generate_email_thread(scenario, ['james', 'sarah', 'marcus'])
        result = process_email_thread(emails)
        assert result.success
```

## Performance Testing

### Benchmark Utilities
```python
@pytest.fixture
def benchmark_timer():
    class Timer:
        def start(self): self.start_time = time.perf_counter()
        def stop(self): self.end_time = time.perf_counter()
        @property
        def elapsed_ms(self): return (self.end_time - self.start_time) * 1000
    return Timer

@pytest.mark.performance
async def test_vector_search_performance(test_session, benchmark_timer):
    # Setup test data
    await create_test_embeddings(test_session, count=10000)

    timer = benchmark_timer()
    timer.start()

    results = await vector_similarity_search(test_session, query_vector, limit=100)

    timer.stop()

    # Validate performance target
    assert timer.elapsed_ms < 500  # <500ms for vector search
    assert len(results) <= 100
```

### Load Testing
```python
@pytest.mark.slow
async def test_concurrent_processing(test_session):
    # Test concurrent AI processing
    tasks = []

    for i in range(50):  # 50 concurrent operations
        email = generate_test_email(f"test-{i}")
        task = process_email_async(email, test_session)
        tasks.append(task)

    results = await asyncio.gather(*tasks)

    # Validate all succeeded
    assert all(r.success for r in results)

    # Validate performance under load
    max_time = max(r.processing_time_ms for r in results)
    assert max_time < 15000  # <15s even under load
```

## Test Configuration

### Environment Variables
```bash
# Test behavior controls
export PENFOLD_ENV=testing          # Use test configuration
export DB_SKIP_TESTS=1              # Skip database tests
export REDIS_SKIP_TESTS=1           # Skip Redis tests
export AI_MOCK_MODE=deterministic   # Use deterministic AI mocks
export PYTEST_VERBOSE_SQL=1         # Enable SQL logging

# Performance test controls
export PERFORMANCE_TARGET_MS=100    # Override performance targets
export LOAD_TEST_CONCURRENT=20      # Concurrent operations for load tests
```

### pytest Configuration
```ini
# pytest.ini
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
    unit: marks tests as unit tests
    integration: marks tests as integration tests
    performance: marks tests as performance tests
    slow: marks tests as slow running (use --runslow)
    requires_db: marks tests as requiring database
    requires_redis: marks tests as requiring Redis

testpaths = tests
python_files = test_*.py
python_classes = Test*
python_functions = test_*
```

### Custom pytest Options
```bash
# Run slow tests
pytest --runslow

# Skip database tests
pytest --skip-db

# Skip Redis tests
pytest --skip-redis

# Run with Docker environment
pytest --docker-env

# Verbose AI logging
pytest --ai-debug
```

## Best Practices

### Test Organization
- Place unit tests in `tests/unit/`
- Place integration tests in `tests/integration/`
- Place performance tests in `tests/performance/`
- Use descriptive test names that explain the scenario
- Group related tests in test classes

### AI Testing Guidelines
- Use full mocking for unit tests (fast, deterministic)
- Use lightweight models for integration tests (realistic but fast)
- Use recorded responses for critical end-to-end tests
- Test AI confidence thresholds and edge cases
- Validate AI response format and structure

### Performance Guidelines
- Set clear performance targets for each test type
- Use benchmark timers for precise measurement
- Test under load conditions with concurrent operations
- Monitor memory usage and cleanup
- Validate both happy path and error scenarios

### Data Management
- Use consistent test personas across scenarios
- Generate realistic but privacy-safe test data
- Parameterize tests for different business scenarios
- Clean up test data automatically
- Avoid hard-coding test data in test files

## Troubleshooting

### Common Issues

**Slow Test Execution**
```bash
# Check if AI models are being used instead of mocks
pytest -v --tb=short tests/unit/  # Should be <100ms per test

# Enable mock debugging
export AI_MOCK_DEBUG=1
```

**Database Connection Issues**
```bash
# Verify test database setup
export DB_DEBUG=1
pytest --setup-only

# Use in-memory database for unit tests
export DB_USE_MEMORY=1
```

**Flaky Tests**
```bash
# Run tests multiple times to identify flaky tests
pytest --count=10 tests/integration/test_specific.py

# Use deterministic AI mocking
export AI_MOCK_MODE=deterministic
```

**Memory Leaks**
```bash
# Enable memory profiling
pip install memory-profiler
pytest --memory-profile

# Check for unclosed database connections
export DB_POOL_DEBUG=1
```

### Performance Debugging
```bash
# Profile test execution time
pytest --profile

# Generate performance report
pytest --benchmark-save=results tests/performance/

# Compare performance over time
pytest --benchmark-compare=results
```

For more detailed information, see:
- [AI Mocking Strategies](./ai-mocking.md)
- [Environment Setup Guide](./environment-setup.md)
- [Performance Testing Guide](./performance-testing.md)
- [Troubleshooting Guide](./troubleshooting.md)