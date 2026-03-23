# Dispatched Task — Completion Instructions

You have been dispatched by the M Pipeline to implement a specific task. The task spec and design context are provided above.

## How to work

1. Read the task spec carefully — implement exactly what it says
2. Follow the acceptance criteria — every criterion should be testable
3. Use the code locations identified in the spec as your starting point

## On completion

Run **`cxp task complete <task-id>`** as your LAST action. This command:
- Commits any remaining changes
- Pushes the branch
- Creates a PR
- Appends evidence to the shard
- Marks the task `needs-review`

You do NOT need to manually create PRs, push branches, or update shard status. The `cxp task complete` command handles all of it.

## If you get stuck

- Append your findings to the shard: `cxp shard append <task-id> --body "stuck on: ..."`
- Label it blocked: `cxp shard label add <task-id> blocked`
- The health monitor will detect this and escalate
