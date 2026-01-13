# AI Model Mocking Framework

## Overview

AI-first development requires comprehensive mocking strategies to ensure tests are fast, deterministic, and don't depend on expensive model execution. This framework provides multiple mocking approaches for different testing scenarios.

## Mocking Strategy by Test Type

### Unit Tests: Full Mocking
**Goal**: Fast, deterministic, isolated component testing
**Approach**: Mock all AI interactions with pre-defined responses
**Performance Target**: <100ms per test

### Integration Tests: Lightweight Models
**Goal**: Realistic AI behavior without performance/cost penalties
**Approach**: Use fast, small models (Phi-3 Mini) instead of production models
**Performance Target**: <10s per test

### End-to-End Tests: Record/Replay
**Goal**: Real AI behavior for critical workflows
**Approach**: Record production AI responses, replay in tests
**Performance Target**: <30s per test

## Mock Architecture Components

### 1. Ollama API Mock Server

```python
# Mock Ollama local API for unit tests
class OllamaMockServer:
    def __init__(self, mode: str = 'deterministic'):
        self.mode = mode
        self.response_library = ResponseLibrary()

    async def generate(self, model: str, prompt: str, **kwargs) -> dict:
        """Mock Ollama generate endpoint with deterministic responses"""
        if self.mode == 'deterministic':
            return self._deterministic_response(model, prompt)
        elif self.mode == 'fuzzy':
            return self._fuzzy_response(model, prompt)
        else:
            return self._recorded_response(model, prompt)

    def _deterministic_response(self, model: str, prompt: str) -> dict:
        """Generate consistent response based on prompt patterns"""
        prompt_hash = hash(prompt + model)

        # Pattern-based responses for common tasks
        if 'summarize' in prompt.lower():
            return {
                'response': f'Summary: [Mock summary for {prompt[:50]}...]',
                'model': model,
                'created_at': '2024-12-01T10:00:00Z',
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
        elif 'categorize' in prompt.lower():
            return {
                'response': json.dumps({
                    'category': 'project_update',
                    'confidence': 0.95,
                    'reasoning': 'Contains project status and timeline information'
                }),
                'model': model,
                'done': True
            }

        # Default response for unmatched patterns
        return {
            'response': f'[Mock response for {model}]',
            'model': model,
            'done': True
        }
```

### 2. Cloud API Mocking Framework

```python
# Mock cloud APIs (Gemini, Claude, GPT) with response caching
class CloudAPIMock:
    def __init__(self, provider: str, mode: str = 'replay'):
        self.provider = provider
        self.mode = mode
        self.response_cache = ResponseCache(provider)

    async def generate_content(self, prompt: str, **kwargs) -> AIResponse:
        """Mock cloud API with cached responses"""
        cache_key = self._generate_cache_key(prompt, kwargs)

        if self.mode == 'replay':
            cached_response = self.response_cache.get(cache_key)
            if cached_response:
                return cached_response
            else:
                return self._generate_fallback_response(prompt)

        elif self.mode == 'record':
            # In record mode, make actual API call and cache result
            real_response = await self._call_real_api(prompt, **kwargs)
            self.response_cache.store(cache_key, real_response)
            return real_response

        else:  # generate mode
            return self._generate_mock_response(prompt, **kwargs)

    def _generate_fallback_response(self, prompt: str) -> AIResponse:
        """Generate reasonable mock response when no cached response available"""
        return AIResponse(
            content=f"[Mock {self.provider} response for: {prompt[:100]}...]",
            usage={'input_tokens': len(prompt.split()), 'output_tokens': 50},
            model=f'{self.provider}-mock',
            finish_reason='stop'
        )
```

### 3. Response Recording Infrastructure

