# CI/CD Pipeline Definition

## Overview

This document defines the continuous integration and continuous deployment (CI/CD) pipeline for Penfold. The pipeline ensures code quality through automated testing, linting, type checking, and enforces quality gates before any code reaches production.

## Pipeline Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Trigger   │───►│    Build    │───►│    Test     │───►│   Deploy    │
│  (PR/Push)  │    │  & Validate │    │  & Quality  │    │  (if main)  │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                         │                   │                   │
                         ▼                   ▼                   ▼
                   • Install deps      • Unit tests       • Staging deploy
                   • Ruff lint         • Integration      • Production deploy
                   • Mypy check        • Coverage 80%+    • Docker publish
                   • Build package     • Contract tests   • Release tags
```

## GitHub Actions Workflows

### 1. Main CI Workflow

Create `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  PYTHON_VERSION: "3.12"
  POSTGRES_USER: postgres
  POSTGRES_PASSWORD: postgres
  POSTGRES_DB: penfold_test

jobs:
  lint:
    name: Lint & Type Check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: ${{ env.PYTHON_VERSION }}
          cache: 'pip'

      - name: Install dependencies
        run: |
          python -m pip install --upgrade pip
          pip install -e ".[dev]"

      - name: Run ruff linting
        run: ruff check . --output-format=github

      - name: Run ruff formatting check
        run: ruff format --check .

      - name: Run mypy type checking
        run: mypy penf_lib --strict

  test-unit:
    name: Unit Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: ${{ env.PYTHON_VERSION }}
          cache: 'pip'

      - name: Install dependencies
        run: |
          python -m pip install --upgrade pip
          pip install -e ".[dev]"

      - name: Run unit tests with coverage
        run: |
          pytest tests/unit -v \
            --cov=penf_lib \
            --cov-report=xml \
            --cov-report=term-missing \
            --cov-fail-under=80

      - name: Upload coverage to Codecov
        uses: codecov/codecov-action@v4
        with:
          file: ./coverage.xml
          flags: unit
          fail_ci_if_error: false

  test-integration:
    name: Integration Tests
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: ${{ env.POSTGRES_USER }}
          POSTGRES_PASSWORD: ${{ env.POSTGRES_PASSWORD }}
          POSTGRES_DB: ${{ env.POSTGRES_DB }}
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: ${{ env.PYTHON_VERSION }}
          cache: 'pip'

      - name: Install dependencies
        run: |
          python -m pip install --upgrade pip
          pip install -e ".[dev]"

      - name: Run integration tests
        env:
          DATABASE_URL: postgresql://${{ env.POSTGRES_USER }}:${{ env.POSTGRES_PASSWORD }}@localhost:5432/${{ env.POSTGRES_DB }}
          REDIS_URL: redis://localhost:6379/0
        run: |
          pytest tests/integration -v \
            --cov=penf_lib \
            --cov-report=xml \
            --cov-append

      - name: Upload coverage to Codecov
        uses: codecov/codecov-action@v4
        with:
          file: ./coverage.xml
          flags: integration
          fail_ci_if_error: false

  test-contract:
    name: Contract Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: ${{ env.PYTHON_VERSION }}
          cache: 'pip'

      - name: Install dependencies
        run: |
          python -m pip install --upgrade pip
          pip install -e ".[dev]"

      - name: Run contract tests
        run: pytest tests/contract -v

  quality-gate:
    name: Quality Gate
    needs: [lint, test-unit, test-integration, test-contract]
    runs-on: ubuntu-latest
    steps:
      - name: All checks passed
        run: echo "All quality checks passed successfully"
```

### 2. Docker Build and Publish Workflow

Create `.github/workflows/docker.yml`:

```yaml
name: Docker Build & Publish

