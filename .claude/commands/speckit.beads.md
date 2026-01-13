---
description: Generate actionable, dependency-ordered beads for the feature based on available design artifacts, replacing tasks.md workflow.
handoffs:
  - label: Analyze For Consistency
    agent: speckit.analyze
    prompt: Run a project analysis for consistency
    send: true
  - label: Implement Project
    agent: speckit.implement
    prompt: Start the implementation in phases using bead tracking
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
   - Note: Not all projects have all documents. Generate beads based on what's available.

3. **Execute bead generation workflow**:
   - Load plan.md and extract tech stack, libraries, project structure
   - Load spec.md and extract user stories with their priorities (P1, P2, P3, etc.)
   - If data-model.md exists: Extract entities and map to user stories
   - If contracts/ exists: Map endpoints to user stories
   - If research.md exists: Extract decisions for setup tasks
   - Generate beads organized by user story and phase
   - Create dependency chains showing completion order
   - Validate bead completeness (each user story has all needed work, independently testable)

4. **Generate beads using bd commands**:
   - **Phase 1**: Create setup beads (project initialization, shared infrastructure)
   - **Phase 2**: Create foundational beads (blocking prerequisites for all user stories)
   - **Phase 3+**: Create one bead per user story (in priority order from spec.md)
   - **Optional**: Create sub-beads for complex user stories (tests, implementation, integration)
   - **Final**: Create polish beads for cross-cutting concerns
   - Set proper dependencies using `bd dep add` commands
   - Set priorities (P1, P2, P3) matching spec.md user story priorities
   - Add detailed descriptions with task references

5. **Report**: Output summary of created beads:
   - Total bead count
   - Bead count per phase
   - Dependency chains established
   - Independent test criteria for each story
   - Suggested work order using `bd ready`
   - Instructions for updating /speckit.implement to work with beads

Context for bead generation: $ARGUMENTS

The beads should be immediately workable - each bead must be specific enough that an LLM can complete it without additional context.

## Bead Generation Rules

**CRITICAL**: Beads MUST be organized by user story to enable independent implementation and testing.

### Bead Structure

**Phase-Level Beads** (Recommended):
- Setup Phase → 1 bead
- Foundational Phase → 1 bead
- Each User Story → 1 bead (or 2-3 for complex stories)
- Polish Phase → 1 bead

**Sub-Task Beads** (For Complex Features):
- User Story Tests → separate bead (TDD approach)
- User Story Implementation → separate bead
- User Story Integration → separate bead

### Bead Properties

1. **Title**: Clear, descriptive name including feature and phase
   - Format: `[Feature Name]: [Phase] - [Story Description]`
   - Example: `Database Schema: Phase 3 - User Story 1 (Core Models)`
   - Example: `Gmail Integration: Setup Phase`

2. **Type**:
   - `epic` for high-level feature completion
   - `task` for implementation phases
   - `bug` for fixes (rarely used in SpecKit)

3. **Priority**: Match spec.md user story priorities
   - P1 (0-1): Critical user stories, setup, foundational
   - P2 (2): Important user stories
   - P3 (3): Nice-to-have user stories
   - P4 (4): Polish and optimizations

4. **Description**: Include specific tasks and file paths
   - Reference original tasks.md task IDs if available
   - List specific files to be created/modified
   - Include acceptance criteria
   - Reference test requirements if applicable

5. **Dependencies**: Proper dependency chains
   - Setup beads → no dependencies
   - Foundational beads → depend on setup
   - User story beads → depend on foundational + previous stories if needed
   - Polish beads → depend on core user stories

### Bead Creation Commands

Use these bd commands in sequence:

```bash
# Create beads
bd create --title="Feature: Phase - Description" --type=task --priority=1 --description="Detailed description with tasks and file paths"

# Set dependencies (after all beads created)
bd dep add [dependent-bead-id] [dependency-bead-id]

# Verify ready work
bd ready
```

### Phase Organization

- **Phase 1 - Setup**: Project initialization, directory structure, configuration
  - 1 bead for entire setup phase
  - Priority: P1 (0)
  - No dependencies

- **Phase 2 - Foundational**: Blocking prerequisites for ALL user stories
  - 1 bead for foundational infrastructure
  - Priority: P1 (0)
  - Depends on: Setup phase

- **Phase 3+ - User Stories**: One phase per user story from spec.md
  - Priority: Match spec.md (P1=1, P2=2, P3=3)
  - Depends on: Foundational + any prerequisite stories
  - For complex stories, consider separate test/implementation beads

- **Final Phase - Polish**: Cross-cutting concerns, optimization, documentation
  - 1 bead for polish work
  - Priority: P3-P4 (3-4)
  - Depends on: Core user stories

### Integration with Existing Workflow

- **Replace tasks.md**: Beads become the source of truth for implementation tracking
- **Update /speckit.implement**: Modify to read from bd commands instead of tasks.md
- **Preserve task details**: Include original task list in bead descriptions for reference
- **Maintain phases**: Same phase structure, just tracked in beads instead of markdown

### Quality Standards

Each bead must:
- Be independently testable when complete
- Have clear acceptance criteria in description
- Include specific file paths and components
- Map to user value (except setup/foundational)
- Be completable within reasonable timeframe (1-5 days ideal)
- Have proper dependencies set to prevent blocking