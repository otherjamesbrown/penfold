---
description: "Pull unread bug reports and requirements, investigate/analyze, triage, implement, and reply - full automated pipeline."
---

# Ingest Pipeline

Full automated pipeline: pull bugs and requirements from inbox, investigate/analyze, triage, implement, verify, reply to penfold, deploy.

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
1. Pull and classify inbox items (bugs vs requirements)
2. **Bugs:** Launch debugger agents to investigate root causes
3. **Requirements:** Launch explore agents to analyze scope and approach
4. Triage findings and create implementation shards
5. Launch test-writing agents (reproduction tests for bugs, acceptance tests for requirements)
6. Launch implementation agents (make failing tests pass)
7. Verify, reply, and deploy

**NEVER use Edit/Write tools yourself for code changes. ALWAYS delegate to sub-agents.**

---

## Phase 1: Pull & Classify

### Step 1: Fetch Unread Messages

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.content, s.creator, s.created_at
FROM unread_for('penfold', 'agent-mycroft') u
JOIN shards s ON s.id = u.id
WHERE s.type = 'message'
ORDER BY s.created_at;
"
```

### Step 2: Classify and Split Into Discrete Items

For each message:
1. Classify each discrete item as one of:
   - **BUG** — Something that used to work (or should work) but doesn't. Has a symptom,
     error, or unexpected behavior. Needs investigation to find root cause.
   - **REQUIREMENT** — A new capability, enhancement, or change request. Something that
     doesn't exist yet and needs to be built. No root cause to investigate — the code
     isn't broken, it's missing.
   - **SKIP** — Questions, status updates, acks, or items that don't need implementation.

2. **Split multi-item messages.** A single message may contain both bugs AND requirements,
   or multiple of each. Each discrete item gets its own shard.

**Classification examples:**
- "review queue fails with timeout" → **BUG** (something broke)
- "reprocess --all not implemented" → **REQUIREMENT** (feature doesn't exist)
- "reprocess returns empty job ID" → **BUG** (unexpected behavior)
- "add a --format json flag to penf status" → **REQUIREMENT** (new feature)
- "glossary export should support CSV" → **REQUIREMENT** (new capability)
- "glossary list crashes when DB is empty" → **BUG** (crash/error)

**How to identify discrete items:**
- Different symptoms or different features requested
- Different components (gateway RPC vs CLI flag vs API behavior)
- Could be fixed/built independently by different agents
- Bugs have root causes; requirements have acceptance criteria

### Step 3: Create Shards (One Per Item)

Create one shard per discrete item, NOT per message.
All shards from the same message link back to the same original message ID.

**For BUGS:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-MESSAGE-ID',
  'investigate: [specific bug title]',
  'Investigate bug reported by [creator]. [specific symptom from message]',
  1, 'agent-mycroft');
"
```

**For REQUIREMENTS:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT create_task_from('penfold', 'agent-mycroft', 'pf-MESSAGE-ID',
  'analyze: [specific requirement title]',
  'Analyze requirement from [creator]. [what needs to be built]',
  1, 'agent-mycroft');
"
```

Note the shard title prefix: `investigate:` for bugs, `analyze:` for requirements. This
prefix is used in later phases to determine which path to follow.

### Step 4: Acknowledge to Penfold

Send **one ack per message** (not per item). List the items identified with their type:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Re: [original subject]',
  \$body\${\"poll_hint\":\"continue\",\"type\":\"ack\"}
Processing N items from your report:
1. [BUG] [issue title]
2. [REQ] [requirement title]
3. [BUG] [issue title]

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
INGEST PIPELINE - Phase 1: Pull & Classify
════════════════════════════════════════════
Messages: N (containing M discrete items: B bugs, R requirements)

Message pf-xxxxxx: "[title]" (from agent-penfold)
  #1 | BUG | pf-inv-aaa | [issue title]
  #2 | REQ | pf-anl-bbb | [requirement title]
  #3 | BUG | pf-inv-ccc | [issue title]

Message pf-yyyyyy: "[title]" (from agent-penfold)
  #4 | REQ | pf-anl-ddd | [requirement title]

Acks sent: N messages acknowledged
Shards created: M (B investigations, R analyses)
Launching Phase 2...
```

