# AI Model Mocking Strategies

Go-based AI mocking framework for testing Penfold's AI-first architecture using testify/mock and interface-based patterns.

## Mocking Strategy Overview

### Three-Tiered Approach

| Test Type | AI Strategy | Performance Target | Use Case |
|-----------|-------------|-------------------|----------|
| Unit | Full Mocking (testify/mock) | <100ms | Component isolation |
| Integration | Function-Based Mocks | <10s | Multi-component testing |
| E2E | Real LLM (Gemini) or Real Embeddings (Ollama) | <30s per test | Complete workflow validation |

### Mocking Patterns in Penfold

The codebase uses two primary mocking approaches:

1. **testify/mock pattern** - For complex interfaces requiring expectation verification
2. **Function-based mocks** - For simpler interfaces with configurable behavior

## Unit Test Mocking (Full Mocking)

### testify/mock Pattern (LLM Provider)

This pattern is used for complex interfaces that require expectation verification. From `pkg/mentions/resolver/resolver_test.go`:

```go
import (
    "context"
    "github.com/stretchr/testify/mock"
)

// MockLLMProvider implements LLMProvider for testing.
type MockLLMProvider struct {
    mock.Mock
}

func (m *MockLLMProvider) Name() string {
    args := m.Called()
    return args.String(0)
}

func (m *MockLLMProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    args := m.Called(ctx, req)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*CompletionResponse), args.Error(1)
}

func (m *MockLLMProvider) CompleteStructured(ctx context.Context, req CompletionRequest, target interface{}) error {
    args := m.Called(ctx, req, target)
    return args.Error(0)
}

func (m *MockLLMProvider) IsAvailable(ctx context.Context) bool {
    args := m.Called(ctx)
    return args.Bool(0)
}

func (m *MockLLMProvider) Close() error {
    args := m.Called()
    return args.Error(0)
}
```

### Function-Based Mock Pattern (AI Client)

For simpler mocking needs, use function-based mocks. From `services/content/entity/extractor_test.go`:

```go
// MockAIClient is a mock implementation of the AIClient interface.
type MockAIClient struct {
    CompleteFunc func(ctx context.Context, prompt string, opts *CompletionOptions) (*CompletionResponse, error)
}

func (m *MockAIClient) Complete(ctx context.Context, prompt string, opts *CompletionOptions) (*CompletionResponse, error) {
    if m.CompleteFunc != nil {
        return m.CompleteFunc(ctx, prompt, opts)
    }
    // Return empty response by default
    return &CompletionResponse{
        Text:  "[]",
        Model: "mock-model",
    }, nil
}
```

### Pattern-Based Response Generation

```go
// Deterministic responses based on prompt patterns
func TestEntityExtraction(t *testing.T) {
    mockLLM := new(MockLLMClient)

    // Match prompts containing "extract" and return structured JSON
    mockLLM.On("Complete", mock.Anything, mock.MatchedBy(func(prompt string) bool {
        return strings.Contains(strings.ToLower(prompt), "extract")
    })).Return(`{
        "people": ["James Brown", "Sarah Chen"],
        "projects": ["Atlas Integration"],
        "decisions": ["Delay timeline by 1 week"]
    }`, nil)

    processor := NewEntityProcessor(mockLLM)
    result, err := processor.Extract(ctx, "Email about Atlas project timeline")

    assert.NoError(t, err)
    assert.Contains(t, result.Projects, "Atlas Integration")
    assert.Contains(t, result.People, "James Brown")
    mockLLM.AssertExpectations(t)
}
```

### Summarization Mocking

```go
func TestSummarization(t *testing.T) {
    mockLLM := new(MockLLMClient)

    // Return consistent summary format
    mockLLM.On("CompleteWithSystem", mock.Anything,
        mock.MatchedBy(func(s string) bool { return strings.Contains(s, "summarize") }),
        mock.Anything,
    ).Return("Summary: Atlas project faces timeline concerns due to resource constraints.", nil)

    summarizer := NewSummarizer(mockLLM)
    result, err := summarizer.Summarize(ctx, longEmailContent)

    assert.NoError(t, err)
    assert.Contains(t, result, "Atlas project")
}
```

