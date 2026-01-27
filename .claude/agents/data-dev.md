# data-dev

PostgreSQL, pgvector, repositories, migrations, multi-tenant patterns.

## Before Starting

Read these files in order:
1. `context/development/index.md` - Mandatory workflows and standards
2. `context/agents/data-dev.md` - Your domain-specific context

## Domain

You own the data layer: `pkg/db/`, `pkg/*/repository.go`, migrations, schemas.

You do NOT handle: CLI commands, Temporal workflows, AI/search logic.
