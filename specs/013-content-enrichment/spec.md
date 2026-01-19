# Feature Specification: Content Enrichment Pipeline

**Feature Branch**: `013-content-enrichment`
**Created**: 2026-01-19
**Status**: Draft
**Input**: Discussion on pre-processor architecture for handling different email types and generic entity resolution

---

## Sub-Specifications

This specification is organized into the following sub-documents:

| Document | Description |
|----------|-------------|
| [classification.md](classification.md) | Content type hierarchy, detection heuristics, processing profiles |
| [entity-resolution.md](entity-resolution.md) | People, teams, projects, aliases, duplicate detection |
| [extraction.md](extraction.md) | AI processing, templates, context injection, assertions, audit trail |
| [type-handlers.md](type-handlers.md) | Jira, calendar, Google notifications, attachments, links |
| [queues-workers.md](queues-workers.md) | Queue architecture, worker pools, error handling, retry logic |
| [observability.md](observability.md) | Events, metrics, tracing, alerting, debugging |
| [configuration.md](configuration.md) | Tenant config, domains, patterns, integrations, initial data load |
| [api.md](api.md) | Query library, search extension, CLI commands |
| [appendix.md](appendix.md) | Test email analysis, example flows |

---

## Spec Review (2026-01-19)

### What Works Well
- Clear 5-stage pipeline with explicit processor registry
- Content type hierarchy (type + subtype + processing_profile)
- Detailed flow examples (Jira notification, regular thread)
- Comprehensive edge case coverage
- Good requirements organization (FR-xxx numbering)
- Context-augmented extraction pattern well documented
- Audit trail for A/B testing and quality control
- Project-based templates with explicit fallback chain

### Gap Status

| Gap | Impact | Priority | Status |
|-----|--------|----------|--------|
| **Queue & Worker Architecture** | Can't understand how work is distributed | P0 | ✅ Added |
| **Error Handling & Recovery** | Pipeline will fail silently | P0 | ✅ Added |
| **Assertions Table Definition** | Extraction output has no home | P0 | ✅ Added |
| **Tenant Configuration** | How do tenants set up domains, Jira, etc.? | P1 | ✅ Added |
| **API Surface** | How does search consume enrichment? | P1 | ✅ Added |
| **Initial Data Load** | How to bootstrap people/teams/projects? | P1 | ✅ Added |
| **Observability** | Events mentioned but not defined | P1 | ✅ Added |
| **Security & Multi-tenancy** | PII handling, data isolation | P2 | Open |

---

## Clarifications

### Session 2026-01-19

- Q: Should entity resolution be email-specific or generic? → A: Generic - same person resolution needed for email, Slack, calendar, documents
- Q: How should different email types (meeting cancellations, Jira notifications, regular emails) be handled? → A: Classification layer that routes to type-specific handlers, then generic enrichment
- Q: Should auto-generated emails (Jira, calendar) receive full AI processing? → A: Configurable - likely skip embedding for notifications, but still extract entities

### Open Questions

**Resolved:**
- ✅ How do we handle external participants? → Use Exchange header `X-MS-Exchange-Organization-AuthAs` + domain matching
- ✅ How to avoid Jira false positives? → Check sender first, require Auto-Submitted header

**Remaining:**
- How should we handle conflicting entity resolution (same email, different display names)?
  - *Proposed*: Keep most recent display name, store all variants as aliases with timestamps
- What is the priority order when the same person appears in multiple sources?
  - *Proposed*: Gmail API > manual import > inferred from content
- Should meeting invites/cancellations update a calendar state model?
  - *Proposed*: Yes, track `meetings` table with invite/cancel/update events
- How do we handle distribution lists that expand to many recipients?
  - *Proposed*: Flag as `account_type=distribution`, don't expand unless membership known
- Should we auto-create person records or require confirmation?
  - *Proposed*: Auto-create with `confidence=low`, queue for user confirmation
- How do we handle forwarded email attribution (original sender in body)?
  - *Proposed*: Parse "From:" lines in quoted content, link to `forwarded_from_person_id`

---

## Overview

This specification defines a **content enrichment pipeline** that processes ingested content through multiple stages:

1. **Source Adapters** - Normalize content from different sources (email, Slack, calendar) into a unified model
2. **Content Classification** - Identify content type (meeting invite, Jira notification, regular email, etc.)
3. **Entity Resolution** - Map identifiers (email addresses, Slack IDs, names) to canonical person/team/project records
4. **Type-Specific Enrichment** - Extract type-specific metadata (meeting details, ticket IDs, action items)
5. **AI Processing** - Generate embeddings, summaries, assertions (existing pipeline)

The goal is to create a **source-agnostic enrichment layer** where entity resolution and classification work identically regardless of whether content came from Gmail, manual .eml upload, Slack, or future sources.

---

## Problem Statement

### Current State

The existing ingest pipeline treats all emails identically:
```
.eml → Parser → Store in sources → Event → AI Pipeline (embed, summarize)
```

### Issues Identified

1. **No Content Classification**: Meeting cancellations, Jira notifications, and substantive emails all receive the same AI processing
2. **No Entity Resolution**: `jabrown@akamai.com`, `James Brown`, and `@jabrown` are treated as different entities
3. **Source-Specific Logic**: Email parsing is tightly coupled - adding Slack would duplicate entity extraction
4. **Wasted Processing**: Auto-generated emails (Jira, calendar) get expensive embeddings that add little search value
5. **No Relationship Tracking**: We don't track who communicates with whom, team structures, or organizational hierarchy

### Evidence from Test Data

Analysis of `/penfold_test_data/email-small/` reveals distinct email types:

| File | Type | Key Characteristics |
|------|------|---------------------|
| `Canceled- MTC Leader Standup[4].eml` | Meeting Cancellation | Calendar action, attendee list, WebEx details |
| `[TRACK-JIRA] Updates for OUT-697...eml` | Jira Notification | Auto-generated, ticket reference, status update |
| `Re- Follow up action on MTC.eml` | Regular Thread | Human-written, action items, Google Doc link |
| `FW- PACE Technical Readiness Review...eml` | Forward | Attribution needed, original sender important |
| `RE- Tiktok FY26 discounts.eml` | Thread Reply | Part of conversation, pricing discussion |

---

## Architecture

