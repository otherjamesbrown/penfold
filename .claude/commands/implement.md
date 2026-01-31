---
description: "Analyze feature requests and launch sub-agents to implement them. Accepts shard IDs or natural language like 'all of the above'."
---

# Implement

Orchestrate implementation of feature requests by analyzing, decomposing, and delegating to sub-agents.

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
- "all of the above" → all shards mentioned
- "HIGH priority ones" → filter by priority column/mention
- "except X" → exclude matching shards

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

2. **What existing patterns should be followed?**
   - Read related existing code
   - Identify conventions

3. **What tests are needed?**
   - Unit tests
   - Integration tests
   - Which test files to modify/create

4. **What are the acceptance criteria?**
   - Extract from shard content
   - Make implicit criteria explicit

## Phase 3: Identify Shared Dependencies

Look across all shards for overlapping requirements:

| Overlap Type | Example | Action |
|--------------|---------|--------|
| Same proto field | Two shards need `ContentRequest.source` | Create foundation shard for proto change |
| Same utility | Multiple shards need date parsing | Create shared util shard first |
| Same API endpoint | Multiple CLI commands call same gateway endpoint | Create endpoint shard first |
| Same test fixture | Multiple tests need same mock data | Create fixture shard first |

**For each shared dependency:**
1. Create a foundation shard in context-palace
2. Add it as a blocker for dependent shards
3. Foundation shards are implemented first

```sql
-- Create foundation shard
SELECT create_shard('penfold',
  'Foundation: [description]',
  '[detailed requirements]',
  'task',
  NULL);  -- Unassigned, will be claimed by sub-agent

-- Get the new shard ID and set up dependencies
-- Use relate_shards() or manual tracking
```

## Phase 4: Build Dependency Graph

Determine execution order:

```
Level 0 (no dependencies):     [foundation shards]
Level 1 (depends on L0):       [shards using foundations]
Level 2 (depends on L1):       [shards using L1 outputs]
```

Shards at the same level can run in parallel.

**Identify parallel tracks:**
- Shards touching different files
- Shards in different domains (CLI vs Gateway vs Worker)
- Independent features

## Phase 5: Pre-Handoff Checklist

Before creating implementation shards, verify:

### Requirements
- [ ] Acceptance criteria explicit for each shard
- [ ] Ambiguities resolved (or flagged)
- [ ] Edge cases identified

### Codebase Context
- [ ] Files to touch identified (prevents sub-agent conflicts)
- [ ] Existing patterns documented (code samples, not descriptions)
- [ ] Related tests identified
- [ ] No file overlap between parallel shards

### Backend Readiness
- [ ] Required APIs exist (or foundation shard created)
- [ ] Required proto fields exist (or foundation shard created)
- [ ] No missing infrastructure

### Safety
- [ ] No database migrations needed (or flagged)
- [ ] No breaking changes (or flagged)
- [ ] Security implications reviewed

**If blockers found, ask the user how to proceed. Otherwise continue.**

## Phase 6: Create Implementation Shards

For each work unit, create an implementation shard with full context:

```sql
SELECT create_shard('penfold',
  'Implement: [specific task]',
  '## Goal
[What this shard accomplishes]

## Context
[Why this is needed, what it connects to]

## Files to Modify
- path/to/file1.go - [what changes]
- path/to/file2.go - [what changes]

## Files to Create
- path/to/new_file.go - [purpose]

## Patterns to Follow
[Code snippets from existing codebase showing the pattern]

```go
// Example from existing code
func ExistingPattern() {
    // ...
}
```

## Tests Required
- [ ] Unit test: [specific test case]
- [ ] Integration test: [specific scenario]
- Test file: path/to/test_file.go

## Acceptance Criteria
- [ ] [Specific, verifiable criterion]
- [ ] [Specific, verifiable criterion]
- [ ] Tests pass

## Dependencies
- Depends on: pf-xxxxx (if any)
- Blocks: pf-yyyyy (if any)

## Verification
```bash
# Commands to verify this shard is complete
go test ./path/to/...
penf [command] --help
```

## Agent
Assign to: [cli-dev | service-dev | worker-dev | data-dev | ai-dev]
',
  'task',
  NULL);
```

## Phase 7: Launch Sub-Agents

For each level in the dependency graph:

1. **Wait for previous level to complete** (if not level 0)

2. **Launch parallel sub-agents** for shards at this level:

```
Use the Task tool with appropriate subagent_type:
- cli-dev: CLI commands in cmd/penf/
- service-dev: Gateway, proto, gRPC
- worker-dev: Temporal workflows, activities
- data-dev: Database, migrations, repositories
- ai-dev: Embeddings, search, ML
```

3. **Sub-agent prompt includes:**
   - The implementation shard ID
   - Instruction to read shard via palace CLI
   - Instruction to update progress via palace CLI
   - Instruction to close shard when done

Example sub-agent prompt:
```
You have been assigned shard pf-xxxxx.

1. Read your assignment:
   palace task get pf-xxxxx

2. Claim the work:
   palace task claim pf-xxxxx

3. Implement as described in the shard content

4. Log progress as you work:
   palace task progress pf-xxxxx "Completed unit tests"

5. Add artifacts:
   palace artifact add pf-xxxxx file cmd/penf/pipeline.go "Added kick command"
   palace artifact add pf-xxxxx commit abc123 "Implemented pipeline kick"

6. When complete:
   palace task close pf-xxxxx "Implemented pipeline kick command with tests"

Do not create a PR. Just implement and close the shard.
```

4. **Monitor progress** via context-palace queries:
```sql
SELECT id, title, status FROM shards
WHERE id IN ('pf-xxx', 'pf-yyy', 'pf-zzz');
```

5. **When level complete**, proceed to next level

## Phase 8: Report Results

When all shards are complete:

```
Implementation Complete

Shards Completed: N
├── pf-xxxxx: [title] ✓
├── pf-yyyyy: [title] ✓
└── pf-zzzzz: [title] ✓

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

## Error Handling

If a sub-agent fails:
1. Read the shard for error details
2. Determine if it's a blocker for other shards
3. Ask the user how to proceed:
   - Fix and retry?
   - Skip and continue with non-blocked shards?
   - Abort?

## Notes

- Sub-agents use `palace` CLI for task management (no SQL needed)
- Orchestrator (you) uses psql for complex queries and shard creation
- No PRs created - just code changes
- Ask questions when genuinely uncertain, otherwise act with confidence
- Launch parallel sub-agents where possible for efficiency