```python
# System for recording real AI responses for later replay
class AIResponseRecorder:
    def __init__(self, storage_path: str = './test-data/ai-responses'):
        self.storage_path = Path(storage_path)
        self.storage_path.mkdir(exist_ok=True)

    async def record_session(self, session_name: str, interactions: List[AIInteraction]):
        """Record a session of AI interactions for later replay"""
        session_file = self.storage_path / f"{session_name}.json"

        recorded_session = {
            'session_name': session_name,
            'recorded_at': datetime.utcnow().isoformat(),
            'interactions': []
        }

        for interaction in interactions:
            recorded_interaction = {
                'model': interaction.model,
                'prompt': interaction.prompt,
                'response': interaction.response,
                'metadata': interaction.metadata,
                'timestamp': interaction.timestamp
            }
            recorded_session['interactions'].append(recorded_interaction)

        with session_file.open('w') as f:
            json.dump(recorded_session, f, indent=2)

    async def replay_session(self, session_name: str) -> AISessionReplay:
        """Load recorded session for test replay"""
        session_file = self.storage_path / f"{session_name}.json"

        with session_file.open('r') as f:
            session_data = json.load(f)

        return AISessionReplay(session_data)

# Usage for creating test recordings
async def record_atlas_project_analysis():
    """Record real AI analysis of Atlas project for test replay"""
    recorder = AIResponseRecorder()
    interactions = []

    # Record email summarization
    email_content = load_test_email('atlas_timeline_concern')
    summary_response = await real_ollama_client.summarize(email_content)
    interactions.append(AIInteraction(
        model='llama-3.1-8b',
        prompt=f'Summarize this email: {email_content}',
        response=summary_response
    ))

    # Record entity extraction
    entities_response = await real_ollama_client.extract_entities(email_content)
    interactions.append(AIInteraction(
        model='llama-3.1-8b',
        prompt=f'Extract people, projects, decisions from: {email_content}',
        response=entities_response
    ))

    await recorder.record_session('atlas_project_analysis', interactions)
```

### 4. Lightweight Model Strategy

```python
# Use fast models for integration testing instead of mocking
class LightweightModelStrategy:
    def __init__(self):
        self.fast_models = {
            'summarization': 'phi-3-mini-3.8b',  # Fast, decent quality
            'entity_extraction': 'qwen2.5-7b',   # Good at structured tasks
            'categorization': 'llama-3.2-3b',    # Fast classification
            'embedding': 'nomic-embed-text'       # Consistent embeddings
        }

    async def process_with_lightweight_model(self, task: str, content: str) -> AIResponse:
        """Process with fast model appropriate for the task"""
        model = self.fast_models.get(task, 'phi-3-mini-3.8b')

        # Use actual model but with optimized parameters for speed
        return await ollama_client.generate(
            model=model,
            prompt=content,
            options={
                'temperature': 0.1,  # More deterministic
                'top_p': 0.8,        # Faster sampling
                'top_k': 20,         # Reduced search space
                'num_predict': 200   # Shorter responses
            }
        )
```

## Mock Response Library

### Structured Response Patterns

```python
# Library of realistic mock responses for different scenarios
class MockResponseLibrary:
    def __init__(self):
        self.patterns = {
            'email_summary': self._email_summary_patterns,
            'entity_extraction': self._entity_extraction_patterns,
            'categorization': self._categorization_patterns,
            'meeting_analysis': self._meeting_analysis_patterns
        }

    def _email_summary_patterns(self, email_content: str) -> str:
        """Generate realistic email summary based on content patterns"""
        if 'atlas' in email_content.lower():
            return "Atlas project timeline concerns raised by client. Engineering investigating delay causes. COO review scheduled."
        elif 'budget' in email_content.lower():
            return "Budget allocation discussion. Finance requesting reallocation justification. Department heads coordination needed."
        elif 'meeting' in email_content.lower():
            return "Meeting coordination for project checkpoint. Agenda items include status review and risk assessment."
        else:
            return f"Business communication requiring attention. Key stakeholders involved in decision process."

    def _entity_extraction_patterns(self, content: str) -> dict:
        """Extract entities with realistic patterns"""
        # Simple pattern matching for consistent test results
        entities = {
            'people': [],
            'projects': [],
            'decisions': [],
            'dates': [],
            'organizations': []
        }

        # Pattern-based entity detection
        if 'james brown' in content.lower():
            entities['people'].append('James Brown')
        if 'sarah chen' in content.lower():
            entities['people'].append('Sarah Chen')
        if 'atlas' in content.lower():
            entities['projects'].append('Atlas Integration')
        if 'delay' in content.lower() or 'postpone' in content.lower():
            entities['decisions'].append('Timeline delay')

        return entities

    def _categorization_patterns(self, content: str) -> dict:
        """Categorize content with confidence scores"""
        categories = {
            'project_update': 0.0,
            'escalation': 0.0,
            'meeting_coordination': 0.0,
            'decision_request': 0.0,
            'status_report': 0.0
        }

        # Pattern-based scoring
        if any(word in content.lower() for word in ['delayed', 'behind', 'concern', 'risk']):
            categories['escalation'] = 0.85
        elif any(word in content.lower() for word in ['meeting', 'schedule', 'agenda']):
            categories['meeting_coordination'] = 0.90
        elif any(word in content.lower() for word in ['approve', 'decision', 'authorize']):
            categories['decision_request'] = 0.88
        elif any(word in content.lower() for word in ['status', 'progress', 'milestone']):
            categories['project_update'] = 0.82

        # Find highest confidence category
        best_category = max(categories, key=categories.get)
        best_confidence = categories[best_category]

        return {
            'category': best_category,
            'confidence': best_confidence,
            'all_scores': categories,
            'reasoning': f'Content patterns indicate {best_category}'
        }
```

