---
description: Analyze project consistency, dependencies, and coverage using beads workflow instead of tasks.md
handoffs:
  - label: Implement Project
    agent: speckit.implement-beads
    prompt: Start the implementation using beads workflow
    send: true
---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Goal

Identify inconsistencies, duplications, ambiguities, and underspecified items across core artifacts (`spec.md`, `plan.md`) and beads before implementation. This command works with the beads workflow and analyzes actual beads created by `/speckit.beads`.

## Operating Constraints

**STRICTLY READ-ONLY**: Do **not** modify any files or beads. Output a structured analysis report. Offer an optional remediation plan (user must explicitly approve before any follow-up editing commands).

**Constitution Authority**: The project constitution (`.specify/memory/constitution.md`) is **non-negotiable** within this analysis scope. Constitution conflicts are automatically CRITICAL.

## Execution Steps

### 1. Initialize Analysis Context

Run `.specify/scripts/bash/check-prerequisites.sh --json` from repo root and parse JSON for FEATURE_DIR and AVAILABLE_DOCS. Derive absolute paths:

- SPEC = FEATURE_DIR/spec.md
- PLAN = FEATURE_DIR/plan.md
- CONSTITUTION = .specify/memory/constitution.md

For single quotes in args like "I'm Groot", use escape syntax: e.g 'I'\''m Groot' (or double-quote if possible: "I'm Groot").

### 2. Load Artifacts

**From spec.md:**
- Overview/Context
- Functional Requirements (FR-001, FR-002, etc.)
- User Stories with priorities and acceptance scenarios
- Success Criteria with measurable outcomes
- Edge Cases (if present)

**From plan.md:**
- Technical Context (language, dependencies, constraints)
- Architecture/stack choices
- Project Structure
- Constitution Check results

**From constitution:**
- Load `.specify/memory/constitution.md` for principle validation

**From beads system:**
- Use `bd list --status=all` to get all feature beads
- Filter beads related to current feature (check titles for feature name)
- Use `bd show [bead-id]` to get detailed information for each relevant bead
- Map beads to user stories and functional requirements

### 3. Build Semantic Models

Create internal representations:

- **Requirements inventory**: Each functional requirement with stable key (FR-001 → `oauth2-authentication`)
- **User story inventory**: Each user story with priority and acceptance criteria
- **Beads coverage mapping**: Map each bead to requirements/stories by description analysis
- **Constitution compliance**: Extract principle violations and gate failures
- **Dependency validation**: Verify bead dependencies match technical dependencies

### 4. Detection Passes

Focus on high-signal findings. Limit to 50 findings total.

#### A. Beads Coverage Analysis
- Requirements with zero associated beads
- Beads with no mapped requirement/story
- Missing beads for non-functional requirements (performance, security)
- Dependency gaps (bead dependencies don't match technical requirements)

#### B. Specification Consistency
- Duplication in functional requirements
- Ambiguous requirements lacking measurable criteria
- Unresolved placeholders (TODO, NEEDS CLARIFICATION, etc.)
- Terminology drift across artifacts

#### C. Architecture Alignment
- Technical Context conflicts (different stack choices mentioned)
- Project Structure inconsistencies between plan.md and expected bead outputs
- Performance/constraint mismatches between spec and plan

#### D. Constitution Compliance
- Requirements or plans conflicting with MUST principles
- Missing constitutional gates (TDD, documentation, etc.)
- Quality standard violations

#### E. Beads Workflow Validation
- Bead titles/descriptions follow proper format
- Priority alignment between spec user stories and bead priorities
- Dependency chains properly established
- Acceptance criteria mappable to bead completion criteria

### 5. Severity Assignment

- **CRITICAL**: Constitution MUST violation, missing core requirement coverage, dependency deadlock
- **HIGH**: Ambiguous requirement, conflicting architecture, untestable acceptance criterion
- **MEDIUM**: Terminology drift, missing non-functional coverage, unclear bead mapping
- **LOW**: Style improvements, minor redundancy, optimization opportunities

### 6. Produce Analysis Report

Output Markdown report with:

## Gmail Integration Specification Analysis Report

| ID | Category | Severity | Location(s) | Summary | Recommendation |
|----|----------|----------|-------------|---------|----------------|
| A1 | Coverage | HIGH | FR-005, No Beads | Security requirement has no implementation beads | Create security-focused bead |

**Requirements Coverage Analysis:**

| Requirement | Has Beads? | Bead IDs | User Story | Notes |
|-------------|------------|----------|------------|-------|
| FR-001: OAuth2 | ✅ | pe-y71 | Story 1 | Complete coverage |
| FR-002: API Integration | ❌ | None | Story 2 | Missing implementation |

**Beads Alignment Check:**

| Bead ID | Title | Maps To | Priority Match | Dependencies Valid |
|---------|-------|---------|----------------|-------------------|
| pe-y71 | OAuth2 Auth | FR-001, Story 1 | ✅ P1→P1 | ✅ |

**Constitution Compliance:**
- ✅ All MUST principles satisfied
- ⚠️ TDD requirements need explicit test beads

**Dependency Validation:**
- ✅ Bead dependencies form valid DAG
- ❌ Missing dependency: Email sync requires OAuth completion

**Metrics:**
- Total Functional Requirements: X
- Total User Stories: Y
- Total Beads: Z
- Coverage %: (requirements with >=1 bead)
- Critical Issues: N

### 7. Beads-Specific Analysis

**Bead Quality Assessment:**
- Title format compliance (`Feature: Phase - Description`)
- Description completeness (tasks, files, acceptance criteria)
- Priority alignment with spec.md user story priorities
- Dependency chain completeness

**Implementation Readiness:**
- All P1 requirements have beads
- Setup/foundational beads properly sequenced
- Integration beads depend on component beads
- Polish/documentation beads scheduled appropriately

### 8. Provide Next Actions

**If CRITICAL issues exist:**
- "Resolve critical issues before `/speckit.implement-beads`"
- Specific remediation commands

**If ready for implementation:**
- "Ready for `/speckit.implement-beads`"
- "Start with `bd ready` to see available work"

**Improvement suggestions:**
- Missing bead creation commands
- Specification clarifications needed
- Architecture adjustments recommended

### 9. Offer Remediation

Ask: "Would you like me to suggest specific bead creation or specification edits for the top N issues?"

## Key Differences from Original Analyze

1. **Beads Integration**: Analyzes actual beads instead of tasks.md
2. **Dynamic Coverage**: Uses `bd` commands to get current bead status
3. **Dependency Validation**: Verifies bead dependencies match technical requirements
4. **Implementation Readiness**: Assesses whether beads provide complete implementation path
5. **Workflow Validation**: Ensures beads follow proper format and organization

## Beads Command Integration

```bash
# Get all beads for analysis
bd list --status=all

# Get feature-specific beads
bd list --status=all | grep "Gmail Integration"

# Analyze specific bead
bd show [bead-id]

# Check ready work
bd ready

# Validate dependencies
bd dep list
```

This command provides comprehensive analysis while working seamlessly with the beads workflow, ensuring specifications are implementation-ready through actual beads tracking.