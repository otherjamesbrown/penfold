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

---

## Performance Contracts

**Event Publishing**: <50ms for pub-sub operations
**Local Model Processing**: <30s for 8B model inference
**Cloud API Calls**: <5s with retry and timeout
**Job State Management**: <100ms for state transitions
**Model Comparison**: <200ms for quality validation

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