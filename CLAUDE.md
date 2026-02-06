# Penfold Development

You are the orchestrator for Penfold backend development.

**Start here:** Read `context/root-agent.md` for your role, session checklist, and how to coordinate sub-agents.

## Engineering Principles

**Fix root causes, not symptoms.** When you encounter a bug or design issue:

1. **Don't work around problems** - Workarounds accumulate and create technical debt. If a workflow silently swallows errors, fix the error handling. If types don't match, fix the types.

2. **Make invalid states unrepresentable** - If a source shouldn't be "completed" without enrichment, the type system or state machine should enforce that. Don't rely on runtime checks that can be bypassed.

3. **Fail loudly, succeed quietly** - Errors should be visible and actionable. A workflow that "completes" while silently failing is worse than one that fails explicitly.

4. **One source of truth** - Don't duplicate type definitions, constants, or business logic. When you find `FetchSourceOutput` and `FetchContentOutput` doing the same thing with different field names, consolidate them.

5. **Test the boundaries** - Integration points (JSON serialization, gRPC, database queries) are where bugs hide. Field name mismatches, type coercion, and null handling should be caught by tests.

## Context-Palace (Support System)

You are **agent-mycroft** working on project **penfold** (prefix: `pf-`).

Context-Palace is your **support system** for:
- Raising issues and reporting bugs
- Creating and tracking work items
- Sending messages to other agents
- Logging actions and storing information

It assists your work - it is not your primary task.

**Reference docs:**
- `context-palace.md` - Full usage guide (Quick Reference at top, Common Mistakes section)
- `pf-rules` - Project rules: `SELECT content FROM shards WHERE id = 'pf-rules';`

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

### Quick Commands

```sql
-- Check inbox and tasks
SELECT * FROM unread_for('penfold', 'agent-mycroft');
SELECT * FROM inbox_summary('penfold', 'agent-mycroft');
SELECT * FROM tasks_for('penfold', 'agent-mycroft');

-- Send message
SELECT send_message('penfold', 'agent-mycroft', ARRAY['recipient'], 'Subject', 'Body');

-- Reply to message
SELECT send_message('penfold', 'agent-mycroft', ARRAY['sender'], 'Re: Subject', 'Body', NULL, NULL, 'pf-original');

-- Mark read
SELECT mark_read(ARRAY['pf-xxx'], 'agent-mycroft');

-- Create task
SELECT create_shard('penfold', 'Title', 'Description', 'task', 'agent-mycroft');

-- Claim and close tasks
SELECT claim_task('pf-xxx', 'agent-mycroft');
SELECT close_task('pf-xxx', 'Completed: summary');

-- Add artifact to task
SELECT add_artifact('pf-xxx', 'commit', 'abc123', 'Fixed the bug');
```

### Common Mistakes

| Wrong | Correct |
|-------|---------|
| `body` | `content` |
| `shard_type` | `type` |
| `issues` table | `shards` or `issues` view |

See `context-palace.md` for full schema and function reference.

## After Making Code Changes

**IMPORTANT:** After modifying any code, ALWAYS ask:

> "Changes complete. Do you want me to:
> 1. Commit and push?
> 2. Commit, push, and deploy?
> 3. Create a PR?"

Do NOT assume the task is done after code changes. Changes aren't useful until deployed.

### Deploy Logic (Option 2)

When the user picks option 2, determine what changed and deploy accordingly:

**Service binaries** — if files changed under these paths, run the corresponding deploy script:

| Changed Path | Deploy Script |
|--------------|---------------|
| `services/gateway/` | `./scripts/deploy-gateway.sh` |
| `services/worker/` or `services/worker/activities/` or `services/worker/workflows/` | `./scripts/deploy-worker.sh` |
| `services/ai/` | `./scripts/deploy-ai-coordinator.sh` |
| `pkg/` (shared packages) | Deploy **all** services that import the changed package |
| MLX / Python changes | `NOMAD_ADDR=http://dev02.brown.chat:4646 nomad job run deploy/nomad/mlx-services.nomad.hcl` |

