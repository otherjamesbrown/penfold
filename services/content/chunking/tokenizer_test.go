package chunking

import (
	"testing"
)

// =============================================================================
// ApproximateTokenizer Tests
// =============================================================================

func TestApproximateTokenizer_Name(t *testing.T) {
	tokenizer := NewApproximateTokenizer(1.3)
	if tokenizer.Name() != "approximate" {
		t.Errorf("expected name 'approximate', got '%s'", tokenizer.Name())
	}
}

func TestApproximateTokenizer_EmptyText(t *testing.T) {
	tokenizer := NewApproximateTokenizer(1.3)
	count := tokenizer.CountTokens("")
	if count != 0 {
		t.Errorf("expected 0 tokens for empty text, got %d", count)
	}
}

func TestApproximateTokenizer_SingleWord(t *testing.T) {
	tokenizer := NewApproximateTokenizer(1.3)
	count := tokenizer.CountTokens("hello")
	// 1 word * 1.3 = 1 token (rounded)
	if count < 1 {
		t.Errorf("expected at least 1 token, got %d", count)
	}
}

func TestApproximateTokenizer_MultipleWords(t *testing.T) {
	tokenizer := NewApproximateTokenizer(1.3)
	count := tokenizer.CountTokens("hello world test")
	// 3 words * 1.3 = ~4 tokens
	if count < 3 || count > 5 {
		t.Errorf("expected 3-5 tokens for 3 words, got %d", count)
	}
}

func TestApproximateTokenizer_DefaultRatio(t *testing.T) {
	tokenizer := NewApproximateTokenizer(0) // Should default to 1.3
	count := tokenizer.CountTokens("hello world")
	if count != 2 { // 2 words * 1.3 = 2.6 -> 2
		t.Errorf("expected 2 tokens, got %d", count)
	}
}

// =============================================================================
// CharacterTokenizer Tests
// =============================================================================

func TestCharacterTokenizer_Name(t *testing.T) {
	tokenizer := NewCharacterTokenizer(4.0)
	if tokenizer.Name() != "character" {
		t.Errorf("expected name 'character', got '%s'", tokenizer.Name())
	}
}

func TestCharacterTokenizer_EmptyText(t *testing.T) {
	tokenizer := NewCharacterTokenizer(4.0)
	count := tokenizer.CountTokens("")
	if count != 0 {
		t.Errorf("expected 0 tokens for empty text, got %d", count)
	}
}

func TestCharacterTokenizer_ShortText(t *testing.T) {
	tokenizer := NewCharacterTokenizer(4.0)
	count := tokenizer.CountTokens("hi") // 2 chars / 4 = 0
	if count != 0 {
		t.Errorf("expected 0 tokens for very short text, got %d", count)
	}
}

func TestCharacterTokenizer_LongText(t *testing.T) {
	tokenizer := NewCharacterTokenizer(4.0)
	count := tokenizer.CountTokens("hello world") // 11 chars / 4 = 2
	if count != 2 {
		t.Errorf("expected 2 tokens, got %d", count)
	}
}

func TestCharacterTokenizer_DefaultCharsPerToken(t *testing.T) {
	tokenizer := NewCharacterTokenizer(0) // Should default to 4.0
	count := tokenizer.CountTokens("hello world") // 11 chars / 4 = 2
	if count != 2 {
		t.Errorf("expected 2 tokens, got %d", count)
	}
}

// =============================================================================
// WhitespaceTokenizer Tests
// =============================================================================

func TestWhitespaceTokenizer_Name(t *testing.T) {
	tokenizer := NewWhitespaceTokenizer()
	if tokenizer.Name() != "whitespace" {
		t.Errorf("expected name 'whitespace', got '%s'", tokenizer.Name())
	}
}

func TestWhitespaceTokenizer_EmptyText(t *testing.T) {
	tokenizer := NewWhitespaceTokenizer()
	count := tokenizer.CountTokens("")
	if count != 0 {
		t.Errorf("expected 0 tokens for empty text, got %d", count)
	}
}

func TestWhitespaceTokenizer_SingleWord(t *testing.T) {
	tokenizer := NewWhitespaceTokenizer()
	count := tokenizer.CountTokens("hello")
	if count != 1 {
		t.Errorf("expected 1 token, got %d", count)
	}
}

func TestWhitespaceTokenizer_MultipleWords(t *testing.T) {
	tokenizer := NewWhitespaceTokenizer()
	count := tokenizer.CountTokens("hello world test")
	if count != 3 {
		t.Errorf("expected 3 tokens, got %d", count)
	}
}

