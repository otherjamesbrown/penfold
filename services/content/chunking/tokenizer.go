// Package chunking provides token counting utilities for chunking strategies.
package chunking

import (
	"strings"
	"sync"
	"unicode"
)

// Tokenizer provides token counting for text.
type Tokenizer interface {
	// CountTokens estimates the number of tokens in the given text.
	CountTokens(text string) int

	// Name returns the tokenizer name.
	Name() string
}

// ApproximateTokenizer estimates tokens by multiplying word count by a factor.
// This is fast but approximate - suitable for chunking where exact counts aren't critical.
type ApproximateTokenizer struct {
	// WordToTokenRatio is the estimated ratio of tokens to words.
	// Common values: 1.3 for English, 1.5 for mixed content.
	wordToTokenRatio float64
}

// NewApproximateTokenizer creates a new approximate tokenizer.
func NewApproximateTokenizer(ratio float64) *ApproximateTokenizer {
	if ratio <= 0 {
		ratio = 1.3 // Default for English
	}
	return &ApproximateTokenizer{
		wordToTokenRatio: ratio,
	}
}

// CountTokens estimates tokens using word count * ratio.
func (t *ApproximateTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	wordCount := len(strings.Fields(text))
	return int(float64(wordCount) * t.wordToTokenRatio)
}

// Name returns the tokenizer name.
func (t *ApproximateTokenizer) Name() string {
	return "approximate"
}

// CharacterTokenizer estimates tokens based on character count.
// Uses the approximation that ~4 characters = 1 token for English.
type CharacterTokenizer struct {
	// CharsPerToken is the average characters per token.
	charsPerToken float64
}

// NewCharacterTokenizer creates a new character-based tokenizer.
func NewCharacterTokenizer(charsPerToken float64) *CharacterTokenizer {
	if charsPerToken <= 0 {
		charsPerToken = 4.0 // Default for English
	}
	return &CharacterTokenizer{
		charsPerToken: charsPerToken,
	}
}

// CountTokens estimates tokens based on character count.
func (t *CharacterTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	return int(float64(len(text)) / t.charsPerToken)
}

// Name returns the tokenizer name.
func (t *CharacterTokenizer) Name() string {
	return "character"
}

// WhitespaceTokenizer counts tokens by splitting on whitespace.
// Each whitespace-separated word is considered one token.
type WhitespaceTokenizer struct{}

// NewWhitespaceTokenizer creates a new whitespace tokenizer.
func NewWhitespaceTokenizer() *WhitespaceTokenizer {
	return &WhitespaceTokenizer{}
}

// CountTokens counts whitespace-separated words.
func (t *WhitespaceTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}

// Name returns the tokenizer name.
func (t *WhitespaceTokenizer) Name() string {
	return "whitespace"
}

// SubwordTokenizer provides a more accurate token estimate by counting subwords.
// It considers that long words and special characters often become multiple tokens.
type SubwordTokenizer struct {
	// averageSubwordLength is the typical length of a subword token.
	averageSubwordLength int
}

// NewSubwordTokenizer creates a new subword tokenizer.
func NewSubwordTokenizer(avgLength int) *SubwordTokenizer {
	if avgLength <= 0 {
		avgLength = 4 // Typical for BPE tokenizers
	}
	return &SubwordTokenizer{
		averageSubwordLength: avgLength,
	}
}

// CountTokens estimates tokens by analyzing word structure.
func (t *SubwordTokenizer) CountTokens(text string) int {
	if text == "" {
		return 0
	}

	totalTokens := 0
	words := strings.Fields(text)

	for _, word := range words {
		tokenCount := t.estimateWordTokens(word)
		totalTokens += tokenCount
	}

	return totalTokens
}

// estimateWordTokens estimates tokens for a single word.
func (t *SubwordTokenizer) estimateWordTokens(word string) int {
	if len(word) == 0 {
		return 0
	}

	// Count special characters that often become separate tokens
	specialCount := 0
	letterCount := 0

	for _, r := range word {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			specialCount++
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letterCount++
		}
	}

	// Estimate: special chars often become separate tokens
	// Long words get split into subwords
	letterTokens := (letterCount + t.averageSubwordLength - 1) / t.averageSubwordLength
	if letterTokens < 1 && letterCount > 0 {
		letterTokens = 1
	}

	return letterTokens + specialCount
}

// Name returns the tokenizer name.
func (t *SubwordTokenizer) Name() string {
	return "subword"
}

// CachingTokenizer wraps another tokenizer and caches results.
type CachingTokenizer struct {
	inner    Tokenizer
	cache    map[string]int
	mu       sync.RWMutex
	maxCache int
}

