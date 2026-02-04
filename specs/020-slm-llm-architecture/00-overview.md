# Penfold: Project Overview

> This document provides external AI engines with enough context to understand and advise on Penfold's architecture without access to the full codebase.

## What Penfold Is

**Penfold is institutional memory for knowledge workers.**

Critical information is scattered across emails, meeting transcripts, documents, and chat messages. Decisions get forgotten. Expertise is invisible. Context switching costs hours of archaeology. Penfold solves this by:

1. **Aggregating** content from all communication channels into one searchable system
2. **Understanding** the people, products, and concepts mentioned in content
3. **Connecting** related information through entity resolution and relationship discovery
4. **Surfacing** relevant knowledge through intelligent search and AI queries

The goal: **Never lose context. Always know who knows what.**

---

## Foundation: Human + AI Collaboration

This is not optional context. This is the design philosophy that governs every architectural decision in Penfold.

### The System Is Neither Fully Automated Nor Manual

Penfold is a collaboration between a human and an AI assistant. The human brings judgment, context, relationships, gut feel, and domain expertise. The AI brings tireless observation, perfect recall, pattern detection across thousands of documents, and the ability to assemble context on demand. Neither is sufficient alone.

The human is not a passive consumer of AI outputs. They are an active participant who adds value: marking what matters, providing context the AI can't see (offline conversations, political dynamics, personal trust), and making judgment calls about priority and significance.

The AI is not a passive tool waiting for questions. It proactively surfaces things that need human attention: new risks, changes in patterns, items that have gone quiet, deadlines approaching without resolution.

### The Radar Model

Think of a radar screen:

- **The AI paints the full picture** — every risk, every mention, every escalation, tracked consistently. The AI's job is completeness, not curation. It tracks the "whole."
- **The human points the spotlight** — "these 3 risks are what I'm watching right now." The human curates what gets focused attention. They concentrate on what matters today.
- **The periphery is still monitored** — when something outside the spotlight starts moving (more mentions, higher severity language, senior people getting involved), the AI notices and surfaces it.
- **The spotlight moves** — things move in and out of focus. When something enters the spotlight, the human needs instant context: When was this first raised? By whom? What's the history? Who's involved? What's the current state?

### Human Signals Are First-Class Data

When the human says "I have a bad feeling about this one" or "Sarah told me offline this is under control" or "elevate this to critical" — those are not overrides of the AI's judgment. They are **inputs** that the AI doesn't have access to any other way. The system must capture and use them:

- **Watch lists** — human-curated assertions/topics under active attention, with optional notes
- **Priority overrides** — human judgment about what matters, independent of AI classification
- **Annotations** — offline context, gut feel, political dynamics, verbal commitments
- **Trust signals** — "when this person says it's an issue, I believe them" (see below)

### Bidirectional Prompting

The human asks Claude questions. But Claude also prompts the human:

- "3 new risks surfaced this week — what are your thoughts?"
- "VxLAN hasn't been mentioned in 2 weeks. Is it resolved, or has everyone just forgotten?"
- "You've been watching the CDN capacity risk for 3 weeks with no change. Should we close it or escalate?"
- "A VP just entered the VxLAN discussion for the first time. Want to add this to your watch list?"

Claude should know when to ask for human input — not for every minor update, but when something has changed character or when human judgment would meaningfully improve the system's understanding.

### Trust and Seniority: Two Different Axes

**Seniority** is organizational hierarchy — VP, Director, Senior Engineer, Junior Engineer. It matters because:
- When a VP flags something, it carries organizational weight regardless of whether they're technically correct
- A VP attending a meeting they weren't previously involved in is a strong signal about organizational priority
- The seniority profile of people discussing a topic is itself a signal (escalation = senior people getting involved)

**Trust** is personal and subjective. There are specific people whose judgment you rely on:
- A staff engineer you've worked with for 5 years who says "this is going to be a problem" — that may carry more weight for you than an unfamiliar VP
- Trust can be domain-specific: "I trust Sarah on technical risks but not on timeline estimates"
- Trust is the human's private signal — the AI captures it but doesn't second-guess it

