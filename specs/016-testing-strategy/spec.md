# Feature Specification: Testing Strategy

**Feature Branch**: `016-testing-strategy`
**Created**: 2026-01-22
**Status**: Draft
**Input**: Review of current testing practices and gap analysis
**Supersedes**: Portions of `010-testing-framework` (now Go-focused, updated infrastructure)

---

## Problem Statement

The current codebase has **160 test files** but they are almost entirely unit tests with mocked dependencies. There is no verification that:

1. Components work together with real databases (PostgreSQL + pgvector)
2. The full ingestion → processing → storage → query pipeline functions correctly
3. LLM integrations parse responses correctly and prompts are effective
4. Entity resolution, glossary expansion, and mention detection work with realistic data

**Current State:**
- Unit tests: Comprehensive coverage of individual functions
- Integration tests: Component wiring with mocks (not real dependencies)
- E2E tests: None (`/tests/e2e/` directory is empty)
- Live LLM tests: None (all AI calls are mocked)

---

## Test Taxonomy

### Overview

| Category | What It Tests | Dependencies | Speed | When to Run |
|----------|---------------|--------------|-------|-------------|
| **Unit** | Pure logic, single function | None (all mocked) | Fast (ms) | Every commit |
| **Integration** | Component + real dependencies | Real DB, Redis | Medium (s) | Every PR |
| **E2E** | Full workflows, mock organization | Real DB + services + local LLM | Slow (min) | Daily / Pre-release |
| **Live** | Real external services | Cloud LLMs (Gemini, etc.) | Slowest | Manual / Weekly |

### Category Definitions

#### Unit Tests
**Purpose**: Verify individual functions/methods work correctly in isolation.

**Characteristics**:
- No external dependencies (database, network, filesystem)
- All dependencies mocked or stubbed
- Deterministic - same input always produces same output
- Fast - entire suite runs in seconds

**Location**: Co-located with source code (`*_test.go` files)

**Example**:
```go
func TestMentionExtractor_Extract(t *testing.T) {
    people := []Person{{ID: 1, CanonicalName: "John Smith", Aliases: []string{"John"}}}
    extractor := NewMentionExtractor(people)
    mentions := extractor.Extract("Talk to John about the project")
    assert.Len(t, mentions, 1)
    assert.Equal(t, "John", mentions[0].MatchedText)
}
```

#### Integration Tests
**Purpose**: Verify components work together with real infrastructure.

**Characteristics**:
- Uses real PostgreSQL + pgvector (via Docker/testcontainers)
- Uses real Redis
- LLM calls still mocked (for speed and determinism)
- Tests actual SQL queries, transactions, vector operations

**Location**: `tests/integration/` or `*_integration_test.go`

**Example**:
```go
func TestGlossaryRepository_ExpandQuery(t *testing.T) {
    db := setupTestDB(t)  // Real PostgreSQL with pgvector
    repo := glossary.NewRepository(db)

    // Seed test data
    repo.Create(ctx, &glossary.Term{Term: "TER", Expansion: "Technical Execution Review"})

    // Test actual query expansion against real DB
    expanded := repo.ExpandQuery(ctx, "What happened at TER?")
    assert.Contains(t, expanded.ExpandedQuery, "Technical Execution Review")
}
```

#### E2E Tests (End-to-End)
**Purpose**: Verify complete workflows with a realistic mock organization.

**Characteristics**:
- Uses real PostgreSQL + pgvector
- Uses real local LLM (vLLM-MLX with Qwen)
- Uses real embeddings service (MLX)
- Tests full pipeline: ingest → process → enrich → store → query
- Uses "Acme Corp" fixture data with known entities

**Location**: `tests/e2e/`

**Example**:
```go
func TestEmailIngestion_MentionResolution(t *testing.T) {
    env := setupE2EEnvironment(t)  // Real DB + real local LLM

    // Load mock organization
    env.LoadFixture("acme-corp")  // People, projects, glossary

    // Ingest test email
    email := loadTestEmail("project-update-with-mentions.eml")
    result, err := env.IngestEmail(ctx, email)
    require.NoError(t, err)

    // Verify mention resolution used real LLM
    assert.Len(t, result.ResolvedMentions, 3)
    assert.Equal(t, "John Smith", result.ResolvedMentions[0].ResolvedTo.CanonicalName)

    // Verify searchable via vector query
    searchResults := env.Search(ctx, "project status update from John")
    assert.NotEmpty(t, searchResults)
    assert.Equal(t, email.ID, searchResults[0].DocumentID)
}
```

#### Live Tests
**Purpose**: Verify integration with real external services.

**Characteristics**:
- Uses real cloud LLM APIs (Gemini, etc.)
- Incurs costs - run sparingly
- Validates API compatibility and response parsing
- Skipped in CI unless explicitly enabled

