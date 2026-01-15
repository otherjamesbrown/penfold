# Penfold Architecture Patterns

**Extracted from implementations**: 004-gmail-integration, 005-meeting-pipeline, 010-testing-framework, 011-observability-framework
**Last Updated**: 2026-01-15

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

## AI-First Testing Patterns

### 8. Multi-Tiered AI Mocking Strategy

**Pattern**: Different mocking approaches based on test type and performance requirements

**Implementation Tiers**:
- **Unit Tests**: Full mocking with deterministic responses (<100ms)
- **Integration Tests**: Lightweight models for realistic behavior (<10s)
- **End-to-End Tests**: Record/replay real AI responses (<30s)

**Key Components**:
- Deterministic response patterns based on prompt analysis
- Response caching and replay infrastructure
- Lightweight model substitution for fast testing
- Performance validation for each tier

```python
# AI Mocking Strategy Pattern
class AIModelMock:
    def __init__(self, mode: str = 'deterministic'):
        self.mode = mode  # deterministic, lightweight, recorded
        self.response_library = ResponseLibrary()

    async def process(self, content: str, model: str) -> AIResponse:
        if self.mode == 'deterministic':
            return self._pattern_based_response(content)
        elif self.mode == 'lightweight':
            return await self._fast_model_response(content)
        else:  # recorded
            return self._cached_response(content)

# Mock Response Library Pattern
class MockResponseLibrary:
    def generate_response(self, task_type: str, content: str) -> dict:
        patterns = {
            'summarization': self._summary_patterns,
            'entity_extraction': self._entity_patterns,
            'categorization': self._category_patterns
        }
        return patterns[task_type](content)
```

### 9. Container-Based Environment Isolation

**Pattern**: Isolated test environments using Docker with in-memory storage for performance

**Implementation Details**:
- PostgreSQL with pgvector in tmpfs for fast database operations
- Redis in-memory for event processing
- Mock AI services with response libraries
- Parallel test execution without interference

```python
# Environment Isolation Pattern
@pytest.fixture(scope="session")
async def test_engine():
    """Isolated test database with automatic cleanup"""
    test_engine = create_async_engine(
        config.database.test_database_url,
        echo=False,
        pool_pre_ping=True,
    )

    async with test_engine.begin() as conn:
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))
        await conn.run_sync(Base.metadata.create_all)

    yield test_engine

    async with test_engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)
    await test_engine.dispose()

# Test Session Isolation Pattern
@pytest.fixture
async def test_session(test_engine) -> AsyncGenerator[AsyncSession, None]:
    TestSessionLocal = async_sessionmaker(test_engine, expire_on_commit=False)

    async with TestSessionLocal() as session:
        # Set tenant context for multi-tenant testing
        test_tenant_id = str(uuid.uuid4())
        await session.execute(
            text("SET app.tenant_id = :tenant_id"),
            {"tenant_id": test_tenant_id}
        )

        transaction = await session.begin()
        try:
            yield session
        finally:
            await transaction.rollback()  # Always rollback for isolation
```

### 10. Realistic Test Data Management

**Pattern**: Consistent, business-representative test data with anonymization

**Implementation Details**:
- Parameterized fixtures for different business scenarios
- Realistic email threads, meeting transcripts, and document collections
- Consistent entity relationships across test scenarios
- Performance-optimized data loading and generation

```python
# Test Data Strategy Pattern
class TestDataGenerator:
    def __init__(self):
        self.business_scenarios = {
            'atlas_project': self._atlas_scenarios,
            'budget_approval': self._budget_scenarios,
            'crisis_response': self._crisis_scenarios
        }

    def generate_email_thread(self, scenario: str, participants: List[Person]) -> List[Email]:
        """Generate realistic email thread with proper threading and relationships"""
        scenario_generator = self.business_scenarios[scenario]
        return scenario_generator(participants)

    def generate_meeting_transcript(self, meeting_type: str, duration_minutes: int) -> MeetingTranscript:
        """Generate meeting with realistic dialogue, decisions, and action items"""
        # Consistent patterns based on meeting type and participants

# Parameterized Test Fixtures Pattern
@pytest.fixture(params=['email_escalation', 'project_crisis', 'decision_workflow'])
def business_scenario(request):
    return load_scenario(request.param)

@pytest.fixture
def sample_source_data():
    return {
        "source_system": "gmail",
        "external_id": "msg_12345",
        "content_hash": "a" * 64,
        "raw_content": "This is a sample email content for testing.",
        "content_type": "text/plain",
        "content_size": 45,
        "ingestion_metadata": {"sender": "test@example.com"},
    }
```

