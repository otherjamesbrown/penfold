# Context Palace - Product Specification

## Document Information

| Field | Value |
|-------|-------|
| Version | 0.1.0 |
| Status | Draft |
| Author | James |
| Created | 2025-01-09 |
| Last Updated | 2025-01-09 |

---

## 1. Executive Summary

### 1.1 Vision Statement

Context Palace is a personal AI-powered information system that aggregates, correlates, and surfaces contextual information from disparate communication channels. It transforms fragmented organisational knowledge into a navigable, queryable institutional memory centred on the user.

### 1.2 Problem Statement

As COO of a 250-person company, critical information is scattered across multiple platforms:

- **Email**: Status updates buried in noise; important context lost or forgotten; poor search makes retrieval difficult
- **Slack/Teams**: Group channels go unmonitored; conversations about projects split across multiple channels; no correlation between discussions
- **Documents**: Hundreds of documents with no way to locate them weeks later; context about decisions lost
- **Meetings**: AI summaries exist but lack connection to email/Slack context; insights are forgotten after the initial read

Additionally:
- **People context is missing**: Nicknames don't map to official names; team affiliations unclear
- **Temporal understanding is absent**: No way to reconstruct timelines of decisions, understand what happened when, or track how initiatives evolved
- **Projects lack a "space"**: No single place to see all artifacts, people, history, and status for a given initiative

### 1.3 Solution Overview

Context Palace provides:

1. **Unified ingestion** from email, Slack, documents, and meeting summaries
2. **AI-powered extraction** of assertions (decisions, risks, commitments, context) from raw content
3. **Entity resolution** for people, teams, and projects
4. **Project spaces** that aggregate all relevant context, artifacts, and history
5. **Daily/weekly review workflows** with human-in-the-loop feedback
6. **Natural language querying** for retrieval and synthesis

### 1.4 Target User

- Single user: The author (COO of 250-person company)
- Personal use for learning AI development
- Power user comfortable with CLI interfaces
- Uses Claude Code extensively

### 1.5 Success Criteria

| ID | Metric | Target |
|----|--------|--------|
| SC-001 | Time to answer "what happened on Project X last month" | < 30 seconds |
| SC-002 | Time to find a specific document mentioned in conversation | < 15 seconds |
| SC-003 | Daily review completion time | < 30 minutes |
| SC-004 | Percentage of "signal" emails correctly identified | > 90% |
| SC-005 | User actively uses the system daily | Sustained usage over 30+ days |

---

## 2. User Stories

### US-001: Daily Morning Review

**As a** COO starting my day  
**I want to** see a triage queue of items needing my input, a digest from news-feed channels, and my processed flagged items  
**So that** I can quickly understand what needs attention and provide feedback to improve the system

**Acceptance Criteria:**
- [ ] System presents items requiring classification (new entities, ambiguous assertions, uncertain person references)
- [ ] System shows summarised highlights from news-feed channels with anomaly detection
- [ ] System shows results of processing my flagged items from yesterday
- [ ] I can provide feedback on classifications with single keystrokes
- [ ] Session completes in under 30 minutes for typical daily volume

**Independent Test:** Run `cxp review --daily` and complete a full triage session, verifying all item types appear and feedback is captured.

---

### US-002: Weekly Project Health Review

**As a** COO planning my week  
**I want to** see health summaries for all active projects including activity levels, risks, decisions, and stalled initiatives  
**So that** I can identify where my attention is needed and spot patterns across the organisation

**Acceptance Criteria:**
- [ ] Each active project shows activity level (high/medium/low/stale) with trend
- [ ] Open risks and overdue commitments are surfaced
- [ ] Projects with declining activity are flagged as potentially stalled
- [ ] Cross-project patterns are detected and highlighted (e.g., "timeline concerns appearing in 4 projects")
- [ ] I can drill down into any project from the summary

**Independent Test:** Run `cxp review --weekly` and verify project health data matches manual inspection of recent sources.

