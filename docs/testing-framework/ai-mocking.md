# AI Model Mocking Strategies

This guide covers the comprehensive AI mocking framework for testing Penfold's AI-first architecture.

## Mocking Strategy Overview

### Three-Tiered Approach

| Test Type | AI Strategy | Performance Target | Use Case |
|-----------|-------------|-------------------|----------|
| Unit | Full Mocking | <100ms | Component isolation |
| Integration | Lightweight Models | <10s | Multi-component workflows |
| End-to-End | Record/Replay | <30s | Critical path validation |

## Unit Test Mocking (Full Mocking)

### Pattern-Based Response Generation

```python
class OllamaMockServer:
    def _deterministic_response(self, model: str, prompt: str) -> dict:
        """Generate consistent response based on prompt patterns"""
        if 'summarize' in prompt.lower():
            return {
                'response': f'Summary: [Mock summary for {prompt[:50]}...]',
                'model': model,
                'done': True
            }
        elif 'extract entities' in prompt.lower():
            return {
                'response': json.dumps({
                    'people': ['James Brown', 'Sarah Chen'],
                    'projects': ['Atlas Integration'],
                    'decisions': ['Delay timeline by 1 week']
                }),
                'model': model,
                'done': True
            }

# Usage in tests
@pytest.fixture
def mock_ollama():
    with patch('penf_lib.ai.ollama_client') as mock:
        mock.generate = AsyncMock(side_effect=OllamaMockServer().generate)
        yield mock

async def test_entity_extraction_mock(mock_ollama):
    processor = EmailProcessor()

    result = await processor.extract_entities("Email about Atlas project")

    assert 'Atlas Integration' in result['projects']
    assert 'James Brown' in result['people']
```

### Cloud API Mocking

```python
class CloudAPIMock:
    def __init__(self, provider: str):
        self.provider = provider
        self.responses = {
            'gemini': self._gemini_responses,
            'claude': self._claude_responses,
            'gpt': self._gpt_responses
        }

    async def generate_content(self, prompt: str) -> AIResponse:
        generator = self.responses[self.provider]
        return generator(prompt)

    def _gemini_responses(self, prompt: str) -> AIResponse:
        if 'categorize' in prompt.lower():
            return AIResponse(
                content=json.dumps({
                    'category': 'project_update',
                    'confidence': 0.95,
                    'reasoning': 'Contains project status information'
                }),
                usage={'input_tokens': len(prompt.split()), 'output_tokens': 20},
                model='gemini-mock'
            )

# Test fixture for cloud API mocking
@pytest.fixture
def mock_cloud_apis():
    with patch('penf_lib.ai.gemini_client') as mock_gemini:
        with patch('penf_lib.ai.claude_client') as mock_claude:
            mock_gemini.generate_content = AsyncMock(
                side_effect=CloudAPIMock('gemini').generate_content
            )
            mock_claude.generate_content = AsyncMock(
                side_effect=CloudAPIMock('claude').generate_content
            )
            yield {
                'gemini': mock_gemini,
                'claude': mock_claude
            }
```

## Integration Test Models (Lightweight Models)

### Fast Model Configuration

```python
LIGHTWEIGHT_MODELS = {
    'summarization': 'phi-3-mini-3.8b',     # 3.8B parameters, fast inference
    'entity_extraction': 'qwen2.5-7b',      # Excellent at structured tasks
    'categorization': 'llama-3.2-3b',       # Fast classification model
    'embedding': 'nomic-embed-text',         # Consistent vector embeddings
    'general': 'phi-3-mini-3.8b'            # Default fallback
}

class LightweightModelStrategy:
    async def process_with_fast_model(self, task: str, content: str) -> AIResponse:
        model = LIGHTWEIGHT_MODELS.get(task, 'phi-3-mini-3.8b')

        return await ollama_client.generate(
            model=model,
            prompt=content,
            options={
                'temperature': 0.1,    # More deterministic
                'top_p': 0.8,          # Faster sampling
                'top_k': 20,           # Reduced search space
                'num_predict': 200     # Shorter responses for speed
            }
        )

# Integration test fixture
@pytest.fixture
async def lightweight_ai():
    strategy = LightweightModelStrategy()

    # Patch model selection to use lightweight models
    with patch('penf_lib.ai.get_model_for_task') as mock_get_model:
        mock_get_model.side_effect = lambda task: LIGHTWEIGHT_MODELS.get(task)
        yield strategy
```

### Model Performance Validation

