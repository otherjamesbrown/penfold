---
description: "Analyze feature requests and launch sub-agents to implement them. Accepts shard IDs or natural language like 'all of the above'."
---

# Implement

Orchestrate implementation of feature requests by analyzing, decomposing, and delegating to sub-agents.

## CRITICAL: Orchestrator Role

You are the **ORCHESTRATOR**. You do **NOT** write code directly.

Your job:
1. Analyze and decompose tasks
2. Identify what can run in parallel
3. Create implementation shards
4. Launch sub-agents to implement
5. Monitor progress and resolve blockers

**NEVER use Edit/Write tools yourself. ALWAYS delegate implementation to sub-agents.**

Sub-agents get spawned with fresh context - this is intentional. They focus solely on their task without accumulated conversation noise.

## User Input

```text
$ARGUMENTS
```

## Phase 1: Parse Input

The user may provide:
- **Explicit IDs**: `pf-43290f pf-503ea3`
- **Natural language**: `all of the above`, `the HIGH priority ones`, `everything except slash commands`
- **Mixed**: `pf-43290f and the pipeline ones`

### If Natural Language

Look back at recent conversation context for:
- Tables with shard IDs (pf-xxxxx patterns)
- Lists of feature requests
- Summaries mentioning specific shards

Extract the relevant shard IDs based on the user's filter:
- "all of the above" -> all shards mentioned
- "HIGH priority ones" -> filter by priority column/mention
- "except X" -> exclude matching shards

**If ambiguous, ask a clarifying question. Otherwise proceed.**

## Phase 2: Fetch and Analyze Shards

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

For each shard ID:
```sql
SELECT id, title, content, status FROM shards WHERE id = 'pf-xxxxx';
```

For each shard, analyze:
1. **What code changes are needed?**
   - Which files will be touched?
   - New files needed?
   - Proto changes?
   - Database changes?

2. **What existing infrastructure can be used NOW?**
   - Existing RPCs that already work
   - Existing client methods
   - Existing patterns to follow

3. **What new infrastructure is needed?**
   - New proto RPCs
   - New gateway handlers
   - New database tables/migrations

4. **What are the acceptance criteria?**
   - Extract from shard content
   - Make implicit criteria explicit

## Phase 3: Check File Claims

**Before planning work, check for conflicts with other sessions.**

```sql
-- Check if any files you need are already claimed
SELECT file_path, claimed_by, shard_id, claimed_at
FROM file_claims
WHERE file_path IN (
  'cmd/penf/cmd/pipeline.go',
  'cmd/penf/cmd/content.go'
  -- list files you plan to modify
)
AND released_at IS NULL;
```

If files are claimed:
1. **Check the claiming shard** - is it still active?
2. **Contact the owner** - coordinate or wait
3. **Find alternative** - can this work use different files?

If no conflicts, proceed to planning.

## Phase 4: Build Parallel Execution Plan

**Goal: Maximize parallelism. Start as much work as possible NOW.**

### Step 1: Separate "Ready Now" vs "Needs Foundation"

```
+-------------------------------------------------------------+
|                    EXISTING INFRASTRUCTURE                   |
|  (RPCs, tables, utilities that already exist)               |
+-----------------------------+-------------------------------+
                              |
          +-------------------+-------------------+
          v                                       v
   +-------------+                         +-------------+
   | READY NOW   |                         | READY NOW   |
   | Agent #1    |                         | Agent #2    |
   | (parallel)  |                         | (parallel)  |
   +-------------+                         +-------------+
                              |
          +-------------------+-------------------+
          |         NEW INFRASTRUCTURE            |
          |  (must be built before dependents)    |
          +-------------------+-------------------+
                              |
                              v
                    +-----------------+
                    | NEEDS FOUNDATION|
                    | Agent #3        |
                    | (after above)   |
                    +-----------------+
```

### Step 2: Identify Parallel Tracks

Work items can run in parallel if they:
- Touch different files (no merge conflicts)
- Use different services (CLI vs Gateway vs Worker)
- Use existing infrastructure (no dependency on new code)

**Example decomposition:**

| Agent | Task | Can Start | Why |
|-------|------|-----------|-----|
| cli-dev #1 | Command using existing RPC | NOW | RPC exists |
| service-dev | New proto + gateway handlers | NOW | No dependencies |
| cli-dev #2 | Commands using new RPCs | After service-dev | Needs new RPCs |

