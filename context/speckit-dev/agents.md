# SpecKit Development Agent Context

This context enables AI agents to execute the complete SpecKit workflow for feature specification, planning, and implementation using the beads-based task tracking system.

## Complete Feature Lifecycle (End-to-End)

**This agent can autonomously complete an entire feature from description to archived documentation.**

### Full Autonomous Workflow

```bash
# PHASE 1: SPECIFICATION
/speckit.specify <feature description>
# Creates: specs/<NNN>-<feature>/spec.md, git branch

/speckit.clarify  # If [NEEDS CLARIFICATION] markers exist
# Updates: spec.md with resolved ambiguities

# PHASE 2: PLANNING
/speckit.plan
# Creates: plan.md, data-model.md, contracts/, research.md

# PHASE 3: WORK BREAKDOWN
/speckit.beads
# Creates: Beads for Setup → Foundational → User Stories → Polish
# Sets: Dependencies between beads

# PHASE 4: IMPLEMENTATION (iterate until all beads closed)
bd ready                              # Find work
bd update <id> --status=in_progress   # Claim
# ... implement with TDD ...
bd close <id> --reason="commit: ..."  # Complete
bd ready                              # Next work
# Repeat until no feature beads remain

# PHASE 5: VALIDATION
/speckit.analyze
# Validates: Spec ↔ Plan ↔ Implementation consistency

# PHASE 6: CONSOLIDATION
# Create: ARCHIVE.md, update ARCHITECTURE.md
# Create: context/<feature>-dev/agents.md (if patterns worth preserving)
# Create: docs/<feature>/ documentation

# PHASE 7: MERGE & CLEANUP
git checkout main
git merge <feature-branch>
git push
git push origin --delete <feature-branch>
bd sync
```

### Autonomous Implementation Loop

The core implementation loop that processes all beads:

```python
# Pseudo-code for autonomous bead processing
while True:
    ready_beads = bd_ready()  # Get available work
    feature_beads = filter_by_feature(ready_beads, feature_name)

    if not feature_beads:
        # All feature work complete
        break

    bead = select_highest_priority(feature_beads)

    # Claim work
    bd_update(bead.id, status="in_progress")

    # Implement (TDD approach)
    write_failing_tests(bead.acceptance_criteria)
    implement_to_pass_tests(bead.description)
    refactor_if_needed()

    # Commit with bead reference
    git_commit(f"feat({component}): {summary} [{bead.id}]")

    # Complete work
    bd_close(bead.id, reason=f"commit {hash}: {summary}")

    # Sync progress
    bd_sync()
    git_push()
```

### Feature Completion Checklist

Before considering a feature complete:

- [ ] **Specification**: spec.md has no [NEEDS CLARIFICATION] markers
- [ ] **Planning**: plan.md, data-model.md exist and are consistent
- [ ] **Beads**: All feature-related beads closed (`bd list --status=closed | grep "<feature>"`)
- [ ] **Tests**: All tests passing (`pytest`)
- [ ] **Validation**: `/speckit.analyze` passes with no issues
- [ ] **Archive**: `specs/<NNN>-<feature>/ARCHIVE.md` created
- [ ] **Patterns**: New patterns extracted to `context/ARCHITECTURE.md`
- [ ] **Documentation**: `docs/<feature>/` created with README
- [ ] **Agent Context**: `context/<feature>-dev/agents.md` if warranted
- [ ] **Git**: Branch merged to main and pruned
- [ ] **Beads**: Consolidation bead closed

## Agent Expertise

**Primary Skills**: Feature specification, technical planning, bead-based task management, TDD implementation

**Key Responsibilities**:
- Transform natural language feature descriptions into formal specifications
- Generate technical implementation plans with proper phasing
- Create dependency-ordered beads for implementation tracking
- Execute implementation following bead workflow
- Maintain consistency across spec artifacts

## SpecKit Beads Workflow (Complete Pipeline)

### Phase 1: Specification (`/speckit.specify`)

**Purpose**: Transform feature description into formal specification

**Command**: `/speckit.specify <feature description>`

**Outputs**:
- `specs/<NNN>-<feature-name>/spec.md` - User stories, requirements, success criteria
- Git branch `<NNN>-<feature-name>`

**Key Sections Generated**:
- Problem Statement
- User Scenarios & Testing (with Gherkin-style acceptance criteria)
- Functional Requirements (FR-001, FR-002, etc.)
- Non-Functional Requirements
- Success Criteria (measurable outcomes)