func TestWhitespaceTokenizer_MultipleSpaces(t *testing.T) {
	tokenizer := NewWhitespaceTokenizer()
	count := tokenizer.CountTokens("hello   world  test")
	if count != 3 {
		t.Errorf("expected 3 tokens (multiple spaces collapsed), got %d", count)
	}
}

func TestWhitespaceTokenizer_TabsAndNewlines(t *testing.T) {
	tokenizer := NewWhitespaceTokenizer()
	count := tokenizer.CountTokens("hello\tworld\ntest")
	if count != 3 {
		t.Errorf("expected 3 tokens, got %d", count)
	}
}

// =============================================================================
// SubwordTokenizer Tests
// =============================================================================

func TestSubwordTokenizer_Name(t *testing.T) {
	tokenizer := NewSubwordTokenizer(4)
	if tokenizer.Name() != "subword" {
		t.Errorf("expected name 'subword', got '%s'", tokenizer.Name())
	}
}

func TestSubwordTokenizer_EmptyText(t *testing.T) {
	tokenizer := NewSubwordTokenizer(4)
	count := tokenizer.CountTokens("")
	if count != 0 {
		t.Errorf("expected 0 tokens for empty text, got %d", count)
	}
}

func TestSubwordTokenizer_ShortWord(t *testing.T) {
	tokenizer := NewSubwordTokenizer(4)
	count := tokenizer.CountTokens("hi")
	if count != 1 {
		t.Errorf("expected 1 token for short word, got %d", count)
	}
}

func TestSubwordTokenizer_LongWord(t *testing.T) {
	tokenizer := NewSubwordTokenizer(4)
	count := tokenizer.CountTokens("supercalifragilisticexpialidocious")
	// 34 letters / 4 = 8+ subword tokens
	if count < 8 {
		t.Errorf("expected at least 8 tokens for long word, got %d", count)
	}
}

func TestSubwordTokenizer_SpecialCharacters(t *testing.T) {
	tokenizer := NewSubwordTokenizer(4)
	count := tokenizer.CountTokens("hello...world!!!")
	// Should count special characters as separate tokens
	if count < 2 {
		t.Errorf("expected at least 2 tokens (words + punctuation), got %d", count)
	}
}

func TestSubwordTokenizer_MixedContent(t *testing.T) {
	tokenizer := NewSubwordTokenizer(4)
	count := tokenizer.CountTokens("Hello, world! How are you?")
	// Multiple words and punctuation
	if count < 5 {
		t.Errorf("expected at least 5 tokens, got %d", count)
	}
}

// =============================================================================
// CachingTokenizer Tests
// =============================================================================

func TestCachingTokenizer_Name(t *testing.T) {
	inner := NewWhitespaceTokenizer()
	tokenizer := NewCachingTokenizer(inner, 100)
	if tokenizer.Name() != "caching-whitespace" {
		t.Errorf("expected name 'caching-whitespace', got '%s'", tokenizer.Name())
	}
}

func TestCachingTokenizer_CachesResults(t *testing.T) {
	callCount := 0
	inner := &countingTokenizer{
		inner:     NewWhitespaceTokenizer(),
		callCount: &callCount,
	}
	tokenizer := NewCachingTokenizer(inner, 100)

	text := "hello world"

	// First call
	count1 := tokenizer.CountTokens(text)
	if callCount != 1 {
		t.Errorf("expected 1 inner call, got %d", callCount)
	}

	// Second call should use cache
	count2 := tokenizer.CountTokens(text)
	if callCount != 1 {
		t.Errorf("expected still 1 inner call (cached), got %d", callCount)
	}

	if count1 != count2 {
		t.Errorf("cached result should match: %d vs %d", count1, count2)
	}
}

func TestCachingTokenizer_SkipsLongStrings(t *testing.T) {
	callCount := 0
	inner := &countingTokenizer{
		inner:     NewWhitespaceTokenizer(),
		callCount: &callCount,
	}
	tokenizer := NewCachingTokenizer(inner, 100)

	// Create a string longer than 1000 chars
	longText := make([]byte, 1500)
	for i := range longText {
		longText[i] = 'a'
	}
	text := string(longText)

	// First call
	tokenizer.CountTokens(text)
	if callCount != 1 {
		t.Errorf("expected 1 inner call, got %d", callCount)
	}

	// Second call should NOT use cache for long strings
	tokenizer.CountTokens(text)
	if callCount != 2 {
		t.Errorf("expected 2 inner calls (not cached for long strings), got %d", callCount)
	}
}

