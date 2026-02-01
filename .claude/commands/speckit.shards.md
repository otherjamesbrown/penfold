---
description: Generate actionable, dependency-ordered shards for the feature based on available design artifacts, replacing tasks.md workflow.
handoffs:
  - label: Analyze For Consistency
    agent: speckit.analyze
    prompt: Run a project analysis for consistency
    send: true
  - label: Implement Project
    agent: speckit.implement
    prompt: Start the implementation in phases using shard tracking
    send: true
---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Outline

1. **Setup**: Run `.specify/scripts/bash/check-prerequisites.sh --json` from repo root and parse FEATURE_DIR and AVAILABLE_DOCS list. All paths must be absolute. For single quotes in args like "I'm Groot", use escape syntax: e.g 'I'\''m Groot' (or double-quote if possible: "I'm Groot").

2. **Load design documents**: Read from FEATURE_DIR:
   - **Required**: plan.md (tech stack, libraries, structure), spec.md (user stories with priorities)
   - **Optional**: data-model.md (entities), contracts/ (API endpoints), research.md (decisions), quickstart.md (test scenarios)
   - Note: Not all projects have all documents. Generate shards based on what's available.

3. **Execute shard generation workflow**:
   - Load plan.md and extract tech stack, libraries, project structure
   - Load spec.md and extract user stories with their priorities (P1, P2, P3, etc.)
   - **Parse Cross-Spec Dependencies section** from spec.md (see format below)
   - If data-model.md exists: Extract entities and map to user stories
   - If contracts/ exists: Map endpoints to user stories
   - If research.md exists: Extract decisions for setup tasks
   - Generate shards organized by user story and phase
   - Create dependency chains showing completion order
   - **Resolve cross-spec dependencies** to actual shard IDs
   - Validate shard completeness (each user story has all needed work, independently testable)

4. **Generate shards using SQL commands**:
   - **Phase 1**: Create setup shards (project initialization, shared infrastructure)
   - **Phase 2**: Create foundational shards (blocking prerequisites for all user stories)
   - **Phase 3+**: Create one shard per user story (in priority order from spec.md)
   - **Optional**: Create sub-shards for complex user stories (tests, implementation, integration)
   - **Final**: Create polish shards for cross-cutting concerns
   - Set proper dependencies using `link()` function
   - Add detailed descriptions with task references

5. **Report**: Output summary of created shards:
   - Total shard count
   - Shard count per phase
   - Dependency chains established
   - Independent test criteria for each story
   - Suggested work order using `tasks_for()`

Context for shard generation: $ARGUMENTS

The shards should be immediately workable - each shard must be specific enough that an LLM can complete it without additional context.

## Shard Generation Rules

**CRITICAL**: Shards MUST be organized by user story to enable independent implementation and testing.

### Shard Structure

**Phase-Level Shards** (Recommended):
- Setup Phase → 1 shard
- Foundational Phase → 1 shard
- Each User Story → 1 shard (or 2-3 for complex stories)
- Polish Phase → 1 shard

**Sub-Task Shards** (For Complex Features):
- User Story Tests → separate shard (TDD approach)
- User Story Implementation → separate shard
- User Story Integration → separate shard

### Shard Properties

1. **Title**: Clear, descriptive name including feature and phase
   - Format: `[Feature Name]: [Phase] - [Story Description]`
   - Example: `Database Schema: Phase 3 - User Story 1 (Core Models)`
   - Example: `Gmail Integration: Setup Phase`

2. **Type**: Use `task` for all implementation shards

3. **Priority**: Match spec.md user story priorities
   - P1: Critical user stories, setup, foundational
   - P2: Important user stories
   - P3: Nice-to-have user stories
   - P4: Polish and optimizations

4. **Description**: Include specific tasks and file paths
   - Reference original tasks.md task IDs if available
   - List specific files to be created/modified
   - Include acceptance criteria
   - Reference test requirements if applicable

5. **Dependencies**: Use `relates-to` edges for grouping
   - Setup shards → no dependencies (ready immediately)
   - Foundational shards → relates-to setup
   - User story shards → relates-to foundational; only relate to other stories if truly required
   - Polish shards → relates-to ALL core user stories (creates convergence point)
   - Group shard → all children relate-to it

### Shard Creation Commands

Use these SQL commands via psql:

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

