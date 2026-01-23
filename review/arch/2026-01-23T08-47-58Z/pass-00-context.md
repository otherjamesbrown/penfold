# Architecture Review: Context & Goals

**Review Date**: 2026-01-23
**Reviewer**: Architecture Review Agent
**Purpose**: Establish the lens through which all subsequent architecture passes will evaluate this system

---

## System Mission

### Core Purpose

Penfold is an **AI-powered personal information system** that transforms scattered business communications into actionable intelligence through **contextual archaeology**. The system aggregates and correlates information from communication channels (email, Slack, documents, meetings) into a queryable institutional memory.

**The fundamental problem being solved**: Executives and knowledge workers spend excessive time piecing together context from fragmented communications. A typical sales escalation scenario currently requires 3 hours of context assembly; Penfold aims to reduce this to 15 minutes with a complete audit trail.

### Target User & Pain Points

**Primary User**: Executive/knowledge worker managing complex business communications

**Pain Points Addressed**:
1. **Context Assembly Overhead**: Hours spent reconstructing decision timelines from scattered emails, meetings, and documents
2. **Entity Fragmentation**: Same person appears as multiple identities (`jabrown@akamai.com`, `James Brown`, `@jabrown`) with no unified view
3. **Information Silos**: Related discussions in email, Slack, meetings, and documents are disconnected
4. **Decision Archaeology**: Understanding "how we got here" for escalations, project status, or relationship history requires manual correlation
5. **ADHD-Unfriendly Workflows**: Existing tools don't support rapid context switching or structured temporal browsing

### Success Metrics

The constitution and specifications define explicit success metrics:

| Metric | Target | Source |
|--------|--------|--------|
| Context Assembly Time | < 15 minutes for complete escalation briefing | Constitution |
| Search Success Rate | > 90% queries return relevant results in top 5 | Constitution |
| Search Response Time | < 15 seconds (< 3 seconds for typical queries) | Architecture |
| Source Truth | 100% of insights traceable to original content | Constitution |
| Local Processing Success | 80% of tasks completed without cloud escalation | Constitution |
| Entity Resolution Accuracy | > 95% for internal participants | Content Enrichment Spec |
| Daily Review Completion | < 30 minutes for 50-100 items | Daily Review Spec |
| AI Accuracy Improvement | 20% over 30-day period | Daily Review Spec |

---

## Design Principles (from Constitution)

### Prioritized Principles

The constitution explicitly orders trade-off resolution priorities:

1. **User Value** - Does this solve a real problem faster/better?
2. **Source Truth** - Can user always trace back to original evidence?
3. **Learning Opportunity** - Does this advance AI experimentation goals?
4. **Implementation Simplicity** - Simplest solution that meets requirements
5. **Future Flexibility** - Maintains options for future enhancement

### Core Principles Hierarchy

**Value Creation Principles**:
- **Immediate Value First**: Every feature must demonstrate concrete time savings
- **Contextual Archaeology Over Prediction**: Focus on "how we got here" not forecasting
- **Human Agency Enhancement**: AI suggests and enhances, never replaces; always provide path back to source

**Technical Architecture Principles**:
- **Immutable Content, Evolving Understanding**: Raw content never changes; analysis is versioned
- **Local-First, Cloud-Strategic**: Process locally by default; cloud only when local fails
- **Evidence-Based Relationships**: All relationships have traceable evidence
- **Multi-Modal Intelligence**: Combine multiple AI models, benchmark approaches

**User Experience Principles**:
- **ADHD-Friendly Design**: Support focus shifts, structured temporal browsing
- **Transparent AI Decision-Making**: Confidence scores, reasoning, traceability
- **Progressive Automation**: Start 100% human review, gradually increase automation

### Trade-off Resolution Order

When making architectural decisions, the constitution mandates this priority order:

1. User Value
2. Source Truth
3. Learning Opportunity
4. Implementation Simplicity
5. Future Flexibility

