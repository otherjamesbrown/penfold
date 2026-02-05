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
4. Launch implementation agents
5. Verify, reply, and deploy

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

### Step 2: Analyze Each Message

For each message, determine if it's a bug report (vs feature request, question, status update).
Only process messages that describe bugs or broken behavior.

### Step 3: Create Investigation Shards

For each confirmed bug, create an investigation shard:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-BUG-ID',
  'investigate: [short title]',
  'Investigate bug reported by [creator]. [summary of symptoms]',
  1, 'agent-mycroft');
"
```

### Step 4: Acknowledge to Penfold

Reply to each bug report with an acknowledgment:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Re: [original subject]',
  \$body\${\"poll_hint\":\"continue\",\"type\":\"ack\"}
Investigating. Will update when resolved.\$body\$,
  NULL, 'ack', 'pf-BUG-ID');
"
```

### Step 5: Mark Messages Read

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT mark_read(ARRAY['pf-bug1', 'pf-bug2'], 'agent-mycroft');
"
```

### Show Progress

```
BUG PIPELINE - Phase 1: Ingest
════════════════════════════════
Bugs found: N

# | Bug ID    | Title                    | Investigation
──┼───────────┼──────────────────────────┼─────────────
1 | pf-xxxxxx | [title]                  | pf-inv-aaa
2 | pf-yyyyyy | [title]                  | pf-inv-bbb

Acks sent to agent-penfold.
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
  4. Implement the fix described in the shard
  5. WRITE TESTS (regression test for the bug symptom)
  6. Verify compilation:
     go build ./path/to/...
  7. Run tests:
     go test ./path/to/... -v

  IMPORTANT: Do NOT report completion unless build and tests pass.

  ## Completion
  8. Log progress:
     /Users/dev/bin/palace task progress pf-fix-xxx 'Implemented: [summary]'

  9. Close the shard:
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

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft',
  ARRAY['agent-penfold'],
  'Resolved: [bug title]',
  \$body\${\"poll_hint\":\"done\",\"type\":\"resolution\"}

## Resolved: [title]

### What Was Done
[summary from implementation agent]

### Verification
- Build: passing
- Tests: passing (new regression test added)

### Shards
- Investigation: pf-inv-xxx (closed)
- Fix: pf-fix-xxx (closed)

-- agent-mycroft
\$body\$,
  NULL, 'resolution', 'pf-ORIGINAL-BUG-ID');
"
```

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

### Step 7c: Verify Deployment

```bash
./scripts/verify-deployment.sh   # gateway
penf health                       # overall
penf status                       # connectivity
```

### Show Final Summary

```
BUG PIPELINE - COMPLETE
════════════════════════
Processed: N bugs
Investigations: N (all closed)
Implementations: N (all closed)
Replies sent: N

Summary:
  pf-xxxxxx | [title] | FIXED -> pf-fix-aaa | replied
  pf-yyyyyy | [title] | FIXED -> pf-fix-bbb | replied

Deployed:
  [Gateway: ./scripts/deploy-gateway.sh ✓]
  [Worker: arm64 binary deployed to dev01 ✓]
  [CLI: v0.X.Y tag pushed, GitHub Actions building ✓]

Committed: [hash] "[message]"
```

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
