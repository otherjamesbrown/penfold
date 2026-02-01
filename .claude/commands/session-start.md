# Session Start

Welcome James back. Help him understand where we are with **Penfold development**.

## Instructions

### Step 1: Get My Identity

Read `My Identity` from CLAUDE.md to get your agent ID (e.g., `agent-penfold`).

### Step 2: Load Recent Handoffs

Get handoffs from the last 7 days:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, created_at
FROM shards
WHERE project = 'penfold'
  AND creator = 'YOUR_AGENT_ID'
  AND title LIKE 'Handoff:%'
  AND created_at > NOW() - INTERVAL '7 days'
ORDER BY created_at DESC
LIMIT 5;"
```

Read them to understand the timeline:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SELECT content FROM shards WHERE id = 'pf-xxx';"
```

### Step 3: Check Current Penfold State

Get a snapshot of where things actually are:

```bash
penf status           # Connection/health
penf content stats    # Content pipeline state
penf health           # System health
```

### Step 4: Start a Session

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT start_session('penfold', 'YOUR_AGENT_ID', 'Session: DATE');"
```

### Step 5: Summarize for James (TIME-FRAMED, PENFOLD-FOCUSED)

**Focus on Penfold - what we're building, not dev tooling.**

Good example:
> "On Friday we got the content pipeline working - emails now process through to COMPLETED state. But enrichment (embeddings, entity extraction) still isn't running.
>
> On Saturday we ingested 19 test emails and confirmed the issue - 0 embeddings generated. I mailed mycroft about it.
>
> Current state: 19 emails in Penfold, all COMPLETED, but no embeddings. The enrichment service might need deployment or config.
>
> Earlier in the week we were working on the `penf content delete` command - that's working now."

Bad example:
> "There are 3 open shards in Context-Palace. You have 2 messages from mycroft. The session pf-xxx was created on Friday..."

**Key elements:**
- What Penfold features we built/tested
- Current state of content, pipeline, entities
- What's working vs broken
- What we were working on before (in case James wants to return to it)

### Step 6: Offer Options

```
Want me to:
1. Check if the enrichment issue is resolved?
2. Continue testing the pipeline?
3. Work on something else?
```

## Key Principles

- **Penfold is the focus** - Content, entities, pipeline, CLI commands
- **Context-Palace is just storage** - Don't report on shards/messages unless directly relevant
- **Time-frame everything** - "On Friday...", "Earlier this week..."
- **Show current state** - Run `penf` commands to see where things actually are
