# Penfold - Product Specification v2.0

## Document Information

| Field | Value |
|-------|-------|
| Version | 2.0.0 |
| Status | Draft |
| Author | James |
| Created | 2025-01-10 |
| Last Updated | 2025-01-10 |
| Previous Version | v1.0 (original Context Palace spec) |

---

## 1. Executive Summary

### 1.1 Vision Statement

Penfold is a **contextual time machine** - a personal AI system that enables you to "rewind time" and understand how any situation evolved. It organizes all your information sources along two axes: **time** (chronological spine) and **projects** (thematic contexts), allowing emergent patterns and relationships to be discovered rather than enforced.

### 1.2 Core Problem

As a COO, you process information well in real-time, but struggle with **retroactive context reconstruction**:

- "I know Bob said something about deployment, but where?"
- "After 3 hours of meetings across 5 projects, what was decided about Y?"
- "Something mentioned casually last week is now critical - what exactly was said?"
- "Who said they'd do X by when, and are they overdue?"

The information exists across email, meetings, Slack, docs - but you **can't find it or reconstruct the chronology** when priorities shift.

### 1.3 Solution Overview

**Phase 1: Contextual Archaeology Foundation**
- Ingest all information sources with timestamps (temporal spine)
- Organize by project contexts (People Management, Atlas Project, General, etc.)
- Import organizational data, AI maps conversational names to official directory
- Enable temporal queries: "Show me everything about Bob around last Tuesday"

**Phase 2: Intelligence Layer**
- Agents surface patterns: missed deadlines, emerging risks, team health
- Automated insights: "3 people consistently missing commitments"
- Weekly summaries per project context

### 1.4 Target User

- **Single user**: COO of 250-person company
- **ADHD-friendly**: Structured browsing when focus shifts from high-level to detail
- **Power user**: Comfortable with CLI, wants source links, not black boxes
- **Review rhythm**: Friday reset + 9am daily triage

---

## 2. Core Mental Model

### 2.1 Dual Organizing Principles

**Time Spine**: Everything happens at `[timestamp]`
```
[2024-01-15 10:30] Meeting: Atlas Kickoff
[2024-01-16 14:22] Email from Bob: Re: Atlas deployment concerns
[2024-01-17 09:15] Slack: #atlas channel - Bob resolved the issue
```

**Project Contexts**: All information belongs to thematic containers
- **Atlas Project**: Technical initiative
- **People Management**: 1:1s, performance, hiring
- **Operations Review**: Weekly business reviews
- **General**: Unclassified or company-wide items

### 2.2 Emergent Relationships

AI discovers connections without rigid schemas:
- Bob appears in multiple contexts over time
- "Engineering lead" mentioned in meeting = Alice from email context
- Deployment concerns → resolution → follow-up actions emerge as narrative

### 2.3 Entity Management

**People**: Import org chart (Robert Smith, Lead Architect, Engineering), AI maps "Bob in email" → Robert Smith
**Topics**: Kafka, deployment, SOC2 compliance (AI discovers from content patterns)
**Artifacts**: Spreadsheets, documents mentioned in sources (AI extracts references, links to URLs)

---

## 3. Phase 1: Contextual Archaeology

### 3.1 Core Capabilities

**Temporal Search**
```bash
penfold search "bob deployment" --last-month
penfold timeline --person "Robert Smith" --since "2 weeks ago"
penfold context "kafka decision" --with-sources
```

**Project Context Queries**
```bash
penfold project timeline "Atlas" --last-quarter
penfold context "who said what about deadline" --project "Atlas"
penfold show "People Management" --this-week
```

**Source Reconstruction**
```bash
penfold meeting --with "Bob" --about "deployment" --around "last tuesday"
penfold email --from "alice@company.com" --mentions "priority"
```

### 3.2 Three-Channel Ingestion System

**1. Manual Tagging** (High Control)
- User explicitly assigns projects + context notes
- Use cases: Important meetings, customer calls, critical documents
- Supports multiple project assignment
```bash
penfold ingest manual --source "transcript.txt" --projects "Atlas,SOC2"
```

**2. AI Suggested with Human Review** (Learning Loop)
- AI analyzes content and suggests project categorization
- Daily review queue for confirmation/correction
- Full semantic understanding, learns from feedback
- Progressive automation as accuracy improves
```bash
penfold review daily  # Review AI suggestions
```

**3. Pre-Tagged** (Automation)
- One-time configuration maps sources to projects
- Project-specific channels, labeled folders
```bash
penfold config set-channel "#atlas-project" --projects "Atlas"
```

**Data Sources**
- Email (Gmail API): AI suggested categorization
- Meeting Transcripts: Manual tagging + Zoom API integration
- Slack Channels: Pre-tagged by channel or AI suggested
- Google Docs: Manual linking or AI suggested

