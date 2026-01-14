# Penfold Architecture Patterns

**Extracted from implementations**: 005-meeting-pipeline
**Last Updated**: 2026-01-14

## Core Architectural Patterns

### 1. Phased Pipeline Processing

**Pattern**: Multi-phase processing pipeline with dependency management and status tracking

**Implementation Example** (Meeting Pipeline):
- Phase 1: Core Infrastructure & Data Model
- Phase 2: File Upload & Storage System
- Phase 3: Audio/Video Processing & Transcription
- Phase 4: Content Analysis & AI Processing
- Phase 5: Search and Discovery Implementation
- Phase 6: Manual Review and Correction Interface

**Key Components**:
- Database entities for tracking processing jobs with status and progress
- Idempotency keys for job reliability
- Error handling with detailed error capture
- Progress tracking with estimated completion times

```python
# Processing Job Model Pattern
class ProcessingJob(Base):
    id: uuid.UUID = primary_key
    meeting_file_id: uuid.UUID = foreign_key
    job_type: str  # transcription, analysis, etc.
    status: str    # pending, in_progress, completed, failed
    progress: int  # 0-100 percentage
    stage: str     # current processing stage
    estimated_completion_time: datetime
    error_details: dict = JSONB
    idempotency_key: str
```

### 2. Multi-Modal AI Processing

**Pattern**: Tiered AI processing with confidence scoring and manual fallbacks

**Implementation Details**:
- Local-first approach with cloud escalation
- Confidence scoring for all AI decisions
- Manual review queues for low-confidence results
- Version control for AI-generated content

**Key Components**:
- Confidence thresholds with user warnings
- Manual correction interfaces
- AI feedback collection for continuous improvement
- Ensemble processing for model comparison

```python
# AI Confidence Pattern
class MeetingTranscript(Base):
    confidence_score: float
    transcription_method: str  # whisper_local, cloud_api, manual
    quality_metrics: dict = JSONB
    speaker_segments: dict = JSONB

# Manual Review Queue Pattern
class ReviewQueue(Base):
    review_type: str           # transcription_quality, speaker_id, entity_resolution
    ai_suggestions: dict = JSONB
    user_feedback: dict = JSONB
    assigned_to: uuid.UUID
```

### 3. Entity Resolution with Provisional States

**Pattern**: AI-suggested entities with human-in-the-loop validation

**Implementation Details**:
- Provisional entities created by AI
- Manual resolution workflows
- Confidence-based auto-acceptance thresholds
- Feedback loops for improving AI accuracy

```python
# Provisional Entity Pattern
class MeetingParticipant(Base):
    person_id: uuid.UUID = foreign_key
    speaker_label: str
    voice_signature_hash: str
    identification_confidence: float
    is_provisional: bool  # True until manually confirmed

# Entity Resolution Pattern
class EntityResolution(Base):
    entity_type: str  # person, topic, project
    ai_suggestion: dict = JSONB
    manual_override: dict = JSONB
    resolution_confidence: float
```

### 4. Version Control for AI Content

**Pattern**: Complete audit trail for all AI-generated content with rollback capabilities

**Implementation Details**:
- Version tracking for all edits
- Diff calculation between versions
- Rollback capabilities to any previous version
- Change attribution and reasoning

```python
# Version Control Pattern
class TranscriptVersion(Base):
    meeting_file_id: uuid.UUID
    version_number: int
    transcript_text: str
    change_type: str  # manual_edit, batch_edit, speaker_assignment
    change_summary: str
    changed_by: str
    diff_from_previous: dict = JSONB
    metadata: dict = JSONB
```

### 5. Semantic Search Integration

**Pattern**: Vector embeddings with metadata filtering for precise content discovery

**Implementation Details**:
- pgvector for semantic similarity search
- Combined keyword and semantic search
- Metadata filtering for temporal and contextual queries
- Real-time search index updates

```python
# Search Integration Pattern
class MeetingTranscript(Base):
    transcript_text: str
    embedding: Vector  # pgvector type

class MeetingSummary(Base):
    summary_text: str
    summary_embedding: Vector
    key_points: dict = JSONB

# Search Implementation
async def search_meetings(
    query: str,
    date_range: Optional[DateRange] = None,
    participants: Optional[List[str]] = None,
    project_context: Optional[str] = None
) -> List[SearchResult]
```

### 6. Progressive File Processing

**Pattern**: Streaming upload and background processing for large files

**Implementation Details**:
- Resumable uploads for 2GB+ files
- Background job queues with progress tracking
- Storage with encryption and privacy controls
- Real-time processing status updates

```python
# File Processing Pattern
class MeetingFile(Base):
    filename: str
    size_bytes: int
    privacy_level: str
    storage_location: str
    encryption_key_id: str
    upload_completed_at: datetime
    metadata: dict = JSONB
```

### 7. Manual Review Workflows

**Pattern**: UI-driven manual correction interfaces with validation workflows

**Implementation Details**:
- Inline editing capabilities
- Speaker re-identification interfaces
- Entity resolution queues
- User feedback collection
- Quality assurance workflows

```python
# Review Workflow Pattern
class ReviewQueue(Base):
    review_type: str
    status: str  # pending, in_review, completed
    ai_suggestions: dict = JSONB
    user_feedback: dict = JSONB
    assigned_to: uuid.UUID
    created_at: datetime
    resolved_at: datetime
```

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

## Security Patterns

### Privacy Controls
- Granular access control integration
- Encryption at rest for sensitive content
- Audit trails for all data access

### Data Protection
- Original file preservation
- Secure storage with privacy levels
- Access logging and compliance tracking