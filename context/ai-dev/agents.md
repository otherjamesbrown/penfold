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

## Core Patterns (From Specs 002 + 003 Production Implementation)

### Multi-Model Coordination Pattern (NEW - Production Ready)
```python
# Central coordination for parallel multi-model processing
class ModelCoordinator:
    """Orchestrates multiple AI models for enhanced quality and reliability."""

    def __init__(self, session: AsyncSession, event_publisher: EventPublisher,
                 job_manager: JobManager):
        self.session = session
        self.event_publisher = event_publisher
        self.job_manager = job_manager
        self.ensemble_combiner = EnsembleCombiner()
        self.escalation_manager = EscalationManager(session)
        self.performance_tracker = PerformanceTracker(session)

    async def coordinate_processing(self, content_id: str, content_type: str,
                                   content_data: Dict[str, Any]) -> str:
        # 1. Select optimal models based on content type and performance history
        selected_models = await self._select_models_for_content(content_type, content_data)

        # 2. Create coordination workflow
        coordination_id = str(uuid.uuid4())

        # 3. Publish event for parallel processing
        event_id = await self.event_publisher.publish(
            event_type="content.ingested",
            event_data={
                "content_id": content_id,
                "content_type": content_type,
                "coordination_id": coordination_id,
                **content_data
            }
        )

        # 4. Create jobs for each model
        for model_id in selected_models:
            await self.job_manager.create_job(
                event_id=event_id,
                processor_name=model_id,
                job_type=f"{content_type}_processing",
                input_data=content_data,
                priority=self.registered_models[model_id].priority
            )

        return coordination_id
```

### Ensemble Learning Pattern (NEW - Production Ready)
```python
# Sophisticated result combination with multiple strategies
class EnsembleCombiner:
    """Combines multiple AI model results for improved quality."""

    async def combine_results(self, individual_results: List[Dict],
                             content_type: str, strategy: str = "weighted_average") -> EnsembleResult:

        if strategy == "weighted_average":
            return await self._weighted_average_combination(individual_results)
        elif strategy == "confidence_voting":
            return await self._confidence_voting_combination(individual_results)
        elif strategy == "majority_vote":
            return await self._majority_vote_combination(individual_results)

    async def _weighted_average_combination(self, results: List[Dict]) -> EnsembleResult:
        """Combine results weighted by confidence scores."""
        total_weight = sum(r["confidence_score"] for r in results)
        weighted_results = {}

        for result in results:
            weight = result["confidence_score"] / total_weight
            for key, value in result["result_data"].items():
                if key not in weighted_results:
                    weighted_results[key] = []
                weighted_results[key].append((value, weight))

        # Combine weighted values
        combined_result = self._aggregate_weighted_values(weighted_results)

        return EnsembleResult(
            ensemble_id=str(uuid.uuid4()),
            primary_result=combined_result,
            supporting_results=results,
            confidence_score=self._calculate_ensemble_confidence(results),
            combination_strategy="weighted_average",
            consensus_strength=self._calculate_consensus_strength(results)
        )
```

### Confidence-Based Escalation Pattern (NEW - Production Ready)
```python
# Intelligent cloud escalation with cost management
class EscalationManager:
    """Manages escalation to cloud models based on confidence and cost analysis."""

    def __init__(self, session: AsyncSession):
        self.session = session
        self.escalation_config = {
            "email": {"confidence_threshold": 0.8, "max_daily_cost": 10.0},
            "document": {"confidence_threshold": 0.85, "max_daily_cost": 20.0},
            "meeting": {"confidence_threshold": 0.9, "max_daily_cost": 15.0}
        }

    async def should_escalate(self, local_result: Dict, content_type: str) -> EscalationDecision:
        """Determine if escalation to cloud model is warranted."""
        config = self.escalation_config.get(content_type, {})
        threshold = config.get("confidence_threshold", 0.8)

        # Check confidence threshold
        if local_result["confidence_score"] >= threshold:
            return EscalationDecision(should_escalate=False, reason="sufficient_confidence")

        # Check budget constraints
        daily_cost = await self._get_daily_escalation_cost(content_type)
        max_cost = config.get("max_daily_cost", 10.0)

        if daily_cost >= max_cost:
            return EscalationDecision(should_escalate=False, reason="budget_exceeded")

        # Calculate escalation value
        cost_per_improvement = await self._estimate_escalation_cost_benefit(content_type)

        return EscalationDecision(
            should_escalate=True,
            reason="quality_improvement",
            estimated_cost=cost_per_improvement,
            target_models=["gpt-4", "claude-3-sonnet"]
        )
```

### Performance Learning Pattern (NEW - Production Ready)
```python
# Continuous model performance optimization
class PerformanceTracker:
    """Tracks model performance and optimizes selection over time."""

    async def optimize_model_selection(self, content_type: str,
                                     available_models: List[str]) -> List[str]:
        """Select optimal models based on historical performance."""

        # Get performance metrics for each model
        performance_data = {}
        for model_id in available_models:
            metrics = await self._get_model_performance(model_id, content_type)
            performance_data[model_id] = {
                "success_rate": metrics.get("success_rate", 0.0),
                "avg_confidence": metrics.get("avg_confidence", 0.0),
                "avg_processing_time": metrics.get("avg_processing_time", float("inf")),
                "cost_efficiency": metrics.get("cost_efficiency", 0.0)
            }

        # Score models based on multiple criteria
        scored_models = []
        for model_id, metrics in performance_data.items():
            score = (
                metrics["success_rate"] * 0.4 +
                metrics["avg_confidence"] * 0.3 +
                (1.0 / max(metrics["avg_processing_time"], 1.0)) * 0.2 +
                metrics["cost_efficiency"] * 0.1
            )
            scored_models.append((model_id, score))

        # Return models sorted by performance score
        scored_models.sort(key=lambda x: x[1], reverse=True)
        return [model_id for model_id, _ in scored_models]
```

---

## Event Processing Integration Patterns (From Specs 002)

### EventPublisher Integration
```python
# Leveraging event processing for model coordination
async def coordinate_ai_models(content_data: Dict) -> str:
    coordination_id = str(uuid.uuid4())

    # Publish coordination event
    await event_publisher.publish(
        event_type="ai.coordination.start",
        event_data={
            "coordination_id": coordination_id,
            "content_data": content_data,
            "target_models": ["llama-3.1-8b", "gpt-4"],
            "require_ensemble": True
        },
        channel="ai.processing"
    )

    return coordination_id
```

### JobManager Integration
```python
# Using job management for parallel processing
async def create_model_processing_jobs(coordination_id: str, models: List[str]) -> List[str]:
    job_ids = []

    for model_id in models:
        job_id = await job_manager.create_job(
            event_id=coordination_id,
            processor_name=model_id,
            job_type="ai_processing",
            input_data=content_data,
            priority=1,
            metadata={"coordination_id": coordination_id}
        )
        job_ids.append(job_id)

    return job_ids
```

---

## Legacy Patterns (Pre-Production)

### Basic Tiered Processing Pattern

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