**Best Practices**:
- Limit [NEEDS CLARIFICATION] markers to max 3
- Each requirement must be testable
- Success criteria must be measurable and technology-agnostic

### Phase 2: Clarification (`/speckit.clarify`)

**Purpose**: Identify and resolve underspecified areas in the spec

**Command**: `/speckit.clarify`

**Process**:
1. Analyze spec.md for ambiguities
2. Generate up to 5 targeted clarification questions
3. Encode user answers back into spec.md
4. Update [NEEDS CLARIFICATION] markers

**When to Use**:
- After initial specification if marked areas exist
- When starting implementation reveals gaps
- Before technical planning if requirements unclear

### Phase 3: Technical Planning (`/speckit.plan`)

**Purpose**: Create implementation roadmap with architecture decisions

**Command**: `/speckit.plan`

**Outputs**:
- `specs/<feature>/plan.md` - Technical implementation plan
- `specs/<feature>/data-model.md` - Entity definitions (if applicable)
- `specs/<feature>/contracts/` - API specifications (if applicable)
- `specs/<feature>/research.md` - Technical decisions and alternatives

**Plan Structure**:
- Tech stack and dependencies
- Project structure and file organization
- Implementation phases (Setup → Foundational → User Stories → Polish)
- Risk assessment and mitigation

### Phase 4: Bead Generation (`/speckit.beads`)

**Purpose**: Create trackable, dependency-ordered work items

**Command**: `/speckit.beads`

**Bead Structure**:
```
Phase 1 - Setup (P1, no dependencies)
    └── 1 bead: Project initialization, directory structure, config

Phase 2 - Foundational (P1, depends on Setup)
    └── 1 bead: Shared infrastructure, base components

Phase 3+ - User Stories (P1-P3, depends on Foundational)
    └── 1 bead per user story (or 2-3 for complex stories)
    └── Priority matches spec.md user story priority

Final Phase - Polish (P3-P4, depends on core stories)
    └── 1 bead: Documentation, optimization, cleanup
```

**Bead Creation Commands**:
```bash
# Create bead
bd create --title="Feature: Phase - Description" \
          --type=task \
          --priority=1 \
          --description="Detailed tasks and file paths"

# Set dependencies
bd dep add <dependent-bead-id> <dependency-bead-id>

# Verify work queue
bd ready
```

**Bead Quality Standards**:
- Independently testable when complete
- Clear acceptance criteria in description
- Specific file paths and components listed
- Completable within 1-5 days
- Proper dependency chains

### Phase 5: Implementation (`/speckit.implement-beads`)

**Purpose**: Execute implementation following bead workflow

**Command**: `/speckit.implement-beads`

**Workflow**:
```bash
# 1. Find available work
bd ready

# 2. Claim work
bd update <bead-id> --status=in_progress

# 3. Implement (TDD approach)
#    - Write failing tests first
#    - Implement to pass tests
#    - Refactor if needed

# 4. Complete work
bd close <bead-id> --reason="commit <hash>: <summary>"

# 5. Check for newly unblocked work
bd ready
```

**Implementation Rules**:
- Work on ONE bead at a time
- Only work on beads shown by `bd ready` (no blockers)
- Write tests before implementation (TDD)
- Update bead status religiously
- Validate acceptance criteria before closing

### Phase 6: Analysis (`/speckit.analyze`)

**Purpose**: Cross-artifact consistency and quality check

**Command**: `/speckit.analyze`

**Checks Performed**:
- Spec ↔ Plan alignment
- Plan ↔ Beads coverage
- User story completeness
- Dependency chain validity
- Success criteria measurability

## Quick Reference Commands

### Full Workflow (New Feature)
```bash
/speckit.specify <feature description>  # Create spec
/speckit.clarify                        # Resolve ambiguities
/speckit.plan                           # Technical planning
/speckit.beads                          # Generate work items
/speckit.implement-beads                # Execute implementation
```

### Bead Management
```bash
bd ready                    # Available work (no blockers)
bd list --status=open       # All open beads
bd show <id>                # Bead details
bd update <id> --status=in_progress  # Claim work
bd close <id> --reason="..."         # Complete work
bd dep add <child> <parent> # Set dependency
bd stats                    # Project health
bd sync                     # Sync with git
```

### Session Protocol
```bash
# Start of session
bd ready                    # Find work

# During work
bd update <id> --status=in_progress  # Claim
# ... implement ...
bd close <id> --reason="..."         # Complete

# End of session
bd sync                     # Sync beads
git add . && git commit     # Commit code
git push                    # MUST push to remote
```

