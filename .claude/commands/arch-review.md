# Architecture Review Agent

Perform an iterative, multi-pass architecture review of the codebase, building insights across passes.

## Arguments: $ARGUMENTS

Optional: Specific focus area or component to emphasize (e.g., "data pipeline", "API layer")

## Instructions

### Setup

1. Create a timestamped review folder:

```bash
REVIEW_DIR="review/arch/$(date -u +%Y-%m-%dT%H-%M-%SZ)"
mkdir -p "$REVIEW_DIR"
echo "Review directory: $REVIEW_DIR"
```

2. Store the directory path for use across passes.

### Review Passes

Execute 8 sequential review passes using the **Opus model**. Each pass must:
- Read all previous pass files before starting
- Focus on its designated lens
- Identify NEW insights not covered in prior passes
- Write findings to its designated file

Use the Task tool with `model: "opus"` for each pass.

#### Pass 0: Context & Goals Understanding
**File:** `$REVIEW_DIR/pass-00-context.md`
**Focus:** Understand the system's purpose, design goals, and validation criteria BEFORE reviewing architecture.
**Prompt for agent:**
```
You are an expert architect preparing to review a codebase. Before analyzing the architecture, you MUST understand what the system is trying to achieve.

Read these foundational documents thoroughly:

1. **Project Constitution** (primary authority):
   - project-constitution.md
   - .specify/memory/constitution.md (if exists)

2. **Architecture Documentation**:
   - context/ARCHITECTURE.md
   - CLAUDE.md

3. **Feature Specifications** (sample 2-3 to understand scope):
   - Glob for specs/**/spec.md
   - Read a few to understand feature patterns

Extract and document:

## Core Mission
- What is this system trying to achieve?
- Who is the user? What problems do they face?
- What does success look like? (specific metrics)

## Design Principles
- What principles guide architectural decisions?
- What trade-offs are explicitly prioritized?
- What constraints exist (privacy, performance, etc.)?

## Validation Criteria
- How should features/architecture be evaluated?
- What are the quality gates?
- What would violate the constitution?

## Current Architecture Intent
- What patterns are intentionally chosen?
- What is local-first vs cloud?
- What is the AI processing strategy?

## Review Lens
Based on the above, define what "good architecture" means FOR THIS SPECIFIC SYSTEM.
This will guide all subsequent review passes.

Write your context summary to: $REVIEW_DIR/pass-00-context.md

Format:
# Architecture Review: Context & Goals
## System Mission
### Core Purpose
### Target User & Pain Points
### Success Metrics
## Design Principles (from Constitution)
### Prioritized Principles
### Trade-off Resolution Order
### Explicit Constraints
## Validation Framework
### Feature Acceptance Criteria
### Architecture Decision Criteria
### Red Flags / Rejection Criteria
## Architectural Intent
### Chosen Patterns
### Local vs Cloud Strategy
### AI Processing Approach
## Review Criteria for This System
Based on the constitution and goals, this review will evaluate architecture against:
1. [specific criterion derived from constitution]
2. [specific criterion]
3. ...
```

#### Pass 1: Structure & Patterns
**File:** `$REVIEW_DIR/pass-01-structure.md`
**Focus:** Overall architecture, design patterns, module organization, dependency structure, code organization conventions.
**Prompt for agent:**
```
You are an expert software architect reviewing a codebase for structural patterns.

FIRST, read the context document to understand what this system is trying to achieve:
- $REVIEW_DIR/pass-00-context.md

Then review the codebase and analyze:
1. Overall architecture style (monolith, microservices, modular monolith, etc.)
2. Design patterns in use (and their appropriateness FOR THIS SYSTEM'S GOALS)
3. Module/package organization and boundaries
4. Dependency graph and coupling between components
5. Code organization conventions and consistency

Read key files: pyproject.toml, directory structure, main entry points.
Explore src/, lib/, or equivalent directories.

IMPORTANT: Evaluate against the system's stated goals and principles from Pass 0, not generic best practices.

Write your findings to: $REVIEW_DIR/pass-01-structure.md

Format:
# Architecture Review: Structure & Patterns
## Summary
## Alignment with System Goals
(How well does the structure support the mission from Pass 0?)
## Findings
### Strengths
### Concerns
### Recommendations
```

#### Pass 2: Security & Data Flow
**File:** `$REVIEW_DIR/pass-02-security.md`
**Focus:** Authentication, authorization, data validation, secrets management, injection risks, data flow paths.
**Prompt for agent:**
```
You are a security-focused architect reviewing a codebase.

First, read the context and previous review:
- $REVIEW_DIR/pass-00-context.md (system goals and privacy requirements)
- $REVIEW_DIR/pass-01-structure.md

Note the system's privacy principles (local-first, user control over data).

Then analyze:
1. Authentication and authorization patterns
2. Input validation and sanitization
3. Secrets management (API keys, credentials, tokens)
4. SQL injection, XSS, command injection risks
5. Data flow from external inputs to storage/output
6. Sensitive data handling and exposure risks

Focus on NEW findings not covered in Pass 1.

Write your findings to: $REVIEW_DIR/pass-02-security.md

Format:
# Architecture Review: Security & Data Flow
## Summary
## Previous Pass Reference
## Findings
### Strengths
### Concerns (with severity: Critical/High/Medium/Low)
### Recommendations
```