**Location**: `tests/live/`

**Example**:
```go
//go:build live

func TestGeminiAPI_ResponseParsing(t *testing.T) {
    if os.Getenv("GEMINI_API_KEY") == "" {
        t.Skip("GEMINI_API_KEY not set")
    }

    client := gemini.NewClient(os.Getenv("GEMINI_API_KEY"))
    response, err := client.Complete(ctx, "Extract entities from: Meeting with John about Project Alpha")
    require.NoError(t, err)

    // Verify we can parse real Gemini response format
    entities, err := parseEntities(response)
    require.NoError(t, err)
    assert.NotEmpty(t, entities)
}
```

---

## Mock Organization: "Acme Corp"

E2E tests require a consistent, realistic dataset representing a fictional organization.

### Fixture Structure

```
tests/fixtures/acme-corp/
├── people.yaml           # 20+ people with roles, emails, aliases
├── teams.yaml            # 5+ teams with membership
├── projects.yaml         # 10+ projects with owners, timelines
├── products.yaml         # Products and their associations
├── glossary.yaml         # 50+ acronyms and terms
├── emails/               # Sample emails referencing above entities
│   ├── project-update.eml
│   ├── meeting-request.eml
│   ├── escalation-thread.eml
│   └── jira-notification.eml
└── meetings/             # Sample meeting transcripts
    ├── weekly-standup.vtt
    └── project-kickoff.txt
```

### People Fixture Example

```yaml
# tests/fixtures/acme-corp/people.yaml
people:
  - id: 1
    canonical_name: "John Smith"
    email: "john.smith@acme.com"
    aliases: ["John", "JS", "Smith"]
    title: "VP Engineering"
    team_id: 1

  - id: 2
    canonical_name: "Sarah Chen"
    email: "sarah.chen@acme.com"
    aliases: ["Sarah", "SC"]
    title: "Product Manager"
    team_id: 2

  - id: 3
    canonical_name: "Marcus Rodriguez"
    email: "marcus.r@acme.com"
    aliases: ["Marcus", "MR", "Rodriguez"]
    title: "Head of Sales"
    team_id: 3
```

### Glossary Fixture Example

```yaml
# tests/fixtures/acme-corp/glossary.yaml
terms:
  - term: "TER"
    expansion: "Technical Execution Review"
    definition: "Weekly engineering review meeting"

  - term: "MVP"
    expansion: "Minimum Viable Product"

  - term: "OKR"
    expansion: "Objectives and Key Results"

  - term: "Project Alpha"
    expansion: null
    definition: "Q1 platform migration initiative"
    context: "project"
```

### Test Email Example

```
# tests/fixtures/acme-corp/emails/project-update.eml
From: john.smith@acme.com
To: sarah.chen@acme.com
Cc: marcus.r@acme.com
Subject: RE: Project Alpha MVP Status
Date: Mon, 20 Jan 2026 09:15:00 -0800
Message-ID: <test-001@acme.com>

Hi Sarah,

Quick update on Project Alpha - we discussed the MVP timeline at TER yesterday.
Marcus mentioned some concerns about the Q1 deadline.

Can we sync with the team on Thursday?

Thanks,
John
```

### Expected Test Assertions

When this email is ingested, E2E tests should verify:

1. **Mention Resolution**:
   - "Sarah" → Sarah Chen (person_id: 2)
   - "Marcus" → Marcus Rodriguez (person_id: 3)
   - "John" → John Smith (person_id: 1)

2. **Glossary Expansion**:
   - "TER" → Technical Execution Review
   - "MVP" → Minimum Viable Product

3. **Entity Linking**:
   - "Project Alpha" → project_id from projects table

4. **Vector Search**:
   - Query "Project Alpha status" returns this email
   - Query "TER meeting" returns this email

---

## Infrastructure Requirements

### E2E Test Environment

```
┌─────────────────────────────────────────────────────────────────┐
│                     E2E Test Environment                        │
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌────────────────┐  │
│  │  PostgreSQL     │  │  Redis          │  │  MLX LLM       │  │
│  │  + pgvector     │  │  (testcontainer)│  │  (Qwen 32B)    │  │
│  │  (testcontainer)│  │                 │  │  localhost:8080│  │
│  └────────┬────────┘  └────────┬────────┘  └───────┬────────┘  │
│           │                    │                    │           │
│           └────────────────────┼────────────────────┘           │
│                                │                                │
│                    ┌───────────▼───────────┐                    │
│                    │   Test Harness        │                    │
│                    │   - Fixture loader    │                    │
│                    │   - Pipeline runner   │                    │
│                    │   - Assertion helpers │                    │
│                    └───────────────────────┘                    │
└─────────────────────────────────────────────────────────────────┘
```

### Local LLM Dependency

E2E tests require the local LLM server running:

