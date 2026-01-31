# Context-Palace Usage Guide for Penfold

This document defines how agents on the Penfold project use Context-Palace for coordination.

---

## Agents

| Agent | Role | Responsibilities |
|-------|------|------------------|
| agent-penf | CLI/Frontend | Bug reports, feature requests, user-facing issues |
| agent-penfdev | Backend | Task triage, implementation, spawning sub-agents |
| agent-cxp | Context-Palace | Manages Context-Palace itself - schema, functions, DX improvements |
| human-james | Human | Oversight, prioritization, direction |

---

## Reporting Context-Palace Issues

When you encounter issues with Context-Palace itself (not your project), report them to **agent-cxp**:

**Labels:** `to:agent-cxp`, `kind:bug` or `kind:feature`

```json
{
  "type": "bug|feature|suggestion",
  "component": "schema|functions|queries|dx",
  "description": "what happened or what you want"
}
```

**Examples of what to report:**
- Query errors or unexpected behavior
- Missing helper functions you wish existed
- Schema improvements
- DX (developer experience) suggestions
- Documentation gaps

**Project:** Use your own project prefix (e.g., `pf-xxx` for penfold). Agent-cxp monitors all projects.

---

## Message Types & Formats

### Bug Reports

**Labels:** `kind:bug`, `component:<name>`, `severity:<level>`

```json
{
  "type": "bug",
  "component": "gateway|cli|worker|search|ingest|ai",
  "severity": "blocking|high|medium|low",
  "command": "the command that failed",
  "error": "error message",
  "version": "v0.x.x",
  "reproduces": "always|sometimes|once",
  "blocking_work": "pf-xxx or null"
}
```

Then markdown body with context, steps to reproduce, expected vs actual.

### Feature Requests

**Labels:** `kind:feature`, `component:<name>`

```json
{
  "type": "feature",
  "component": "cli",
  "priority": "high|normal|low",
  "use_case": "one-line summary"
}
```

### Status Updates

**Labels:** `kind:status`

```json
{
  "type": "status",
  "related_task": "pf-xxx",
  "status": "blocked|in_progress|testing|done",
  "blockers": ["list of blockers if any"]
}
```

### RFCs / Proposals

**Labels:** `kind:rfc`

Freeform markdown. Use for protocol discussions, architecture decisions.

---

## Priority Mapping

| Severity | Priority | Meaning |
|----------|----------|---------|
| blocking | P0 | Drop everything |
| high | P1 | Do today |
| medium | P2 | This week |
| low | P3 | Backlog |

---

## Workflows

### Message → Task Workflow

Messages are for **communication**. Tasks are for **work**.

When a message requests work:
1. Create a **task** shard with `parent_id` pointing to the message
2. The task tracks the work item
3. Close the task when work is done
4. Reply to the **message** (not the task) to communicate completion

```sql
-- Create task from message
SELECT create_shard('penfold',
  'Implement: feature X',
  'Task content...',
  'task',
  'agent-penfdev',
  'pf-message-id'  -- parent_id links to original message
);

-- When done, close task and reply to message
SELECT close_task('pf-task-id', 'Completed: summary');
SELECT send_message('penfold', 'agent-penfdev',
  ARRAY['requester'],
  'Re: Original Subject',
  'Your request has been implemented...',
  NULL, NULL,
  'pf-message-id'  -- reply to the message, not the task
);
```

**Key distinction:**
- `parent_id` = structural relationship (task belongs to message)
- `replies-to` edge = conversational threading

### Bug Report → Task

1. **agent-penf** sends bug with JSON frontmatter + labels
2. **agent-penfdev** parses, creates task with `parent_id` = bug shard
3. **agent-penfdev** replies with task ID
4. On completion, close task and reply to original bug

### Task Completion

1. Agent completes work
2. Agent closes task: `SELECT close_task('pf-xxx', 'Done: summary')`
3. Agent sends status update to reporter (reply to original message)

### Message Threading

- Use `replies-to` edge to link replies to original
- Use `get_thread('pf-xxx')` to view full conversation
- Use `parent_id` to link tasks to their originating messages

---

## Edge Types

| Edge | Use |
|------|-----|
| `replies-to` | Message reply chains |
| `discovered-from` | Task created from bug/message |
| `blocks` | Task dependency |
| `relates-to` | Loose association |

---

## Session Workflow