### 11. Performance Benchmarking Integration

**Pattern**: Automated performance validation with timing utilities and success criteria

**Implementation Details**:
- Benchmark timing utilities for precise measurement
- Performance targets integrated into test assertions
- Automated regression detection for response times
- Resource monitoring and bottleneck identification

```python
# Performance Testing Pattern
@pytest.fixture
def benchmark_timer():
    """Precision timing utility for performance tests"""
    class Timer:
        def start(self): self.start_time = time.perf_counter()
        def stop(self): self.end_time = time.perf_counter()
        @property
        def elapsed_ms(self): return (self.end_time - self.start_time) * 1000
    return Timer

@pytest.mark.performance
async def test_database_operation_performance(benchmark_timer):
    """Validate database operations meet performance targets"""
    timer = benchmark_timer()
    timer.start()
    await database_operation()
    timer.stop()

    # Validate meets target (<100ms)
    assert timer.elapsed_ms < 100

# Performance Success Criteria Pattern
PERFORMANCE_TARGETS = {
    'crud_operations_ms': 100,
    'vector_search_ms': 500,
    'ai_processing_ms': 10000,
    'environment_setup_s': 60
}

class PerformanceValidator:
    @staticmethod
    def validate_target(operation: str, actual_ms: float):
        target_ms = PERFORMANCE_TARGETS[f'{operation}_ms']
        assert actual_ms < target_ms, f"{operation} took {actual_ms}ms, target was {target_ms}ms"
```

### 12. Test Categorization and Environment Controls

**Pattern**: Automatic test categorization with environment-specific execution controls

**Implementation Details**:
- Automatic marking based on test file location and fixtures
- Environment variables for skipping expensive tests
- Custom pytest markers for different test types
- CI/CD integration with selective test execution

```python
# Test Categorization Pattern
def pytest_collection_modifyitems(config, items):
    """Automatically mark tests based on their location and fixtures"""
    for item in items:
        # Auto-mark based on test file location
        if "unit" in str(item.fspath):
            item.add_marker(pytest.mark.unit)
        elif "integration" in str(item.fspath):
            item.add_marker(pytest.mark.integration)
            item.add_marker(pytest.mark.requires_db)
        elif "performance" in str(item.fspath):
            item.add_marker(pytest.mark.performance)
            item.add_marker(pytest.mark.slow)

        # Mark based on fixtures used
        if any(fixture in item.fixturenames for fixture in ["test_session", "test_engine"]):
            item.add_marker(pytest.mark.requires_db)

# Environment Control Pattern
def pytest_runtest_setup(item):
    """Control test execution based on environment variables"""
    # Skip database tests if DB_SKIP_TESTS is set
    if "requires_db" in [marker.name for marker in item.iter_markers()]:
        if os.environ.get("DB_SKIP_TESTS"):
            pytest.skip("Database tests skipped (DB_SKIP_TESTS set)")

    # Skip slow tests unless explicitly requested
    if "slow" in [marker.name for marker in item.iter_markers()]:
        if not item.config.getoption("--runslow", default=False):
            pytest.skip("Slow test skipped (use --runslow to run)")
```

## Testing Performance Patterns

### AI Mock Performance
- **Unit Test Mocks**: <100ms response time for deterministic patterns
- **Integration Mocks**: <10s with lightweight models (Phi-3 Mini, Qwen2.5-7B)
- **E2E Recorded**: <30s with cached real AI responses
- **Load Testing**: 50+ concurrent AI operations with simulated latency

