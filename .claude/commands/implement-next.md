---
description: "Resume implementation: launch agents for ready shards, verify, reply, deploy."
---

# Implement Next

Resume implementation after a pause, failure, or manual triage. Picks up where `/bug-ingest` or `/bug-triage` left off.

## When to Use

- `/bug-ingest` was interrupted mid-pipeline
- You ran `/bug-triage` manually and want to continue implementing
- An implementation agent failed and you want to retry the rest
- Newly unblocked work is available after earlier fixes completed

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

## CRITICAL: Orchestrator Role

You are the **ORCHESTRATOR**. You do **NOT** write code directly.
**NEVER use Edit/Write tools yourself for code changes. ALWAYS delegate to sub-agents.**

---

## Step 1: Find Ready Work

Query for unblocked implementation shards:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.status,
  (SELECT array_agg(e.to_id) FROM edges e
   WHERE e.from_id = s.id AND e.edge_type = 'blocked-by'
   AND (SELECT status FROM shards WHERE id = e.to_id) != 'closed') as still_blocked_by,
  (SELECT array_agg(l.label) FROM labels l
   WHERE l.shard_id = s.id AND l.label LIKE 'agent:%') as agent
FROM shards s
WHERE s.project = 'penfold' AND s.type = 'task'
  AND s.status IN ('open', 'in_progress')
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%')
ORDER BY s.priority, s.created_at;
"
```

Also check ready_tasks:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT * FROM ready_tasks('penfold');
"
```

Filter to items where `still_blocked_by` is NULL (ready to implement).

If nothing is ready:
```
IMPLEMENT NEXT
══════════════
No ready implementation shards found.

Blocked items: N (waiting on dependencies)
In progress: N (agents still working)

Use /bug-status for full pipeline view.
Use /bug-triage to reassess after changes.
```

## Step 2: Check File Conflicts

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT fc.file_path, fc.shard_id, fc.agent_id, fc.claimed_at
FROM file_claims fc
WHERE fc.expires_at > NOW()
ORDER BY fc.file_path;
"
```

If ready shards need files that are currently claimed, either:
- Wait for the claiming shard to complete
- Skip that shard and launch others

## Step 3: Group into Parallel Tracks

Group ready items by independence:
- Items touching different files -> can run in parallel
- Items touching same files -> must be sequenced (add blocked-by if needed)

### Show Queue

```
IMPLEMENT NEXT
══════════════
Ready to launch: N items

PARALLEL BATCH:
  pf-fix-aaa | fix: [title] | cli-dev    | [files]
  pf-fix-bbb | fix: [title] | service-dev | [files]

BLOCKED (will run after batch completes):
  pf-fix-ccc | fix: [title] | worker-dev | blocked by pf-fix-aaa

Launching parallel batch...
```

## Step 4: Launch Implementation Agents

Launch ALL ready agents in a **single message** (parallel, background):

For each ready shard:
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

## Step 5: Monitor Completion

Poll shard status + read background agent output files:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, status, closed_reason
FROM shards
WHERE id IN ('pf-fix-aaa', 'pf-fix-bbb')
ORDER BY created_at;
"
```

Wait until all current-batch shards are closed.

## Step 6: Verify

For each completed implementation:

```bash
# Verify build
go build ./...

# Verify tests
go test ./path/to/changed/... -v

# Check for test files
git diff --name-only | grep "_test.go"
```

If tests are missing, re-launch the agent with explicit test requirements.
If build fails, fix small issues directly or re-launch agent.

## Step 7: Reply to Penfold

For each completed fix, trace back to the original bug and reply:

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

Send resolution reply:

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

## Step 8: Loop

Check for newly unblocked work:

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

- If more work is ready (still_blocked_by is NULL) -> **loop back to Step 4**
- If nothing left -> **proceed to Step 9**

## Step 9: Build, Deploy & Release

After ALL implementations complete and are verified:

### Commit and Push

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

Resolves: pf-xxx, pf-yyy

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
git push origin HEAD
```

### Deploy Based on What Changed

Check `git diff --name-only` and deploy accordingly:

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
git tag -l 'v*' | sort -V | tail -1
# Bump version in cmd/penf/cmd/version.go
git tag v0.X.Y
git push origin v0.X.Y
```

### Verify Deployment

```bash
./scripts/verify-deployment.sh
penf health
penf status
```

### Show Final Summary

```
IMPLEMENT NEXT - COMPLETE
══════════════════════════
Implemented: N shards
Replies sent: N

Summary:
  pf-fix-aaa | [title] | FIXED | replied
  pf-fix-bbb | [title] | FIXED | replied

Deployed:
  [component]: [status]

Committed: [hash] "[message]"
```

## Key Principles

1. **NEVER write code yourself** - always delegate to sub-agents
2. **Maximize parallelism** - launch all independent agents simultaneously
3. **Loop until empty** - keep going while there's unblocked work
4. **Reply to penfold** - every resolved bug gets a reply
5. **Deploy changes** - implementation isn't done until deployed