on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build:
    name: Build Docker Image
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to Container Registry
        if: github.event_name != 'pull_request'
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch
            type=ref,event=pr
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Run Trivy vulnerability scanner
        if: github.event_name != 'pull_request'
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:sha-${{ github.sha }}
          format: 'sarif'
          output: 'trivy-results.sarif'

      - name: Upload Trivy scan results
        if: github.event_name != 'pull_request'
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: 'trivy-results.sarif'
```

### 3. Release Workflow

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags: ['v*']

permissions:
  contents: write
  packages: write

jobs:
  release:
    name: Create Release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Python
        uses: actions/setup-python@v5
        with:
          python-version: "3.12"

      - name: Install build tools
        run: pip install build

      - name: Build package
        run: python -m build

      - name: Generate changelog
        id: changelog
        run: |
          # Get the previous tag
          PREV_TAG=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo "")

          if [ -n "$PREV_TAG" ]; then
            echo "## Changes since $PREV_TAG" > CHANGELOG.md
            git log $PREV_TAG..HEAD --pretty=format:"- %s" >> CHANGELOG.md
          else
            echo "## Initial Release" > CHANGELOG.md
            git log --pretty=format:"- %s" >> CHANGELOG.md
          fi

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v1
        with:
          body_path: CHANGELOG.md
          files: |
            dist/*.whl
            dist/*.tar.gz
          draft: false
          prerelease: ${{ contains(github.ref, 'alpha') || contains(github.ref, 'beta') || contains(github.ref, 'rc') }}
```

## Linting Configuration

### Ruff Configuration

The ruff linter is configured in `pyproject.toml`:

```toml
[tool.ruff]
target-version = "py312"
line-length = 88
select = [
    "E",   # pycodestyle errors
    "W",   # pycodestyle warnings
    "F",   # pyflakes
    "I",   # isort
    "B",   # flake8-bugbear
    "C4",  # flake8-comprehensions
    "UP",  # pyupgrade
]
ignore = [
    "E501",  # line too long (handled by formatter)
    "B008",  # do not perform function calls in argument defaults
    "C901",  # too complex
]

[tool.ruff.per-file-ignores]
"tests/**/*" = ["S101"]  # Allow assert in tests
```

### Running Locally

```bash
# Check for linting issues
ruff check .

# Auto-fix linting issues
ruff check --fix .

# Check formatting
ruff format --check .

# Apply formatting
ruff format .
```

## Type Checking Configuration

### Mypy Configuration

The mypy type checker is configured in `pyproject.toml`:

```toml
[tool.mypy]
python_version = "3.12"
check_untyped_defs = true
disallow_any_generics = true
disallow_incomplete_defs = true
disallow_untyped_defs = true
no_implicit_optional = true
strict_equality = true
warn_redundant_casts = true
warn_return_any = true
warn_unreachable = true
warn_unused_configs = true

[[tool.mypy.overrides]]
module = [
    "pgvector.*",
    "msgpack.*",
]
ignore_missing_imports = true
```

### Running Locally

```bash
# Run type checking
mypy penf_lib --strict

# Run with detailed output
mypy penf_lib --strict --show-error-codes
```

## Test Execution

### Test Categories

The test suite is organized into categories:

| Category | Directory | Purpose | CI Stage |
|----------|-----------|---------|----------|
| Unit | `tests/unit/` | Fast, isolated tests | Parallel |
| Integration | `tests/integration/` | Database/service tests | With services |
| Contract | `tests/contract/` | API schema validation | Parallel |
| Performance | `tests/performance/` | Load/stress tests | Manual |

### Running Tests Locally

```bash
# Run all tests
pytest

# Run by category
pytest tests/unit -v
pytest tests/integration -v
pytest tests/contract -v

# Run with coverage
pytest --cov=penf_lib --cov-report=term-missing --cov-fail-under=80

# Run specific markers
pytest -m "unit and not slow"
pytest -m "integration"

# Run with verbose output
pytest -v --tb=short
```

### Coverage Requirements

