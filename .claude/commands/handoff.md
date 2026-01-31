# Handoff

Save a checkpoint of current work progress before context clears or when switching tasks.

## Arguments: $ARGUMENTS

Required: Brief description of current state (e.g., "Working on TLS bug fixes", "Debugging gateway connection")

## Instructions

### Step 1: Check Active Session

First check if there's an active session:

```bash
penf session context
```

If no active session exists, start one first:

```bash
penf session start "$(echo $ARGUMENTS | head -c 50)"
```

### Step 2: Gather Context

Before creating the checkpoint, summarize:

1. **What you were working on** - Current task/problem
2. **What was accomplished** - Steps completed, findings
3. **What's next** - Immediate next steps when resuming
4. **Key decisions** - Any important choices made

### Step 3: Create Checkpoint

Run the checkpoint command with the user's description plus your context:

```bash
penf session checkpoint "$ARGUMENTS

Working on: <task>
Completed: <steps>
Next: <immediate actions>
Files changed: <list>"
```

### Step 4: Confirm

Output:

```
Checkpoint saved.

Session: <session-id>
Checkpoint: $ARGUMENTS

To resume after context clear:
  /pickup

Or manually:
  penf session resume
```

## Notes

- Checkpoints append to the current session's content
- Multiple checkpoints can be made within one session
- Use /pickup to load the checkpoint after context clears
