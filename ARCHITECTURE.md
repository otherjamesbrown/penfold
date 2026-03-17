# Penfold Architecture

This file is intentionally thin.

The detailed architecture map for Penfold lives in Context Palace knowledge shards, which are easier for agents to load selectively and keep current than a large flat document.

## Source-of-Truth Order

When architecture knowledge and implementation disagree, use this precedence:

1. `AGENTS.md` and other repo law files
2. Context Palace playbook and KB shards
3. Code, tests, migrations, and runtime evidence

If this file ever drifts from the implementation or the KB, treat this file as the weaker source.

## System Shape

This repository contains the backend side of Penfold, including:

- request-time API and orchestration services
- workflow execution and background processing
- ingestion connectors
- AI coordination
- MCP-facing tool exposure
- shared domain packages
- database migrations
- automated tests

## Main Runtime Services

- `services/gateway`
  Main API surface and orchestration layer
- `services/worker`
  Temporal workflows and activities
- `services/gmail`
  Gmail auth, sync, and push ingestion
- `services/ai`
  AI coordinator and model/provider routing
- `services/mcp`
  MCP server exposing Penfold capabilities to agent clients

## Where to Look Next

For subsystem detail, load the relevant KB branch:

- ingest and processing questions
  load the Ingest Pipeline branch
- entities, glossary, or mention questions
  load the Knowledge Graph branch
- search, digests, or retrieval questions
  load the Search & Retrieval branch
- model routing or provider behavior
  load the AI & Models branch
- deployment, scheduling, observability, or runtime boundaries
  load the Infrastructure branch
- workflow, review, or process questions
  load the How We Work branch

## What This File Should Not Do

This file should not try to be:

- a full subsystem encyclopedia
- a live inventory of ports, models, schedules, or config values
- a replacement for the KB tree

Those details drift too quickly and are better handled in KB shards or verified directly in code and runtime state.
