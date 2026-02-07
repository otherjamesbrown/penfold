---
description: "Pull unread bug reports, investigate, triage, implement, and reply - full automated pipeline."
---

# Bug Ingest Pipeline

Full automated pipeline: pull bugs from inbox, investigate, triage, implement fixes, verify, reply to penfold, deploy.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## CRITICAL: Orchestrator Role

You are the **ORCHESTRATOR**. You do **NOT** write code directly.

Your job:
1. Pull and analyze bug reports
2. Launch debugger agents to investigate
3. Triage findings and create implementation shards
4. Launch test-writing agents to create/fix reproduction tests
5. Launch implementation agents (make failing tests pass)
6. Verify, reply, and deploy

**NEVER use Edit/Write tools yourself for code changes. ALWAYS delegate to sub-agents.**

---

## Phase 1: Pull Bugs

### Step 1: Fetch Unread Bug Reports

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.content, s.creator, s.created_at
FROM unread_for('penfold', 'agent-mycroft') u
JOIN shards s ON s.id = u.id
WHERE s.type = 'message'
ORDER BY s.created_at;
"
```

### Step 2: Analyze Each Message and Split Into Discrete Issues

For each message:
1. Determine if it contains bug reports (vs feature requests, questions, status updates)
2. **Split multi-issue messages into discrete bugs.** A single message may report multiple
   independent issues. Each distinct symptom/problem gets its own investigation shard.

**Example:** A message saying "review queue fails with timeout, reprocess --all not implemented,
and reprocess returns empty job ID" is **3 separate bugs**, not one.

**How to identify discrete issues:**
- Different symptoms (timeout vs missing flag vs empty response)
- Different components (gateway RPC vs CLI flag vs API behavior)
- Could be fixed independently by different agents
- Would get separate root causes

Skip items that are feature requests, questions, or status updates.

### Step 3: Create Investigation Shards (One Per Issue)

Create one investigation shard per discrete issue, NOT per message.
All shards from the same message link back to the same original message ID.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-MESSAGE-ID',
  'investigate: [specific issue title]',
  'Investigate bug reported by [creator]. [specific symptom from message]',
  1, 'agent-mycroft');
"
```

Repeat for each discrete issue found in the message. Example for a 3-issue message:
```sql
-- Issue 1
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-1c9c0b',
  'investigate: review queue connection timeout',
  'ReviewService RPC cannot connect - context deadline exceeded on connect', 1, 'agent-mycroft');
-- Issue 2
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-1c9c0b',
  'investigate: reprocess --all not implemented',
  'reprocess --all flag shows as future in help, not functional', 1, 'agent-mycroft');
-- Issue 3
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-1c9c0b',
  'investigate: reprocess returns empty job ID',
  'reprocess command returns but Job ID is empty', 1, 'agent-mycroft');
```

### Step 4: Acknowledge to Penfold

Send **one ack per message** (not per issue). List the issues identified:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Re: [original subject]',
  \$body\${\"poll_hint\":\"continue\",\"type\":\"ack\"}
Investigating N issues from your report:
1. [issue title]
2. [issue title]
3. [issue title]

Will update as each is resolved.\$body\$,
  NULL, 'ack', 'pf-MESSAGE-ID');
"
```

### Step 5: Mark Messages Read

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT mark_read(ARRAY['pf-msg1', 'pf-msg2'], 'agent-mycroft');
"
```

### Show Progress

```
BUG PIPELINE - Phase 1: Ingest
════════════════════════════════
Messages: N (containing M discrete issues)

Message pf-xxxxxx: "[title]" (from agent-penfold)
  #1 | pf-inv-aaa | [issue title]
  #2 | pf-inv-bbb | [issue title]
  #3 | pf-inv-ccc | [issue title]

Message pf-yyyyyy: "[title]" (from agent-penfold)
  #4 | pf-inv-ddd | [issue title]

Acks sent: N messages acknowledged
Investigations created: M
Launching debuggers...
```

---

