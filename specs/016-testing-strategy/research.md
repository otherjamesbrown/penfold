# Research: Testing Strategy

**Feature**: 016-testing-strategy
**Date**: 2026-01-22

## Research Items

### 1. Go Testcontainers vs Direct Database

**Question**: Should we use testcontainers-go for isolated test databases or connect directly to test databases on home-01?

**Decision**: Direct database connection to home-01.brown.chat

**Rationale**:
- Simpler setup - no Docker-in-Docker complexity for CI
- Faster test startup - no container provisioning overhead
- pgvector extension already installed and configured
- Existing infrastructure reused (no additional resources)
- Network latency from dev01 → home-01 is negligible (~1ms)

**Alternatives Considered**:
- Testcontainers-go: More isolated, but adds complexity and startup time
- SQLite for testing: Doesn't support pgvector, incompatible with production

**Implementation**:
```go
// tests/integration/helpers.go
func SetupTestDB(t *testing.T) *pgxpool.Pool {
    dbName := os.Getenv("PENFOLD_TEST_DB") // e.g., penfold_test_integration
    host := os.Getenv("PENFOLD_DB_HOST")   // home-01.brown.chat
    // Connect and return pool
}
```

---

### 2. Go Build Tag Patterns

**Question**: How to properly use build tags to separate test categories?

**Decision**: Use `//go:build` directive (Go 1.17+ syntax) with tags: `integration`, `e2e`, `live`, `flaky`

**Rationale**:
- Modern Go syntax (`//go:build` vs old `// +build`)
- Clear separation of test execution contexts
- CI can run specific subsets: `go test -tags=integration ./...`
- Flaky tests excluded by default, included with `-tags=flaky`

**Implementation**:
```go
// Integration test
//go:build integration

package integration

// E2E test
//go:build e2e

package e2e

// Live test (cloud APIs)
//go:build live

package live

// Flaky test (quarantined)
//go:build flaky

package integration
```

**Run Commands**:
```bash
go test ./...                           # Unit only (no tags)
go test -tags=integration ./...         # Unit + Integration
go test -tags=e2e ./tests/e2e/...       # E2E only
go test -tags=live ./tests/live/...    # Live only (manual)
go test -tags=flaky ./...               # Include quarantined
```

---

### 3. YAML Fixture Loading Patterns

**Question**: Best approach for loading YAML fixtures into Go test databases?

**Decision**: Use `gopkg.in/yaml.v3` with typed structs matching fixture schemas

**Rationale**:
- Standard library-compatible YAML parsing
- Type-safe fixture definitions catch schema errors at compile time
- Easy to extend with new fixture types
- Supports anchors/aliases for DRY fixture data

**Implementation**:
```go
// tests/fixtures/loader.go
type FixtureLoader struct {
    db     *pgxpool.Pool
    logger logging.Logger
}

type PeopleFixture struct {
    People []PersonFixture `yaml:"people"`
}

type PersonFixture struct {
    ID            int64    `yaml:"id"`
    CanonicalName string   `yaml:"canonical_name"`
    Email         string   `yaml:"email"`
    Aliases       []string `yaml:"aliases"`
    Title         string   `yaml:"title"`
    TeamID        int64    `yaml:"team_id"`
}

func (l *FixtureLoader) LoadAcmeCorp(ctx context.Context) error {
    // Load people.yaml, teams.yaml, etc.
    // Insert into test database
}
```

---

### 4. GitHub Actions Self-Hosted Runner

**Question**: How to configure self-hosted runner on dev01 for E2E tests?

**Decision**: Install GitHub Actions runner as user service on dev01.brown.chat

**Rationale**:
- Required for MLX LLM access (Apple Silicon only)
- No cloud costs for Apple Silicon runners
- Full access to local infrastructure (home-01 DB, Redis)
- Persistent runner with launchd service management

**Implementation**:
```bash
# On dev01.brown.chat
mkdir -p ~/actions-runner && cd ~/actions-runner
curl -o actions-runner.tar.gz -L https://github.com/actions/runner/releases/download/v2.311.0/actions-runner-osx-arm64-2.311.0.tar.gz
tar xzf actions-runner.tar.gz

# Configure (get token from GitHub repo settings)
./config.sh --url https://github.com/otherjamesbrown/penfold \
  --token <RUNNER_TOKEN> \
  --labels self-hosted,dev01,apple-silicon,macos

# Install as service
./svc.sh install
./svc.sh start
```

**Workflow Configuration**:
```yaml
e2e:
  runs-on: [self-hosted, dev01]
  env:
    LLM_URL: http://localhost:8080
    PENFOLD_DB_HOST: home-01.brown.chat
```

---

## Summary

All research items resolved. Key decisions:

| Item | Decision |
|------|----------|
| Database isolation | Direct connection to home-01 test databases |
| Build tags | `//go:build integration`, `e2e`, `live`, `flaky` |
| Fixture loading | YAML with typed structs via yaml.v3 |
| CI runner | Self-hosted on dev01 with launchd service |

No blocking unknowns remain. Ready to proceed to Phase 1.
