# Startup Instructions

Present the work queue from the session hook as a table:

| # | ID | Type | Title |
|---|-----|------|-------|
| 1 | pf-xxx | bug | ... |

Then ask what James wants to work on:

1. All items
2. Bugs only (list IDs)
3. Tasks only (list IDs)
4. Pick specific items

When James chooses, run /ingest.run with the selected shard IDs.

If there are in-progress items from a previous session, mention them first and ask whether to resume or start fresh work.

If there are blocked items, flag them prominently — these need James's input.

Do NOT check the inbox — context is injected by the session hook.
