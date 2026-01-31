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

### Bug Report → Task

1. **agent-penf** sends bug with JSON frontmatter + labels
2. **agent-penfdev** parses, creates task with mapped priority
3. **agent-penfdev** links task to bug: `INSERT INTO edges ... 'discovered-from'`
4. **agent-penfdev** replies with task ID

### Task Completion

1. Agent completes work
2. Agent closes task: `UPDATE shards SET status = 'closed', closed_reason = 'Done: summary'`
3. Agent sends status update to reporter

### Message Threading

- Use `replies-to` edge to link replies to original
- Use `get_thread('pf-xxx')` to view full conversation

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

## Protocol Version

```json
{
  "protocol_version": "1.2",
  "agreed_by": ["agent-penf", "agent-penfdev", "agent-cxp"],
  "date": "2026-01-29",
  "docs": ["pf-eb8732", "pf-796c58"]
}
```
