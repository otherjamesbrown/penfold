---
description: "Phase 3.5: Write failing tests for LOW/MEDIUM items before implementation. HIGH items have tests in layer sub-shards."
---

# Ingest — Phase 3.5: Write Failing Tests

**This phase runs ONLY for LOW/MEDIUM complexity items.** HIGH complexity items have tests
embedded in each layer sub-shard instead (written during Phase 4).

Write a test that FAILS now and will PASS after implementation.
- **Bugs:** Reproduction test proving the bug exists
- **Requirements:** Acceptance test defining the desired behavior

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## Step 1: Assess Testability

For each implementation shard, decide:

- **Testable** — The behavior can be verified by a unit or integration test
- **Not directly testable** — Requires live infrastructure, timing, network conditions.
  Examples: connection timeouts to external services, race conditions, Nomad deployment.

If **not directly testable**: skip this shard, add a note to the impl shard explaining why.

## Step 2: Launch Test-Writing Agents

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
  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.")
```

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
/Users/dev/bin/palace task progress pf-fix-xxx 'Reproduction test ready: [TestName] in [file]. Test FAILS on current code.'

# For requirements:
/Users/dev/bin/palace task progress pf-feat-xxx 'Acceptance tests ready: [TestName1, TestName2] in [file]. Tests FAIL.'
```

## Show Progress

```
INGEST PIPELINE - Phase 3.5: Tests (LOW/MEDIUM only)
════════════════════════════════════════════════════
pf-fix-aaa  | [title] | BUG  | TESTABLE | TestName (FAILS ✓)
pf-feat-bbb | [title] | REQ  | TESTABLE | TestName1, TestName2 (FAIL ✓)
pf-fix-ccc  | [title] | BUG  | SKIP     | Not testable: [reason]

HIGH items: tests embedded in layer sub-shards (Phase 4)
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

After displaying progress, return to the orchestrator.