```sql
-- Create group shard for the feature
SELECT create_shard('penfold', '[GROUP] Feature Name: Implementation', 'Overview', 'task', 'agent-mycroft');
-- Returns: pf-group

-- Create shards (create ALL shards first, then link)
SELECT create_shard('penfold', 'Feature: Phase 1 - Setup', 'Detailed description', 'task', 'agent-mycroft');
-- Returns: pf-setup

SELECT create_shard('penfold', 'Feature: Phase 2 - Foundation', 'Description', 'task', 'agent-mycroft');
-- Returns: pf-foundation

-- Link shards to group
SELECT link('pf-setup', 'pf-group', 'relates-to');
SELECT link('pf-foundation', 'pf-group', 'relates-to');

-- View group structure
SELECT s.id, s.title, s.status FROM shards s
JOIN edges e ON s.id = e.from_id
WHERE e.to_id = 'pf-group' AND e.edge_type = 'relates-to';

-- Find available work
SELECT * FROM tasks_for('penfold', 'agent-mycroft');
```

### Phase Organization

- **Phase 1 - Setup**: Project initialization, directory structure, configuration
  - 1 shard for entire setup phase
  - Priority: P0
  - No dependencies (ready immediately)

- **Phase 2 - Foundational**: Blocking prerequisites for ALL user stories
  - 1 shard for foundational infrastructure
  - Priority: P0

- **Phase 3+ - User Stories**: One phase per user story from spec.md
  - Priority: Match spec.md (P1, P2, P3)
  - **PARALLEL WORK**: Only link to foundational unless story truly requires another story
  - Analyze each story: Does it NEED data/APIs from another story? If not, it can run in parallel
  - For complex stories, consider separate test/implementation shards

- **Final Phase - Polish**: Cross-cutting concerns, optimization, documentation
  - 1 shard for polish work
  - Priority: P3
  - Link to ALL user story shards (convergence point)
  - This is where parallel streams reunite

### Example Parallel Structure

```
Phase 1 (Setup) ─► Phase 2 (Foundation) ─┬─► US1 (P1) ─► US2 (P1) ─► US3 (P1) ─┬─► Polish ─► Group
                                         ├─► US4 (P2) ─────────────────────────┤
                                         └─► US5 (P3) ─────────────────────────┘
```

In this example:
- US1 → US2 → US3: Sequential (each builds on previous)
- US4, US5: Parallel with US1-3 (independent work streams)
- Polish: Blocked until US3, US4, AND US5 all complete

### Integration with Existing Workflow

- **Replace tasks.md**: Shards become the source of truth for implementation tracking
- **Update /speckit.implement-shards**: Uses SQL queries instead of bd commands
- **Preserve task details**: Include original task list in shard descriptions for reference
- **Maintain phases**: Same phase structure, just tracked in shards instead of markdown

### Quality Standards

Each shard must:
- Be independently testable when complete
- Have clear acceptance criteria in description
- Include specific file paths and components
- Map to user value (except setup/foundational)
- Be completable within reasonable timeframe (1-5 days ideal)
- Have proper links set to prevent blocking

### Cross-Spec Shard Dependencies

When a feature depends on work from another specification, add a `## Cross-Spec Shard Dependencies` section to spec.md to enable automatic cross-spec dependency creation.

#### Spec.md Format

Add this section after the Dependencies section:

```markdown
## Cross-Spec Shard Dependencies

<!--
  Format: this-phase → other-spec/other-phase
  Phases: Setup, Foundation, US1, US2, ..., Polish
  The shard generator will resolve these to actual shard IDs
-->

| This Phase | Depends On | Reason |
|------------|------------|--------|
| US5 (Search Integration) | 007-search-interface/US1 | Relationship queries need NL search infrastructure |
| Polish | 007-search-interface/Foundation | Integration tests need search API available |
```

#### Resolution Process

When generating shards:

1. **Parse the table**: Extract phase mappings from spec.md
2. **Find target shards**: Search for shards matching the other-spec pattern:
   ```sql
   SELECT id, title FROM shards
   WHERE project = 'penfold' AND title ILIKE '%search-interface%US1%';
   ```
3. **Create dependencies**: After creating this spec's shards:
   ```sql
   SELECT link('pf-this-shard', 'pf-other-shard', 'relates-to');
   ```
4. **Verify**: Show cross-spec dependencies in the summary

#### Benefits

- **Autonomous agents**: `tasks_for()` respects cross-spec dependencies
- **Correct ordering**: Can't start work that needs another spec's output
- **Visibility**: Full project dependency graph across all specs
- **Parallelism**: Independent work proceeds while waiting on cross-spec blockers
