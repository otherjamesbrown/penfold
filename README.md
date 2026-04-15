# Penfold

Penfold is James's institutional-memory system. This repository contains the backend services, workflow execution, shared packages, tests, and migrations that power ingestion, enrichment, retrieval, and agent-facing operations.

## Context

Penfold is one of four **operator instruments** James is building to support a senior engineering role at a PE-backed company. See `~/SYSTEMS.md` for cross-project data flows and ownership boundaries.

**Operator instruments:**
- **Penfold** (this repo) -- email, Teams, meeting transcripts → unified institutional memory
- **[Mycroft](https://github.com/otherjamesbrown/mycroft)** -- GitLab/GitHub metrics + code scanning for velocity, quality, secure coding
- **Moneypenny** -- consolidates disparate facts into a single queryable source
- **M-Intel** -- topic research and hypothesis generation with evidence grading

**Infrastructure:**
- **[Context Palace](https://github.com/otherjamesbrown/context-palace)** -- work tracking + KB for AI agents (`cxp` CLI)
- **[CoBuild](https://github.com/otherjamesbrown/cobuild)** -- design → decompose → implement → review → deploy automation

**Client:**
- **[penf-cli](https://github.com/otherjamesbrown/penf-cli)** -- command-line interface for this repo's gRPC services. Shares the `pf-` Context Palace namespace.

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
