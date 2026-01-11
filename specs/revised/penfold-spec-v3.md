# Penfold - Product Specification v3.0

## Document Information

| Field | Value |
|-------|-------|
| Version | 3.0.0 |
| Status | Final Design |
| Author | James |
| Created | 2025-01-11 |
| Last Updated | 2025-01-11 |
| Previous Version | v2.0 (initial reframe), v1.0 (original Context Palace) |

---

## 1. Executive Summary

### 1.1 Vision Statement

Penfold is a **contextual time machine** and **AI learning laboratory** - a personal system that enables COOs to "rewind time" and understand how any business situation evolved, while building institutional knowledge through human-AI collaboration.

### 1.2 Core Problem

As a COO processing 200 emails and 15 meetings per week, the challenge isn't real-time information processing - it's **retroactive context reconstruction**:

**Sales Escalation Scenario**: Email thread about customer issues, parallel engineering discussions about performance, support tickets about symptoms. All related, but scattered. When decision time comes, piecing together the complete story takes hours and risks missing critical context.

**Key Pain Points**:
- Information silos: Sales, engineering, and support discussing same issue separately
- Context scattering: Related discussions use different terminology
- Timeline fragmentation: Can't reconstruct decision evolution
- Source truth loss: "Who said what when?" becomes impossible to verify

### 1.3 Solution Overview

**Immutable Content + Dynamic Relationships Architecture**:
- Store all raw content permanently for future re-analysis
- Build flexible relationship overlays that evolve with evidence
- Local-first AI processing with strategic cloud escalation
- Human-guided institutional knowledge building

**Core Capabilities**:
1. **Contextual Archaeology**: "Rewind time" to understand situation evolution
2. **Intelligent Correlation**: AI discovers + human validates business relationships
3. **Source Truth Traceability**: Always link back to original content
4. **Asset Versioning**: Re-analyze historical content as AI improves

### 1.4 Target User & Goals

**User**: COO of 250-person company, ADHD-friendly workflows, CLI power user
**Learning Goal**: AI experimentation platform using real business problems
**Success Metric**: Transform "3 hours catching up on escalation" → "15 minutes fully briefed with complete context"

---

## 2. Information Architecture

### 2.1 Immutable Content Layer

**Raw Assets (Never Change)**:
```
Content Storage:
├── emails/
│   ├── uuid-123/ (Gmail thread)
│   │   ├── raw-content.json
│   │   ├── metadata.json (participants, timestamp, thread-id)
│   │   └── attachments/
├── meetings/
│   ├── uuid-456/ (Atlas planning meeting)
│   │   ├── audio-recording.mp4
│   │   ├── video-recording.mp4
│   │   ├── user-notes.md
│   │   ├── ai-summary.txt
│   │   ├── shared-documents/
│   │   └── metadata.json (title, attendees, project contexts)
└── manual-content/
    ├── documents/
    └── links/
```

### 2.2 Dynamic Relationship Overlay

**Flexible Relationship System**:
```yaml
Relationship Types:
  Global: # Permanent business knowledge
    - "Bob" = "Robert Smith" = "Lead Architect"
    - "Infrastructure changes" → "Performance impacts" (95% confidence)
    - "Customer escalations" → "Sales team coordination"

  Project-Scoped: # Context-specific within projects
    Project: "Atlas Deployment"
      - "Tiger Team" = [Bob, Alice, Sarah]
      - "Performance issues" → "Launch timeline impact"
      - "Customer feedback" → "Feature priority adjustments"

  Strength Levels: # Evidence-based validation
    - Strong (90%+): User-validated + data-confirmed
    - Probable (70-89%): Pattern observed + domain logic
    - Tentative (50-69%): AI suggested + timing correlation
    - Experimental (<50%): Hypothesis needing validation
```

### 2.3 Analysis Versioning System

**Progressive AI Enhancement**:
```
Raw Asset → Analysis Versions (Evolving):
├── v1-2025-01-basic/
│   ├── transcript-whisper-v1.txt
│   ├── summary-llama.md
│   └── entities-basic.json
├── v2-2025-03-enhanced/
│   ├── transcript-whisper-v2.txt
│   ├── speaker-identification.json (NEW: Voice analysis)
│   ├── summary-qwen-finetuned.md
│   └── entities-enhanced.json
└── v3-2025-06-multimodal/
    ├── visual-analysis.json (NEW: Slide content)
    ├── sentiment-analysis.json (NEW: Emotional context)
    └── cross-meeting-links.json (NEW: Timeline connections)
```

