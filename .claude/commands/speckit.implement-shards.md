---
description: Execute the implementation plan by processing shards for the current feature, replacing tasks.md workflow
---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Outline

1. Run `.specify/scripts/bash/check-prerequisites.sh --json` from repo root and parse FEATURE_DIR and AVAILABLE_DOCS list. All paths must be absolute. For single quotes in args like "I'm Groot", use escape syntax: e.g 'I'\''m Groot' (or double-quote if possible: "I'm Groot").

2. **Check checklists status** (if FEATURE_DIR/checklists/ exists):
   - Scan all checklist files in the checklists/ directory
   - For each checklist, count total/completed/incomplete items
   - Create status table and determine overall PASS/FAIL status
   - If any checklist incomplete, ask user whether to proceed

3. **Load implementation context**:
   - **REQUIRED**: Read plan.md for tech stack, architecture, and file structure
   - **IF EXISTS**: Read spec.md for user stories and acceptance criteria
   - **IF EXISTS**: Read data-model.md for entities and relationships
   - **IF EXISTS**: Read contracts/ for API specifications and test requirements
   - **IF EXISTS**: Read research.md for technical decisions and constraints
   - **IF EXISTS**: Read quickstart.md for integration scenarios

4. **Project Setup Verification**:
   - Create/verify ignore files based on project setup (.gitignore, etc.)
   - Use same technology detection logic as original speckit.implement

5. **Identify current feature shards**:
   - Use SQL to get all open shards for current feature
   - Use `tasks_for()` to identify immediately actionable shards (no blockers)
   - Query shard details for next shard to work on

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

6. **Execute implementation using shard workflow**:
   - **Select next shard**: Choose highest priority shard from `tasks_for()` that matches current feature
   - **Update status**: Use `claim_task()` to claim work
   - **Implement shard**: Execute the work described in shard content
     - Follow TDD approach if tests are mentioned
     - Create/modify files as specified in description
     - Respect dependencies and execution order
   - **Complete shard**: Use `close_task()` when work is finished
   - **Check next work**: Run `tasks_for()` to see newly unblocked shards

7. **Implementation execution rules**:
   - **Shard-driven workflow**: Work on one shard at a time, complete it fully before moving to next
   - **Dependency respect**: Only work on shards shown by `tasks_for()` (no blockers)
   - **Status tracking**: Keep shard status updated (open → in_progress → closed)
   - **Tests first**: If shard description mentions tests, write failing tests before implementation
   - **File-based coordination**: Ensure file changes don't conflict across shards
   - **Validation checkpoints**: Verify shard completion criteria before closing

8. **Progress tracking and error handling**:
   - Report progress after each completed shard using status queries
   - Check shard details if stuck
   - Halt execution if any shard fails - update shard with error details
   - Provide clear error messages with context for debugging
   - Suggest next steps if implementation cannot proceed

9. **Completion validation**:
   - Verify all feature-related shards are closed
   - Check that implemented features match original specification
   - Validate that tests pass and coverage meets requirements
   - Confirm implementation follows technical plan
   - Report final status with summary of completed shards

## Shard Workflow Integration

### Finding Work
```sql
-- Get current feature shards ready to work on
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- Get all shards for a feature
SELECT id, title, status FROM shards
WHERE project = 'penfold' AND title ILIKE '%Feature Name%';

-- Get details for specific shard
SELECT * FROM shards WHERE id = 'pf-xxx';
```

### Working on Shards
```sql
-- Claim work
SELECT claim_task('pf-xxx', 'agent-penfdev');

-- Complete work
SELECT close_task('pf-xxx', 'Implementation complete: [summary]');

-- Check newly available work
SELECT * FROM tasks_for('penfold', 'agent-penfdev');
```

### Progress Tracking
```sql
-- Count by status for project health
SELECT status, COUNT(*) FROM shards
WHERE project = 'penfold'
GROUP BY status;

-- Check specific feature progress (closed)
SELECT id, title FROM shards
WHERE project = 'penfold' AND title ILIKE '%Feature Name%' AND status = 'closed';

-- Check specific feature progress (open)
SELECT id, title FROM shards
WHERE project = 'penfold' AND title ILIKE '%Feature Name%' AND status != 'closed';
```

### Error Handling
```sql
-- If shard fails, document the issue by updating content
UPDATE shards SET content = 'BLOCKED: [error description]. Original: [original content]'
WHERE id = 'pf-xxx';

-- Create new shard for fix if needed
SELECT create_shard('penfold', 'Fix: [issue]', 'Resolve blocking issue in pf-xxx', 'task', 'agent-penfdev');
```

## Key Differences from tasks.md Workflow

1. **Dynamic work queue**: `tasks_for()` shows current actionable work vs static task list
2. **Dependency enforcement**: Shards system prevents working on blocked items via relates-to edges
3. **Status tracking**: Real-time status updates vs manual checkbox marking
4. **Collaborative ready**: Multiple developers can see and claim work
5. **Error tracking**: Failed shards can be updated with blocking issues
6. **Progress visibility**: SQL queries give instant project health vs manual counting

## Implementation Strategy

1. **Start with setup/foundational shards** - these typically have no dependencies
2. **Follow tasks_for() workflow** - only work on shards with no blockers
3. **Complete shards fully** - don't partially implement and move on
4. **Update status religiously** - keeps workflow accurate for next sessions
5. **Use shard content** - it contains the specific tasks and file paths
6. **Validate before closing** - ensure acceptance criteria met
7. **Check for newly available work** after each completion

Note: This command assumes shards have been created for the feature using `/speckit.shards`. If no shards exist, suggest running `/speckit.shards` first to generate the work breakdown.
