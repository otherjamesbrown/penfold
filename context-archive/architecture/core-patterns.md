# Core Architecture Patterns

> **Note**: Code examples are from the original Python implementation for reference. Go implementations are in the respective service directories.

## 1. SLM/LLM Content Processing Pipeline

**Pattern**: 6-stage pipeline splitting work between local SLMs (classification, extraction) and remote LLMs (reasoning, synthesis)

**Stages**:
- **Stage 0 (Parse)**: No AI — library-based HTML stripping, header extraction, format detection
- **Stage 1 (Triage)**: SLM classifies content and rates importance. LOW/PERSONAL content stops here (~50-70% filtered)
- **Stage 2 (Extract)**: SLM extracts entities, dates, action items, risks. Split into 2a (NER) and 2b (semantic)
- **Stage 3 (Context)**: Code + DB lookups resolve extracted names to entity IDs (people, products, glossary)
- **Stage 4 (Deep Analysis)**: Remote LLM receives clean text + extracted entities + resolved context + background knowledge. Produces sentiment, strategic insights, risk mapping
- **Stage 5 (Embed)**: SLM generates multi-level embeddings (raw text, summary, action items)

**Key principle**: If the answer is in the text, use the SLM. If it requires reasoning, use the LLM.

**Go Implementation**: `services/worker/workflows/`, `services/worker/activities/`

**Full reference**: [SLM/LLM Pipeline](slm-llm-pipeline.md)

## 2. Multi-Modal AI Processing

**Pattern**: Tiered AI processing with model selection matched to task complexity

**SLM (Local, Stages 1-2)**:
- Classification into fixed category set (7 categories)
- Structured extraction of explicitly stated facts
- Short summarisation, structured JSON output
- Model: Qwen2.5-32B on Apple Silicon (dev01)

**LLM (Remote, Stage 4)**:
- Cross-context reasoning (connecting email to meeting risks)
- Nuanced business sentiment (decoding corporate euphemism)
- Strategic synthesis and insight generation
- Model: Gemini Pro for high-importance, Gemini Flash for standard

**Quality controls**:
- Confidence scoring for entity resolution (Stage 3)
- Quality gates between stages (e.g., RISK_ISSUE triage + zero risks extracted → re-run)
- Manual review queues for low-confidence results
- Langfuse instrumentation for prompt quality tracking

## 3. Entity Resolution with Provisional States

**Pattern**: AI-suggested entities with human-in-the-loop validation

**Implementation Details**:
- Provisional entities created by AI
- Manual resolution workflows
- Confidence-based auto-acceptance thresholds
- Feedback loops for improving AI accuracy

## 4. Version Control for AI Content

**Pattern**: Complete audit trail for all AI-generated content with rollback capabilities

**Implementation Details**:
- Version tracking for all edits
- Diff calculation between versions
- Rollback capabilities to any previous version
- Change attribution and reasoning

## 5. Semantic Search Integration

**Pattern**: Vector embeddings with metadata filtering for precise content discovery

**Implementation Details**:
- pgvector for semantic similarity search
- Combined keyword and semantic search
- Metadata filtering for temporal and contextual queries
- Real-time search index updates

**Go Implementation**: `api/proto/search/v1/`, `cmd/penf/cmd/search.go`

## 6. Progressive File Processing

**Pattern**: Streaming upload and background processing for large files

**Implementation Details**:
- Resumable uploads for 2GB+ files
- Background job queues with progress tracking
- Storage with encryption and privacy controls
- Real-time processing status updates

## 7. Manual Review Workflows

**Pattern**: UI-driven manual correction interfaces with validation workflows

**Implementation Details**:
- Inline editing capabilities
- Speaker re-identification interfaces
- Entity resolution queues
- User feedback collection
- Quality assurance workflows

**Go Implementation**: `api/proto/review/v1/`, `cmd/penf/cmd/review.go`

---

## Integration Patterns

### Event-Driven Architecture
- Publishing processing events to central event framework
- Asynchronous job processing with status updates
- Cross-component communication via events

### Database Extension Pattern
- Extending base schema with feature-specific entities
- Foreign key relationships to core entities (Person, Project)
- JSONB for flexible metadata storage

### AI Orchestration
- Integration with AI coordination framework
- Model selection based on content type and quality requirements
- Confidence scoring and fallback strategies

---

## Performance Patterns

### File Handling
- Streaming uploads for large files (2GB+)
- Background processing with progress tracking
- Storage optimization with encryption

### Search Performance
- Vector similarity search with metadata filtering
- Response time targets: <3 seconds for 1000+ meetings
- Real-time index updates

### Concurrent Processing
- Support for 20+ concurrent uploads
- Job queue management with priority handling
- Resource isolation for different processing types

---

## Security Patterns

### Privacy Controls
- Granular access control integration
- Encryption at rest for sensitive content
- Audit trails for all data access

### Data Protection
- Original file preservation
- Secure storage with privacy levels
- Access logging and compliance tracking