Both seniority and trust factor into how the system weights assertions and surfaces changes:
- An assertion from a trusted person or a senior person gets higher initial visibility
- A change in the seniority profile of people involved with a topic triggers a peripheral alert
- The human can always override by adding something to their watch list regardless of who raised it

### The Collaboration Loop

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│   AI tracks everything            Human focuses the spotlight       │
│   (completeness)                  (judgment)                        │
│        │                                │                           │
│        ▼                                ▼                           │
│   AI detects changes ◄──── Human adds context ────▶ Watch list     │
│   in the periphery          (trust, notes,          updated        │
│        │                     gut feel, offline       │              │
│        │                     conversations)          │              │
│        ▼                                             ▼              │
│   AI surfaces alerts ────▶ Human makes decisions ◄── Briefings     │
│   "This changed"           "Elevate this"            on demand     │
│   "VP just joined"         "Close that one"                        │
│   "Gone quiet"             "Note: Mike says handled"               │
│                                                                     │
│                    Both get smarter over time                        │
│         AI learns what the human watches and trusts                 │
│         Human trusts the AI's peripheral monitoring                 │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Claude's Memory Problem — and the Solution

The human wakes up with their memory intact. Claude wakes up with amnesia. Every session starts from zero context.

The solution separates **personality** from **memory**:

| Layer | Source | What It Contains | When It Changes |
|-------|--------|-----------------|-----------------|
| **Personality** | CLAUDE.md | How to behave, what the system is, when to prompt the human, how to interpret trust/seniority | When the system design changes |
| **Memory** | `penf context morning` | Watch list, recent changes, active projects, trusted people, last session summary | Every day, every session |
| **Depth** | `penf` queries on demand | Full assertion history, golden thread, complete briefings | Real-time |

**Session bootstrap**: Claude's first action every session is `penf context morning`, which returns a structured briefing: what you're tracking, what changed since last session, who matters, what needs your input. Claude reconstructs its working memory from the database.

**Going deeper**: The bootstrap is a summary — enough to start the conversation. When the human asks about a specific risk, Claude queries `penf assertion briefing --root-id 101` for the full golden thread: origin, lifecycle events, key people, escalation chain, linked content.

**Session persistence**: At session end, Claude persists what it learned that isn't already captured: `penf context session-end --summary "..."`. This loads next morning as part of the bootstrap.

**The key insight**: Claude's memory lives in the database, not in its context window. The context window is a scratchpad for the current conversation. The database is long-term memory. `penf` is the bridge between them.

### What This Means for the AI Pipeline

The SLM/LLM architecture is designed around this collaboration model:

1. **The AI tracks the whole** — every piece of content is processed, every assertion extracted, every mention logged. No filtering based on "importance." Completeness first.
2. **Extraction is weighted** — assertions carry weight based on who said them (seniority + trust). The AI doesn't decide significance; it observes and weights.
3. **The human curates through conversation** — via Claude Code, the human manages their spotlight, adds context, and makes priority decisions in natural language.
4. **Change detection is proactive** — the AI monitors the periphery and surfaces changes worth human attention, especially changes in seniority profile or assertion frequency.
5. **Briefings assemble on demand** — when the spotlight moves, the system assembles full context instantly because it's been tracking everything all along.

---

## How Users Interact

Penfold is **AI-native** — it is designed for an AI assistant (Claude Code) to help users, not for direct human CLI use:

```
User (natural language) → Claude Code → penf CLI (JSON output) → Gateway (gRPC) → Services
```

Users ask questions in natural language. Claude Code translates that into appropriate CLI commands, processes the structured output, and presents findings conversationally. The human never sees CLI commands or raw JSON.

**Example interaction:**

