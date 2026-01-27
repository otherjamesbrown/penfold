# Architecture Review

Weekly architecture health check for Penfold. Detects drift, regressions, and emerging issues before they become problems.

## Arguments: $ARGUMENTS

- `quick` - Fast health check (single pass, ~5 min)
- `deep` - Full multi-pass review (default, ~20 min)
- `focus:<area>` - Emphasize specific area (e.g., `focus:gateway`, `focus:worker`)

## Instructions

### Setup

```bash
REVIEW_DIR="review/arch/$(date -u +%Y-%m-%dT%H-%M-%SZ)"
mkdir -p "$REVIEW_DIR"
echo "Review directory: $REVIEW_DIR"

# Find previous review for comparison
PREV_REVIEW=$(ls -d review/arch/20* 2>/dev/null | tail -1)
echo "Previous review: ${PREV_REVIEW:-none}"
```

### Mode Selection

If `$ARGUMENTS` contains "quick":
- Run only Pass 1 (Quick Health Check)
- Skip detailed analysis passes
- Output simple health score

Otherwise, run full review (all passes).

---

## Quick Mode (single pass)

**File:** `$REVIEW_DIR/health-check.md`

```
You are reviewing the Penfold codebase for a quick weekly health check.

## Penfold Architecture Overview
Penfold is a personal information system with:
- Gateway service (gRPC API, services/gateway/)
- Worker service (Temporal workflows, services/worker/)
- CLI (cmd/penf/)
- PostgreSQL + pgvector for storage
- MLX for local embeddings

## Quick Checks

Perform these rapid checks:

1. **Build Health**
   ```bash
   go build ./... 2>&1 | head -20
   go test ./... -race -short 2>&1 | tail -20
   ```

2. **Dependency Health**
   - Check go.mod for outdated/vulnerable deps
   - Any new dependencies since last review?

3. **Code Complexity Spot Check**
   - Sample 3-5 recently modified files
   - Any functions > 50 lines? Any obvious code smells?

4. **Test Coverage Trend**
   - Are new files covered by tests?
   - Any test files removed?

5. **Architecture Drift**
   - Any new packages that don't fit existing structure?
   - Cross-boundary imports that shouldn't exist?

If previous review exists at $PREV_REVIEW, compare:
- New concerns introduced?
- Previous concerns resolved?
- Health score trend (improving/stable/declining)?

## Output Format

# Weekly Health Check - [date]

## Health Score: X/5
(1=critical issues, 3=healthy, 5=excellent)

## Build Status
- [ ] Builds clean
- [ ] Tests pass
- [ ] No race conditions

## Quick Findings
### New Since Last Review
- ...

### Concerns
- ...

### Improvements Noted
- ...

## Comparison to Previous Review
(if available)

## Recommendation
[ ] No action needed
[ ] Minor attention needed: ...
[ ] Schedule deep review: ...

Write to: $REVIEW_DIR/health-check.md
```

**After quick mode, output:**
```
Architecture Health Check Complete
==================================
Score: X/5
Status: [Healthy | Attention Needed | Deep Review Recommended]
Report: $REVIEW_DIR/health-check.md

Previous scores: [list last 3 if available]
```

---

## Deep Mode (full review)

### Pass 1: Context & Baseline

**File:** `$REVIEW_DIR/01-context.md`

```
You are an expert architect reviewing Penfold's architecture.

## System Context

Read these files to understand Penfold:
- CLAUDE.md (entry point)
- context/root-agent.md (architecture overview)
- context/shared/vision.md (system purpose)
- context/shared/entities.md (data model)

## Penfold Architecture

Key components:
| Component | Location | Purpose |
|-----------|----------|---------|
| Gateway | services/gateway/ | gRPC API, routing, auth |
| Worker | services/worker/ | Temporal workflows, async processing |
| CLI | cmd/penf/ | User/AI interface |
| AI | pkg/ai/, pkg/search/ | Embeddings, search, LLM |
| Data | pkg/storage/, migrations/ | PostgreSQL, pgvector |

Key patterns:
- Temporal for durable workflows
- gRPC for service communication
- Repository pattern for data access
- Tenant isolation via RLS

## Baseline Comparison

If previous review exists at $PREV_REVIEW:
- Read $PREV_REVIEW/05-synthesis.md for previous health scores
- Note which issues were flagged previously
- Track what's been addressed vs still outstanding

## Output

Document:
1. Current understanding of system goals
2. Key architectural decisions in place
3. Previous review findings (if any)
4. Specific areas of focus for this review

Write to: $REVIEW_DIR/01-context.md
```

### Pass 2: Structure & Security

**File:** `$REVIEW_DIR/02-structure-security.md`

```
You are reviewing Penfold for structure and security.

First read: $REVIEW_DIR/01-context.md

## Structure Analysis

1. **Service Boundaries**
   - Is gateway/worker separation clean?
   - Any inappropriate cross-service dependencies?
   - Package organization within services?

2. **Dependency Direction**
   - Do dependencies flow inward (domain at center)?
   - Any circular dependencies?
   - Is pkg/ truly reusable across services?

3. **Code Organization**
   - Consistent patterns across packages?
   - Clear separation of concerns?
   - Test file organization?

## Security Analysis

1. **Authentication & Authorization**
   - Tenant isolation (RLS policies)?
   - API authentication?
   - Token handling?

2. **Input Validation**
   - gRPC request validation?
   - SQL injection prevention (parameterized queries)?
   - Path traversal risks?

3. **Secrets Management**
   - No hardcoded secrets?
   - Environment variable handling?
   - Credential storage?

4. **Data Flow**
   - PII handling in logs?
   - Sensitive data exposure in errors?

## Output

Rate each area 1-5 and note specific concerns.

Write to: $REVIEW_DIR/02-structure-security.md
```