---

## 3. Multi-Channel Ingestion System

### 3.1 Manual Tagging (High Control)
**Use Cases**: Important meetings, customer calls, critical documents
```bash
penfold ingest manual --source "customer-call.mp4" --projects "Atlas,CustomerSuccess"
> Upload content types: [notes] [summary] [transcript] [docs] [audio] [video]
> AI processing: Entity extraction, project suggestions
> Human review: Confirm categorization, add business context
```

### 3.2 AI Suggested with Human Review (Learning Loop)
**Use Cases**: Email threads, general communications
```bash
penfold review daily
> 12 items need categorization:
> Email: "Deployment timeline concerns" (Bob → You, Alice)
> AI suggests: [Atlas: 87%] [Operations: 34%]
> Reasoning: Technical deployment + Atlas team participants
> [Accept] [Modify] [Add relationship: "deployment concerns" → "timeline pressure"]
```

### 3.3 Pre-Tagged (Automation)
**Use Cases**: Project channels, labeled folders
```bash
penfold config set-channel "#atlas-project" --projects "Atlas"
penfold config set-folder "Gmail:SOC2 Compliance" --projects "SOC2,Operations"
```

---

## 4. Local-First AI Architecture

### 4.1 Processing Tiers

**Tier 1: Local Real-Time (Mac Mini M4)**
- Models: Llama 3.1 8B, Phi-3 Mini 3.8B, Qwen2.5 7B
- Tasks: Entity extraction, quick summarization, categorization scoring
- Approach: Multi-model ensemble for learning and comparison

**Tier 2: Local Batch (Intel NUC Database Server)**
- Database: PostgreSQL + Qdrant vector storage
- Tasks: Nightly relationship discovery, pattern analysis, cross-document correlation
- Approach: Deep analysis when time isn't critical

**Tier 3: Cloud On-Demand (Corporate Gemini)**
- Usage: Complex synthesis, failed local processing, user-initiated queries
- Approach: Local pre-processing to minimize API calls and costs

### 4.2 Re-Analysis Pipeline
```bash
# When new AI capabilities become available
penfold reanalyze bulk --since "2025-01-01" --add-capability "speaker-identification"
> Found 47 meetings for voice analysis enhancement
> Estimated processing: 23 hours
> New analysis version: v2-enhanced
> Previous analysis preserved for comparison
```

---

## 5. Core Use Cases

### 5.1 Sales Escalation Resolution

**Scenario**: Customer escalation about performance, need complete context for decision
```bash
# Discover all related threads
penfold correlate "bitmovin escalation" --expand-search
> Primary: Sales escalation thread (12 emails)
> Related: Engineering "slow boot-up" discussion (8 emails)
> Related: Support performance tickets (5 emails)
>
> AI correlation confidence:
> ├── Sales ↔ Support: 92% (direct customer mention)
> ├── Sales ↔ Engineering: 78% (timing + symptoms match)
>
> Generate executive brief? [y/n]
```

**Result**: Complete story assembly with business context, technical details, and customer impact.

### 5.2 Timeline Forensics

**Scenario**: "What led to the Atlas decision?" - trace decision evolution
```bash
penfold trace "atlas architecture decision" --full-context
> Decision timeline reconstruction:
> 2025-01-08: Bob's email about scalability concerns (source)
> 2025-01-12: Engineering meeting discussion (23:45-26:30) (source)
> 2025-01-15: Customer requirements clarification (source)
> 2025-01-18: Architecture decision finalized (source)
>
> [Play meeting moment] [Show email thread] [Complete context]
```

### 5.3 Institutional Knowledge Building

**Scenario**: Learning business patterns for future prediction
```bash
# AI suggests relationship, human adds context
> AI: "Database migrations" ↔ "API performance issues" (67% confidence)
> You: Confirm as causal: "Migrations cause temporary latency spikes"
> System learns: Future migrations → predict performance impact
> Business application: Proactive customer communication planning
```

---

## 6. Technical Implementation

### 6.1 Hardware Architecture

**Mac Mini M4 (32GB RAM, 2TB SSD)**:
- Role: AI model serving, real-time processing, development
- Models: Multiple local LLMs simultaneously
- Storage: Active working data, recent analysis

**Intel NUC (32GB RAM, NVMe SSD)**:
- Role: Database server, batch processing, monitoring
- Services: PostgreSQL + pgvector, Qdrant, Prometheus
- Processing: Nightly analysis jobs, relationship discovery

