# Session End

Consolidate today's **Penfold development work** into a handoff for future session-start.

## Arguments: $ARGUMENTS

Optional: Brief note (e.g., "end of day", "switching projects")

## Instructions

### Step 1: Load Current Session

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, content, created_at FROM current_session('penfold', 'YOUR_AGENT_ID');"
```

### Step 2: Check Current Penfold State

Capture where things are now:

```bash
penf status
penf content stats
penf health
```

### Step 3: Create Handoff (PENFOLD-FOCUSED)

The handoff should capture **what we did with Penfold**, not Context-Palace activity.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_shard('penfold',
  'Handoff: BRIEF_PENFOLD_TOPIC - DATE',
  $md$
## Today (DAY, DATE)

**Working on:** What Penfold feature/issue

**What we did:**
- Tested X command - working/broken
- Built Y feature
- Debugged Z issue

**Current Penfold state:**
- Content: X items, Y% with embeddings
- Pipeline: working/broken/partial
- Key issue: description

## Earlier This Week

**DAYNAME:** Brief note of what we worked on
**DAYNAME:** Brief note (so James can go back if needed)

## Blocked/Waiting

- Issue X: waiting on mycroft to deploy enrichment service
- Issue Y: needs investigation

## What's Next

When James runs /session-start:
1. First priority
2. Second if time
$md$,
  'task',
  'YOUR_AGENT_ID');
EOSQL
```

### Step 4: Close Session

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT end_session('pf-SESSION-ID', 'Ended: BRIEF_SUMMARY');"
```

### Step 5: Close Previous Handoff

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
UPDATE shards SET status = 'closed', closed_at = NOW(), closed_reason = 'Superseded by pf-NEW'
WHERE id = 'pf-OLD-HANDOFF';"
```

### Step 6: Git Reminder

```
Session ended. Handoff: pf-xxx

Before closing:
  git status && git add <files> && git commit -m "..." && git push

Tomorrow: /session-start
```

## Key Principles

- **Penfold is the subject** - What we built, tested, debugged in the actual product
- **Include actual state** - Run `penf` commands and capture output
- **Time-frame for context** - Dates help James reconstruct the timeline
- **Track paused work** - Note what we set aside, not just current task
- **Context-Palace is just the envelope** - The handoff is stored there, but it's about Penfold
