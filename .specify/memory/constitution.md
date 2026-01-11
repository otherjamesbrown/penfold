<!--
Sync Impact Report:
- Version change: 1.0.0 → 1.1.0 (added development principles)
- Modified principles:
  * I. Specification-First Development (unchanged)
  * II. CLI + Library Architecture (unchanged)
  * III. Test-First Development (NEW - added TDD requirements)
  * IV. Integration Testing (NEW - added contract testing)
  * V. Observability & Debugging (NEW - added structured logging)
  * VI. Versioning & Breaking Changes (NEW - added semver rules)
  * VII. Simplicity & YAGNI (NEW - added complexity management)
  * VIII. Temporal-First Data Organization (moved from III)
  * IX. Research-Driven Design (moved from IV)
  * X. Local-First AI with Privacy (moved from V)
- Added sections: Code Quality Standards (expanded)
- Templates requiring updates: ✅ All existing templates validated
- Follow-up TODOs: None
-->

# Penfold Constitution

## Core Principles

### I. Specification-First Development
All functionality begins with comprehensive specification before implementation. Specifications must be validated through research, user interviews, and technical feasibility analysis. No implementation work begins until specifications achieve "optimal shape" through product management review and challenge process.

**Rationale**: Penfold is a complex AI system requiring careful design. Early specification investment prevents costly architectural mistakes and ensures optimal technical decisions.

### II. CLI + Library Architecture
Every feature starts as a standalone library with clear interfaces. All libraries must expose functionality via CLI with text-in/text-out protocols. Libraries must be independently testable, documented, and serve a single clear purpose.

**Rationale**: Modular architecture ensures maintainability, testability, and enables incremental development of complex AI pipelines.

### III. Test-First Development (NON-NEGOTIABLE)
Test-Driven Development is mandatory: Tests written → User approved → Tests fail → Then implement. Red-Green-Refactor cycle strictly enforced. No code commits without corresponding tests.

**Rationale**: AI systems are complex and error-prone. TDD ensures correctness, prevents regressions, and serves as living documentation of expected behavior.

### IV. Integration Testing
Focus areas requiring integration tests: New library contract tests, Contract changes, Inter-service communication, Shared schemas. All external dependencies must have contract tests.

**Rationale**: Penfold's modular architecture creates many integration points. Contract testing prevents interface breakage and enables confident refactoring.

### V. Observability & Debugging
Text I/O ensures debuggability. Structured logging required for all operations. Error messages must be actionable with clear context. All operations must be traceable through logs.

**Rationale**: AI pipelines are complex black boxes. Comprehensive logging enables debugging production issues and understanding system behavior.

### VI. Versioning & Breaking Changes
Semantic versioning (MAJOR.MINOR.PATCH) strictly enforced. Breaking changes require MAJOR version bump, deprecation warnings, and migration guides. Libraries must maintain backward compatibility within MAJOR versions.

**Rationale**: Modular architecture requires reliable versioning. Clear versioning prevents dependency hell and enables confident upgrades.

### VII. Simplicity & YAGNI
Start simple, avoid premature optimization. Implement only required features - no speculative functionality. Complexity must be justified with clear business value. Prefer composition over inheritance.

**Rationale**: AI systems are inherently complex. Managing essential complexity requires eliminating accidental complexity through simple, focused design.

### VIII. Temporal-First Data Organization
Time is the primary organizing axis - all data must include timestamps and support temporal queries. Emergent structure is preferred over rigid schemas. AI discovers relationships rather than enforcing predefined taxonomies.

**Rationale**: Core to Penfold's "contextual time machine" vision. Temporal organization matches human information retrieval patterns and supports retroactive context reconstruction.

### IX. Research-Driven Design
Technical decisions must be informed by current state-of-the-art research in knowledge management, information extraction, entity resolution, and semantic search. All architectural choices require evidence-based justification with consideration of alternatives.

**Rationale**: Penfold operates in a rapidly evolving AI landscape. Research-driven approach maximizes success probability and prevents obsolete technical choices.

### X. Local-First AI with Privacy
Local LLM processing for classification and sensitive operations, cloud APIs only for non-sensitive extraction tasks. User data remains on local infrastructure with explicit consent for any cloud processing. Source truth always maintained with links to original documents.

**Rationale**: Personal information system requires privacy guarantees. Local-first approach provides performance, cost control, and data sovereignty while enabling advanced AI capabilities.

## Code Quality Standards

**Testing Requirements**:
- Unit tests for all functions and classes
- Integration tests for all inter-service communication
- Contract tests for external APIs
- Performance tests for critical paths
- Test coverage minimum 80% for core libraries

**Code Standards**:
- Type hints required for all function signatures
- Docstrings required for all public functions and classes
- Linting with `ruff` and `mypy` - zero warnings allowed
- Code formatting with `black` - strictly enforced
- Import organization with `isort`

**Documentation Standards**:
- README.md for each library with usage examples
- API documentation for all CLI commands
- Architecture Decision Records (ADRs) for significant choices
- Inline comments only for complex business logic

**Performance Standards**:
- Sub-second response for temporal queries
- Real-time ingestion for email/meetings (< 5 second delay)
- Weekly batch processing for complex analytics
- Memory usage monitoring and leak detection

## Development Workflow

Product management role: Challenge assumptions, research best practices, drive optimal design, validate requirements. All specifications require approval before implementation. Test-Driven Development mandatory for core libraries.

Git workflow: Feature branches, comprehensive PR reviews, automated testing gates. No direct commits to main branch.

## Governance

Constitution supersedes all other development practices. Amendments require documentation of rationale, approval process, and migration plan for existing work. All PRs must verify constitutional compliance.

Version control follows semantic versioning: MAJOR for backward-incompatible governance changes, MINOR for new principles or expanded guidance, PATCH for clarifications and refinements.

**Version**: 1.1.0 | **Ratified**: 2026-01-11 | **Last Amended**: 2026-01-11