### Environment Performance
- **Test Environment Setup**: <60 seconds for full containerized stack
- **Parallel Test Execution**: 5+ concurrent test suites without interference
- **Database Test Isolation**: 100% isolation through transaction rollback
- **Environment Teardown**: <30 seconds for complete cleanup

### Test Data Performance
- **Fixture Loading**: <15 seconds for complete business scenario data
- **Data Generation**: Real-time creation of consistent test entities
- **Cross-Test Consistency**: 100% reproducible test results
- **Memory Management**: Efficient cleanup preventing test pollution

## Testing Security Patterns

### AI Response Security
- **Response Sanitization**: All recorded AI responses anonymized
- **Test Data Privacy**: Business-representative but privacy-safe content
- **Model Access Control**: Isolated AI services for testing environments
- **API Key Management**: Separate credentials for test environments

### Environment Security
- **Container Isolation**: Complete separation between test environments
- **Database Security**: Isolated test databases with limited permissions
- **Network Isolation**: Test services cannot access production systems
- **Secret Management**: Environment-specific configuration and credentials

## Production Agent Observability Patterns

### 13. Agent Health Monitoring

**Pattern**: Centralized monitoring for autonomous AI agent health and processing status

**Implementation Details**:
- Real-time status tracking for all processing agents
- Quality metrics with confidence scores and accuracy trends
- Resource usage monitoring (CPU, memory, I/O) per agent
- Schedule adherence tracking for processing jobs

```python
# Agent Health Model Pattern
class AgentHealth(Base):
    agent_id: str
    agent_type: str  # processing, analysis, coordination, review
    status: str  # healthy, degraded, failed
    last_active_at: datetime
    processing_completion_rate: float
    average_confidence_score: float
    resource_usage: dict = JSONB  # cpu, memory, io metrics
    error_count_24h: int
    tenant_id: uuid.UUID

# Health Check Pattern
async def check_agent_health(agent_id: str) -> AgentHealthStatus:
    recent_jobs = await get_recent_jobs(agent_id, hours=24)
    return AgentHealthStatus(
        completion_rate=calculate_completion_rate(recent_jobs),
        avg_confidence=calculate_avg_confidence(recent_jobs),
        error_rate=calculate_error_rate(recent_jobs),
        resource_usage=get_current_resource_usage(agent_id)
    )
```

### 14. Cross-Agent Workflow Tracing

**Pattern**: Distributed tracing for content flow through multiple agent boundaries

**Implementation Details**:
- Content flow tracking through extraction → categorization → storage
- Multi-stage processing timeline visualization
- Bottleneck identification and performance analysis
- End-to-end success rate tracking

```python
# Workflow Event Model Pattern
class WorkflowEvent(Base):
    workflow_id: uuid.UUID
    agent_id: str
    timestamp: datetime
    event_type: str  # workflow_started, stage_completed, handoff, error
    event_status: str  # success, failure, timeout, retry
    processing_time_ms: float
    event_data: dict = JSONB
    parent_workflow_id: uuid.UUID  # For nested workflows
    tenant_id: uuid.UUID

# Workflow Tracing Pattern
@trace_workflow("email_processing")
async def process_email(email_id: uuid.UUID) -> ProcessingResult:
    with trace_stage("extraction"):
        entities = await extract_entities(email_id)

    with trace_stage("categorization"):
        category = await categorize_content(email_id, entities)

    with trace_stage("storage"):
        await store_processed_email(email_id, entities, category)

    return ProcessingResult(success=True, stages_completed=3)
```

### 15. Agent Decision Tracing

**Pattern**: Logging all agent decision points for debugging and analysis

**Implementation Details**:
- Decision point capture with context and alternatives considered
- Confidence threshold logging for human review escalation
- Model selection decision logging
- Quality gate decision tracking