### Explicit Constraints

**Local vs Cloud Decision Matrix**:
- **Choose Local**: Privacy-sensitive, learning value high, acceptable time (hours OK for meetings), model benchmarking needed
- **Choose Cloud**: Local models insufficient, complex synthesis required, user requests immediate results, local consistently fails

**Manual vs Automated Decision Matrix**:
- **Choose Manual**: High accuracy required, training data collection, user expertise essential, error cost > time cost
- **Choose Automated**: User validated pattern exists, confidence > 90%, error recovery straightforward, time savings > accuracy loss

---

## Validation Framework

### Feature Acceptance Criteria

Every feature must pass ALL of these tests (from constitution):

**Value Validation**:
- [ ] Time Savings: Measurably reduces time for specific workflow
- [ ] Pain Relief: Addresses documented frustration
- [ ] Frequency: Solves problem that occurs weekly or more often
- [ ] Criticality: Failure would impact business decisions

**Principle Alignment**:
- [ ] Source Truth: Maintains audit trail back to original content
- [ ] Local-First: Processes locally unless cloud essential
- [ ] User Control: User can override/validate/correct AI decisions
- [ ] Evidence-Based: Relationships backed by concrete evidence

**ADHD-Friendly UX**:
- [ ] Context Switching: Supports rapid focus shifts between overview and detail
- [ ] Cognitive Load: Reduces mental processing burden
- [ ] Structured Browsing: Provides organized navigation
- [ ] Clear Hierarchy: Important information visually prioritized

### Architecture Decision Criteria

**Technical Robustness**:
- [ ] Scalability: Handles 200 emails + 15 meetings/week
- [ ] Performance: < 15 seconds for search, < 3 seconds typical
- [ ] Reliability: Graceful degradation when components fail
- [ ] Maintainability: Complexity manageable for single developer

**Learning Laboratory Criteria**:
- [ ] Experimentation: Enables AI model comparison and benchmarking
- [ ] Improvement: Content becomes more valuable as capabilities advance
- [ ] Local Development: Supports AI learning without cloud dependencies
- [ ] Real-World Testing: Uses actual business problems as test cases

**Future-Proofing**:
- [ ] Extensibility: New content types and AI capabilities can be added
- [ ] Migration Path: Existing data and analysis can be preserved
- [ ] Integration Ready: Can connect to new data sources without redesign
- [ ] Evolution Support: Analysis can be re-run as models improve

### Red Flags / Rejection Criteria

**Immediate Design Rejection** (from constitution):
- **Blackboxes decisions**: User cannot understand or trace AI reasoning
- **Removes user control**: Automation cannot be overridden
- **Ignores source truth**: Insights not traceable to original content
- **Increases cognitive load**: Makes decision-making harder
- **Cloud-dependent**: Requires cloud for basic functionality
- **Value-negative**: Increases time or effort for workflows