> **Human:** "What's happening with my watched risks this week?"
>
> **Claude:** "VxLAN had 3 new mentions — two passing, one substantive. The substantive one was in Thursday's TER where Sarah Chen (VP Engineering) said she wants a mitigation plan by end of month. That's new — she wasn't involved before. The other two risks on your watch list are quiet."
>
> **Human:** "OK, escalate VxLAN to critical. And that CDN capacity one — I talked to Mike offline, he says it's handled. Add that note and drop it from my watch list."
>
> **Claude:** *Updates via penf commands. The offline context is now in the system.*

The CLI (`penf`) is optimized for AI consumption:
- Commands support `--format json` for structured output
- Batch processing commands exist for intelligent bulk operations
- Help text is designed for AI comprehension

---

## Core Principles

### 1. Human + AI Collaboration (see above)
The system is a partnership. The AI tracks everything; the human focuses the spotlight. Both add value the other can't.

### 2. Entity-Centric
Everything revolves around entities: **People**, **Products**, **Projects**, **Teams**, and **Glossary Terms**. Content is linked to entities through mention resolution, and entities are linked to each other through participation, ownership, and context.

### 3. Progressive Enhancement
The knowledge base improves over time:
- Auto-discovered entities start as "needs review"
- Human feedback confirms and refines
- Resolution patterns learned from corrections auto-apply to new content
- Human trust signals and annotations accumulate, making the system more attuned

### 4. Privacy by Design
- All data stays on infrastructure you control
- Multi-tenant isolation keeps contexts separate
- Local AI models (MLX on Apple Silicon) avoid sending content to external services
- Trust and personal annotations are private to the user

## Primary Use Cases

| Use Case | Priority | Status |
|----------|----------|--------|
| **Semantic Search** — hybrid search across all content types | Tier 1 | Implemented |
| **Meeting Intelligence** — transcript ingestion, speaker ID, topic extraction | Tier 1 | Implemented |
| **Email Archive** — .eml ingestion with thread reconstruction | Tier 1 | Implemented |
| **Terminology Management** — glossary with query expansion | Tier 1 | Implemented |
| **Expertise Discovery** — who knows what, who owns what | Tier 2 | Partial |
| **Product Knowledge** — product history, teams, decisions, timeline | Tier 2 | Partial |
| **Question Resolution** — review queue for AI questions | Tier 2 | Implemented |
| **Risk & Assertion Tracking** — RAID lifecycle with human spotlight | Tier 2 | Designed |
| **Daily Review & Triage** — prioritized daily digest with bidirectional prompting | Tier 3 | Planned |

## Content Types Processed

| Type | Source Format | Key Characteristics |
|------|-------------|-------------------|
| Email | .eml files (MIME) | Thread reconstruction, participant extraction, attachment handling |
| Meeting transcripts | Text/VTT from Webex/Teams/Zoom | Speaker identification, long form (44-64K chars), topic segmentation |
| Calendar events | iCal | Meeting series, attendees, recurrence |
| Documents | Various | Attachments extracted from emails, link enrichment |
| Slack messages (planned) | JSON export | High volume, threaded conversations |

## What This Spec Folder Contains

| File | Purpose |
|------|---------|
| `00-overview.md` | This file — vision, **collaboration philosophy**, and context |
| `01-architecture.md` | Service topology, deployment, communication patterns |
| `02-data-model.md` | Database schema, key tables, relationships |
| `03-entities.md` | Entity types, resolution, how they interconnect |
| `04-ai-services.md` | AI stack, backends, routing, embeddings |
| `05-content-pipeline.md` | Ingestion, processing, enrichment, search |
| `06-constraints.md` | Hardware, cost, and operational constraints |
| `design.md` | The core SLM/LLM architecture design narrative |
| `model-selection.md` | 7B vs 14B vs 32B tradeoffs on Apple Silicon |
| `prompt-engineering.md` | SLM vs LLM prompt rules, examples, validation |
| `test-data-validation.md` | Analysis of 267 emails and 18 transcripts |
| `cost-model.md` | Per-email cost breakdowns, batch economics |
| `implementation.md` | What exists, what needs building, design principles |
| `README.md` | Orientation guide for external AI advisors |
