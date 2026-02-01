# Handoff

Save current Penfold work state before context clears. This is for ME to remember.

## Arguments: $ARGUMENTS

Optional: Brief note about current state

## Instructions

### Step 1: Get Current Session

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title FROM current_session('penfold', 'YOUR_AGENT_ID');"
```

If no session, create one:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT start_session('penfold', 'YOUR_AGENT_ID', 'Session: DATE');"
```

### Step 2: Capture Penfold State

What do I need to remember about the actual work?

1. **What Penfold feature/issue** we're working on
2. **What we tested/built** this cycle
3. **Current state** - what's working, what's broken
4. **Next step** - immediate action when resuming
5. **Key findings** - root causes, decisions

### Step 3: Add Checkpoint

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT add_checkpoint('pf-SESSION-ID', $md$
## Checkpoint: TIME

**Working on:** Penfold feature/issue
**Did:** What got done
**State:** What's working/broken
**Next:** Immediate next step
**Found:** Key discoveries
$md$);
EOSQL
```

### Step 4: Confirm

```
Checkpoint saved.

Context can be cleared. Use /pickup to resume.
```

## Key Principles

- **Quick and focused** - Working memory, not a report
- **Penfold work** - What we're building/testing, not Context-Palace state
- **For me, not James** - He knows what we're doing
- **Clear next step** - I should know exactly what to do after /pickup