### Mention Resolution Mocking

```go
func TestMentionResolution(t *testing.T) {
    mockLLM := new(MockLLMClient)

    // JSON response for mention resolution
    mockLLM.On("CompleteWithSystem", mock.Anything, mock.Anything, mock.Anything).
        Return(`{
            "person_id": 1,
            "canonical_name": "John Smith",
            "confidence": 0.95,
            "reasoning": "Exact match on first name 'John'"
        }`, nil)

    resolver := NewMentionResolver(mockLLM, peopleContext)
    result, err := resolver.Resolve(ctx, "John mentioned the deadline")

    assert.NoError(t, err)
    assert.Equal(t, int64(1), result.PersonID)
    assert.Equal(t, 0.95, result.Confidence)
}
```

### Embedding Client Mocking

The embedding client uses a built-in mock. From `pkg/embeddings/client.go`:

```go
// MockClient is a mock implementation of the Client interface for testing.
type MockClient struct {
    embedFunc      func(ctx context.Context, text string) ([]float32, error)
    batchEmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
    dimensions     int
    modelInfo      *ModelInfo
}

// NewMockClient creates a new mock embedding client.
func NewMockClient(dimensions int, embedFunc func(ctx context.Context, text string) ([]float32, error)) *MockClient {
    return &MockClient{
        embedFunc:  embedFunc,
        dimensions: dimensions,
        modelInfo: &ModelInfo{
            Name:        "mock-model",
            Dimensions:  dimensions,
            MaxTokens:   512,
            Provider:    "mock",
            IsLocal:     true,
            Description: "Mock embedding model for testing",
        },
    }
}

func (m *MockClient) Embed(ctx context.Context, text string) ([]float32, error) {
    if m.embedFunc != nil {
        return m.embedFunc(ctx, text)
    }
    // Default: return zero vector
    return make([]float32, m.dimensions), nil
}
```

Usage example:

```go
func TestSearchWithEmbeddings(t *testing.T) {
    // Create mock that returns predictable embeddings
    mockClient := embeddings.NewMockClient(1024, func(ctx context.Context, text string) ([]float32, error) {
        // Return consistent embedding based on input
        if strings.Contains(text, "project") {
            return projectEmbedding, nil
        }
        return defaultEmbedding, nil
    })

    searcher := NewSearcher(mockClient)
    results, err := searcher.Search(ctx, "project timeline")

    require.NoError(t, err)
    assert.NotEmpty(t, results)
}
```

## Temporal Workflow Mocking

For Temporal workflows that invoke AI activities, use the Temporal test suite with mock activities. From `services/worker/workflows/content_test.go`:

### Mock Activity Struct

```go
import (
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/suite"
    "go.temporal.io/sdk/activity"
    "go.temporal.io/sdk/testsuite"
)

// ContentIngestionMockActivities provides mock implementations for content ingestion activities.
type ContentIngestionMockActivities struct {
    mock.Mock
}

func (m *ContentIngestionMockActivities) GenerateContentEmbedding(ctx context.Context, input GenerateEmbeddingInput) (int64, error) {
    args := m.Called(ctx, input)
    return args.Get(0).(int64), args.Error(1)
}

func (m *ContentIngestionMockActivities) GenerateContentSummary(ctx context.Context, input GenerateSummaryInput) (int64, error) {
    args := m.Called(ctx, input)
    return args.Get(0).(int64), args.Error(1)
}

func (m *ContentIngestionMockActivities) ExtractEntities(ctx context.Context, input ExtractEntitiesInput) (int, error) {
    args := m.Called(ctx, input)
    return args.Get(0).(int), args.Error(1)
}
```

### Workflow Test Suite

