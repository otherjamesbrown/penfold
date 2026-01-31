# Shards Workflow

**NEVER start work without a shard.**

## Recognizing Shard References

**Shard IDs in this project use the format `pf-<xxx>`** (e.g., `pf-t3st`, `pf-0ilh`).

**Shards are stored in Context-Palace** (PostgreSQL on dev02).

| Role | Tool | Use For |
|------|------|---------|
| **Sub-agents** | `palace` CLI | Task ops (get, claim, progress, close, artifact) |
| **Orchestrator** | `psql` SQL | Complex queries, shard creation, linking, messaging |

When someone asks you to "work on pf-xxx" or references a `pf-` ID:

1. **Get the shard details:** `SELECT * FROM shards WHERE id = 'pf-xxx';`
2. **Read the content** - understand what's being asked
3. **Brief investigation** - understand the scope (5-10 min max)
4. **Spawn a sub-agent** - You are the architect, not the implementer
   - Assign to appropriate agent based on domain (see Agent Assignment below)
   - Pass the shard ID to the sub-agent
   - Let them write the code

**You do NOT write implementation code.** Your job is to:
- Understand the problem
- Break it into smaller shards if needed
- Assign to the right sub-agent
- Coordinate and review

Do NOT search for files matching the shard ID. Shards are tracked in Context-Palace (PostgreSQL) and accessed via SQL.

---

## Connection

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

---

## Essential Commands

```sql
-- Find available work
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- Show shard details
SELECT * FROM shards WHERE id = 'pf-xxx';

-- Create new work
SELECT create_shard('penfold', 'Title', 'Description', 'task', 'agent-penfdev');

-- Claim work (set status to in_progress)
SELECT claim_task('pf-xxx', 'agent-penfdev');

-- Complete work
SELECT close_task('pf-xxx', 'Completed: summary');

-- Add comment to shard
SELECT send_message('penfold', 'agent-penfdev', ARRAY['agent-penfdev'], 'Comment', 'Body', NULL, NULL, 'pf-xxx');
```

## Critical Rules

1. **Find or create shard BEFORE writing code**
2. **Assign agent when creating**: `UPDATE shards SET owner = 'cli-dev' WHERE id = 'pf-xxx';`
3. **Update status when starting**: `SELECT claim_task('pf-xxx', 'agent-penfdev');`
4. **Reference shard in commits**: `feat(component): description [pf-xxx]`
5. **Close with commit hash**: `SELECT close_task('pf-xxx', 'commit <hash>: summary');`
6. **(No sync needed - always live in DB)**

## Agent Assignment

When creating shards, always specify which agent should do the work:

```sql
-- Create shard and assign agent
SELECT create_shard('penfold', 'Fix search help text', 'Details here', 'task', 'agent-penfdev');
-- Then assign
UPDATE shards SET owner = 'cli-dev' WHERE id = 'pf-xxx';

-- Or for investigation work
SELECT create_shard('penfold', 'Investigate flaky test', 'Details here', 'task', 'agent-penfdev');
UPDATE shards SET owner = 'debugger' WHERE id = 'pf-xxx';
```

| Work Type | Assign To |
|-----------|-----------|
| CLI commands, help text, CLI docs | `cli-dev` |
| Database, migrations, repositories | `data-dev` |
| AI/ML features, embeddings | `ai-dev` |
| Temporal workflows, activities | `worker-dev` |
| Gmail connector, OAuth | `gmail-dev` |
| Bug investigation, root cause | `debugger` |
| Test framework, fixtures | `testing-dev` |
| Feature specs, planning | `speckit-dev` |

## After Closing a Shard

When closing a shard that belongs to a group (parent task), check if all related shards are also closed:

```sql
-- Check related shards
SELECT s.id, s.title, s.status
FROM shards s
JOIN edges e ON s.id = e.from_id
WHERE e.to_id = 'pf-parent'
  AND e.edge_type = 'relates-to'
  AND s.status != 'closed';
```

If all related shards are closed (count = 0), suggest closing the parent group task to the user.

### Context Validation

**Before closing, verify context docs are accurate:**

- If implementation changed system behavior -> update `infrastructure.md` or `ARCHITECTURE.md`
- If docs described a "plan" that's now complete -> update status from "planned" to "deployed"
- If you referenced docs that were wrong -> create a shard to fix them

**Don't silently work around stale docs.** Fix them or track the fix.

---

## Task Grouping (relates-to Edges)

**Group related shards using `relates-to` edges** for organization.

### Creating Groups

```sql
-- Create parent task (the "group")
SELECT create_shard('penfold', '[GROUP] Feature Name', 'Overview of the feature', 'task', 'agent-penfdev');
-- Returns: pf-parent

-- Create child task and link
SELECT create_shard('penfold', 'Sub-task 1', 'Details', 'task', 'agent-penfdev');
SELECT link('pf-child1', 'pf-parent', 'relates-to');

-- Create another child task and link
SELECT create_shard('penfold', 'Sub-task 2', 'Details', 'task', 'agent-penfdev');
SELECT link('pf-child2', 'pf-parent', 'relates-to');
```

### Checking Group Status

```sql
-- Check group completion (count open children)
SELECT COUNT(*) FROM shards s
JOIN edges e ON s.id = e.from_id
WHERE e.to_id = 'pf-parent'
  AND e.edge_type = 'relates-to'
  AND s.status != 'closed';
-- When 0, manually close pf-parent
```

### Viewing Group Structure

```sql
-- Show all shards related to a parent
SELECT s.id, s.title, s.status, s.owner
FROM shards s
JOIN edges e ON s.id = e.from_id
WHERE e.to_id = 'pf-parent'
  AND e.edge_type = 'relates-to'
ORDER BY s.created_at;

-- Show all edges for a shard
SELECT * FROM edges WHERE from_id = 'pf-xxx' OR to_id = 'pf-xxx';
```

### Group Naming Convention

- `[GROUP] SpecKit: Complete All Feature Specifications`
- `[GROUP] Operationalization: Dev Agents and Documentation`
- `[GROUP] Integration: Cross-Cutting System Concerns`
- `[GROUP] Maintenance: Cleanup and Audits`

### Rare Non-Group Exceptions

**ONLY create standalone shards for:**
- Immediate blockers preventing all work
- Emergency security issues
- Quick research tasks (<30 min, inform group planning)
- Group creation itself