---

### US-003: Project Context Retrieval

**As a** user preparing for a meeting  
**I want to** ask "catch me up on Project X over the last month" and receive a synthesised narrative  
**So that** I can quickly understand the current state, recent decisions, and open issues

**Acceptance Criteria:**
- [ ] System retrieves all assertions linked to the project in the time range
- [ ] Assertions are grouped by type (milestones, decisions, risks, concerns)
- [ ] A narrative summary is generated highlighting key events
- [ ] Source links are provided for each assertion
- [ ] Response time is under 30 seconds

**Independent Test:** Query a known project with known recent activity and verify the summary is accurate and complete.

---

### US-004: Artifact Location

**As a** user looking for a document  
**I want to** ask "find the SOW for Project X" and receive the direct link  
**So that** I don't waste time searching through Drive/SharePoint/email

**Acceptance Criteria:**
- [ ] System searches project artifacts by name, description, and tags
- [ ] Direct URL is returned for matching documents
- [ ] If multiple matches exist, ranked list is presented
- [ ] If no exact match, similar artifacts are suggested
- [ ] Response time is under 15 seconds

**Independent Test:** Add a known artifact to a project, then query for it using various phrasings.

---

### US-005: Decision Archaeology

**As a** user trying to understand past decisions  
**I want to** ask "when did we decide to use Kafka instead of RabbitMQ" and see the decision with context  
**So that** I can understand the rationale and who was involved

**Acceptance Criteria:**
- [ ] System searches assertions of type "decision" semantically
- [ ] Returns the decision assertion with timestamp and actors
- [ ] Provides link to source (meeting, email, document)
- [ ] Shows related assertions from the same source for context
- [ ] Can navigate to the full source to understand the discussion

**Independent Test:** Query for a known decision and verify the context and actors are correct.

---

### US-006: Person Context Lookup

**As a** user seeing an unfamiliar name  
**I want to** ask "who is Rishi" and understand who they are and their role  
**So that** I have context when their name appears in discussions

**Acceptance Criteria:**
- [ ] System resolves aliases to canonical person records
- [ ] Shows person's team, role, and any notes I've added
- [ ] Shows recent activity (assertions they appear in)
- [ ] Shows projects they're associated with

**Independent Test:** Query for a person by alias and verify resolution to canonical record.

---

### US-007: Source Mode Configuration

**As a** user setting up the system  
**I want to** configure different channels for different processing modes (deep analysis, news-feed, opt-in)  
**So that** I control the depth of analysis based on channel importance

**Acceptance Criteria:**
- [ ] Can designate Slack channels as "deep analysis" (full extraction)
- [ ] Can designate Slack channels as "news-feed" (summary only, anomaly detection)
- [ ] Can mark email as "opt-in" (only process flagged items)
- [ ] Configuration persists and is applied during ingestion
- [ ] Can change mode for any source at any time

**Independent Test:** Configure a channel in each mode, run ingestion, verify processing matches the mode.

---

### US-008: Flagging Content for Analysis

**As a** user reading email  
**I want to** flag specific emails or threads for full analysis  
**So that** important content gets extracted even from opt-in sources

**Acceptance Criteria:**
- [ ] Can flag emails via label/folder (Gmail)
- [ ] Flagged items are processed in next ingestion run
- [ ] Extraction results appear in daily review
- [ ] Can un-flag to exclude from future processing

**Independent Test:** Flag an email, run ingestion, verify it appears in processed items.

---

### US-009: Entity Graph Management

**As a** user during daily review  
**I want to** resolve unknown person references and create new entity records  
**So that** the system builds accurate knowledge of my organisation

**Acceptance Criteria:**
- [ ] System prompts for unknown name resolution ("Mike" could be Mike Chen or Mike Torres)
- [ ] Can create new person record with canonical name, aliases, team, role
- [ ] Can merge duplicate person records
- [ ] Can add notes to person records
- [ ] Changes take effect immediately for future queries