```go
type ContentIngestionWorkflowTestSuite struct {
    suite.Suite
    testsuite.WorkflowTestSuite

    env        *testsuite.TestWorkflowEnvironment
    activities *ContentIngestionMockActivities
}

func (s *ContentIngestionWorkflowTestSuite) SetupTest() {
    s.env = s.NewTestWorkflowEnvironment()
    s.activities = &ContentIngestionMockActivities{}

    // Register mock activities
    s.env.RegisterActivityWithOptions(s.activities.GenerateContentEmbedding, activity.RegisterOptions{
        Name: "GenerateContentEmbedding",
    })
    s.env.RegisterActivityWithOptions(s.activities.GenerateContentSummary, activity.RegisterOptions{
        Name: "GenerateContentSummary",
    })
}

func (s *ContentIngestionWorkflowTestSuite) AfterTest(suiteName, testName string) {
    s.env.AssertExpectations(s.T())
}
```

### Testing AI Workflow Success

```go
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_Success() {
    // Arrange - mock AI activities
    s.activities.On("GenerateContentEmbedding", mock.Anything, mock.MatchedBy(func(input GenerateEmbeddingInput) bool {
        return input.TenantID == "tenant-123" && input.SourceID == 456
    })).Return(int64(1001), nil)

    s.activities.On("GenerateContentSummary", mock.Anything, mock.MatchedBy(func(input GenerateSummaryInput) bool {
        return input.TenantID == "tenant-123" && input.SourceID == 456
    })).Return(int64(2001), nil)

    s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(5, nil)

    // Act
    s.env.ExecuteWorkflow(ContentIngestionWorkflow, pkgtemporal.ContentIngestionInput{
        TenantID: "tenant-123",
        SourceID: 456,
    })

    // Assert
    require.True(s.T(), s.env.IsWorkflowCompleted())
    require.NoError(s.T(), s.env.GetWorkflowError())

    var result ContentIngestionResult
    require.NoError(s.T(), s.env.GetWorkflowResult(&result))
    s.Equal(int64(1001), *result.EmbeddingID)
    s.Equal(5, result.EntityCount)
}
```

### Testing AI Activity Failures (Graceful Degradation)

```go
func (s *ContentIngestionWorkflowTestSuite) TestContentIngestionWorkflow_EmbeddingFailsContinues() {
    // Embedding fails but workflow continues with other activities
    s.activities.On("GenerateContentEmbedding", mock.Anything, mock.Anything).Return(
        int64(0),
        temporal.NewApplicationError("embedding service unavailable", "ServiceUnavailable"),
    )

    // Other activities succeed
    s.activities.On("GenerateContentSummary", mock.Anything, mock.Anything).Return(int64(200), nil)
    s.activities.On("ExtractEntities", mock.Anything, mock.Anything).Return(3, nil)

    s.env.ExecuteWorkflow(ContentIngestionWorkflow, input)

    // Workflow completes successfully despite embedding failure
    require.True(s.T(), s.env.IsWorkflowCompleted())
    require.NoError(s.T(), s.env.GetWorkflowError())

    var result ContentIngestionResult
    require.NoError(s.T(), s.env.GetWorkflowResult(&result))
    s.Equal("completed", result.Status)
    s.Nil(result.EmbeddingID)  // Embedding failed but workflow continued
}
```

## Error Handling Mocks

### Simulating LLM Failures

```go
func TestLLMFailure(t *testing.T) {
    mockLLM := new(MockLLMClient)

    // Simulate timeout
    mockLLM.On("Complete", mock.Anything, mock.Anything).
        Return("", errors.New("context deadline exceeded"))

    processor := NewProcessor(mockLLM)
    _, err := processor.Process(ctx, "some input")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestLLMRetry(t *testing.T) {
    mockLLM := new(MockLLMClient)

    // First call fails, second succeeds
    mockLLM.On("Complete", mock.Anything, mock.Anything).
        Return("", errors.New("temporary error")).Once()
    mockLLM.On("Complete", mock.Anything, mock.Anything).
        Return("success response", nil).Once()

    processor := NewProcessorWithRetry(mockLLM, 3)
    result, err := processor.Process(ctx, "some input")

    assert.NoError(t, err)
    assert.Equal(t, "success response", result)
}
```

### Invalid Response Handling

