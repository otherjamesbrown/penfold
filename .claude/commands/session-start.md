# Session Start

Pick up where we left off. Find what matters, summarize it, and get to work.

## Instructions

### Step 1: Find Recent Handoffs

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SELECT id, title, status, created_at FROM shards WHERE project = 'penfold' AND title LIKE '%Handoff%' AND status IN ('open', 'in_progress') ORDER BY created_at DESC LIMIT 3;"
```

If found, read the most recent one:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SELECT content FROM shards WHERE id = 'pf-xxx';"
```

### Step 2: Check for Waiting Messages

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SELECT id, title, creator, created_at FROM shards WHERE project = 'penfold' AND type = 'message' AND status = 'open' ORDER BY created_at DESC LIMIT 5;"
```

### Step 3: Summarize for James

**Don't regurgitate the handoff.** Parse it and tell James what he needs to know:

- **What were we doing?** (1 sentence)
- **Where did we leave off?** (the key blocker or next step)
- **What's changed since?** (any replies, deployments, new info)
- **What can we do now?** (actionable options)

Example good summary:
> "Last session we were cleaning up test data but hit a blocker - ContentProcessorService wasn't deployed. Agent-penfdev said they'd deploy it. We should check if that's done, then we can delete the test data."

Example bad summary:
> "Session Goal: Review feature requests. Progress Made: [list of 10 things]. Remaining Work: [list of 5 things]..."

**Be conversational.** James doesn't need a status report, he needs context to make a decision.

### Step 4: Offer Clear Options

End with something like:

> "Want me to:
> 1. Check if the deployment is done and continue?
> 2. Work on something else?
> 3. Show me more details about the handoff?"

### Step 5: Claim the Handoff

Once James decides to continue, mark it claimed:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SELECT claim_task('pf-xxx', 'agent-penf');"
```

## No Handoff Found?

If no handoff exists, just say:

> "No recent handoff found. What would you like to work on?"

Then check for open tasks or messages that might need attention.

## Key Principle

**Add value, don't just relay information.** You've read the handoff - now help James understand what it means and what to do about it.
