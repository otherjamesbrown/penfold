# Context-Palace

A shared memory system for AI agents. Tasks, messages, logs, and data - all in one place.

---

## What is Context-Palace?

Context-Palace replaces separate mail and task tracking systems with one unified database. Everything is a **shard** - tasks, messages, logs, configs, notes. Shards connect to each other via **edges** (relationships).

```
┌─────────────────────────────────────────────┐
│              Context-Palace                 │
│                                             │
│  message ────discovered-from────► task      │
│     │                              │        │
│     └─────── replies-to ───────────┘        │
│                                             │
│  All shards. Same API. Same queries.        │
└─────────────────────────────────────────────┘
```

**Use it for:**
- Tasks and bugs
- Messaging between agents
- Logging actions
- Storing configs and data
- Anything you need to persist or share

---

## Agent Identity

### Your Name

You are **agent-penfdev** - use this consistently in all queries.

Format: `agent-{name}` (e.g., `agent-cli`, `agent-backend`, `agent-rusticdesert`)

### Your Project

You work on project **penfold** with ID prefix **pf-**.

Always include your project in queries to avoid mixing data with other projects.

---

## Connection

SSL certificates must be installed in `~/.postgresql/` (see secrets repo).

```bash
# Interactive
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"

# Single command
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

No password - SSL certificate provides authentication.

---

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Shard** | Everything is a shard - tasks, messages, logs, configs |
| **Project** | Namespace separating data between projects (each has unique ID prefix) |
| **Edge** | Relationship between shards (blocks, replies-to, relates-to) |
| **Label** | Tags on shards (recipients, categories) |

### Projects and ID Prefixes

Each project has a unique ID prefix. Your project is **penfold** with prefix **pf-**.

| Project | Prefix | Example ID |
|---------|--------|------------|
| penfold | `pf` | `pf-a1b2c3` |
| context-palace | `cp` | `cp-d4e5f6` |

Register new projects:
```sql
INSERT INTO projects (name, prefix) VALUES ('my-project', 'mp');
```

### Shard Types

| Type | Purpose |
|------|---------|
| `task` | Work to be done |
| `message` | Communication between agents/humans |
| `log` | Activity record |
| `config` | Configuration data |
| `doc` | Documentation, notes |

### Statuses

| Status | Meaning |
|--------|---------|
| `open` | Not started |
| `in_progress` | Being worked on |
| `closed` | Done |

### Priorities

| Priority | Meaning |
|----------|---------|
| 0 | Critical - drop everything |
| 1 | High - do today |
| 2 | Normal - this week |
| 3 | Low - when possible |

---

## Helper Functions

Use these instead of writing complex SQL:

| Function | Purpose | Returns |
|----------|---------|---------|
| `unread_for(project, agent)` | Your unread messages | id, title, creator, kind, created_at |
| `tasks_for(project, agent)` | Your assigned open tasks | id, title, priority, status, created_at |
| `ready_tasks(project)` | Open tasks not blocked | id, title, priority, owner, created_at |
| `get_thread(shard_id)` | Conversation thread | id, title, creator, content, depth, created_at |
| `create_shard(...)` | Create a new shard | The new shard ID |

---

## Common Operations

### Check Your Inbox

```sql
SELECT * FROM unread_for('penfold', 'agent-penfdev');
```

### Mark Message as Read

```sql
INSERT INTO read_receipts (shard_id, agent_id) VALUES ('pf-xxx', 'agent-penfdev') ON CONFLICT DO NOTHING;
```

### Read Full Message

```sql
SELECT * FROM shards WHERE id = 'pf-xxx';
```

### Get Your Tasks

```sql
SELECT * FROM tasks_for('penfold', 'agent-penfdev');
```

### Get Ready Tasks (Claimable)

```sql
SELECT * FROM ready_tasks('penfold');
```

### Send a Message

```sql
-- Create message (returns new ID like pf-a1b2c3)
SELECT create_shard('penfold', 'Subject', 'Body text', 'message', 'agent-penfdev');

-- Add recipient (use the returned ID)
INSERT INTO labels (shard_id, label) VALUES ('pf-NEWID', 'to:recipient-agent');

-- Add kind (optional)
INSERT INTO labels (shard_id, label) VALUES ('pf-NEWID', 'kind:status-update');
```

### Reply to a Message

```sql
-- Create reply (returns new ID)
SELECT create_shard('penfold', 'Re: Subject', 'Reply text', 'message', 'agent-penfdev');

-- Link to original
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-REPLY', 'pf-ORIGINAL', 'replies-to');

-- Notify sender
INSERT INTO labels (shard_id, label) VALUES ('pf-REPLY', 'to:original-sender');
```

### Get Conversation Thread

```sql
SELECT * FROM get_thread('pf-ROOT-MESSAGE');
```

### Create a Task

```sql
-- Simple (returns ID like pf-a1b2c3)
SELECT create_shard('penfold', 'Task title', 'Description here', 'task', 'agent-penfdev');

-- With owner and priority
SELECT create_shard('penfold', 'Task title', 'Description', 'task', 'agent-penfdev', 'target-agent', 2);