```go
func TestInvalidJSON(t *testing.T) {
    mockLLM := new(MockLLMClient)

    // Return malformed JSON
    mockLLM.On("CompleteWithSystem", mock.Anything, mock.Anything, mock.Anything).
        Return("not valid json {", nil)

    resolver := NewMentionResolver(mockLLM, peopleContext)
    _, err := resolver.Resolve(ctx, "John mentioned the deadline")

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "failed to parse")
}
```

## Service Mock Patterns

### Embedding Cache Mocking

For testing embedding cache behavior. From `pkg/embeddings/cache_test.go`:

```go
// MockRedisClient implements RedisClient for testing
type MockRedisClient struct {
    data      map[string][]byte
    getErr    error
    setErr    error
    delErr    error
    flushErr  error
    dbSizeErr error
}

func NewMockRedisClient() *MockRedisClient {
    return &MockRedisClient{
        data: make(map[string][]byte),
    }
}

func (m *MockRedisClient) Get(ctx context.Context, key string) ([]byte, error) {
    if m.getErr != nil {
        return nil, m.getErr
    }
    if data, ok := m.data[key]; ok {
        return data, nil
    }
    return nil, errors.New("key not found")
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    if m.setErr != nil {
        return m.setErr
    }
    m.data[key] = value
    return nil
}
```

Usage:

```go
func TestRedisCache_GetSet(t *testing.T) {
    client := NewMockRedisClient()
    cache, _ := NewRedisCache(nil, client)
    ctx := context.Background()

    // Test cache miss
    _, err := cache.Get(ctx, "nonexistent")
    require.True(t, errors.Is(err, ErrCacheMiss))

    // Test cache hit
    embedding := []float32{1.0, 2.0, 3.0}
    err = cache.Set(ctx, "test", embedding)
    require.NoError(t, err)

    result, err := cache.Get(ctx, "test")
    require.NoError(t, err)
    require.Equal(t, len(embedding), len(result))
}
```

### LLM Service Mock (Activities)

For Temporal activities that call LLM services. From `services/worker/activities/activities_test.go`:

```go
// MockLLMService is a mock implementation for testing.
type MockLLMService struct {
    SummarizeFunc func(ctx context.Context, content string) (int64, error)
    ExtractFunc   func(ctx context.Context, content string) (int, error)
}

func (m *MockLLMService) Summarize(ctx context.Context, content string) (int64, error) {
    if m.SummarizeFunc != nil {
        return m.SummarizeFunc(ctx, content)
    }
    return 0, errors.New("SummarizeFunc not set")
}

func (m *MockLLMService) Extract(ctx context.Context, content string) (int, error) {
    if m.ExtractFunc != nil {
        return m.ExtractFunc(ctx, content)
    }
    return 0, errors.New("ExtractFunc not set")
}
```

Usage:

```go
func TestGenerateSummary_WithMock_Success(t *testing.T) {
    mockService := &MockLLMService{
        SummarizeFunc: func(ctx context.Context, content string) (int64, error) {
            return 100, nil
        },
    }

    summaryID, err := mockService.Summarize(context.Background(), "test content")
    require.NoError(t, err)
    require.Equal(t, int64(100), summaryID)
}
```

## E2E Testing with Real LLM

### LLM Client for E2E Tests

```go
// tests/e2e/llm_client.go
type LLMClient struct {
    baseURL string
    client  *http.Client
}

func NewLLMClient(baseURL string) *LLMClient {
    return &LLMClient{
        baseURL: baseURL,
        client: &http.Client{
            Timeout: 300 * time.Second, // LLM calls can be slow
        },
    }
}

// Chat sends a chat completion request to the LLM.
func (c *LLMClient) Chat(ctx context.Context, messages []Message) (string, error) {
    req := ChatCompletionRequest{
        Model:       "gemini-2.0-flash",
        Messages:    messages,
        Temperature: 0.0, // Deterministic for testing
        MaxTokens:   2048,
    }
    // ... make request and parse response
}

// Complete is a simpler interface for single-turn completion.
func (c *LLMClient) Complete(ctx context.Context, prompt string) (string, error) {
    return c.Chat(ctx, []Message{
        {Role: "user", Content: prompt},
    })
}

// CompleteWithSystem sends a prompt with a system message.
func (c *LLMClient) CompleteWithSystem(ctx context.Context, system, prompt string) (string, error) {
    return c.Chat(ctx, []Message{
        {Role: "system", Content: system},
        {Role: "user", Content: prompt},
    })
}
```

