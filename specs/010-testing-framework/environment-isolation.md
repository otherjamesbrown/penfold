# Environment Isolation Strategy

## Overview

Local hardware constraints require a container-based approach to create isolated test environments that can run in parallel without interference. This strategy addresses the dev/test/staging challenge for AI-first applications running on local infrastructure.

## Environment Types and Requirements

### Development Environment
**Purpose**: Active development with live data and real AI models
**Isolation**: Minimal - developer's local setup
**Data**: Real or realistic development data
**AI Models**: Full local models + cloud APIs

### Unit Test Environment
**Purpose**: Fast, isolated component testing
**Isolation**: Complete - temporary containers with mocked services
**Data**: Minimal synthetic fixtures
**AI Models**: Fully mocked

### Integration Test Environment
**Purpose**: Multi-component testing with realistic behavior
**Isolation**: High - dedicated containers with lightweight services
**Data**: Realistic test fixtures
**AI Models**: Lightweight models or cached responses

### End-to-End Test Environment
**Purpose**: Full workflow testing
**Isolation**: Medium - shared containers with real services
**Data**: Complete realistic scenarios
**AI Models**: Mix of lightweight models and recorded responses

### Staging Environment
**Purpose**: Production-like testing with full features
**Isolation**: Production-like - persistent containers
**Data**: Production-like volume and complexity
**AI Models**: Full model stack

## Container Architecture

### Base Infrastructure Stack

```yaml
# docker-compose.base.yml - Shared infrastructure components
version: '3.8'

x-common-environment: &common-env
  POSTGRES_DB: ${DB_NAME:-penfold}
  POSTGRES_USER: ${DB_USER:-penfold_user}
  POSTGRES_PASSWORD: ${DB_PASSWORD:-dev_password}
  REDIS_URL: redis://redis:6379/0

services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      <<: *common-env
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER:-penfold_user}"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
    ports:
      - "${REDIS_PORT:-6379}:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 3s
      retries: 5

  ollama:
    image: ollama/ollama:latest
    volumes:
      - ollama_data:/root/.ollama
    ports:
      - "${OLLAMA_PORT:-11434}:11434"
    environment:
      - OLLAMA_KEEP_ALIVE=5m
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:11434/api/version"]
      interval: 30s
      timeout: 10s
      retries: 3

volumes:
  postgres_data:
  redis_data:
  ollama_data:
```

### Development Environment

```yaml
# docker-compose.dev.yml
version: '3.8'

include:
  - docker-compose.base.yml

services:
  postgres:
    environment:
      POSTGRES_DB: penfold_dev
    volumes:
      - dev_postgres_data:/var/lib/postgresql/data
      - ./test-data/dev-seed.sql:/docker-entrypoint-initdb.d/seed.sql
    ports:
      - "5432:5432"

  redis:
    volumes:
      - dev_redis_data:/data
    ports:
      - "6379:6379"

  ollama:
    volumes:
      - dev_ollama_data:/root/.ollama
      - ./ai-models:/models
    ports:
      - "11434:11434"
    command: >
      bash -c "
        ollama serve &
        sleep 10 &&
        ollama pull llama3.1:8b &&
        ollama pull phi3:mini &&
        ollama pull nomic-embed-text &&
        wait
      "

  # Development application container
  penfold-dev:
    build:
      context: .
      dockerfile: Dockerfile.dev
    volumes:
      - .:/app
      - /app/__pycache__
      - /app/.pytest_cache
    environment:
      - ENVIRONMENT=development
      - DATABASE_URL=postgresql://penfold_user:dev_password@postgres:5432/penfold_dev
      - REDIS_URL=redis://redis:6379/0
      - OLLAMA_URL=http://ollama:11434
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      ollama:
        condition: service_healthy
    ports:
      - "8000:8000"

volumes:
  dev_postgres_data:
  dev_redis_data:
  dev_ollama_data:
```

### Unit Test Environment