## Phase 2: Investigate (Parallel Debuggers)

### Step 1: Launch Debugger Agents

Launch ALL debugger agents in a **single message** (parallel, background):

For each investigation shard, use:
```
Task(subagent_type="debugger", run_in_background=true,
  description="Debug: [short title]",
  prompt="Investigate shard pf-inv-xxx.

  Bug report from agent-penfold:
  [paste full bug content here]

  Your investigation shard: pf-inv-xxx

  ## Setup
  1. /Users/dev/bin/palace task claim pf-inv-xxx

  ## Investigation
  2. Investigate using read-only tools (Read, Grep, Glob, Bash for go build/test)
  3. Identify root cause, affected files, and proposed fix

  ## Completion
  3. /Users/dev/bin/palace task close pf-inv-xxx 'ROOT CAUSE: [category]. [summary]. FILES: [file1, file2]. FIX: [description]'

  Root cause categories: cli_ux, config_drift, temporal_workflow, grpc_wiring, data_layer, proto_mismatch, missing_feature, test_gap")
```

### Step 2: Monitor Completion

Poll investigation shard status until all are closed:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, status, closed_reason
FROM shards
WHERE id IN ('pf-inv-aaa', 'pf-inv-bbb')
ORDER BY created_at;
"
```

Also use `Read` on background agent output files to check progress.

Wait until ALL investigation shards show `status = 'closed'`. Check every 30-60 seconds.

### Show Progress

```
BUG PIPELINE - Phase 2: Investigate
════════════════════════════════════
pf-inv-aaa | [title] | DONE - [category]: [summary]
pf-inv-bbb | [title] | DONE - [category]: [summary]

All N investigations complete. Proceeding to triage...
```

---

## Phase 3: Triage (Auto)

### Step 3a: Extract Findings

Read closed investigation shards:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.content, s.closed_reason
FROM shards s
WHERE s.id IN ('pf-inv-aaa', 'pf-inv-bbb')
  AND s.status = 'closed';
"
```

Parse each `closed_reason` for:
- Root cause category + explanation
- Affected files
- Fix description
- Complexity (Low/Medium/High)

Map category to agent type:

| Category | Agent Type |
|----------|-----------|
| cli_ux | cli-dev |
| config_drift | service-dev or worker-dev |
| temporal_workflow | worker-dev |
| grpc_wiring | service-dev |
| data_layer | data-dev |
| proto_mismatch | service-dev |
| missing_feature | (depends on layer) |
| test_gap | testing-dev |

### Step 3b: Cross-Reference In-Flight Work

Check for overlap with existing work:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status, s.owner,
  (SELECT array_agg(fc.file_path) FROM file_claims fc WHERE fc.shard_id = s.id) as files
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%');
"
```

Three checks:
1. **Overlap:** Two bugs affect same files -> merge into one impl shard or add dependency
2. **Already covered:** In-progress fix will resolve this bug too -> link, don't duplicate
3. **File conflict:** Target files claimed by another session -> set as blocked

### Step 3c: Create Implementation Shards

For each unique fix needed:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard('penfold', 'agent-mycroft', '<agent-type>',
  'fix: [short title]',
  $md$## Goal
[from investigation findings]

## Root Cause
[category]: [explanation]

## Files to Modify
- [files from investigation]

## Fix Description
[from debugger's proposed fix]

## Acceptance Criteria
- [ ] Bug symptom no longer reproducible
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...
- [ ] Regression test added

## Original Bug
pf-[bug-id]: [title]
$md$,
  ARRAY['file1.go', 'file2.go'],
  ARRAY['pf-dependency-or-NULL'],
  'pf-investigation-id'
);
EOSQL
```

### Step 3d: Build Queue

