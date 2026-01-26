# CI/CD Pipeline Definition

## Overview

This document defines the continuous integration and continuous deployment (CI/CD) pipeline for Penfold. The pipeline ensures code quality through automated testing, linting, and enforces quality gates before any code reaches production.

## Pipeline Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Trigger   │───►│    Build    │───►│    Test     │───►│   Deploy    │
│  (PR/Push)  │    │  & Validate │    │  & Quality  │    │  (if main)  │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
                         │                   │                   │
                         ▼                   ▼                   ▼
                   • go build          • Unit tests       • Staging deploy
                   • golangci-lint     • Integration      • Production deploy
                   • go vet            • E2E tests        • Docker publish
                   • buf lint          • Coverage         • Release tags
```

## GitHub Actions Workflows

### 1. Test Suite Workflow (`.github/workflows/test.yml`)

The primary test workflow that runs unit, integration, and E2E tests:

```yaml
name: Test Suite

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  # Unit tests run on all pushes and PRs
  unit-tests:
    name: Unit Tests
    runs-on: ubuntu-latest
    strategy:
      matrix:
        module:
          - pkg
          - cmd/penf
          - services/gateway
          - services/gmail
          - services/search
          - services/worker

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true
          cache-dependency-path: "**/go.sum"

      - name: Run unit tests
        run: |
          if [ -f "${{ matrix.module }}/go.mod" ]; then
            cd ${{ matrix.module }}
            go test -v -race -short -coverprofile=coverage.out ./...
          else
            echo "No go.mod found in ${{ matrix.module }}, skipping"
          fi

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        if: hashFiles(format('{0}/coverage.out', matrix.module)) != ''
        with:
          files: ${{ matrix.module }}/coverage.out
          flags: unit-${{ matrix.module }}
          fail_ci_if_error: false

  # Integration tests require database but no LLM
  integration-tests:
    name: Integration Tests
    runs-on: ubuntu-latest
    needs: unit-tests
    if: github.event_name == 'pull_request' || github.ref == 'refs/heads/main'

    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: penfold
          POSTGRES_PASSWORD: penfold_test_password
          POSTGRES_DB: penfold_test_integration
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432

    env:
      PENFOLD_DB_HOST: localhost
      PENFOLD_DB_PORT: 5432
      PENFOLD_DB_USER: penfold
      PENFOLD_DB_PASSWORD: penfold_test_password
      PENFOLD_DB_NAME: penfold_test_integration

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true
          cache-dependency-path: "**/go.sum"

      - name: Wait for PostgreSQL
        run: |
          until pg_isready -h localhost -p 5432 -U penfold; do
            echo "Waiting for PostgreSQL..."
            sleep 2
          done

      - name: Create pgvector extension
        run: |
          PGPASSWORD=penfold_test_password psql -h localhost -U penfold -d penfold_test_integration -c "CREATE EXTENSION IF NOT EXISTS vector;"

      - name: Run database migrations
        run: |
          cd cmd/penf
          go run . migrate up

      - name: Run integration tests
        run: |
          cd tests
          go test -tags=integration -v -race ./integration/...

  # E2E tests run only on main branch (requires self-hosted runner with LLM)
  e2e-tests:
    name: E2E Tests
    runs-on: [self-hosted, macos, ARM64]
    needs: integration-tests
    if: github.ref == 'refs/heads/main'

    env:
      PENFOLD_DB_HOST: dev02.brown.chat
      PENFOLD_DB_PORT: 5432
      PENFOLD_DB_USER: penfold
      PENFOLD_DB_PASSWORD: ${{ secrets.PENFOLD_DB_PASSWORD }}
      PENFOLD_DB_NAME: penfold_test_e2e
      LLM_URL: http://localhost:8080

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true
          cache-dependency-path: "**/go.sum"

      - name: Check LLM availability
        run: |
          curl -sf http://localhost:8080/v1/models || {
            echo "LLM not available, skipping E2E tests"
            exit 0
          }

      - name: Run database migrations
        run: |
          cd cmd/penf
          go run . migrate up

      - name: Run E2E tests
        run: |
          cd tests
          go test -tags=e2e -v -timeout 10m ./e2e/...

  # Validate test fixtures
  validate-fixtures:
    name: Validate Test Fixtures
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true
          cache-dependency-path: "**/go.sum"

      - name: Validate fixture YAML files
        run: |
          cd pkg
          go test -v ./testfixtures/... -run TestYAML
