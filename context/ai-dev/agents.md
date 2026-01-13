# AI Developer Agent Context

> **Inherits**: CLAUDE.md → context/agents.md → this file
> **Domain**: Model Integration, Pub-Sub Processing, Event Coordination

---

## Domain Ownership

**You own:**
- AI model integration (local and cloud)
- Event-driven processing framework (pub-sub)
- Processing job management and state
- AI response aggregation and comparison
- Model selection and routing logic
- AI pipeline performance optimization
- Event publishing and subscription logic
- AI-related CLI commands

**You do NOT own:**
- Database schema or storage (→ database-dev)
- External system connectors (→ integration-dev)
- Search interface or queries (→ search-dev)
- Test framework or mocking (→ testing-dev)
- Raw data ingestion

---

## AI-Specific Rules

**NEVER:**
- Call cloud APIs without cost management
- Process data without tenant context
- Create processing jobs without error handling
- Modify AI responses after confidence scoring
- Skip model performance benchmarking
- Process events without proper state tracking

**ALWAYS:**
- Use local models first, escalate to cloud selectively
- Track processing costs and performance metrics
- Maintain tenant isolation in all AI operations
- Store processing results with attribution and confidence
- Implement retry logic with exponential backoff
- Compare multiple model outputs for quality validation

---

## Core Patterns (Distilled from AI Architecture)

### Tiered Processing Pattern
```python
# Local-first, cloud-selective processing
class AIProcessor:
    def __init__(self, tenant_id: str):
        self.tenant_id = tenant_id
        self.local_models = LocalModelRegistry()
        self.cloud_models = CloudModelRegistry()

    async def process_content(self, content: str, task_type: str) -> ProcessingResult:
        # Try local first
        local_result = await self.local_models.process(content, task_type)

        if local_result.confidence > 0.8:
            return local_result

        # Escalate to cloud for low confidence
        cloud_result = await self.cloud_models.process(content, task_type)
        return self._aggregate_results([local_result, cloud_result])
```

### Event-Driven Processing Pattern
```python
# Pub-sub event coordination
@subscribe_to('content.ingested')
async def process_with_multiple_models(event: ProcessingEvent):
    content = event.payload['content']

    # Spawn multiple processors
    summarization_job = await start_processing_job(
        'summarization', content, event.tenant_id
    )
    entity_extraction_job = await start_processing_job(
        'entity_extraction', content, event.tenant_id
    )
    categorization_job = await start_processing_job(
        'categorization', content, event.tenant_id
    )

    # Store job state for aggregation
    await store_processing_jobs([
        summarization_job, entity_extraction_job, categorization_job
    ], event.id)
```

### Multi-Model Comparison Pattern
```python
# Benchmarking and quality validation
class ModelBenchmark:
    async def compare_models(self, content: str, task: str) -> BenchmarkResult:
        models = ['llama-3.1-8b', 'phi-3-mini', 'qwen2.5-7b']
        results = []

        for model in models:
            start_time = time.time()
            result = await self.process_with_model(model, content, task)
            elapsed = time.time() - start_time

            results.append({
                'model': model,
                'result': result,
                'latency': elapsed,
                'cost': self.calculate_cost(model, elapsed),
                'confidence': result.confidence
            })

        return BenchmarkResult(results)
```

### Production Event Processing Patterns ✅ (From 002-event-processing Implementation)

#### EventPublisher Pattern - Redis with PostgreSQL Fallback
```python
# Production-ready event publishing with fallback
class EventPublisher:
    def __init__(self, redis_client: Redis, db_session: AsyncSession):
        self.redis = redis_client
        self.db = db_session

    async def publish_event(self, event_type: str, payload: dict, tenant_id: str):
        event = ProcessingEvent(
            type=event_type,
            payload=payload,
            tenant_id=tenant_id,
            timestamp=datetime.utcnow()
        )

        try:
            # Primary: Redis pub-sub for real-time
            await self.redis.publish(f"events:{tenant_id}:{event_type}",
                                   event.to_json())
        except RedisError:
            # Fallback: PostgreSQL LISTEN/NOTIFY
            await self.db.execute(
                text("NOTIFY penfold_events, :payload"),
                {"payload": event.to_json()}
            )

        # Always store in database for reliability
        self.db.add(event)
        await self.db.commit()
        return event
```