**Independent Test:** Encounter an unknown person in review, create record, verify subsequent references resolve correctly.

---

### US-010: Project Space Management

**As a** user tracking initiatives  
**I want to** create and manage project spaces with associated people, artifacts, and context  
**So that** all project-related information is accessible in one place

**Acceptance Criteria:**
- [ ] Can create new project with name, description, owner, team, goals
- [ ] Can add artifacts (documents, links, spreadsheets) to project
- [ ] Can associate Slack channels with project
- [ ] System auto-links sources to projects based on keywords, participants, channels
- [ ] Can view project timeline showing all assertions chronologically
- [ ] Can mark projects as active, on-hold, completed, cancelled

**Independent Test:** Create a project, add artifacts, associate channels, verify sources auto-link correctly.

---

### US-011: Theme and Project Discovery

**As a** user reviewing new information  
**I want to** be alerted when the system detects potential new projects or themes  
**So that** emerging initiatives are tracked without manual setup

**Acceptance Criteria:**
- [ ] System clusters related assertions that don't match existing projects
- [ ] Suggests project creation with proposed name, related people, sample assertions
- [ ] User can accept (create project), reject (ignore), or defer
- [ ] Accepted suggestions create project with initial assertions linked

**Independent Test:** Ingest sources with content about a new initiative, verify system suggests project creation.

---

### US-012: Timeline Visualisation

**As a** user reviewing a project  
**I want to** see a timeline view of all decisions, milestones, and key events  
**So that** I understand the chronological progression of the initiative

**Acceptance Criteria:**
- [ ] Timeline shows assertions filtered by project and type
- [ ] Can filter to specific assertion types (decisions only, risks only, etc.)
- [ ] Can zoom in/out on time ranges
- [ ] Each item links to source for full context
- [ ] Timeline renders in CLI as text-based visualisation

**Independent Test:** View timeline for a project with known history, verify chronological accuracy.

---

## 3. Functional Requirements

### 3.1 Data Ingestion

| ID | Requirement |
|----|-------------|
| FR-001 | System MUST connect to Gmail API to retrieve emails |
| FR-002 | System MUST connect to Google Drive API to retrieve documents |
| FR-003 | System MUST connect to Slack API to retrieve channel messages |
| FR-004 | System MUST support batch ingestion on a configurable schedule (default: nightly) |
| FR-005 | System MUST track last-sync timestamp per source to enable incremental ingestion |
| FR-006 | System MUST store raw content for re-processing and context retrieval |
| FR-007 | System MUST support three processing modes per source: deep-analysis, news-feed, opt-in |
| FR-008 | System SHOULD support meeting transcript ingestion (format TBD based on tooling) |

### 3.2 Assertion Extraction

| ID | Requirement |
|----|-------------|
| FR-010 | System MUST use tiered LLM processing: local model for classification/routing, cloud API for extraction |
| FR-011 | System MUST extract assertions of types: decision, risk, concern, commitment, milestone, position, context, outcome |
| FR-012 | System MUST link assertions to source records |
| FR-013 | System MUST extract actor references from assertions |
| FR-014 | System MUST assign confidence scores to extractions |
| FR-015 | System MUST generate embeddings for semantic search |
| FR-016 | System SHOULD filter noise before sending to cloud API (cost optimisation) |
| FR-017 | System MUST support re-extraction when extraction logic improves |

### 3.3 Entity Resolution

| ID | Requirement |
|----|-------------|
| FR-020 | System MUST maintain a person entity graph with canonical names and aliases |
| FR-021 | System MUST resolve name references in assertions to person entities |
| FR-022 | System MUST flag unknown name references for user resolution |
| FR-023 | System MUST support person attributes: team, role, notes |
| FR-024 | System MUST maintain a team entity with membership |
| FR-025 | System SHOULD suggest alias mappings based on context |

### 3.4 Projects and Themes