```python
@pytest.mark.performance
async def test_lightweight_model_performance(lightweight_ai, benchmark_timer):
    """Ensure lightweight models meet performance targets"""

    timer = benchmark_timer()
    timer.start()

    result = await lightweight_ai.process_with_fast_model(
        'summarization',
        "Long email content that needs to be summarized quickly for integration testing"
    )

    timer.stop()

    # Should complete in under 10 seconds
    assert timer.elapsed_ms < 10000
    assert result is not None
    assert len(result.content) > 10  # Non-trivial response
```

## End-to-End Recording/Replay

### Recording Real AI Sessions

```python
class AIResponseRecorder:
    def __init__(self, storage_path: str = './test-data/ai-responses'):
        self.storage_path = Path(storage_path)
        self.storage_path.mkdir(exist_ok=True)

    async def record_business_scenario(self, scenario_name: str):
        """Record real AI processing for a business scenario"""

        # Load real business scenario data
        scenario_data = load_business_scenario(scenario_name)

        interactions = []
        for email in scenario_data.emails:
            # Record summarization
            summary = await real_ollama_client.summarize(email.content)
            interactions.append(AIInteraction(
                model='llama-3.1-8b',
                task='summarization',
                input=email.content,
                output=summary,
                metadata={'email_id': email.id, 'scenario': scenario_name}
            ))

            # Record entity extraction
            entities = await real_ollama_client.extract_entities(email.content)
            interactions.append(AIInteraction(
                model='llama-3.1-8b',
                task='entity_extraction',
                input=email.content,
                output=entities,
                metadata={'email_id': email.id, 'scenario': scenario_name}
            ))

        await self.save_session(f"{scenario_name}_complete", interactions)

# Script to record test sessions
async def record_test_sessions():
    recorder = AIResponseRecorder()

    scenarios = [
        'atlas_project_escalation',
        'budget_approval_workflow',
        'meeting_coordination',
        'crisis_response'
    ]

    for scenario in scenarios:
        print(f"Recording {scenario}...")
        await recorder.record_business_scenario(scenario)
        print(f"✓ {scenario} recorded")

# Run with: python -m scripts.record_ai_sessions
```

### Replaying Recorded Sessions

```python
class AISessionReplay:
    def __init__(self, session_data: dict):
        self.session_data = session_data
        self.interactions = {
            self._interaction_key(i): i
            for i in session_data['interactions']
        }

    def _interaction_key(self, interaction: dict) -> str:
        """Create lookup key for interaction"""
        content_hash = hashlib.md5(interaction['input'].encode()).hexdigest()
        return f"{interaction['model']}:{interaction['task']}:{content_hash[:8]}"

    async def replay_interaction(self, model: str, task: str, input_content: str):
        """Replay recorded interaction"""
        content_hash = hashlib.md5(input_content.encode()).hexdigest()
        key = f"{model}:{task}:{content_hash[:8]}"

        if key in self.interactions:
            interaction = self.interactions[key]
            return interaction['output']
        else:
            # Fallback to pattern-based mock if no recording found
            return self._generate_fallback(task, input_content)

# Test fixture for recorded responses
@pytest.fixture
def recorded_ai_sessions():
    recorder = AIResponseRecorder()

    sessions = {
        'atlas_project': recorder.load_session('atlas_project_escalation_complete'),
        'budget_approval': recorder.load_session('budget_approval_workflow_complete'),
        'meeting_coordination': recorder.load_session('meeting_coordination_complete'),
        'crisis_response': recorder.load_session('crisis_response_complete')
    }

    return sessions

@pytest.mark.slow
async def test_atlas_project_complete_workflow(recorded_ai_sessions):
    """Test complete Atlas project workflow with recorded AI responses"""

    session = recorded_ai_sessions['atlas_project']

    # Process Atlas project emails with recorded responses
    emails = load_atlas_project_emails()

    with patch('penf_lib.ai.ollama_client.generate') as mock_generate:
        mock_generate.side_effect = session.replay_interaction

        results = []
        for email in emails:
            result = await process_email_complete(email)
            results.append(result)

    # Validate recorded session produced expected results
    assert len(results) == len(emails)
    assert all(r.success for r in results)

    # Validate extracted entities match recorded session
    entities = [r.extracted_entities for r in results]
    assert any('Atlas Integration' in e.projects for e in entities)
    assert any('James Brown' in e.people for e in entities)
```

## Mock Response Quality Validation

### Response Realism Scoring