#### JobManager Pattern - Complete Lifecycle Management
```python
# Production job management with atomic state transitions
class JobManager:
    async def create_job(self, event: ProcessingEvent, processor_type: str) -> ProcessingJob:
        job = ProcessingJob(
            event_id=event.id,
            processor_type=processor_type,
            status=JobStatus.QUEUED,
            tenant_id=event.tenant_id,
            created_at=datetime.utcnow()
        )

        self.db.add(job)
        await self.db.commit()

        # Publish job available event
        await self.event_publisher.publish_event(
            'job.available', {'job_id': job.id}, job.tenant_id
        )

        return job

    async def claim_job(self, job_id: str, processor_id: str) -> bool:
        # Atomic job claiming with Redis lock
        lock_key = f"job_lock:{job_id}"
        async with self.redis.lock(lock_key, timeout=300):
            job = await self.db.get(ProcessingJob, job_id)

            if job.status == JobStatus.QUEUED:
                job.status = JobStatus.CLAIMED
                job.processor_id = processor_id
                job.claimed_at = datetime.utcnow()
                await self.db.commit()
                return True

        return False
```

#### SubscriptionManager Pattern - Dynamic Event Routing
```python
# JSONB-based filtering with dynamic subscriptions
class SubscriptionManager:
    async def subscribe(self, processor_id: str, event_types: list,
                       filters: dict = None) -> Subscription:
        subscription = Subscription(
            processor_id=processor_id,
            event_types=event_types,
            filter_criteria=filters or {},  # JSONB field for flexible filtering
            created_at=datetime.utcnow()
        )

        self.db.add(subscription)
        await self.db.commit()

        # Register Redis subscription patterns
        for event_type in event_types:
            pattern = f"events:*:{event_type}"
            await self.redis.psubscribe(pattern)

        return subscription

    async def route_event(self, event: ProcessingEvent):
        # Find matching subscriptions using JSONB queries
        matching_subs = await self.db.execute(
            select(Subscription).where(
                Subscription.event_types.contains([event.type]),
                func.jsonb_matches(Subscription.filter_criteria, event.payload)
            )
        )

        # Create jobs for matching processors
        for subscription in matching_subs.scalars():
            await self.job_manager.create_job(event, subscription.processor_id)
```

#### ResultAggregator Pattern - Multi-Model Quality Validation
```python
# Production result aggregation with confidence scoring
class ResultAggregator:
    async def aggregate_results(self, job_ids: list[str]) -> AggregatedResult:
        jobs = await self.get_completed_jobs(job_ids)
        results = [job.result for job in jobs if job.result]

        if not results:
            raise ValueError("No completed results to aggregate")

        # Calculate ensemble confidence
        avg_confidence = sum(r.confidence for r in results) / len(results)

        # Compare results for consistency
        consistency_score = self._calculate_consistency(results)

        # Select best result based on confidence and consistency
        best_result = max(results, key=lambda r: r.confidence * consistency_score)

        aggregated = AggregatedResult(
            primary_result=best_result,
            supporting_results=results,
            confidence_score=avg_confidence,
            consistency_score=consistency_score,
            selection_reasoning=f"Selected based on {best_result.confidence:.2f} confidence"
        )

        return aggregated

    def _calculate_consistency(self, results: list[ProcessingResult]) -> float:
        # Implementation of result similarity analysis
        if len(results) < 2:
            return 1.0

        similarity_scores = []
        for i, result1 in enumerate(results):
            for result2 in results[i+1:]:
                similarity = self._calculate_similarity(result1.content, result2.content)
                similarity_scores.append(similarity)

        return sum(similarity_scores) / len(similarity_scores) if similarity_scores else 1.0
```

---

## Performance Contracts ✅ (Production Validated)

**Event Publishing**: <10ms for events up to 1MB payload ✅ TESTED
**Job Creation & Queuing**: <50ms from event publication ✅ TESTED
**State Transitions**: <100ms with atomic consistency ✅ TESTED
**Result Aggregation**: <200ms for up to 10 processors ✅ TESTED
**Concurrent Processing**: 1000+ simultaneous jobs ✅ TESTED
**Queue Latency**: Sub-second under normal load ✅ TESTED

**Local Model Processing**: <30s for 8B model inference
**Cloud API Calls**: <5s with retry and timeout
**Health Monitoring**: 30-second processor timeout detection
**Retry Logic**: Exponential backoff (1s, 2s, 4s, 8s, 16s) max 5 attempts
**Dead Letter Queue**: <5% failure rate isolation

### Performance Testing Pattern
```python
@pytest.mark.performance
async def test_ai_processing_performance():
    content = "Test email content for processing"

    start_time = time.time()
    result = await ai_processor.summarize_locally(content)
    elapsed = time.time() - start_time

    assert elapsed < 30.0, f"Local processing took {elapsed:.1f}s, target <30s"
    assert result.confidence > 0.7, "Processing quality below threshold"
```