## Architectural Patterns

### User Story Organization

**Pattern**: One bead per user story enables independent implementation and testing

```
spec.md User Stories:
├── US1 (P0) - Core functionality
├── US2 (P0) - Essential feature
├── US3 (P1) - Important enhancement
└── US4 (P2) - Nice-to-have

Generated Beads:
├── Setup Phase (P1) - depends: none
├── Foundational (P1) - depends: Setup
├── US1 Implementation (P1) - depends: Foundational
├── US2 Implementation (P1) - depends: Foundational
├── US3 Implementation (P2) - depends: US1, US2
├── US4 Implementation (P3) - depends: US1
└── Polish Phase (P4) - depends: US1, US2, US3
```

### TDD Implementation Flow

**Pattern**: Tests first for each bead

```python
# 1. Write failing test (from acceptance criteria)
def test_user_can_create_account():
    """US1: Given no existing account, When user submits valid info..."""
    result = create_account(valid_user_data)
    assert result.success
    assert result.user.email == valid_user_data["email"]

# 2. Run test - should FAIL
# pytest tests/unit/test_account.py -v

# 3. Implement minimum code to pass
# 4. Run test - should PASS
# 5. Refactor if needed
# 6. Close bead with commit reference
```

### Dependency Chain Management

**Pattern**: Proper dependencies prevent blocked work

```bash
# Setup has no dependencies
bd create --title="Feature: Setup" --priority=1

# Foundational depends on Setup
bd create --title="Feature: Foundational" --priority=1
bd dep add <foundational-id> <setup-id>

# User stories depend on Foundational
bd create --title="Feature: US1 - Core" --priority=1
bd dep add <us1-id> <foundational-id>

# Later stories may depend on earlier ones
bd create --title="Feature: US3 - Enhancement" --priority=2
bd dep add <us3-id> <us1-id>
```

## Error Handling

### Blocked Bead
```bash
# Document the blocker
bd update <id> --description="BLOCKED: <reason>. Original: <description>"

# Create fix bead if needed
bd create --title="Fix: <issue>" --type=bug --priority=1
bd dep add <blocked-id> <fix-id>
```

### Failed Implementation
```bash
# Don't close the bead - keep in_progress
# Document failure in bead
bd update <id> --description="FAILED: <error>. <original>"

# Create investigation bead if needed
bd create --title="Investigate: <issue>" --type=task --priority=1
```

### Missing Dependencies
```bash
# Check what's blocking
bd show <id>  # Shows DEPENDS ON section

# Verify dependency is closeable
bd show <dependency-id>

# Work on dependency first
bd update <dependency-id> --status=in_progress
```

## Integration with Project Workflow

### Commit Messages
```bash
# Reference bead in commits
git commit -m "feat(component): description [pe-xxx]"

# Close bead with commit hash
bd close pe-xxx --reason="commit abc123: Implemented feature"
```

### Session End Protocol
```bash
# 1. Check status
git status
bd stats

# 2. Sync everything
bd sync
git add .
git commit -m "feat: progress on feature [pe-xxx]"
git push  # MANDATORY - never leave work local
```

### Branch Management
```bash
# Feature branches created by /speckit.specify
# Format: <NNN>-<feature-name>
# Example: 012-user-authentication

# Merge when all feature beads closed
git checkout main
git merge <feature-branch>
git push
git push origin --delete <feature-branch>  # Prune after merge
```

## Post-Implementation: Consolidation & Archiving

### Phase 7: Documentation & Consolidation

**Purpose**: Capture implementation knowledge and create reusable artifacts

**Tasks**:
1. **Create ARCHIVE.md** in spec directory
2. **Extract patterns** to `context/ARCHITECTURE.md`
3. **Create dev agent** context if significant patterns emerged
4. **Create user documentation** in `docs/<feature>/`

### Creating ARCHIVE.md

**Location**: `specs/<NNN>-<feature>/ARCHIVE.md`

**Template**:
```markdown
# <Feature Name> Specification - ARCHIVED

**Archived Date**: YYYY-MM-DD
**Status**: COMPLETED - Consolidated into operational documentation
**Implementation**: Successfully implemented and patterns extracted

## Archival Summary

<Brief description of what was implemented>

### Implementation Achievements

✅ **<Achievement 1>** - <Details>
✅ **<Achievement 2>** - <Details>
...

### Success Criteria Achieved

| Criterion | Target | Achieved | Status |
|-----------|--------|----------|--------|
| <Metric>  | <Target> | <Actual> | ✅/❌ |

### Patterns Extracted to Architecture

- Pattern 1: <Name and location>
- Pattern 2: <Name and location>

### Documentation Created

- `context/<feature>-dev/agents.md` - Agent context
- `docs/<feature>/README.md` - User documentation
- `docs/<feature>/quickstart.md` - Getting started

### Lessons Learned

1. **<Lesson>**: <Details>
2. **<Lesson>**: <Details>

## References

- Implementation: `<lib>/`
- Agent Context: `context/<feature>-dev/agents.md`
- Architecture Patterns: `context/ARCHITECTURE.md`
```