### 3.3 Project Context Management

**Multi-Project Support**
- Any information can belong to multiple projects
- Example: "Atlas deployment security" = [Atlas + SOC2 + Operations]
- No forced single categorization

**Default Contexts**
- General (catch-all)
- People Management (1:1s, performance, hiring)
- Operations (weekly reviews, metrics)

**Dynamic Contexts**
- User creates: "Atlas Project", "SOC2 Compliance", "Q1 Planning"
- AI suggests new projects when detecting unknown patterns
- Progressive automation: 100% review → high-confidence auto-assignment

### 3.4 Entity Resolution

**People Resolution**
- Import organizational data: Employee directory, org chart, team structures
- AI maps conversational names to directory: "Bob in email" → "Robert Smith (Lead Architect, Engineering)"
- Handle aliases/nicknames: Bob, Rob, Bobby all map to Robert Smith
- No guessing about roles/teams - use authoritative HR data

**Topic Clustering**
- AI identifies recurring themes: "deployment", "kafka migration", "team capacity"
- No predefined categories - let patterns emerge
- Cross-reference with project contexts

---

## 4. Technical Architecture

### 4.1 Data Model

**Information Entities**
```
id: UUID
timestamp: datetime
source_type: enum (email, meeting, slack, document)
raw_content: text
participants: [detected_person_references]
project_contexts: [project_ids] (can be multiple)
source_url: string (link back to original)
metadata: jsonb (source-specific fields)
```

**People (Imported + Mapped)**
```
canonical_name: string
official_title: string
team: string
aliases: [string] (Bob, Robert, Rob)
org_data: jsonb (imported from HR systems)
mapping_confidence: float (for alias resolution)
```

**Project Contexts**
```
name: string
description: text
created_at: datetime
is_active: boolean
keywords: [string] (for auto-classification)
```

**Discovered Relationships**
```
entity1_type: enum (person, topic, artifact)
entity1_id: UUID
entity2_type: enum (person, topic, artifact)
entity2_id: UUID
relationship_type: string (AI discovered: "mentioned", "decided", "assigned")
confidence: float
source_information_id: UUID
```

### 4.2 Multi-Model Processing Pipeline

**Real-Time Ingestion** (Local Tier 1 Models)
1. Fetch from source APIs (Gmail, meeting platforms, manual uploads)
2. Multiple local models process in parallel:
   - Llama 3.1 8B: Entity extraction + summarization
   - Phi-3 Mini: Quick categorization scoring
   - Qwen2.5 7B: Relationship identification
3. Ensemble scoring for project categorization confidence
4. Store raw content + multi-model analysis results

**Nightly Batch Analysis** (Local Tier 2 Models)
1. Cross-document relationship discovery
2. Timeline gap detection and reconstruction
3. Pattern analysis across historical data
4. Project health metrics generation
5. Emerging topic clustering

**On-Demand Deep Analysis** (Cloud Tier 3 Models)
1. User-initiated complex queries
2. Timeline synthesis for project catchups
3. Cross-project dependency analysis
4. Executive insight generation

**Vector Database Integration**
- Multiple embeddings per document (nomic, BGE, E5, custom)
- Hybrid search: vector similarity + BM25 + temporal filtering
- Local Qdrant instance for privacy

### 4.3 Query Engine

**Semantic Search**
- Vector embeddings for content similarity
- Hybrid search (semantic + keyword)
- Temporal filtering

**Graph Traversal**
- "Find all information connected to Bob around this timeframe"
- "Trace the evolution of this decision across sources"

**Timeline Reconstruction**
- Chronological ordering within project contexts
- Cross-reference between sources for same events

---

## 5. MVP Definition

### 5.1 MVP Scope (Phase 1A)

**Core Goal**: Perfect search + timeline for email + meetings

**In Scope**
- Gmail API integration with incremental sync
- Meeting transcript/summary ingestion (manual upload + Zoom API)
- Three-channel ingestion system (manual, AI-suggested, pre-tagged)
- Import organizational directory (CSV/API integration)
- Multi-model local AI processing (Llama, Phi, Qwen)
- Vector database with multiple embeddings (Qdrant)
- Daily review workflow for AI categorization suggestions
- Progressive automation (100% review → confidence-based auto-assignment)
- CLI for temporal and contextual queries
- Source linking (jump back to original email/transcript/meeting)

**Success Criteria**
- Find any email/meeting mention in <15 seconds
- Reconstruct timeline for any 2-week period
- Manual entity resolution in <5 minutes during weekly review

**Out of Scope**
- Slack integration
- Automated project context detection
- Agent insights/recommendations
- Complex entity relationship modeling

### 5.2 MVP User Stories

**US-001: Find Bob's Email**
```
penfold search "bob deployment concerns" --last-month
→ Returns ranked results with timestamps and source links
→ Click through to original Gmail thread
```

