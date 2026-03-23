#!/bin/bash
# Mycroft SessionStart hook — injects dynamic context (work queue, playbook)
# Static context is loaded from .cxp/context/ files via pipeline config.
# stdout becomes additionalContext in Claude Code.

set -euo pipefail

# If dispatched by the pipeline, skip — dispatch context is in CLAUDE.md
if [ "${CXP_DISPATCH:-}" = "true" ]; then
  echo "[Dispatched session — context provided via worktree CLAUDE.md]"
  exit 0
fi

PROJ_DIR="$HOME/.claude/projects/-Users-james-github-otherjamesbrown-penfold"

# Instance identity
INSTANCE_ID=$(basename "$(ls -t "$PROJ_DIR"/*.jsonl 2>/dev/null | head -1)" .jsonl 2>/dev/null | cut -c1-8)
echo "[Instance: mycroft:${INSTANCE_ID:-unknown}]"

# Work queue (dynamic — changes every session)
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

# In-progress work (dynamic)
IN_PROGRESS=$(cxp shard list --type bug,task,spec --status in_progress --assigned-to agent-mycroft --limit 10 2>/dev/null || true)
if [ -n "$IN_PROGRESS" ] && ! echo "$IN_PROGRESS" | grep -q "No shards found"; then
  echo ""
  echo "# In Progress #"
  echo ""
  echo "$IN_PROGRESS"
fi

# Blocked items (dynamic)
BLOCKED=$(cxp shard list --label blocked --limit 10 2>/dev/null || true)
if [ -n "$BLOCKED" ] && ! echo "$BLOCKED" | grep -q "No shards found"; then
  echo ""
  echo "# Blocked (needs guidance) #"
  echo ""
  echo "$BLOCKED"
fi

# Standing instructions (dynamic)
INSTRUCTIONS=$(cxp memory search "standing instruction for mycroft" --limit 5 2>/dev/null || true)
if [ -n "$INSTRUCTIONS" ]; then
  echo ""
  echo "# Standing Instructions #"
  echo ""
  echo "$INSTRUCTIONS"
fi