```yaml
# docker-compose.test.yml
version: '3.8'

services:
  # Lightweight, ephemeral test database
  test-postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: penfold_test
      POSTGRES_USER: test_user
      POSTGRES_PASSWORD: test_pass
    tmpfs:
      - /var/lib/postgresql/data:rw,noexec,nosuid,size=100m
    ports:
      - "5433:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U test_user"]
      interval: 5s
      timeout: 3s
      retries: 3

  # Lightweight Redis for pub/sub testing
  test-redis:
    image: redis:7-alpine
    tmpfs:
      - /data:rw,noexec,nosuid,size=50m
    ports:
      - "6380:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 2s
      retries: 3

  # Mock AI services for fast unit testing
  ai-mock:
    build:
      context: ./test-infrastructure
      dockerfile: Dockerfile.ai-mock
    environment:
      - MOCK_MODE=deterministic
      - RESPONSE_LIBRARY=/app/responses
    volumes:
      - ./test-data/ai-responses:/app/responses:ro
      - ./test-infrastructure/mock-configs:/app/configs:ro
    ports:
      - "11435:11434"  # Mock Ollama API
      - "8001:8001"    # Mock cloud APIs
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:11434/api/version"]
      interval: 10s
      timeout: 5s
      retries: 3

  # Test runner container
  test-runner:
    build:
      context: .
      dockerfile: Dockerfile.test
    environment:
      - ENVIRONMENT=testing
      - DATABASE_URL=postgresql://test_user:test_pass@test-postgres:5432/penfold_test
      - REDIS_URL=redis://test-redis:6379/0
      - OLLAMA_URL=http://ai-mock:11434
      - AI_MOCKING_MODE=full
    volumes:
      - .:/app:ro
      - test_results:/app/test-results
    depends_on:
      test-postgres:
        condition: service_healthy
      test-redis:
        condition: service_healthy
      ai-mock:
        condition: service_healthy
    command: pytest tests/unit/ -v --tb=short --junit-xml=test-results/unit-tests.xml

volumes:
  test_results:
```

### Integration Test Environment

```yaml
# docker-compose.integration.yml
version: '3.8'

services:
  integration-postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: penfold_integration
      POSTGRES_USER: integration_user
      POSTGRES_PASSWORD: integration_pass
    volumes:
      - integration_postgres_data:/var/lib/postgresql/data
      - ./test-data/integration-seed.sql:/docker-entrypoint-initdb.d/seed.sql
    ports:
      - "5434:5432"

  integration-redis:
    image: redis:7-alpine
    volumes:
      - integration_redis_data:/data
    ports:
      - "6381:6379"

  # Lightweight Ollama with small models for realistic but fast testing
  lightweight-ollama:
    image: ollama/ollama:latest
    volumes:
      - integration_ollama_data:/root/.ollama
    ports:
      - "11436:11434"
    command: >
      bash -c "
        ollama serve &
        sleep 10 &&
        ollama pull phi3:mini &&
        ollama pull qwen2.5:7b &&
        ollama pull nomic-embed-text &&
        wait
      "

  # Integration test runner
  integration-runner:
    build:
      context: .
      dockerfile: Dockerfile.test
    environment:
      - ENVIRONMENT=integration
      - DATABASE_URL=postgresql://integration_user:integration_pass@integration-postgres:5432/penfold_integration
      - REDIS_URL=redis://integration-redis:6379/0
      - OLLAMA_URL=http://lightweight-ollama:11434
      - AI_MOCKING_MODE=lightweight
    volumes:
      - .:/app:ro
      - ./test-data:/app/test-data:ro
      - integration_results:/app/test-results
    depends_on:
      - integration-postgres
      - integration-redis
      - lightweight-ollama
    command: pytest tests/integration/ -v --tb=short --junit-xml=test-results/integration-tests.xml

volumes:
  integration_postgres_data:
  integration_redis_data:
  integration_ollama_data:
  integration_results:
```

### Staging Environment

