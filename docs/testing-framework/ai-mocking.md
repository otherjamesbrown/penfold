# AI Model Mocking Strategies

Go-based AI mocking framework for testing Penfold's AI-first architecture using testify/mock.

## Mocking Strategy Overview

### Two-Tiered Approach

| Test Type | AI Strategy | Performance Target | Use Case |
|-----------|-------------|-------------------|----------|
| Unit | Full Mocking (testify/mock) | <100ms | Component isolation |
| E2E | Real LLM (vLLM-MLX) | <30s per test | Complete workflow validation |

## Unit Test Mocking (Full Mocking)

### testify/mock Pattern

```go
// pkg/ai/llm_client.go - Define interface
type LLMClient interface {
    Complete(ctx context.Context, prompt string) (string, error)
    CompleteWithSystem(ctx context.Context, system, prompt string) (string, error)
}

// Mock implementation for tests
type MockLLMClient struct {
    mock.Mock
}

func (m *MockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
    args := m.Called(ctx, prompt)
    return args.String(0), args.Error(1)
}

func (m *MockLLMClient) CompleteWithSystem(ctx context.Context, system, prompt string) (string, error) {
    args := m.Called(ctx, system, prompt)
    return args.String(0), args.Error(1)
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

## E2E Testing with Real LLM

### LLM Client for E2E Tests

```go
// tests/e2e/llm_client.go
type LLMClient struct {
    baseURL    string
    httpClient *http.Client
}

func NewLLMClient(baseURL string) *LLMClient {
    return &LLMClient{
        baseURL:    baseURL,
        httpClient: &http.Client{Timeout: 60 * time.Second},
    }
}

func (c *LLMClient) CompleteWithSystem(ctx context.Context, system, prompt string) (string, error) {
    reqBody := map[string]any{
        "model": "qwen2.5-32b-instruct",
        "messages": []map[string]string{
            {"role": "system", "content": system},
            {"role": "user", "content": prompt},
        },
        "temperature": 0.0,  // More deterministic for tests
        "max_tokens":  1000,
    }

    body, _ := json.Marshal(reqBody)
    req, _ := http.NewRequestWithContext(ctx, "POST",
        c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("LLM request failed: %w", err)
    }
    defer resp.Body.Close()

    // Parse response...
    return content, nil
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
| Unit | testify/mock | Fast, deterministic component tests |
| Integration | testify/mock | Database + component interaction |
| E2E | Real LLM (vLLM-MLX) | Full workflow validation |
| Live | Real Cloud APIs | API connectivity verification |

Key principles:
- Use mocks for speed and determinism in unit tests
- Use real LLM with semantic assertions for E2E tests
- Skip gracefully when prerequisites are missing
- Verify mock expectations are met
- Keep mock responses realistic
