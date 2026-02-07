---
description: "Dashboard: show current state of the entire bug processing pipeline at a glance."
---

# Bug Status

Show current state of the entire bug processing pipeline. Read-only dashboard.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

---

## Step 1: Check Inbox for Unprocessed Bugs

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.creator, s.created_at
FROM unread_for('penfold', 'agent-mycroft') u
JOIN shards s ON s.id = u.id
WHERE s.type = 'message'
ORDER BY s.created_at;
"
```

## Step 2: Query All Bug-Related Shards

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status, s.type, s.created_at, s.closed_at,
  s.closed_reason, s.owner, s.priority
FROM shards s
WHERE s.project = 'penfold'
  AND (s.title LIKE 'investigate:%'
    OR s.title LIKE 'fix:%'
    OR s.title LIKE 'Implement:%'
    OR s.id IN (SELECT DISTINCT e.to_id FROM edges e
                JOIN shards inv ON inv.id = e.from_id
                WHERE inv.title LIKE 'investigate:%'
                AND e.edge_type = 'discovered-from'))
ORDER BY s.created_at DESC;
"
```

## Step 3: Check Implementation Queue Dependencies

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status, s.owner,
  (SELECT array_agg(e.to_id) FROM edges e
   WHERE e.from_id = s.id AND e.edge_type = 'blocked-by'
   AND (SELECT status FROM shards WHERE id = e.to_id) != 'closed') as blocked_by,
  (SELECT array_agg(l.label) FROM labels l
   WHERE l.shard_id = s.id AND l.label LIKE 'agent:%') as agent_type,
  (SELECT array_agg(fc.file_path) FROM file_claims fc
   WHERE fc.shard_id = s.id) as claimed_files
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%')
ORDER BY s.priority, s.created_at;
"
```

## Step 4: Check Ready Tasks

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT * FROM ready_tasks('penfold');
"
```

## Step 5: Display Dashboard

Categorize all shards by pipeline stage and display:

```
BUG PIPELINE STATUS
═══════════════════

INBOX (unprocessed):              N
INVESTIGATING (debugger active):  N
AWAITING TRIAGE (needs shards):   N
QUEUED (blocked):                 N
READY (can implement):            N
IN PROGRESS (agent working):      N
COMPLETED (fixed & replied):      N

─── INBOX ───────────────────────────────────────────────
[id] | [title] | from [creator] | [age]

─── INVESTIGATING ───────────────────────────────────────
[id] | [title] | owner: [owner] | started [time ago]

─── AWAITING TRIAGE ─────────────────────────────────────
[id] | [title] | closed: [reason summary]
(investigations complete but no impl shard created)

─── QUEUED (blocked) ────────────────────────────────────
[id] | [title] | [agent-type] | blocked by: [shard ids]

─── READY ───────────────────────────────────────────────
[id] | [title] | [agent-type] | files: [claimed files]

─── IN PROGRESS ─────────────────────────────────────────
[id] | [title] | [agent-type] | owner: [owner]

─── COMPLETED ───────────────────────────────────────────
[id] | [title] | closed: [summary] | [time ago]
```

### Stage Classification Rules

| Stage | Criteria |
|-------|----------|
| INBOX | Unread messages in inbox |
| INVESTIGATING | `investigate:` shards with status `open` or `in_progress` |
| AWAITING TRIAGE | `investigate:` shards with status `closed` AND no linked `fix:` shard |
| QUEUED | `fix:` shards with status `open` AND has unresolved `blocked-by` edges |
| READY | `fix:` shards with status `open` AND no unresolved `blocked-by` edges |
| IN PROGRESS | `fix:` shards with status `in_progress` |
| COMPLETED | `fix:` shards with status `closed` |

### Suggested Actions

Based on the pipeline state, suggest the appropriate next action:

| State | Suggestion |
|-------|-----------|
| Inbox has items | `/ingest` to process new items |
| Awaiting triage has items | `/bug-triage` to create impl shards |
| Ready has items | `/implement-next` to launch agents |
| Everything complete | No action needed |
| Investigating active | Wait for debuggers to finish |
| In progress active | Wait for agents to finish |

```
Suggested: /[skill-name] ([reason])
```

## Key Principles

1. **Read-only** - this skill only queries and displays, never modifies
2. **Complete picture** - show every stage of the pipeline
3. **Actionable** - always suggest the appropriate next skill to run
4. **Quick** - minimize queries, display results efficiently