```yaml
# docker-compose.staging.yml - Production-like environment
version: '3.8'

include:
  - docker-compose.base.yml

services:
  postgres:
    environment:
      POSTGRES_DB: penfold_staging
    volumes:
      - staging_postgres_data:/var/lib/postgresql/data
      - ./test-data/staging-seed.sql:/docker-entrypoint-initdb.d/seed.sql
    ports:
      - "5435:5432"

  redis:
    volumes:
      - staging_redis_data:/data
    ports:
      - "6382:6379"

  ollama:
    volumes:
      - staging_ollama_data:/root/.ollama
    ports:
      - "11437:11434"
    command: >
      bash -c "
        ollama serve &
        sleep 10 &&
        ollama pull llama3.1:8b &&
        ollama pull llama3.1:70b &&
        ollama pull phi3:mini &&
        ollama pull qwen2.5:7b &&
        ollama pull nomic-embed-text &&
        wait
      "

  # Staging application with production-like configuration
  penfold-staging:
    build:
      context: .
      dockerfile: Dockerfile.staging
    environment:
      - ENVIRONMENT=staging
      - DATABASE_URL=postgresql://penfold_user:dev_password@postgres:5432/penfold_staging
      - REDIS_URL=redis://redis:6379/0
      - OLLAMA_URL=http://ollama:11434
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - LOG_LEVEL=INFO
    ports:
      - "8002:8000"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      ollama:
        condition: service_healthy

volumes:
  staging_postgres_data:
  staging_redis_data:
  staging_ollama_data:
```

## Test Orchestration Scripts

### Environment Management Script

```bash
#!/bin/bash
# scripts/test-env.sh - Environment management

set -e

ENVIRONMENT=${1:-unit}
ACTION=${2:-up}

case $ENVIRONMENT in
  "unit")
    COMPOSE_FILE="docker-compose.test.yml"
    ;;
  "integration")
    COMPOSE_FILE="docker-compose.integration.yml"
    ;;
  "e2e")
    COMPOSE_FILE="docker-compose.integration.yml"
    ;;
  "staging")
    COMPOSE_FILE="docker-compose.staging.yml"
    ;;
  "dev")
    COMPOSE_FILE="docker-compose.dev.yml"
    ;;
  *)
    echo "Unknown environment: $ENVIRONMENT"
    echo "Available: unit, integration, e2e, staging, dev"
    exit 1
    ;;
esac

case $ACTION in
  "up")
    echo "Starting $ENVIRONMENT environment..."
    docker-compose -f $COMPOSE_FILE up -d
    echo "Waiting for services to be healthy..."
    docker-compose -f $COMPOSE_FILE ps
    ;;
  "down")
    echo "Stopping $ENVIRONMENT environment..."
    docker-compose -f $COMPOSE_FILE down
    ;;
  "reset")
    echo "Resetting $ENVIRONMENT environment..."
    docker-compose -f $COMPOSE_FILE down -v
    docker-compose -f $COMPOSE_FILE up -d
    ;;
  "logs")
    docker-compose -f $COMPOSE_FILE logs -f
    ;;
  "test")
    echo "Running tests in $ENVIRONMENT environment..."
    docker-compose -f $COMPOSE_FILE up -d

    # Wait for services to be ready
    sleep 30

    # Run tests
    if [ "$ENVIRONMENT" = "unit" ]; then
      docker-compose -f $COMPOSE_FILE exec test-runner pytest tests/unit/ -v
    elif [ "$ENVIRONMENT" = "integration" ]; then
      docker-compose -f $COMPOSE_FILE exec integration-runner pytest tests/integration/ -v
    fi
    ;;
  *)
    echo "Unknown action: $ACTION"
    echo "Available: up, down, reset, logs, test"
    exit 1
    ;;
esac
```

### Parallel Test Execution

```bash
#!/bin/bash
# scripts/run-all-tests.sh - Run all test suites in parallel

set -e

echo "Starting parallel test execution..."

# Start all test environments in parallel
./scripts/test-env.sh unit up &
UNIT_PID=$!

./scripts/test-env.sh integration up &
INTEGRATION_PID=$!

# Wait for environments to start
wait $UNIT_PID
wait $INTEGRATION_PID

echo "All test environments started. Running tests..."

# Run tests in parallel
./scripts/test-env.sh unit test &
UNIT_TEST_PID=$!

./scripts/test-env.sh integration test &
INTEGRATION_TEST_PID=$!

# Wait for all tests to complete
wait $UNIT_TEST_PID
UNIT_RESULT=$?

wait $INTEGRATION_TEST_PID
INTEGRATION_RESULT=$?

echo "Test Results:"
echo "Unit Tests: $([ $UNIT_RESULT -eq 0 ] && echo "PASSED" || echo "FAILED")"
echo "Integration Tests: $([ $INTEGRATION_RESULT -eq 0 ] && echo "PASSED" || echo "FAILED")"

# Clean up environments
./scripts/test-env.sh unit down &
./scripts/test-env.sh integration down &
wait

# Exit with error if any tests failed
if [ $UNIT_RESULT -ne 0 ] || [ $INTEGRATION_RESULT -ne 0 ]; then
  echo "Some tests failed!"
  exit 1
fi

echo "All tests passed!"
```

