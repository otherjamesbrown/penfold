package summarization

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// mockAIClient implements AIClient for testing.
type mockAIClient struct {
	generateResponse string
	generateErr      error
	tokenEstimate    int
	defaultModel     string
	callCount        int
}

func newMockAIClient(response string) *mockAIClient {
	return &mockAIClient{
		generateResponse: response,
		tokenEstimate:    100,
		defaultModel:     "test-model",
	}
}

func (m *mockAIClient) GenerateCompletion(ctx context.Context, prompt string, maxTokens int) (string, error) {
	m.callCount++
	if m.generateErr != nil {
		return "", m.generateErr
	}
	return m.generateResponse, nil
}

func (m *mockAIClient) GenerateCompletionWithModel(ctx context.Context, prompt string, maxTokens int, model string) (string, error) {
	return m.GenerateCompletion(ctx, prompt, maxTokens)
}

func (m *mockAIClient) GetDefaultModel() string {
	return m.defaultModel
}

func (m *mockAIClient) EstimateTokens(text string) int {
	return len(text) / 4
}

// testLogger returns a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelError, // Suppress logs during tests
		ServiceName: "summarization-test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// Sample content for testing.
const sampleContent = `The Go programming language was designed at Google in 2007 by Robert Griesemer, Rob Pike, and Ken Thompson.
Go is a statically typed, compiled language known for its simplicity and efficiency.
The language features garbage collection, structural typing, and CSP-style concurrency.
Go was publicly announced in November 2009 and version 1.0 was released in March 2012.
It has since become popular for building web services, cloud infrastructure, and command-line tools.
Companies like Google, Uber, Twitch, and Dropbox use Go extensively in their production systems.
The language emphasizes clean syntax, fast compilation, and built-in support for concurrent programming.
Go's standard library is comprehensive and includes packages for networking, cryptography, and testing.`

const shortContent = "Go is a programming language designed at Google."

const longContent = `The Go programming language, also known as Golang, is a statically typed, compiled programming language designed at Google by Robert Griesemer, Rob Pike, and Ken Thompson. Go was designed to address criticisms of other languages while maintaining their positive characteristics. It was designed with simplicity, efficiency, and readability in mind.

Go was publicly announced in November 2009, and version 1.0 was released in March 2012. The language was created to improve programming productivity in an era of multicore processors, networked systems, and large codebases. Go was designed to provide the efficiency of statically typed compiled languages while maintaining the ease of programming of dynamically typed interpreted languages.

One of Go's notable features is its built-in support for concurrent programming through goroutines and channels. Goroutines are lightweight threads managed by the Go runtime, allowing developers to create thousands of concurrent tasks with minimal overhead. Channels provide a way for goroutines to communicate and synchronize their execution.

Go includes garbage collection, which automatically manages memory allocation and deallocation. The garbage collector has been continuously improved over time to reduce pause times and improve overall performance. This feature eliminates many common memory-related bugs found in languages like C and C++.

The language has a simple and consistent syntax that emphasizes readability. Go does not support classes and inheritance in the traditional object-oriented sense, instead favoring composition over inheritance through interfaces and struct embedding. This design choice leads to more explicit and maintainable code.

Go's standard library is comprehensive and includes packages for networking, HTTP servers, JSON encoding, cryptography, and testing. The go command provides tools for building, testing, and managing Go code, creating a unified development experience without the need for external build tools.

Companies like Google, Uber, Twitch, Dropbox, and many others use Go in production for building scalable web services, cloud infrastructure, and distributed systems. The language's performance characteristics and built-in concurrency make it well-suited for these applications.

Go continues to evolve with regular releases adding new features and improvements. Recent versions have introduced generics, which allow writing more flexible and reusable code. The Go community is active and contributes to a rich ecosystem of third-party packages and tools.`

func TestSummaryRequest_SetDefaults(t *testing.T) {
	req := &SummaryRequest{
		Content: sampleContent,
	}

	req.SetDefaults()

	if req.Strategy != StrategyTypeHybrid {
		t.Errorf("Expected strategy %s, got %s", StrategyTypeHybrid, req.Strategy)
	}
	if req.Length != SummaryLengthParagraph {
		t.Errorf("Expected length %s, got %s", SummaryLengthParagraph, req.Length)
	}
	if req.Format != SummaryFormatProse {
		t.Errorf("Expected format %s, got %s", SummaryFormatProse, req.Format)
	}
}

func TestSummaryRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *SummaryRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     &SummaryRequest{Content: sampleContent},
			wantErr: false,
		},
		{
			name:    "empty content",
			req:     &SummaryRequest{Content: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSummaryRequest_TargetTokens(t *testing.T) {
	tests := []struct {
		name      string
		length    SummaryLength
		maxTokens int
		want      int
	}{
		{"one line", SummaryLengthOneLine, 0, 30},
		{"paragraph", SummaryLengthParagraph, 0, 100},
		{"full", SummaryLengthFull, 0, 300},
		{"custom max tokens", SummaryLengthParagraph, 50, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SummaryRequest{
				Length:    tt.length,
				MaxTokens: tt.maxTokens,
			}
			if got := req.TargetTokens(); got != tt.want {
				t.Errorf("TargetTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExtractiveStrategy_Summarize(t *testing.T) {
	strategy := NewExtractiveStrategy(testLogger())

	tests := []struct {
		name    string
		content string
		length  SummaryLength
	}{
		{"normal content", sampleContent, SummaryLengthParagraph},
		{"one line", sampleContent, SummaryLengthOneLine},
		{"full summary", sampleContent, SummaryLengthFull},
		{"short content", shortContent, SummaryLengthParagraph},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SummaryRequest{
				Content: tt.content,
				Length:  tt.length,
			}
			req.SetDefaults()

			ctx := context.Background()
			result, err := strategy.Summarize(ctx, req)

			if err != nil {
				t.Fatalf("Summarize() error = %v", err)
			}

			if result.Summary == "" {
				t.Error("Expected non-empty summary")
			}

			// Verify compression occurred for longer content
			if len(tt.content) > 200 && len(result.Summary) >= len(tt.content) {
				t.Error("Expected summary to be shorter than original content")
			}
		})
	}
}

func TestExtractiveStrategy_KeyPoints(t *testing.T) {
	strategy := NewExtractiveStrategy(testLogger())

	req := &SummaryRequest{
		Content: sampleContent,
		Length:  SummaryLengthParagraph,
	}
	req.SetDefaults()

	ctx := context.Background()
	result, err := strategy.Summarize(ctx, req)

	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	if len(result.KeyPoints) == 0 {
		t.Error("Expected key points to be extracted")
	}

	// Key points should be non-empty strings
	for i, kp := range result.KeyPoints {
		if kp == "" {
			t.Errorf("Key point %d is empty", i)
		}
	}
}

func TestAbstractiveStrategy_Summarize(t *testing.T) {
	aiClient := newMockAIClient("This is a summary of the Go programming language, designed at Google.")
	strategy := NewAbstractiveStrategy(aiClient, testLogger())

	req := &SummaryRequest{
		Content: sampleContent,
		Length:  SummaryLengthParagraph,
	}
	req.SetDefaults()

	ctx := context.Background()
	result, err := strategy.Summarize(ctx, req)

	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	if result.ModelUsed != "test-model" {
		t.Errorf("Expected model %s, got %s", "test-model", result.ModelUsed)
	}

	if aiClient.callCount != 1 {
		t.Errorf("Expected 1 AI call, got %d", aiClient.callCount)
	}
}

func TestAbstractiveStrategy_NoClient(t *testing.T) {
	strategy := NewAbstractiveStrategy(nil, testLogger())

	req := &SummaryRequest{
		Content: sampleContent,
	}
	req.SetDefaults()

	ctx := context.Background()
	_, err := strategy.Summarize(ctx, req)

	if err != ErrAIClientRequired {
		t.Errorf("Expected ErrAIClientRequired, got %v", err)
	}
}

func TestHybridStrategy_Summarize(t *testing.T) {
	aiClient := newMockAIClient("A polished hybrid summary of Go programming language.")
	strategy := NewHybridStrategy(aiClient, testLogger())

	req := &SummaryRequest{
		Content: sampleContent,
		Length:  SummaryLengthParagraph,
	}
	req.SetDefaults()

	ctx := context.Background()
	result, err := strategy.Summarize(ctx, req)

	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	// Hybrid should have higher confidence
	if result.Confidence < 0.85 {
		t.Errorf("Expected confidence >= 0.85, got %f", result.Confidence)
	}
}

func TestHybridStrategy_FallbackToExtractive(t *testing.T) {
	strategy := NewHybridStrategy(nil, testLogger())

	req := &SummaryRequest{
		Content: sampleContent,
		Length:  SummaryLengthParagraph,
	}
	req.SetDefaults()

	ctx := context.Background()
	result, err := strategy.Summarize(ctx, req)

	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	if result.Summary == "" {
		t.Error("Expected non-empty summary from extractive fallback")
	}
}

func TestHierarchicalStrategy_Summarize(t *testing.T) {
	response := `One-Line: Go is a statically typed language designed at Google.

Paragraph: Go is a programming language designed at Google in 2007 by Robert Griesemer, Rob Pike, and Ken Thompson. It features garbage collection, structural typing, and CSP-style concurrency.

Full: The Go programming language was designed at Google in 2007 by Robert Griesemer, Rob Pike, and Ken Thompson. Go is a statically typed, compiled language known for its simplicity and efficiency. The language features garbage collection, structural typing, and CSP-style concurrency. Companies like Google, Uber, Twitch, and Dropbox use Go extensively in their production systems.`

	aiClient := newMockAIClient(response)
	strategy := NewHierarchicalStrategy(aiClient, testLogger())

	ctx := context.Background()
	result, err := strategy.SummarizeHierarchical(ctx, sampleContent, "test-tenant")

	if err != nil {
		t.Fatalf("SummarizeHierarchical() error = %v", err)
	}

	if result.OneLine == "" {
		t.Error("Expected non-empty one-line summary")
	}

	if result.Paragraph == "" {
		t.Error("Expected non-empty paragraph summary")
	}

	if result.Full == "" {
		t.Error("Expected non-empty full summary")
	}

	// One-line should be shorter than paragraph
	if len(result.OneLine) >= len(result.Paragraph) && result.Paragraph != result.OneLine {
		t.Logf("One-line (%d chars): %s", len(result.OneLine), result.OneLine)
		t.Logf("Paragraph (%d chars): %s", len(result.Paragraph), result.Paragraph)
	}
}

func TestSummarizer_Integration(t *testing.T) {
	aiClient := newMockAIClient("AI-generated summary of the content.")
	logger := testLogger()

	cfg := DefaultConfig()
	cfg.MinContentLength = 10
	cfg.EnableCache = false

	summarizer := NewSummarizer(cfg, logger, aiClient)

	// Test with different strategies
	strategies := []StrategyType{
		StrategyTypeExtractive,
		StrategyTypeAbstractive,
		StrategyTypeHybrid,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy), func(t *testing.T) {
			req := &SummaryRequest{
				Content:  sampleContent,
				Strategy: strategy,
				Length:   SummaryLengthParagraph,
			}

			ctx := context.Background()
			result, err := summarizer.Summarize(ctx, req)

			if err != nil {
				t.Fatalf("Summarize() error = %v", err)
			}

			if result.Summary == "" {
				t.Error("Expected non-empty summary")
			}

			if result.Strategy != strategy {
				t.Errorf("Expected strategy %s, got %s", strategy, result.Strategy)
			}

			if result.ProcessingTime == 0 {
				t.Error("Expected non-zero processing time")
			}

			if result.CompressionRatio == 0 {
				t.Error("Expected non-zero compression ratio")
			}
		})
	}
}

func TestSummarizer_ContentLengthValidation(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MinContentLength = 100
	cfg.EnableCache = false

	summarizer := NewSummarizer(cfg, logger, nil)

	req := &SummaryRequest{
		Content:  shortContent, // Too short
		Strategy: StrategyTypeExtractive,
	}

	ctx := context.Background()
	_, err := summarizer.Summarize(ctx, req)

	if err == nil {
		t.Error("Expected error for content too short")
	}
}

func TestSummarizer_BatchProcessing(t *testing.T) {
	aiClient := newMockAIClient("Summary")
	logger := testLogger()

	cfg := DefaultConfig()
	cfg.MinContentLength = 10
	cfg.EnableCache = false

	summarizer := NewSummarizer(cfg, logger, aiClient)

	requests := []*SummaryRequest{
		{Content: sampleContent, Strategy: StrategyTypeExtractive},
		{Content: sampleContent, Strategy: StrategyTypeExtractive},
		{Content: sampleContent, Strategy: StrategyTypeAbstractive},
	}

	ctx := context.Background()
	results, err := summarizer.SummarizeBatch(ctx, requests)

	if err != nil {
		t.Fatalf("SummarizeBatch() error = %v", err)
	}

	if len(results) != len(requests) {
		t.Errorf("Expected %d results, got %d", len(requests), len(results))
	}

	for i, result := range results {
		if result.Summary == "" {
			t.Errorf("Result %d has empty summary", i)
		}
	}
}

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		// The regex splits on [.!?]+\s+ and filters out fragments < 10 chars
		{"multiple sentences", "First sentence. Second sentence. Third sentence here.", 3},
		{"with exclamation", "Hello there friend! How are you doing? I am fine today.", 3},
		{"single sentence", "This is just one sentence", 1},
		{"empty", "", 0},
		{"short fragments", "Hi. Go. No.", 0}, // All filtered as < 10 chars
		{"long text", "The Go programming language was designed at Google. It is a compiled language. Go is fast.", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentences := splitSentences(tt.text)
			if len(sentences) != tt.expected {
				t.Errorf("splitSentences() returned %d sentences, expected %d, got: %v", len(sentences), tt.expected, sentences)
			}
		})
	}
}

func TestSentenceSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		s1       string
		s2       string
		minScore float64
		maxScore float64
	}{
		{
			name:     "identical sentences",
			s1:       "The quick brown fox",
			s2:       "The quick brown fox",
			minScore: 0.8,
			maxScore: 1.1,
		},
		{
			name:     "similar sentences",
			s1:       "Go is a programming language",
			s2:       "Go is a compiled language",
			minScore: 0.3,
			maxScore: 0.9,
		},
		{
			name:     "different sentences",
			s1:       "The weather is nice today",
			s2:       "Programming requires focus",
			minScore: 0.0,
			maxScore: 0.2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := sentenceSimilarity(tt.s1, tt.s2)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("sentenceSimilarity() = %f, expected between %f and %f", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	text := "The Go programming language was designed at Google."
	tokens := tokenize(text)

	// Should filter stopwords
	for _, token := range tokens {
		if token == "the" || token == "was" || token == "at" {
			t.Errorf("Stopword '%s' should have been filtered", token)
		}
	}

	// Should include meaningful words
	found := make(map[string]bool)
	for _, token := range tokens {
		found[token] = true
	}

	if !found["programming"] {
		t.Error("Expected 'programming' in tokens")
	}
	if !found["google"] {
		t.Error("Expected 'google' in tokens")
	}
}

func TestCleanSummaryResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with prefix",
			input:    "Here is a summary: The Go language is great.",
			expected: "The Go language is great.",
		},
		{
			name:     "without prefix",
			input:    "The Go language is great.",
			expected: "The Go language is great.",
		},
		{
			name:     "with whitespace",
			input:    "  Summary: The content is good.  ",
			expected: "The content is good.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanSummaryResponse(tt.input)
			if result != tt.expected {
				t.Errorf("cleanSummaryResponse() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestExtractBulletPoints(t *testing.T) {
	input := `Here are the key points:
- First point about Go
- Second point about programming
* Third point with asterisk
1. Fourth point numbered`

	points := extractBulletPoints(input)

	if len(points) != 4 {
		t.Errorf("Expected 4 bullet points, got %d", len(points))
	}

	if len(points) > 0 && points[0] != "First point about Go" {
		t.Errorf("First point incorrect: %s", points[0])
	}
}

func TestMergeKeyPoints(t *testing.T) {
	list1 := []string{"Point A", "Point B"}
	list2 := []string{"Point B", "Point C"} // "Point B" is duplicate

	merged := mergeKeyPoints(list1, list2, 5)

	if len(merged) != 3 {
		t.Errorf("Expected 3 merged points, got %d", len(merged))
	}

	// Test max count limit
	merged = mergeKeyPoints(list1, list2, 2)
	if len(merged) != 2 {
		t.Errorf("Expected 2 merged points (limited), got %d", len(merged))
	}
}

func TestParseHierarchicalResponse(t *testing.T) {
	input := `One-Line: Go is a fast programming language.

Paragraph: Go is a statically typed language designed at Google. It features garbage collection and concurrency.

Full: The Go programming language was designed at Google in 2007. It is known for simplicity and efficiency. Go has built-in support for concurrent programming.`

	oneLine, paragraph, full := parseHierarchicalResponse(input)

	if oneLine == "" {
		t.Error("Expected non-empty one-line")
	}

	if paragraph == "" {
		t.Error("Expected non-empty paragraph")
	}

	if full == "" {
		t.Error("Expected non-empty full")
	}

	// One-line should be shortest
	if len(oneLine) > len(paragraph) {
		t.Error("One-line should be shorter than paragraph")
	}
}

// Benchmark tests

func BenchmarkExtractiveStrategy(b *testing.B) {
	strategy := NewExtractiveStrategy(testLogger())
	req := &SummaryRequest{
		Content: longContent,
		Length:  SummaryLengthParagraph,
	}
	req.SetDefaults()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.Summarize(ctx, req)
	}
}

func BenchmarkSentenceSimilarity(b *testing.B) {
	s1 := "The Go programming language was designed at Google by Robert Griesemer"
	s2 := "Go is a statically typed compiled language known for its simplicity"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sentenceSimilarity(s1, s2)
	}
}

func BenchmarkTokenize(b *testing.B) {
	text := "The Go programming language was designed at Google by Robert Griesemer, Rob Pike, and Ken Thompson."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tokenize(text)
	}
}

func BenchmarkSplitSentences(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = splitSentences(longContent)
	}
}
