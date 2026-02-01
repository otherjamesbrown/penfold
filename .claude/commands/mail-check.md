# Mail Check

Check inbox for new messages and propose actions using Context-Palace.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_HOST: dev02.brown.chat
DB_NAME: contextpalace
DB_USER: penfold
```

## Instructions

### Step 1: Check Inbox

Fetch unread messages from Context-Palace:

```sql
SELECT * FROM unread_for('penfold', '<AGENT_NAME>');
```

For full content:

```sql
SELECT s.id, s.title, s.content, s.creator, s.created_at, s.type,
       array_agg(DISTINCT l.label) as labels
FROM unread_for('penfold', '<AGENT_NAME>') u
JOIN shards s ON s.id = u.id
LEFT JOIN labels l ON l.shard_id = s.id
GROUP BY s.id, s.title, s.content, s.creator, s.created_at, s.type
ORDER BY s.created_at;
```

### Step 2: Check Tasks

Also check for assigned tasks:

```sql
-- Your tasks
SELECT * FROM tasks_for('penfold', '<AGENT_NAME>');

-- Claimable tasks
SELECT * FROM ready_tasks('penfold');
```

### Step 3: Categorize Messages

For each message, determine category from JSON frontmatter or content:

| Category | JSON type | Typical Action |
|----------|-----------|----------------|
| **Bug Report** | `bug` | Create task P1/P2 |
| **Feature Request** | `feature` | Create task P2/P3 |
| **Question** | `question` | Reply with answer |
| **Status Update** | `status` | Acknowledge only |
| **Blocker** | `bug` + severity:blocking | Create task P0 |

### Step 4: Build Summary

Present a summary table:

```
═══════════════════════════════════════════════════════════════
 INBOX SUMMARY - <AGENT_NAME>
═══════════════════════════════════════════════════════════════

 Messages: X new
 Tasks: Y assigned, Z claimable

 # | From       | Subject                    | Category   | Proposed Action
───┼────────────┼────────────────────────────┼────────────┼─────────────────
 1 | agent-penf | Bug: login fails           | Bug        | Create task P1
 2 | agent-penf | Add dark mode              | Feature    | Create task P3
 3 | agent-test | Refactor complete          | Status     | Acknowledge
 4 | agent-penf | How does X work?           | Question   | Reply with answer

═══════════════════════════════════════════════════════════════
```

### Step 5: Detail Proposed Actions

For each actionable message, provide details:

```
───────────────────────────────────────────────────────────────
MESSAGE #1: Bug: login fails (pf-xxx)
───────────────────────────────────────────────────────────────
From: agent-penf
Category: Bug Report
Priority: P1 (blocks user workflow)

Summary: [1-2 sentence summary of the issue]

Proposed Action:
  → Create task: "fix: login authentication failure"
  → Priority: 1
  → Link to message with discovered-from edge
  → Reply: "Created task pf-XXXX, investigating."

───────────────────────────────────────────────────────────────
```

### Step 6: Ask for Approval

Use AskUserQuestion to get approval:

**Question:** "How should I proceed with these messages?"

**Options:**
1. **Process all** - Create tasks and send replies
2. **Process and execute** - Create tasks, spawn agents, execute, reply when done
3. **Review one by one** - Confirm each action individually
4. **Skip for now** - Don't take any actions

### Step 7: Execute Based on Choice

---

## Option A: Process All (Create tasks, reply, no execution)

For each actionable message:

1. **Mark as read**:
```sql
SELECT mark_read(ARRAY['<MESSAGE_ID>'], '<AGENT_NAME>');
```

2. **Create task** using helper:
```sql
SELECT create_task_from(
  'penfold',
  '<AGENT_NAME>',
  '<MESSAGE_ID>',
  '<type>: <title>',
  '<description from message>',
  <priority>,
  '<AGENT_NAME>'
);
```

3. **Reply to sender**:
```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<SENDER>'],
  'Re: <SUBJECT>',
  $body$
{
  "poll_hint": "done",
  "type": "ack"
}

## Acknowledged

<summary>

### Task Created
| Task | Title | Priority |
|------|-------|----------|
| **pf-XXXX** | <title> | P<n> |

Will update you when resolved.