## CI/CD Integration

### GitHub Actions Workflow

```yaml
# .github/workflows/test.yml
name: Test Suite

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v3

    - name: Start test environment
      run: ./scripts/test-env.sh unit up

    - name: Run unit tests
      run: ./scripts/test-env.sh unit test

    - name: Upload test results
      uses: actions/upload-artifact@v4
      with:
        name: unit-test-results
        path: test-results/unit-tests.xml

  integration-tests:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v3

    - name: Start integration environment
      run: ./scripts/test-env.sh integration up

    - name: Run integration tests
      run: ./scripts/test-env.sh integration test

    - name: Upload test results
      uses: actions/upload-artifact@v4
      with:
        name: integration-test-results
        path: test-results/integration-tests.xml

  staging-deployment:
    runs-on: ubuntu-latest
    needs: [unit-tests, integration-tests]
    if: github.ref == 'refs/heads/main'
    steps:
    - uses: actions/checkout@v4

    - name: Deploy to staging
      run: ./scripts/test-env.sh staging up

    - name: Run staging smoke tests
      run: pytest tests/e2e/smoke_tests.py -v
```

## Resource Management and Optimization

### Container Resource Limits

```yaml
# Resource constraints for test containers
services:
  test-postgres:
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M

  ai-mock:
    deploy:
      resources:
        limits:
          cpus: '0.25'
          memory: 256M
        reservations:
          cpus: '0.1'
          memory: 128M

  test-runner:
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 512M
```

### Performance Monitoring

```python
# Container performance monitoring during tests
class TestEnvironmentMonitor:
    def __init__(self, environment: str):
        self.environment = environment
        self.docker_client = docker.from_env()

    async def monitor_resource_usage(self) -> dict:
        """Monitor container resource usage during tests"""
        containers = self.docker_client.containers.list(
            filters={'label': f'environment={self.environment}'}
        )

        metrics = {}
        for container in containers:
            stats = container.stats(stream=False)
            metrics[container.name] = {
                'cpu_usage': self._calculate_cpu_percentage(stats),
                'memory_usage': stats['memory_stats']['usage'],
                'memory_limit': stats['memory_stats']['limit']
            }

        return metrics

    def _calculate_cpu_percentage(self, stats: dict) -> float:
        """Calculate CPU usage percentage"""
        cpu_delta = (
            stats['cpu_stats']['cpu_usage']['total_usage'] -
            stats['precpu_stats']['cpu_usage']['total_usage']
        )
        system_delta = (
            stats['cpu_stats']['system_cpu_usage'] -
            stats['precpu_stats']['system_cpu_usage']
        )

        if system_delta > 0:
            return (cpu_delta / system_delta) * 100.0
        return 0.0
```

## Implementation Roadmap

### Week 1: Basic Container Infrastructure
- Create base Docker Compose configurations
- Implement unit test environment with mocked services
- Set up basic test orchestration scripts

### Week 2: Integration Environment
- Build integration test environment with lightweight models
- Implement resource constraints and monitoring
- Create parallel test execution framework

### Week 3: CI/CD Integration
- Set up GitHub Actions workflows
- Implement automated environment provisioning
- Add test result reporting and artifacts

### Week 4: Performance Optimization
- Optimize container startup times
- Implement resource monitoring and alerting
- Fine-tune parallel execution strategies

This environment isolation strategy provides comprehensive testing capabilities while working within local hardware constraints, enabling confident development and deployment of AI-first features.