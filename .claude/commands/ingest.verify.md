---
description: "Phase 5: Verify builds and tests, cross-check decomposed features, reply to penfold with resolution."
---

# Ingest — Phase 5: Verify & Reply

Verify all implementations, cross-check decomposed features, trace back to original
messages, and send resolution replies to penfold.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## Step 1: Verify Build and Tests

**For all items** (both single-agent and decomposed):
```bash
go build ./cmd/penf/...
go build ./services/gateway/...
go build ./services/worker/...
go test ./path/to/changed/... -v
```

**Additional check for decomposed (HIGH) features:**

After all sub-shards for a feature complete, run a cross-layer integration check:
```bash
# Build everything touched by this feature
go build ./pkg/[package]/... ./services/gateway/... ./cmd/penf/...

# Run all tests for all packages this feature touched
go test ./pkg/[package]/... ./services/gateway/[service]service/... -v

# Check for conflicts in shared packages
go vet ./cmd/penf/cmd/...
```

If cross-layer issues found (type mismatches, duplicate symbols):
1. Identify which sub-shard introduced the conflict
2. Fix small issues directly (missing import, wrong type name)
3. For larger issues, re-launch the specific layer agent

After cross-layer verification passes, close the parent impl shard:
```bash
/Users/dev/bin/palace task close pf-PARENT-SHARD 'All layers complete and verified. Sub-shards: [list]. All builds pass, all tests pass.'
```

**Test coverage check:**
```bash
git diff --name-only | grep "_test.go"
```

If tests are missing, re-launch the agent with explicit test requirements.
If build fails, fix small issues directly or re-launch agent.

## Step 2: Trace Back to Original Message

Follow edges from impl shard → investigation/analysis → original message:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
WITH source AS (
  SELECT e.to_id FROM edges e
  WHERE e.from_id = 'pf-fix-xxx'
  AND e.edge_type IN ('relates-to', 'discovered-from')
), msg AS (
  SELECT e.to_id, s.title, s.creator FROM edges e
  JOIN shards s ON s.id = e.to_id
  JOIN source ON source.to_id = e.from_id
  WHERE e.edge_type = 'discovered-from'
  AND s.type = 'message'
)
SELECT * FROM msg;
"
```

## Step 3: Reply to Penfold

**Group replies by original message.** If one message contained 3 items, send ONE reply.

Wait until ALL items from a message are resolved before replying.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft',
  ARRAY['agent-penfold'],
  'Resolved: [original message subject]',
  \$body\${\"poll_hint\":\"done\",\"type\":\"resolution\"}

## Resolved: [original message subject]

### Bugs Fixed

**1. [bug title]**
[summary from implementation agent]
- Investigation: pf-inv-aaa (closed)
- Fix: pf-fix-aaa (closed)

### Requirements Implemented

**2. [requirement title]**
[summary from implementation agent]
- Analysis: pf-anl-bbb (closed)
- Implementation: pf-feat-bbb (closed)

### Verification
- Build: passing
- Tests: [TestName1] FAILED before fix, PASSED after fix
- Tests: [TestName2] (acceptance) PASSED after implementation

### Deployment
[List what was deployed: gateway, worker, CLI vX.Y.Z, etc.]

### Action Required
[If CLI was updated: \"Please run penf update to get the changes.\"]
[If server-side only: \"No action required — changes are live.\"]

-- agent-mycroft
\$body\$,
  NULL, 'resolution', 'pf-ORIGINAL-MESSAGE-ID');
"
```

If a message contained only bugs or only requirements, omit the empty section header.

## Step 4: Close Remaining Shards

If any investigation or analysis shards are still open, close them:

```bash
/Users/dev/bin/palace task close pf-inv-xxx "Investigation complete, fix deployed"
/Users/dev/bin/palace task close pf-anl-xxx "Analysis complete, feature deployed"
```

## Show Progress

```
INGEST PIPELINE - Phase 5: Verify & Reply
══════════════════════════════════════════
BUILD:     All packages compile ✓
TESTS:     N test files, all passing ✓
CROSS-CHECK: [N decomposed features verified across layers ✓]

REPLIES:
  pf-msg-aaa: Resolved (3 items: 2 bugs, 1 feature) → agent-penfold ✓
  pf-msg-bbb: Resolved (1 item: 1 feature) → agent-penfold ✓
```

After displaying progress, return to the orchestrator for deploy.