**Network Storage (2TB NVMe + 6TB HDD)**:
- Role: Raw asset storage, archives, model artifacts
- Strategy: Hot (NVMe) for active content, cold (HDD) for archives

### 6.2 Software Stack

**Core Framework**:
- Language: Python 3.12
- Database: PostgreSQL + Qdrant vector database
- AI Serving: Ollama + vLLM for performance
- Workflow: Apache Airflow for batch orchestration
- Monitoring: Prometheus + custom metrics
- Interface: Click-based CLI

**AI Models**:
- Local: Llama 3.1 8B, Phi-3 Mini, Qwen2.5 7B, custom fine-tuned variants
- Cloud: Gemini Pro/Ultra via corporate account
- Embeddings: Multiple models (nomic, BGE, E5) for comparison
- Speech: Local Whisper for transcription

---

## 7. MVP Implementation Plan

### 7.1 MVP Scope (Phase 1A - 3-4 months)

**Core Goal**: Prove contextual archaeology value with email + meetings

**In Scope**:
- Multi-channel ingestion system (manual, AI-suggested, pre-tagged, expandable)
- Gmail API integration with incremental sync
- Manual meeting upload workflow with multi-content support
- Local multi-model AI processing with ensemble comparison
- Basic relationship discovery and human validation
- Vector search with source truth linking
- Daily review workflow for AI suggestions
- Progressive automation (start 100% review → confidence-based auto-assignment)

**Success Criteria**:
- Find any email/meeting mention in <15 seconds
- Reconstruct complete timeline for any escalation scenario
- Successfully correlate related discussions across different contexts
- Daily review workflow completes in <30 minutes

### 7.2 Phase 1B Extensions (Months 4-6)

**Enhanced Capabilities**:
- Fine-tuned local models based on user feedback
- Advanced relationship discovery across larger timeframes
- Improved correlation algorithms with confidence calibration
- Meeting content analysis (speaker identification, visual content)
- Cross-project pattern recognition

### 7.3 Phase 2: Intelligence Layer (Months 6-12)

**Advanced AI Features**:
- Predictive insights based on learned patterns
- Automated risk detection across projects
- Weekly executive summaries per project
- Advanced timeline synthesis and gap detection
- Sophisticated relationship network analysis

---

## 8. Key Design Principles

1. **Immutable Content**: Raw assets never change, analysis evolves
2. **Source Truth**: Always traceable back to original content
3. **Local-First**: Process locally, escalate to cloud strategically
4. **Human-Guided Learning**: AI suggests, human validates, system learns
5. **Flexible Relationships**: Dynamic overlays that adapt to business reality
6. **Asset Investment**: Content becomes more valuable as AI improves
7. **Evidence-Based Validation**: Relationship strength based on confirmation
8. **ADHD-Friendly**: Structured browsing supports focus shifts

---

## 9. Success Metrics

### 9.1 Primary Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Context Reconstruction Speed | <15 minutes for complete escalation briefing | Time from query to full context |
| Search Accuracy | >90% relevant results in top 5 | User feedback on search quality |
| Relationship Validation | >85% AI suggestions accepted after review | User acceptance rate |
| Daily Review Efficiency | <30 minutes for daily triage | Time spent on AI suggestion review |
| Source Truth Usage | 100% of insights traceable to source | Audit trail completeness |

### 9.2 Learning and Development Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Model Performance Improvement | +20% accuracy over 6 months | Before/after comparison on same content |
| Local vs Cloud Usage | 80% local processing | Cost and capability analysis |
| Relationship Network Growth | Institutional knowledge accumulation | Number of validated business patterns |
| Re-analysis Value | Demonstrable improvement from enhanced AI | Quality comparison across analysis versions |

---

## 10. Risk Management

### 10.1 Technical Risks

**Local Model Limitations**: Mitigation through cloud escalation and model comparison
**Data Volume Growth**: Graduated storage strategy and efficient indexing
**Hardware Constraints**: Upgrade path planned (Mac Studio if needed)
**Integration Complexity**: Manual-first approach reduces API dependencies

### 10.2 Business Risks

**User Adoption**: Focus on immediate value (escalation briefing) for engagement
**Accuracy Expectations**: Clear confidence scoring and human validation loops
**Time Investment**: Acceptable processing time (1 hour per meeting) for learning value
**Scope Creep**: Strict MVP definition with clear phase gates

This specification represents a mature, implementable system that balances sophisticated AI capabilities with practical business value and learning opportunities.