```

### 2. Lint Workflow (`.github/workflows/lint.yml`)

Go linting with golangci-lint:

```yaml
name: Lint

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  golangci-lint:
    name: golangci-lint
    runs-on: ubuntu-latest
    strategy:
      matrix:
        module: [pkg, penfold-go-pipeline]

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.22"
          cache: true
          cache-dependency-path: ${{ matrix.module }}/go.sum

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest
          working-directory: ${{ matrix.module }}
          args: --timeout=5m
```

### 3. Proto Workflow (`.github/workflows/proto.yml`)

Protocol Buffer validation with Buf:

```yaml
name: Proto

on:
  push:
    branches: [main]
    paths:
      - "api/proto/**"
      - "buf.yaml"
      - "buf.gen.yaml"
  pull_request:
    branches: [main]
    paths:
      - "api/proto/**"
      - "buf.yaml"
      - "buf.gen.yaml"

jobs:
  buf-lint:
    name: Buf Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Buf
        uses: bufbuild/buf-setup-action@v1
        with:
          version: latest

      - name: Buf lint
        working-directory: api/proto
        run: buf lint

  buf-breaking:
    name: Buf Breaking Change Detection
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Buf
        uses: bufbuild/buf-setup-action@v1
        with:
          version: latest

      - name: Buf breaking
        working-directory: api/proto
        run: buf breaking --against ".git#branch=main,subdir=api/proto"
```

### 4. Docker Build Workflow (`.github/workflows/docker.yml`)

```yaml
name: Docker

on:
  push:
    branches: [main]
    tags: ["v*"]
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build:
    name: Build and Push
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

  scan:
    name: Security Scan
    runs-on: ubuntu-latest
    needs: build
    if: github.event_name != 'pull_request'
    permissions:
      contents: read
      packages: read
      security-events: write

    steps:
      - uses: actions/checkout@v4

      - name: Run Trivy vulnerability scanner
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ github.ref_name }}
          format: "sarif"
          output: "trivy-results.sarif"
          severity: "CRITICAL,HIGH"

      - name: Upload Trivy scan results
        uses: github/codeql-action/upload-sarif@v3
        if: always()
        with:
          sarif_file: "trivy-results.sarif"
