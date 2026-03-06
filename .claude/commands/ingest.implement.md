---
description: "Phase 4: Launch implementation agents. Single agent for LOW/MEDIUM, layer-by-layer waves for HIGH."
---

# Ingest — Phase 4: Implement

Two execution modes based on complexity routing from Phase 3.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
CP_CLI: cp
MAX_RETRIES: 3
```

## Agent Context Requirements

**Every implementation agent MUST read the project context docs before starting work.**
Include these instructions in every agent prompt:

```
## Setup (MANDATORY — do this before any code changes)
1. cxp knowledge show mycroft-dev-index — project architecture, conventions, patterns
2. cxp knowledge show mycroft-agent-<agent-type> — your domain-specific context
3. Read your assignment: cxp task get pf-xxx
4. Claim the work: cxp task claim pf-xxx
```

## Coding Standards (Include in All Prompts)

Include these standards in every implementation agent prompt:

```
## Coding Standards
- Error handling: Return errors up the call stack. Use fmt.Errorf("context: %w", err) for wrapping.
  Do NOT log and return — either log OR return, not both.
- Logging: Use structured logging (slog). Include relevant IDs (tenant_id, content_id, entity_id).
- Naming: Follow existing codebase conventions. Check nearby files for patterns.
- Comments: Only where the logic isn't obvious. No redundant comments on clear code.
- Tests: Table-driven tests preferred. Test happy path + error cases + edge cases.
- Imports: Group stdlib, external, internal. Use goimports formatting.
- API stability: Do NOT change existing function signatures. If you need additional parameters,
  add them with defaults, create a new function, or use an options struct. Changing a signature
  cascades to all callers and wastes context fixing compile errors.
```

## Context Budget Warning (Include in All Prompts)

Include this in every implementation agent prompt:

```
## Context Budget
You have a limited context window. Prioritize in this order:
1. Complete the implementation (working code that compiles)
2. Make all tests pass (both pre-existing and new)
3. Clean up (formatting, comments, imports)

If you are running low on context:
- STOP adding features. Finish what you have.
- Ensure go build passes.
- Close the shard with a progress note: "Implemented: [what's done]. Remaining: [what's left]."
- Do NOT defer tests — if you can't write tests, say so explicitly in the close message.

BAIL-OUT RULE: If you hit cascading compile errors (e.g. a signature change broke 5+ callers),
STOP immediately. Do NOT grind through fixing caller after caller. Instead:
- Revert the signature change
- Find a backwards-compatible approach (add new function, use defaults, options struct)
- If you cannot find one, close the shard with: "BLOCKED: [explanation]. Needs orchestrator guidance."
```

---

## Implementation Loop

Two retry mechanisms, depending on execution mode:

### Mode: Dispatched Sessions (Agent Factory)

When work is dispatched as a standalone Claude session (via dispatch-agent or the poller),
use the **ralph-loop plugin** for session-level retry:

```bash
/ralph-loop "Run /ingest.implement pf-XXX. Follow it exactly. Output <promise>DONE</promise> when complete, or <promise>BLOCKED</promise> if stuck." --max-iterations 5 --completion-promise "DONE"
```

The stop hook blocks session exit and re-feeds the prompt. Each iteration sees the
previous attempt's file changes and can course-correct with full context.

### Mode: Sub-Agent (Orchestrator Retry)

When launched as sub-agents via the Task tool (current model), the orchestrator manages
retries externally. Same prompt, fresh context, persistent code changes.

**Why this matters:** Sub-agents that fail on the first attempt have already modified files.
A fresh sub-agent sees those changes, plus the error output from the previous attempt, and
can course-correct with a full context window. This is fundamentally better than retrying
within a depleted context.

**Loop logic (applied to EVERY sub-agent):**

```
for attempt in 1..MAX_RETRIES:
    launch sub-agent (fresh context)
    wait for completion

    ## TWO-SIGNAL VERIFICATION (both must agree)
    # Signal 1: Independent build/test check (PRIMARY — ground truth)
    run: go build ./path/to/... && go test ./path/to/... -v
    # Signal 1b: If Penfold acceptance test exists, run it too (must also pass)
    run: go test -tags=quality -run TestQuality/NNN ./tests/quality/ (or e2e equivalent)
    # Signal 2: Shard close reason prefix (SECONDARY — agent's self-report)
    read shard close reason

    # Classification:
    if build+tests PASS and close reason starts with "DONE:" → SUCCESS, break
    if close reason starts with "BLOCKED:" → ESCALATE to penfold immediately, break
    if build+tests FAIL (regardless of close reason) → FAILED, enter retry
    if build+tests PASS but close reason is not "DONE:" → treat as SUCCESS with warning
        (log: "Agent used non-standard close prefix. Verified independently.")

    # Retry path:
    if attempt < MAX_RETRIES:
        capture: build errors, test failures, close reason
        re-open shard: cxp task reopen pf-xxx
        add progress note:
          cxp task progress pf-xxx "Attempt [N] failed: [error summary]. Retrying with fresh agent."
        continue loop
    else:
        ESCALATE to penfold — all retries exhausted