### Step 3: Right-Size the Agents

**Too granular** (avoid):
- 1 agent per function
- 1 agent per file

**Too coarse** (avoid):
- 1 agent for entire feature spanning proto + gateway + CLI + tests

**Right balance**:
- Group by layer AND coherence
- Proto + Gateway handler = 1 agent (tightly coupled)
- Related CLI commands = 1 agent (share patterns)
- Independent CLI command using existing RPC = 1 agent (can parallelize)

## Phase 5: Create Implementation Shards and Claim Files

Use the `create_impl_shard()` helper which:
- Creates the shard with proper structure
- Auto-creates dependency edges via `depends_on`
- Claims files automatically

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard(
  'penfold',
  'agent-mycroft',
  'cli-dev',  -- agent_type: cli-dev | service-dev | worker-dev | data-dev | ai-dev
  'Implement: [specific task]',
  $md$## Goal
[What this shard accomplishes]

## Context
[Why this is needed, what it connects to]

## Files to Modify
- path/to/file1.go - [what changes]
- path/to/file2.go - [what changes]

## Files to Create
- path/to/new_file.go - [purpose]

## Existing Code to Reference
- cmd/penf/cmd/workflow.go - similar command structure

## Acceptance Criteria
- [ ] [Specific, verifiable criterion]
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...
$md$,
  ARRAY['cmd/penf/cmd/pipeline.go', 'cmd/penf/cmd/content.go'],  -- files to claim
  ARRAY['pf-dependency-id'],  -- depends_on (NULL if none)
  'pf-parent-feature'  -- parent shard ID
);
EOSQL
```

### Shard Templates by Agent Type

Use these templates as starting points:

**cli-dev template:**
```
## Goal
Add `penf [command]` command to [purpose].

## Files to Modify
- cmd/penf/cmd/[file].go - Add command

## Tests to Write
- cmd/penf/cmd/[file]_test.go - Test [specific functions/behavior]

## Existing Code to Reference
- cmd/penf/cmd/workflow.go - command pattern
- cmd/penf/cmd/workflow_test.go - test pattern

## Acceptance Criteria
- [ ] Command works: penf [command] --help
- [ ] Code compiles: go build ./cmd/penf/...
- [ ] Tests written and passing: go test ./cmd/penf/cmd/... -run [TestName]
```

**service-dev template:**
```
## Goal
Add [RPC name] RPC to [service].

## Files to Modify
- api/proto/[service]/v1/[service].proto - Add RPC definition
- services/gateway/[service]service/service.go - Add handler

## Tests to Write
- services/gateway/[service]service/service_test.go - Test [handler/validation logic]

## Existing Code to Reference
- api/proto/pipeline/v1/pipeline.proto - RPC patterns
- services/gateway/pipelineservice/service.go - handler patterns
- services/gateway/ingestservice/service_test.go - test patterns