### Proposed Pipeline

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              INGEST LAYER                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Email (.eml)      Gmail API       Slack Export      Calendar (ICS)        │
│        │                │                │                  │                │
│        ▼                ▼                ▼                  ▼                │
│   ┌─────────┐      ┌─────────┐      ┌─────────┐      ┌─────────┐            │
│   │  Email  │      │  Gmail  │      │  Slack  │      │ Calendar│            │
│   │ Adapter │      │ Adapter │      │ Adapter │      │ Adapter │            │
│   └────┬────┘      └────┬────┘      └────┬────┘      └────┬────┘            │
│        │                │                │                  │                │
│        └────────────────┴────────────────┴──────────────────┘                │
│                                    │                                         │
│                                    ▼                                         │
│                         ┌───────────────────┐                                │
│                         │  Unified Content  │                                │
│                         │      Model        │                                │
│                         └─────────┬─────────┘                                │
│                                   │                                          │
└───────────────────────────────────┼──────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ENRICHMENT LAYER                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌─────────────────┐                                                        │
│   │   Classifier    │  → email/thread, calendar/invite, notification/jira   │
│   └────────┬────────┘                                                        │
│            │                                                                 │
│            ▼                                                                 │
│   ┌─────────────────┐                                                        │
│   │ Entity Resolver │  → people, teams, projects                            │
│   └────────┬────────┘                                                        │
│            │                                                                 │
│            ▼                                                                 │
│   ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│   │ Meeting Handler │     │  Jira Handler   │     │ Standard Handler│       │
│   │ (if calendar/*)│     │(if notification/│     │  (if email/*)   │       │
│   └────────┬────────┘     │      jira)      │     └────────┬────────┘       │
│            │              └────────┬────────┘              │                │
│            └───────────────────────┼───────────────────────┘                │
│                                    │                                         │
│                                    ▼                                         │
│                         ┌───────────────────┐                                │
│                         │  Enriched Content │                                │
│                         └─────────┬─────────┘                                │
│                                   │                                          │
└───────────────────────────────────┼──────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AI PROCESSING LAYER                                │
│                        (existing pipeline)                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐       │
│   │   Embedding     │     │   Summarization │     │    Assertion    │       │
│   │   Generation    │     │                 │     │   Extraction    │       │
│   └─────────────────┘     └─────────────────┘     └─────────────────┘       │
│                                                                              │
│   (Skip for notification/* types based on configuration)                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5-Stage Pipeline Summary

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           ENRICHMENT PIPELINE                                    │
│                                                                                  │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐      │
│  │  STAGE 1 │──►│  STAGE 2 │──►│  STAGE 3 │──►│  STAGE 4 │──►│  STAGE 5 │      │
│  │ Classify │   │  Enrich  │   │  Extract │   │  Route   │   │    AI    │      │
│  └──────────┘   └──────────┘   └──────────┘   └──────────┘   └──────────┘      │
│                                                                                  │
│  Determines     Applies to      Type-specific   Decides       Based on          │
│  type/subtype   ALL content     handlers        AI profile    profile           │
└─────────────────────────────────────────────────────────────────────────────────┘
```

See [classification.md](classification.md) for detailed pipeline stages.

---

## Core Data Model

### Primary Tables

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│     people      │       │     teams       │       │    projects     │
├─────────────────┤       ├─────────────────┤       ├─────────────────┤
│ id              │       │ id              │       │ id              │
│ tenant_id       │       │ tenant_id       │       │ tenant_id       │
│ canonical_name  │◄──────│ name            │       │ name            │
│ primary_email   │       │ description     │       │ description     │
│ title           │       │ created_at      │       │ keywords[]      │
│ department      │       └────────┬────────┘       │ jira_projects[] │
│ is_internal     │                │                │ created_at      │
│ account_type    │                │                └────────┬────────┘
│ created_at      │       ┌────────┴────────┐                │
│ updated_at      │       │  team_members   │       ┌────────┴────────┐
└────────┬────────┘       ├─────────────────┤       │ project_members │
         │                │ team_id         │       ├─────────────────┤
         │                │ person_id       │       │ project_id      │
┌────────┴────────┐       │ role            │       │ person_id       │
│ person_aliases  │       │ joined_at       │       │ role            │
├─────────────────┤       └─────────────────┘       └─────────────────┘
│ person_id       │
│ alias_type      │  ← email/slack_id/name/display_name
│ alias_value     │
│ confidence      │
│ source          │
└─────────────────┘
```

### Account Types

| Type | Description | Example |
|------|-------------|---------|
| `person` | Human individual | `sweisman@akamai.com` |
| `role` | Shared role/function account | `Prb-Facilitator@akamai.com` |
| `distribution` | Mailing list | `team-mtc@akamai.com` |
| `bot` | Automated system | `gsd-jira@akamai.com` |
| `external_service` | External notification | `comments-noreply@docs.google.com` |

### Content Enrichment

```
┌─────────────────────┐
│ content_enrichment  │
├─────────────────────┤
│ source_id           │
│ content_type        │  ← email, calendar, document, attachment
│ content_subtype     │  ← thread, notification/jira, invite
│ processing_profile  │  ← full_ai, metadata_only, state_tracking
│ participants[]      │  ← resolved person_ids
│ teams[]             │  ← auto-detected teams
│ projects[]          │  ← linked projects
│ extracted_data      │  ← type-specific JSON
│ ai_processed        │  ← true/false
│ ai_skip_reason      │  ← why AI skipped (if applicable)
│ enriched_at         │
└─────────────────────┘
```

See [entity-resolution.md](entity-resolution.md) for full data model details.

---

## Dependencies

- **012-manual-ingest**: Email parsing and storage (implemented)
- **004-gmail-integration**: Gmail API integration (implemented)
- **007-search-interface**: Search filtering by classification (implemented)
- **009-relationship-discovery**: Related but separate - this spec provides entity data

---

## Enabled Queries

With the enrichment layer, these previously impossible queries become available:

### People-Centric
- "Show me all communications with Sara Weisman" → unified across email variations
- "Who does Rick Eskelsen communicate with most?" → participant graph
- "Show me everyone who's worked on the MTC project" → from emails, Jira, meetings

### Document-Centric
- "What documents are referenced in the TikTok discussions?" → extracted links
- "Show me emails that link to this Google Doc" → bidirectional link lookup
- "Who has access to this document?" → enriched from Google API

### Project-Centric
- "Show me all activity on OUT-697" → Jira state changes + email discussions + meetings
- "What's the history of the MTC project?" → across tickets, threads, docs
- "Which tickets are blocked or stale?" → from Jira notification state

### Meeting-Centric
- "What meetings did I miss last week?" → cancelled meetings I was invited to
- "Who usually organizes meetings about TikTok?" → meeting organizer patterns
- "Show me action items from the MTC standup" → from meeting-related emails

### Thread-Centric
- "Show me the full discussion about FY26 discounts" → unified thread view
- "What decisions were made in this thread?" → thread summary + assertions
- "Who participated but never replied?" → CC participants

### Attachment-Centric
- "Find spreadsheets about pricing" → attachment text search
- "Show me all PDFs from Q1 2026" → attachment filtering
- "What files has this person shared?" → attachment + sender correlation

---

## Success Metrics

### Core Enrichment
- Entity resolution accuracy: >95% for internal participants
- Classification accuracy: >90% for defined content types
- Processing latency: <200ms per item through full enrichment
- AI processing cost reduction: 30% reduction by skipping notifications
- Link extraction recall: >98% of URLs in body/HTML extracted
- Attachment text extraction: >90% success rate for PDF/Office docs
- Thread grouping accuracy: >95% of replies correctly associated

### Extraction Quality
- Schema compliance: >99% of extractions parse successfully
- Entity grounding rate: >85% of names resolved to person_ids
- User correction rate: <10% of extractions require manual correction
- Action completeness: >90% of actions have assignee + due date

### Audit & Testing
- Audit coverage: 100% of extractions have full audit trail
- Replay fidelity: Replayed extractions match originals (same input → same output)
- A/B test power: Experiments reach statistical significance within 1 week

### Thread Processing (Batch)
- Thread detection accuracy: >99% of replies grouped correctly
- Batch efficiency: 60% reduction in AI calls via latest-only extraction
- Thread summary quality: User satisfaction >4/5 on thread overviews

---

## Implementation Files

```
pkg/enrichment/
├── pipeline/
│   ├── pipeline.go         # Main orchestrator
│   ├── stages.go           # Stage definitions
│   └── registry.go         # Processor registry
├── classification/
│   ├── classifier.go       # Content classification
│   └── rules.go            # Classification rules
├── entities/
│   ├── resolver.go         # Entity resolution
│   ├── people.go           # Person management
│   ├── teams.go            # Team management
│   └── projects.go         # Project management
├── handlers/
│   ├── jira.go             # Jira notification handler
│   ├── calendar.go         # Calendar event handler
│   ├── google.go           # Google notification handler
│   └── email.go            # Standard email handler
├── extraction/
│   ├── context.go          # Context builder
│   ├── templates.go        # Template management
│   ├── extractor.go        # LLM extraction
│   └── audit.go            # Audit trail
├── workers/
│   ├── pool.go             # Worker pool management
│   ├── ingest.go           # Ingest workers
│   ├── enrichment.go       # Enrichment workers
│   └── ai.go               # AI processing workers
├── queues/
│   ├── redis.go            # Redis queue implementation
│   ├── messages.go         # Message types
│   └── dlq.go              # Dead letter queue
├── observability/
│   ├── events.go           # Event schemas
│   ├── metrics.go          # Metrics collection
│   └── tracing.go          # Distributed tracing
├── query/
│   ├── people.go           # People queries
│   ├── threads.go          # Thread queries
│   ├── assertions.go       # Assertion queries
│   └── status.go           # Status queries
├── experiments/
│   ├── runner.go           # A/B test runner
│   └── metrics.go          # Experiment metrics
└── config/
    ├── tenant.go           # Tenant configuration
    └── secrets.go          # Secrets management
```
