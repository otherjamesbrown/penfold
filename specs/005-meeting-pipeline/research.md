# Meeting Pipeline Research & Technical Decisions

**Created**: 2026-01-14
**Phase**: 0 - Research & Technical Validation

## 1. File Storage Architecture

### Decision: Hybrid Local-First with Cloud Migration

**Rationale**: Supports local-first philosophy while enabling scalability for large files and long-term storage.

**Implementation Strategy**:
- **Local Storage**: Files <500MB with high privacy levels (confidential, team-only)
- **Cloud Migration**: Files >500MB with infrequent access after 30+ days
- **Security**: AES-256 encryption at rest, user/team privacy level inheritance
- **Upload**: TUS protocol for resumable 2GB+ uploads with 10MB chunks

**Technology Stack**:
- **Local**: Direct filesystem with organized directory structure
- **Cloud**: AWS S3 or Google Cloud Storage with intelligent tiering
- **Upload**: `tuspy` client with custom chunk management
- **Encryption**: Python `cryptography` library with per-privacy-level keys

**Alternatives Considered**:
- Cloud-only storage (rejected: conflicts with local-first principle)
- Local-only storage (rejected: insufficient for 2GB+ files at scale)
- Database blob storage (rejected: poor performance for large files)

## 2. Speech-to-Text Service Selection

### Decision: Hybrid WhisperX + Google Cloud Speech-to-Text

**Rationale**: WhisperX provides excellent local accuracy (95%+) with privacy protection, Google Cloud provides quality escalation for challenging audio.

**Implementation Strategy**:
- **Primary**: WhisperX Large-v3 with Pyannote diarization for local processing
- **Fallback**: Google Cloud Speech-to-Text for poor quality audio (confidence <80%, SNR <15dB)
- **Cost**: $0 for 80% of meetings, ~$0.48-1.92 for cloud escalation
- **Performance**: 2-hour meeting processed in 1-2 minutes locally

**Technology Stack**:
- **Local**: WhisperX with faster-whisper backend, Pyannote 3.1 for speaker identification
- **Cloud**: Google Cloud Speech-to-Text with Chirp 3 for enhanced accuracy
- **Hardware**: Optimal for Mac M4 with 32GB unified memory
- **Quality**: Confidence scoring and audio SNR analysis for escalation decisions

**Alternatives Considered**:
- Local-only Whisper (rejected: insufficient for poor audio quality)
- Cloud-only STT (rejected: conflicts with privacy and local-first principles)
- Azure/AWS STT (rejected: lower accuracy and higher costs than Google)

## 3. Background Processing Architecture

### Decision: Procrastinate (PostgreSQL-native job queue)

**Rationale**: Eliminates broker complexity, leverages existing PostgreSQL infrastructure, provides ACID compliance for job processing.

**Implementation Strategy**:
- **Job Queue**: Procrastinate with PostgreSQL backend for job persistence
- **Progress Tracking**: Server-Sent Events with PostgreSQL LISTEN/NOTIFY
- **Concurrency**: 20+ concurrent uploads, 4 concurrent CPU-intensive (transcription) jobs
- **Error Handling**: Exponential backoff with dead letter queue for persistent failures

**Technology Stack**:
- **Queue**: Procrastinate with asyncio support
- **Events**: PostgreSQL LISTEN/NOTIFY for real-time job updates
- **Monitoring**: psutil for resource monitoring and file system cleanup
- **Retry Logic**: Tenacity library for exponential backoff patterns

**Alternatives Considered**:
- Celery + Redis (rejected: additional infrastructure complexity)
- Dramatiq + Redis (rejected: higher performance but additional Redis dependency)
- Direct asyncio queuing (rejected: no persistence across restarts)

## 4. Audio Processing Pipeline

### Decision: Integrated WhisperX + Pyannote Pipeline

**Rationale**: Single integrated pipeline provides transcription, speaker identification, and word-level timestamps with high accuracy.

**Processing Flow**:
1. **Audio Quality Assessment**: SNR analysis to determine local vs cloud processing
2. **Pre-processing**: Audio normalization and noise reduction if needed
3. **Transcription**: WhisperX with speaker diarization via Pyannote
4. **Quality Validation**: Confidence scoring and accuracy thresholds
5. **Cloud Escalation**: Google Cloud STT for failed quality checks
6. **Post-processing**: Entity resolution and meeting metadata extraction

**Performance Targets**:
- **Accuracy**: 95%+ for clear audio, 90%+ speaker identification
- **Processing Time**: 1-2 minutes for 2-hour meeting on Mac M4
- **Concurrency**: 4 simultaneous transcription jobs without degradation
- **Memory Usage**: <8GB per transcription job

## 5. Privacy and Security Controls

### Decision: Multi-layer Security with User-Specified Privacy Levels

**Rationale**: Supports confidential meeting content while enabling team collaboration and organizational knowledge sharing.

**Security Architecture**:
- **Privacy Levels**: public, organization, team_only, confidential
- **Access Controls**: Role-based permissions with meeting participant validation
- **Encryption**: AES-256 with separate keys per privacy level
- **Audit Trail**: Complete access logging for compliance

**Implementation**:
- **User Control**: Privacy level specified during upload with team defaults
- **Inheritance**: Team and organizational privacy settings automatically applied
- **Encryption**: Client-side encryption before storage for confidential content
- **Key Management**: Separate encryption keys managed per privacy level

## 6. Database Integration Strategy

### Decision: PostgreSQL + pgvector Extension

**Rationale**: Leverages existing database infrastructure, provides semantic search capabilities, integrates well with job queue and event system.

**Schema Design**:
- **meeting_files**: File metadata, storage location, privacy controls
- **meeting_transcripts**: Full transcript text with pgvector embeddings
- **meeting_participants**: Speaker identification with person entity linking
- **processing_jobs**: Background job tracking with FIFO queue management
- **review_queue**: Manual review tasks for failed automatic processing

**Search Integration**:
- **Full-text**: PostgreSQL built-in search for keyword queries
- **Semantic**: pgvector embeddings for conceptual meeting search
- **Hybrid**: Combined approach for comprehensive search capability

## Implementation Priorities

### Phase 1: Core Infrastructure (Weeks 1-2)
1. **File Upload**: TUS protocol implementation for resumable uploads
2. **Job Queue**: Procrastinate integration with PostgreSQL
3. **Basic Transcription**: WhisperX local processing pipeline
4. **Database Schema**: Core meeting entities and job tracking

### Phase 2: AI Processing (Weeks 3-4)
1. **Quality Assessment**: Audio analysis for local vs cloud decision
2. **Speaker Diarization**: Pyannote integration for participant identification
3. **Cloud Escalation**: Google Cloud STT fallback implementation
4. **Progress Tracking**: Real-time job status updates

### Phase 3: Search & Integration (Weeks 5-6)
1. **Semantic Search**: pgvector embedding generation and search
2. **Entity Resolution**: Person and project linking with review queue
3. **Privacy Controls**: User-specified privacy levels and access controls
4. **Manual Corrections**: User interface for transcript editing

### Phase 4: Production Features (Weeks 7-8)
1. **Batch Processing**: FIFO queue with concurrency limits
2. **Performance Monitoring**: Resource usage tracking and optimization
3. **Error Recovery**: Comprehensive retry logic and dead letter queue
4. **Audit Logging**: Complete access and processing trail

This research provides the foundation for implementing a robust meeting processing pipeline that balances local-first processing, high accuracy, and production scalability while maintaining strong privacy controls.