## Acceptance Criteria
- [ ] Proto compiles: make proto
- [ ] Gateway builds: go build ./services/gateway/...
- [ ] Tests written and passing: go test ./services/gateway/[service]service/...
```

**Skipping Tests (edge cases only):**

Some code is genuinely hard to unit test (e.g., CLI output formatting, external API calls without mocks).
If skipping tests, the shard MUST include:
```
## Tests Skipped (with justification)
- [function/feature]: [Why it's hard to test, e.g., "requires live database connection"]
```

The orchestrator will review skipped tests and may request alternatives (integration tests, manual verification steps).

### Fallback: Manual Shard Creation

If `create_impl_shard()` is not available, use manual creation:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_shard('penfold', 'Title', $md$Content$md$, 'task', 'agent-mycroft', 'pf-parent');
-- Then manually add edges and file claims:
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-new', 'pf-dependency', 'blocked-by');
INSERT INTO file_claims (file_path, claimed_by, shard_id) VALUES ('path/file.go', 'agent-mycroft', 'pf-new');
EOSQL
```

## Phase 6: Launch Sub-Agents in Parallel

**Launch ALL agents that can start NOW in a single message with multiple Task tool calls.**

Available agent types:
- cli-dev: CLI commands in cmd/penf/
- service-dev: Gateway, proto, gRPC
- worker-dev: Temporal workflows, activities
- data-dev: Database, migrations, repositories
- ai-dev: Embeddings, search, ML

### Sub-Agent Prompt Template

**IMPORTANT:** Use the `palace` CLI for all Context-Palace operations, not psql.

```
You have been assigned shard pf-xxxxx.

## Setup

1. Read your assignment:
   ```bash
   /Users/dev/bin/palace task get pf-xxxxx
   ```

2. Claim the work:
   ```bash
   /Users/dev/bin/palace task claim pf-xxxxx
   ```

## File Scope

IMPORTANT: Only modify files listed in your shard's "Files to Modify" and "Tests to Write" sections.
The orchestrator has claimed these files for your exclusive use.

## Implementation

3. Read the existing code patterns mentioned in the shard's "Existing Code to Reference"

4. Implement the changes described in the shard

5. **WRITE TESTS for your changes:**
   - Look at the "Tests to Write" section in your shard
   - Follow existing test patterns in the codebase
   - Test the core logic, edge cases, and error conditions
   - If something is genuinely hard to test, document WHY in your completion message

6. **Verify your work compiles:**
   ```bash
   go build ./...
   ```

7. **Run your new tests AND existing tests:**
   ```bash
   go test ./path/to/changed/... -v
   ```

   **IMPORTANT:** Do NOT report completion unless tests pass. If tests fail, fix them first.

8. **Test the feature works** (for CLI commands):
   ```bash
   ./penf [new-command] --help
   ```

## Completion

9. Log what you did (include tests written):
   ```bash
   /Users/dev/bin/palace task progress pf-xxxxx "Implemented X, added TestY and TestZ"
   ```

10. Close the shard (this releases file claims automatically):
   ```bash
   /Users/dev/bin/palace task close pf-xxxxx "Done: [feature] with tests [TestNames]"
   ```

Do not create a PR. Just implement, write tests, verify, and close the shard.
```

### Example Sub-Agent Invocation

```
You have been assigned shard pf-35587b.

## Setup
1. Read your assignment:
   /Users/dev/bin/palace task get pf-35587b

2. Claim the work:
   /Users/dev/bin/palace task claim pf-35587b

## File Scope
Only modify: cmd/penf/cmd/pipeline.go

## Implementation
3. Read cmd/penf/client/logs_client.go for existing client methods
4. Add the `penf pipeline logs` command following the pattern in pipeline.go
5. Verify: go build ./cmd/penf/...
6. Test: ./penf pipeline logs --help

## Completion
7. /Users/dev/bin/palace task progress pf-35587b "Added pipeline logs command with filtering and streaming"
8. /Users/dev/bin/palace task close pf-35587b "Done: penf pipeline logs with --tail, --since, --level, --service flags"
```

### Parallel Launch Pattern

To launch agents in parallel, use multiple Task tool invocations in a SINGLE response message. Do NOT send them sequentially - group all "can start NOW" agents together.

Example: If cli-dev #1 and service-dev can both start NOW, invoke both Task tools in the same message.

## Phase 7: Monitor, Verify, and Coordinate

### Check Progress

Use the implementation status helper:

```sql
-- Get status of all implementation shards for a feature
SELECT * FROM impl_status('pf-parent-feature');
```

**Output:**
```
parent_id  | pf-503ea3
total      | 3
completed  | 2
in_progress| 1
blocked    | 0
shards:
  pf-35587b | cli-dev    | closed  | penf pipeline logs
  pf-c60392 | service-dev| closed  | Gateway RPCs
  pf-43ef25 | cli-dev    | open    | queue/health/trace (blocked by pf-c60392)
```

**Fallback** if helper not available:
```sql
SELECT id, title, status, owner,
  (SELECT array_agg(to_id) FROM edges WHERE from_id = s.id AND edge_type = 'blocked-by') as blocked_by
FROM shards s
WHERE parent_id = 'pf-parent-feature' OR id IN ('pf-xxx', 'pf-yyy');
```

### Verify After Each Agent Completes

**IMPORTANT:** After each sub-agent reports completion, verify the build AND tests:

```bash
# Always verify compilation
go build ./...

# For CLI changes
./penf [new-command] --help

# For gateway changes
go build ./services/gateway/...

# Run the tests (should include new tests from this change)
go test ./path/to/changed/... -v
```

**CRITICAL: Verify tests were written:**
```bash
# Check for new/modified test files
git diff --name-only | grep "_test.go"

# If no test files appear, the agent did NOT write tests
# Re-launch with explicit test requirements
```

If verification fails:
1. Check the shard for what was implemented
2. If tests are missing: re-launch agent with explicit instruction to write tests
3. If tests fail: either fix directly (small issues) or re-launch agent with fix instructions

**Do NOT proceed to deployment if tests were not written** (unless the shard explicitly documents why tests were skipped).

### When Foundation Complete

Once a foundation agent (e.g., service-dev) completes AND verification passes:
1. Launch dependent agents (e.g., cli-dev #2)
2. Continue monitoring

### Handle Failures

If a sub-agent fails:
1. Read the shard for error details
2. Determine if it's a blocker for other shards
3. Ask the user how to proceed:
   - Fix and retry?
   - Skip and continue with non-blocked shards?
   - Abort?

## Phase 8: Commit, Push, and Deploy

**CRITICAL: Implementation is not complete until changes are deployed.**

After all sub-agents complete and verification passes:

### Step 1: Commit Changes

```bash
# Check what changed
git status
git diff --name-only

# Stage and commit with descriptive message
git add -A
git commit -m "feat: [summary of changes]

- [bullet point for each major change]
- Added tests for [what was tested]

Implements: pf-xxxxx, pf-yyyyy"
```

### Step 2: Push to Remote

```bash
git push origin HEAD
```

### Step 3: Deploy Based on What Changed

Determine what needs deployment by checking which files changed:

**If Gateway files changed** (`services/gateway/`):
```bash
./scripts/deploy-gateway.sh
```

**If Worker files changed** (`services/worker/`):
```bash
# Build for Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o worker-darwin-arm64 -ldflags="-s -w" ./services/worker

# Copy to dev01
scp worker-darwin-arm64 james@dev01.brown.chat:/tmp/penfold-worker

# SSH to dev01 and restart via launchd
ssh james@dev01.brown.chat << 'EOF'
sudo launchctl unload /Library/LaunchDaemons/com.penfold.worker.plist
sudo mv /tmp/penfold-worker /opt/penfold/bin/penfold-worker
sudo chmod +x /opt/penfold/bin/penfold-worker
sudo launchctl load /Library/LaunchDaemons/com.penfold.worker.plist
sudo launchctl list | grep penfold
EOF
```

**If CLI files changed** (`cmd/penf/`):
```bash
# Create and push a new version tag
# Get current version
git tag -l 'v*' | sort -V | tail -1

# Bump version (e.g., v0.4.7 -> v0.4.8)
git tag v0.4.X
git push origin v0.4.X

# GitHub Actions will build and release
```

**If Proto files changed** (`api/proto/`):
- Gateway deployment will pick up the changes
- CLI release will pick up the changes
- Deploy both if both are affected

### Step 4: Verify Deployment

```bash
# For gateway
./scripts/verify-deployment.sh

# For CLI (after GitHub Actions completes)
penf version  # should show new version after `penf update`
```

### Deployment Decision Matrix

| Files Changed | Action Required |
|---------------|-----------------|
| `services/gateway/**` | `./scripts/deploy-gateway.sh` |
| `services/worker/**` | Build arm64 binary, deploy to dev01 via launchd |
| `cmd/penf/**` | Create git tag, push (triggers release) |
| `api/proto/**` | Deploy gateway + release CLI |
| `migrations/**` | Deploy gateway (runs migrations) |
| `pkg/**` only | Deploy services that import the package |

See [deploy/README.md](../../deploy/README.md) for detailed service management commands.

## Phase 9: Report Results and Close Shards

When all sub-agents have finished AND deployment is complete:

### Verify All Implementation Shards Are Closed

**IMPORTANT:** Before closing the feature request, verify ALL implementation shards are closed:

```sql
-- Check for any open implementation shards
SELECT id, title, status FROM shards
WHERE parent_id = 'pf-feature-id' AND status != 'closed';
```

If any are still open, close them manually:
```bash
/Users/dev/bin/palace task close pf-xxxxx "Manually closed: implementation complete"
```

### Close the Original Feature Request Shards

The original feature request shards (from Phase 1) should be closed:

```sql
-- Close the original feature request shard
SELECT close_task('pf-original', 'Implemented: [summary of what was built]');
```

### Reply to the Original Requester

If the feature request came from a message, reply to the sender:

```sql
SELECT send_message('penfold', 'agent-mycroft',
  ARRAY['original-sender'],
  'Re: [Original Subject]',
  $md$Your feature request has been implemented.

## What was built
- [List of commands/features added]

## How to get it
[Include ONE of the following based on what changed:]

**CLI update required:**
Run `penf update` to get the latest version.

**Gateway deployed:**
Changes are live - no action needed.

**Worker update:**
Changes deployed to worker service - no action needed.

## Usage examples
```
penf [new-command] --help
penf [new-command] [args]
```

Let me know if you have questions or issues.
$md$,
  NULL, NULL, 'pf-original-message');
```

### Summary for User

```
Implementation Complete

Shards Completed: N
+-- pf-xxxxx: [title] Done
+-- pf-yyyyy: [title] Done
+-- pf-zzzzz: [title] Done

Files Changed:
- cmd/penf/pipeline.go (new)
- cmd/penf/content.go (new)
- services/gateway/handlers.go (modified)

Tests Added:
- cmd/penf/pipeline_test.go
- cmd/penf/content_test.go

Committed: [commit hash] "[commit message]"
Pushed: origin/[branch]

Deployed:
- Gateway: ./scripts/deploy-gateway.sh ✓
- CLI: v0.4.X tag pushed, GitHub Actions building

Verification:
- Gateway health: ✓
- Smoke tests: ✓
```

## Key Principles

1. **NEVER write code yourself** - always delegate to sub-agents
2. **ALWAYS require tests** - no code ships without tests (unless explicitly justified)
3. **ALWAYS commit, push, and deploy** - implementation isn't done until changes are live
4. **Maximize parallelism** - launch all independent work simultaneously
5. **Right-size agents** - not too granular, not too coarse
6. **Fresh context is good** - sub-agents focus without distraction
7. **Sub-agents use palace CLI** - orchestrator uses psql for complex queries
8. **Verify after completion** - always run `go build` AND `go test` after each agent finishes
9. **Check for test files** - verify `_test.go` files were created/modified
10. **Deploy after verification** - gateway changes need `deploy-gateway.sh`, CLI needs version tag
11. **Claim files before work** - check file_claims to avoid conflicts with other sessions
12. **Scoped file access** - sub-agents only modify files explicitly listed in their shard
13. **Use helpers** - prefer `create_impl_shard()` and `impl_status()` over raw SQL

## Troubleshooting

### Sub-agent didn't write tests
This is a common issue. Re-launch the agent with explicit instructions:
```
You completed pf-xxxxx but did NOT write tests.

Please add tests for [specific function/feature].

Look at [existing_test_file.go] for patterns.

Required tests:
- Test[FunctionName]_Success - happy path
- Test[FunctionName]_InvalidInput - error handling
- Test[FunctionName]_EdgeCase - [specific edge case]

Run: go test ./path/... -v -run Test[FunctionName]
```

### Sub-agent didn't close shard
```bash
/Users/dev/bin/palace task close pf-xxxxx "Manually closed: [reason]"
```

### Build fails after agent completion
1. Check what the agent changed: `git diff`
2. Fix small issues directly
3. For larger issues, create a fix shard and launch another agent

### Tests fail after agent completion
1. Run tests with verbose output: `go test ./... -v`
2. Check if it's a test bug or implementation bug
3. If test is wrong, fix the test
4. If implementation is wrong, re-launch agent with the failing test output

### File conflict with another session
```sql
-- Check who has the file
SELECT * FROM file_claims WHERE file_path = 'path/to/file.go' AND released_at IS NULL;
-- Contact the owner or wait for release
```

### Dependencies not respected
Ensure edges exist:
```sql
SELECT * FROM edges WHERE from_id = 'pf-blocked' AND edge_type = 'blocked-by';
-- If missing, add manually:
INSERT INTO edges (from_id, to_id, edge_type) VALUES ('pf-blocked', 'pf-blocker', 'blocked-by');
```
