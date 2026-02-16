---
description: "Phase 2: Launch debugger agents for bugs and explorer agents for requirements in parallel. SPECs skip this phase."
---

# Ingest — Phase 2: Investigate & Analyze

This phase handles BOTH bugs (investigation) and requirements (analysis) in parallel.
**SPECs skip this phase entirely** — they already contain the analysis.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

## SPEC Handling

If Phase 1 classified any items as SPEC (title starts with `spec:`), those shards are
**already complete analyses**. Do NOT launch explore agents for them.

- Mark SPEC shards as ready for triage: close them with the spec content as findings
- They flow directly to Phase 3

```bash
# For each SPEC shard:
cxp task close pf-spc-xxx 'SPEC provided by sender. COMPLEXITY: [assess from spec content]. LAYERS: [from spec]. SCOPE: [from spec]. FILES: [from spec]. PATTERN: [from spec if mentioned]. Full spec content preserved in shard.'
```

If the spec doesn't explicitly state complexity, assess it from the spec structure:
- 1 layer, few files → Low
- 1-2 layers, clear pattern → Medium
- 3+ layers, new subsystem → High

## Bug Investigation

**Runs ONLY for items classified as BUG.** Skip if there are no bugs.

### Launch Debugger Agents

**Batch size: max 4 agents at a time.** If there are more than 4 items, launch in batches
of 4, wait for the batch to complete, then launch the next batch. This prevents all agent
outputs from flooding the orchestrator's context at once.

Launch each batch in a **single message** (parallel, background):

For each investigation shard (title starts with `investigate:`), use:
```
Task(subagent_type="debugger", run_in_background=true,
  description="Debug: [short title]",
  prompt="Investigate shard pf-inv-xxx.

  Bug report from agent-penfold:
  [paste full bug content here]

  Your investigation shard: pf-inv-xxx

  ## Setup
  1. cxp knowledge show mycroft-dev-index — mandatory project context
  2. cxp knowledge show mycroft-agent-debugger — your domain context
  3. cxp task claim pf-inv-xxx

  ## Penfold Acceptance Test
  If the shard mentions a Penfold-provided test (quality golden file or e2e test),
  read it first — it defines exactly what 'fixed' looks like. Your investigation
  should identify the root cause of WHY that test fails. The test itself is not
  under investigation — it is the ground truth.

  ## Investigation
  4. Investigate using read-only tools (Read, Grep, Glob, Bash for go build/test)
  5. Identify root cause, affected files, and proposed fix

  IMPORTANT: Trace the FULL data path — not just the root cause layer.

  For pipeline bugs (data processing):
  - Where does the input data come from? (DB column, upstream stage, metadata)
  - Is the data transformed or filtered between stages?
  - What happens when upstream stages fail or return nil?
  - Are there existing records in the DB that predate the fix?

  For visibility/wiring bugs (data not showing in CLI):
  - Trace from DB column → repository (SELECT + scan) → types.go (struct)
    → service.go (mapping) → proto → CLI client (struct) → CLI command (display)
  - List EVERY file in EVERY layer. Don't stop at the gRPC service — the CLI
    client and command layers have their own structs that also need updating.
  - Count files per layer: e.g. '6 files across gateway (3) + CLI (3)'

  For data cleanup:
  - Does existing data in the DB need correction? (e.g. column rename doesn't
    fix garbage data already in the column)
  - Does the metadata key name match the new column name?

  ## Write Investigation Report
  6. Before closing, write a detailed report to /tmp/report-pf-inv-xxx.md covering:
     - Files examined and what was found in each (with line numbers)
     - Chain of reasoning from symptom to root cause
     - Alternative causes considered and ruled out
     - Exact error messages, stack traces, or data found
     - Full affected file list across ALL layers
     - Proposed fix with specific code locations

  7. Create a report shard and link it as a child:
     ```bash
     cxp shard create --type report \
       --title 'report: debugger phase-2 — pf-inv-xxx' \
       --body-file /tmp/report-pf-inv-xxx.md
     # Use the shard ID returned by create (e.g. pf-abc123)
     cxp shard link pf-REPORT-ID --child-of pf-inv-xxx
     ```

  ## Completion
  8. cxp task close pf-inv-xxx 'ROOT CAUSE: [category]. [summary]. FILES: [file1, file2, ...ALL files across ALL layers]. FIX: [description]. COMPLEXITY: [Low/Medium/High]. LAYERS: [list which sub-agent domains are needed]. REPORT: pf-REPORT-ID'

  Root cause categories: cli_ux, config_drift, temporal_workflow, grpc_wiring, data_layer, proto_mismatch, missing_feature, test_gap")
```

## Requirement Analysis

**Runs ONLY for items classified as REQUIREMENT.** Skip if there are no requirements.

### Launch Explore Agents

**Batch size: max 4 agents at a time** (same rule as debuggers).

Launch each batch in a **single message** (parallel, background):