Query the implementation queue:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status,
  (SELECT array_agg(e.to_id) FROM edges e
   WHERE e.from_id = s.id AND e.edge_type = 'blocked-by'
   AND (SELECT status FROM shards WHERE id = e.to_id) != 'closed') as blocked_by,
  (SELECT array_agg(l.label) FROM labels l
   WHERE l.shard_id = s.id AND l.label LIKE 'agent:%') as agent
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%')
ORDER BY s.priority, s.created_at;
"
```

### Show Progress

```
BUG PIPELINE - Phase 3: Triage
════════════════════════════════
Impl shards created: N
Overlaps: [details if any]

QUEUE:
  Ready: pf-fix-aaa (cli-dev), pf-fix-bbb (service-dev)
  Blocked: pf-fix-ccc (worker-dev, blocked by pf-fix-aaa)

Launching ready items...
```

---

## Phase 3.5: Reproduction Tests (Parallel Agents)

Before implementing fixes, ensure each bug has a failing test that proves the issue exists.
This phase gates implementation — no fix proceeds without a validated reproduction.

### Step 1: Assess Testability

For each implementation shard, the orchestrator decides:

- **Testable** — The bug can be caught by a unit or integration test (wrong return value,
  missing field, incorrect query, logic error, etc.)
- **Not directly testable** — The bug requires live infrastructure, timing, network conditions,
  or UI interaction that cannot be meaningfully unit-tested. Examples: connection timeouts to
  external services, race conditions under load, Nomad deployment issues.

If **not directly testable**: skip this phase for that shard, add a note to the impl shard
content explaining why, and proceed directly to Phase 4. The implementation agent should still
add the closest possible test (e.g., testing the error-handling path even if the trigger can't
be simulated).

### Step 2: Launch Test-Writing Agents

Launch ALL testable bug agents in a **single message** (parallel, background):

For each testable implementation shard:
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Test: [short title]",
  prompt="Write a reproduction test for bug pf-inv-xxx.

  ## Bug Summary
  [paste root cause and symptom from investigation/impl shard]

  ## Files Affected
  [files from shard]

  ## Your Task

  ### Step 0: Check for Existing Tests
  Look for existing tests that SHOULD have caught this bug:
  - Search test files in the same package as the affected code
  - Look for tests covering the same function/method/handler
  - Check if there is a test that tests this path but has an error (wrong assertion,
    missing case, incorrect expected value, stale mock)

  ### Step 1: Fix or Create Test
  - **If an existing test should catch this but doesn't:** Fix the existing test so it
    correctly exercises the buggy path. Document what was wrong with the test.
  - **If no relevant test exists:** Write a new focused test that reproduces the bug symptom.
    Follow existing test patterns in the package.

  ### Step 2: Validate the Test Catches the Bug
  Run the test and confirm it **FAILS** against the current (unfixed) code:
    go test ./path/to/... -run TestName -v
  The test MUST fail. A passing test means it does not reproduce the bug — revise it.

  ### Step 3: Report
  Output a summary:
  - Test file and test name
  - Whether this was a fix to an existing test or a new test
  - Confirmation the test fails (paste the failure output)

  CRITICAL: Do NOT fix the bug itself. Only write/fix the test.
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.")
```

### Step 3: Monitor Completion

Read background agent output files to check progress. Wait until all test-writing agents
complete.

For each completed agent, verify:
1. A test file was created or modified
2. The test was run and **failed** (proving it catches the bug)

If an agent's test passes (doesn't catch the bug), re-launch with feedback:
"Your test passed — it does not reproduce the bug. The test must FAIL against current code.
Review the root cause and write a test that exercises the specific failing path."

### Step 4: Update Implementation Shards

For each impl shard, add the test information to the shard content so the implementation
agent knows what test to make pass:

```bash
/Users/dev/bin/palace task progress pf-fix-xxx 'Reproduction test ready: [TestName] in [file]. Test FAILS on current code. Implementation must make this test pass.'
```

### Show Progress

```
BUG PIPELINE - Phase 3.5: Reproduction Tests
══════════════════════════════════════════════
pf-fix-aaa | [title] | TESTABLE  | New test: TestReviewQueueTimeout (FAILS ✓)
pf-fix-bbb | [title] | TESTABLE  | Fixed existing: TestReprocessAll (FAILS ✓)
pf-fix-ccc | [title] | SKIP      | Not testable: requires live Nomad cluster

All reproduction tests validated. Proceeding to implementation...
```