**Warning Signs of Constitutional Drift**:
- Feature complexity creep (adding features that don't solve core problems)
- Technical complexity growth (architecture becoming unmaintainable)
- User workflow disruption (requiring users to change successful patterns)
- AI accuracy stagnation (learning and improvement stops)
- Local processing abandonment (everything escalates to cloud)

---

## Architectural Intent

### Chosen Patterns

**Service Architecture**:
```
CLI (penf) --> gRPC --> API Gateway --> Services (Gmail, Search, Review)
                             |
                             v
                    Temporal Worker --> PostgreSQL + pgvector
                             |
                             v
                    MLX Embeddings (local Apple Silicon)
```

**Deployment Topology** (intentionally split):
- **dev01** (Apple Silicon): Worker, MLX Embeddings, CLI - GPU/Neural Engine for embeddings
- **home-01** (Intel): Gateway, PostgreSQL, Redis, Temporal - Data storage, no GPU needed

**Key Patterns**:
- **Protocol Buffers + gRPC**: Service communication with explicit contracts
- **Temporal Workflows**: Durable execution with retry and error handling
- **CLI + Library Architecture**: Every feature starts as standalone library with CLI exposure
- **Content Pipeline**: Ingest -> Classify -> Enrich -> Extract -> AI Process

### Local vs Cloud Strategy

**Local-First Implementation**:
- **MLX Embeddings Sidecar**: Apple Silicon local embedding generation (port 8081)
- **PostgreSQL + pgvector**: Local vector storage and search
- **Temporal Worker**: Local durable workflow execution
- All content processing runs locally by default

**Cloud Escalation Points** (explicit):
- Complex synthesis across large datasets
- When local models demonstrably insufficient
- User-requested immediate results
- Local processing consistently fails

### AI Processing Approach

**Multi-Stage Pipeline**:
1. **Classification**: Identify content type (meeting invite, Jira notification, regular email)
2. **Entity Resolution**: Map identifiers to canonical records (people, teams, projects)
3. **Type-Specific Enrichment**: Extract metadata per content type
4. **AI Processing**: Generate embeddings, summaries, assertions (configurable per type)

**AI Decision Transparency**:
- Every AI suggestion includes confidence score and reasoning
- User can always trace back to source evidence
- Progressive automation: start manual, increase as trust builds
- Audit trail for A/B testing and quality control

**Learning Loop**:
- Daily Review workflow for human validation
- Corrections feed back into learning system
- Learning rules auto-apply after user-validated patterns
- Target: 60% automatic handling after 60-day learning period

---

## Review Criteria for This System

Based on the constitution and goals, this architecture review will evaluate against:

### Primary Evaluation Criteria

1. **Source Truth Preservation**: Does every component maintain audit trails back to original content? Can any insight be traced to its evidence source?

2. **Local-First Compliance**: Is cloud processing truly optional? Can the system function without internet connectivity for core operations?

3. **User Control Preservation**: At every AI decision point, can the user override, correct, or validate? Is automation level controllable?

4. **Single-Developer Maintainability**: Is the architecture complexity manageable for one developer? Are there unnecessary abstractions?

5. **ADHD-Friendly Data Access**: Does the data model support rapid context switching between overview and detail? Is temporal navigation first-class?

6. **Learning Laboratory Enablement**: Can AI models be swapped, compared, and benchmarked? Does the architecture support experimentation?

7. **Evidence-Based Relationships**: Are all entity relationships traceable to specific content? Do relationships include confidence and source?

8. **Progressive Automation Support**: Does the architecture support gradual transition from 100% human review to automated processing?

### Secondary Evaluation Criteria

9. **Performance Budget Compliance**: Are response time targets (< 15s search, < 3s typical) achievable with current architecture?

10. **Graceful Degradation**: When components fail (embedding service, cloud, network), does the system degrade gracefully?

11. **Future Source Integration**: Can new content sources (Slack, calendar, documents) be added without architectural changes?

12. **Analysis Evolution**: Can historical content be re-processed as AI capabilities improve?

### Anti-Pattern Detection

The review will specifically flag:
- Any component that blackboxes AI decisions
- Any required cloud dependency for basic operation
- Any feature that increases cognitive load without proportional value
- Any relationship without traceable evidence
- Any architectural complexity not justified by clear user value

---

## Notes for Subsequent Review Passes

This context document establishes the "lens" for all subsequent architecture review passes:

- **Pass 01 (Structure)**: Evaluate package organization against single-developer maintainability and CLI+Library architecture principle
- **Pass 02 (Data Flow)**: Trace source truth preservation through the pipeline from ingest to search results
- **Pass 03 (AI Integration)**: Assess learning laboratory enablement and progressive automation support
- **Pass 04 (Dependencies)**: Verify local-first compliance and graceful degradation
- **Pass 05 (Performance)**: Validate performance budget achievability
- **Pass 06 (Evolution)**: Assess future source integration and analysis evolution capabilities

Each pass should reference this document's criteria when making judgments about architectural fitness.
