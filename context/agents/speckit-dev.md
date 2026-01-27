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
| Bead generation | `/speckit.beads` | Beads in `bd` |
| Implementation | `/speckit.implement-beads` | Working code |
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

# 4. Generate beads
/speckit.beads

# 5. STOP - Get user approval

# 6. Implement (after approval)
/speckit.implement-beads

# 7. Validate consistency
/speckit.analyze
```

## MANDATORY: Implementation Gate

**NEVER start `/speckit.implement-beads` without explicit user approval.**

After completing planning and bead generation:

1. Present summary of what will be implemented
2. List all beads with descriptions
3. Ask: "Ready to begin implementation? (yes/no)"
4. **WAIT for user confirmation**

This is non-negotiable.

## Bead Commands

```bash
bd ready                              # Find available work
bd show <id>                          # View bead details
bd update <id> --status=in_progress   # Claim work
bd close <id> --reason="..."          # Complete work
bd dep tree <epic-id>                 # View epic structure
bd sync                               # Sync with git
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

# After bead generation
bd list | grep <feature>              # Verify beads created

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

4. Close epic bead

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
## Cross-Spec Bead Dependencies

| This Phase | Depends On | Reason |
|------------|------------|--------|
| US3 | 007-search/US1 | Needs search API |
```

## Feature Completion Checklist

Before closing a feature:

- [ ] All beads closed
- [ ] Tests pass
- [ ] Documentation updated
- [ ] ARCHIVE.md created (in `.specify/features/<name>/`)
- [ ] Reusable patterns extracted to `context/architecture/`
- [ ] Epic bead closed
