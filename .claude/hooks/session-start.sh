#!/bin/bash
# Mycroft SessionStart hook — injects instance identity and work queue
# stdout becomes additionalContext in Claude Code.

set -euo pipefail

PROJ_DIR="$HOME/.claude/projects/-Users-james-github-otherjamesbrown-penfold"

# 1. Instance identity (first 8 chars of conversation UUID)
INSTANCE_ID=$(basename "$(ls -t "$PROJ_DIR"/*.jsonl 2>/dev/null | head -1)" .jsonl 2>/dev/null | cut -c1-8)
echo "[Instance: mycroft:${INSTANCE_ID:-unknown}]"

# 2. Work queue — shards marked ready for implementation
READY_JSON=$(cxp shard list --type bug,task,spec --assigned-to agent-mycroft --limit 20 -o json 2>/dev/null || echo '{"results":[]}')
READY_COUNT=$(echo "$READY_JSON" | jq '.results | length' 2>/dev/null || echo 0)

BUGS=$(echo "$READY_JSON" | jq '[.results[] | select(.type == "bug")] | length' 2>/dev/null || echo 0)
TASKS=$(echo "$READY_JSON" | jq '[.results[] | select(.type == "task")] | length' 2>/dev/null || echo 0)
SPECS=$(echo "$READY_JSON" | jq '[.results[] | select(.type == "spec")] | length' 2>/dev/null || echo 0)

echo ""
echo "# Work Queue ($READY_COUNT items: $BUGS bugs, $TASKS tasks, $SPECS specs) #"
echo ""
if [ "$READY_COUNT" -gt 0 ]; then
  echo "$READY_JSON" | jq -r '.results[] | "\(.id)  \(.type)  \(.title)"' 2>/dev/null || true
else
  echo "No items ready."
fi

# 3. In-progress work (from any previous session)
IN_PROGRESS=$(cxp shard list --type bug,task,spec --status in_progress --assigned-to agent-mycroft --limit 10 2>/dev/null || true)
if [ -n "$IN_PROGRESS" ] && ! echo "$IN_PROGRESS" | grep -q "No shards found"; then
  echo ""
  echo "# In Progress #"
  echo ""
  echo "$IN_PROGRESS"
fi

# 4. Blocked items needing attention
BLOCKED=$(cxp shard list --label blocked --limit 10 2>/dev/null || true)
if [ -n "$BLOCKED" ] && ! echo "$BLOCKED" | grep -q "No shards found"; then
  echo ""
  echo "# Blocked (needs guidance) #"
  echo ""
  echo "$BLOCKED"
fi

# 5. Playbook
echo ""
echo "# Playbook #"
echo ""
cxp knowledge show pf-2b76b4 2>/dev/null || true

# 6. Standing instructions
INSTRUCTIONS=$(cxp memory search "standing instruction for mycroft" --limit 5 2>/dev/null || true)
if [ -n "$INSTRUCTIONS" ]; then
  echo ""
  echo "# Standing Instructions #"
  echo ""
  echo "$INSTRUCTIONS"
fi

# 7. Presentation instructions for the agent
cat <<'INST'

# Startup Instructions

Present the work queue above to James as a table:

| # | ID | Type | Title |
|---|-----|------|-------|
| 1 | pf-xxx | bug | ... |

Then ask what he wants to work on:

1. All items
2. Bugs only (list IDs)
3. Tasks only (list IDs)
4. Pick specific items

When James chooses, run /ingest.run with the selected shard IDs.
Example: if James picks "2" and bugs are pf-xxx and pf-yyy, run /ingest.run pf-xxx pf-yyy

If there are in-progress items from a previous session, mention them first and ask
whether to resume or start fresh work.

If there are blocked items, flag them prominently — these need James's input.

Do NOT check the inbox — context is injected by this hook.
INST
