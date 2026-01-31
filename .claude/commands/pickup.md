# Pickup

Resume work from the last checkpoint after a context clear or at session start.

## Arguments: $ARGUMENTS

Optional: Session ID to resume (defaults to most recent active session)

## Instructions

### Step 1: Load Session State

Run the resume command:

```bash
penf session resume
```

This displays:
- Session title and start time
- All checkpoints with timestamps
- Current session state

### Step 2: Parse Context

From the session output, identify:

1. **Original goal** - What the session was working toward
2. **Latest checkpoint** - Most recent state
3. **Next steps** - What was planned next
4. **Files/shards involved** - Context references

### Step 3: Check Related Work

If the session references shards or files, check their current state:

```bash
# If shards mentioned
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SELECT id, title, status FROM shards WHERE id IN ('pf-xxx');"

# If files mentioned
git status
git log --oneline -3
```

### Step 4: Output Summary

```
Session Resumed: <session-title>
Started: <start-time>
Last Checkpoint: <checkpoint-summary>

Continuing from: <last checkpoint description>

Next Steps:
1. <action from checkpoint>
2. <action from checkpoint>

Ready to continue. What would you like to work on?
```

### Step 5: If No Active Session

If no active session found:

```
No active session found.

Recent sessions:
  penf session history

To start a new session:
  penf session start "description"
```

## Notes

- Resume loads context without modifying the session
- Use /handoff to save progress before context clears
- Use /session-end for more detailed session handoffs
