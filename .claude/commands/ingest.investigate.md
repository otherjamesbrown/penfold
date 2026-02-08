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
cp task close pf-spc-xxx 'SPEC provided by sender. COMPLEXITY: [assess from spec content]. LAYERS: [from spec]. SCOPE: [from spec]. FILES: [from spec]. PATTERN: [from spec if mentioned]. Full spec content preserved in shard.'
```

If the spec doesn't explicitly state complexity, assess it from the spec structure:
- 1 layer, few files → Low
- 1-2 layers, clear pattern → Medium
- 3+ layers, new subsystem → High

## Bug Investigation

**Runs ONLY for items classified as BUG.** Skip if there are no bugs.

### Launch Debugger Agents

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
  1. cp knowledge show mycroft-dev-index — mandatory project context
  2. cp knowledge show mycroft-agent-debugger — your domain context
  3. cp task claim pf-inv-xxx

  ## Investigation
  4. Investigate using read-only tools (Read, Grep, Glob, Bash for go build/test)
  5. Identify root cause, affected files, and proposed fix

  IMPORTANT: Trace the FULL data path from database → activity → function.
  Do not just verify the function works in isolation. Identify:
  - Where does the input data come from? (DB column, upstream stage, metadata)
  - Is the data transformed or filtered between stages?
  - What happens when upstream stages fail or return nil?
  - Are there existing records in the DB that predate the fix?

  ## Completion
  6. cp task close pf-inv-xxx 'ROOT CAUSE: [category]. [summary]. FILES: [file1, file2]. FIX: [description]. COMPLEXITY: [Low/Medium/High]'

  Root cause categories: cli_ux, config_drift, temporal_workflow, grpc_wiring, data_layer, proto_mismatch, missing_feature, test_gap")
```

## Requirement Analysis

**Runs ONLY for items classified as REQUIREMENT.** Skip if there are no requirements.

### Launch Explore Agents

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
  1. cp knowledge show mycroft-dev-index — mandatory project context
  2. cp task claim pf-anl-xxx

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

  ## Completion
  8. cp task close pf-anl-xxx 'COMPLEXITY: [Low/Medium/High]. LAYERS: [db,service,cli,pipeline]. SCOPE: [summary of what to build]. FILES: [file1, file2, new:file3]. PATTERN: [existing feature to follow].'

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

After displaying progress, return to the orchestrator. It will invoke the next phase.
