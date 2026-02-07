---
description: "Phase 2: Launch debugger agents for bugs and explorer agents for requirements in parallel."
---

# Ingest — Phase 2: Investigate & Analyze

This phase handles BOTH bugs (Phase 2) and requirements (Phase 2R) in parallel.

## Configuration

```yaml
AGENT_NAME: agent-mycroft
PROJECT: penfold
DB_CONN: "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full"
```

## Bug Investigation (Phase 2)

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
  3. /Users/dev/bin/palace task close pf-inv-xxx 'ROOT CAUSE: [category]. [summary]. FILES: [file1, file2]. FIX: [description]. COMPLEXITY: [Low/Medium/High]'

  Root cause categories: cli_ux, config_drift, temporal_workflow, grpc_wiring, data_layer, proto_mismatch, missing_feature, test_gap")
```

## Requirement Analysis (Phase 2R)

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
  1. /Users/dev/bin/palace task claim pf-anl-xxx

  ## Analysis
  Explore the codebase to answer:

  2. **Existing patterns:** Find the closest existing feature to what's being requested.
     How is it structured? What packages, files, and patterns does it use?

  3. **Scope:** What files need to be created or modified? Is this a new file, a new
     function in an existing file, or changes across multiple files?

  4. **Dependencies:** Does this require changes to protos, database schema, or shared
     packages? Are there upstream/downstream impacts?

  5. **Layers touched:** Which architectural layers does this feature span?
     - Database (migrations, repository methods)
     - Service (proto definitions, gRPC handlers)
     - CLI (Cobra commands, output formatting)
     - Pipeline (Temporal workflows/activities)
     - AI/ML (embeddings, LLM integration)

  6. **Complexity:** Based on layers touched and scope:
     - **Low** — Single layer, single file, follow existing pattern directly
     - **Medium** — 1-2 layers, multiple files but clear pattern to follow
     - **High** — 3+ layers (e.g. DB + proto + service + CLI), new subsystem,
       cross-cutting concerns, or 10+ files to create/modify

  ## Completion
  7. /Users/dev/bin/palace task close pf-anl-xxx 'COMPLEXITY: [Low/Medium/High]. LAYERS: [db,service,cli,pipeline]. SCOPE: [summary of what to build]. FILES: [file1, file2, new:file3]. PATTERN: [existing feature to follow].'

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

All N items complete. Returning to orchestrator for triage...
```

After displaying progress, return to the orchestrator. It will invoke the next phase.
