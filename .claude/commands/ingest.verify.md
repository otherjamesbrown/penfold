---
description: "Phase 5: Verify builds, unit tests, integration tests, cross-check decomposed features, send pre-deploy review to penfold."
---

# Ingest — Phase 5: Verify & Review

Verify all implementations, run integration tests, cross-check decomposed features,
trace back to original messages, and send pre-deploy review to penfold.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## Step 1: Verify Build and Unit Tests

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

## Step 2: Integration Tests

**Run integration tests against the actual codebase** — unit tests alone don't catch
wiring bugs. This was learned the hard way: unit tests pass but pipeline wiring mismatches
slip through.

```bash
# If integration test suite exists:
go test ./tests/integration/... -v -tags=integration

# If no integration test suite, run smoke checks:
go build ./...
go vet ./...
```

**For changes that affect the pipeline (worker, activities, workflows):**
```bash
# Build the worker to verify it compiles with all changes
go build ./services/worker/...

# Run worker-specific tests
go test ./services/worker/... -v
```

**For changes that affect gRPC services:**
```bash
# Verify proto generation is up to date
# (only if proto files were modified)
go build ./services/gateway/...
go test ./services/gateway/... -v
```

If integration tests fail:
1. Identify which component is broken
2. Fix the wiring issue directly if small
3. Re-launch the relevant agent if larger
4. Do NOT proceed to deploy until integration tests pass

## Step 2.5: Test Quality Review

**Tests existing is not enough. Tests must test the right things.**

For each implementation shard, review the test assertions:

```bash
# Show test functions and their assertions
grep -A 10 'func Test' path/to/test_file.go | grep -E '(func Test|assert\.|require\.)'
```

**Red flags — re-launch the agent if you see these:**
- Tests that only assert `assert.NoError` or `require.NoError` without checking return values
- Tests that assert `!= nil` without verifying specific fields
- Bug reproduction tests that don't exercise the specific failing path from the investigation
- Acceptance tests that don't map to the shard's acceptance criteria
- Tests with no edge cases (only happy path)

**For SPEC items specifically:** Cross-check that every acceptance criterion in the spec
has at least one test assertion. List the mapping:
```
Criterion 1: "X returns only Y" → TestX_ReturnsOnlyY (line 45)
Criterion 2: "Z with no data returns empty" → TestZ_EmptyResult (line 78)
Criterion 3: "Unknown input returns error" → ??? MISSING — flag for fix
```

## Step 2.6: Real-Data Verification (Pipeline/Data Changes Only)

**Skip this step if the changes don't affect the pipeline, AI processing, or data output.**

For bugs or features that change how content is processed, extracted, or stored:

1. **Identify affected content.** Find at least one content item that demonstrates the
   bug or that the feature should improve:
   ```bash
   penf content list --limit 10
   # Pick an item that was affected by the bug, or a representative item
   ```

2. **Capture before state.** Record the current output for comparison:
   ```bash
   penf content show pf-CONTENT-ID
   # For entity bugs: penf entity list | head -20
   # For assertion bugs: penf assertion list | head -20
   # For acronym bugs: penf glossary list | head -20
   ```

3. **Reprocess after deploy** (this runs AFTER Phase 6+7 deploy, but plan for it here):
   ```bash
   penf pipeline reprocess pf-CONTENT-ID
   # Wait for completion, then capture after state
   ```

4. **Include before/after in resolution.** The resolution message to penfold must show:
   - Before: [paste output from step 2]
   - After: [paste output from step 3]
   - What changed: [specific differences]

**If the output didn't change after reprocessing, the fix didn't work.** Investigate
whether the correct binary is running (ghost deploy) or whether the fix doesn't affect
the code path for this content.

**This step is non-negotiable for pipeline changes.** Penfold will reject resolutions
for pipeline bugs that don't include reprocessed output.

## Step 3: Trace Back to Original Message

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

## Step 4: Pre-Deploy Review to Penfold

**Before deploying**, send a review message to penfold with what was built. This is the
last checkpoint before changes go live.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Pre-deploy review: [N items ready]',
  \$body\${\"poll_hint\":\"review\",\"type\":\"progress\"}
## Pre-Deploy Review

All items built and verified. Ready to deploy.

### Bugs Fixed
1. **[bug title]** — [1-sentence fix summary]
   - Test: [TestName] FAILED before fix, PASSES after fix
   - Files: [modified files]

### Features Built
1. **[feature title]** — [1-sentence summary]
   - Tests: [TestName1, TestName2] PASS
   - Files: [modified/created files]
   - Acceptance criteria: [N/N met]

### Verification
- Build: all packages compile ✓
- Unit tests: all passing ✓
- Integration tests: [passing ✓ / not applicable / N failures]
- Cross-layer check: [N features verified ✓]

### What Will Be Deployed
- [Gateway: yes/no]
- [Worker: yes/no]
- [CLI: yes/no — version bump to vX.Y.Z]

Proceeding to deploy. Reply if you want to hold.

-- agent-mycroft
\$body\$,
  NULL, 'progress', NULL);
"
```

**Do NOT block on penfold's response.** Continue to deploy phase. But if penfold replies
with "hold" or corrections before deploy completes, pause and incorporate.

## Step 5: Send Resolution Replies

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

### Specs Implemented

**3. [spec title]**
[summary from implementation agent]
- Spec: pf-spc-ccc (closed)
- Implementation: pf-feat-ccc (closed)
- Acceptance criteria met: [N/N]

### Verification
- Build: passing
- Tests: [TestName1] FAILED before fix, PASSED after fix
- Tests: [TestName2] (acceptance) PASSED after implementation
- Integration: [passing / not applicable]
- Test quality: [criteria-to-test mapping verified / N/A]

### Real-Data Verification (pipeline changes only)
Content ID: pf-CONTENT-ID
Before (pre-fix output):
[paste actual output]
After (post-fix, reprocessed output):
[paste actual output]
What changed: [specific differences]

### Deployment
[List what was deployed: gateway, worker, CLI vX.Y.Z, etc.]
[Deployed version verified: commit [hash] running on [service]]

### Action Required
[If CLI was updated: \"Please run penf update to get the changes.\"]
[If server-side only: \"No action required — changes are live.\"]

-- agent-mycroft
\$body\$,
  NULL, 'resolution', 'pf-ORIGINAL-MESSAGE-ID');
"
```

If a message contained only bugs or only requirements, omit the empty section header.

## Step 6: Close Remaining Shards

If any investigation, analysis, or spec shards are still open, close them:

```bash
/Users/dev/bin/palace task close pf-inv-xxx "Investigation complete, fix deployed"
/Users/dev/bin/palace task close pf-anl-xxx "Analysis complete, feature deployed"
/Users/dev/bin/palace task close pf-spc-xxx "Spec implemented and deployed"
```

## Show Progress

```
INGEST PIPELINE - Phase 5: Verify & Review
═══════════════════════════════════════════
BUILD:        All packages compile ✓
UNIT TESTS:   N test files, all passing ✓
INTEGRATION:  [N tests passing ✓ / smoke checks pass ✓]
CROSS-CHECK:  [N decomposed features verified across layers ✓]

PRE-DEPLOY REVIEW: Sent to penfold (non-blocking) ✓

REPLIES:
  pf-msg-aaa: Resolved (3 items: 2 bugs, 1 feature) → agent-penfold ✓
  pf-msg-bbb: Resolved (1 item: 1 spec) → agent-penfold ✓
```

After displaying progress, return to the orchestrator for deploy.