```

**Why two signals:** Sub-agents may use non-standard prefixes ("Completed:", "Implemented:").
Build and test results are ground truth. The close reason is useful context for retries but
should never be the sole determinant of success or failure.

**Retry prompt additions:** When re-launching (attempt > 1), prepend this to the sub-agent prompt:

```
## RETRY CONTEXT — Attempt [N] of [MAX_RETRIES]

Previous attempt failed. The previous agent's code changes are already in the files.

**Previous failure:**
[paste closed_reason or error output from previous attempt]

**What to do:**
1. Read the files the previous agent modified — understand what was done
2. Run `go build` and `go test` to see the current state
3. Identify what went wrong (don't assume the previous approach was correct)
4. Fix the issue — you may need to revise the approach, not just patch
5. Verify everything passes before closing
```

**IMPORTANT:** Each retry is a FRESH sub-agent with full context budget. Do not ask the
same agent to retry within its session — that wastes context. Kill and re-launch.

---

## Mode A: Single Agent (LOW/MEDIUM Complexity)

**Batch size: max 4 agents at a time.** If more than 4 items are ready, launch in batches
of 4, wait for the batch to complete, then launch the next. This prevents context pressure
from all outputs arriving simultaneously.

Launch each batch in a **single message** (parallel, background):

**For BUG shards** (title starts with `fix:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Fix: [short title]",
  prompt="You have been assigned shard pf-fix-xxx.

  ## Setup (MANDATORY)
  1. cxp knowledge show mycroft-dev-index — project architecture and conventions
  2. cxp knowledge show mycroft-agent-<agent-type> — your domain context
  3. Read your assignment: cxp task get pf-fix-xxx
  4. Claim the work: cxp task claim pf-fix-xxx

  ## Coding Standards
  - Error handling: Return errors up the call stack. Use fmt.Errorf('context: %w', err).
    Do NOT log and return — either log OR return, not both.
  - Logging: Use structured logging (slog). Include relevant IDs.
  - Naming: Follow existing codebase conventions. Check nearby files.
  - Tests: Table-driven tests preferred. Happy path + error + edge cases.
  - API stability: Do NOT change existing function signatures. Add new params with defaults or create new functions.

  ## File Scope
  IMPORTANT: Only modify files listed in your shard's 'Files to Modify' section.
  Only modify: [files from shard]

  ## Implementation
  5. Read existing code patterns in the affected files
  6. Check shard progress notes for reproduction test details
  7. Run the reproduction test FIRST to confirm it fails:
     go test ./path/to/... -run TestName -v
  8. Check if a Penfold acceptance test exists (look for 'Penfold Acceptance Test'
     section in the shard). If so, run it to confirm it also fails:
     [test command from shard — e.g. go test -tags=quality -run TestQuality/011 ./tests/quality/]
  9. Implement the fix described in the shard
  10. Run the reproduction test again — it MUST now pass
  11. If a Penfold acceptance test exists, run it again — it MUST now pass.
      Do NOT modify the Penfold test. If it still fails, your fix is incomplete.
  12. Run the full test suite for affected packages:
      go build ./path/to/...
      go test ./path/to/... -v

  Cross-layer verification:
  1. Does the fix change the actual code path the workflow calls?
  2. Does the fix handle existing/stale data?
  3. Does the fix degrade gracefully when upstream stages fail?
  4. For visibility/wiring bugs: does the field appear in CLI output?
     Trace: DB → repository struct → gRPC service mapping → proto →
     CLI client struct → CLI command struct → JSON/text output.
     If ANY layer in this chain is missing the field, the fix is incomplete.

  IMPORTANT: Do NOT report completion unless build and ALL tests pass (including Penfold test).
  IMPORTANT: Do NOT modify Penfold-provided tests. They define what 'fixed' looks like.
  IMPORTANT: If there is no reproduction test, write one yourself.

  ## Write Implementation Report
  13. Before closing, write a report to /tmp/report-pf-fix-xxx-impl.md covering:
      - Approach taken and why
      - Files modified with summary of changes per file
      - Challenges encountered and how they were resolved
      - Test results: full output of test run (pass/fail)
      - If Penfold test exists: its output
      - Cross-layer verification results

  14. Create a report shard and link as child:
      ```bash
      cxp shard create --type report \
        --title 'report: [agent-type] phase-4 attempt-[N] — pf-fix-xxx' \
        --body-file /tmp/report-pf-fix-xxx-impl.md
      cxp shard link pf-REPORT-ID --child-of pf-fix-xxx
      ```

  ## Completion — EXACT FORMAT REQUIRED
  15. cxp task progress pf-fix-xxx 'Implemented: [summary]'
  16. Close with one of these EXACT prefixes:
      - SUCCESS: cxp task close pf-fix-xxx 'DONE: [summary]. Tests: [TestNames]. Penfold test: [PASS/N/A]. Files modified: [list]. REPORT: pf-REPORT-ID'
      - STUCK:   cxp task close pf-fix-xxx 'BLOCKED: [what is blocking]. Needs: [what you need]. REPORT: pf-REPORT-ID'
      - PARTIAL: cxp task close pf-fix-xxx 'FAILED: [what went wrong]. Done so far: [what works]. Remaining: [what is left]. REPORT: pf-REPORT-ID'
      Use DONE/BLOCKED/FAILED exactly — the orchestrator classifies your result by this prefix.

  ## Context Budget
  Prioritize: (1) working fix that compiles, (2) tests pass (including Penfold test), (3) create report shard.
  If running low, close with FAILED prefix listing what's done and what remains.

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.
  Only modify files. The orchestrator handles all git operations.")
```

**For REQUIREMENT/SPEC shards** (title starts with `feat:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Feat: [short title]",
  prompt="You have been assigned shard pf-feat-xxx.

  ## Setup (MANDATORY)
  1. cxp knowledge show mycroft-dev-index — project architecture and conventions
  2. cxp knowledge show mycroft-agent-<agent-type> — your domain context
  3. Read your assignment: cxp task get pf-feat-xxx
  4. Claim the work: cxp task claim pf-feat-xxx

  ## Coding Standards
  - Error handling: Return errors up the call stack. Use fmt.Errorf('context: %w', err).
    Do NOT log and return — either log OR return, not both.
  - Logging: Use structured logging (slog). Include relevant IDs.
  - Naming: Follow existing codebase conventions. Check nearby files.
  - Tests: Table-driven tests preferred. Happy path + error + edge cases.
  - API stability: Do NOT change existing function signatures. Add new params with defaults or create new functions.

  ## File Scope
  IMPORTANT: Only modify/create files listed in your shard's 'Files to Modify/Create'.
  Files: [files from shard]

  ## Implementation
  5. Read the existing pattern referenced in the shard — follow it closely
  6. Check shard progress notes for acceptance test details
  7. If acceptance tests exist, run them FIRST to confirm they fail
  8. Implement the feature following the approach and pattern specified
  9. Run acceptance tests again — they MUST now pass
  10. Run the full test suite:
      go build ./path/to/...
      go test ./path/to/... -v

  ## Acceptance Criteria Check
  Before closing, verify EVERY acceptance criterion in the shard is satisfied.
  If the shard came from a SPEC, the acceptance criteria are the sender's exact
  requirements — do not skip any.

  IMPORTANT: Follow the existing pattern closely. Don't over-engineer.
  IMPORTANT: Do NOT report completion unless build and ALL tests pass.
  IMPORTANT: If there are no acceptance tests, write them yourself.

  ## Write Implementation Report
  11. Before closing, write a report to /tmp/report-pf-feat-xxx-impl.md covering:
      - Approach taken and rationale
      - Files modified/created with summary of changes per file
      - How existing pattern was followed (or diverged, with reasoning)
      - Challenges encountered and how they were resolved
      - Test results: full output of test run (pass/fail)
      - Acceptance criteria checklist with pass/fail per criterion

  12. Create a report shard and link as child:
      ```bash
      cxp shard create --type report \
        --title 'report: [agent-type] phase-4 attempt-[N] — pf-feat-xxx' \
        --body-file /tmp/report-pf-feat-xxx-impl.md
      cxp shard link pf-REPORT-ID --child-of pf-feat-xxx
      ```

  ## Completion — EXACT FORMAT REQUIRED
  13. cxp task progress pf-feat-xxx 'Implemented: [summary]'
  14. Close with one of these EXACT prefixes:
      - SUCCESS: cxp task close pf-feat-xxx 'DONE: [summary]. Tests: [TestNames]. Acceptance criteria: [list which are met]. Files modified: [list]. REPORT: pf-REPORT-ID'
      - STUCK:   cxp task close pf-feat-xxx 'BLOCKED: [what is blocking]. Needs: [what you need]. REPORT: pf-REPORT-ID'
      - PARTIAL: cxp task close pf-feat-xxx 'FAILED: [what went wrong]. Done so far: [what works]. Remaining: [what is left]. REPORT: pf-REPORT-ID'
      Use DONE/BLOCKED/FAILED exactly — the orchestrator classifies your result by this prefix.

  ## Context Budget
  Prioritize: (1) working implementation that compiles, (2) tests pass, (3) create report shard.
  If running low, close with FAILED prefix listing what's done and what remains.

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.
  Only modify files. The orchestrator handles all git operations.")
```

---

## Mode B: Layer-by-Layer (HIGH Complexity — Decomposed)

Execute sub-shards in **dependency waves**. Within each wave, sub-shards from different
features can run in parallel. Within a single feature, layers execute sequentially.

**CRITICAL:** For each wave, the test-writing step (Phase 3.5) runs BEFORE implementation.
The sequence per wave is: write tests → implement → verify.

### Wave Execution

```
Wave 1: Write DB tests → Implement DB sub-shards (feat-db:) — no dependencies
  → pf-featA-db + pf-featB-db in parallel
  → Wait → Verify: go build ./pkg/... && go test ./pkg/... -v

Wave 2: Write Service tests → Implement Service sub-shards (feat-svc:) — depend on DB
  → pf-featA-svc + pf-featB-svc in parallel
  → Wait → Verify: go build ./services/gateway/... && go test ./services/gateway/... -v

Wave 3: Write CLI tests → Implement CLI sub-shards (feat-cli:) — depend on Service
  → pf-featA-cli + pf-featB-cli in parallel
  → Wait → Verify: go build ./cmd/penf/...

Wave 4: Write Pipeline tests → Implement Pipeline sub-shards (feat-pipe:) — depend on DB
  → In parallel
  → Wait → Verify: go build ./services/worker/... && go test ./services/worker/... -v
```

**CRITICAL: Verify between waves.** If a wave's build or tests fail:
- Small issues (missing import, typo): fix directly
- Larger issues: apply the retry loop (Monitor & Retry section) — re-launch with fresh agent
- Do NOT proceed to the next wave until the current one passes
- Each sub-shard gets up to MAX_RETRIES fresh attempts before escalating

### Launch Sub-Shard Agents

For each sub-shard in the current wave:
```
Task(subagent_type="<agent-type-from-sub-shard>", run_in_background=true,
  description="[Layer]: [feature name]",
  prompt="You have been assigned sub-shard pf-xxx-layer.

  ## Setup (MANDATORY)
  1. cxp knowledge show mycroft-dev-index — project architecture and conventions
  2. cxp knowledge show mycroft-agent-<agent-type> — your domain context
  3. Read your assignment: cxp task get pf-xxx-layer
  4. Claim the work: cxp task claim pf-xxx-layer

  ## Coding Standards
  - Error handling: Return errors up the call stack. Use fmt.Errorf('context: %w', err).
    Do NOT log and return — either log OR return, not both.
  - Logging: Use structured logging (slog). Include relevant IDs.
  - Naming: Follow existing codebase conventions. Check nearby files.
  - Tests: Table-driven tests preferred. Happy path + error + edge cases.
  - API stability: Do NOT change existing function signatures. Add new params with defaults or create new functions.

  ## Context
  This is ONE LAYER of a larger feature. You are implementing ONLY the [layer] layer.
  Other layers are handled by other agents in sequence.

  [If not Wave 1:]
  Previous layer output is already in the codebase. Read these files to understand
  the types and interfaces you need to use:
  [list files from previous layer's sub-shard]

  ## File Scope
  IMPORTANT: Only modify/create files listed in your sub-shard.
  Files: [files from sub-shard]

  ## Implementation
  5. Read the existing pattern referenced in the shard
  6. Check shard progress notes for layer test details
  7. If layer tests exist (from Phase 3.5), run them FIRST to confirm they fail
  8. Implement the scope described in the sub-shard
  9. Run layer tests again — they MUST now pass
  10. Verify build and tests pass:
      go build ./path/to/...
      go test ./path/to/... -v

  ## Acceptance Criteria Check
  Before closing, verify EVERY acceptance criterion in the sub-shard is satisfied.

  IMPORTANT: Follow the existing pattern closely.
  IMPORTANT: Do NOT report completion unless build and tests pass.
  IMPORTANT: Write tests for every method/handler you create, even if Phase 3.5 tests
  don't cover everything.

  ## Shared Package Warning (CLI agents only)
  The cmd/penf/cmd/ package is shared. Before defining ANY helper function, search
  the package for existing definitions:
    grep -r 'func functionName' cmd/penf/cmd/
  Use existing helpers. Do NOT redefine them.

  ## Write Implementation Report
  7. Before closing, write a report to /tmp/report-pf-xxx-layer-impl.md covering:
     - Approach taken for this layer
     - Files modified/created with summary of changes per file
     - How existing pattern was followed
     - Challenges encountered and resolutions
     - Test results: full output of test run (pass/fail)
     - Layer acceptance criteria checklist with pass/fail

  8. Create a report shard and link as child:
     ```bash
     cxp shard create --type report \
       --title 'report: [agent-type] phase-4 attempt-[N] — pf-xxx-layer' \
       --body-file /tmp/report-pf-xxx-layer-impl.md
     cxp shard link pf-REPORT-ID --child-of pf-xxx-layer
     ```

  ## Completion — EXACT FORMAT REQUIRED
  9. Close with one of these EXACT prefixes:
     - SUCCESS: cxp task close pf-xxx-layer 'DONE: [summary]. Tests: [TestNames]. Files: [files modified]. REPORT: pf-REPORT-ID'
     - STUCK:   cxp task close pf-xxx-layer 'BLOCKED: [what is blocking]. Needs: [what you need]. REPORT: pf-REPORT-ID'
     - PARTIAL: cxp task close pf-xxx-layer 'FAILED: [what went wrong]. Done so far: [what works]. Remaining: [what is left]. REPORT: pf-REPORT-ID'
     Use DONE/BLOCKED/FAILED exactly — the orchestrator classifies your result by this prefix.

  ## Context Budget
  Prioritize: (1) working code that compiles, (2) all tests pass, (3) create report shard.
  If running low, close with FAILED prefix listing what's done and what remains.
  Do NOT defer tests — say explicitly if tests are incomplete.

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.")
```

---

## Monitor & Retry Loop (Both Modes)

After launching a batch, monitor completion and apply the retry loop:

### Step 1: Wait for Completion

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, status, closed_reason
FROM shards
WHERE id IN ('pf-fix-aaa', 'pf-feat-bbb', 'pf-xxx-db', 'pf-xxx-svc')
ORDER BY created_at;
"
```

Also use `Read` on background agent output files.
Wait until all current-batch shards/sub-shards are closed.

### Step 2: Classify Results

For each completed shard, read `closed_reason` and classify:

| Close Reason Starts With | Result | Action |
|--------------------------|--------|--------|
| `Done:` | SUCCESS | No retry needed |
| `BLOCKED:` | BLOCKED | Escalate to penfold immediately — do NOT retry |
| Anything else | FAILED | Enter retry loop |

### Step 3: Retry Failed Shards

For each FAILED shard (attempt < MAX_RETRIES):

1. **Capture the failure:** Read the shard's `closed_reason` and any progress notes
2. **Re-open the shard:**
   ```bash
   cxp task reopen pf-xxx
   cxp task progress pf-xxx "Attempt [N] failed: [1-line error summary]. Retrying with fresh agent."
   ```
3. **Re-launch with retry context:** Launch a NEW sub-agent using the same prompt template
   (Mode A or Mode B as appropriate), but prepend the retry context block from the
   "Implementation Loop" section above
4. **Wait and re-classify** — repeat until success or MAX_RETRIES exhausted

**Batch retries:** If multiple shards in a batch failed, re-launch them all in parallel
(same batch-size rules apply). Don't serialize retries unnecessarily.

### Step 4: Escalate After MAX_RETRIES

Only mark the shard as blocked when all retry attempts are exhausted:

```bash
# Add blocked label so it appears in the board's attention section
cxp shard label add pf-xxx blocked

# Update shard content with retry history
cxp task progress pf-xxx "Retries exhausted (N attempts). Attempt 1: [summary]. Attempt 2: [summary]. Attempt 3: [summary]. Pattern: [what kept going wrong]. Current state: [builds? which tests fail?]. Needs guidance."
```

No message to penfold — the `blocked` label surfaces the shard on the session board.

**Key difference from before:** Penfold only hears about failures that survived 3 fresh
attempts. If it gets here, the problem is likely in the spec/approach, not the implementation.

## Show Progress

```
INGEST PIPELINE - Phase 4: Implement
═════════════════════════════════════
LOW/MEDIUM (single agent):
  pf-fix-aaa  | [title] | DONE (attempt 1/3)
  pf-feat-bbb | [title] | DONE (attempt 2/3) — retry fixed test failure

HIGH (layer-by-layer):
  Feature: [name]
    Wave 1 | pf-ccc-db  | DB      | DONE (1/3) | build ✓ tests ✓
    Wave 2 | pf-ccc-svc | Service | DONE (2/3) | build ✓ tests ✓ — retry fixed missing import
    Wave 3 | pf-ccc-cli | CLI     | DONE (1/3) | build ✓

FAILED (retries exhausted):
  pf-fix-ddd  | [title] | FAILED (3/3) — escalated to penfold
```

## Checkpoint (MANDATORY)

Before returning to the orchestrator, write a checkpoint:

```bash
cxp session checkpoint "$(cat <<'CKPT'
## Phase 4 Complete: Implementation

**Completed:** [N shards] — [list shard IDs + 1-line summaries]
**Retried:** [N shards needed retries] — [shard IDs + attempt counts + what the retry fixed]
**Failed (retries exhausted):** [N shards, 0 if all succeeded] — [shard IDs + failure pattern]
**Files modified:** [total count, list paths]
**Waves executed:** [for HIGH items: wave count and order]
**Next:** Phase 5 (Verify) — build, test, go vet across all changed packages
CKPT
)"
```

After all agents complete and the checkpoint is written, return to the orchestrator for verification.