```

### 5. Release Workflow (`.github/workflows/release.yml`)

Cross-platform Go binary releases:

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write

env:
  GO_VERSION: '1.22'

jobs:
  build-cli:
    name: Build CLI (${{ matrix.os }}/${{ matrix.arch }})
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - os: darwin
            arch: arm64
          - os: darwin
            arch: amd64
          - os: linux
            arch: amd64
          - os: linux
            arch: arm64

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Get version info
        id: version
        run: |
          echo "version=${GITHUB_REF_NAME}" >> $GITHUB_OUTPUT
          echo "commit=${GITHUB_SHA::8}" >> $GITHUB_OUTPUT
          echo "build_time=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >> $GITHUB_OUTPUT

      - name: Build CLI binary
        env:
          GOOS: ${{ matrix.os }}
          GOARCH: ${{ matrix.arch }}
          CGO_ENABLED: 0
        run: |
          cd cmd/penf
          go build -ldflags "\
            -X main.version=${{ steps.version.outputs.version }} \
            -X main.commit=${{ steps.version.outputs.commit }} \
            -X main.buildTime=${{ steps.version.outputs.build_time }}" \
            -o penf .

      - name: Prepare artifacts
        run: |
          mkdir -p dist
          cp cmd/penf/penf dist/penf-${{ matrix.os }}-${{ matrix.arch }}

      - name: Upload binary artifact
        uses: actions/upload-artifact@v4
        with:
          name: penf-${{ matrix.os }}-${{ matrix.arch }}
          path: dist/penf-${{ matrix.os }}-${{ matrix.arch }}

  release:
    name: Create Release
    needs: build-cli
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Download all artifacts
        uses: actions/download-artifact@v4
        with:
          path: dist

      - name: Prepare release assets
        run: |
          mkdir -p release
          find dist -name "penf-*" -type f -exec cp {} release/ \;
          chmod +x release/penf-*
          ls -la release/

      - name: Generate checksums
        run: |
          cd release
          sha256sum penf-* > checksums.txt
          cat checksums.txt

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v1
        with:
          body: |
            ## Installation

            ### Update existing installation (recommended)
            ```bash
            penf update
            ```

            ### Fresh install
            ```bash
            # macOS (Apple Silicon)
            curl -L -o /usr/local/bin/penf https://github.com/otherjamesbrown/penfold/releases/download/${{ github.ref_name }}/penf-darwin-arm64
            chmod +x /usr/local/bin/penf

            # macOS (Intel)
            curl -L -o /usr/local/bin/penf https://github.com/otherjamesbrown/penfold/releases/download/${{ github.ref_name }}/penf-darwin-amd64
            chmod +x /usr/local/bin/penf

            # Linux (x86_64)
            curl -L -o /usr/local/bin/penf https://github.com/otherjamesbrown/penfold/releases/download/${{ github.ref_name }}/penf-linux-amd64
            chmod +x /usr/local/bin/penf

            # Linux (ARM64)
            curl -L -o /usr/local/bin/penf https://github.com/otherjamesbrown/penfold/releases/download/${{ github.ref_name }}/penf-linux-arm64
            chmod +x /usr/local/bin/penf
            ```

            Then initialize:
            ```bash
            penf init
            ```
          draft: false
          prerelease: ${{ contains(github.ref, '-alpha') || contains(github.ref, '-beta') || contains(github.ref, '-rc') }}
          files: |
            release/penf-*
            release/checksums.txt
          generate_release_notes: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Linting Configuration

### golangci-lint

The project uses `golangci-lint` for comprehensive Go linting. Run locally with:

```bash
# Run on all modules
make lint

# Run on specific module
cd pkg && golangci-lint run --timeout=5m ./...
```

### go vet

Static analysis with `go vet`:

```bash
# Run on all modules
make vet

# Run on specific module
cd pkg && go vet ./...
```

### Running Locally

```bash
# Full linting suite
make lint vet