---

## Phase 2: Investigate Bugs (Parallel Debuggers)

**This phase runs ONLY for items classified as BUG.** Skip if there are no bugs.

### Step 1: Launch Debugger Agents

Launch ALL debugger agents in a **single message** (parallel, background):

For each investigation shard (title starts with `investigate:`), use:
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

  IMPORTANT: Trace the FULL data path from database → activity → function.
  Do not just verify the function works in isolation. Identify:
  - Where does the input data come from? (DB column, upstream stage, metadata)
  - Is the data transformed or filtered between stages?
  - What happens when upstream stages fail or return nil?
  - Are there existing records in the DB that predate the fix?

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
INGEST PIPELINE - Phase 2: Investigate Bugs
═════════════════════════════════════════════
pf-inv-aaa | [title] | DONE - [category]: [summary]
pf-inv-bbb | [title] | DONE - [category]: [summary]

All N investigations complete. Proceeding to triage...
```

---

## Phase 2R: Analyze Requirements (Parallel Explorers)

**This phase runs ONLY for items classified as REQUIREMENT.** Skip if there are no requirements.

Requirements don't need root-cause investigation — the code isn't broken, it's missing.
Instead, launch Explore agents to understand the scope, find existing patterns, and
determine the implementation approach.

### Step 1: Launch Explore Agents

Launch ALL explore agents in a **single message** (parallel, background):

For each analysis shard (title starts with `analyze:`), use:
```
Task(subagent_type="Explore", run_in_background=true,
  description="Analyze: [short title]",
  prompt="Analyze requirement shard pf-anl-xxx.

  Requirement from agent-penfold:
  [paste full requirement content here]

  Your analysis shard: pf-anl-xxx

  ## Setup
  1. /Users/dev/bin/palace task claim pf-anl-xxx

  ## Analysis
  Explore the codebase to answer:

  2. **Existing patterns:** Find the closest existing feature to what's being requested.
     How is it structured? What packages, files, and patterns does it use?
     Example: if the requirement is 'add CSV export to glossary', find how existing
     export/list commands work and what patterns they follow.

  3. **Scope:** What files need to be created or modified? Is this a new file, a new
     function in an existing file, or changes across multiple files?

  4. **Dependencies:** Does this require changes to protos, database schema, or shared
     packages? Are there upstream/downstream impacts?

  5. **Agent type:** Based on the layer this touches, which agent type should implement it?
     - CLI commands/flags/output → cli-dev
     - gRPC services, proto changes → service-dev
     - Temporal workflows/activities → worker-dev
     - Database queries/migrations → data-dev
     - AI/ML features → ai-dev

  6. **Complexity:** Low (single file, follow existing pattern), Medium (multiple files,
     some design decisions), High (new subsystem, cross-cutting concerns).

  ## Completion
  7. /Users/dev/bin/palace task close pf-anl-xxx 'SCOPE: [agent-type]. [summary of what to build]. FILES: [file1, file2, new:file3]. PATTERN: [existing feature to follow]. COMPLEXITY: [Low/Medium/High]'

  IMPORTANT: Do NOT write any code. Only analyze and report.")
```

### Step 2: Monitor Completion

Poll analysis shard status until all are closed:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, status, closed_reason
FROM shards
WHERE id IN ('pf-anl-aaa', 'pf-anl-bbb')
ORDER BY created_at;
"
```

Also use `Read` on background agent output files to check progress.

Wait until ALL analysis shards show `status = 'closed'`. Check every 30-60 seconds.

### Show Progress

```
INGEST PIPELINE - Phase 2R: Analyze Requirements
══════════════════════════════════════════════════
pf-anl-aaa | [title] | DONE - [agent-type]: [summary]
pf-anl-bbb | [title] | DONE - [agent-type]: [summary]

All N analyses complete. Proceeding to triage...
```

---

## Phase 3: Triage (Auto)

This phase processes findings from BOTH Phase 2 (bug investigations) and Phase 2R
(requirement analyses) and creates implementation shards for all of them.

### Step 3a: Extract Findings

Read all closed investigation and analysis shards:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT s.id, s.title, s.content, s.closed_reason
FROM shards s
WHERE s.id IN ('pf-inv-aaa', 'pf-anl-bbb')
  AND s.status = 'closed';
"
```

**For bug investigations**, parse each `closed_reason` for:
- Root cause category + explanation
- Affected files
- Fix description
- Complexity (Low/Medium/High)

**For requirement analyses**, parse each `closed_reason` for:
- Agent type
- Scope summary (what to build)
- Files to create/modify
- Existing pattern to follow
- Complexity (Low/Medium/High)

Map to agent type:

| Category / Layer | Agent Type |
|------------------|-----------|
| cli_ux, CLI commands | cli-dev |
| config_drift | service-dev or worker-dev |
| temporal_workflow | worker-dev |
| grpc_wiring, proto changes | service-dev |
| data_layer, DB queries | data-dev |
| proto_mismatch | service-dev |
| ai/ml features | ai-dev |
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
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%' OR s.title LIKE 'feat:%');
"
```

Three checks:
1. **Overlap:** Two items affect same files -> merge into one impl shard or add dependency
2. **Already covered:** In-progress work will resolve this too -> link, don't duplicate
3. **File conflict:** Target files claimed by another session -> set as blocked

### Step 3c: Create Implementation Shards

**For BUGS** — use `fix:` prefix, include root cause:

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

**For REQUIREMENTS** — use `feat:` prefix, include approach and pattern:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_impl_shard('penfold', 'agent-mycroft', '<agent-type>',
  'feat: [short title]',
  $md$## Goal
[what needs to be built, from the original requirement]

## Approach
[from explorer's analysis — how to build it, which patterns to follow]

## Existing Pattern to Follow
[specific file/function/package that serves as a template for this work]

## Files to Modify/Create
- [existing files to modify]
- NEW: [new files to create]

## Acceptance Criteria
- [ ] [specific behavioral criteria from the requirement]
- [ ] [e.g., "penf status --format json outputs valid JSON"]
- [ ] [e.g., "glossary export --csv writes CSV to stdout"]
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...
- [ ] Acceptance tests added

## Original Requirement
pf-[req-id]: [title]
$md$,
  ARRAY['file1.go', 'file2.go'],
  ARRAY['pf-dependency-or-NULL'],
  'pf-analysis-id'
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
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'Implement:%' OR s.title LIKE 'feat:%')
ORDER BY s.priority, s.created_at;
"
```

### Show Progress

```
INGEST PIPELINE - Phase 3: Triage
══════════════════════════════════
Impl shards created: N (B fixes, R features)
Overlaps: [details if any]

QUEUE:
  Ready: pf-fix-aaa (cli-dev), pf-feat-bbb (service-dev)
  Blocked: pf-fix-ccc (worker-dev, blocked by pf-fix-aaa)

Launching tests...
```

---

## Phase 3.5: Tests (Parallel Agents)

Before implementing, ensure each item has a failing test that defines success.
This phase gates implementation — nothing proceeds without a validated test.

**For BUGS:** Reproduction tests — prove the bug exists (test FAILS against current code).
**For REQUIREMENTS:** Acceptance tests — define the desired behavior (test FAILS because
the feature doesn't exist yet).

In both cases the pattern is the same: write a test that FAILS now and will PASS after
implementation.

### Step 1: Assess Testability

For each implementation shard, the orchestrator decides:

- **Testable** — The behavior can be verified by a unit or integration test
- **Not directly testable** — Requires live infrastructure, timing, network conditions,
  or UI interaction that cannot be meaningfully unit-tested. Examples: connection timeouts
  to external services, race conditions under load, Nomad deployment issues.

If **not directly testable**: skip this phase for that shard, add a note to the impl shard
content explaining why, and proceed directly to Phase 4. The implementation agent should
still add the closest possible test.

### Step 2: Launch Test-Writing Agents

Launch ALL testable agents in a **single message** (parallel, background):

**For BUG shards** (title starts with `fix:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Test: [short title]",
  prompt="Write a reproduction test for bug pf-fix-xxx.

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

  CRITICAL: Do NOT fix the bug itself. Only write/fix the test.
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.")
```

**For REQUIREMENT shards** (title starts with `feat:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Test: [short title]",
  prompt="Write acceptance tests for requirement pf-feat-xxx.

  ## Requirement Summary
  [paste goal and acceptance criteria from impl shard]

  ## Approach
  [paste approach and pattern to follow from impl shard]

  ## Files Affected
  [files from shard — both existing and new]

  ## Your Task

  ### Step 0: Understand the Pattern
  Read the existing pattern referenced in the shard (the file/function that serves as
  a template). Understand how similar features are tested in this codebase.

  ### Step 1: Write Acceptance Tests
  Write tests that verify each acceptance criterion from the shard. The tests should:
  - Follow existing test patterns in the package
  - Test the PUBLIC interface (CLI command output, gRPC response, function return value)
  - Cover the happy path AND key edge cases from the acceptance criteria
  - Be placed in the appropriate test file (existing or new, following package conventions)

  For each acceptance criterion, write at least one test case. Example:
  - Criterion: 'penf status --format json outputs valid JSON'
  - Test: TestStatusCommand_JSONFormat — calls the command handler with --format=json,
    verifies output parses as valid JSON and contains expected fields

  ### Step 2: Validate Tests Fail
  Run the tests and confirm they **FAIL** because the feature doesn't exist yet:
    go test ./path/to/... -run TestName -v
  The tests MUST fail (function not found, flag not recognized, missing output, etc.).
  A passing test means it's not testing the new behavior — revise it.

  If the test fails to COMPILE because the function/type doesn't exist yet, that counts
  as a valid failure. Note this in your report — the implementation agent will create
  the missing code.

  ### Step 3: Report
  Output a summary:
  - Test file and test name(s)
  - What each test verifies (mapped to acceptance criteria)
  - Confirmation the tests fail (paste the failure/compile-error output)

  CRITICAL: Do NOT implement the feature itself. Only write the tests.
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.")
```

### Step 3: Monitor Completion

Read background agent output files to check progress. Wait until all test-writing agents
complete.

For each completed agent, verify:
1. A test file was created or modified
2. The test was run and **failed** (proving it catches the bug / feature is missing)

If an agent's test passes (doesn't catch the bug or already satisfied):
- **Bug:** Re-launch with: "Your test passed — it does not reproduce the bug. The test
  must FAIL against current code. Review the root cause and write a test that exercises
  the specific failing path."
- **Requirement:** Re-launch with: "Your test passed — it does not verify the new behavior.
  The feature doesn't exist yet, so the test should fail. Make sure you're testing the new
  capability, not existing behavior."

### Step 4: Update Implementation Shards

For each impl shard, add the test information so the implementation agent knows what to
make pass:

**For bugs:**
```bash
/Users/dev/bin/palace task progress pf-fix-xxx 'Reproduction test ready: [TestName] in [file]. Test FAILS on current code. Implementation must make this test pass.'
```

**For requirements:**
```bash
/Users/dev/bin/palace task progress pf-feat-xxx 'Acceptance tests ready: [TestName1, TestName2] in [file]. Tests FAIL (feature not implemented). Implementation must make all tests pass.'
```

### Show Progress

```
INGEST PIPELINE - Phase 3.5: Tests
════════════════════════════════════
pf-fix-aaa  | [title] | BUG  | TESTABLE | New test: TestReviewQueueTimeout (FAILS ✓)
pf-fix-bbb  | [title] | BUG  | TESTABLE | Fixed existing: TestReprocessAll (FAILS ✓)
pf-feat-ccc | [title] | REQ  | TESTABLE | New tests: TestStatusJSON, TestStatusCSV (FAIL ✓)
pf-fix-ddd  | [title] | BUG  | SKIP     | Not testable: requires live Nomad cluster

All tests validated. Proceeding to implementation...
```

---

## Phase 4: Implement (Parallel Agents)

### Step 1: Launch Implementation Agents

Launch ALL "ready now" agents in a **single message** (parallel, background):

**For BUG shards** (title starts with `fix:`):
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

  After implementing the fix, verify it addresses the PIPELINE-LEVEL issue:
  1. Does the fix change the actual code path the workflow calls? (not just a
     helper function that's never invoked)
  2. Does the fix handle existing/stale data? (not just new records)
  3. Does the fix degrade gracefully when upstream stages fail?

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

**For REQUIREMENT shards** (title starts with `feat:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Feat: [short title]",
  prompt="You have been assigned shard pf-feat-xxx.

  ## Setup
  1. Read your assignment:
     /Users/dev/bin/palace task get pf-feat-xxx

  2. Claim the work:
     /Users/dev/bin/palace task claim pf-feat-xxx

  ## File Scope
  IMPORTANT: Only modify/create files listed in your shard's 'Files to Modify/Create' section.
  Files: [files from shard]

  ## Implementation
  3. Read the existing pattern referenced in the shard — follow its structure closely
  4. Check shard progress notes for acceptance test details (test names and file)
  5. If acceptance tests exist, run them FIRST to confirm they fail:
     go test ./path/to/... -run TestName -v
  6. Implement the feature described in the shard, following the approach and pattern specified
  7. Run the acceptance tests again — they MUST now pass
  8. Run the full test suite for affected packages:
     go build ./path/to/...
     go test ./path/to/... -v

  IMPORTANT: Follow the existing pattern closely. Don't over-engineer or add capabilities
  beyond what the acceptance criteria specify.
  IMPORTANT: Do NOT report completion unless build and ALL tests pass.
  IMPORTANT: If there are no acceptance tests noted in shard progress, write them yourself.

  ## Completion
  9. Log progress:
     /Users/dev/bin/palace task progress pf-feat-xxx 'Implemented: [summary]'

  10. Close the shard:
     /Users/dev/bin/palace task close pf-feat-xxx 'Done: [summary]. Tests: [TestNames]'

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
WHERE id IN ('pf-fix-aaa', 'pf-feat-bbb')
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

### Step 5b: Trace Back to Original Message

Follow edges from impl shard -> investigation/analysis -> original message:

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

### Step 5c: Reply to Penfold

**Group replies by original message.** If one message contained 3 items (mix of bugs and
requirements), send ONE resolution reply covering all resolved items from that message.

Wait until ALL items from a message are resolved before replying. If some items from a
message are still in progress, hold the reply until the full batch completes.

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

### Step 5d: Close Analysis/Investigation Shards

If any investigation or analysis shards are still open, close them:

```bash
/Users/dev/bin/palace task close pf-inv-xxx "Investigation complete, fix deployed"
/Users/dev/bin/palace task close pf-anl-xxx "Analysis complete, feature deployed"
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
  AND (s.title LIKE 'fix:%' OR s.title LIKE 'feat:%')
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

# 4. Commit — use appropriate prefix based on content
#    If mixed bugs+features: "fix+feat: ..."
#    If only bugs: "fix: ..."
#    If only features: "feat: ..."
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

Display a per-item summary table. For EACH item processed, include all of these fields:

```
INGEST PIPELINE - COMPLETE
════════════════════════════

## Bug 1: [short title]
Shard:       pf-fix-aaa (investigation: pf-inv-aaa)
Bug:         [1-2 sentence summary of the reported symptom and root cause]
Fix:         [1-2 sentence summary of what was changed to resolve it]
Test:        [TestName] in [file] — FAILED before fix ✓, PASSED after fix ✓
             [or: "Not directly testable — [reason]. Closest test: [TestName]"]
             [or: "Fixed existing test [TestName] — had [what was wrong]"]
Deploy:      [Gateway deployed ✓ VERIFIED RUNNING | Worker deployed ✓ VERIFIED RUNNING | CLI v0.X.Y released ✓ | None needed]
Notified:    agent-penfold replied ✓ [user action required: "run penf update" | no action needed]

## Feature 1: [short title]
Shard:       pf-feat-bbb (analysis: pf-anl-bbb)
Requirement: [1-2 sentence summary of what was requested]
Built:       [1-2 sentence summary of what was implemented]
Test:        [TestName1, TestName2] in [file] — acceptance tests PASS ✓
Deploy:      [details — must include VERIFIED RUNNING for each service]
Notified:    agent-penfold replied ✓ [user action required: "run penf update" | no action needed]

────────────────────────
Totals: B bugs fixed, R features built, N deployed, N replies sent
Commit: [hash] "[message]"

Deployment verification:
  Gateway:  RUNNING ✓ (health check passed)
  Worker:   RUNNING ✓ (health check passed)
  CLI:      v0.X.Y released ✓
```

**Rules for the summary:**
- Every item MUST have all 6 fields (Shard, Bug/Requirement, Fix/Built, Test, Deploy, Notified)
- Test field for bugs must confirm both pre-fix failure AND post-fix pass (or explain why not testable)
- Test field for features must confirm acceptance tests pass after implementation
- Deploy field must list every deployment action taken AND confirm "VERIFIED RUNNING" for each
  service, or "None needed" if code-only. Never say just "deployed" without verification.
- Notified field must confirm the reporter was told, and whether they need to do anything
  (e.g. "run `penf update`" for CLI changes, "no action needed" for server-side fixes)
- If an item was partially fixed or deferred, say so explicitly with next steps
- If deployment verification failed, the summary MUST say so — do not mark as complete

---

## Error Handling

### Debugger Agent Fails
- Read background output file for error details
- Re-launch with more context or different approach
- If investigation is inconclusive, close shard with partial findings and mark impl as "needs manual investigation"

### Explorer Agent Fails
- Read background output file for error details
- Re-launch with more specific search guidance
- If analysis is inconclusive, close shard with partial findings and the orchestrator
  fills in the gaps based on the requirement text and its own codebase knowledge

### Implementation Agent Fails
- Check shard status and agent output
- If build failure: re-launch with error output
- If test failure: re-launch with failing test details
- If stuck: close shard, create a new one with refined instructions

### No Actionable Items Found
If inbox has no bugs or requirements:
```
INGEST PIPELINE - Phase 1: Pull & Classify
════════════════════════════════════════════
No actionable items found in inbox.

Use /bug-status to check existing pipeline state.
```

### Partial Completion
If some implementations succeed and others fail:
- Deploy what's ready
- Reply to penfold for resolved items
- Leave failed items in queue for `/implement-next`

## Key Principles

1. **NEVER write code yourself** - always delegate to sub-agents
2. **Maximize parallelism** - launch all independent agents simultaneously (Phase 2 bugs
   and Phase 2R requirements can run in parallel too)
3. **Auto-continue** - don't stop between phases unless there's an error
4. **Reply to penfold** - every item gets an ack and a resolution reply
5. **Deploy changes** - implementation isn't done until deployed
6. **Trace edges** - maintain the chain: message -> investigation/analysis -> implementation
7. **Bugs investigate, requirements analyze** - don't waste time debugging something that
   simply doesn't exist yet

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

### Anti-Pattern 5: Acceptance Tests That Test Existing Behavior (Requirements Only)

**BAD:** Test for "add --format json flag" checks that the command runs without error.
**REALITY:** The command already runs without error — it just doesn't support --format json.
The test passes without any implementation.

**Rule:** Acceptance tests for requirements must test the **new** behavior specifically.
If the feature is a new flag, the test must verify the flag is recognized AND produces
the expected output format. If the test passes before implementation, it's testing the
wrong thing.
