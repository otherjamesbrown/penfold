---
description: "Phase 3.5: Write failing tests before implementation. All complexity levels get test-first treatment."
---

# Ingest — Phase 3.5: Write Failing Tests

Write tests that FAIL now and will PASS after implementation. This applies to ALL items.
- **Bugs:** Reproduction test proving the bug exists
- **Requirements/Specs:** Acceptance test defining the desired behavior
- **HIGH items:** Tests are written per wave, before each implementation wave

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
CP_CLI: cp
```

## Step 1: Assess Testability

For each implementation shard (or sub-shard for HIGH items), decide:

- **Testable** — The behavior can be verified by a unit or integration test
- **Not directly testable** — Requires live infrastructure, timing, network conditions.
  Examples: connection timeouts to external services, race conditions, Nomad deployment.

If **not directly testable**: skip this shard, add a note to the impl shard explaining why.

## Step 2: Launch Test-Writing Agents

### For LOW/MEDIUM Items

Launch ALL testable agents in a **single message** (parallel, background):

**For BUG shards** (title starts with `fix:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Test: [short title]",
  prompt="Write a reproduction test for bug pf-fix-xxx.

  ## Setup
  1. cp knowledge show mycroft-dev-index — mandatory project context
  2. cp knowledge show mycroft-agent-<agent-type> — your domain context

  ## Bug Summary
  [paste root cause and symptom from investigation/impl shard]

  ## Files Affected
  [files from shard]

  ## Your Task

  ### Step 0: Check for Existing Tests
  Look for existing tests that SHOULD have caught this bug:
  - Search test files in the same package as the affected code
  - Look for tests covering the same function/method/handler
  - Check if there is a test that tests this path but has an error

  ### Step 1: Fix or Create Test
  - **If an existing test should catch this but doesn't:** Fix it.
  - **If no relevant test exists:** Write a new focused test.

  ### Step 2: Validate the Test Catches the Bug
  Run the test and confirm it **FAILS** against the current (unfixed) code:
    go test ./path/to/... -run TestName -v
  The test MUST fail. A passing test means it does not reproduce the bug — revise it.

  ### Step 3: Report
  Output: test file, test name, whether fix or new, confirmation it fails.

  CRITICAL: Do NOT fix the bug itself. Only write/fix the test.
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.

  ## Context Budget
  If you are running low on context, prioritize: (1) write the test, (2) confirm it fails,
  (3) report. Skip searching for existing tests if budget is tight.")
```

**For REQUIREMENT/SPEC shards** (title starts with `feat:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Test: [short title]",
  prompt="Write acceptance tests for requirement pf-feat-xxx.

  ## Setup
  1. cp knowledge show mycroft-dev-index — mandatory project context
  2. cp knowledge show mycroft-agent-<agent-type> — your domain context

  ## Requirement Summary
  [paste goal and acceptance criteria from impl shard]

  ## Approach
  [paste approach and pattern to follow from impl shard]

  ## Files Affected
  [files from shard]

  ## Your Task

  ### Step 0: Understand the Pattern
  Read the existing pattern referenced in the shard. Understand how similar features
  are tested in this codebase.

  ### Step 1: Write Acceptance Tests
  Write tests that verify each acceptance criterion. Tests should:
  - Follow existing test patterns in the package
  - Test the PUBLIC interface (CLI output, gRPC response, function return)
  - Cover happy path AND key edge cases

  ### Step 2: Validate Tests Fail
  Run and confirm they **FAIL** because the feature doesn't exist yet:
    go test ./path/to/... -run TestName -v
  Compile failures count as valid failures.

  ### Step 3: Report
  Output: test file, test names, what each verifies, confirmation they fail.

  CRITICAL: Do NOT implement the feature. Only write the tests.
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.

  ## Context Budget
  If you are running low on context, prioritize: (1) write the most important test (happy
  path), (2) confirm it fails, (3) report. Skip edge case tests if budget is tight.")