```toml
[tool.coverage.run]
source = ["penf_lib"]
omit = [
    "*/tests/*",
    "*/migrations/*",
]

[tool.coverage.report]
exclude_lines = [
    "pragma: no cover",
    "def __repr__",
    "if self.debug:",
    "if settings.DEBUG",
    "raise AssertionError",
    "raise NotImplementedError",
    "if 0:",
    "if __name__ == .__main__.:",
    "class .*\\bProtocol\\):",
    "@(abc\\.)?abstractmethod",
]
```

## Quality Gates

### Mandatory Checks (All PRs)

| Gate | Tool | Threshold | Failure Action |
|------|------|-----------|----------------|
| Linting | Ruff | Zero errors | Block merge |
| Formatting | Ruff | Zero diffs | Block merge |
| Type Checking | Mypy | Zero errors | Block merge |
| Unit Tests | Pytest | All pass | Block merge |
| Coverage | Coverage.py | >= 80% | Block merge |
| Contract Tests | Pytest | All pass | Block merge |

### Integration Requirements (Main Branch)

| Gate | Tool | Threshold | Failure Action |
|------|------|-----------|----------------|
| Integration Tests | Pytest | All pass | Block merge |
| Security Scan | Trivy | No critical/high | Warning |
| Docker Build | Docker | Successful | Block merge |

### Release Requirements (Tags)

| Gate | Tool | Threshold | Failure Action |
|------|------|-----------|----------------|
| All CI Passes | GitHub Actions | Green | Block release |
| Version Bump | pyproject.toml | Valid semver | Block release |
| Changelog | Generated | Present | Block release |

## Deployment Strategy

### Environment Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Deployment Environments                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐              │
│  │ Development │    │   Staging   │    │ Production  │              │
│  │   (Local)   │───►│   (Auto)    │───►│  (Manual)   │              │
│  └─────────────┘    └─────────────┘    └─────────────┘              │
│        │                  │                  │                       │
│        ▼                  ▼                  ▼                       │
│  • Local Docker     • Auto deploy      • Manual approval             │
│  • Mock services    • Real services    • Full monitoring             │
│  • Hot reload       • Smoke tests      • Rollback ready              │
│                                                                       │
└─────────────────────────────────────────────────────────────────────┘
```

### Staging Deployment

Automatic deployment to staging on merge to main:

```yaml
# .github/workflows/deploy-staging.yml
name: Deploy to Staging

on:
  push:
    branches: [main]

jobs:
  deploy:
    name: Deploy to Staging
    runs-on: ubuntu-latest
    environment: staging
    needs: [ci]  # Reference the CI workflow

    steps:
      - uses: actions/checkout@v4

      - name: Deploy to staging
        run: |
          # Deploy using docker-compose or your deployment tool
          # Example with SSH deployment:
          # ssh $STAGING_HOST "cd /app && docker-compose pull && docker-compose up -d"
          echo "Deploying to staging environment"

      - name: Run smoke tests
        run: |
          # Verify deployment health
          curl -f https://staging.penfold.example.com/health || exit 1

      - name: Notify deployment
        if: always()
        run: |
          echo "Staging deployment completed with status: ${{ job.status }}"
```

### Production Deployment

Manual deployment to production for tagged releases:

```yaml
# .github/workflows/deploy-production.yml
name: Deploy to Production

on:
  release:
    types: [published]

jobs:
  deploy:
    name: Deploy to Production
    runs-on: ubuntu-latest
    environment: production

    steps:
      - uses: actions/checkout@v4

      - name: Verify release
        run: |
          # Ensure all CI checks passed
          gh pr checks ${{ github.sha }} --required

      - name: Create deployment record
        run: |
          echo "Deploying ${{ github.ref_name }} to production"

      - name: Deploy to production
        run: |
          # Production deployment steps
          echo "Production deployment in progress"

      - name: Health check
        run: |
          curl -f https://penfold.example.com/health || exit 1

      - name: Update deployment status
        if: always()
        run: |
          echo "Production deployment: ${{ job.status }}"