---

## Phase 4: Implement (Parallel Agents)

### Step 1: Launch Implementation Agents

Launch ALL "ready now" agents in a **single message** (parallel, background):

For each ready implementation shard:
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Fix: [short title]",
  prompt="You have been assigned shard pf-fix-xxx.

  ## Setup
  1. Read your assignment:
     /Users/dev/bin/palace task get pf-fix-xxx

  2. Claim the work:
     /Users/dev/bin/palace task claim pf-fix-xxx

  ## File Scope
  IMPORTANT: Only modify files listed in your shard's 'Files to Modify' section.
  Only modify: [files from shard]

  ## Implementation
  3. Read existing code patterns in the affected files
  4. Check shard progress notes for reproduction test details (test name and file)
  5. Run the reproduction test FIRST to confirm it fails:
     go test ./path/to/... -run TestName -v
  6. Implement the fix described in the shard
  7. Run the reproduction test again — it MUST now pass
  8. Run the full test suite for affected packages:
     go build ./path/to/...
     go test ./path/to/... -v

  IMPORTANT: Do NOT report completion unless build and tests pass.
  IMPORTANT: If there is no reproduction test noted in shard progress, write one yourself.

  ## Completion
  9. Log progress:
     /Users/dev/bin/palace task progress pf-fix-xxx 'Implemented: [summary]'

  10. Close the shard:
     /Users/dev/bin/palace task close pf-fix-xxx 'Done: [summary]. Tests: [TestNames]'

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.
  Only modify files. The orchestrator will handle all git operations.

  Do not create a PR. Just implement, write tests, verify, and close the shard.")
```

### Step 2: Monitor Completion

Poll shard status + read background agent output files.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, status, closed_reason
FROM shards
WHERE id IN ('pf-fix-aaa', 'pf-fix-bbb')
ORDER BY created_at;
"
```

Wait until all current-batch implementation shards are closed.

---

## Phase 5: Verify & Reply (Auto)

For each completed implementation:

### Step 5a: Verify Build and Tests

```bash
go build ./...
go test ./path/to/changed/... -v
git diff --name-only | grep "_test.go"
```

If tests are missing, re-launch the agent with explicit test requirements.
If build fails, fix small issues directly or re-launch agent.

### Step 5b: Trace Back to Original Bug

Follow edges from impl shard -> investigation -> original bug message:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
WITH inv AS (
  SELECT e.to_id FROM edges e
  WHERE e.from_id = 'pf-fix-xxx'
  AND e.edge_type IN ('relates-to', 'discovered-from')
), bug AS (
  SELECT e.to_id, s.title, s.creator FROM edges e
  JOIN shards s ON s.id = e.to_id
  JOIN inv ON inv.to_id = e.from_id
  WHERE e.edge_type = 'discovered-from'
  AND s.type = 'message'
)
SELECT * FROM bug;
"
```

### Step 5c: Reply to Penfold

**Group replies by original message.** If one message contained 3 issues, send ONE resolution
reply covering all resolved issues from that message (not 3 separate replies).

Wait until ALL issues from a message are resolved before replying. If some issues from a
message are still in progress, hold the reply until the full batch completes.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft',
  ARRAY['agent-penfold'],
  'Resolved: [original message subject]',
  \$body\${\"poll_hint\":\"done\",\"type\":\"resolution\"}

## Resolved: [original message subject]

### Issues Fixed

**1. [issue title]**
[summary from implementation agent]
- Investigation: pf-inv-aaa (closed)
- Fix: pf-fix-aaa (closed)

**2. [issue title]**
[summary from implementation agent]
- Investigation: pf-inv-bbb (closed)
- Fix: pf-fix-bbb (closed)

**3. [issue title]**
[summary from implementation agent]
- Investigation: pf-inv-ccc (closed)
- Fix: pf-fix-ccc (closed)

### Verification
- Build: passing
- Tests: [TestName1] FAILED before fix, PASSED after fix
- Tests: [TestName2] FAILED before fix, PASSED after fix

### Deployment
[List what was deployed: gateway, worker, CLI vX.Y.Z, etc.]

### Action Required
[If CLI was updated: "Please run `penf update` to get the fix."]
[If server-side only: "No action required — fix is live."]

-- agent-mycroft
\$body\$,
  NULL, 'resolution', 'pf-ORIGINAL-MESSAGE-ID');
"
```