| ID | Requirement |
|----|-------------|
| FR-030 | System MUST support hierarchical project structure (theme → project → sub-project, max 2 levels) |
| FR-031 | System MUST support project attributes: name, description, owner, team, stakeholders, status, goals |
| FR-032 | System MUST support project artifacts with type, name, URL, description, tags |
| FR-033 | System MUST auto-discover artifact links from source content |
| FR-034 | System MUST auto-link sources to projects based on keywords, participants, and channel associations |
| FR-035 | System MUST support manual project-source linking |
| FR-036 | System SHOULD detect emerging themes from unlinked assertion clusters |
| FR-037 | System MUST track project activity metrics (assertion count, last activity, trend) |

### 3.5 Query and Retrieval

| ID | Requirement |
|----|-------------|
| FR-040 | System MUST support natural language queries via CLI |
| FR-041 | System MUST support semantic search across assertions using embeddings |
| FR-042 | System MUST support filtered queries by: project, person, time range, assertion type |
| FR-043 | System MUST synthesise narrative responses using LLM |
| FR-044 | System MUST provide source citations in query responses |
| FR-045 | System MUST support direct artifact lookup by project |
| FR-046 | System MUST support timeline queries with chronological ordering |

### 3.6 Review Workflows

| ID | Requirement |
|----|-------------|
| FR-050 | System MUST generate daily triage queue with items needing user input |
| FR-051 | System MUST generate news-feed summaries for configured channels |
| FR-052 | System MUST generate weekly project health summaries |
| FR-053 | System MUST capture user feedback (classifications, corrections) and persist |
| FR-054 | System MUST use feedback to improve future extraction accuracy |
| FR-055 | System SHOULD track review session metrics (items processed, time spent) |

### 3.7 CLI Interface

| ID | Requirement |
|----|-------------|
| FR-060 | System MUST provide CLI tool `cxp` with subcommands |
| FR-061 | System MUST support `cxp ingest` for manual ingestion trigger |
| FR-062 | System MUST support `cxp review --daily` for morning review workflow |
| FR-063 | System MUST support `cxp review --weekly` for weekly review workflow |
| FR-064 | System MUST support `cxp ask "<query>"` for natural language queries |
| FR-065 | System MUST support `cxp project <name>` for project operations |
| FR-066 | System MUST support `cxp timeline <query>` for timeline queries |
| FR-067 | System MUST support `cxp person <name>` for person lookups |
| FR-068 | System MUST support `cxp config` for source mode configuration |

---

## 4. Data Model

### 4.1 Core Entities

