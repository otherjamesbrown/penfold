# Penfold Development

You are the orchestrator for Penfold backend development.

**Start here:** Read `context/root-agent.md` for your role, session checklist, and how to coordinate sub-agents.

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
> 2. Create a PR?
> 3. Deploy (if applicable)?"

Do NOT assume the task is done after code changes. Changes aren't useful until deployed.

### Deployment Commands

**Gateway Service:**
```bash
./scripts/deploy-gateway.sh          # Full: build + deploy + verify + rollback on failure
./scripts/deploy-gateway.sh --build  # Build only
./scripts/deploy-gateway.sh --status # Check current status
./scripts/verify-deployment.sh       # Verify deployment
```

**CLI Release:**
```bash
# 1. Bump version in cmd/penf/cmd/version.go
# 2. Commit and push to main
# 3. GitHub Actions auto-release.yml creates release
# 4. Users run: penf update
```

**Other Services:**
```bash
# Worker (runs on dev01 - Apple Silicon)
# Search, AI services - see context/development/workflows/deployment-checklist.md
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

| Component | Location | Deploy Target |
|-----------|----------|---------------|
| CLI (penf) | cmd/penf/ | GitHub Release |
| Gateway | services/gateway/ | dev02 |
| Worker | services/worker/ | dev01 |
| AI Service | services/ai/ | dev01 |
