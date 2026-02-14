# SLM/LLM Architecture — Context for AI Advisors

> **Status:** PARTIALLY IMPLEMENTED
> **Current state:** See Context Palace `penfold-arch-pipeline`
> **This spec covers:** All 14 work packages (WP1-WP14) completed. Remaining: Stage 2/3/4 refinement, advanced prompt engineering, full E2E validation at scale

## What This Is

You have been given this folder so you can advise on the design of **Penfold**, a knowledge management system that processes emails, meeting transcripts, and documents to build institutional memory for knowledge workers.

Specifically, we are designing **how to split AI workloads between local Small Language Models (7B, running on Apple Silicon) and remote Large Language Models (Gemini, OpenAI)** — and how to build a human-AI collaboration system around them.

These documents are self-contained. You do not need access to the codebase. Everything you need to understand the project, its architecture, its data model, and its constraints is here.

## What We Want From You

We want honest, critical feedback on the design. In particular:

- **Is the SLM/LLM split sound?** Are we routing the right tasks to the right models? Are there tasks we're assigning to the 7B SLM that it will struggle with? Are there tasks going to the remote LLM that the SLM could handle?
- **Is the human-AI collaboration model practical?** The system is designed around a "radar model" where the AI tracks everything and the human focuses a spotlight. Does this hold up? Are there gaps?
- **Is the session bootstrap design viable?** Claude (the AI assistant) has no memory between sessions. We solve this with a `penf context morning` command that loads a structured briefing. Is there a better approach?
- **Does the data model support the design?** We're using PostgreSQL with pgvector, not a graph database. The assertion versioning chain, trust/seniority weighting, and watch list concepts — do they work with relational data?
- **What are we missing?** Blind spots, failure modes, scalability concerns, alternative approaches we haven't considered.

Be direct. If something is over-engineered, say so. If something is under-designed, say so. We would rather hear hard truths now than discover them during implementation.

## How to Read These Documents

Read them in order. Each builds on the previous.

### Start here:

| # | File | What It Covers |
|---|------|---------------|
| 0 | `00-overview.md` | **Read this first.** Project vision, the Human+AI collaboration philosophy (this is the foundation, not an appendix), how users interact, core principles. |
| 1 | `01-architecture.md` | Service topology (4 Go services), gRPC APIs, deployment across 2 machines, technology stack. |
| 2 | `02-data-model.md` | PostgreSQL schema — 60 tables, key relationships, enums. Proposed seniority/trust extensions. |
| 3 | `03-entities.md` | Entity types (people, products, projects, teams, glossary), mention resolution pipeline, trust and seniority weighting. |
| 4 | `04-ai-services.md` | AI backends (MLX local, Gemini remote, OpenAI remote), model router, registry, audit trail, current limitations. |
| 5 | `05-content-pipeline.md` | 8-stage Temporal workflow, content types, search, thread reconstruction, proposed improvements. |
| 6 | `06-constraints.md` | Hardware (Mac Mini Apple Silicon + Linux server), cost model, volume estimates, current limitations. |

### Then the main design:

| File | What It Covers |
|------|---------------|
| `design.md` | **The core design narrative.** ~1,900 lines. Covers the collaboration philosophy, SLM vs LLM capabilities, the 6-stage pipeline (stages 0-5 in detail), content-type handling (emails, transcripts, Slack), knowledge feedback loops, assertion lifecycle and golden thread tracking, session bootstrap, progressive availability, and a worked end-to-end example. |

### Reference documents (supporting detail):

| File | What It Covers |
|------|---------------|
| `model-selection.md` | 7B vs 14B vs 32B tradeoffs on the M4 Mac Mini, hardware reality, task-based selection strategies, benchmark guidance |
| `prompt-engineering.md` | SLM vs LLM prompt rules, example prompts, output validation, quality tracking |
| `test-data-validation.md` | Analysis of 267 real emails and 18 transcripts — file size vs text size, does the design hold up |
| `cost-model.md` | Per-email cost breakdowns, batch processing economics, SLM throughput estimates |
| `07-session-bootstrap.md` | Session bootstrap specification — `penf context morning` response format, failure modes, staleness, size constraints, drill-down commands |
| `implementation.md` | What exists in the codebase, what needs building, what gets modified, design principles, FAQ |

## Key Design Concepts

These are the ideas that matter most. If you only have time to understand a few things:

1. **Human + AI Collaboration (Radar Model)**: The AI tracks everything (completeness). The human focuses the spotlight (judgment). Neither is sufficient alone. The human adds context the AI can't see: trust signals, offline conversations, gut feel. Claude proactively prompts the human, not just the reverse.

2. **SLM/LLM Task Split**: Classification, extraction, and embedding run on a local 7B model (free, fast, private). Deep analysis, synthesis, and reasoning go to a remote LLM (paid, slower, more capable). The split is based on whether the task requires reasoning or just pattern matching.

3. **Trust and Seniority**: Two different axes for weighting assertions. Seniority is organizational fact (VP > IC). Trust is personal and subjective ("I believe Sarah when she says it's a problem"). Both affect how information surfaces.

4. **Session Bootstrap**: Claude has no memory between sessions. `penf context morning` loads a structured briefing from the database: watch list, recent changes, active projects, trusted people, last session summary. The database is long-term memory; Claude's context window is a scratchpad.

5. **Assertion Lifecycle**: Risks, decisions, and actions are tracked across content items with versioning chains. The "golden thread" lets you trace a risk from when it was first raised through every escalation, decision, and resolution — without wading through every passing mention.

## Important Context

- **This is a real project**, not a thought experiment. There is working code, a deployed system, and real test data (267 emails, 18 meeting transcripts). The design needs to be practical.
- **Hardware is fixed**: One Mac Mini M4 (32GB unified memory, shared between OS and GPU) and one Linux server (no GPU). We can't add machines.
- **The user interacts through Claude Code** (an AI coding assistant), not through a GUI or CLI directly. Claude translates natural language into `penf` CLI commands. The CLI is designed for AI consumption, not human ergonomics.
- **PostgreSQL is the database**. We are not adopting a graph database. If you think we should, make the case, but understand we have 27 migrations and a working system on PostgreSQL + pgvector already.
- **Privacy matters**: Local models are preferred for content processing. Remote LLMs are used selectively, not as the default path.
