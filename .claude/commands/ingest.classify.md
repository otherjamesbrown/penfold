---
description: "Phase 1: Pull unread messages from inbox, classify as bug/requirement/skip, create shards, acknowledge."
---

# Ingest — Phase 1: Pull & Classify

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

## Step 1: Fetch Unread Messages

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.content, s.creator, s.created_at
FROM unread_for('penfold', 'agent-mycroft') u
JOIN shards s ON s.id = u.id
WHERE s.type = 'message'
ORDER BY s.created_at;
"
```

## Step 2: Classify and Split Into Discrete Items

For each message:
1. Classify each discrete item as one of:
   - **BUG** — Something that used to work (or should work) but doesn't. Has a symptom,
     error, or unexpected behavior. Needs investigation to find root cause.
   - **REQUIREMENT** — A new capability, enhancement, or change request. Something that
     doesn't exist yet and needs to be built. No root cause to investigate — the code
     isn't broken, it's missing.
   - **SKIP** — Questions, status updates, acks, or items that don't need implementation.

2. **Split multi-item messages.** A single message may contain both bugs AND requirements,
   or multiple of each. Each discrete item gets its own shard.

**Classification examples:**
- "review queue fails with timeout" → **BUG** (something broke)
- "reprocess --all not implemented" → **REQUIREMENT** (feature doesn't exist)
- "reprocess returns empty job ID" → **BUG** (unexpected behavior)
- "add a --format json flag to penf status" → **REQUIREMENT** (new feature)
- "glossary export should support CSV" → **REQUIREMENT** (new capability)
- "glossary list crashes when DB is empty" → **BUG** (crash/error)

**How to identify discrete items:**
- Different symptoms or different features requested
- Different components (gateway RPC vs CLI flag vs API behavior)
- Could be fixed/built independently by different agents
- Bugs have root causes; requirements have acceptance criteria

## Step 3: Create Shards (One Per Item)

Create one shard per discrete item, NOT per message.
All shards from the same message link back to the same original message ID.

**For BUGS:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-MESSAGE-ID',
  'investigate: [specific bug title]',
  'Investigate bug reported by [creator]. [specific symptom from message]',
  1, 'agent-mycroft');
"
```

**For REQUIREMENTS:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-MESSAGE-ID',
  'analyze: [specific requirement title]',
  'Analyze requirement from [creator]. [what needs to be built]',
  1, 'agent-mycroft');
"
```

Note the shard title prefix: `investigate:` for bugs, `analyze:` for requirements. This
prefix is used in later phases to determine which path to follow.

## Step 4: Acknowledge to Penfold

Send **one ack per message** (not per item). List the items identified with their type:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Re: [original subject]',
  \$body\${\"poll_hint\":\"continue\",\"type\":\"ack\"}
Processing N items from your report:
1. [BUG] [issue title]
2. [REQ] [requirement title]
3. [BUG] [issue title]

Will update as each is resolved.\$body\$,
  NULL, 'ack', 'pf-MESSAGE-ID');
"
```

## Step 5: Mark Messages Read

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT mark_read(ARRAY['pf-msg1', 'pf-msg2'], 'agent-mycroft');
"
```

## Show Progress

```
INGEST PIPELINE - Phase 1: Pull & Classify
════════════════════════════════════════════
Messages: N (containing M discrete items: B bugs, R requirements)

Message pf-xxxxxx: "[title]" (from agent-penfold)
  #1 | BUG | pf-inv-aaa | [issue title]
  #2 | REQ | pf-anl-bbb | [requirement title]
  #3 | BUG | pf-inv-ccc | [issue title]

Message pf-yyyyyy: "[title]" (from agent-penfold)
  #4 | REQ | pf-anl-ddd | [requirement title]

Acks sent: N messages acknowledged
Shards created: M (B investigations, R analyses)
```

After displaying progress, return to the orchestrator. It will invoke the next phase.