# Check a specific module
cd services/gateway && golangci-lint run ./... && go vet ./...
```

## Test Execution

### Test Categories

The test suite is organized into categories:

| Category | Location | Purpose | CI Stage |
|----------|----------|---------|----------|
| Unit | `<module>/*_test.go` | Fast, isolated tests | All PRs |
| Integration | `tests/integration/` | Database/service tests | PRs + main |
| E2E | `tests/e2e/` | Full pipeline with LLM | main only |
| Fixtures | `pkg/testfixtures/` | YAML validation | All PRs |

### Running Tests Locally

```bash
# Run all tests via Makefile
make test

# Run tests for specific module
cd pkg && go test -v -race ./...

# Run unit tests only (short mode)
go test -v -race -short ./...

# Run with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # View in browser

# Run integration tests (requires database)
cd tests && go test -tags=integration -v ./integration/...

# Run E2E tests (requires LLM)
cd tests && go test -tags=e2e -v -timeout 10m ./e2e/...
```

### Coverage Requirements

Coverage is tracked per module and uploaded to Codecov. Target coverage:
- Core packages (`pkg/`): >= 80%
- Services: >= 70%

## Quality Gates

### Mandatory Checks (All PRs)

| Gate | Tool | Threshold | Failure Action |
|------|------|-----------|----------------|
| Linting | golangci-lint | Zero errors | Block merge |
| Static Analysis | go vet | Zero warnings | Block merge |
| Unit Tests | go test | All pass | Block merge |
| Proto Lint | buf lint | Zero errors | Block merge (if protos changed) |
| Breaking Changes | buf breaking | None | Block merge (if protos changed) |

### Integration Requirements (Main Branch)

| Gate | Tool | Threshold | Failure Action |
|------|------|-----------|----------------|
| Integration Tests | go test | All pass | Block merge |
| E2E Tests | go test | All pass | Warning |
| Security Scan | Trivy | No critical/high | Warning |
| Docker Build | Docker | Successful | Block merge |

### Release Requirements (Tags)

| Gate | Tool | Threshold | Failure Action |
|------|------|-----------|----------------|
| All CI Passes | GitHub Actions | Green | Block release |
| Cross-compile | go build | All platforms | Block release |
| Checksums | sha256sum | Generated | Block release |

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
│  • Local binaries    • Auto deploy      • Manual approval             │
│  • Local services    • Real services    • Full monitoring             │
│  • Hot rebuild       • Smoke tests      • Rollback ready              │
│                                                                       │
└─────────────────────────────────────────────────────────────────────┘
```

### Staging Deployment

Automatic deployment to staging on merge to main via Docker workflow.

### Production Deployment

Manual deployment to production for tagged releases via release workflow.

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
# Ensure all tests pass
make all

# Create and push tag
git tag -a v1.0.0 -m "Release v1.0.0: Description"
git push origin v1.0.0
```

### Release Checklist

1. [ ] All CI checks passing on main
2. [ ] Documentation updated
3. [ ] Tag created with descriptive message
4. [ ] GitHub Release created with cross-platform binaries
5. [ ] Docker image published
6. [ ] Staging deployment verified
7. [ ] Production deployment approved and executed

## Local Development Setup

### Prerequisites

```bash
# Install Go 1.22+
# Install Docker (for integration tests)
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Clone repository
git clone https://github.com/otherjamesbrown/penfold.git
cd penfold
```

### Running CI Checks Locally

```bash
# Run full CI suite locally
make all

# Individual targets:
make lint     # Run golangci-lint on all modules
make vet      # Run go vet on all modules
make build    # Build all Go services
make test     # Run all tests

# Run tests with coverage
make test-coverage
```

### Makefile Reference

```makefile
# Available targets:
make all            # lint, vet, build, test
make build          # Build all Go services
make test           # Run all tests
make test-coverage  # Run tests with coverage
make lint           # Run golangci-lint on all modules
make vet            # Run go vet on all modules
make proto          # Generate protobuf code
make proto-lint     # Lint protobuf files
make proto-breaking # Check for breaking proto changes
make deps           # Download dependencies for all modules
make tidy           # Run go mod tidy for all modules
make clean          # Clean build artifacts
make help           # Show available targets
```

## Troubleshooting

### Common CI Failures

| Error | Cause | Solution |
|-------|-------|----------|
| golangci-lint failed | Linting violations | Run `golangci-lint run --fix ./...` |
| go vet errors | Static analysis issues | Fix the reported issues |
| Test timeout | Slow tests or deadlock | Add `-timeout` flag, check for blocking code |
| Integration test failure | Database not ready | Check service health, increase wait time |
| Docker build failed | Missing dependencies | Check Dockerfile and go.mod |
| Proto lint failed | Schema violations | Run `buf lint` locally and fix |

### Debugging CI

```bash
# View workflow run logs
gh run view <run-id> --log

# Re-run failed jobs
gh run rerun <run-id> --failed

# List recent workflow runs
gh run list --workflow=test.yml
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
| `GITHUB_TOKEN` | Container registry, releases | Automatic |
| `CODECOV_TOKEN` | Coverage upload | Optional |
| `PENFOLD_DB_PASSWORD` | E2E test database | E2E tests |

## Monitoring CI Health

### Metrics to Track

- Build success rate (target: >95%)
- Average CI duration (target: <10 minutes for unit tests)
- Flaky test rate (target: <1%)
- Time to merge after PR approval

### Alerts

Set up notifications for:
- CI pipeline failures on main branch
- Deployment failures
- Security vulnerability detection
- Coverage drops below threshold