```python
class MockQualityValidator:
    def validate_response_quality(self, mock_response: str, expected_type: str) -> float:
        """Score mock response quality from 0.0 to 1.0"""
        score = 0.0

        # Length appropriateness
        if expected_type == 'summary':
            if 50 <= len(mock_response) <= 200:
                score += 0.2
        elif expected_type == 'entity_extraction':
            if len(mock_response) > 20:  # Non-trivial extraction
                score += 0.2

        # Business language appropriateness
        business_terms = ['project', 'timeline', 'decision', 'team', 'deadline', 'status']
        if any(term in mock_response.lower() for term in business_terms):
            score += 0.3

        # Format validation for structured responses
        if expected_type == 'entity_extraction':
            try:
                parsed = json.loads(mock_response)
                if all(key in parsed for key in ['people', 'projects']):
                    score += 0.3
                if all(isinstance(parsed[key], list) for key in parsed):
                    score += 0.2
            except json.JSONDecodeError:
                pass  # No penalty for non-JSON responses

        # Consistency check (same input should produce same output)
        return score

# Quality validation in tests
def test_mock_response_quality():
    validator = MockQualityValidator()
    mock_server = OllamaMockServer()

    # Test summary quality
    summary_response = mock_server._deterministic_response(
        'llama-3.1-8b',
        'Summarize this email about Atlas project timeline concerns'
    )

    quality_score = validator.validate_response_quality(
        summary_response['response'],
        'summary'
    )

    assert quality_score > 0.7  # Minimum quality threshold
```

### Mock Performance Validation

```python
@pytest.mark.performance
class TestMockPerformance:
    async def test_ollama_mock_performance(self, benchmark_timer):
        """Ensure Ollama mocks meet <100ms target"""
        mock_server = OllamaMockServer()

        timer = benchmark_timer()
        timer.start()

        response = await mock_server.generate(
            'llama-3.1-8b',
            'Extract entities from this business email content'
        )

        timer.stop()

        assert timer.elapsed_ms < 100
        assert response is not None
        assert 'response' in response

    async def test_cloud_api_mock_performance(self, benchmark_timer):
        """Ensure cloud API mocks meet <100ms target"""
        mock_api = CloudAPIMock('gemini')

        timer = benchmark_timer()
        timer.start()

        response = await mock_api.generate_content(
            'Categorize this email as project update, escalation, or meeting coordination'
        )

        timer.stop()

        assert timer.elapsed_ms < 100
        assert response.content is not None

    async def test_mock_concurrent_performance(self):
        """Test mock performance under concurrent load"""
        mock_server = OllamaMockServer()

        async def single_request():
            return await mock_server.generate('llama-3.1-8b', 'Test prompt')

        # Run 50 concurrent requests
        tasks = [single_request() for _ in range(50)]

        start_time = time.time()
        results = await asyncio.gather(*tasks)
        total_time = time.time() - start_time

        # Should complete 50 requests in under 1 second
        assert total_time < 1.0
        assert len(results) == 50
        assert all(r is not None for r in results)
```

## Configuration and Environment Setup

### Test Environment Configuration

```python
# test_config.py
TEST_AI_CONFIG = {
    'unit': {
        'mode': 'full_mock',
        'response_source': 'pattern_library',
        'performance_target_ms': 100,
        'deterministic': True
    },
    'integration': {
        'mode': 'lightweight_models',
        'models': LIGHTWEIGHT_MODELS,
        'performance_target_ms': 10000,
        'ollama_options': {
            'temperature': 0.1,
            'top_p': 0.8,
            'top_k': 20
        }
    },
    'e2e': {
        'mode': 'recorded_responses',
        'response_source': 'test-data/ai-responses',
        'fallback_mode': 'lightweight_models',
        'performance_target_ms': 30000
    }
}

def get_ai_config(test_type: str) -> dict:
    return TEST_AI_CONFIG.get(test_type, TEST_AI_CONFIG['unit'])
```

### Environment Variables

```bash
# AI mocking behavior
export AI_MOCK_MODE=deterministic     # deterministic, lightweight, recorded
export AI_MOCK_DEBUG=1                # Enable debug logging
export AI_RESPONSE_CACHE_DIR=./test-data/ai-responses

# Performance controls
export AI_MOCK_LATENCY_MS=0           # Add artificial latency
export AI_MOCK_FAILURE_RATE=0.0       # Simulate failures (0.0-1.0)

# Model configuration
export OLLAMA_HOST=http://localhost:11434
export LIGHTWEIGHT_MODEL_TIMEOUT=10   # Seconds
```

This comprehensive AI mocking strategy ensures fast, reliable tests while maintaining realistic AI behavior for thorough validation of AI-first applications.