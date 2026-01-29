# Finding Current Priorities

## Dynamic Priority Discovery

```bash
bd ready                # Current available work
bd stats                # Project health overview
bd list --status=open   # All open work
```

## Priority Guidelines

1. **Blocked work** - Unblock others first
2. **P0/P1 priorities** - Critical path items
3. **Complete epic chains** - Finish what's started
4. **Follow dependencies** - Use bead dependency chains

**When in doubt:** Run `bd ready` and ask user which direction they prefer.

## Cannot Start New Work If

- Any P0 exists (fix it first)
- You have ≥3 independent work streams in_progress (finish something first)
- A P1 has been open >7 days (address it)

## Before Starting Work

```bash
# 1. Check for blockers to new work
bd list --status open --priority 0    # Any P0? Fix first!
bd list --status open --priority 1    # P1s >7 days? Address first.
bd list --status in_progress          # Already ≥3? Finish something.

# 2. Find existing bead or create new one
bd ready                    # Find unblocked tasks
bd list --status open       # All open issues
bd create --title="..." --type=task

# 3. Claim the work
bd update <id> --status in_progress
```