```

## Version Tagging and Release Management

### Semantic Versioning

Follow [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR** (`X.0.0`): Breaking API changes
- **MINOR** (`0.X.0`): New features, backward compatible
- **PATCH** (`0.0.X`): Bug fixes, backward compatible

### Pre-release Tags

- **Alpha**: `v1.0.0-alpha.1` - Early development, unstable
- **Beta**: `v1.0.0-beta.1` - Feature complete, testing
- **RC**: `v1.0.0-rc.1` - Release candidate, final testing

### Creating a Release

```bash
# Update version in pyproject.toml
# Update CHANGELOG.md

# Create and push tag
git tag -a v1.0.0 -m "Release v1.0.0: Description"
git push origin v1.0.0
```

### Release Checklist

1. [ ] All CI checks passing on main
2. [ ] Version bumped in `pyproject.toml`
3. [ ] CHANGELOG.md updated
4. [ ] Documentation updated
5. [ ] Tag created with descriptive message
6. [ ] GitHub Release created
7. [ ] Docker image published
8. [ ] Staging deployment verified
9. [ ] Production deployment approved and executed

## Local Development Setup

### Prerequisites

```bash
# Install Python 3.12
# Install Docker and Docker Compose

# Clone repository
git clone https://github.com/otherjamesbrown/penfold.git
cd penfold

# Create virtual environment
python -m venv .venv
source .venv/bin/activate  # or .venv\Scripts\activate on Windows

# Install dependencies with dev extras
pip install -e ".[dev]"
```

### Running CI Checks Locally

```bash
# Run full CI suite locally
make ci  # If Makefile exists

# Or run manually:
ruff check .
ruff format --check .
mypy penf_lib --strict
pytest --cov=penf_lib --cov-fail-under=80
```

### Pre-commit Hooks

Install pre-commit hooks to catch issues before push:

```bash
# Install pre-commit
pip install pre-commit

# Install hooks
pre-commit install

# Run manually
pre-commit run --all-files
```

Example `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/astral-sh/ruff-pre-commit
    rev: v0.1.6
    hooks:
      - id: ruff
        args: [--fix]
      - id: ruff-format

  - repo: https://github.com/pre-commit/mirrors-mypy
    rev: v1.7.0
    hooks:
      - id: mypy
        additional_dependencies: [types-all]
        args: [--strict]
```

## Troubleshooting

### Common CI Failures

| Error | Cause | Solution |
|-------|-------|----------|
| Ruff check failed | Linting violations | Run `ruff check --fix .` |
| Mypy errors | Type annotation issues | Fix type hints in code |
| Coverage < 80% | Insufficient tests | Add tests for uncovered code |
| Integration test timeout | Service not ready | Increase health check retries |
| Docker build failed | Missing dependencies | Check Dockerfile and requirements |

### Debugging CI

```bash
# View workflow run logs
gh run view <run-id> --log

# Re-run failed jobs
gh run rerun <run-id> --failed

# List recent workflow runs
gh run list --workflow=ci.yml
```

## Security Considerations

### Secrets Management

- Store secrets in GitHub Secrets, not in code
- Use environment-specific secrets (staging vs production)
- Rotate credentials regularly
- Never commit `.env` files

### Required Secrets

| Secret | Purpose | Scope |
|--------|---------|-------|
| `GITHUB_TOKEN` | Container registry | Automatic |
| `CODECOV_TOKEN` | Coverage upload | Optional |
| `STAGING_SSH_KEY` | Staging deployment | Environment |
| `PRODUCTION_SSH_KEY` | Production deployment | Environment |

## Monitoring CI Health

### Metrics to Track

- Build success rate (target: >95%)
- Average CI duration (target: <10 minutes)
- Flaky test rate (target: <1%)
- Time to merge after PR approval

### Alerts

Set up notifications for:
- CI pipeline failures on main branch
- Deployment failures
- Security vulnerability detection
- Coverage drops below threshold
