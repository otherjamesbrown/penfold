---
description: Execute the implementation plan by processing beads for the current feature, replacing tasks.md workflow
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

5. **Identify current feature beads**:
   - Use `bd list --status=open` to get all open beads
   - Filter beads related to current feature (check titles for feature name from FEATURE_DIR)
   - Use `bd ready` to identify immediately actionable beads (no blockers)
   - Use `bd show [bead-id]` to get detailed information for next bead to work on

6. **Execute implementation using bead workflow**:
   - **Select next bead**: Choose highest priority bead from `bd ready` that matches current feature
   - **Update status**: Use `bd update [bead-id] --status=in_progress` to claim work
   - **Implement bead**: Execute the work described in bead description
     - Follow TDD approach if tests are mentioned
     - Create/modify files as specified in description
     - Respect dependencies and execution order
   - **Complete bead**: Use `bd close [bead-id]` when work is finished
   - **Check next work**: Run `bd ready` to see newly unblocked beads

7. **Implementation execution rules**:
   - **Bead-driven workflow**: Work on one bead at a time, complete it fully before moving to next
   - **Dependency respect**: Only work on beads shown by `bd ready` (no blockers)
   - **Status tracking**: Keep bead status updated (open → in_progress → closed)
   - **Tests first**: If bead description mentions tests, write failing tests before implementation
   - **File-based coordination**: Ensure file changes don't conflict across beads
   - **Validation checkpoints**: Verify bead completion criteria before closing

8. **Progress tracking and error handling**:
   - Report progress after each completed bead using `bd stats`
   - Use `bd show [bead-id]` to check dependencies if stuck
   - Halt execution if any bead fails - update bead with error details
   - Provide clear error messages with context for debugging
   - Suggest next steps if implementation cannot proceed

9. **Completion validation**:
   - Verify all feature-related beads are closed using `bd list --status=closed`
   - Check that implemented features match original specification
   - Validate that tests pass and coverage meets requirements
   - Confirm implementation follows technical plan
   - Report final status with summary of completed beads

## Bead Workflow Integration

### Finding Work
```bash
# Get current feature beads ready to work on
bd ready | grep "Feature Name"

# Get details for specific bead
bd show [bead-id]
```

### Working on Beads
```bash
# Claim work
bd update [bead-id] --status=in_progress

# Complete work
bd close [bead-id] --reason="Implementation complete: [summary]"

# Check newly available work
bd ready
```

### Progress Tracking
```bash
# Overall project health
bd stats

# Check specific feature progress
bd list --status=closed | grep "Feature Name"
bd list --status=open | grep "Feature Name"
```

### Error Handling
```bash
# If bead fails, document the issue
bd update [bead-id] --description="BLOCKED: [error description]. Original: [original description]"

# Create new bead for fix if needed
bd create --title="Fix: [issue]" --type=bug --priority=1 --description="Resolve blocking issue in [bead-id]"
```

## Key Differences from tasks.md Workflow

1. **Dynamic work queue**: `bd ready` shows current actionable work vs static task list
2. **Dependency enforcement**: Beads system prevents working on blocked items
3. **Status tracking**: Real-time status updates vs manual checkbox marking
4. **Collaborative ready**: Multiple developers can see and claim work
5. **Error tracking**: Failed beads can be updated with blocking issues
6. **Progress visibility**: `bd stats` gives instant project health vs manual counting

## Implementation Strategy

1. **Start with setup/foundational beads** - these typically have no dependencies
2. **Follow bd ready workflow** - only work on beads with no blockers
3. **Complete beads fully** - don't partially implement and move on
4. **Update status religiously** - keeps workflow accurate for next sessions
5. **Use bead descriptions** - they contain the specific tasks and file paths
6. **Validate before closing** - ensure acceptance criteria met
7. **Check for newly available work** after each completion

Note: This command assumes beads have been created for the feature using `/speckit.beads`. If no beads exist, suggest running `/speckit.beads` first to generate the work breakdown.