### Pass 3: Performance & Reliability

**File:** `$REVIEW_DIR/03-performance-reliability.md`

```
You are reviewing Penfold for performance and reliability.

First read:
- $REVIEW_DIR/01-context.md
- $REVIEW_DIR/02-structure-security.md

## Performance Analysis

1. **Database Patterns**
   - N+1 query risks?
   - Missing indexes (check migrations/)?
   - Connection pooling?
   - Large result set handling?

2. **Concurrency**
   - Goroutine management?
   - Context cancellation?
   - Race condition risks?

3. **Resource Management**
   - Memory allocation patterns?
   - File handle cleanup?
   - gRPC stream handling?

## Reliability Analysis

1. **Error Handling**
   - Consistent error wrapping?
   - Appropriate error types?
   - Recovery from panics?

2. **Temporal Workflows**
   - Activity timeout configuration?
   - Retry policies appropriate?
   - Workflow determinism?

3. **Observability**
   - Logging coverage?
   - Metrics instrumentation?
   - Trace propagation?

## Output

Rate each area 1-5 and note specific concerns.

Write to: $REVIEW_DIR/03-performance-reliability.md
```

### Pass 4: Maintainability & Testing

**File:** `$REVIEW_DIR/04-maintainability.md`

```
You are reviewing Penfold for maintainability and test quality.

First read:
- $REVIEW_DIR/01-context.md
- $REVIEW_DIR/02-structure-security.md
- $REVIEW_DIR/03-performance-reliability.md

## Maintainability Analysis

1. **Code Complexity**
   - Functions too long (>50 lines)?
   - Deep nesting?
   - Cognitive complexity?

2. **Consistency**
   - Naming conventions followed?
   - Error handling patterns consistent?
   - Code style uniform?

3. **Documentation**
   - Package docs present?
   - Complex logic commented?
   - API documentation current?

## Test Quality

1. **Coverage**
   - Unit test coverage adequate?
   - Integration tests for critical paths?
   - E2E tests for user journeys?

2. **Test Design**
   - Tests isolated?
   - Appropriate use of mocks?
   - Test data management?

3. **Test Infrastructure**
   - CI pipeline reliable?
   - Test execution fast enough?
   - Flaky tests?

## Output

Rate each area 1-5 and note specific concerns.

Write to: $REVIEW_DIR/04-maintainability.md
```

### Pass 5: Synthesis & Action Plan

**File:** `$REVIEW_DIR/05-synthesis.md`

```
You are creating the final architecture review synthesis.

Read all previous passes:
- $REVIEW_DIR/01-context.md
- $REVIEW_DIR/02-structure-security.md
- $REVIEW_DIR/03-performance-reliability.md
- $REVIEW_DIR/04-maintainability.md

Also check if previous review exists: $PREV_REVIEW/05-synthesis.md

## Synthesis Tasks

1. **Consolidate Findings**
   - Extract all concerns from passes 2-4
   - Deduplicate overlapping issues
   - Resolve any contradictions

2. **Calculate Health Scores**

   | Dimension | Score | Trend | Notes |
   |-----------|-------|-------|-------|
   | Structure | 1-5 | ↑↓→ | ... |
   | Security | 1-5 | ↑↓→ | ... |
   | Performance | 1-5 | ↑↓→ | ... |
   | Reliability | 1-5 | ↑↓→ | ... |
   | Maintainability | 1-5 | ↑↓→ | ... |
   | Testing | 1-5 | ↑↓→ | ... |
   | **Overall** | 1-5 | ↑↓→ | ... |

3. **Prioritize Issues**
   - Critical (blocks progress or causes failures)
   - High (should address this week)
   - Medium (address within month)
   - Low (backlog)

4. **Compare to Previous Review**
   - Resolved issues ✓
   - Persistent issues ⚠
   - New issues 🆕
   - Regressions 🔴

5. **Action Items**
   - Quick wins (< 1 hour)
   - Scheduled work (needs planning)
   - Technical debt to track

## Output Format

# Architecture Review Synthesis
## Date: [timestamp]
## Overall Health: X/5 [trend]

## Executive Summary
[2-3 sentences on overall health]

## Health Scores
[table from above]

## Comparison to Previous Review
[what changed]

## Critical Issues
[list with owners/deadlines if applicable]

## Prioritized Recommendations
### This Week
1. ...

### This Month
1. ...

### Backlog
1. ...

## Quick Wins
- [ ] ...

## Notes for Next Review
[things to watch]

Write to: $REVIEW_DIR/05-synthesis.md
```

---

### Execution

For **quick mode**, run only the Quick Health Check agent.

For **deep mode**, run passes 1-5 sequentially using Task tool with:
- `subagent_type`: "general-purpose"
- `model`: "opus"

After completion:

```
Architecture Review Complete
============================
Mode: [quick|deep]
Location: $REVIEW_DIR

Overall Health: X/5 [trend vs previous]

Scores:
  Structure:      X/5 [trend]
  Security:       X/5 [trend]
  Performance:    X/5 [trend]
  Reliability:    X/5 [trend]
  Maintainability: X/5 [trend]
  Testing:        X/5 [trend]

Critical Issues: N
Action Items: N

Start with: $REVIEW_DIR/05-synthesis.md
Previous review: $PREV_REVIEW

Related commands:
  /review-context  - Review documentation consistency
  bd list          - View existing beads
```

### Creating Beads

After review, ask:
"Found N issues. Create beads for tracking? (yes/no)"

If yes, create beads only for High/Critical issues:
```bash
bd create --title "arch: [brief description]" --type task --priority [1-2] --labels "arch-review,$(date +%Y-%m)"
```