-- Or full control with manual INSERT
INSERT INTO shards (id, project, title, content, type, status, creator, owner, priority)
VALUES (
  gen_shard_id('penfold'),
  'penfold',
  'Task title',
  '## Description\nWhat needs doing\n\n## Acceptance Criteria\n- Done when X',
  'task',
  'open',
  'agent-penfdev',
  'target-agent',
  2
)
RETURNING id;
```

### Claim a Task

```sql
UPDATE shards
SET owner = 'agent-penfdev', status = 'in_progress'
WHERE id = 'pf-xxx' AND (owner IS NULL);
```

### Complete a Task

```sql
UPDATE shards
SET status = 'closed', closed_at = NOW(), closed_reason = 'Completed: summary'
WHERE id = 'pf-xxx';
```

### Create Task from Bug Report

```sql
-- Create task
SELECT create_shard('penfold', 'fix: Bug title', 'Details', 'task', 'agent-penfdev');

-- Link to source message (use returned IDs)
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-MESSAGE', 'pf-NEWTASK', 'discovered-from');

-- Close the message
UPDATE shards SET status = 'closed' WHERE id = 'pf-MESSAGE';
```

### Add Blocking Dependency

```sql
-- Task A is blocked by Task B
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-taskA', 'pf-taskB', 'blocks');
```

### Log an Action

```sql
SELECT create_shard('penfold', 'Did something', 'Details of action', 'log', 'agent-penfdev');
```

### Search

```sql
SELECT id, title, status
FROM shards, to_tsquery('english', 'oauth & error') query
WHERE project = 'penfold' AND search_vector @@ query
ORDER BY ts_rank(search_vector, query) DESC
LIMIT 10;
```

---

## Task Assignment

### Explicit Assignment

When you create a task and know who should do it:

```sql
SELECT create_shard('penfold', 'Title', 'Details', 'task', 'agent-penfdev', 'agent-backend', 2);
```

### Claim Model

When anyone can take a task, leave `owner = NULL`. Agents claim from ready tasks:

```sql
-- Find claimable tasks
SELECT * FROM ready_tasks('penfold') WHERE owner IS NULL;

-- Claim one
UPDATE shards SET owner = 'agent-penfdev', status = 'in_progress' WHERE id = 'pf-xxx' AND owner IS NULL;
```

### Label Routing

Use labels to indicate what kind of agent should take it:

```sql
INSERT INTO labels (shard_id, label) VALUES ('pf-xxx', 'for:backend');
```

Agents filter by their specialty:

```sql
SELECT s.* FROM ready_tasks('penfold') s
JOIN labels l ON l.shard_id = s.id
WHERE l.label = 'for:backend';
```

---

## Labels Reference

### Recipients (for messages)
- `to:agent-backend` - Send to specific agent
- `to:human-james` - Send to human

### Message Kinds
- `kind:bug-report` - Bug, needs triage
- `kind:feature-request` - Feature request
- `kind:status-update` - FYI, progress report
- `kind:question` - Needs response
- `kind:completion` - Work done notification

### Task Routing
- `for:backend` - Backend agent should take this
- `for:frontend` - Frontend agent should take this
- `for:any` - Anyone can take this

### Task Labels
- `backend`, `frontend`, `database`, `infra` - Component
- `urgent`, `blocked` - Status hints

---

## Edge Types

| Edge | Meaning | Direction |
|------|---------|-----------|
| `replies-to` | Message reply | Reply → Original |
| `relates-to` | Loose association | Any → Any |
| `discovered-from` | Created from source | New → Source |
| `blocks` | Dependency | Blocked → Blocker |

---

## Session Workflow

```
1. CHECK INBOX
   SELECT * FROM unread_for('penfold', 'agent-penfdev');

2. PROCESS MESSAGES
   - Read each message
   - Mark as read
   - Create tasks if needed
   - Reply if needed

3. CHECK TASKS
   SELECT * FROM tasks_for('penfold', 'agent-penfdev');

4. CLAIM OR WORK
   - Claim an unowned task, or
   - Work on assigned task

5. COMPLETE
   - Close task with reason
   - Send status update if needed

6. REPEAT
```

---

## Database Schema

### shards
```
id, project, title, content, type, status, priority, creator, owner, parent_id,
created_at, updated_at, closed_at, closed_reason
```

### edges
```
from_id, to_id, edge_type, created_at, metadata
```

### labels
```
shard_id, label
```

### read_receipts
```
shard_id, agent_id, read_at
```

---

## Connection Details

| Property | Value |
|----------|-------|
| Host | dev02.brown.chat |
| Port | 5432 |
| Database | contextpalace |
| User | penfold |
| Auth | SSL client certificates |
| Certs | `~/.postgresql/` |

---

## Quick Reference

```sql
-- Inbox
SELECT * FROM unread_for('penfold', 'agent-penfdev');

-- Mark read
INSERT INTO read_receipts (shard_id, agent_id) VALUES ('pf-xxx', 'agent-penfdev') ON CONFLICT DO NOTHING;

-- My tasks
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- Ready tasks
SELECT * FROM ready_tasks('penfold');

-- Create task
SELECT create_shard('penfold', 'title', 'description', 'task', 'agent-penfdev');

-- Create task with owner/priority
SELECT create_shard('penfold', 'title', 'desc', 'task', 'agent-penfdev', 'owner-agent', 2);

-- Send message
SELECT create_shard('penfold', 'subject', 'body', 'message', 'agent-penfdev');
INSERT INTO labels (shard_id, label) VALUES ('pf-NEWID', 'to:recipient');

-- Claim task
UPDATE shards SET owner = 'agent-penfdev', status = 'in_progress' WHERE id = 'pf-xxx' AND owner IS NULL;

-- Close task
UPDATE shards SET status = 'closed', closed_at = NOW(), closed_reason = 'Done' WHERE id = 'pf-xxx';

-- Thread
SELECT * FROM get_thread('pf-root');

-- Register new project
INSERT INTO projects (name, prefix) VALUES ('my-project', 'mp');
```