For single-issue messages, the reply is the same but with just one item under "Issues Fixed".

### Step 5d: Close Investigation Shards

If any investigation shards are still open, close them:

```bash
/Users/dev/bin/palace task close pf-inv-xxx "Investigation complete, fix deployed"
```

---

## Phase 6: Loop (Auto)

After all current implementations complete:

### Step 1: Check for Newly Unblocked Work

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT * FROM ready_tasks('penfold');
"
```

Also check for implementation shards that were blocked and are now unblocked:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status,
  (SELECT array_agg(e.to_id) FROM edges e
   WHERE e.from_id = s.id AND e.edge_type = 'blocked-by'
   AND (SELECT status FROM shards WHERE id = e.to_id) != 'closed') as still_blocked_by
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND s.title LIKE 'fix:%'
ORDER BY s.priority, s.created_at;
"
```

### Step 2: Decision

- If more work is ready (no blockers) -> **loop back to Phase 4** (launch next batch)
- If nothing left -> **proceed to Phase 7** (build, deploy, release)

---

## Phase 7: Build, Deploy & Release (Auto)

After ALL implementations complete and are verified:

### Step 7a: Commit and Push

**CRITICAL: Do NOT use `git add -A` or `git add .`** — this captures ALL dirty files
including changes from other agent sessions. Stage only the files YOUR sub-agents modified.

```bash
# 1. Review what changed
git status
git diff --name-only

# 2. Cross-check: are any dirty files NOT from our implementation shards?
#    Compare git diff --name-only against the file lists from our impl shards.
#    If there are unexpected files, DO NOT stage them.

# 3. Stage ONLY files from our shards (list them explicitly)
git add path/to/file1.go path/to/file2.go path/to/file3_test.go

# 4. Commit
git commit -m "$(cat <<'EOF'
fix: [summary of all bug fixes]

- [bullet per fix]

Resolves: pf-xxx, pf-yyy, pf-zzz

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
git push origin HEAD
```

### Step 7b: Deploy Based on What Changed

Check `git diff --name-only` to determine what needs deployment:

| Changed Files | Deploy Action |
|---------------|---------------|
| `services/gateway/**` | `./scripts/deploy-gateway.sh` |
| `services/worker/**` | Build arm64 + deploy to dev01 |
| `cmd/penf/**` | CLI release: bump version + tag + push tag |
| `api/proto/**` | Deploy gateway + release CLI |
| `pkg/**` only | Deploy services that import the changed package |

**Worker deploy:**
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
# Get current version tag
git tag -l 'v*' | sort -V | tail -1
# Bump version in cmd/penf/cmd/version.go
# Create new tag (patch bump)
git tag v0.X.Y
git push origin v0.X.Y
# GitHub Actions auto-release.yml creates release
```

### Step 7c: Verify Deployment is Live

**This step is MANDATORY. Do NOT skip it. Do NOT proceed to the summary until every
deployed service is confirmed running the new code.**

For each service that was deployed, verify the new version is actually running in Nomad:

```bash
export NOMAD_ADDR=http://dev02.brown.chat:4646

# 1. Check Nomad job status — confirm allocation is "running", not "pending" or "dead"
nomad job status penfold-gateway
nomad job status penfold-worker

# 2. Check deploy script status for each deployed service
./scripts/deploy-gateway.sh --status   # confirms healthy + reachable
./scripts/deploy-worker.sh --status    # confirms healthy + reachable