For each analysis shard (title starts with `analyze:`), use:
```
Task(subagent_type="Explore", run_in_background=true,
  description="Analyze: [short title]",
  prompt="Analyze requirement shard pf-anl-xxx.

  Requirement from agent-penfold:
  [paste full requirement content here]

  Your analysis shard: pf-anl-xxx

  ## Setup
  1. cxp knowledge show mycroft-dev-index — mandatory project context
  2. cxp task claim pf-anl-xxx

  ## Analysis
  Explore the codebase to answer:

  3. **Existing patterns:** Find the closest existing feature to what's being requested.
     How is it structured? What packages, files, and patterns does it use?

  4. **Scope:** What files need to be created or modified? Is this a new file, a new
     function in an existing file, or changes across multiple files?

  5. **Dependencies:** Does this require changes to protos, database schema, or shared
     packages? Are there upstream/downstream impacts?

  6. **Layers touched:** Which architectural layers does this feature span?
     - Database (migrations, repository methods)
     - Service (proto definitions, gRPC handlers)
     - CLI (Cobra commands, output formatting)
     - Pipeline (Temporal workflows/activities)
     - AI/ML (embeddings, LLM integration)

  7. **Complexity:** Based on layers touched and scope:
     - **Low** — Single layer, single file, follow existing pattern directly
     - **Medium** — 1-2 layers, multiple files but clear pattern to follow
     - **High** — 3+ layers (e.g. DB + proto + service + CLI), new subsystem,
       cross-cutting concerns, or 10+ files to create/modify

  ## Write Analysis Report
  8. Before closing, write a detailed report to /tmp/report-pf-anl-xxx.md covering:
     - Files explored and patterns found (with line numbers)
     - Existing feature analysis — how the closest pattern works
     - Dependency map — what imports what, upstream/downstream impacts
     - Complexity reasoning — why Low/Medium/High
     - Full file list: existing files to modify, new files to create
     - Recommended approach with rationale

  9. Create a report shard and link it as a child:
     ```bash
     cxp shard create --type report \
       --title 'report: explorer phase-2 — pf-anl-xxx' \
       --body-file /tmp/report-pf-anl-xxx.md
     # Use the shard ID returned by create (e.g. pf-abc123)
     cxp shard link pf-REPORT-ID --child-of pf-anl-xxx
     ```

  ## Completion
  10. cxp task close pf-anl-xxx 'COMPLEXITY: [Low/Medium/High]. LAYERS: [db,service,cli,pipeline]. SCOPE: [summary of what to build]. FILES: [file1, file2, new:file3]. PATTERN: [existing feature to follow]. REPORT: pf-REPORT-ID'

  IMPORTANT: Do NOT write any code. Only analyze and report.
  IMPORTANT: The COMPLEXITY and LAYERS fields are critical — they determine how implementation is structured.")
```

## Monitor Completion (Both Types)

Poll shard status until all are closed:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT id, title, status, closed_reason
FROM shards
WHERE id IN ('pf-inv-aaa', 'pf-anl-bbb')
ORDER BY created_at;
"
```

Also use `Read` on background agent output files to check progress.

Wait until ALL shards show `status = 'closed'`. Check every 30-60 seconds.

## Send Findings Summary to Penfold

**After ALL investigations and analyses complete**, send a progress update to penfold with
the findings. This lets penfold validate root causes and complexity assessments before
implementation resources are committed.

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "
SELECT send_message('penfold', 'agent-mycroft', ARRAY['agent-penfold'],
  'Findings: [N bugs investigated, M requirements analyzed]',
  \$body\${\"poll_hint\":\"review\",\"type\":\"progress\"}
## Investigation & Analysis Complete

### Bug Findings
1. **[bug title]** — ROOT CAUSE: [category]. [1-sentence summary]. Complexity: [L/M/H]
2. **[bug title]** — ROOT CAUSE: [category]. [1-sentence summary]. Complexity: [L/M/H]

### Requirement Findings
1. **[req title]** — [complexity]. Layers: [list]. [1-sentence approach]
2. **[req title]** — [complexity]. Layers: [list]. [1-sentence approach]

### Specs (pre-analyzed, skipped investigation)
1. **[spec title]** — [complexity]. Proceeding directly to triage.

Proceeding to triage and implementation. Reply if you want to adjust anything.

-- agent-mycroft
\$body\$,
  NULL, 'progress', NULL);
"
```

**Do NOT block on penfold's response.** Continue to Phase 3. But if penfold replies with
corrections before implementation starts, incorporate them.

## Show Progress

```
INGEST PIPELINE - Phase 2: Investigate & Analyze
═════════════════════════════════════════════════
BUGS:
  pf-inv-aaa | [title] | DONE - [category]: [summary]
  pf-inv-bbb | [title] | DONE - [category]: [summary]

REQUIREMENTS:
  pf-anl-ccc | [title] | DONE - [complexity]: [layers]: [summary]
  pf-anl-ddd | [title] | DONE - [complexity]: [layers]: [summary]

SPECS (skipped analysis):
  pf-spc-eee | [title] | READY - [complexity]: [layers]

Findings sent to penfold (non-blocking).
All N items complete. Returning to orchestrator for triage...
```

## Checkpoint (MANDATORY)

Before returning to the orchestrator, write a checkpoint:

```bash
cxp session checkpoint "$(cat <<'CKPT'
## Phase 2 Complete: Investigate & Analyze

**Bugs investigated:** [N] — [list shard IDs + root cause categories]
**Requirements analyzed:** [N] — [list shard IDs + complexity]
**Specs (skipped):** [N] — [list shard IDs]
**Key findings:** [any surprises, merged items, shared root causes]
**Next:** Phase 3 (Triage) with shard IDs: [list]
CKPT
)"
```

After displaying progress and writing the checkpoint, return to the orchestrator.