```
Source
├── id: UUID
├── type: enum (email | slack_message | slack_thread | document | meeting)
├── external_id: string (Gmail ID, Slack TS, Drive ID)
├── title: string
├── timestamp: datetime
├── participants: [Person.id]
├── raw_content: text
├── source_url: string
├── channel_id: string (for Slack)
├── thread_id: string (for threaded content)
├── processing_mode: enum (deep | news_feed | opt_in)
├── processed_at: datetime
├── projects: [Project.id]
├── metadata: jsonb

Assertion
├── id: UUID
├── source_id: Source.id
├── type: enum (decision | risk | concern | commitment | milestone | position | context | outcome)
├── content: text
├── status: enum (open | resolved | superseded | cancelled) [nullable]
├── owner: Person.id [nullable]
├── due_date: date [nullable]
├── actors: [Person.id]
├── projects: [Project.id]
├── salience: enum (high | medium | low)
├── confidence: float
├── embedding: vector(1536)
├── created_at: datetime
├── updated_at: datetime

Person
├── id: UUID
├── canonical_name: string
├── aliases: [string]
├── email: string [nullable]
├── team_id: Team.id [nullable]
├── role: string [nullable]
├── notes: text [nullable]
├── created_at: datetime
├── updated_at: datetime

Team
├── id: UUID
├── name: string
├── parent_team_id: Team.id [nullable]
├── created_at: datetime

Project
├── id: UUID
├── name: string
├── description: text
├── status: enum (active | on_hold | completed | cancelled)
├── owner: Person.id
├── team: [Person.id]
├── stakeholders: [Person.id]
├── parent_project_id: Project.id [nullable]
├── theme_id: Theme.id [nullable]
├── keywords: [string]
├── goals: [string]
├── started_at: date [nullable]
├── expected_completion: date [nullable]
├── actual_completion: date [nullable]
├── created_at: datetime
├── updated_at: datetime

Theme
├── id: UUID
├── name: string
├── description: text
├── keywords: [string]
├── created_at: datetime

ProjectArtifact
├── id: UUID
├── project_id: Project.id
├── type: enum (document | spreadsheet | link | recording | repository)
├── name: string
├── url: string
├── description: text [nullable]
├── tags: [string]
├── discovered_from_source_id: Source.id [nullable]
├── added_by: enum (system | user)
├── created_at: datetime

AssertionLink
├── id: UUID
├── assertion_id_1: Assertion.id
├── assertion_id_2: Assertion.id
├── link_type: enum (related | supersedes | contradicts | clarifies)
├── created_by: enum (system | user)
├── confidence: float [nullable]
├── created_at: datetime

SourceConfig
├── id: UUID
├── source_type: enum (gmail | slack_channel | drive_folder)
├── source_identifier: string (channel ID, folder ID, label)
├── processing_mode: enum (deep | news_feed | opt_in)
├── enabled: boolean
├── last_sync_at: datetime [nullable]
├── created_at: datetime
├── updated_at: datetime

UserFeedback
├── id: UUID
├── feedback_type: enum (entity_resolution | assertion_classification | project_link | other)
├── entity_type: string
├── entity_id: UUID
├── original_value: jsonb
├── corrected_value: jsonb
├── created_at: datetime
```

### 4.2 Key Relationships

```
Source (1) ──────── (*) Assertion
Source (*) ──────── (*) Project
Assertion (*) ───── (*) Person (actors)
Assertion (*) ───── (*) Project
Assertion (1) ───── (0..1) Person (owner)
Person (*) ──────── (0..1) Team
Project (*) ─────── (0..1) Theme
Project (*) ─────── (*) ProjectArtifact
Project (0..1) ──── (*) Project (parent/children)
Assertion (*) ───── (*) Assertion (via AssertionLink)
```

---

## 5. Technical Architecture

### 5.1 Development Environment

| Component | Technology | Notes |
|-----------|------------|-------|
| Development Machine | Mac Mini M4, 32GB RAM | Primary dev environment |
| Local LLM | Ollama + Llama 3.1 8B | Tier 1: classification, routing, entity extraction |
| Local Database | PostgreSQL 16 + pgvector | Development database |
| Local Embeddings | nomic-embed-text via Ollama | Free, fast, offline |
| Cloud LLM | Gemini API | Tier 2/3: extraction, synthesis |
| Language | Python 3.12 | Primary implementation language |

### 5.2 Production Environment (Future)

| Component | Technology | Notes |
|-----------|------------|-------|
| Database | Cloud SQL (PostgreSQL) | Managed PostgreSQL on GCP |
| Blob Storage | Cloud Storage | Raw content storage |
| Batch Jobs | Cloud Run Jobs | Nightly ingestion |
| Scheduler | Cloud Scheduler | Cron triggers |

### 5.3 Tiered LLM Processing

```
Tier 1: Local (Llama 3.1 8B via Ollama)
├── Classification: signal vs noise
├── Routing: which project does this relate to
├── Entity extraction: who is mentioned
├── Cost: Free
├── Latency: ~50-100ms

Tier 2: Gemini Flash
├── Assertion extraction from routine content
├── Summarisation for news-feed channels
├── Bulk processing
├── Cost: Low (~$0.01/day estimated)
├── Latency: ~500ms

Tier 3: Gemini Pro / Claude
├── Complex synthesis queries
├── Nuanced extraction from important sources
├── Interactive user queries
├── Weekly review analysis
├── Cost: Higher (usage-based)
├── Latency: ~1-2s
```

