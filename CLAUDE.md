# Penfold Development

You are the orchestrator for Penfold backend development.

**Start here:** Read `context/root-agent.md` for your role, session checklist, and how to coordinate sub-agents.

## Context-Palace

You are **agent-penfdev** working on project **penfold** (prefix: `pf-`).

Context-Palace is your shared memory system. Use it to:
- Create and track tasks/bugs
- Send messages to other agents and humans
- Log your actions
- Store information that needs to persist

**Full guide:** Read `context-palace.md` in this folder.

### Connection

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

### Start of Session

Always check for messages and tasks at the start of a session:

```sql
-- Check inbox
SELECT * FROM unread_for('penfold', 'agent-penfdev');

-- Check your tasks
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- See ready tasks anyone can claim
SELECT * FROM ready_tasks('penfold');
```

### Quick Commands

```sql
-- Mark message read
INSERT INTO read_receipts (shard_id, agent_id) VALUES ('pf-xxx', 'agent-penfdev') ON CONFLICT DO NOTHING;

-- Create task (simple)
SELECT create_shard('penfold', 'Title', 'Details', 'task', 'agent-penfdev');
-- Returns: pf-a1b2c3

-- Create task with owner and priority
SELECT create_shard('penfold', 'Title', 'Details', 'task', 'agent-penfdev', 'target-agent', 2);

-- Send message
SELECT create_shard('penfold', 'Subject', 'Body', 'message', 'agent-penfdev');
INSERT INTO labels (shard_id, label) VALUES ('pf-NEWID', 'to:recipient');

-- Reply to message
SELECT create_shard('penfold', 'Re: Subject', 'Reply text', 'message', 'agent-penfdev');
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-REPLY', 'pf-ORIGINAL', 'replies-to');
INSERT INTO labels (shard_id, label) VALUES ('pf-REPLY', 'to:original-sender');

-- Claim task
UPDATE shards SET owner = 'agent-penfdev', status = 'in_progress' WHERE id = 'pf-xxx' AND owner IS NULL;

-- Close task
UPDATE shards SET status = 'closed', closed_at = NOW(), closed_reason = 'Done: summary' WHERE id = 'pf-xxx';

-- Log an action
SELECT create_shard('penfold', 'Did something', 'Details of action', 'log', 'agent-penfdev');

-- Get conversation thread
SELECT * FROM get_thread('pf-xxx');

-- Search
SELECT id, title, status FROM shards, to_tsquery('english', 'keyword') query
WHERE project = 'penfold' AND search_vector @@ query ORDER BY ts_rank(search_vector, query) DESC LIMIT 10;
```

### Priorities

| Priority | Meaning |
|----------|---------|
| 0 | Critical - drop everything |
| 1 | High - do today |
| 2 | Normal - this week |
| 3 | Low - when possible |

### Message Labels

- `to:agent-xxx` - Send to agent
- `to:human-xxx` - Send to human
- `kind:bug-report` - Bug report
- `kind:status-update` - FYI / progress
- `kind:question` - Needs response

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