```bash
# On dev01.brown.chat
# Already managed by launchd: com.penfold.mlx-llm-server

# Model: mlx-community/Qwen2.5-32B-Instruct-4bit
# Endpoint: http://localhost:8080
# API: OpenAI-compatible /v1/chat/completions
```

**Important**: E2E tests will skip if LLM is unavailable:
```go
func setupE2EEnvironment(t *testing.T) *E2EEnv {
    if !isLLMAvailable("http://localhost:8080") {
        t.Skip("Local LLM not available - skipping E2E test")
    }
    // ...
}
```

### Testcontainers for Database

```go
func setupTestDB(t *testing.T) *pgxpool.Pool {
    ctx := context.Background()

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "pgvector/pgvector:pg16",
            ExposedPorts: []string{"5432/tcp"},
            Env: map[string]string{
                "POSTGRES_DB":       "penfold_test",
                "POSTGRES_USER":     "test",
                "POSTGRES_PASSWORD": "test",
            },
            WaitingFor: wait.ForListeningPort("5432/tcp"),
        },
        Started: true,
    })
    require.NoError(t, err)

    t.Cleanup(func() { container.Terminate(ctx) })

    // Run migrations and return pool
    // ...
}
```

---

## Directory Structure

```
penfold/
├── pkg/
│   └── auth/
│       ├── auth.go
│       └── auth_test.go          # Unit tests (co-located)
├── services/
│   └── content/
│       ├── pipeline.go
│       ├── pipeline_test.go      # Unit tests
│       └── pipeline_integration_test.go  # Integration tests
└── tests/
    ├── integration/
    │   ├── db_test.go            # Database integration tests
    │   ├── search_test.go        # Vector search integration
    │   └── helpers.go            # Shared test utilities
    ├── e2e/
    │   ├── ingest_test.go        # Full ingestion pipeline
    │   ├── search_test.go        # Query → results E2E
    │   ├── environment.go        # E2E test harness
    │   └── helpers.go            # Assertion helpers
    ├── live/
    │   ├── gemini_test.go        # Real Gemini API tests
    │   └── gmail_test.go         # Real Gmail API tests
    └── fixtures/
        └── acme-corp/
            ├── people.yaml
            ├── teams.yaml
            ├── projects.yaml
            ├── glossary.yaml
            ├── emails/
            └── meetings/
```

---

## Running Tests

### Commands

```bash
# Unit tests only (fast, no dependencies)
go test ./... -short

# Unit + Integration tests (requires Docker)
go test ./...

# E2E tests (requires Docker + local LLM)
go test ./tests/e2e/... -v

# Live tests (requires API keys, incurs costs)
go test ./tests/live/... -tags=live -v

# All tests with coverage
go test ./... -coverprofile=coverage.out -covermode=atomic
```

### CI Configuration

```yaml
# .github/workflows/test.yml
jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./... -short -race

  integration:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: pgvector/pgvector:pg16
        # ...
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./... -race

  e2e:
    runs-on: [self-hosted, apple-silicon]  # Requires MLX
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./tests/e2e/... -v
```

---

## Success Criteria

### Phase 1: Foundation
- [ ] Integration test infrastructure with testcontainers
- [ ] At least one integration test for each: DB, Redis, Vector search
- [ ] E2E test harness with fixture loading
- [ ] "Acme Corp" fixture with 10+ people, 5+ projects, 20+ glossary terms

### Phase 2: Coverage
- [ ] E2E test for email ingestion → mention resolution
- [ ] E2E test for glossary expansion in search
- [ ] E2E test for full pipeline: ingest → enrich → search
- [ ] Integration tests for all repository classes

### Phase 3: Confidence
- [ ] 80%+ code coverage for core packages
- [ ] E2E tests run in CI on Apple Silicon runner
- [ ] Live tests for cloud LLM APIs (manual trigger)
- [ ] Performance benchmarks for critical paths

---

## Relationship to 010-testing-framework

This spec **supersedes** `010-testing-framework` for practical implementation guidance:

| Aspect | 010-testing-framework | This Spec (016) |
|--------|----------------------|-----------------|
| Language | Python-focused | Go-focused |
| LLM | Ollama | vLLM-MLX with Qwen |
| Scope | Aspirational/theoretical | Practical/actionable |
| Mock Org | Mentioned | Fully specified |
| Taxonomy | Implicit | Explicit categories |

The 010 spec's concepts around AI mocking and test data strategy remain valid context, but this spec provides the concrete implementation plan.

---

## Next Steps

1. **Create fixture structure**: `tests/fixtures/acme-corp/` with YAML files
2. **Build test harness**: `tests/e2e/environment.go` with testcontainers
3. **First E2E test**: Email ingestion with mention resolution
4. **CI integration**: Self-hosted Apple Silicon runner for E2E