```python
# Decision Trace Model Pattern
class DecisionTrace(Base):
    agent_id: str
    timestamp: datetime
    decision_type: str  # model_selection, categorization, confidence_threshold
    decision_context: dict = JSONB
    alternatives_considered: list = JSONB
    confidence_score: float
    reasoning: str
    workflow_id: uuid.UUID
    tenant_id: uuid.UUID

# Decision Logging Pattern
async def make_categorization_decision(content: str) -> CategoryDecision:
    alternatives = await get_possible_categories(content)

    decision = select_best_category(alternatives)

    await log_decision(DecisionTrace(
        decision_type="categorization",
        decision_context={"content_hash": hash(content)},
        alternatives_considered=alternatives,
        confidence_score=decision.confidence,
        reasoning=decision.reasoning
    ))

    return decision
```

### 16. Business Value KPI Tracking

**Pattern**: Measuring system value delivery through business-focused metrics

**Implementation Details**:
- Context reconstruction speed measurement
- Search accuracy and relevance scoring
- Relationship validation acceptance rates
- Local vs cloud processing cost analysis

```python
# Business KPI Model Pattern
class BusinessKPI(Base):
    kpi_type: str  # context_reconstruction, search_accuracy, relationship_validation
    timestamp: datetime
    metric_value: float
    metric_target: float
    metadata: dict = JSONB
    tenant_id: uuid.UUID

# KPI Tracking Pattern
BUSINESS_TARGETS = {
    'context_reconstruction_minutes': 15,
    'search_accuracy_percent': 90,
    'relationship_validation_rate': 80,
    'email_processing_minutes': 30,
    'meeting_analysis_minutes': 60
}

async def track_context_reconstruction(start_time: datetime, end_time: datetime):
    elapsed_minutes = (end_time - start_time).total_seconds() / 60
    await record_kpi(
        kpi_type='context_reconstruction',
        metric_value=elapsed_minutes,
        metric_target=BUSINESS_TARGETS['context_reconstruction_minutes']
    )
```

### 17. TimescaleDB Time-Series Storage

**Pattern**: Efficient time-series storage using PostgreSQL with TimescaleDB extension

**Implementation Details**:
- Hypertables for automatic partitioning of metrics data
- 1-day chunk intervals for efficient retention management
- Automatic compression for historical data
- Continuous aggregates for fast dashboards

```sql
-- Time-series metrics table with hypertable
CREATE TABLE agent_metrics (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(agent_id),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metric_name TEXT NOT NULL,
    metric_value FLOAT NOT NULL,
    metric_type TEXT DEFAULT 'gauge',
    labels JSONB DEFAULT '{}',
    tenant_id UUID NOT NULL
);

-- Convert to hypertable (1-day chunks)
SELECT create_hypertable('agent_metrics', 'timestamp',
                        chunk_time_interval => INTERVAL '1 day');

-- Add compression policy for data >7 days
SELECT add_compression_policy('agent_metrics', INTERVAL '7 days');

-- Add retention policy (90 days)
SELECT add_retention_policy('agent_metrics', INTERVAL '90 days');

-- Continuous aggregate for hourly summaries
CREATE MATERIALIZED VIEW agent_metrics_hourly
WITH (timescaledb.continuous) AS
SELECT agent_id,
       time_bucket('1 hour', timestamp) AS bucket,
       metric_name,
       avg(metric_value) as avg_value,
       max(metric_value) as max_value,
       count(*) as sample_count
FROM agent_metrics
GROUP BY agent_id, bucket, metric_name;
```

## Observability Performance Patterns

### Metrics Collection
- **Collection Overhead**: <50ms per operation instrumented
- **Storage Efficiency**: TimescaleDB compression reduces storage 90%+
- **Query Performance**: Continuous aggregates enable <1s dashboard loads
- **Retention**: 90 days detailed, 1 year aggregated

### Workflow Tracing
- **Trace Overhead**: <100ms per workflow traced
- **Cross-Agent Correlation**: Sub-second trace assembly
- **Historical Analysis**: 30-day trace retention with search
- **Real-time Visibility**: <5s latency for live workflow status