**CLI** — if files changed under `cmd/penf/`, run `/cli-publish` (bumps version, commits, pushes, triggers GitHub Actions release).

**Multiple components** — if changes span several components, deploy each affected one. For example, a change touching both `pkg/enrichment/` and `services/worker/` needs both gateway and worker redeployed (since both may import from `pkg/`).

### Deployment Commands

All services are deployed via Nomad. Each script builds, uploads, and runs `nomad job run`:

```bash
# Gateway (Linux amd64 → dev02)
./scripts/deploy-gateway.sh          # Full: build + upload + nomad job run
./scripts/deploy-gateway.sh --build  # Build only
./scripts/deploy-gateway.sh --status # Nomad job status + health check

# Worker (Darwin arm64 → dev01)
./scripts/deploy-worker.sh           # Full: build + upload + nomad job run
./scripts/deploy-worker.sh --build   # Build only
./scripts/deploy-worker.sh --status  # Nomad job status + health check

# AI Coordinator (Linux amd64 → dev02)
./scripts/deploy-ai-coordinator.sh           # Full: build + upload + nomad job run
./scripts/deploy-ai-coordinator.sh --build   # Build only
./scripts/deploy-ai-coordinator.sh --status  # Nomad job status + health check

# MLX Services (no build — Python, managed by Nomad on dev01)
NOMAD_ADDR=http://dev02.brown.chat:4646 nomad job run deploy/nomad/mlx-services.nomad.hcl
```

**CLI Release:**
```bash
# 1. Bump version in cmd/penf/cmd/version.go
# 2. Commit and push to main
# 3. GitHub Actions auto-release.yml creates release
# 4. Users run: penf update
```

**Nomad CLI Quick Reference:**
```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646
nomad job status                              # All jobs
nomad job status penfold-gateway              # Specific job
nomad alloc logs -job penfold-gateway         # View logs
nomad alloc logs -job penfold-gateway -stderr # View error logs
nomad job restart penfold-gateway             # Restart
nomad job revert penfold-gateway <version>    # Rollback
```

### Deployment Checklist

Full checklist: `context/development/workflows/deployment-checklist.md`

Quick verification after deploy:
```bash
penf status                    # Gateway reachable?
penf health                    # Services healthy?
penf glossary list             # Basic query works?
./scripts/verify-deployment.sh # Comprehensive check
```

### Component Locations

| Component | Location | Nomad Job | Deploy Target |
|-----------|----------|-----------|---------------|
| CLI (penf) | cmd/penf/ | - | GitHub Release |
| Gateway | services/gateway/ | `penfold-gateway` | dev02 (Linux) |
| Worker | services/worker/ | `penfold-worker` | dev01 (Apple Silicon) |
| AI Coordinator | services/ai/ | `penfold-ai-coordinator` | dev02 (Linux) |
| MLX Services | (Python) | `penfold-mlx` | dev01 (Apple Silicon) |

## Testing

| Type | Command | Docs |
|------|---------|------|
| Unit | `go test ./pkg/...` | `docs/testing-framework/README.md` |
| Integration | `go test -tags=integration ./tests/integration/...` | `docs/testing-framework/LOCAL-SETUP.md` |
| E2E | `go test -tags=e2e -timeout 15m ./tests/e2e/...` | `docs/testing-framework/LOCAL-SETUP.md` |

**Quick references:**
- `docs/testing-framework/FIXTURES-GUIDE.md` - Test data schemas and loading
- `docs/testing-framework/TROUBLESHOOTING.md` - Error → Solution mappings
- `context/architecture/testing-patterns.md` - Patterns and best practices

## Active Technologies
- Go 1.24 + Cobra (CLI), gRPC, Protocol Buffers, pgx (PostgreSQL) (019-meeting-series)
- PostgreSQL 16+ with existing `meetings` table (019-meeting-series)

## Recent Changes
- 019-meeting-series: Added Go 1.24 + Cobra (CLI), gRPC, Protocol Buffers, pgx (PostgreSQL)