### 5.4 System Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLI (cxp)                                │
│  ├── ingest    - Trigger ingestion                             │
│  ├── review    - Daily/weekly review workflows                 │
│  ├── ask       - Natural language queries                      │
│  ├── project   - Project management                            │
│  ├── person    - Person lookups                                │
│  ├── timeline  - Timeline queries                              │
│  └── config    - Configuration management                      │
├─────────────────────────────────────────────────────────────────┤
│                    Core Library (cxp_lib)                       │
│  ├── connectors/   - Gmail, Drive, Slack connectors           │
│  ├── extraction/   - LLM-based extraction pipeline            │
│  ├── entities/     - Entity resolution logic                  │
│  ├── projects/     - Project management                       │
│  ├── query/        - Search and retrieval                     │
│  ├── review/       - Review workflow logic                    │
│  └── storage/      - Database access layer                    │
├─────────────────────────────────────────────────────────────────┤
│                      Data Layer                                 │
│  ├── PostgreSQL + pgvector                                     │
│  └── Blob storage (raw content)                                │
├─────────────────────────────────────────────────────────────────┤
│                    External Services                            │
│  ├── Gmail API                                                 │
│  ├── Google Drive API                                          │
│  ├── Slack API                                                 │
│  ├── Ollama (local)                                            │
│  └── Gemini API                                                │
└─────────────────────────────────────────────────────────────────┘
```

---

## 6. MVP Definition

### 6.1 MVP Scope

The MVP validates the core hypothesis: that AI-assisted extraction and organisation of information across sources provides genuine value for institutional memory and daily workflow.

**In Scope for MVP:**

1. **Single source: Gmail**
   - OAuth setup and API connection
   - Incremental sync (new emails since last run)
   - Opt-in mode only (flagged emails via label)

2. **Basic extraction pipeline**
   - Local model (Ollama) for noise filtering
   - Gemini API for assertion extraction
   - Support assertion types: decision, action, risk, information

3. **Minimal entity graph**
   - Person records with canonical name + aliases
   - Manual creation during review
   - No team hierarchy yet

4. **Single project space**
   - Create one test project
   - Manual artifact addition
   - Manual source linking

5. **Basic CLI**
   - `cxp ingest` - run extraction on flagged emails
   - `cxp review` - see extracted assertions, resolve unknown people
   - `cxp ask "<query>"` - semantic search + basic synthesis

6. **Local development only**
   - PostgreSQL + pgvector on Mac
   - No cloud deployment

**Out of Scope for MVP:**

- Slack integration
- Google Drive integration
- Meeting transcripts
- News-feed mode (summaries)
- Weekly review
- Project auto-discovery
- Timeline visualisation
- Assertion linking
- Cloud deployment

### 6.2 MVP User Stories

| Priority | User Story |
|----------|------------|
| P0 | US-008: Flagging Content for Analysis (Gmail labels) |
| P0 | US-003: Project Context Retrieval (basic version) |
| P0 | US-009: Entity Graph Management (manual creation) |
| P1 | US-005: Decision Archaeology (semantic search) |
| P1 | US-010: Project Space Management (manual only) |

### 6.3 MVP Success Criteria

| Criteria | Measure |
|----------|---------|
| End-to-end works | Flag email → extract assertions → query and find |
| Extraction quality | >80% of decisions correctly identified in test set |
| Query relevance | Semantic search returns relevant results for test queries |
| Daily usability | Author uses system for 5 consecutive days |

---

## 7. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Gmail API rate limits | Low | Medium | Batch requests, incremental sync, respect quotas |
| Extraction quality insufficient | Medium | High | Iterative prompt refinement, human feedback loop, confidence thresholds |
| Local model too slow/poor | Medium | Medium | Can fall back to cloud-only; M4 chip should be adequate |
| Scope creep | High | Medium | Strict MVP definition, resist adding sources until Gmail works well |
| Cost overruns on Gemini API | Low | Low | Local tier-1 filtering, monitor usage, set budget alerts |
| OAuth token management | Medium | Low | Use established library (google-auth), handle refresh properly |

---

## 8. Open Questions

| ID | Question | Status |
|----|----------|--------|
| OQ-001 | What meeting transcript tool is used? What format are outputs? | Pending |
| OQ-002 | Should assertions support custom types beyond the defined set? | Pending |
| OQ-003 | How should cross-project assertions be handled? | Pending |
| OQ-004 | What retention policy for raw content? | Pending - suggest 1 year |
| OQ-005 | Should there be an MCP server for Claude Desktop integration? | Deferred to post-MVP |

---

## 9. Glossary

| Term | Definition |
|------|------------|
| Assertion | A discrete piece of extracted meaning from a source (decision, risk, commitment, etc.) |
| Source | A raw piece of content from an external system (email, Slack message, document, meeting transcript) |
| Deep Analysis | Processing mode where full assertion extraction is performed on all content |
| News Feed | Processing mode where content is summarised and anomalies detected, but not fully extracted |
| Opt-in | Processing mode where content is only processed when explicitly flagged by user |
| Entity Resolution | The process of mapping name references to canonical person records |
| Project Space | A first-class entity aggregating all context, artifacts, people, and history for an initiative |
| Theme | A high-level category that may contain multiple related projects |
| Salience | The importance level of an assertion (how critical is this to remember) |
| Institutional Memory | The accumulated knowledge and context about an organisation's decisions, history, and operations |

---

## 10. Appendices

### Appendix A: CLI Command Reference (Planned)

```bash
# Ingestion
cxp ingest                      # Run full ingestion
cxp ingest --source gmail       # Run for specific source
cxp ingest --since yesterday    # Override time range