// NewCachingTokenizer creates a tokenizer that caches results.
func NewCachingTokenizer(inner Tokenizer, maxCacheSize int) *CachingTokenizer {
	if maxCacheSize <= 0 {
		maxCacheSize = 10000
	}
	return &CachingTokenizer{
		inner:    inner,
		cache:    make(map[string]int),
		maxCache: maxCacheSize,
	}
}

// CountTokens returns cached count or computes and caches.
func (t *CachingTokenizer) CountTokens(text string) int {
	// Only cache shorter strings (caching very long strings is inefficient)
	if len(text) > 1000 {
		return t.inner.CountTokens(text)
	}

	t.mu.RLock()
	if count, ok := t.cache[text]; ok {
		t.mu.RUnlock()
		return count
	}
	t.mu.RUnlock()

	count := t.inner.CountTokens(text)

	t.mu.Lock()
	// Simple eviction: clear cache when full
	if len(t.cache) >= t.maxCache {
		t.cache = make(map[string]int)
	}
	t.cache[text] = count
	t.mu.Unlock()

	return count
}

// Name returns the tokenizer name.
func (t *CachingTokenizer) Name() string {
	return "caching-" + t.inner.Name()
}

// ClearCache removes all cached entries.
func (t *CachingTokenizer) ClearCache() {
	t.mu.Lock()
	t.cache = make(map[string]int)
	t.mu.Unlock()
}

// ModelTokenizer represents a tokenizer configured for a specific model.
type ModelTokenizer struct {
	modelName  string
	inner      Tokenizer
}

// ModelTokenizerConfig maps model names to their tokenizer settings.
var ModelTokenizerConfig = map[string]struct {
	Ratio         float64
	CharsPerToken float64
}{
	// OpenAI models (GPT-3.5, GPT-4)
	"gpt-4":       {Ratio: 1.3, CharsPerToken: 4.0},
	"gpt-3.5":     {Ratio: 1.3, CharsPerToken: 4.0},
	"text-embedding-ada-002": {Ratio: 1.3, CharsPerToken: 4.0},

	// Claude models
	"claude-3":    {Ratio: 1.35, CharsPerToken: 3.8},
	"claude-2":    {Ratio: 1.35, CharsPerToken: 3.8},

	// Llama models
	"llama-3":     {Ratio: 1.4, CharsPerToken: 3.5},
	"llama-2":     {Ratio: 1.4, CharsPerToken: 3.5},

	// Gemini models
	"gemini-pro":  {Ratio: 1.35, CharsPerToken: 3.8},
	"gemini-1.5":  {Ratio: 1.35, CharsPerToken: 3.8},

	// Default/fallback
	"default":     {Ratio: 1.3, CharsPerToken: 4.0},
}

// NewModelTokenizer creates a tokenizer configured for a specific model.
func NewModelTokenizer(modelName string) *ModelTokenizer {
	config, ok := ModelTokenizerConfig[modelName]
	if !ok {
		config = ModelTokenizerConfig["default"]
	}

	return &ModelTokenizer{
		modelName: modelName,
		inner:     NewApproximateTokenizer(config.Ratio),
	}
}

// CountTokens estimates tokens for the configured model.
func (t *ModelTokenizer) CountTokens(text string) int {
	return t.inner.CountTokens(text)
}

// Name returns the tokenizer name.
func (t *ModelTokenizer) Name() string {
	return "model-" + t.modelName
}

// TokenizerFactory creates tokenizers based on type.
type TokenizerFactory struct{}

// Create creates a tokenizer by type name.
func (f *TokenizerFactory) Create(tokenizerType string, options map[string]interface{}) Tokenizer {
	switch tokenizerType {
	case "approximate":
		ratio := 1.3
		if r, ok := options["ratio"].(float64); ok {
			ratio = r
		}
		return NewApproximateTokenizer(ratio)

	case "character":
		chars := 4.0
		if c, ok := options["chars_per_token"].(float64); ok {
			chars = c
		}
		return NewCharacterTokenizer(chars)

	case "whitespace":
		return NewWhitespaceTokenizer()

	case "subword":
		avgLen := 4
		if l, ok := options["avg_length"].(int); ok {
			avgLen = l
		}
		return NewSubwordTokenizer(avgLen)

	case "model":
		modelName := "default"
		if m, ok := options["model"].(string); ok {
			modelName = m
		}
		return NewModelTokenizer(modelName)

	default:
		return NewApproximateTokenizer(1.3)
	}
}

// EstimateTokens is a convenience function for quick token estimation.
func EstimateTokens(text string) int {
	return NewApproximateTokenizer(1.3).CountTokens(text)
}

// EstimateTokensForModel estimates tokens for a specific model.
func EstimateTokensForModel(text, modelName string) int {
	return NewModelTokenizer(modelName).CountTokens(text)
}