### Extracting Patterns to ARCHITECTURE.md

**When to Extract**:
- Pattern used in 2+ places
- Novel solution to common problem
- Performance-critical implementation
- Security-sensitive approach

**Pattern Template**:
```markdown
### N. <Pattern Name>

**Pattern**: <One-line description>

**Implementation Details**:
- <Detail 1>
- <Detail 2>

**Key Components**:
- <Component and purpose>

\`\`\`python
# Code example showing the pattern
\`\`\`
```

**Update Header**:
```markdown
**Extracted from implementations**: ..., <NNN>-<new-feature>
```

### Creating Dev Agent Context

**When to Create**: Feature has reusable patterns worth preserving

**Location**: `context/<feature>-dev/agents.md`

**Structure**:
```markdown
# <Feature> Development Agent Context

## Agent Expertise
- Primary skills
- Key responsibilities

## Architectural Patterns (Production-Proven)
- Pattern implementations with code examples

## Quick Reference
- Common commands
- Key file locations

## Integration Points
- How this connects to other systems
```

### Creating User Documentation

**Location**: `docs/<feature>/`

**Required Files**:
- `README.md` - Overview and quick start
- Additional guides as needed (setup, API, troubleshooting)

**README.md Template**:
```markdown
# <Feature Name>

<One paragraph description>

## Overview

<What this feature provides>

## Quick Start

### Prerequisites
- <Requirement 1>
- <Requirement 2>

### Installation
\`\`\`bash
<installation commands>
\`\`\`

### Basic Usage
\`\`\`python
<usage example>
\`\`\`

## Components

### <Component 1>
<Description and usage>

## CLI Commands
\`\`\`bash
<available commands>
\`\`\`

## Configuration

<Configuration options>

## Related Documentation

- [Architecture Patterns](../../context/ARCHITECTURE.md)
- [Agent Context](../../context/<feature>-dev/agents.md)
```

### Consolidation Workflow

**Complete Checklist**:
```bash
# 1. Create archive
# Write specs/<NNN>-<feature>/ARCHIVE.md

# 2. Extract patterns to ARCHITECTURE.md
# Add new patterns with incrementing numbers
# Update "Extracted from" header

# 3. Create dev agent context (if warranted)
mkdir -p context/<feature>-dev
# Write context/<feature>-dev/agents.md

# 4. Create user documentation
mkdir -p docs/<feature>
# Write docs/<feature>/README.md
# Copy/adapt quickstart.md if exists in spec

# 5. Update cross-references
# Ensure ARCHIVE.md references all created docs
# Ensure docs reference ARCHITECTURE.md patterns

# 6. Commit and push
git add specs/<NNN>-<feature>/ARCHIVE.md
git add context/<feature>-dev/
git add docs/<feature>/
git add context/ARCHITECTURE.md
git commit -m "docs(<feature>): consolidate and archive specification [pe-xxx]"
git push
```

### Closing Consolidation Beads

**Pattern**: Create consolidation bead when feature implementation completes

```bash
# Create consolidation bead
bd create --title="Consolidate & Document: <Feature>" \
          --type=task \
          --priority=2 \
          --description="Final consolidation:
1. Create ARCHIVE.md with implementation summary
2. Extract patterns to ARCHITECTURE.md
3. Create context/<feature>-dev/agents.md
4. Create docs/<feature>/ documentation
5. Update cross-references"

# Set dependency on feature completion
bd dep add <consolidation-id> <last-feature-bead-id>
```

## Quality Checklist

Before closing any bead:
- [ ] All acceptance criteria from description met
- [ ] Tests written and passing
- [ ] Code follows project patterns (check ARCHITECTURE.md)
- [ ] No ruff/mypy warnings
- [ ] Bead description tasks completed
- [ ] Commit references bead ID

Before completing feature:
- [ ] All feature beads closed
- [ ] `/speckit.analyze` passes
- [ ] Integration tests pass
- [ ] Documentation updated
- [ ] Branch merged and pruned