---

## Common Tasks

### Adding New AI Model
1. Register model in `penf_lib/ai/models/`
2. Create model adapter with standard interface
3. Add to model selection logic
4. Implement cost tracking
5. Write performance benchmarks
6. Add to A/B testing framework

### Processing Pipeline Integration
1. Define event types in `penf_lib/events/`
2. Create event handlers with `@subscribe_to`
3. Implement job state management
4. Add retry logic and error handling
5. Store results with attribution
6. Create aggregation logic

### Model Performance Optimization
1. Monitor processing latencies
2. Optimize model parameters (temperature, top_p)
3. Implement caching for similar requests
4. Add batch processing capabilities
5. Benchmark against alternatives

---

## Event Framework

### Event Types
- `content.ingested` - New content requires processing
- `ai.processing.started` - Processing job initiated
- `ai.processing.completed` - Processing job finished
- `ai.processing.failed` - Processing job error
- `ai.results.aggregated` - Multi-model results combined

### Job States
```python
class JobState(Enum):
    QUEUED = "queued"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    FAILED = "failed"
    RETRYING = "retrying"
    CANCELLED = "cancelled"
```

### Processing Result Format
```python
@dataclass
class ProcessingResult:
    job_id: str
    tenant_id: str
    model: str
    task_type: str
    input_content: str
    output_content: str
    confidence: float
    processing_time: float
    cost: float
    metadata: dict
    created_at: datetime
```

---

## Cost Management

### Local Model Strategy
- Use local models for 80% of processing
- Cloud escalation only for low confidence (<0.8)
- Cache frequent queries to reduce recomputation
- Batch similar requests for efficiency

### Cloud API Cost Controls
```python
class CloudCostManager:
    def __init__(self, daily_budget: float = 10.0):
        self.daily_budget = daily_budget
        self.current_spend = 0.0

    async def can_make_request(self, estimated_cost: float) -> bool:
        if self.current_spend + estimated_cost > self.daily_budget:
            logger.warning(f"Daily budget {self.daily_budget} would be exceeded")
            return False
        return True
```

---

## Troubleshooting

### Model Performance Issues
```bash
# Check model response times
grep "model_latency" logs/ai-processing.log | tail -20

# Monitor model accuracy
SELECT model, AVG(confidence), COUNT(*) FROM processing_results
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY model;
```

### Event Processing Delays
```bash
# Check event queue depth
redis-cli llen "events:pending"

# Monitor job state distribution
SELECT state, COUNT(*) FROM processing_jobs GROUP BY state;
```

### Cost Overruns
```bash
# Daily cost tracking
SELECT DATE(created_at), SUM(cost) FROM processing_results
WHERE model LIKE 'gemini%' OR model LIKE 'gpt%'
GROUP BY DATE(created_at);
```

---

## Handoff Conditions

Create handoff beads for:

| Condition | Handoff To | Example |
|-----------|------------|---------|
| Processing results need storage | database-dev | "AI results ready, need optimized storage" |
| Events trigger external systems | integration-dev | "Processing done, need Gmail sync" |
| Search needs AI query expansion | search-dev | "Models ready, need query interface" |
| Performance testing needed | testing-dev | "Pipeline ready, need load testing" |
| Database queries for AI features | database-dev | "Need vector similarity queries" |

---

## Success Criteria

Before closing AI beads:
- [ ] Local models prioritized over cloud APIs
- [ ] Cost tracking implemented and within budget
- [ ] Processing jobs have proper error handling
- [ ] Multi-model comparison shows quality improvement
- [ ] Performance targets met (<50ms events, <30s local processing)
- [ ] Tenant isolation maintained in all AI operations
- [ ] Results stored with attribution and confidence scores

---

## Key Files

**Models**: `penf_lib/ai/models/`
**Events**: `penf_lib/events/`
**Processing**: `penf_lib/ai/processing/`
**Jobs**: `penf_lib/ai/jobs.py`
**Tests**: `tests/unit/ai/`, `tests/integration/ai/`
**Config**: `penf_lib/ai/config.py`

---

## Quick Commands

```bash
# AI operations
pytest tests/unit/ai/ -v           # Unit tests
pytest tests/integration/ai/ -v    # Integration tests
pytest tests/performance/ -k ai    # Performance tests

# Event monitoring
redis-cli monitor                  # Watch event pub-sub
bd list --status open --assignee ai-dev  # AI tasks

# Model benchmarking
python -m penf_lib.ai.benchmark    # Compare model performance
```