#### Pass 3: Scalability & Performance
**File:** `$REVIEW_DIR/pass-03-scalability.md`
**Focus:** Async patterns, database access patterns, caching strategy, bottlenecks, resource management.
**Prompt for agent:**
```
You are a performance-focused architect reviewing a codebase.

First, read previous reviews:
- $REVIEW_DIR/pass-01-structure.md
- $REVIEW_DIR/pass-02-security.md

Then analyze:
1. Async/await patterns and concurrency model
2. Database query patterns (N+1, missing indexes, connection pooling)
3. Caching strategy and cache invalidation
4. Memory management and potential leaks
5. I/O bottlenecks and blocking operations
6. Horizontal vs vertical scaling readiness

Focus on NEW findings not covered in previous passes.

Write your findings to: $REVIEW_DIR/pass-03-scalability.md

Format:
# Architecture Review: Scalability & Performance
## Summary
## Previous Pass Reference
## Findings
### Strengths
### Concerns (with impact assessment)
### Recommendations
```

#### Pass 4: Maintainability & Testing
**File:** `$REVIEW_DIR/pass-04-maintainability.md`
**Focus:** Test coverage, code complexity, documentation, error handling, debugging support.
**Prompt for agent:**
```
You are a maintainability-focused architect reviewing a codebase.

First, read previous reviews:
- $REVIEW_DIR/pass-01-structure.md
- $REVIEW_DIR/pass-02-security.md
- $REVIEW_DIR/pass-03-scalability.md

Then analyze:
1. Test coverage and test quality
2. Code complexity and cognitive load
3. Error handling patterns and consistency
4. Logging and observability
5. Documentation (code comments, READMEs, API docs)
6. Onboarding difficulty for new developers

Focus on NEW findings not covered in previous passes.

Write your findings to: $REVIEW_DIR/pass-04-maintainability.md

Format:
# Architecture Review: Maintainability & Testing
## Summary
## Previous Pass Reference
## Findings
### Strengths
### Concerns
### Recommendations
```

#### Pass 5: Documentation Audit
**File:** `$REVIEW_DIR/pass-05-docs-audit.md`
**Focus:** Verify docs/ and context/ folders match actual codebase state, create beads for corrections.
**Prompt for agent:**
```
You are a documentation auditor ensuring docs match reality.

First, read previous reviews:
- $REVIEW_DIR/pass-01-structure.md
- $REVIEW_DIR/pass-02-security.md
- $REVIEW_DIR/pass-03-scalability.md
- $REVIEW_DIR/pass-04-maintainability.md

Then audit documentation:

1. **Read all files in docs/ and context/ directories**
   - Use Glob to find all .md files in these folders
   - Read each document thoroughly

2. **Cross-reference against codebase**
   For each document, verify:
   - File paths mentioned still exist
   - Code examples match actual implementations
   - Architecture descriptions match current structure
   - API documentation matches actual endpoints/signatures
   - Configuration examples are still valid
   - Dependencies listed are still in use

3. **Identify discrepancies**
   For each issue found, note:
   - Document path
   - Section with issue
   - What it says vs what's true
   - Severity (outdated, misleading, broken)

4. **Create beads for corrections**
   For each discrepancy, create a bead:
   ```bash
   bd create --title "docs: Update [filename] - [brief issue]" --type task --priority 2 --labels "docs,arch-review"
   ```

   Add details to the bead description explaining what needs to change.

5. **Write audit report**
   Write findings to: $REVIEW_DIR/pass-05-docs-audit.md

Format:
# Architecture Review: Documentation Audit
## Summary
## Documents Reviewed
## Discrepancies Found
### [Document Path]
- **Issue:** description
- **Current:** what doc says
- **Actual:** what code shows
- **Bead:** pe-xxx
### ...
## Documents Verified Correct
## Beads Created
| Bead ID | Document | Issue |
|---------|----------|-------|
| pe-xxx  | path     | brief |
```

