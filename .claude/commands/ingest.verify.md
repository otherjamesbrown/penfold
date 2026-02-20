---
description: "Phase 5: Verify builds, unit tests, integration tests, cross-check decomposed features, send pre-deploy review to penfold."
---

# Ingest — Phase 5: Verify & Review

Verify all implementations, run integration tests, cross-check decomposed features,
trace back to original messages, and send pre-deploy review to penfold.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: cxp
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

## MANDATORY: File Bugs for ALL Failures

**Any failure encountered during verification — whether caused by this session or pre-existing —
MUST be filed as a bug shard in Context Palace.** No exceptions. No "noting and moving on."

This includes:
- Unit test failures (even if pre-existing)
- Go vet warnings (even if pre-existing)
- Smoke test failures (even if pre-existing)
- Build warnings
- Missing test coverage for changed code

```bash
# For each failure found:
psql "$DB_CONN" <<'EOSQL'
SELECT create_shard('penfold', 'agent-mycroft',
  'Pre-existing: [short description]',
  $md$## Failure
[what failed, exact error message]

## Context
Found during Phase 5 verification of [session work].
Pre-existing: [yes/no — was this caused by our changes?]

## Reproduction
[command that triggers the failure]
$md$,
  'bug', ARRAY['kind:bug']);
EOSQL
```

**Why:** Pre-existing failures that aren't tracked become invisible. They mask new regressions
and erode trust in the test suite. Filing them creates accountability.

After cross-layer verification passes, close the parent impl shard:
```bash
cxp task close pf-PARENT-SHARD 'All layers complete and verified. Sub-shards: [list]. All builds pass, all tests pass.'
```

**Test coverage check:**
```bash
git diff --name-only | grep "_test.go"
```

If tests are missing, re-launch the agent with explicit test requirements.
If build fails, fix small issues directly or re-launch agent.

## Step 1.5: Run Penfold Acceptance Tests

**For any bug that included a Penfold-provided test**, run it now as an independent verification.
This is the acceptance gate — if the Penfold test doesn't pass, the fix is not done.

```bash
# Quality tests (golden file changes):
go test -tags=quality -v -timeout 10m -run TestQuality/NNN ./tests/quality/

# E2E tests:
go test -tags=e2e -v -run TestE2E_FeatureName ./tests/e2e/
```

**If the Penfold test fails:**
1. The fix is incomplete — do NOT proceed to deploy.
2. Return to Phase 4 with the Penfold test failure output as the error to fix.
3. The Penfold test is the ground truth. Do NOT modify it to make it pass.

**If the Penfold test fails to compile or run** (setup error, missing flag, import failure,
build constraint issue): this is a **BLOCKER**, not a skip. It means the test infrastructure
is broken — either the test was never runnable against the current CLI, or a recent change
broke it. Report the setup failure to penfold immediately as a new bug. Do NOT mark it N/A,
do NOT proceed to deploy, do NOT treat a test that can't run as a test that passed.

**If the Penfold test passes:** Record the result — include the full test stdout in the
pre-deploy review and resolution messages. Not a summary — the actual output.

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

## Step 2.6: Real-Data Verification (Pipeline, Data, AND Visibility Changes)

**Skip this step ONLY if the changes are purely structural (refactoring, test-only, docs).**

**Do NOT skip for:** pipeline changes, data output changes, wiring/visibility bugs,
new fields added to entities or content, column renames, or any fix where the symptom
was "field missing" or "data not showing."

For bugs or features that change how content is processed, extracted, stored, or displayed:

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

**This step is non-negotiable for pipeline AND visibility changes.** Penfold will reject
resolutions that don't include actual CLI output showing the fix works.

**For visibility/wiring bugs specifically:** Run the exact CLI command the user would run
and verify the field appears in JSON output. Example:
```bash
# Bug was "sent_count not visible on entity"
penf relationship entity show ent-person-123 -o json
# Verify: sent_count field present with correct value
```

If the CLI binary needs rebuilding to pick up proto changes, that's part of the fix.
Include `penf update` or `go build ./cmd/penf/...` in the deployment steps.

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

## Step 4: Update Shards with Evidence and Mark for Review

**Before deploying**, update each shard's content with verification evidence, then set
status to `needs-review`. Penfold sees these on the session board.

**The shard content IS the evidence.** If penfold reads the shard and can't find test
output, file list, or verification results, the work will be sent back. Terminal output
alone is not enough — it must be written to the shard.

For each completed shard, append evidence to the shard body:

```bash
cxp shard update pf-SHARD-ID --body-file <(cat <<'EOF'
[preserve existing shard content above — read it first with cxp shard show]

---
**[TIMESTAMP] agent-mycroft:** VERIFIED — ready to deploy.

**Summary:** [1-sentence description of what was done]
**Files modified:** [list every file changed]
**Tests:**
- [TestName]: FAILED before fix, PASSES after
- [paste actual go test stdout — not a summary, the real output]
**Penfold acceptance test:** [PASSES with stdout / N/A — no test provided]
**Deploy targets:** [gateway/worker/CLI]

[For pipeline changes — include real-data verification:]
**Before:** [paste penf command output before fix]
**After:** [paste penf command output after fix]
EOF
)

# Then set status — surfaces in board's "Needs Review" section
cxp shard status pf-SHARD-ID needs-review
```

**Required evidence by type:**

| Type | Must include |
|------|-------------|
| Bug fix | Test name, before/after test output, files modified |
| Feature | Acceptance criteria mapping (N/N met), test output, files modified |
| Pipeline change | All of the above PLUS before/after CLI output from reprocessed content |

**Do NOT proceed to deploy if evidence is missing from any shard.**

**Do NOT close shards here.** Closing happens in Phase 6+7 after commit + deploy, when
the commit hash exists. Setting `needs-review` is the pre-deploy signal.

## Step 6: Close Remaining Shards

If any investigation, analysis, or spec shards are still open, close them:

```bash
cxp task close pf-inv-xxx "Investigation complete, fix deployed"
cxp task close pf-anl-xxx "Analysis complete, feature deployed"
cxp task close pf-spc-xxx "Spec implemented and deployed"
```

## Show Progress

```
INGEST PIPELINE - Phase 5: Verify & Review
═══════════════════════════════════════════
BUILD:        All packages compile ✓
UNIT TESTS:   N test files, all passing ✓
INTEGRATION:  [N tests passing ✓ / smoke checks pass ✓]
CROSS-CHECK:  [N decomposed features verified across layers ✓]

SHARDS IN needs-review:
  pf-fix-aaa: [bug title] — verified, ready to deploy
  pf-feat-bbb: [feature title] — verified, ready to deploy
```

## Checkpoint (MANDATORY)

Before returning to the orchestrator, write a checkpoint:

```bash
cxp session checkpoint "$(cat <<'CKPT'
## Phase 5 Complete: Verify & Review

**Build:** [pass/fail]
**Unit tests:** [pass/fail — N suites, N tests]
**Penfold acceptance tests:** [N/N pass / none provided]
**Integration:** [pass/fail/N/A]
**Go vet:** [clean/warnings]
**Shards in needs-review:** [shard IDs]
**Failures encountered:** [list any — pre-existing or new, with bug shard IDs filed]
**Files to commit:** [count, list paths]
**Deploy targets:** [gateway/worker/CLI — based on changed packages]
**Next:** Phase 6+7 (Deploy) — commit, deploy, verify version
CKPT
)"
```

After displaying progress and writing the checkpoint, return to the orchestrator for deploy.
