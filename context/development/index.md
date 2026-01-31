# Development Context (Sub-Agents)

> **Minimal context for spawned development agents.**
> You don't need product knowledge - just follow processes and write good code.
> **Last updated:** 2026-01-31

---

## CRITICAL: Deployment Architecture

> **The CLI runs on James's LAPTOP, not on any server. There is NO localhost.**
>
> ```
> LAPTOP (MacBook Pro)     →  dev02 (Gateway :50051)  →  dev01 (Worker)
>      penf CLI                   PostgreSQL               MLX/Temporal
> ```
>
> - **ALL CLI commands go over the network** to dev02.brown.chat:50051
> - **The CLI has NO direct database access** - everything goes through the Gateway
> - The Gateway handles search, review, relationships DIRECTLY (built-in services)
> - There are NO separate Search/Review/Content/Relationship service processes
> - Worker runs on dev01 for MLX access, connects to DB/Temporal on dev02
>
> **Never assume localhost access from CLI. Never bypass the Gateway. Never add direct DB calls to CLI.**

---

## Your Job

You are a **sub-agent** spawned by the root agent (orchestrator) to complete a specific task.

### Work Autonomously

**Don't ask for permission to continue. Don't ask for code review. Just do the work.**

The orchestrator gave you a task via a bead. The plan was already agreed with James. Your job is to execute it end-to-end.

Only ask if you need clarification on what the bead is asking for - not permission to continue.

### Shards Are Everything

**Your task comes from a shard. Your progress goes in the shard. Your completion closes the shard.**

```sql
-- Read your task
SELECT * FROM shards WHERE id = 'pf-xxx';

-- Log progress as you work
SELECT send_message('penfold', 'agent-penfdev', ARRAY['agent-penfdev'], 'Progress', 'Update details', NULL, NULL, 'pf-xxx');

-- Mark complete when done
SELECT close_task('pf-xxx', 'Completed: summary');
```

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

If your session dies, the shard preserves your progress. Update it frequently.

### Critical Rules

1. **Do only what the bead asks** - Nothing more. No "helpful" extras, no refactoring nearby code, no adding features that weren't requested.

2. **Stay in your domain** - You own specific files/packages. Don't touch anything else.

3. **Follow the 3-iteration rule** - If your test doesn't pass after 3 code-test-fix cycles, STOP. Mark the bead as `blocked`, document what you tried, and let the orchestrator reassess.

4. **Update the bead as you work** - Log progress so if your session dies, we know where you were.

5. **Test before committing** - Never commit code that doesn't pass tests.

---

## Session Start - MANDATORY

### Must Read

| Order | Document | Why |
|-------|----------|-----|
| 1 | This file | Entry point for sub-agents |
| 2 | `workflows/shards.md` | Work tracking is mandatory |
| 3 | `workflows/session.md` | Know how to end properly |
| 4 | `standards/autonomy.md` | Know when to ask vs proceed |

### Then Read Your Agent Context

After reading the above, read your specific agent file:
- `../agents/cli-dev.md` - CLI commands, help text, output formatting
- `../agents/data-dev.md` - Database, PostgreSQL, migrations
- `../agents/ai-dev.md` - AI/ML, embeddings, LLM integration
- `../agents/worker-dev.md` - Temporal workflows, activities
- `../agents/gmail-dev.md` - Gmail integration, OAuth, sync
- `../agents/service-dev.md` - Go services, gRPC, protobufs, Gateway
- `../agents/testing-dev.md` - Test infrastructure, fixtures
- `../agents/debugger.md` - Bug investigation (read-only)

---

## Quick Reference

| Task | SQL Command |
|------|-------------|
| Find work | `SELECT * FROM tasks_for('penfold', 'agent-penfdev');` |
| Claim work | `SELECT claim_task('pf-xxx', 'agent-penfdev');` |
| Before ending | `git push` (no sync needed - DB is always live) |
| Show shard | `SELECT * FROM shards WHERE id = 'pf-xxx';` |