### Semantic Assertions for LLM Output

```go
// tests/e2e/assertions.go
type ResolvedMention struct {
    PersonID      int64   `json:"person_id"`
    CanonicalName string  `json:"canonical_name"`
    Confidence    float64 `json:"confidence"`
}

// AssertMentionResolved verifies LLM resolved a mention correctly
func AssertMentionResolved(t *testing.T, response string, expectedName string) {
    t.Helper()

    var result ResolvedMention
    err := json.Unmarshal([]byte(response), &result)
    if err != nil {
        // Try extracting from text response
        assert.Contains(t, response, expectedName, "response should mention the person")
        return
    }

    assert.Contains(t, result.CanonicalName, expectedName)
    assert.Greater(t, result.Confidence, 0.5, "confidence should be reasonable")
}
```

### E2E Test Example

```go
//go:build e2e

func TestMentionResolutionWithRealLLM(t *testing.T) {
    env := SetupE2EEnvironment(t)

    // Load fixtures for context
    err := env.LoadFixture("acme-corp")
    require.NoError(t, err)

    client := NewLLMClient(env.LLMURL)
    ctx := context.Background()

    // Build context from fixtures
    peopleContext := buildPeopleContext(t, env)

    response, err := client.CompleteWithSystem(ctx,
        mentionResolutionSystemPrompt,
        fmt.Sprintf(`Text: "John mentioned the timeline concerns."

Available people:
%s

Identify who "John" refers to. Return JSON.`, peopleContext),
    )
    require.NoError(t, err)

    // Semantic assertion - LLM output is non-deterministic
    AssertMentionResolved(t, response, "John Smith")
}
```

## Cloud API Testing (Live Tests)

### Gemini API Test

```go
//go:build live

func TestGeminiAPIConnection(t *testing.T) {
    apiKey := RequireGeminiAPIKey(t)

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    url := fmt.Sprintf(
        "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s",
        apiKey,
    )

    reqBody := map[string]any{
        "contents": []map[string]any{
            {"parts": []map[string]string{{"text": "What is 2 + 2?"}}},
        },
        "generationConfig": map[string]any{
            "temperature":    0.0,
            "maxOutputTokens": 100,
        },
    }

    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()

    require.Equal(t, http.StatusOK, resp.StatusCode)

    // Parse and verify response contains "4"
    // ...
}
```

### Embeddings API Test

```go
//go:build live

func TestGeminiEmbeddings(t *testing.T) {
    apiKey := RequireGeminiAPIKey(t)

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    url := fmt.Sprintf(
        "https://generativelanguage.googleapis.com/v1beta/models/embedding-001:embedContent?key=%s",
        apiKey,
    )

    reqBody := map[string]any{
        "model": "models/embedding-001",
        "content": map[string]any{
            "parts": []map[string]string{
                {"text": "This is a test document for embedding generation."},
            },
        },
    }

    // ... make request ...

    // Verify embedding dimensions
    assert.Equal(t, 768, len(result.Embedding.Values), "embedding-001 returns 768 dimensions")
}
```

## Mock Quality Best Practices

### Response Realism

```go
// Good: Realistic mock response
mockLLM.On("CompleteWithSystem", mock.Anything, mock.Anything, mock.Anything).
    Return(`{
        "person_id": 1,
        "canonical_name": "John Smith",
        "confidence": 0.92,
        "reasoning": "First name match with high confidence"
    }`, nil)

// Bad: Oversimplified response
mockLLM.On("Complete", mock.Anything, mock.Anything).
    Return("John Smith", nil)  // Missing structure
```

