---
description: "Phase 1: Pull unread messages from inbox, classify as bug/requirement/spec/skip, create shards, acknowledge."
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
     isn't broken, it's missing. Loosely described, needs analysis.
   - **SPEC** — A fully analyzed requirement with structured detail. The sender has already
     done the analysis work. Contains acceptance criteria, file lists, data models, SQL,
     or layer breakdowns. Does NOT need exploration — skip straight to triage/decomposition.
   - **SKIP** — Questions, status updates, acks, or items that don't need implementation.

2. **Split multi-item messages.** A single message may contain bugs AND requirements AND
   specs, or multiple of each. Each discrete item gets its own shard.

**Classification examples:**
- "review queue fails with timeout" → **BUG** (something broke)
- "reprocess --all not implemented" → **REQUIREMENT** (feature doesn't exist, loosely described)
- "reprocess returns empty job ID" → **BUG** (unexpected behavior)
- "add a --format json flag to penf status" → **REQUIREMENT** (new feature, simple)
- "glossary export should support CSV" → **REQUIREMENT** (new capability)
- "glossary list crashes when DB is empty" → **BUG** (crash/error)
- Message with `## Goal`, `## Acceptance Criteria`, `## Files`, `## Data Model` → **SPEC**
- Message labeled `kind:requirement` with SQL, proto definitions, layer breakdown → **SPEC**
- Message with just "add entity filtering" and no structure → **REQUIREMENT**

**SPEC detection criteria** — classify as SPEC if the message contains **3 or more** of:
- Structured sections (## Goal, ## Scope, ## Acceptance Criteria, ## Files)
- Specific file paths to create or modify
- Data model or schema definitions (SQL, proto, table structures)
- Layer identification (db, service, cli, pipeline)
- Explicit acceptance criteria with testable conditions
- Existing pattern references (e.g., "follow the glossary command pattern")

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

**For SPECS:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-MESSAGE-ID',
  'spec: [specific feature title]',
  \$body\$Pre-analyzed spec from [creator]. Skipping investigation/analysis — proceed directly to triage.

## Original Spec Content
[paste the FULL spec content here — this IS the analysis]
\$body\$,
  1, 'agent-mycroft');
"
```

Note the shard title prefix:
- `investigate:` for bugs → Phase 2 launches debugger
- `analyze:` for requirements → Phase 2 launches explorer
- `spec:` for specs → **Phase 2 is skipped**, goes directly to Phase 3 triage

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
3. [SPEC] [feature title] — skipping analysis, using your spec directly
4. [BUG] [issue title]

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
Messages: N (containing M discrete items: B bugs, R requirements, S specs)

Message pf-xxxxxx: "[title]" (from agent-penfold)
  #1 | BUG  | pf-inv-aaa | [issue title]
  #2 | REQ  | pf-anl-bbb | [requirement title]
  #3 | SPEC | pf-spc-ccc | [feature title] — analysis skipped

Message pf-yyyyyy: "[title]" (from agent-penfold)
  #4 | REQ  | pf-anl-ddd | [requirement title]

Acks sent: N messages acknowledged
Shards created: M (B investigations, R analyses, S specs)
SPECs skip Phase 2 — proceeding directly to triage for those items.
```

After displaying progress, return to the orchestrator. It will invoke the next phase.