### Alerting
- **Alert Latency**: <2 minutes from threshold breach to notification
- **False Positive Rate**: <5% through adaptive thresholds
- **Alert Aggregation**: Intelligent grouping prevents alert storms
- **Escalation**: Automatic escalation for unacknowledged alerts

## Email Integration Patterns

### 18. OAuth2 Token Management with Encryption

**Pattern**: Secure storage and lifecycle management for OAuth2 credentials

**Implementation Details**:
- AES-256 encryption for token storage at rest
- Automatic token refresh before expiration
- Revocation detection and re-authorization flow
- Multi-account credential isolation

```python
# OAuth2 Token Storage Pattern
class EncryptedTokenStorage:
    def __init__(self, encryption_key: bytes):
        self.cipher = Fernet(encryption_key)

    async def store_token(self, account_id: str, token_data: dict):
        encrypted = self.cipher.encrypt(json.dumps(token_data).encode())
        await self.db.execute(
            "UPDATE gmail_accounts SET encrypted_token = :token WHERE id = :id",
            {"token": encrypted, "id": account_id}
        )

    async def get_token(self, account_id: str) -> dict:
        result = await self.db.fetch_one(
            "SELECT encrypted_token FROM gmail_accounts WHERE id = :id",
            {"id": account_id}
        )
        return json.loads(self.cipher.decrypt(result["encrypted_token"]))

# Token Refresh Pattern
async def ensure_valid_token(account_id: str) -> str:
    token_data = await token_storage.get_token(account_id)

    if datetime.now() >= token_data["expiry"] - timedelta(minutes=5):
        # Refresh before expiration
        new_token = await refresh_oauth_token(token_data["refresh_token"])
        await token_storage.store_token(account_id, new_token)
        return new_token["access_token"]

    return token_data["access_token"]
```

### 19. Real-Time Sync with Push/Poll Fallback

**Pattern**: Hybrid synchronization using push notifications with polling fallback

**Implementation Details**:
- Gmail Push notifications via Cloud Pub/Sub webhooks
- Automatic fallback to polling if push fails
- Configurable sync intervals for fallback mode
- Catch-up sync after connectivity restoration

```python
# Push/Poll Hybrid Pattern
class HybridSyncManager:
    def __init__(self, account_id: str):
        self.account_id = account_id
        self.push_active = False
        self.poll_interval = 60  # seconds

    async def start_sync(self):
        try:
            # Attempt push notification setup
            await self.setup_push_notifications()
            self.push_active = True
        except PushSetupError:
            # Fallback to polling
            logger.warning(f"Push setup failed for {self.account_id}, using polling")
            await self.start_polling()

    async def handle_push_notification(self, notification: dict):
        history_id = notification["historyId"]
        await self.process_changes_since(history_id)

    async def start_polling(self):
        while not self.push_active:
            await self.check_for_new_messages()
            await asyncio.sleep(self.poll_interval)

# Webhook Handler Pattern
@app.post("/webhooks/gmail/{account_id}")
async def gmail_webhook(account_id: str, request: Request):
    notification = await request.json()
    await sync_manager.handle_push_notification(notification)
    return {"status": "ok"}
```

### 20. Multi-Account Priority Scheduling

**Pattern**: Intelligent scheduling across multiple accounts with priority management

**Implementation Details**:
- Priority-based account ordering (work > personal)
- Resource allocation based on account priority
- Concurrent sync limits to prevent overload
- Cross-account deduplication

```python
# Multi-Account Scheduler Pattern
class AccountScheduler:
    def __init__(self, max_concurrent: int = 3):
        self.max_concurrent = max_concurrent
        self.semaphore = asyncio.Semaphore(max_concurrent)

    async def schedule_sync(self, accounts: List[GmailAccount]):
        # Sort by priority (lower number = higher priority)
        sorted_accounts = sorted(accounts, key=lambda a: a.priority)

        tasks = []
        for account in sorted_accounts:
            task = asyncio.create_task(self._sync_with_limit(account))
            tasks.append(task)

        await asyncio.gather(*tasks, return_exceptions=True)

    async def _sync_with_limit(self, account: GmailAccount):
        async with self.semaphore:
            await self.sync_account(account)

# Priority Configuration Pattern
class GmailAccount(Base):
    id: uuid.UUID
    email: str
    priority: int  # 1=highest (work), 2=medium, 3=lowest (newsletters)
    sync_enabled: bool
    last_sync_at: datetime
    sync_interval_minutes: int  # Higher priority = shorter interval
```

