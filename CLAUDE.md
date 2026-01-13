# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 🚨 AUTONOMOUS DEVELOPMENT - READ FIRST

**This is 100% AI-coding assistant driven development.**

### Before Starting Any New Specification or Major Work

**ALWAYS ASK:** "Do you want me to start on [specification/feature] and continue until complete (or questions arise)?"

**Wait for user confirmation before beginning new major work streams.**

### Autonomous Development Rules

**CONTINUE AUTONOMOUSLY for:**
- Writing code and tests within an approved specification
- Making technical implementation decisions
- Running commands and tests
- Committing and pushing changes
- Following established patterns and specifications
- Bug fixes and improvements within current work

**ONLY ASK USER when:**
- Starting a new specification or major feature
- Business requirements are ambiguous
- Multiple valid approaches exist and user preference is needed
- You need external credentials or resources
- Technical blockers require user intervention
- **Adding new architectural components** (observability, monitoring, logging, etc.)
- **Modifying system architecture or infrastructure patterns**

### Architecture Coordination - Critical

**STOP and CHECK before adding:**
- Observability/monitoring systems
- Logging infrastructure
- Message queues or event systems
- Authentication/authorization
- Configuration management
- Caching layers
- Backup/recovery systems
- CI/CD pipelines

**Search codebase first** - if similar infrastructure exists, use it or ask how to integrate.

## Beads Workflow - Essential for All Sessions

**NEVER start work without a bead.**

### Core Commands
```bash
bd ready                    # Find available work
bd create --title="..." --type=task --priority=2
bd update <id> --status=in_progress  # Claim work
bd close <id> --reason="commit <hash>: <summary>"
bd sync                     # Sync with git (run at session end)
```

### Critical Rules
- Create or find bead BEFORE writing code
- Update bead status when starting: `bd update <id> --status=in_progress`
- Reference bead in commits: `feat(component): description [pe-xxx]`
- Close bead with commit hash and summary
- Run `bd sync` before ending session

## Session Close Protocol - MANDATORY

**Work is NOT complete until pushed to remote.**

```bash
git status              # Check what changed
git add <files>         # Stage changes
bd sync                 # Sync beads
git commit -m "..."     # Commit code
git push                # MUST PUSH TO REMOTE
```

**NEVER:**
- Stop before pushing to remote
- Say "ready to push when you are" - YOU must push
- Leave work stranded locally

## Project Overview

**Penfold** is a personal AI-powered information system that aggregates, correlates, and surfaces contextual information from disparate communication channels (email, Slack, documents, meetings). The system transforms fragmented organizational knowledge into a navigable, queryable institutional memory.

**Current Status**: Active development - implementing foundational database layer with event-driven processing framework.

## Architecture

### Core Components
- **CLI Tool (`penf`)**: Main interface for ingestion, review, queries, and management
- **Core Library (`penf_lib`)**: Contains connectors, extraction pipeline, entity resolution, project management, search/retrieval, and storage layers
- **Data Layer**: PostgreSQL with pgvector extension for semantic search
- **LLM Pipeline**: Tiered approach using local models (Ollama) for classification and cloud APIs (Gemini) for extraction

### Data Model
Key entities include:
- **Source**: Raw content from external systems (email, Slack, documents)
- **Assertion**: Extracted meaningful information (decisions, risks, commitments, milestones, etc.)
- **Person**: Entity resolution for individuals with canonical names and aliases
- **Project**: Hierarchical structure for organizing initiatives with artifacts and timeline
- **Team**: Organizational structure

## Development Environment

### Target Stack
- **Language**: Python 3.12
- **Database**: PostgreSQL 16 + pgvector
- **Local LLM**: Ollama + Llama 3.1 8B
- **Cloud LLM**: Gemini API
- **Embeddings**: nomic-embed-text (local via Ollama)
- **Platform**: Mac Mini M4 with 32GB RAM

### CLI Commands (In Development)
```bash
penf ingest                          # Run ingestion pipeline
penf review --daily                  # Morning review workflow
penf review --weekly                 # Weekly project health review
penf ask "<query>"                   # Natural language queries
penf project <name>                  # Project operations
penf person <name>                   # Person lookups
penf timeline <query>                # Timeline visualizations
penf config                          # Source configuration
```

## Current Implementation Status

**Current Implementation**: Database Schema and Storage Layer (001)
- PostgreSQL 16+ with pgvector extension for hybrid storage
- SQLAlchemy 2.0 async models for core entities (Source, Assertion, Person, Project, Team)
- Vector storage with HNSW indexing for 768-dimensional embeddings
- Event-driven processing framework with Redis pub-sub
- Migration system with Alembic for schema versioning
- Comprehensive test suite with async fixtures and performance benchmarks

**Remaining Features**: 8 additional features with complete SpecKit specifications ready for implementation