# 3. Health check — confirms services respond
penf health
penf status

# 4. Smoke test — run a basic query to confirm the service is functional
penf glossary list
```

**Verification checklist (check ALL that were deployed):**

| Service | Check | Pass? |
|---------|-------|-------|
| Gateway | `deploy-gateway.sh --status` shows "running" + health check passes | ✓/✗ |
| Worker | `deploy-worker.sh --status` shows "running" + health check passes | ✓/✗ |
| AI Coordinator | `deploy-ai-coordinator.sh --status` shows "running" | ✓/✗ |
| CLI | GitHub Actions release completed, `penf update` fetches new version | ✓/✗ |

**If ANY service fails verification:**
1. Check allocation logs: `nomad alloc logs -job <job-name> -stderr`
2. If crash loop: revert with `nomad job revert <job-name> <previous-version>`
3. Do NOT proceed to summary — fix the deployment first
4. If unable to fix, note the failure explicitly in the summary

### Show Final Summary

Display a per-bug summary table. For EACH bug processed, include all of these fields:

```
BUG PIPELINE - COMPLETE
════════════════════════

## Bug 1: [short title]
Shard:       pf-fix-aaa (investigation: pf-inv-aaa)
Bug:         [1-2 sentence summary of the reported symptom and root cause]
Fix:         [1-2 sentence summary of what was changed to resolve it]
Test:        [TestName] in [file] — FAILED before fix ✓, PASSED after fix ✓
             [or: "Not directly testable — [reason]. Closest test: [TestName]"]
             [or: "Fixed existing test [TestName] — had [what was wrong]"]
Deploy:      [Gateway deployed ✓ VERIFIED RUNNING | Worker deployed ✓ VERIFIED RUNNING | CLI v0.X.Y released ✓ | None needed]
Notified:    agent-penfold replied ✓ [user action required: "run penf update" | no action needed]

## Bug 2: [short title]
Shard:       pf-fix-bbb (investigation: pf-inv-bbb)
Bug:         [summary]
Fix:         [summary]
Test:        [details]
Deploy:      [details — must include VERIFIED RUNNING for each service]
Notified:    [details]

────────────────────────
Totals: N bugs fixed, N deployed, N replies sent
Commit: [hash] "[message]"

Deployment verification:
  Gateway:  RUNNING ✓ (health check passed)
  Worker:   RUNNING ✓ (health check passed)
  CLI:      v0.X.Y released ✓
```

**Rules for the summary:**
- Every bug MUST have all 6 fields (Shard, Bug, Fix, Test, Deploy, Notified)
- Test field must confirm both pre-fix failure AND post-fix pass (or explain why not testable)
- Deploy field must list every deployment action taken AND confirm "VERIFIED RUNNING" for each
  service, or "None needed" if code-only. Never say just "deployed" without verification.
- Notified field must confirm the reporter was told, and whether they need to do anything
  (e.g. "run `penf update`" for CLI changes, "no action needed" for server-side fixes)
- If a bug was partially fixed or deferred, say so explicitly with next steps
- If deployment verification failed, the summary MUST say so — do not mark as complete

---

## Error Handling

### Debugger Agent Fails
- Read background output file for error details
- Re-launch with more context or different approach
- If investigation is inconclusive, close shard with partial findings and mark impl as "needs manual investigation"

### Implementation Agent Fails
- Check shard status and agent output
- If build failure: re-launch with error output
- If test failure: re-launch with failing test details
- If stuck: close shard, create a new one with refined instructions

### No Bugs Found
If inbox has no bug reports:
```
BUG PIPELINE - Phase 1: Ingest
════════════════════════════════
No bug reports found in inbox.

