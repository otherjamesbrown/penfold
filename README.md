# Penfold

Penfold is James's institutional-memory system. This repository contains the backend services, workflow execution, shared packages, tests, and migrations that power ingestion, enrichment, retrieval, and agent-facing operations.

## Start Here

For AI-agent onboarding, the preferred retrieval order is:

1. `AGENTS.md`
2. Context Palace playbook and KB shards
3. Code and tests

The KB is the primary architecture map. Repo-local docs should stay thin and should not compete with the KB for subsystem truth.

## Runtime Surface

The main runtime services in this repo are:

- `services/gateway`
  Primary API and orchestration surface
- `services/worker`
  Temporal workflows and activities
- `services/gmail`
  Gmail auth, sync, and push-driven ingestion
- `services/ai`
  AI coordinator and provider routing
- `services/mcp`
  MCP server exposing Penfold capabilities as toolsets

Shared code lives under `pkg/`.
Protocol definitions live under `api/proto/`.
Database migrations live under `migrations/`.
Integration and behavior tests live under `tests/`.

## Repo Structure

```text
api/proto/      Protocol and service contracts
migrations/     Database schema and config evolution
pkg/            Shared libraries, repositories, and domain packages
services/       Runtime services
tests/          Unit, integration, e2e, and quality tests
docs/           Local supporting docs only
specs/          Design and historical implementation specs
```

## Development

This project uses `bd` for issue tracking.

Useful commands:

```bash
bd ready
bd show <id>
bd update <id> --status in_progress
bd close <id>
bd sync
```

## KB and Architecture

The Context Palace KB is the preferred architecture surface for navigational knowledge.

Key KB areas include:

- playbook / system map
- architectural principles
- ingest pipeline
- knowledge graph
- search and retrieval
- AI and models
- infrastructure
- workflows and ways of working
- testing architecture

This repo's local architecture documents should remain lightweight and point into the KB rather than duplicating volatile subsystem detail.