### **Key Specifications**
- **Main Spec**: `specs/revised/penfold-spec-v3.md` - Complete system architecture
- **Database Design**: `specs/001-database-schema/` - Storage layer implementation guide
- **AI Architecture**: `specs/revised/ai-architecture.md` - Multi-model processing design
- **Event Processing**: `specs/revised/ingestion-and-categorization.md` - Event-driven coordination

### Key Changes in v2.0:
- **Mental Model**: From knowledge management to "contextual time machine"
- **Core Focus**: Temporal spine + project contexts, emergent relationships
- **Use Cases**: "Rewind time" queries vs formal review workflows
- **MVP Scope**: Much simpler - email + meetings with basic entity discovery

## Current Focus

**Priority: Implementing foundational database layer** (specs/001-database-schema)

### Development Standards
- **Test-Driven Development**: Write tests first, ensure they fail, then implement
- **Constitution Compliance**: Follow all principles in `project-constitution.md`
- **Quality Standards**: Type hints, docstrings, linting with ruff/mypy, black formatting
- **Async Architecture**: SQLAlchemy 2.0 with asyncpg driver for high-performance operations
- **Complete Workflows**: Take features from specification to working, tested, committed code

### Current Implementation Focus
- **Database Models**: Async SQLAlchemy entities with proper relationships and constraints
- **Migration System**: Alembic integration with CLI commands
- **Vector Operations**: pgvector with HNSW indexing for semantic search
- **Event Framework**: Redis pub-sub with PostgreSQL fallback
- **CLI Interface**: Database management, migration, and monitoring commands
- **Test Infrastructure**: Async fixtures, unit tests, integration tests, performance benchmarks

### Success Criteria
Each implementation must meet measurable performance targets:
- CRUD operations: <100ms for datasets up to 10K records
- Vector similarity search: <500ms for 100K vectors
- Event pub/sub operations: <50ms for real-time processing
- Concurrent operations: Support 50+ simultaneous connections
- Test coverage: Minimum 80% for core libraries

## Quick Reference

### Key Files
- **Current Work**: `specs/001-database-schema/` (database implementation)
- **System Architecture**: `specs/revised/penfold-spec-v3.md`
- **Development Standards**: `project-constitution.md`
- **Available Tasks**: Run `bd ready` to see ready work

### Development Workflow
1. Find/create bead: `bd ready` or `bd create --title="..." --type=task`
2. Claim work: `bd update <id> --status=in_progress`
3. Write failing tests → Implement → Test
4. Commit with bead reference: `feat(component): description [pe-xxx]`
5. Close bead: `bd close <id> --reason="commit <hash>: summary"`
6. Push to remote: `git push`

## Key Design Principles

1. **Temporal First**: Time is the primary organizing axis - everything happens at [timestamp]
2. **Event-Driven Processing**: Pub-sub framework enables flexible multi-model AI coordination
3. **Emergent Structure**: Let AI discover relationships, don't force rigid schemas
4. **Source Truth**: Always maintain links back to original documents/meetings/emails
5. **Context Containers**: Project contexts (Atlas, People Management, General) provide thematic grouping
6. **Human-in-Loop**: AI suggests, human confirms critical entity resolutions
7. **Local-First with Cloud Quality Gates**: Process locally, validate with cloud models selectively
8. **ADHD-Friendly**: Structured temporal browsing for when focus shifts from high-level to detail

## Technology Stack (Implemented)

### Core Infrastructure
- **Language**: Python 3.12 with type hints and async/await
- **Database**: PostgreSQL 16+ with pgvector extension
- **ORM**: SQLAlchemy 2.0 with asyncpg driver for async operations
- **Migrations**: Alembic for schema versioning
- **Message Queue**: Redis for pub-sub event processing
- **Testing**: pytest with asyncio support, async fixtures

### Development Tools
- **Linting**: ruff (replaces flake8, isort, pyupgrade)
- **Type Checking**: mypy with strict settings
- **Formatting**: black for consistent code style
- **Task Tracking**: Beads for issue/feature management
- **CLI Framework**: Click for command-line interface

### Vector Operations
- **Embeddings**: 768-dimensional vectors (nomic-embed-text compatible)
- **Indexing**: HNSW algorithm with M=16, ef_construction=200
- **Search**: L2 distance for similarity queries

## Development Standards

### Code Quality
- All functions require type hints and docstrings
- Minimum 80% test coverage for core libraries
- Zero warnings from ruff and mypy
- TDD workflow: tests first, then implementation

### Performance Targets
- CRUD operations: <100ms (10K records)
- Vector search: <500ms (100K vectors)
- Event processing: <50ms (pub/sub)
- Concurrent connections: 50+ simultaneous

## Recent Implementation Progress
- ✅ Database schema specification completed with SpecKit workflow
- ✅ All analysis issues resolved (HNSW parameters, backup tasks, async strategy)
- 🚧 Ready for `/speckit.implement` to begin actual code development
- 📋 87 implementation tasks organized by user story priority
