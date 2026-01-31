---
name: speckit-reference
description: Reference documentation for speckit skills (not an agent)
---

# Speckit Reference

> **This is reference documentation for the `/speckit.*` skills, not a sub-agent.**
> The root agent (orchestrator) uses these skills directly for feature planning.

Speckit manages feature lifecycle: from specification through implementation to documentation archival.

## Scope

### Handles

| Phase | Command | Output |
|-------|---------|--------|
| Specification | `/speckit.specify` | `spec.md` |
| Clarification | `/speckit.clarify` | Updated `spec.md` |
| Planning | `/speckit.plan` | `plan.md` |
| Shard generation | `/speckit.shards` | Shards in `bd` |
| Implementation | `/speckit.implement-shards` | Working code |
| Analysis | `/speckit.analyze` | Consistency report |
| Archival | Manual | `ARCHIVE.md`, patterns |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| AI/search implementation | ai-dev |
| CLI commands | cli-dev |
| Workflows | worker-dev |
| Database schema | data-dev |
| Test framework | testing-dev |
| Gmail integration | gmail-dev |

## Workflow

```bash
# 1. Create specification
/speckit.specify "Feature description here"

# 2. Clarify ambiguities (interactive)
/speckit.clarify

# 3. Create technical plan
/speckit.plan

# 4. Generate shards
/speckit.shards

# 5. STOP - Get user approval

# 6. Implement (after approval)
/speckit.implement-shards

# 7. Validate consistency
/speckit.analyze
```

## MANDATORY: Implementation Gate

**NEVER start `/speckit.implement-shards` without explicit user approval.**

After completing planning and shard generation:

1. Present summary of what will be implemented
2. List all shards with descriptions
3. Ask: "Ready to begin implementation? (yes/no)"
4. **WAIT for user confirmation**

This is non-negotiable.

## Shard Commands

```bash
# View shard details
palace task get pf-xxx

# Claim work
palace task claim pf-xxx

# Log progress
palace task progress pf-xxx "Completed spec.md, starting plan.md"

# Complete work
palace task close pf-xxx "Completed: summary"
```

## Feature Directory Structure

```
.specify/features/<feature-name>/
├── spec.md           # User stories, requirements
├── plan.md           # Technical implementation plan
├── data-model.md     # Entity definitions (optional)
├── research.md       # Technical decisions (optional)
├── quickstart.md     # Test scenarios (optional)
└── contracts/        # API definitions (optional)
    └── api.yaml
```

## Specification Quality

A good `spec.md` includes:

- [ ] Clear user stories with priorities (P1, P2, P3)
- [ ] Acceptance criteria for each story
- [ ] Dependencies on other features
- [ ] Out of scope items explicitly listed
- [ ] Success metrics defined

## Plan Quality

A good `plan.md` includes:

- [ ] Tech stack and libraries
- [ ] Project structure (new files, modified files)
- [ ] Implementation phases
- [ ] Testing strategy
- [ ] Risks and mitigations

## Escalation Points

Stop and ask the user when:

- **Before starting implementation** (ALWAYS)
- Business requirements are ambiguous
- Multiple valid technical approaches exist
- Adding new architectural components
- Dependencies on unfinished features
- Scope creep detected

## Quality Gates

Before completing each phase:

```bash
# After specification
cat .specify/features/<name>/spec.md  # Verify completeness

# After planning
cat .specify/features/<name>/plan.md  # Verify technical plan

# After shard generation - verify shards created
psql ... -c "SELECT id, title FROM shards WHERE project = 'penfold' AND title LIKE '%<feature>%';"

# After implementation
go build ./...                        # Builds
go test ./... -race                   # Tests pass
```

## Archival Process

After feature completion:

1. Create `ARCHIVE.md` with:
   - Summary of what was built
   - Key decisions and rationale
   - Patterns extracted for reuse
   - Lessons learned

2. Extract reusable patterns to `context/architecture/`

3. Update relevant documentation

4. Close group shard

## Common Patterns

### Multi-Phase Implementation

```markdown
## Phase 1: Foundation
- Database schema
- Basic repository
- Unit tests

## Phase 2: Core Logic
- Service layer
- Business rules
- Integration tests

## Phase 3: Integration
- CLI commands
- Workflow integration
- E2E tests
```

### Cross-Feature Dependencies

```markdown
## Cross-Spec Shard Dependencies

| This Phase | Depends On | Reason |
|------------|------------|--------|
| US3 | 007-search/US1 | Needs search API |
```

## Feature Completion Checklist

Before closing a feature:

- [ ] All shards closed
- [ ] Tests pass
- [ ] Documentation updated
- [ ] ARCHIVE.md created (in `.specify/features/<name>/`)
- [ ] Reusable patterns extracted to `context/architecture/`
- [ ] Group shard closed