---

## Read When Needed

| Situation | Read |
|-----------|------|
| Writing Go code | `standards/go-patterns.md` |
| Working on CLI/Gateway | `standards/architecture.md` |
| Unsure what to work on | `workflows/priorities.md` |
| Releasing CLI changes | `workflows/releases.md` |
| **Deploying to dev02** | `workflows/deployment-checklist.md` |
| Understanding system design | `../ARCHITECTURE.md` |
| Deployment/connections | `../infrastructure.md` |
| Writing tests | `standards/testing.md` |
| Running tests | `/test.unit`, `/test.integration`, `/test.e2e` |

---

## Document Index

### Workflows (HOW to do things)

| Document | Purpose |
|----------|---------|
| `workflows/shards.md` | Shard lifecycle, task grouping, agent assignment |
| `workflows/session.md` | Session close protocol, git workflow |
| `workflows/releases.md` | CLI versioning and release process |
| `workflows/priorities.md` | Finding work, priority guidelines |
| `workflows/deployment-checklist.md` | **MANDATORY after deploying** - verification steps |

### Standards (WHAT to follow)

| Document | Purpose |
|----------|---------|
| `standards/go-patterns.md` | Lint-compliant patterns, error handling, HTTP patterns |
| `standards/architecture.md` | CLI→Gateway→DB flow, gRPC contracts, proto locations |
| `standards/autonomy.md` | When to proceed vs ask, architecture coordination |
| `standards/testing.md` | **MANDATORY** - test rules, no t.Skip on failure, mocks |

---

## Domain Boundaries

**You only write code for your domain.**

If work is needed for another agent:

```sql
-- Create handoff shard
SELECT create_shard('penfold', 'Handoff: description', 'Details', 'task', 'agent-penfdev');
-- Then assign
UPDATE shards SET owner = 'target-agent' WHERE id = 'pf-xxx';
```

**Never modify files outside your domain without explicit handoff.**

---

## Coordination Rules

**NEVER:**
- Work outside your agent domain without handoff bead
- Exceed 30 minutes without documenting progress in shard
- Modify ARCHITECTURE.md without user approval
- Create infrastructure that duplicates existing systems
- Add features, refactor code, or make "improvements" beyond what the shard asks
- Continue past 3 failed test iterations - abandon and mark blocked instead

**ALWAYS:**
- Update shards with progress as you work
- Create handoff shards when crossing domains
- Document what and why in handoffs
- Run tests before committing
- Push to remote before ending
- **Run `./scripts/verify-deployment.sh` after deploying to dev02**

### The 3-Iteration Rule

When working on a shard with a test:

```
write code → run test → FAIL → fix code → run test → FAIL → fix code → run test → FAIL → STOP
```

After 3 code-test-fix cycles without passing:
1. **Stop working** - Don't keep trying
2. **Mark shard as blocked** - Update the shard content to indicate blocked status
3. **Document what you tried** - Add notes to the shard
4. **Report back** - The orchestrator will reassess (maybe the shard was too big, instructions unclear, or you're missing context)

---

## Completion Checklist

Before reporting complete:
- [ ] Shard exists and is in_progress
- [ ] Tests written for new functionality
- [ ] Tests pass: `go test ./... -v`
- [ ] Commits reference shard ID: `[pf-xxx]`
- [ ] Shard closed with commit hash and summary
- [ ] Handoff shards created if needed
- [ ] Work pushed to remote
- [ ] Git status shows "up to date with origin"
- [ ] **If deployed: `./scripts/verify-deployment.sh` passes (exit 0 or 2)**

---

## Report Format

When completing work, report:

```markdown
**Shard**: pf-xxx (closed)

**Summary**: What was accomplished

**Commits**: `abc1234`: description [pf-xxx]

**Files Changed**: path/to/file.go

**Tests**: Added/updated (or "N/A - no new functionality")

**Handoffs**: Shards created for other agents (or "None")

**Domain**: Confirmed work stayed within <agent> domain
```
