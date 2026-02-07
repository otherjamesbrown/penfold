---
description: "Phase 4: Launch implementation agents. Single agent for LOW/MEDIUM, layer-by-layer waves for HIGH."
---

# Ingest — Phase 4: Implement

Two execution modes based on complexity routing from Phase 3.

## Configuration

```yaml
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
PALACE_CLI: /Users/dev/bin/palace
```

## Mode A: Single Agent (LOW/MEDIUM Complexity)

Launch ALL "ready now" LOW/MEDIUM agents in a **single message** (parallel, background):

**For BUG shards** (title starts with `fix:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Fix: [short title]",
  prompt="You have been assigned shard pf-fix-xxx.

  ## Setup
  1. Read your assignment: /Users/dev/bin/palace task get pf-fix-xxx
  2. Claim the work: /Users/dev/bin/palace task claim pf-fix-xxx

  ## File Scope
  IMPORTANT: Only modify files listed in your shard's 'Files to Modify' section.
  Only modify: [files from shard]

  ## Implementation
  3. Read existing code patterns in the affected files
  4. Check shard progress notes for reproduction test details
  5. Run the reproduction test FIRST to confirm it fails:
     go test ./path/to/... -run TestName -v
  6. Implement the fix described in the shard
  7. Run the reproduction test again — it MUST now pass
  8. Run the full test suite for affected packages:
     go build ./path/to/...
     go test ./path/to/... -v

  Pipeline-level verification:
  1. Does the fix change the actual code path the workflow calls?
  2. Does the fix handle existing/stale data?
  3. Does the fix degrade gracefully when upstream stages fail?

  IMPORTANT: Do NOT report completion unless build and tests pass.
  IMPORTANT: If there is no reproduction test, write one yourself.

  ## Completion
  9. /Users/dev/bin/palace task progress pf-fix-xxx 'Implemented: [summary]'
  10. /Users/dev/bin/palace task close pf-fix-xxx 'Done: [summary]. Tests: [TestNames]'

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.
  Only modify files. The orchestrator handles all git operations.")
```

**For REQUIREMENT shards** (title starts with `feat:`):
```
Task(subagent_type="<agent-type>", run_in_background=true,
  description="Feat: [short title]",
  prompt="You have been assigned shard pf-feat-xxx.

  ## Setup
  1. Read your assignment: /Users/dev/bin/palace task get pf-feat-xxx
  2. Claim the work: /Users/dev/bin/palace task claim pf-feat-xxx

  ## File Scope
  IMPORTANT: Only modify/create files listed in your shard's 'Files to Modify/Create'.
  Files: [files from shard]

  ## Implementation
  3. Read the existing pattern referenced in the shard — follow it closely
  4. Check shard progress notes for acceptance test details
  5. If acceptance tests exist, run them FIRST to confirm they fail
  6. Implement the feature following the approach and pattern specified
  7. Run acceptance tests again — they MUST now pass
  8. Run the full test suite:
     go build ./path/to/...
     go test ./path/to/... -v

  IMPORTANT: Follow the existing pattern closely. Don't over-engineer.
  IMPORTANT: Do NOT report completion unless build and ALL tests pass.
  IMPORTANT: If there are no acceptance tests, write them yourself.

  ## Completion
  9. /Users/dev/bin/palace task progress pf-feat-xxx 'Implemented: [summary]'
  10. /Users/dev/bin/palace task close pf-feat-xxx 'Done: [summary]. Tests: [TestNames]'

  CRITICAL: Do NOT run git add, git commit, git push, or any git write commands.
  Only modify files. The orchestrator handles all git operations.")
```

---

## Mode B: Layer-by-Layer (HIGH Complexity — Decomposed)

Execute sub-shards in **dependency waves**. Within each wave, sub-shards from different
features can run in parallel. Within a single feature, layers execute sequentially.

### Wave Execution

```
Wave 1: All DB sub-shards (feat-db:) — no dependencies
  → pf-featA-db + pf-featB-db in parallel
  → Wait → Verify: go build ./pkg/... && go test ./pkg/... -v

Wave 2: All Service sub-shards (feat-svc:) — depend on DB
  → pf-featA-svc + pf-featB-svc in parallel
  → Wait → Verify: go build ./services/gateway/... && go test ./services/gateway/... -v

Wave 3: All CLI sub-shards (feat-cli:) — depend on Service
  → pf-featA-cli + pf-featB-cli in parallel
  → Wait → Verify: go build ./cmd/penf/...

Wave 4: All Pipeline sub-shards (feat-pipe:) — depend on DB
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

  ## Setup
  1. Read your assignment: /Users/dev/bin/palace task get pf-xxx-layer
  2. Claim the work: /Users/dev/bin/palace task claim pf-xxx-layer

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
  3. Read the existing pattern referenced in the shard
  4. Implement the scope described in the sub-shard
  5. Write tests for YOUR layer (not other layers)
  6. Verify build and tests pass:
     go build ./path/to/...
     go test ./path/to/... -v

  IMPORTANT: Follow the existing pattern closely.
  IMPORTANT: Do NOT report completion unless build and tests pass.
  IMPORTANT: Write tests for every method/handler you create.

  ## Shared Package Warning (CLI agents only)
  The cmd/penf/cmd/ package is shared. Before defining ANY helper function, search
  the package for existing definitions:
    grep -r 'func functionName' cmd/penf/cmd/
  Use existing helpers. Do NOT redefine them.

  ## Completion
  7. /Users/dev/bin/palace task close pf-xxx-layer 'Done: [summary]. Tests: [TestNames]. Files: [files modified]'

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