```sql
-- 1. Check inbox
SELECT * FROM unread_for('penfold', 'agent-penf');

-- 2. Check tasks
SELECT * FROM tasks_for('penfold', 'agent-penf');

-- 3. Check claimable tasks
SELECT * FROM ready_tasks('penfold');

-- 4. Mark messages read after processing
INSERT INTO read_receipts (shard_id, agent_id) VALUES ('pf-xxx', 'agent-penf') ON CONFLICT DO NOTHING;
```

---

## Verification Workflow (agent-penf)

### When Receiving an ACK

agent-penfdev sends:
```json
{
  "type": "ack",
  "investigation": "pf-xxx",
  "tasks": ["pf-yyy"],
  "priority": 1
}
```

**My action:**
1. Mark ACK as read
2. Note task ID(s) for tracking
3. No reply needed unless additional info

### When Receiving a Resolution

agent-penfdev sends:
```json
{
  "type": "resolution",
  "bug": "pf-xxx",
  "fixed_by": ["pf-yyy"],
  "summary": "what was done",
  "verify": "penf team add Test"
}
```

**My action:**
1. Run the verification command
2. Reply with verification result

**If fixed:**
```json
{
  "type": "verification",
  "bug": "pf-xxx",
  "status": "confirmed",
  "tested": "penf team add Test"
}
```
Label: `kind:verification`
Edge: `INSERT INTO edges ... VALUES ('pf-VERIFY', 'pf-BUG', 'verifies')`

**If NOT fixed:**
```json
{
  "type": "verification",
  "bug": "pf-xxx",
  "status": "failed",
  "tested": "penf team add Test",
  "error": "still getting error"
}
```

### Regression / Partial Fix Handling

| Situation | Action |
|-----------|--------|
| Same symptom returns | Reply to thread, no new bug |
| Distinct new issue | New bug with `kind:regression`, edge `relates-to` original |
| Partial fix | Verify original fixed, new bug with `kind:partial` for revealed issue |

---

## Edge Types (Complete)

| Edge | From → To | Added By |
|------|-----------|----------|
| `investigates` | investigation → bug | agent-penfdev |
| `implements` | task → investigation | agent-penfdev |
| `discovered-from` | task → bug | agent-penfdev |
| `fixed-by` | bug → task | agent-penfdev |
| `verifies` | verification → bug | agent-penf |
| `relates-to` | new bug → original | agent-penf |
| `replies-to` | reply → original | both |
| `extends` | doc addition → doc | both |

---

## File Claims (Multi-Session Coordination)

When multiple agents/sessions work in parallel, use `file_claims` to prevent conflicts.

### Schema

```sql
CREATE TABLE file_claims (
  id SERIAL PRIMARY KEY,
  file_path TEXT NOT NULL,
  claimed_by TEXT NOT NULL,      -- agent identifier
  shard_id TEXT REFERENCES shards(id),  -- task being worked
  claimed_at TIMESTAMPTZ DEFAULT NOW(),
  released_at TIMESTAMPTZ,       -- NULL = active claim
  UNIQUE(file_path, shard_id)
);
```

### Workflow

**Before starting work:**
```sql
-- Check for conflicts
SELECT file_path, claimed_by, shard_id
FROM file_claims
WHERE file_path IN ('cmd/penf/cmd/pipeline.go', 'cmd/penf/cmd/content.go')
AND released_at IS NULL;
```

**Claim files when creating implementation shards:**
```sql
INSERT INTO file_claims (file_path, claimed_by, shard_id)
VALUES
  ('cmd/penf/cmd/pipeline.go', 'agent-penfdev', 'pf-task-id'),
  ('cmd/penf/cmd/content.go', 'agent-penfdev', 'pf-task-id');
```

**Release claims when task closes** (automatic via trigger on shard close, or manual):
```sql
UPDATE file_claims SET released_at = NOW()
WHERE shard_id = 'pf-task-id' AND released_at IS NULL;
```

### Conflict Resolution

If files are already claimed:
1. Check if the claiming shard is still active
2. Contact the owner to coordinate
3. Wait for release, or find alternative approach

---

## Protocol Version

```json
{
  "protocol_version": "1.2",
  "agreed_by": ["agent-penf", "agent-penfdev", "agent-cxp"],
  "date": "2026-01-29",
  "docs": ["pf-eb8732", "pf-796c58"]
}
```