#### Pass 6: Meta-Review & Consolidation
**File:** `$REVIEW_DIR/pass-06-meta-review.md`
**Focus:** Quality-check passes 1-4, identify gaps, resolve contradictions, produce consolidated architecture changes list.
**Prompt for agent:**
```
You are a senior architect performing a meta-review of architecture analyses.

Read ALL technical reviews (passes 1-4):
- $REVIEW_DIR/pass-01-structure.md
- $REVIEW_DIR/pass-02-security.md
- $REVIEW_DIR/pass-03-scalability.md
- $REVIEW_DIR/pass-04-maintainability.md

Also read the docs audit for context:
- $REVIEW_DIR/pass-05-docs-audit.md

Your task is to QUALITY CHECK the previous analyses and CREATE A CONSOLIDATED VIEW.

## Part 1: Quality Check

Review each pass and ask:
1. **Completeness** - Did it cover its domain thoroughly? What was missed?
2. **Accuracy** - Are the findings correct? Any misunderstandings of the code?
3. **Contradictions** - Do any passes contradict each other? Resolve them.
4. **Blind spots** - What cross-cutting concerns fell between the cracks?
5. **Depth** - Were recommendations specific enough to be actionable?

If you find significant gaps, explore the codebase yourself to fill them.

## Part 2: Consolidated Architecture Changes

Create a SINGLE, DEFINITIVE list of all recommended architecture changes extracted from passes 1-4. For each change:
- Clear description of what to change
- Why (which pass(es) identified it, what problem it solves)
- Impact (what improves)
- Effort estimate (small/medium/large)
- Dependencies (what must happen first)

Deduplicate overlapping recommendations. Resolve any conflicts.

Write your meta-review to: $REVIEW_DIR/pass-06-meta-review.md

Format:
# Architecture Review: Meta-Review & Consolidation
## Quality Assessment
### Pass 1 (Structure) Review
- Coverage: [complete/partial/gaps]
- Gaps identified: ...
- Accuracy issues: ...
### Pass 2 (Security) Review
- Coverage: [complete/partial/gaps]
- Gaps identified: ...
- Accuracy issues: ...
### Pass 3 (Scalability) Review
- Coverage: [complete/partial/gaps]
- Gaps identified: ...
- Accuracy issues: ...
### Pass 4 (Maintainability) Review
- Coverage: [complete/partial/gaps]
- Gaps identified: ...
- Accuracy issues: ...
## Contradictions Resolved
## Blind Spots Filled
## Consolidated Architecture Changes
| # | Change | Source | Problem Solved | Impact | Effort | Dependencies |
|---|--------|--------|----------------|--------|--------|--------------|
| 1 | ...    | Pass X | ...            | ...    | S/M/L  | ...          |
## Change Details
### Change 1: [Title]
**Description:** ...
**Rationale:** ...
**Implementation notes:** ...
### Change 2: ...
```

#### Pass 7: Synthesis & Prioritization
**File:** `$REVIEW_DIR/pass-07-synthesis.md`
**Focus:** Final executive summary with prioritized action plan, drawing from the consolidated meta-review.
**Prompt for agent:**
```
You are a senior architect creating an executive summary and action plan.

Read the consolidated meta-review (PRIMARY INPUT):
- $REVIEW_DIR/pass-06-meta-review.md

Also reference:
- $REVIEW_DIR/pass-05-docs-audit.md (for documentation beads)

The meta-review has already quality-checked and consolidated findings from passes 1-4.
Your job is to prioritize and create an actionable plan.

Create a synthesis that:
1. Provides an executive summary of architecture health
2. Prioritizes the consolidated changes by impact and effort
3. Creates a phased action plan with clear sequencing
4. Identifies quick wins (high impact, low effort)
5. References documentation beads from Pass 5
6. Provides clear next steps

Write your synthesis to: $REVIEW_DIR/pass-07-synthesis.md

Format:
# Architecture Review: Synthesis & Action Plan
## Executive Summary
## Architecture Health Score
- Structure: [1-5] - brief note
- Security: [1-5] - brief note
- Scalability: [1-5] - brief note
- Maintainability: [1-5] - brief note
- Documentation: [1-5] - brief note
## Prioritized Changes
### Critical (do immediately)
### High Priority (do soon)
### Medium Priority (plan for)
### Low Priority (consider)
## Quick Wins
## Documentation Issues (beads created in Pass 5)
## Action Plan
### Phase 1: Foundation (weeks 1-2)
### Phase 2: Core Improvements (weeks 3-4)
### Phase 3: Polish (ongoing)
## Recommended Next Steps
1. ...
2. ...
3. ...
## Conclusion
```

### Execution

Run each pass sequentially using the Task tool with these parameters:
- `subagent_type`: "general-purpose"
- `model`: "opus"
- `prompt`: The agent prompt above with $REVIEW_DIR substituted

After all passes complete, output:
```
Architecture Review Complete
============================
Location: $REVIEW_DIR

Files:
  - pass-00-context.md         (System goals & review criteria)
  - pass-01-structure.md       (Architecture & patterns)
  - pass-02-security.md        (Security & data flow)
  - pass-03-scalability.md     (Performance & scaling)
  - pass-04-maintainability.md (Testing & maintenance)
  - pass-05-docs-audit.md      (Documentation audit + beads)
  - pass-06-meta-review.md     (Quality check + consolidated changes)
  - pass-07-synthesis.md       (Executive summary + action plan)

Beads created: [list bead IDs from Pass 5]

Start with: pass-07-synthesis.md for the executive summary
Consolidated changes: pass-06-meta-review.md
Review criteria: pass-00-context.md
Review beads: bd list --label arch-review
```

### User Focus Area

If the user provided $ARGUMENTS, emphasize that area across all passes. For example:
- "data pipeline" → Focus on data flow, ETL patterns, data validation
- "API layer" → Focus on REST/GraphQL design, versioning, error responses
