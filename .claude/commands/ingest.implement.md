---
description: "Phase 4: Launch implementation agents. Single agent for LOW/MEDIUM, layer-by-layer waves for HIGH."
---

# Ingest — Phase 4: Implement

Two execution modes based on complexity routing from Phase 3.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
CP_CLI: cp
```

## Agent Context Requirements

**Every implementation agent MUST read the project context docs before starting work.**
Include these instructions in every agent prompt:

```
## Setup (MANDATORY — do this before any code changes)
1. cp knowledge show mycroft-dev-index — project architecture, conventions, patterns
2. cp knowledge show mycroft-agent-<agent-type> — your domain-specific context
3. Read your assignment: cp task get pf-xxx
4. Claim the work: cp task claim pf-xxx
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
```

---

## Mode A: Single Agent (LOW/MEDIUM Complexity)

Launch ALL "ready now" LOW/MEDIUM agents in a **single message** (parallel, background):

**For BUG shards** (title starts with `fix:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Fix: [short title]",
  prompt="You have been assigned shard pf-fix-xxx.

  ## Setup (MANDATORY)
  1. cp knowledge show mycroft-dev-index — project architecture and conventions
  2. cp knowledge show mycroft-agent-<agent-type> — your domain context
  3. Read your assignment: cp task get pf-fix-xxx
  4. Claim the work: cp task claim pf-fix-xxx

  ## Coding Standards
  - Error handling: Return errors up the call stack. Use fmt.Errorf('context: %w', err).
    Do NOT log and return — either log OR return, not both.
  - Logging: Use structured logging (slog). Include relevant IDs.
  - Naming: Follow existing codebase conventions. Check nearby files.
  - Tests: Table-driven tests preferred. Happy path + error + edge cases.

  ## File Scope
  IMPORTANT: Only modify files listed in your shard's 'Files to Modify' section.
  Only modify: [files from shard]

  ## Implementation
  5. Read existing code patterns in the affected files
  6. Check shard progress notes for reproduction test details
  7. Run the reproduction test FIRST to confirm it fails:
     go test ./path/to/... -run TestName -v
  8. Implement the fix described in the shard
  9. Run the reproduction test again — it MUST now pass
  10. Run the full test suite for affected packages:
      go build ./path/to/...
      go test ./path/to/... -v

  Pipeline-level verification:
  1. Does the fix change the actual code path the workflow calls?
  2. Does the fix handle existing/stale data?
  3. Does the fix degrade gracefully when upstream stages fail?

  IMPORTANT: Do NOT report completion unless build and tests pass.
  IMPORTANT: If there is no reproduction test, write one yourself.

  ## Completion
  11. cp task progress pf-fix-xxx 'Implemented: [summary]'
  12. cp task close pf-fix-xxx 'Done: [summary]. Tests: [TestNames]. Files modified: [list]'

  ## Context Budget
  Prioritize: (1) working fix that compiles, (2) tests pass, (3) cleanup.
  If running low, close the shard with progress notes listing what's done and what remains.

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.
  Only modify files. The orchestrator handles all git operations.")
```

**For REQUIREMENT/SPEC shards** (title starts with `feat:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Feat: [short title]",
  prompt="You have been assigned shard pf-feat-xxx.

  ## Setup (MANDATORY)
  1. cp knowledge show mycroft-dev-index — project architecture and conventions
  2. cp knowledge show mycroft-agent-<agent-type> — your domain context
  3. Read your assignment: cp task get pf-feat-xxx
  4. Claim the work: cp task claim pf-feat-xxx

  ## Coding Standards
  - Error handling: Return errors up the call stack. Use fmt.Errorf('context: %w', err).
    Do NOT log and return — either log OR return, not both.
  - Logging: Use structured logging (slog). Include relevant IDs.
  - Naming: Follow existing codebase conventions. Check nearby files.
  - Tests: Table-driven tests preferred. Happy path + error + edge cases.

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

  ## Completion
  11. cp task progress pf-feat-xxx 'Implemented: [summary]'
  12. cp task close pf-feat-xxx 'Done: [summary]. Tests: [TestNames]. Acceptance criteria: [list which are met]. Files modified: [list]'

  ## Context Budget
  Prioritize: (1) working implementation that compiles, (2) tests pass, (3) cleanup.
  If running low, close the shard with progress notes listing what's done and what remains.

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
- Larger issues: re-launch the failed agent with the error output
- Do NOT proceed to the next wave until the current one passes

### Launch Sub-Shard Agents

For each sub-shard in the current wave:
```
Task(subagent_type="<agent-type-from-sub-shard>", run_in_background=true,
  description="[Layer]: [feature name]",
  prompt="You have been assigned sub-shard pf-xxx-layer.

  ## Setup (MANDATORY)
  1. cp knowledge show mycroft-dev-index — project architecture and conventions
  2. cp knowledge show mycroft-agent-<agent-type> — your domain context
  3. Read your assignment: cp task get pf-xxx-layer
  4. Claim the work: cp task claim pf-xxx-layer

  ## Coding Standards
  - Error handling: Return errors up the call stack. Use fmt.Errorf('context: %w', err).
    Do NOT log and return — either log OR return, not both.
  - Logging: Use structured logging (slog). Include relevant IDs.
  - Naming: Follow existing codebase conventions. Check nearby files.
  - Tests: Table-driven tests preferred. Happy path + error + edge cases.

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

  ## Completion
  7. cp task close pf-xxx-layer 'Done: [summary]. Tests: [TestNames]. Files: [files modified]'

  ## Context Budget
  Prioritize: (1) working code that compiles, (2) all tests pass, (3) cleanup.
  If running low, close the shard with progress notes listing what's done and what remains.
  Do NOT defer tests — say explicitly if tests are incomplete.

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.")
```

---

## Monitor Completion (Both Modes)

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

## Notify Penfold on Failures

If any implementation agent fails (build errors, test failures, context exhaustion):

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Implementation issue: [shard title]',
  \$body\${\"poll_hint\":\"review\",\"type\":\"progress\"}
## Implementation Issue

Shard: pf-xxx ([title])
Agent: <agent-type>
Status: FAILED — [build error / test failure / context exhaustion]

Error:
[paste relevant error output]

Action: Re-launching with [additional context / error details].
If you have insight into this failure, reply and I'll incorporate it.

-- agent-mycroft
\$body\$,
  NULL, 'progress', NULL);
"
```

## Show Progress

```
INGEST PIPELINE - Phase 4: Implement
═════════════════════════════════════
LOW/MEDIUM (single agent):
  pf-fix-aaa  | [title] | DONE
  pf-feat-bbb | [title] | DONE

HIGH (layer-by-layer):
  Feature: [name]
    Wave 1 | pf-ccc-db  | DB      | DONE ✓ | build ✓ tests ✓
    Wave 2 | pf-ccc-svc | Service | DONE ✓ | build ✓ tests ✓
    Wave 3 | pf-ccc-cli | CLI     | DONE ✓ | build ✓
```

After all agents complete, return to the orchestrator for verification.