**US-002: Weekly Timeline Reconstruction**
```
penfold timeline --project "Atlas" --last-week
→ Chronological view of all emails/meetings about Atlas
→ Shows participants, key topics, source links
```

**US-003: Entity Resolution Review**
```
penfold review entities
→ Shows unclear name mappings: "Bob in email = Robert Smith in meeting?"
→ Manual confirmation creates canonical mapping
```

### 5.3 Technical Implementation

**Stack**
- **Language**: Python 3.12
- **Database**: PostgreSQL + Qdrant (vector database)
- **Local AI**: Ollama (Llama 3.1 8B, Phi-3, Qwen2.5) + vLLM for performance
- **Cloud AI**: Gemini Pro/Ultra for complex reasoning
- **ML Ops**: Weights & Biases, MLflow for model versioning
- **Pipeline**: Apache Airflow for batch jobs
- **CLI**: Click-based command interface

**Data Flow**
1. **Real-time ingestion**: Gmail API, meeting uploads, manual tagging
2. **Multi-model processing**: Parallel analysis by local models
3. **Daily review**: AI categorization suggestions with confidence scores
4. **Nightly batch**: Cross-document analysis, pattern discovery
5. **On-demand queries**: Hybrid search with cloud model synthesis
6. **Continuous learning**: User feedback improves local model accuracy

---

## 6. Phase 2: Intelligence Layer

### 6.1 Planned Capabilities

**Pattern Recognition**
- Identify consistently missed deadlines by person
- Detect emerging risks mentioned across sources
- Surface cross-project dependencies

**Automated Insights**
- Weekly project health summaries
- "Things you might have missed" based on priority shifts
- Relationship mapping: who influences what decisions

**Proactive Notifications**
- Overdue commitments based on extracted promises
- Similar situations from historical context
- Anomaly detection in communication patterns

### 6.2 Agent Architecture

**Context-Aware Agents**
- Each project context gets specialized agent
- Agents understand project history and key players
- Cross-pollination between project insights

**Query Expansion**
- Natural language to structured queries
- "What's the Atlas status?" → timeline + health + risks + next actions

---

## 7. Success Metrics

### 7.1 Phase 1 Success

| Metric | Target | Measurement |
|--------|--------|-------------|
| Search Speed | <15 seconds to find any specific mention | Time from query to result click |
| Timeline Completeness | 95% of project activities captured | Manual verification against known events |
| Weekly Review Time | <30 minutes for entity resolution | Timed review sessions |
| Daily Usage | 5+ queries per day sustained | CLI usage analytics |

### 7.2 Phase 2 Success

| Metric | Target | Measurement |
|--------|--------|-------------|
| Insight Relevance | 80% of weekly insights acted upon | User feedback on agent suggestions |
| Missed Deadline Detection | 90% catch rate for overdue items | Comparison with manual tracking |
| Cross-Project Pattern Recognition | 3+ useful connections per week | User validation of discovered patterns |

---

## 8. Implementation Notes

### 8.1 Key Design Principles

1. **Source Truth**: Always link back to original documents
2. **Emergent Structure**: Let AI discover patterns, don't force schemas
3. **Temporal First**: Time is the primary organizing axis
4. **Context Containers**: Projects provide thematic grouping
5. **Human in Loop**: AI suggests, human confirms critical entities

### 8.2 ADHD-Friendly Features

- **Structured Browsing**: Timeline views for when focus shifts to detail
- **Context Switching**: Easy project context switching
- **Visual Timelines**: Clear chronological progression
- **Quick Source Access**: One-click to original documents

### 8.3 Privacy and Control

- **Local Processing**: Entity extraction via local LLM where possible
- **Opt-in Sync**: User controls what sources to include
- **Data Ownership**: All data stored locally, cloud APIs only for processing
- **Audit Trail**: Track what AI decisions influenced results

---

## 9. Open Questions

| Question | Priority | Notes |
|----------|----------|-------|
| Meeting transcript source integration? | High | Need to understand current tooling |
| Project context auto-detection accuracy threshold? | Medium | Balance automation vs manual control |
| Cross-project duplicate detection? | Low | Some overlap may be acceptable |
| Archive/retention policy for raw content? | Low | Disk space vs historical value |

---

## 10. Glossary

| Term | Definition |
|------|------------|
| **Temporal Spine** | Chronological organization where every piece of information has a timestamp |
| **Project Context** | Thematic container for organizing information (Atlas, People Management, etc.) |
| **Contextual Archaeology** | The ability to "rewind time" and understand how situations evolved |
| **Entity Resolution** | AI-assisted mapping of name variants to canonical people/topics |
| **Emergent Relationships** | Connections discovered by AI rather than predefined in schema |
| **Source Truth** | Always maintaining link back to original document/meeting/email |