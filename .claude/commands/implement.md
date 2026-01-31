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

## Phase 3: Build Parallel Execution Plan

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

## Phase 4: Create Implementation Shards

Use heredoc with dollar-quoting for content with backticks:

```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" <<'EOSQL'
SELECT create_shard('penfold',
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
[Specific file paths the agent should read for patterns]
- cmd/penf/cmd/workflow.go - similar command structure
- services/gateway/workflowservice/service.go - similar handler pattern

## Acceptance Criteria
- [ ] [Specific, verifiable criterion]
- [ ] Code compiles: go build ./...
- [ ] Tests pass: go test ./...

## Verification Commands
go build ./cmd/penf/...
penf [command] --help
go test ./path/to/...

## Agent Type
Assign to: [cli-dev | service-dev | worker-dev | data-dev | ai-dev]
$md$,
  'task',
  NULL);
EOSQL
```

## Phase 5: Launch Sub-Agents in Parallel

**Launch ALL agents that can start NOW in a single message with multiple Task tool calls.**

Available agent types:
- cli-dev: CLI commands in cmd/penf/
- service-dev: Gateway, proto, gRPC
- worker-dev: Temporal workflows, activities
- data-dev: Database, migrations, repositories
- ai-dev: Embeddings, search, ML

### Sub-Agent Prompt Template

```
You have been assigned shard pf-xxxxx.

## Setup
1. Read your assignment:
   /Users/dev/bin/palace task get pf-xxxxx

2. Claim the work:
   /Users/dev/bin/palace task claim pf-xxxxx

## Implementation
3. Read the existing code patterns mentioned in the shard
4. Implement the changes described
5. Ensure code compiles: go build ./...
6. Run relevant tests: go test ./...

## Completion
7. Log what you did:
   /Users/dev/bin/palace task progress pf-xxxxx "Implemented X, Y, Z"

8. Close the shard:
   /Users/dev/bin/palace task close pf-xxxxx "Done: [summary]"

Do not create a PR. Just implement and close the shard.
```

### Parallel Launch Pattern

To launch agents in parallel, use multiple Task tool invocations in a SINGLE response message. Do NOT send them sequentially - group all "can start NOW" agents together.

Example: If cli-dev #1 and service-dev can both start NOW, invoke both Task tools in the same message.

## Phase 6: Monitor and Coordinate

### Check Progress

```sql
SELECT id, title, status, owner
FROM shards
WHERE id IN ('pf-xxx', 'pf-yyy', 'pf-zzz');
```

### When Foundation Complete

Once a foundation agent (e.g., service-dev) completes:
1. Verify the changes compile
2. Launch dependent agents (e.g., cli-dev #2)

### Handle Failures

If a sub-agent fails:
1. Read the shard for error details
2. Determine if it's a blocker for other shards
3. Ask the user how to proceed:
   - Fix and retry?
   - Skip and continue with non-blocked shards?
   - Abort?

## Phase 7: Report Results

When all shards are complete:

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

Next Steps:
1. Review changes: git diff
2. Run full test suite: make test
3. Commit when ready: git add . && git commit
```

## Key Principles

1. **NEVER write code yourself** - always delegate to sub-agents
2. **Maximize parallelism** - launch all independent work simultaneously
3. **Right-size agents** - not too granular, not too coarse
4. **Fresh context is good** - sub-agents focus without distraction
5. **Sub-agents use palace CLI** - orchestrator uses psql for complex queries
6. **No PRs** - just code changes, user decides when to commit