Use /bug-status to check existing pipeline state.
```

### Partial Completion
If some implementations succeed and others fail:
- Deploy what's ready
- Reply to penfold for resolved bugs
- Leave failed items in queue for `/implement-next`

## Key Principles

1. **NEVER write code yourself** - always delegate to sub-agents
2. **Maximize parallelism** - launch all independent debuggers/implementers simultaneously
3. **Auto-continue** - don't stop between phases unless there's an error
4. **Reply to penfold** - every bug gets an ack and a resolution reply
5. **Deploy changes** - implementation isn't done until deployed
6. **Trace edges** - maintain the chain: bug message -> investigation -> implementation

---

## Test Quality Requirements (Lessons Learned)

Previous bug-fix cycles had tests that **passed but didn't catch the actual bugs**. This
section encodes the patterns to avoid.

### Anti-Pattern 1: Testing Functions Instead of Pipelines

**BAD:** Test calls `ResolveOrCreate(email, "Alice")` directly with correct inputs.
**REALITY:** In production, `FetchSource` extracts emails but not display names, so
`ResolveOrCreate(email, "")` is called with empty string.

**Rule:** Reproduction tests must exercise the **actual code path** from the entry point
the pipeline uses, not call the fixed function directly with ideal inputs. If the bug is
that data doesn't flow correctly between stages, the test must cover that flow.

### Anti-Pattern 2: Testing With Fresh State

**BAD:** Test creates a new entity and verifies `DetectAccountType` returns `"role"`.
**REALITY:** Entity already exists in DB with `account_type='person'` from before the
pattern was added. `ResolveOrCreate` finds it and returns it unchanged.

**Rule:** Reproduction tests for data-layer bugs must set up **pre-existing state** that
simulates the production scenario (stale records, previously-created entities, etc.).

### Anti-Pattern 3: Assuming Upstream Stages Succeed

**BAD:** Test provides a populated `DeepAnalyzeOutput` to acronym detection.
**REALITY:** Stage 4 (LLM) times out, so `Analysis` is nil. Acronym detection finds
nothing because it only reads from Stage 4 output.

**Rule:** Reproduction tests must cover the failure mode, not just the happy path. If the
bug manifests when an upstream stage fails, the test must simulate that failure (nil input,
empty output, error return).

### Anti-Pattern 4: Misattributing Root Cause to External Services

**BAD:** "The LLM service is overwhelmed by concurrent requests."
**REALITY:** The Temporal HeartbeatTimeout was too short for chunked processing. The LLM
service (Gemini) handles concurrency fine — the activity framework killed the task.

**Rule:** Before attributing a failure to an external service (LLM, DB, network), verify
the framework configuration (timeouts, retries, heartbeats). Infrastructure config issues
masquerade as service failures.

### How to Apply These Rules

**For debugger agents (Phase 2):** Add this to the investigation prompt:
```
IMPORTANT: Trace the FULL data path from database → activity → function.
Do not just verify the function works in isolation. Identify:
- Where does the input data come from? (DB column, upstream stage, metadata)
- Is the data transformed or filtered between stages?
- What happens when upstream stages fail or return nil?
- Are there existing records in the DB that predate the fix?
```

**For test-writing agents (Phase 3.5):** Add this to the test prompt:
```
IMPORTANT: Your test must reproduce the ACTUAL production failure, not just test
the fixed function in isolation. Common mistakes to avoid:
1. Do NOT call the function directly with ideal inputs — test through the same
   entry point the pipeline uses
2. Do NOT use fresh/empty state — set up pre-existing records that simulate
   production (stale DB records, previously-created entities)
3. Do NOT assume all upstream stages succeed — if the bug appears when a stage
   fails, simulate that failure (nil analysis, empty extraction, timeout)
4. The test MUST fail before the fix AND pass after — if it passes without the
   fix, it's testing the wrong thing
```

**For implementation agents (Phase 4):** Add this to the impl prompt:
```
After implementing the fix, verify it addresses the PIPELINE-LEVEL issue:
1. Does the fix change the actual code path the workflow calls? (not just a
   helper function that's never invoked)
2. Does the fix handle existing/stale data? (not just new records)
3. Does the fix degrade gracefully when upstream stages fail?
```