### Test Isolation

```go
func TestFeatureA(t *testing.T) {
    mockLLM := new(MockLLMClient)  // Fresh mock per test
    // ...
}

func TestFeatureB(t *testing.T) {
    mockLLM := new(MockLLMClient)  // Fresh mock per test
    // ...
}
```

### Verification

```go
func TestAllExpectationsMet(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.Anything).Return("response", nil)

    // ... use mock ...

    // Verify all expected calls were made
    mockLLM.AssertExpectations(t)
}

func TestCallCount(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.Anything).Return("response", nil)

    // ... use mock multiple times ...

    // Verify exact call count
    mockLLM.AssertNumberOfCalls(t, "Complete", 3)
}
```

## Performance Validation

```go
func TestMockPerformance(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.Anything).Return("response", nil)

    start := time.Now()

    for i := 0; i < 1000; i++ {
        _, _ = mockLLM.Complete(context.Background(), "test prompt")
    }

    elapsed := time.Since(start)

    // 1000 mock calls should complete in under 100ms
    assert.Less(t, elapsed.Milliseconds(), int64(100),
        "mock performance should be fast")
}
```

## Environment Configuration

### Test Helper for LLM Availability

```go
// tests/e2e/helpers.go
func (e *E2EEnv) LLMAvailable() bool {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, "GET", e.LLMURL+"/v1/models", nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return false
    }
    defer resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}
```

### Graceful Skipping

```go
func SetupE2EEnvironment(t *testing.T) *E2EEnv {
    t.Helper()

    llmURL := os.Getenv("LLM_URL")
    if llmURL == "" {
        llmURL = "http://localhost:8080"
    }

    env := &E2EEnv{LLMURL: llmURL}

    if !env.LLMAvailable() {
        t.Skip("Local LLM not available - skipping E2E test")
    }

    return env
}
```

### Live Test Prerequisites

```go
func RequireGeminiAPIKey(t *testing.T) string {
    t.Helper()

    key := os.Getenv("GEMINI_API_KEY")
    if key == "" {
        t.Skip("GEMINI_API_KEY not set - skipping live test")
    }
    return key
}
```

## Summary

| Test Type | Mock Strategy | When to Use |
|-----------|--------------|-------------|
| Unit | testify/mock or Function-based | Fast, deterministic component tests |
| Integration | testify/mock with DB | Database + component interaction |
| Workflow | Temporal TestSuite + Mock Activities | Temporal workflow testing |
| E2E | Real Embeddings (Ollama) or Real LLM (Gemini) | Full workflow validation |
| Live | Real Cloud APIs | API connectivity verification |

### Mock Pattern Decision Guide

| Pattern | When to Use | Example |
|---------|-------------|---------|
| testify/mock | Complex interfaces, expectation verification | `MockLLMProvider` |
| Function-based | Simple interfaces, behavior injection | `MockAIClient`, `MockLLMService` |
| Built-in mock | Library-provided mocks | `embeddings.NewMockClient()` |
| Temporal TestSuite | Workflow testing | `testsuite.TestWorkflowEnvironment` |

### Key Principles

1. **Use mocks for speed and determinism** in unit tests
2. **Use function-based mocks** for simple, configurable behavior
3. **Use testify/mock** when you need expectation verification
4. **Use real LLM with semantic assertions** for E2E tests
5. **Test graceful degradation** - AI failures should not crash workflows
6. **Skip gracefully** when prerequisites are missing
7. **Keep mock responses realistic** - match expected JSON structures

### File Locations

| Component | Mock Location |
|-----------|---------------|
| LLM Provider | `pkg/mentions/resolver/resolver_test.go` |
| AI Client | `services/content/entity/extractor_test.go` |
| Embedding Client | `pkg/embeddings/client.go` (built-in) |
| Redis Cache | `pkg/embeddings/cache_test.go` |
| Workflow Activities | `services/worker/workflows/content_test.go` |
| Escalation | `services/ai/escalation/escalation_test.go` |
| E2E LLM Client | `tests/e2e/llm_client.go` |