func TestCachingTokenizer_ClearCache(t *testing.T) {
	callCount := 0
	inner := &countingTokenizer{
		inner:     NewWhitespaceTokenizer(),
		callCount: &callCount,
	}
	tokenizer := NewCachingTokenizer(inner, 100)

	text := "hello world"

	// First call
	tokenizer.CountTokens(text)

	// Clear cache
	tokenizer.ClearCache()

	// Second call should NOT use cache
	tokenizer.CountTokens(text)
	if callCount != 2 {
		t.Errorf("expected 2 inner calls after cache clear, got %d", callCount)
	}
}

// countingTokenizer wraps a tokenizer and counts calls.
type countingTokenizer struct {
	inner     Tokenizer
	callCount *int
}

func (t *countingTokenizer) CountTokens(text string) int {
	*t.callCount++
	return t.inner.CountTokens(text)
}

func (t *countingTokenizer) Name() string {
	return t.inner.Name()
}

// =============================================================================
// ModelTokenizer Tests
// =============================================================================

func TestModelTokenizer_Name(t *testing.T) {
	tokenizer := NewModelTokenizer("gpt-4")
	if tokenizer.Name() != "model-gpt-4" {
		t.Errorf("expected name 'model-gpt-4', got '%s'", tokenizer.Name())
	}
}

func TestModelTokenizer_KnownModels(t *testing.T) {
	models := []string{
		"gpt-4",
		"gpt-3.5",
		"claude-3",
		"claude-2",
		"llama-3",
		"llama-2",
		"gemini-pro",
		"gemini-1.5",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			tokenizer := NewModelTokenizer(model)
			count := tokenizer.CountTokens("hello world test")
			if count < 1 {
				t.Errorf("expected at least 1 token for model %s, got %d", model, count)
			}
		})
	}
}

func TestModelTokenizer_UnknownModel(t *testing.T) {
	tokenizer := NewModelTokenizer("unknown-model")
	count := tokenizer.CountTokens("hello world")
	// Should use default config
	if count < 1 {
		t.Errorf("expected at least 1 token for unknown model, got %d", count)
	}
}

// =============================================================================
// TokenizerFactory Tests
// =============================================================================

func TestTokenizerFactory_Create(t *testing.T) {
	factory := &TokenizerFactory{}

	tests := []struct {
		tokenizerType string
		options       map[string]interface{}
		expectedName  string
	}{
		{"approximate", nil, "approximate"},
		{"approximate", map[string]interface{}{"ratio": 1.5}, "approximate"},
		{"character", nil, "character"},
		{"character", map[string]interface{}{"chars_per_token": 3.0}, "character"},
		{"whitespace", nil, "whitespace"},
		{"subword", nil, "subword"},
		{"subword", map[string]interface{}{"avg_length": 3}, "subword"},
		{"model", nil, "model-default"},
		{"model", map[string]interface{}{"model": "gpt-4"}, "model-gpt-4"},
		{"unknown", nil, "approximate"}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.tokenizerType, func(t *testing.T) {
			tokenizer := factory.Create(tt.tokenizerType, tt.options)
			if tokenizer.Name() != tt.expectedName {
				t.Errorf("expected name '%s', got '%s'", tt.expectedName, tokenizer.Name())
			}
		})
	}
}

// =============================================================================
// Convenience Function Tests
// =============================================================================

func TestEstimateTokens(t *testing.T) {
	count := EstimateTokens("hello world test")
	// 3 words * 1.3 = 3 tokens
	if count < 3 || count > 5 {
		t.Errorf("expected 3-5 tokens, got %d", count)
	}
}

func TestEstimateTokensForModel(t *testing.T) {
	count := EstimateTokensForModel("hello world test", "gpt-4")
	// Should use GPT-4's ratio
	if count < 3 || count > 5 {
		t.Errorf("expected 3-5 tokens, got %d", count)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkApproximateTokenizer(b *testing.B) {
	tokenizer := NewApproximateTokenizer(1.3)
	text := "This is a test sentence with multiple words to tokenize."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.CountTokens(text)
	}
}

func BenchmarkSubwordTokenizer(b *testing.B) {
	tokenizer := NewSubwordTokenizer(4)
	text := "This is a test sentence with multiple words to tokenize."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.CountTokens(text)
	}
}

func BenchmarkCachingTokenizer(b *testing.B) {
	tokenizer := NewCachingTokenizer(NewWhitespaceTokenizer(), 1000)
	text := "This is a test sentence with multiple words to tokenize."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.CountTokens(text)
	}
}
