---
description: Analyze project consistency, dependencies, and coverage using shards workflow instead of tasks.md
handoffs:
  - label: Implement Project
    agent: speckit.implement-shards
    prompt: Start the implementation using shards workflow
    send: true
---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Goal

Identify inconsistencies, duplications, ambiguities, and underspecified items across core artifacts (`spec.md`, `plan.md`) and shards before implementation. This command works with the shards workflow and analyzes actual shards created by `/speckit.shards`.

## Operating Constraints

**STRICTLY READ-ONLY**: Do **not** modify any files or shards. Output a structured analysis report. Offer an optional remediation plan (user must explicitly approve before any follow-up editing commands).

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

**From shards system (Context-Palace):**

```sql
-- Get all shards for feature
SELECT id, title, status, content, owner FROM shards
WHERE project = 'penfold' AND title ILIKE '%Feature Name%';

-- Get shard details
SELECT * FROM shards WHERE id = 'pf-xxx';

-- Get edges/links
SELECT * FROM edges WHERE from_id = 'pf-xxx' OR to_id = 'pf-xxx';
```

**Connection:**
```bash
psql "host=dev02.brown.chat dbname=contextpalace user=penfold sslmode=verify-full" -c "SQL"
```

### 3. Build Semantic Models

Create internal representations:

- **Requirements inventory**: Each functional requirement with stable key (FR-001 → `oauth2-authentication`)
- **User story inventory**: Each user story with priority and acceptance criteria
- **Shards coverage mapping**: Map each shard to requirements/stories by description analysis
- **Constitution compliance**: Extract principle violations and gate failures
- **Dependency validation**: Verify shard links match technical dependencies

### 4. Detection Passes

Focus on high-signal findings. Limit to 50 findings total.

#### A. Shards Coverage Analysis
- Requirements with zero associated shards
- Shards with no mapped requirement/story
- Missing shards for non-functional requirements (performance, security)
- Dependency gaps (shard links don't match technical requirements)

#### B. Specification Consistency
- Duplication in functional requirements
- Ambiguous requirements lacking measurable criteria
- Unresolved placeholders (TODO, NEEDS CLARIFICATION, etc.)
- Terminology drift across artifacts

#### C. Architecture Alignment
- Technical Context conflicts (different stack choices mentioned)
- Project Structure inconsistencies between plan.md and expected shard outputs
- Performance/constraint mismatches between spec and plan

#### D. Constitution Compliance
- Requirements or plans conflicting with MUST principles
- Missing constitutional gates (TDD, documentation, etc.)
- Quality standard violations

#### E. Shards Workflow Validation
- Shard titles/descriptions follow proper format
- Priority alignment between spec user stories and shard priorities
- Dependency chains properly established via relates-to edges
- Acceptance criteria mappable to shard completion criteria

### 5. Severity Assignment

- **CRITICAL**: Constitution MUST violation, missing core requirement coverage, dependency deadlock
- **HIGH**: Ambiguous requirement, conflicting architecture, untestable acceptance criterion
- **MEDIUM**: Terminology drift, missing non-functional coverage, unclear shard mapping
- **LOW**: Style improvements, minor redundancy, optimization opportunities

### 6. Produce Analysis Report

Output Markdown report with:

## Feature Specification Analysis Report

| ID | Category | Severity | Location(s) | Summary | Recommendation |
|----|----------|----------|-------------|---------|----------------|
| A1 | Coverage | HIGH | FR-005, No Shards | Security requirement has no implementation shards | Create security-focused shard |

**Requirements Coverage Analysis:**

| Requirement | Has Shards? | Shard IDs | User Story | Notes |
|-------------|------------|----------|------------|-------|
| FR-001: OAuth2 | ✅ | pf-y71 | Story 1 | Complete coverage |
| FR-002: API Integration | ❌ | None | Story 2 | Missing implementation |

**Shards Alignment Check:**

| Shard ID | Title | Maps To | Priority Match | Links Valid |
|---------|-------|---------|----------------|-------------|
| pf-y71 | OAuth2 Auth | FR-001, Story 1 | ✅ P1→P1 | ✅ |

**Constitution Compliance:**
- ✅ All MUST principles satisfied
- ⚠️ TDD requirements need explicit test shards

**Dependency Validation:**
- ✅ Shard links form valid DAG
- ❌ Missing link: Email sync requires OAuth completion

**Metrics:**
- Total Functional Requirements: X
- Total User Stories: Y
- Total Shards: Z
- Coverage %: (requirements with >=1 shard)
- Critical Issues: N

### 7. Shards-Specific Analysis

**Shard Quality Assessment:**
- Title format compliance (`Feature: Phase - Description`)
- Description completeness (tasks, files, acceptance criteria)
- Priority alignment with spec.md user story priorities
- Dependency chain completeness via relates-to edges

**Implementation Readiness:**
- All P1 requirements have shards
- Setup/foundational shards properly sequenced
- Integration shards link to component shards
- Polish/documentation shards scheduled appropriately

### 8. Provide Next Actions

**If CRITICAL issues exist:**
- "Resolve critical issues before `/speckit.implement-shards`"
- Specific remediation commands

**If ready for implementation:**
- "Ready for `/speckit.implement-shards`"
- "Start with `SELECT * FROM tasks_for('penfold', 'agent-penfdev');` to see available work"

**Improvement suggestions:**
- Missing shard creation commands
- Specification clarifications needed
- Architecture adjustments recommended

### 9. Offer Remediation

Ask: "Would you like me to suggest specific shard creation or specification edits for the top N issues?"

## Key Differences from Original Analyze

1. **Shards Integration**: Analyzes actual shards in Context-Palace instead of tasks.md
2. **Dynamic Coverage**: Uses SQL queries to get current shard status
3. **Dependency Validation**: Verifies shard links (relates-to edges) match technical requirements
4. **Implementation Readiness**: Assesses whether shards provide complete implementation path
5. **Workflow Validation**: Ensures shards follow proper format and organization

## Shard Query Integration

```sql
-- Get all shards for analysis
SELECT id, title, status, owner FROM shards WHERE project = 'penfold';

-- Get feature-specific shards
SELECT id, title, status FROM shards
WHERE project = 'penfold' AND title ILIKE '%Gmail Integration%';

-- Analyze specific shard
SELECT * FROM shards WHERE id = 'pf-xxx';

-- Check ready work
SELECT * FROM tasks_for('penfold', 'agent-penfdev');

-- Validate dependencies (edges)
SELECT e.from_id, e.to_id, e.edge_type, s.title
FROM edges e
JOIN shards s ON e.from_id = s.id
WHERE e.to_id = 'pf-group';
```

This command provides comprehensive analysis while working seamlessly with the shards workflow, ensuring specifications are implementation-ready through actual shards tracking.
