# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Penfold** is an early-stage repository for a personal AI-powered information system that aggregates, correlates, and surfaces contextual information from disparate communication channels (email, Slack, documents, meetings). The system transforms fragmented organizational knowledge into a navigable, queryable institutional memory.

This is currently a specification-only project with no implementation yet.

## Architecture (Planned)

### Core Components
- **CLI Tool (`cxp`)**: Main interface for ingestion, review, queries, and management
- **Core Library (`cxp_lib`)**: Contains connectors, extraction pipeline, entity resolution, project management, search/retrieval, and storage layers
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

### Planned CLI Commands
```bash
cxp ingest                           # Run ingestion pipeline
cxp review --daily                   # Morning review workflow
cxp review --weekly                  # Weekly project health review
cxp ask "<query>"                    # Natural language queries
cxp project <name>                   # Project operations
cxp person <name>                    # Person lookups
cxp timeline <query>                 # Timeline visualizations
cxp config                           # Source configuration
```

## Current State

This repository contains specifications and no implementation.

**Current Spec**: `specs/revised/penfold-spec-v2.md` - Revised specification based on user research
**Original Spec**: `specs/initial/penfold-spec.md` - Original complex knowledge management approach
**Architecture Comparison**: `specs/revised/architecture-comparison.md` - Analysis of changes from v1 to v2
**AI Architecture**: `specs/revised/ai-architecture.md` - Multi-model local-first AI design
**Ingestion System**: `specs/revised/ingestion-and-categorization.md` - Three-channel ingestion with learning

### Key Changes in v2.0:
- **Mental Model**: From knowledge management to "contextual time machine"
- **Core Focus**: Temporal spine + project contexts, emergent relationships
- **Use Cases**: "Rewind time" queries vs formal review workflows
- **MVP Scope**: Much simpler - email + meetings with basic entity discovery

## Current Phase: Specification Refinement

**Priority: Refine specification before implementation begins**

The current focus is on getting the specification into optimal shape through product management expertise and research. This involves:

### Product Management Role
Claude should act as an experienced product manager specializing in knowledge management AI systems with responsibilities to:

1. **Challenge Assumptions**: Question design decisions, technical choices, and architectural patterns
2. **Research Best Practices**: Investigate current state-of-the-art approaches for:
   - Knowledge graph construction and maintenance
   - Information extraction from unstructured sources
   - Entity resolution in enterprise contexts
   - Semantic search and retrieval systems
   - Human-in-the-loop ML workflows
3. **Drive Optimal Design**: Push toward technical decisions that maximize success probability
4. **Validate Requirements**: Ensure user stories and acceptance criteria are realistic and measurable

### Key Areas for Challenge and Research
- **Entity Resolution**: Are the proposed person/team models sufficient? What about organizational changes over time?
- **Assertion Types**: Is the 8-type taxonomy complete and non-overlapping? Should it be extensible?
- **LLM Architecture**: Is the tiered approach cost-effective? Are there better alternatives?
- **Data Model**: Will the schema scale? Are there missing relationships or entities?
- **Search Strategy**: Is pgvector + embeddings the optimal approach for this use case?
- **MVP Scope**: Is the planned MVP truly minimal while proving value?

### Outcome Goal
Achieve a specification that represents best-in-class technical design informed by current research and proven patterns, ready for confident implementation.

## Getting Started

Since this is a specification-only project:

1. **Read `specs/revised/penfold-spec-v2.md`** - The current specification based on user research
2. **Review `specs/revised/architecture-comparison.md`** - Understand the evolution from v1 to v2
3. **Start with MVP Phase 1A**: Email + meetings with temporal search and basic entity discovery
4. **Focus on emergent relationships**: AI discovers connections, no rigid schemas
5. **Time + Projects as organizing principles**: Temporal spine with project context containers

## Key Design Principles

1. **Temporal First**: Time is the primary organizing axis - everything happens at [timestamp]
2. **Emergent Structure**: Let AI discover relationships, don't force rigid schemas
3. **Source Truth**: Always maintain links back to original documents/meetings/emails
4. **Context Containers**: Project contexts (Atlas, People Management, General) provide thematic grouping
5. **Human-in-Loop**: AI suggests, human confirms critical entity resolutions
6. **ADHD-Friendly**: Structured temporal browsing for when focus shifts from high-level to detail