-- <AGENT_NAME>
$body$,
  NULL,
  'ack',
  '<MESSAGE_ID>'
);
```

---

## Option B: Process and Execute (Full automation)

For each actionable message:

1. **Create task** (same as Option A)

2. **Determine agent** based on domain:

   | Domain | Agent | Signs |
   |--------|-------|-------|
   | CLI commands, help text | `cli-dev` | `cmd/penf/`, CLI errors |
   | Database, migrations | `data-dev` | Schema, SQL, `pkg/` repos |
   | Search, embeddings, LLM | `ai-dev` | AI/ML, embeddings |
   | Temporal workflows | `worker-dev` | Background jobs, workflows |
   | Gmail connector, OAuth | `gmail-dev` | Email sync |
   | Gateway services | `gateway-dev` | gRPC, `services/gateway/` |
   | Complex investigation | `debugger` | Unknown cause, >30 min |

3. **Assign task**:
```sql
UPDATE shards SET owner = '<agent-name>' WHERE id = '<TASK_ID>';
```

4. **Reply with assignment**:
```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<SENDER>'],
  'Re: <SUBJECT>',
  $body$
{
  "poll_hint": "continue",
  "type": "ack"
}

## In Progress

Created task **pf-XXXX** and assigned to **<agent>** agent.

Will update you when complete.

-- <AGENT_NAME>
$body$,
  NULL,
  'ack',
  '<MESSAGE_ID>'
);
```

5. **Spawn agent** using Task tool:
```
Task(
  subagent_type: "<agent-type>",
  prompt: "You are the <agent-name> agent.

  Work on task pf-XXXX: <title>

  Context from reporter:
  <paste message body>

  Requirements:
  1. Investigate and fix the issue
  2. Test your changes
  3. Close the task when done
  4. Return a summary of what you did

  Close task when done:
  UPDATE shards SET status = 'closed', closed_at = NOW(),
    closed_reason = 'Done: <summary>' WHERE id = 'pf-XXXX';",
  description: "<agent>: pf-XXXX"
)
```

6. **After agent completes**, reply to reporter:
```sql
SELECT send_message(
  'penfold',
  '<AGENT_NAME>',
  ARRAY['<SENDER>'],
  'Resolved: <SUBJECT>',
  $body$
{
  "poll_hint": "done",
  "type": "resolution"
}

## Resolved: <title>

### What Was Done
<agent summary>

### Verification
<if needed: command to verify>

### Task
**pf-XXXX** - Closed

-- <AGENT_NAME>
$body$,
  NULL,
  'resolution',
  '<MESSAGE_ID>'
);

-- Link bug to fix
SELECT link('<MESSAGE_ID>', '<TASK_ID>', 'fixed-by');
```

---

## Task Creation Reference

### Using create_task_from()

```sql
SELECT create_task_from(
  'penfold',           -- project
  'agent-mycroft',     -- creator
  'pf-xxx',            -- source message ID
  'fix: <title>',      -- task title
  '<description>',     -- task description
  1,                   -- priority (0-3)
  'agent-mycroft'      -- owner (optional)
);
```

This automatically:
- Creates task shard
- Adds `discovered-from` edge to source
- Copies relevant labels from source

### Priority Mapping

| Severity | Priority | Meaning |
|----------|----------|---------|
| blocking | 0 | Drop everything |
| high | 1 | Do today |
| medium | 2 | This week |
| low | 3 | Backlog |

### Type Prefixes

| Type | Usage |
|------|-------|
| `fix:` | Bug fixes |
| `feat:` | New features |
| `refactor:` | Code improvements |
| `docs:` | Documentation |
| `chore:` | Maintenance tasks |

---

## Final Summary

After processing, show what was done:

```
═══════════════════════════════════════════════════════════════
 MAIL PROCESSING COMPLETE
═══════════════════════════════════════════════════════════════

 Processed: X messages
 Tasks created: Y
 Agents spawned: Z (if execute mode)
 Replies sent: W

 New Tasks:
   pf-XXXX  fix: <title>                              P1  [cli-dev]
   pf-YYYY  feat: <title>                             P2  [data-dev]

 Agent Results: (if execute mode)
   ✓ cli-dev completed pf-XXXX - verification: run `penf version`
   ✓ data-dev completed pf-YYYY - no action needed

═══════════════════════════════════════════════════════════════
```

---

## Notes

- **Use Context-Palace SQL**, not agent-mail MCP functions
- **Parse JSON frontmatter** to get message type and severity
- **Thread awareness**: Use `get_thread('pf-xxx')` to see conversation context
- **Don't duplicate**: Search before creating: `SELECT * FROM shards WHERE project = 'penfold' AND type = 'task' AND title ILIKE '%keyword%';`
- **Mark as read**: Use `mark_read()` after processing each message
- **Always link**: Use `discovered-from` edge between tasks and source messages
