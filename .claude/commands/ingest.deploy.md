---
description: "Phase 6+7: Loop for unblocked work, then commit, deploy, verify deployment with version check, show final summary."
---

# Ingest — Phase 6+7: Loop & Deploy

Check for newly unblocked work (loop), then commit, deploy, verify, and summarize.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

## Phase 6: Loop

### Check for Newly Unblocked Work

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status,
  (SELECT array_agg(e.to_id) FROM edges e
   WHERE e.from_id = s.id AND e.edge_type = 'blocked-by'
   AND (SELECT status FROM shards WHERE id = e.to_id) != 'closed') as still_blocked_by
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'feat:%' OR s.title LIKE 'feat-db:%' OR s.title LIKE 'feat-svc:%' OR s.title LIKE 'feat-cli:%')
ORDER BY s.priority, s.created_at;
"
```

### Decision

- If more work is ready (no blockers) → tell orchestrator to loop back to `/ingest.implement`
- If nothing left → proceed to Phase 7 below

---

## Phase 7: Build, Deploy & Release

### Step 1: Commit and Push

**CRITICAL: Do NOT use `git add -A` or `git add .`** — this captures ALL dirty files
including changes from other agent sessions. Stage only files YOUR sub-agents modified.

```bash
# 1. Review what changed
git status
git diff --name-only

# 2. Cross-check: any dirty files NOT from our implementation shards?
#    Compare git diff --name-only against file lists from impl shards.
#    If unexpected files exist, DO NOT stage them.

# 3. Stage ONLY files from our shards (list them explicitly)
git add path/to/file1.go path/to/file2.go path/to/file3_test.go

# 4. Commit with appropriate prefix
git commit -m "$(cat <<'EOF'
fix+feat: [summary of all changes]

Fixes:
- [bullet per bug fix]

Features:
- [bullet per requirement]

