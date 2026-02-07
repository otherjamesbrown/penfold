---
description: "Phase 6+7: Loop for unblocked work, then commit, deploy, verify deployment, show final summary."
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

| Changed Files | Deploy Action |
|---------------|---------------|
| `services/gateway/**` | `./scripts/deploy-gateway.sh` |
| `services/worker/**` | `./scripts/deploy-worker.sh` |
| `cmd/penf/**` | CLI release: bump version + tag + push |
| `api/proto/**` | Deploy gateway + release CLI |
| `pkg/**` only | Deploy services that import the changed package |

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
penf glossary list
```

**Verification checklist:**

| Service | Check | Pass? |
|---------|-------|-------|
| Gateway | `deploy-gateway.sh --status` running + health check | ✓/✗ |
| Worker | `deploy-worker.sh --status` running + health check | ✓/✗ |
| AI Coordinator | `deploy-ai-coordinator.sh --status` running | ✓/✗ |
| CLI | GitHub Actions release completed | ✓/✗ |

**If ANY service fails:**
1. Check logs: `nomad alloc logs -job <job-name> -stderr`
2. If crash loop: `nomad job revert <job-name> <previous-version>`
3. Do NOT proceed to summary — fix first
4. If unable to fix, note failure explicitly in summary

### Show Final Summary

```
INGEST PIPELINE - COMPLETE
════════════════════════════

## Bug 1: [short title]
Shard:       pf-fix-aaa (investigation: pf-inv-aaa)
Bug:         [1-2 sentence summary]
Fix:         [1-2 sentence summary]
Test:        [TestName] in [file] — FAILED before fix ✓, PASSED after ✓
Deploy:      [Gateway ✓ VERIFIED RUNNING | Worker ✓ VERIFIED RUNNING | None needed]
Notified:    agent-penfold ✓ [action required or "no action needed"]

## Feature 1: [short title] (LOW/MEDIUM — single agent)
Shard:       pf-feat-bbb (analysis: pf-anl-bbb)
Requirement: [1-2 sentence summary]
Built:       [1-2 sentence summary]
Test:        [TestName1, TestName2] — acceptance tests PASS ✓
Deploy:      [details + VERIFIED RUNNING]
Notified:    agent-penfold ✓

## Feature 2: [short title] (HIGH — decomposed)
Shard:       pf-feat-ccc (analysis: pf-anl-ccc)
Requirement: [1-2 sentence summary]
Layers:
  DB:        pf-ccc-db  — [summary]. Tests: [TestNames]
  Service:   pf-ccc-svc — [summary]. Tests: [TestNames]
  CLI:       pf-ccc-cli — [summary]. Tests: [TestNames]
Cross-check: All layers build ✓, all tests pass ✓, no conflicts ✓
Deploy:      [details + VERIFIED RUNNING]
Notified:    agent-penfold ✓

────────────────────────
Totals: B bugs fixed, R features built, N deployed, N replies sent
Commit: [hash] "[message]"

Deployment verification:
  Gateway:  RUNNING ✓ (health check passed)
  Worker:   RUNNING ✓ (health check passed)
  CLI:      v0.X.Y released ✓
```

**Summary rules:**
- Every item MUST have all 6 fields (Shard, Bug/Requirement, Fix/Built, Test, Deploy, Notified)
- Test: bugs confirm pre-fix fail AND post-fix pass; features confirm acceptance tests pass
- Deploy: must say "VERIFIED RUNNING" or "None needed" — never just "deployed"
- Notified: confirm reporter was told + whether they need to act
- Decomposed features: list each layer with sub-shard ID, summary, tests
- Partial/deferred items: say so explicitly with next steps
- Failed deployment: MUST say so — do not mark as complete