## Test Configuration Framework

### Environment-Specific Mocking

```yaml
# test-config.yml - Different mocking strategies per environment
test_environments:
  unit:
    ai_mocking_mode: "full_mock"
    response_source: "pattern_library"
    performance_target_ms: 100

  integration:
    ai_mocking_mode: "lightweight_models"
    models:
      summarization: "phi-3-mini-3.8b"
      entity_extraction: "qwen2.5-7b"
    performance_target_ms: 10000

  e2e:
    ai_mocking_mode: "recorded_responses"
    response_source: "test-data/ai-responses"
    fallback_mode: "lightweight_models"
    performance_target_ms: 30000

  load:
    ai_mocking_mode: "pattern_mock_with_delays"
    simulated_latency_ms: 500
    concurrent_limit: 50
```

### Test Fixtures with AI Mocking

```python
# pytest fixtures for different mocking strategies
@pytest.fixture
def mock_ai_full():
    """Full AI mocking for unit tests"""
    with patch('penf_lib.ai.ollama_client') as mock_ollama:
        with patch('penf_lib.ai.gemini_client') as mock_gemini:
            mock_ollama.generate = AsyncMock(side_effect=OllamaMockServer().generate)
            mock_gemini.generate_content = AsyncMock(side_effect=CloudAPIMock('gemini').generate_content)
            yield {'ollama': mock_ollama, 'gemini': mock_gemini}

@pytest.fixture
def lightweight_ai():
    """Use fast models for integration tests"""
    strategy = LightweightModelStrategy()
    with patch('penf_lib.ai.get_model_for_task') as mock_get_model:
        mock_get_model.side_effect = lambda task: strategy.fast_models.get(task)
        yield strategy

@pytest.fixture
def recorded_ai_responses():
    """Use pre-recorded AI responses for deterministic e2e tests"""
    recorder = AIResponseRecorder()
    replay_sessions = {
        'atlas_project': recorder.replay_session('atlas_project_analysis'),
        'meeting_analysis': recorder.replay_session('meeting_analysis_session'),
        'email_processing': recorder.replay_session('email_processing_batch')
    }
    yield replay_sessions
```

## Performance and Quality Validation

### Mock Performance Testing
```python
# Ensure mocks meet performance targets
class MockPerformanceValidator:
    async def validate_mock_performance(self, mock_type: str, target_ms: int):
        """Ensure mocks meet performance targets"""
        start_time = time.time()

        if mock_type == 'ollama_mock':
            await OllamaMockServer().generate('llama-3.1-8b', 'test prompt')
        elif mock_type == 'cloud_api_mock':
            await CloudAPIMock('gemini').generate_content('test prompt')

        elapsed_ms = (time.time() - start_time) * 1000
        assert elapsed_ms < target_ms, f"Mock took {elapsed_ms}ms, target was {target_ms}ms"

# Quality validation for mock responses
class MockQualityValidator:
    def validate_response_realism(self, mock_response: str, content_type: str) -> float:
        """Score mock response realism (0.0-1.0)"""
        quality_score = 0.0

        # Check for appropriate length
        if content_type == 'summary' and 50 <= len(mock_response) <= 200:
            quality_score += 0.3

        # Check for business-appropriate language
        if any(word in mock_response.lower() for word in ['project', 'timeline', 'decision', 'team']):
            quality_score += 0.3

        # Check for structured format (for entity extraction)
        if content_type == 'entity_extraction':
            try:
                parsed = json.loads(mock_response)
                if all(key in parsed for key in ['people', 'projects']):
                    quality_score += 0.4
            except json.JSONDecodeError:
                pass

        return quality_score
```

## Implementation Roadmap

### Week 1: Basic Mocking Infrastructure
- Implement OllamaMockServer with deterministic responses
- Create CloudAPIMock with fallback responses
- Set up basic test fixtures

### Week 2: Response Recording System
- Build AIResponseRecorder for capturing real interactions
- Create test sessions for major scenarios (Atlas project, meeting analysis)
- Implement replay functionality

### Week 3: Lightweight Model Integration
- Set up fast model configurations for integration tests
- Implement model selection strategy
- Performance tuning for test speed

### Week 4: Quality and Performance Validation
- Mock performance testing framework
- Response quality validation
- End-to-end testing with full mock integration

This mocking framework ensures fast, reliable tests while maintaining realistic AI behavior patterns for comprehensive validation.