### 21. Attachment Processing Pipeline

**Pattern**: Background queue processing for email attachments with format handling

**Implementation Details**:
- Celery task queue for background processing
- Format-specific extractors (PDF, DOCX, images)
- Size limits and validation
- Retry logic for transient failures

```python
# Attachment Queue Pattern
@celery.task(bind=True, max_retries=3)
def process_attachment(self, attachment_id: str):
    try:
        attachment = get_attachment(attachment_id)

        # Select extractor based on mime type
        extractor = get_extractor(attachment.mime_type)
        if not extractor:
            mark_unsupported(attachment_id)
            return

        # Extract content
        content = extractor.extract(attachment.data)

        # Store extracted content
        store_extracted_content(attachment_id, content)

    except TransientError as e:
        # Retry with exponential backoff
        raise self.retry(exc=e, countdown=2 ** self.request.retries)

# Format Extractor Registry Pattern
EXTRACTORS = {
    "application/pdf": PDFExtractor(),
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": DocxExtractor(),
    "image/png": OCRExtractor(),
    "image/jpeg": OCRExtractor(),
}

def get_extractor(mime_type: str) -> Optional[ContentExtractor]:
    return EXTRACTORS.get(mime_type)
```

### 22. Privacy Filter Chain

**Pattern**: Configurable filtering pipeline for sensitive content

**Implementation Details**:
- Chain of responsibility for filter evaluation
- Label-based sensitivity detection
- Configurable exclusion rules
- Audit logging for filtered content

```python
# Privacy Filter Chain Pattern
class PrivacyFilterChain:
    def __init__(self):
        self.filters: List[PrivacyFilter] = []

    def add_filter(self, filter: PrivacyFilter):
        self.filters.append(filter)

    async def should_process(self, email: Email) -> Tuple[bool, Optional[str]]:
        for filter in self.filters:
            allowed, reason = await filter.check(email)
            if not allowed:
                await self.log_filtered(email, filter.name, reason)
                return False, reason
        return True, None

# Filter Implementations
class LabelFilter(PrivacyFilter):
    def __init__(self, excluded_labels: List[str]):
        self.excluded_labels = excluded_labels

    async def check(self, email: Email) -> Tuple[bool, Optional[str]]:
        for label in email.labels:
            if label in self.excluded_labels:
                return False, f"Excluded label: {label}"
        return True, None

class SenderFilter(PrivacyFilter):
    def __init__(self, excluded_domains: List[str]):
        self.excluded_domains = excluded_domains

    async def check(self, email: Email) -> Tuple[bool, Optional[str]]:
        domain = email.sender.split("@")[1]
        if domain in self.excluded_domains:
            return False, f"Excluded domain: {domain}"
        return True, None

# Usage
filter_chain = PrivacyFilterChain()
filter_chain.add_filter(LabelFilter(["confidential", "personal"]))
filter_chain.add_filter(SenderFilter(["private.com"]))
```

## Email Integration Performance

### Sync Performance
- **Real-time Detection**: <60 seconds for new email notification
- **Historical Import**: 100-150 emails/minute with rate limit compliance
- **Attachment Processing**: Background queue, <5 minutes for common formats

### Rate Limit Management
- **Gmail API Quota**: 250 units/user/second, 1B units/day
- **Token Bucket**: Smooth request distribution across quota window
- **Backoff Strategy**: Exponential backoff on 429 responses

### Multi-Account Efficiency
- **Concurrent Syncs**: Up to 3 accounts simultaneously
- **Priority Scheduling**: High-priority accounts sync more frequently
- **Resource Isolation**: Failures in one account don't affect others