# Review
cxp review --daily              # Morning review workflow
cxp review --weekly             # Weekly project review

# Queries
cxp ask "what happened on Atlas this week"
cxp ask "who is leading the SOC2 project"
cxp ask "find the SOW document for Atlas"

# Projects
cxp project list                # List all projects
cxp project show atlas          # Show project details
cxp project create              # Interactive project creation
cxp project add-artifact atlas  # Add artifact to project

# People
cxp person show rishi           # Lookup person by name/alias
cxp person create               # Create new person record
cxp person alias rishi "Hrishikesh Sharma"  # Add alias

# Timeline
cxp timeline atlas              # Show project timeline
cxp timeline atlas --since 2024-01-01
cxp timeline --type decision    # All decisions across projects

# Configuration
cxp config sources              # List configured sources
cxp config set-mode slack:engineering deep
cxp config set-mode gmail opt-in
```

### Appendix B: Assertion Type Definitions

| Type | Description | Has Status | Has Owner |
|------|-------------|------------|-----------|
| decision | A choice that was made | No | No (but has actors) |
| risk | Something that might go wrong | Yes (open/mitigated/realised) | Optional |
| concern | Someone expressing worry or disagreement | No | No (but has actors) |
| commitment | Someone saying they'll do something, or a deadline set | Yes (open/done/missed) | Yes |
| milestone | Something was completed or achieved | No | No |
| position | Someone's stance or opinion on a topic | No | No (but has actors) |
| context | Background information that explains something | No | No |
| outcome | The result of something (success, failure, change) | No | No |

### Appendix C: Source Mode Comparison

| Aspect | Deep Analysis | News Feed | Opt-in |
|--------|---------------|-----------|--------|
| Content processed | All | All | Flagged only |
| Assertion extraction | Full | None | Full |
| Summarisation | No | Yes | No |
| Anomaly detection | No | Yes | No |
| Storage | Full content + assertions | Summary only | Full content + assertions |
| Use case | Core project channels | High-volume low-priority channels | Noisy sources with occasional signal |