```

### For HIGH Items (Per-Wave Test-First)

HIGH items are decomposed into layer sub-shards (DB → Service → CLI). For each wave,
write the test BEFORE launching the implementation agent for that wave.

**Wave sequence for each HIGH feature:**
```
Wave 1: Write DB layer tests → Implement DB layer → Verify
Wave 2: Write Service layer tests → Implement Service layer → Verify
Wave 3: Write CLI layer tests → Implement CLI layer → Verify
```

For each wave's sub-shard, launch a test-writing agent:
```
Task(subagent_type="<agent-type-for-this-layer>", run_in_background=true,
  description="Test: [layer] [feature name]",
  prompt="Write failing tests for sub-shard pf-xxx-layer.

  ## Setup
  1. cp knowledge show mycroft-dev-index — mandatory project context
  2. cp knowledge show mycroft-agent-<agent-type> — your domain context

  ## Sub-Shard Summary
  [paste the sub-shard's Goal, Scope, and Acceptance Criteria]

  ## Layer: [DB / Service / CLI]

  [If not Wave 1:]
  Previous layer output is already in the codebase. Read these files to understand
  the types and interfaces available:
  [list files from previous layer's completed sub-shard]

  ## Your Task

  ### Step 1: Write Layer Tests
  Write tests that verify the acceptance criteria for THIS layer only:
  - **DB layer:** Test repository methods — CRUD, edge cases, empty results
  - **Service layer:** Test gRPC handlers — request/response, validation, error cases
  - **CLI layer:** Test command output — format, flags, help text

  Follow existing test patterns in the package.

  ### Step 2: Validate Tests Fail
  Run and confirm they **FAIL** because the layer doesn't exist yet:
    go test ./path/to/... -run TestName -v
  Compile failures count as valid failures.

  ### Step 3: Report
  Output: test file, test names, what each verifies, confirmation they fail.

  CRITICAL: Do NOT implement the feature. Only write tests for THIS layer.
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.

  ## Context Budget
  If you are running low on context, prioritize: (1) write the most important test per
  acceptance criterion, (2) confirm it fails, (3) report.")
```

**IMPORTANT:** The orchestrator must wait for each wave's tests to be written before
launching that wave's implementation agent. The test agent and implementation agent for
the same wave run sequentially, NOT in parallel.

## Step 3: Monitor and Validate

For each completed agent, verify:
1. A test file was created or modified
2. The test was run and **failed**

If an agent's test **passes** (wrong — should fail):
- **Bug:** Re-launch: "Your test passed — it does not reproduce the bug. Review the root
  cause and write a test that exercises the specific failing path."
- **Requirement:** Re-launch: "Your test passed — it does not verify the new behavior.
  The feature doesn't exist yet, so the test should fail."

## Step 4: Update Implementation Shards

```bash
# For bugs:
cp task progress pf-fix-xxx 'Reproduction test ready: [TestName] in [file]. Test FAILS on current code.'

# For requirements/specs:
cp task progress pf-feat-xxx 'Acceptance tests ready: [TestName1, TestName2] in [file]. Tests FAIL.'

# For HIGH sub-shards:
cp task progress pf-xxx-layer 'Layer tests ready: [TestName1, TestName2] in [file]. Tests FAIL.'
```

## Show Progress

```
INGEST PIPELINE - Phase 3.5: Tests
════════════════════════════════════
LOW/MEDIUM:
  pf-fix-aaa  | [title] | BUG  | TESTABLE | TestName (FAILS ✓)
  pf-feat-bbb | [title] | SPEC | TESTABLE | TestName1, TestName2 (FAIL ✓)
  pf-fix-ccc  | [title] | BUG  | SKIP     | Not testable: [reason]

HIGH (per-wave, tests precede each implementation wave):
  Feature: [name]
    Wave 1 | pf-xxx-db  | DB tests     | TestRepo_Create, TestRepo_List (FAIL ✓)
    Wave 2 | pf-xxx-svc | Svc tests    | (pending — after Wave 1 implementation)
    Wave 3 | pf-xxx-cli | CLI tests    | (pending — after Wave 2 implementation)
```

## Test Quality Anti-Patterns

**Do NOT let agents fall into these traps:**

1. **Testing functions instead of pipelines** — Test through the same entry point the
   pipeline uses, not the fixed function directly with ideal inputs.

2. **Testing with fresh state** — Set up pre-existing records that simulate production
   (stale DB records, previously-created entities), not empty/clean state.

3. **Assuming upstream stages succeed** — If the bug manifests when an upstream stage fails,
   simulate that failure (nil input, empty output, timeout).

4. **Misattributing root cause** — Before blaming an external service, verify framework
   config (timeouts, retries, heartbeats).

5. **Testing existing behavior (requirements)** — If the test passes before implementation,
   it's testing the wrong thing. The feature doesn't exist yet.

6. **Duplicate helpers in shared packages** — Before defining any helper in `cmd/penf/cmd/`,
   search for existing definitions. Two agents defining `truncate()` breaks the build.

7. **Writing tests that only check "no error"** — Tests must assert specific output values,
   not just `assert.NoError`. A function that returns nil,nil passes "no error" but is wrong.

8. **Ignoring the spec's acceptance criteria** — For SPEC items, each acceptance criterion
   from the original spec should map to at least one test assertion. Don't invent your own
   criteria — the spec is the source of truth.

After displaying progress, return to the orchestrator.
