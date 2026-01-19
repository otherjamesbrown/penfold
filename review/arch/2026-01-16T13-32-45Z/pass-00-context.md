# Architecture Review: Context & Goals

**Review Date**: 2026-01-16
**Reviewer**: Architecture Review Pass 0 - Context Extraction
**Constitution Version**: 1.1.0

---

## System Mission

### Core Purpose

Penfold is an AI-powered personal information system that performs **contextual archaeology** - transforming scattered business communications (emails, Slack, documents, meetings) into actionable intelligence. The system reconstructs decision timelines and provides complete situational awareness for executive decision-making.

**Mission Statement**: "Penfold transforms scattered business communications into actionable intelligence through contextual archaeology, enabling executive decision-making with complete situational awareness."

### Target User & Pain Points

**Primary User**: Executive/knowledge worker managing complex business communications across multiple channels.

**Core Pain Points Addressed**:
1. **Fragmented context**: Information scattered across email, meetings, documents, and chat systems
2. **Time-consuming reconstruction**: Currently takes 3+ hours to piece together escalation context
3. **Decision paralysis**: Lacking complete picture when urgent decisions are needed
4. **ADHD-related challenges**: Difficulty switching context and maintaining focus across information sources
5. **Lost institutional memory**: Critical decisions and their rationale buried in communication history

**Primary Use Case**: Sales escalation context assembly - understanding "how we got here" for customer situations requiring executive attention.

### Success Metrics

**Primary Value Metric**:
- **Target**: Transform "3 hours piecing together escalation context" into "15 minutes fully briefed with complete audit trail"

**Constitutional Success Metrics**:

| Category | Metric | Target |
|----------|--------|--------|
| Context Assembly | Time to complete escalation briefing | <15 minutes |
| Search Accuracy | Relevant results in top 5 | >90% |
| Source Truth | Insights traceable to original content | 100% |
| Local Processing | Tasks completed without cloud escalation | 80% |
| Daily Usage | System used consistently | 5+ days/week |
| Cognitive Load | User-reported mental effort reduction | Measurable decrease |

**Warning Metrics (Constitutional Violations)**:
- Time to complete workflows increases
- Users bypass system for important decisions
- Users cannot trace insights back to sources
- AI decisions cannot be overridden
- Local processing consistently fails or is bypassed

---

## Design Principles (from Constitution)

### Prioritized Principles

The constitution establishes a clear hierarchy for resolving design trade-offs:

1. **User Value** - Does this solve a real problem faster/better?
2. **Source Truth** - Can user always trace back to original evidence?
3. **Learning Opportunity** - Does this advance AI experimentation goals?
4. **Implementation Simplicity** - Simplest solution that meets requirements
5. **Future Flexibility** - Maintains options for future enhancement

### Core Technical Principles

#### 1. Immutable Content, Evolving Understanding
- Raw content (emails, meetings, documents) NEVER changes once stored
- Analysis results are versioned and can evolve as AI capabilities improve
- Past content becomes more valuable over time as processing improves

#### 2. Local-First, Cloud-Strategic
- Process everything locally by default for privacy and learning
- Escalate to cloud ONLY for complex synthesis or when local processing fails
- User controls what data leaves local environment

#### 3. Evidence-Based Relationships
- All relationships between information must have traceable evidence
- Relationship strength correlates with validation evidence
- Business domain knowledge accumulates through human-guided learning

#### 4. Human Agency Enhancement
- AI suggests and enhances human decision-making, never replaces it
- Always provide path back to original source material
- User maintains complete control over categorization and relationship validation

#### 5. Progressive Automation
- Start with 100% human review, gradually increase automation as trust builds
- User controls automation levels per context and content type
- Always provide manual override for AI decisions

### Trade-off Resolution Order

When facing design choices:

1. **User Value** - Measurably reduces time or improves decisions
2. **Source Truth** - Maintains audit trail to original content
3. **Learning Opportunity** - Advances AI experimentation
4. **Implementation Simplicity** - Simplest working solution
5. **Future Flexibility** - Preserves enhancement options

### Explicit Constraints

**Data Volume Assumptions**:
- ~200 emails per week
- ~15 meetings per week
- Up to 100,000 content items total

**Performance Targets**:
- <100ms for CRUD operations
- <500ms for vector similarity search
- <15 seconds for any search query
- <60 seconds for new email detection

**Resource Constraints**:
- Single-developer maintainability
- Single-machine deployment (Mac Mini M4, 32GB RAM)
- Offline operation capability for local content

---

## Validation Framework

### Feature Acceptance Criteria

Every proposed feature must pass ALL of these tests:

#### Value Validation
- [ ] **Time Savings**: Measurably reduces time for specific user workflow
- [ ] **Pain Relief**: Addresses documented frustration in current process
- [ ] **Frequency**: Solves problem that occurs weekly or more often
- [ ] **Criticality**: Failure would impact business decisions

#### Principle Alignment
- [ ] **Source Truth**: Maintains audit trail back to original content
- [ ] **Local-First**: Processes locally unless cloud processing essential
- [ ] **User Control**: User can override, validate, or correct AI decisions
- [ ] **Evidence-Based**: Relationships and insights backed by concrete evidence

#### ADHD-Friendly UX
- [ ] **Context Switching**: Supports rapid focus shifts between overview and detail
- [ ] **Cognitive Load**: Reduces rather than increases mental processing burden
- [ ] **Structured Browsing**: Provides organized navigation through information
- [ ] **Clear Hierarchy**: Important information visually prioritized

### Architecture Decision Criteria