Resolves: pf-xxx, pf-yyy, pf-zzz

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
git push origin HEAD
```

### Step 2: Deploy Based on What Changed

**MANDATORY: Map changed packages to deployment targets.** Do NOT reason about this
from memory — check the import graph. A change in `pkg/` may affect gateway, worker,
or both depending on which services import it.

```bash
# For each changed pkg/ directory, check which services import it:
grep -r "import_path" services/gateway/ services/worker/ --include="*.go" -l
```

| Changed Files | Deploy Action |
|---------------|---------------|
| `services/gateway/**` | `./scripts/deploy-gateway.sh` |
| `services/worker/**` | `./scripts/deploy-worker.sh` |
| `cmd/penf/**` | CLI release: bump version + tag + push |
| `api/proto/**` | Deploy gateway + release CLI |
| `pkg/**` only | Deploy **all services that import the changed package** (check imports!) |

**Worker deploy (if deploy script not available):**
```bash
GOOS=darwin GOARCH=arm64 go build -o worker-darwin-arm64 -ldflags="-s -w" ./services/worker
scp worker-darwin-arm64 james@dev01.brown.chat:/tmp/penfold-worker
ssh james@dev01.brown.chat << 'DEPLOY'
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist
sudo mv /tmp/penfold-worker /opt/penfold/bin/penfold-worker
sudo chmod +x /opt/penfold/bin/penfold-worker
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
sudo launchctl list | grep penfold
DEPLOY
```

**CLI release:**
```bash
git tag -l 'v*' | sort -V | tail -1   # Current version
# Bump version in cmd/penf/cmd/version.go
git tag v0.X.Y
git push origin v0.X.Y
# GitHub Actions auto-release.yml creates release
```

### Step 3: Verify Deployment is Live

**MANDATORY. Do NOT skip. Do NOT proceed to summary until verified.**

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646

nomad job status penfold-gateway
nomad job status penfold-worker

./scripts/deploy-gateway.sh --status
./scripts/deploy-worker.sh --status

penf health
penf status
```

### Step 4: Verify Deployed Version Matches Commit

**CRITICAL. Nomad is unreliable — binaries upload but allocations don't always restart.**
This step catches ghost deploys where the old binary is still running.

```bash
# Get the commit we just deployed
EXPECTED_COMMIT=$(git rev-parse --short HEAD)

# Check running version via /version endpoint or CLI
RUNNING_VERSION=$(penf version --server 2>/dev/null || echo "unknown")

echo "Expected: $EXPECTED_COMMIT"
echo "Running:  $RUNNING_VERSION"
```

**If version doesn't match:**
1. The allocation didn't restart. Force it:
```bash
# Force restart the allocation
nomad job restart penfold-gateway
nomad job restart penfold-worker

# Wait 15 seconds for restart
sleep 15

# Re-check version
penf version --server
```
2. If still mismatched after force restart, check allocation logs:
```bash
nomad alloc logs -job penfold-gateway -stderr | tail -50
nomad alloc logs -job penfold-worker -stderr | tail -50
```
3. If crash looping: `nomad job revert <job-name> <previous-version>`
4. **Notify penfold** of deployment issue.

### Step 5: Functional Smoke Tests

**After verifying the service is running with the correct version**, run smoke tests
specific to what was changed. These catch wiring bugs that unit tests miss.

```bash
# Always run:
penf health
penf status
penf content stats

# If entities were changed:
penf entity list --limit 5

# If assertions were changed:
penf assertion list --limit 5

# If glossary was changed:
penf glossary list --limit 5

# If content processing was changed:
penf content list --limit 5
```

**If any smoke test fails:**
1. Check if it's a pre-existing issue or caused by this deploy
2. If caused by this deploy: investigate, fix, and re-deploy
3. If pre-existing: **file a bug shard** in Context Palace — this is MANDATORY, not optional.
   Do NOT "note it in the summary" or "flag it as pre-existing" without filing a shard.
4. **Notify penfold** of smoke test failure

**This applies to ALL failures** — smoke tests, unit tests, go vet warnings, build warnings.
Pre-existing failures that aren't tracked become invisible and mask new regressions.

### Step 6: Close Shards with Evidence

**MANDATORY. After commit + deploy + version verification, close each shard with the
commit hash and deployment evidence.** This is the permanent record penfold reviews.

The commit hash was generated in Step 1. The version was verified in Step 4. Write both
to each shard now.

```bash
COMMIT_HASH=$(git rev-parse --short HEAD)

# For each shard processed in this session:
cxp shard update pf-SHARD-ID --body-file <(cat <<EOF
[read existing shard content first with cxp shard show — preserve it]

---
**[TIMESTAMP] agent-mycroft:** DEPLOYED and verified.

**Commit:** $COMMIT_HASH
**Deploy targets:** [gateway v$COMMIT_HASH / worker v$COMMIT_HASH / CLI vX.Y.Z]
**Version verified:** [gateway RUNNING v$COMMIT_HASH ✓ / worker RUNNING v$COMMIT_HASH ✓]
**Smoke tests:** [penf health ✓ / penf content stats ✓ / etc]

[For pipeline changes — reprocess and include output:]
**Reprocessed:** [content-id]
**Output after deploy:**
[paste penf content show output]
EOF
)

# Close the shard — this is the "done" signal to penfold
cxp shard close pf-SHARD-ID
```

**Every closed shard MUST contain:**
1. Commit hash (from this step)
2. Test output (from Phase 5 Step 4)
3. Files modified (from Phase 5 Step 4)
4. Version verification (from Step 4 above)

**If any of these are missing, DO NOT close the shard.** Go back and add the evidence first.

### Checkpoint (MANDATORY)

```bash
# For each shard deployed:
cxp shard append pf-SHARD-ID --body "[$(date -u +%H:%M)] Phase 6 (Deploy): Deployed [commit], version verified."

cxp session checkpoint "$(cat <<'CKPT'
## Phase 6+7 Complete: Deploy

**Commit:** [hash] "[message]"
**Deployed:** [gateway ✓ / worker ✓ / CLI vX.Y.Z ✓]
**Version verified:** [gateway v[hash] ✓ / worker v[hash] ✓]
**Smoke tests:** [N/N passed]
**Shards closed:** [list shard IDs]
**Shards still open:** [list any not deployed, with reason]
CKPT
)"
```

### Show Final Summary

```
INGEST PIPELINE - COMPLETE
════════════════════════════

## Bug 1: [short title]
Shard:       pf-fix-aaa (investigation: pf-inv-aaa)
Bug:         [1-2 sentence summary]
Fix:         [1-2 sentence summary]
Test:        [TestName] in [file] — FAILED before fix ✓, PASSED after ✓
Penfold:     [Acceptance test PASSES ✓: [command] | N/A (no Penfold test)]
Deploy:      [Gateway ✓ VERIFIED v[hash] | Worker ✓ VERIFIED v[hash] | None needed]
Smoke:       [penf health ✓ | penf entity list ✓]
Real-data:   [Reprocessed pf-CONTENT: before=[summary], after=[summary] | N/A (not pipeline)]
Notified:    agent-penfold ✓ [action required or "no action needed"]

## Feature 1: [short title] (LOW/MEDIUM — single agent)
Shard:       pf-feat-bbb (analysis: pf-anl-bbb)
Requirement: [1-2 sentence summary]
Built:       [1-2 sentence summary]
Test:        [TestName1, TestName2] — acceptance tests PASS ✓
Deploy:      [details + VERIFIED v[hash]]
Smoke:       [relevant smoke test results]
Notified:    agent-penfold ✓

## Feature 2: [short title] (from SPEC, HIGH — decomposed)
Shard:       pf-feat-ccc (spec: pf-spc-ccc)
Spec:        [1-2 sentence summary]
Criteria:    [N/N acceptance criteria met]
Layers:
  DB:        pf-ccc-db  — [summary]. Tests: [TestNames]
  Service:   pf-ccc-svc — [summary]. Tests: [TestNames]
  CLI:       pf-ccc-cli — [summary]. Tests: [TestNames]
Cross-check: All layers build ✓, all tests pass ✓, no conflicts ✓
Deploy:      [details + VERIFIED v[hash]]
Smoke:       [relevant smoke test results]
Notified:    agent-penfold ✓

────────────────────────
Totals: B bugs fixed, R features built, S specs implemented, N deployed, N replies sent
Commit: [hash] "[message]"

Deployment verification:
  Gateway:        RUNNING ✓ (version [hash] verified)
  Worker:         RUNNING ✓ (version [hash] verified)
  CLI:            v0.X.Y released ✓
  Smoke tests:    [N/N passed]
```

**Summary rules:**
- Every item MUST have all fields (Shard, Bug/Requirement/Spec, Fix/Built, Test, Deploy, Smoke, Notified)
- Test: bugs confirm pre-fix fail AND post-fix pass; features confirm acceptance tests pass
- Deploy: must say "VERIFIED v[hash]" — never just "deployed"
- Smoke: list which smoke tests ran and passed
- Real-data: pipeline changes MUST show before/after from reprocessed content
- Notified: confirm reporter was told + whether they need to act
- Decomposed features: list each layer with sub-shard ID, summary, tests
- SPEC features: include acceptance criteria count (N/N met)
- Partial/deferred items: say so explicitly with next steps
- Failed deployment: MUST say so — do not mark as complete
- Version mismatch: MUST be resolved before marking complete
