# Pickup

Resume after context clear. Load my working memory and continue with Penfold work.

## Arguments: $ARGUMENTS

Optional: Session ID (defaults to current)

## Instructions

### Step 1: Load Session

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, content FROM current_session('penfold', 'YOUR_AGENT_ID');"
```

### Step 2: Parse Latest Checkpoint

From the session content, find the most recent checkpoint:

1. **What we're working on** - Penfold feature/issue
2. **What's done** - Already completed
3. **Current state** - What's working/broken
4. **Next step** - My immediate action
5. **Key findings** - Context I need

### Step 3: Quick State Check

Verify Penfold state matches expectations:

```bash
penf status
penf content stats
```

### Step 4: Resume Quietly

**Don't explain to James.** He knows what we're doing.

Good:
> "Ready. Continuing with enrichment investigation."

Or:
> "Picked up. Where were we?"

Bad:
> "I've loaded the checkpoint. We were working on X. The state is Y. The next steps are Z..."

### Step 5: Flag Changes Only

Only speak up if something changed:

> "Picked up. Note: content stats changed - 3 more items now COMPLETED. Continue?"

## Key Principles

- **Silent load** - Don't narrate
- **Quick confirmation** - Just signal readiness
- **Check Penfold state** - Verify things match checkpoint
- **Flag actual changes** - In Penfold, not Context-Palace