#### Technical Robustness
- [ ] **Scalability**: Handles projected data volume (200 emails + 15 meetings/week)
- [ ] **Performance**: Meets response time targets (<15 seconds for search)
- [ ] **Reliability**: Graceful degradation when components fail
- [ ] **Maintainability**: Code complexity manageable for single developer

#### Learning Laboratory Criteria
- [ ] **Experimentation**: Enables AI model comparison and benchmarking
- [ ] **Improvement**: Content becomes more valuable as capabilities advance
- [ ] **Local Development**: Supports AI learning without cloud dependencies
- [ ] **Real-World Testing**: Uses actual business problems as test cases

### Red Flags / Rejection Criteria

**Immediate Design Rejection** if the design:
- **Blackboxes decisions**: User cannot understand or trace AI reasoning
- **Removes user control**: Automation cannot be overridden or validated
- **Ignores source truth**: Insights not traceable to original content
- **Increases cognitive load**: Makes decision-making harder, not easier
- **Cloud-dependent**: Requires cloud processing for basic functionality
- **Value-negative**: Increases time or effort for user workflows

**Warning Signs of Constitutional Drift**:
- Feature complexity creep: Adding features that don't solve core problems
- Technical complexity growth: Architecture becoming unmaintainable
- User workflow disruption: System requires users to change successful patterns
- AI accuracy stagnation: Learning and improvement stops occurring
- Local processing abandonment: Everything escalates to cloud

---

## Architectural Intent

### Chosen Patterns

Based on the ARCHITECTURE.md and specifications, the system employs:

#### 1. Phased Pipeline Processing
- Multi-phase processing with dependency management
- Database entities for tracking processing jobs
- Idempotency keys for reliability
- Progress tracking with estimated completion times

#### 2. Multi-Modal AI Processing
- Tiered AI with confidence scoring
- Local-first with cloud escalation
- Manual review queues for low-confidence results
- Ensemble processing for model comparison

#### 3. Entity Resolution with Provisional States
- AI-suggested entities with human-in-the-loop validation
- Confidence-based auto-acceptance thresholds
- Feedback loops for improving AI accuracy

#### 4. Version Control for AI Content
- Complete audit trail for AI-generated content
- Diff calculation between versions
- Rollback capabilities to any previous version

#### 5. Semantic Search Integration
- pgvector for semantic similarity search
- Combined keyword and semantic search
- Metadata filtering for temporal/contextual queries

### Local vs Cloud Strategy

**Choose Local When**:
- Privacy sensitive content
- Learning/experimentation value high
- Acceptable processing time (hours for meetings OK)
- Model comparison/benchmarking needed

**Choose Cloud When**:
- Local models demonstrably insufficient
- Complex synthesis across large datasets required
- User requests immediate results
- Local processing consistently fails

**Target**: 80% of tasks completed without cloud escalation

### AI Processing Approach

**Multi-Model Strategy**:
- Combine multiple AI models to maximize learning and capability
- Benchmark and compare approaches to build expertise
- Use ensemble methods to improve accuracy and reliability

**Confidence-Based Workflow**:
- All AI decisions include confidence scores
- Low-confidence results queue for human review
- High-confidence (90%+) can be auto-accepted per user settings
- Feedback improves future accuracy

**Technology Stack**:
- Local models via Ollama
- Cloud APIs (Gemini) for escalation
- PostgreSQL + pgvector for hybrid storage
- Redis for event processing

---

## Review Criteria for This System

Based on the constitution and goals, this architecture review will evaluate against:

### 1. Contextual Archaeology Capability
Does the architecture support reconstructing decision timelines and understanding "how we got here"? Are temporal queries, relationship discovery, and source linking first-class concerns?

### 2. Sub-15-Minute Context Assembly
Can the system actually deliver the promised <15 minute escalation briefing? Are there architectural bottlenecks that would prevent this?

### 3. Source Truth Preservation
Is there 100% traceability from any insight to original content? Are original documents immutable with versioned analysis overlays?

### 4. Local-First Processing
Can 80%+ of tasks complete without cloud escalation? Is local processing the default path, with cloud as explicit fallback?

### 5. Human Agency & Override
Can users always override AI decisions? Is transparency built into AI reasoning? Are manual correction workflows first-class?

### 6. Single-Developer Maintainability
Is the complexity manageable for one person? Are patterns consistent and documented? Is cognitive load on the developer reasonable?

### 7. ADHD-Friendly Design
Does the architecture support rapid context switching? Is information hierarchical with clear drill-down paths? Is cognitive load minimized in the user-facing layers?

### 8. Progressive Automation
Does the architecture support starting with 100% human review and gradually automating? Are trust levels and confidence thresholds configurable per user/context?

### 9. Evidence-Based Relationships
Are all discovered relationships backed by traceable evidence? Is relationship confidence tied to validation evidence?

### 10. Learning Laboratory Support
Does the architecture enable AI experimentation and benchmarking? Can historical content be reprocessed as models improve? Are comparisons between approaches built-in?

---

## Summary

Penfold is a highly constrained system with clear priorities:

1. **Primary Goal**: Turn 3-hour context assembly into 15 minutes
2. **Core Capability**: Contextual archaeology - understanding decision timelines
3. **Primary User**: ADHD-friendly executive/knowledge worker
4. **Key Constraint**: Single developer, single machine, local-first
5. **AI Strategy**: Local models by default, cloud escalation when needed, human always in control

The architecture review should evaluate whether current implementation serves these goals, or whether complexity has crept in that undermines the core value proposition.

**Key Question**: Does every architectural component directly serve getting a user from "what happened with this customer?" to "complete briefing with audit trail" in under 15 minutes?
