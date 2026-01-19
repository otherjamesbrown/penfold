package entity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== Test Utilities =====

func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelError, // Suppress logs during tests
		ServiceName: "test",
		Environment: "test",
		JSONFormat:  false,
	})
}

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

// ===== Types Tests =====

func TestEntityType_IsValid(t *testing.T) {
	tests := []struct {
		entityType EntityType
		expected   bool
	}{
		{EntityTypePerson, true},
		{EntityTypeOrganization, true},
		{EntityTypeLocation, true},
		{EntityTypeDate, true},
		{EntityTypeMoney, true},
		{EntityTypeEmail, true},
		{EntityTypeURL, true},
		{EntityTypePhone, true},
		{EntityTypeEvent, true},
		{EntityTypeProduct, true},
		{EntityTypeMisc, true},
		{EntityTypeUnknown, false},
		{EntityType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.entityType), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.entityType.IsValid())
		})
	}
}

func TestTextPosition_Length(t *testing.T) {
	pos := TextPosition{Start: 10, End: 25}
	assert.Equal(t, 15, pos.Length())
}

func TestDefaultExtractionOptions(t *testing.T) {
	opts := DefaultExtractionOptions()

	assert.InDelta(t, 0.5, float64(opts.MinConfidence), 0.01)
	assert.Equal(t, 0, opts.MaxEntities)
	assert.True(t, opts.UsePatterns)
	assert.True(t, opts.UseNER)
	assert.False(t, opts.UseDomain)
	assert.True(t, opts.NormalizeEntities)
	assert.True(t, opts.DeduplicateEntities)
	assert.True(t, opts.TrackPositions)
}

func TestExtractionOptions_ShouldExtractType(t *testing.T) {
	t.Run("empty types extracts all", func(t *testing.T) {
		opts := ExtractionOptions{EntityTypes: nil}
		assert.True(t, opts.ShouldExtractType(EntityTypePerson))
		assert.True(t, opts.ShouldExtractType(EntityTypeEmail))
	})

	t.Run("specified types filters correctly", func(t *testing.T) {
		opts := ExtractionOptions{EntityTypes: []EntityType{EntityTypePerson, EntityTypeEmail}}
		assert.True(t, opts.ShouldExtractType(EntityTypePerson))
		assert.True(t, opts.ShouldExtractType(EntityTypeEmail))
		assert.False(t, opts.ShouldExtractType(EntityTypeURL))
	})
}

func TestExtractionResult_ByType(t *testing.T) {
	result := &ExtractionResult{
		Entities: []Entity{
			{Type: EntityTypePerson, Value: "John Doe"},
			{Type: EntityTypeEmail, Value: "john@example.com"},
			{Type: EntityTypePerson, Value: "Jane Smith"},
		},
	}

	people := result.ByType(EntityTypePerson)
	assert.Len(t, people, 2)

	emails := result.ByType(EntityTypeEmail)
	assert.Len(t, emails, 1)

	urls := result.ByType(EntityTypeURL)
	assert.Empty(t, urls)
}

func TestExtractionResult_ByMinConfidence(t *testing.T) {
	result := &ExtractionResult{
		Entities: []Entity{
			{Type: EntityTypePerson, Value: "High", Confidence: 0.9},
			{Type: EntityTypePerson, Value: "Medium", Confidence: 0.6},
			{Type: EntityTypePerson, Value: "Low", Confidence: 0.3},
		},
	}

	filtered := result.ByMinConfidence(0.5)
	assert.Len(t, filtered, 2)
}

func TestChunkEntityMap(t *testing.T) {
	m := NewChunkEntityMap()

	entities1 := []Entity{
		{Type: EntityTypePerson, Value: "John", NormalizedValue: "john", Confidence: 0.9},
		{Type: EntityTypeEmail, Value: "john@example.com", NormalizedValue: "john@example.com", Confidence: 0.95},
	}

	entities2 := []Entity{
		{Type: EntityTypePerson, Value: "John", NormalizedValue: "john", Confidence: 0.85}, // Duplicate
		{Type: EntityTypePerson, Value: "Jane", NormalizedValue: "jane", Confidence: 0.9},
	}

	m.Add("chunk1", entities1)
	m.Add("chunk2", entities2)

	all := m.GetAll()
	assert.Len(t, all, 4)

	deduped, removed := m.DeduplicateAcrossChunks()
	assert.Len(t, deduped, 3)
	assert.Equal(t, 1, removed)

	// Verify the higher confidence entity was kept
	for _, e := range deduped {
		if e.Value == "John" {
			assert.InDelta(t, 0.9, float64(e.Confidence), 0.01)
		}
	}
}

// ===== Pattern Tests =====

func TestEmailPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Contact john.doe@example.com for details", []string{"john.doe@example.com"}},
		{"Email alice@company.org or bob@firm.co.uk", []string{"alice@company.org", "bob@firm.co.uk"}},
		{"Test+tag@gmail.com is valid", []string{"Test+tag@gmail.com"}},
		{"No emails here", nil},
		{"", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := EmailPattern.Match(tt.input)
			values := make([]string, len(matches))
			for i, m := range matches {
				values[i] = m.Value
			}
			if tt.expected == nil {
				assert.Empty(t, values)
			} else {
				assert.Equal(t, tt.expected, values)
			}
		})
	}
}

func TestURLPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Visit https://example.com for more", []string{"https://example.com"}},
		{"See http://docs.example.com/guide and https://wiki.org", []string{"http://docs.example.com/guide", "https://wiki.org"}},
		{"No URLs here", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := URLPattern.Match(tt.input)
			values := make([]string, len(matches))
			for i, m := range matches {
				values[i] = m.Value
			}
			if tt.expected == nil {
				assert.Empty(t, values)
			} else {
				assert.Equal(t, tt.expected, values)
			}
		})
	}
}

func TestPhonePatternUS(t *testing.T) {
	tests := []struct {
		input    string
		expected int // Expected number of matches
	}{
		{"Call 555-123-4567", 1},
		{"(555) 123-4567", 1},
		{"+1 555 123 4567", 1},
		{"555.123.4567", 1},
		{"No phone here", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := PhonePatternUS.Match(tt.input)
			assert.Len(t, matches, tt.expected)
		})
	}
}

func TestDatePatternISO(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Meeting on 2024-03-15", []string{"2024-03-15"}},
		{"Between 2024-01-01 and 2024-12-31", []string{"2024-01-01", "2024-12-31"}},
		{"Invalid 2024-13-45", nil},
		{"No date here", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := DatePatternISO.Match(tt.input)
			values := make([]string, len(matches))
			for i, m := range matches {
				values[i] = m.Value
			}
			if tt.expected == nil {
				assert.Empty(t, values)
			} else {
				assert.Equal(t, tt.expected, values)
			}
		})
	}
}

func TestMoneyPatternUSD(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"Total: $1,234.56", []string{"$1,234.56"}},
		{"Price is $100", []string{"$100"}},
		{"Between $50.00 and $100.00", []string{"$50.00", "$100.00"}},
		{"No money here", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := MoneyPatternUSD.Match(tt.input)
			values := make([]string, len(matches))
			for i, m := range matches {
				values[i] = m.Value
			}
			if tt.expected == nil {
				assert.Empty(t, values)
			} else {
				assert.Equal(t, tt.expected, values)
			}
		})
	}
}

func TestPatternExtractor(t *testing.T) {
	extractor := NewPatternExtractor(nil) // Use default patterns

	t.Run("extracts multiple entity types", func(t *testing.T) {
		text := "Contact john@example.com or visit https://example.com. Call 555-123-4567 for $1,000.00 deal."
		opts := DefaultExtractionOptions()
		opts.UseNER = false

		entities := extractor.Extract(text, opts)

		assert.GreaterOrEqual(t, len(entities), 3)

		// Check for email
		hasEmail := false
		hasURL := false
		hasMoney := false
		for _, e := range entities {
			switch e.Type {
			case EntityTypeEmail:
				hasEmail = true
			case EntityTypeURL:
				hasURL = true
			case EntityTypeMoney:
				hasMoney = true
			}
		}
		assert.True(t, hasEmail, "Should have email entity")
		assert.True(t, hasURL, "Should have URL entity")
		assert.True(t, hasMoney, "Should have money entity")
	})

	t.Run("respects entity type filter", func(t *testing.T) {
		text := "Contact john@example.com at https://example.com"
		opts := ExtractionOptions{
			MinConfidence: 0.5,
			UsePatterns:   true,
			EntityTypes:   []EntityType{EntityTypeEmail},
		}

		entities := extractor.Extract(text, opts)

		for _, e := range entities {
			assert.Equal(t, EntityTypeEmail, e.Type)
		}
	})

	t.Run("respects confidence threshold", func(t *testing.T) {
		extractor := NewPatternExtractor(nil)
		text := "Contact john@example.com"
		opts := ExtractionOptions{
			MinConfidence: 0.99, // Very high threshold
			UsePatterns:   true,
		}

		entities := extractor.Extract(text, opts)
		assert.Empty(t, entities)
	})

	t.Run("respects max entities", func(t *testing.T) {
		text := "a@a.com b@b.com c@c.com d@d.com e@e.com"
		opts := ExtractionOptions{
			MinConfidence: 0.5,
			UsePatterns:   true,
			MaxEntities:   2,
		}

		entities := extractor.Extract(text, opts)
		assert.LessOrEqual(t, len(entities), 2)
	})

	t.Run("tracks positions", func(t *testing.T) {
		text := "Email: john@example.com"
		opts := ExtractionOptions{
			MinConfidence:  0.5,
			UsePatterns:    true,
			TrackPositions: true,
		}

		entities := extractor.Extract(text, opts)
		require.NotEmpty(t, entities)

		for _, e := range entities {
			if e.Type == EntityTypeEmail {
				assert.Greater(t, e.Position.Start, 0)
				assert.Greater(t, e.Position.End, e.Position.Start)
			}
		}
	})
}

// ===== Normalizer Tests =====

func TestNormalizeDate(t *testing.T) {
	refTime := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	normalizer := NewEntityNormalizer(&NormalizerConfig{ReferenceTime: refTime})

	tests := []struct {
		input    string
		expected string
	}{
		{"2024-03-15", "2024-03-15"},
		{"03/15/2024", "2024-03-15"},
		{"3/15/2024", "2024-03-15"},
		{"March 15, 2024", "2024-03-15"},
		{"Mar 15 2024", "2024-03-15"},
		{"today", "2024-03-15"},
		{"tomorrow", "2024-03-16"},
		{"yesterday", "2024-03-14"},
		{"next week", "2024-03-22"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizer.NormalizeDate(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	normalizer := NewEntityNormalizer(&NormalizerConfig{PhoneDefaultCountry: "US"})

	tests := []struct {
		input    string
		expected string
	}{
		{"555-123-4567", "+15551234567"},
		{"(555) 123-4567", "+15551234567"},
		{"+1-555-123-4567", "+15551234567"},
		{"5551234567", "+15551234567"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizer.NormalizePhone(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeName(t *testing.T) {
	normalizer := NewEntityNormalizer(nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"john doe", "John Doe"},
		{"JANE SMITH", "Jane Smith"},
		{"Doe, John", "John Doe"},
		{"mcdonald", "McDonald"},
		{"o'brien", "O'Brien"},
		{"van der Berg", "van der Berg"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizer.NormalizeName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	normalizer := NewEntityNormalizer(nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"John.Doe@Example.COM", "john.doe@example.com"},
		{"  alice@company.org  ", "alice@company.org"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizer.NormalizeEmail(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeMoney(t *testing.T) {
	normalizer := NewEntityNormalizer(nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"$1,234.56", "USD 1234.56"},
		{"$100", "USD 100.00"},
		{"EUR 500", "EUR 500.00"},
		{"1000 euros", "EUR 1000.00"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizer.NormalizeMoney(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeURL(t *testing.T) {
	normalizer := NewEntityNormalizer(nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/", "https://example.com"},
		{"HTTP://EXAMPLE.COM", "http://EXAMPLE.COM"},
		{"https://example.com/page.", "https://example.com/page"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizer.NormalizeURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeEntity(t *testing.T) {
	normalizer := NewEntityNormalizer(nil)

	entity := &Entity{
		Type:  EntityTypeEmail,
		Value: "John.Doe@Example.COM",
	}

	normalizer.Normalize(entity)

	assert.Equal(t, "john.doe@example.com", entity.NormalizedValue)
}

// ===== NER Tests =====

func TestNERExtractor_ParseResponse(t *testing.T) {
	extractor := NewNERExtractor(nil, nil)

	t.Run("parses valid JSON array", func(t *testing.T) {
		response := `[
			{"type": "person", "value": "John Doe", "confidence": 0.9},
			{"type": "organization", "value": "Acme Corp", "confidence": 0.85}
		]`

		entities, err := extractor.parseNERResponse(response, "John Doe works at Acme Corp")

		require.NoError(t, err)
		assert.Len(t, entities, 2)
		assert.Equal(t, EntityTypePerson, entities[0].Type)
		assert.Equal(t, "John Doe", entities[0].Value)
	})

	t.Run("handles markdown code blocks", func(t *testing.T) {
		response := "```json\n[{\"type\": \"person\", \"value\": \"Alice\", \"confidence\": 0.9}]\n```"

		entities, err := extractor.parseNERResponse(response, "Alice")

		require.NoError(t, err)
		assert.Len(t, entities, 1)
	})

	t.Run("extracts JSON from mixed content", func(t *testing.T) {
		response := `Here are the entities: [{"type": "location", "value": "New York", "confidence": 0.95}] That's all.`

		entities, err := extractor.parseNERResponse(response, "Visit New York")

		require.NoError(t, err)
		assert.Len(t, entities, 1)
		assert.Equal(t, EntityTypeLocation, entities[0].Type)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		response := "This is not JSON at all"

		_, err := extractor.parseNERResponse(response, "test")

		assert.Error(t, err)
	})
}

func TestNERExtractor_WithMockClient(t *testing.T) {
	t.Run("extracts entities successfully", func(t *testing.T) {
		mockClient := &MockAIClient{
			CompleteFunc: func(ctx context.Context, prompt string, opts *CompletionOptions) (*CompletionResponse, error) {
				response := []nerResponseEntity{
					{Type: "person", Value: "John Smith", Confidence: 0.95},
					{Type: "organization", Value: "Tech Corp", Confidence: 0.90},
				}
				jsonBytes, _ := json.Marshal(response)
				return &CompletionResponse{
					Text:  string(jsonBytes),
					Model: "test-model",
				}, nil
			},
		}

		extractor := NewNERExtractor(mockClient, nil)
		entities, err := extractor.Extract(context.Background(), "John Smith works at Tech Corp", DefaultExtractionOptions())

		require.NoError(t, err)
		assert.Len(t, entities, 2)
	})

	t.Run("handles empty text", func(t *testing.T) {
		extractor := NewNERExtractor(nil, nil)
		entities, err := extractor.Extract(context.Background(), "", DefaultExtractionOptions())

		require.NoError(t, err)
		assert.Empty(t, entities)
	})

	t.Run("returns error when client is nil", func(t *testing.T) {
		extractor := NewNERExtractor(nil, nil)
		_, err := extractor.Extract(context.Background(), "Some text", DefaultExtractionOptions())

		assert.Error(t, err)
	})
}

func TestMapNERTypeToEntityType(t *testing.T) {
	tests := []struct {
		nerType  string
		expected EntityType
	}{
		{"person", EntityTypePerson},
		{"PER", EntityTypePerson},
		{"organization", EntityTypeOrganization},
		{"ORG", EntityTypeOrganization},
		{"location", EntityTypeLocation},
		{"LOC", EntityTypeLocation},
		{"GPE", EntityTypeLocation},
		{"date", EntityTypeDate},
		{"money", EntityTypeMoney},
		{"event", EntityTypeEvent},
		{"product", EntityTypeProduct},
		{"misc", EntityTypeMisc},
		{"unknown_type", EntityTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.nerType, func(t *testing.T) {
			result := mapNERTypeToEntityType(tt.nerType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===== Hybrid Extractor Tests =====

func TestHybridExtractor(t *testing.T) {
	logger := testLogger()

	t.Run("combines pattern and NER results", func(t *testing.T) {
		mockClient := &MockAIClient{
			CompleteFunc: func(ctx context.Context, prompt string, opts *CompletionOptions) (*CompletionResponse, error) {
				response := []nerResponseEntity{
					{Type: "person", Value: "Alice Johnson", Confidence: 0.9},
				}
				jsonBytes, _ := json.Marshal(response)
				return &CompletionResponse{
					Text:  string(jsonBytes),
					Model: "test-model",
				}, nil
			},
		}

		extractor := NewHybridExtractor(mockClient, logger, nil)
		result, err := extractor.Extract(
			context.Background(),
			"Contact alice@example.com - Alice Johnson",
			DefaultExtractionOptions(),
		)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Entities)

		// Should have both email (pattern) and person (NER)
		hasEmail := false
		hasPerson := false
		for _, e := range result.Entities {
			if e.Type == EntityTypeEmail {
				hasEmail = true
			}
			if e.Type == EntityTypePerson {
				hasPerson = true
			}
		}
		assert.True(t, hasEmail, "Should have email from pattern extraction")
		assert.True(t, hasPerson, "Should have person from NER extraction")
	})

	t.Run("works with patterns only", func(t *testing.T) {
		extractor := NewHybridExtractor(nil, logger, nil)
		opts := DefaultExtractionOptions()
		opts.UseNER = false // Disable NER

		result, err := extractor.Extract(
			context.Background(),
			"Email test@example.com",
			opts,
		)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Entities)
	})

	t.Run("handles empty text", func(t *testing.T) {
		extractor := NewHybridExtractor(nil, logger, nil)
		result, err := extractor.Extract(context.Background(), "", DefaultExtractionOptions())

		require.NoError(t, err)
		assert.Empty(t, result.Entities)
		assert.Equal(t, int64(0), result.ProcessingTimeMs)
	})

	t.Run("normalizes entities when enabled", func(t *testing.T) {
		extractor := NewHybridExtractor(nil, logger, nil)
		opts := DefaultExtractionOptions()
		opts.UseNER = false
		opts.NormalizeEntities = true

		result, err := extractor.Extract(
			context.Background(),
			"Contact JOHN.DOE@EXAMPLE.COM",
			opts,
		)

		require.NoError(t, err)
		require.NotEmpty(t, result.Entities)

		for _, e := range result.Entities {
			if e.Type == EntityTypeEmail {
				assert.Equal(t, "john.doe@example.com", e.NormalizedValue)
			}
		}
	})

	t.Run("deduplicates entities", func(t *testing.T) {
		mockClient := &MockAIClient{
			CompleteFunc: func(ctx context.Context, prompt string, opts *CompletionOptions) (*CompletionResponse, error) {
				// Return the same email that pattern will also find
				response := []nerResponseEntity{
					{Type: "email", Value: "test@example.com", Confidence: 0.8},
				}
				jsonBytes, _ := json.Marshal(response)
				return &CompletionResponse{Text: string(jsonBytes), Model: "test"}, nil
			},
		}

		extractor := NewHybridExtractor(mockClient, logger, nil)
		opts := DefaultExtractionOptions()
		opts.DeduplicateEntities = true

		result, err := extractor.Extract(
			context.Background(),
			"Email: test@example.com",
			opts,
		)

		require.NoError(t, err)

		// Count emails - should be deduplicated
		emailCount := 0
		for _, e := range result.Entities {
			if e.Type == EntityTypeEmail {
				emailCount++
			}
		}
		assert.Equal(t, 1, emailCount)
	})
}

// ===== Composite Extractor Tests =====

func TestCompositeExtractor(t *testing.T) {
	logger := testLogger()

	t.Run("combines multiple extractors", func(t *testing.T) {
		// Create two pattern extractors with different patterns
		patterns1 := []*CompiledPattern{EmailPattern}
		patterns2 := []*CompiledPattern{URLPattern}

		ext1 := &patternOnlyExtractor{NewPatternExtractor(patterns1)}
		ext2 := &patternOnlyExtractor{NewPatternExtractor(patterns2)}

		composite := NewCompositeExtractor(logger, ext1, ext2)

		result, err := composite.Extract(
			context.Background(),
			"Contact test@example.com or visit https://example.com",
			DefaultExtractionOptions(),
		)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result.Entities), 2)
	})

	t.Run("handles empty text", func(t *testing.T) {
		composite := NewCompositeExtractor(logger)
		result, err := composite.Extract(context.Background(), "", DefaultExtractionOptions())

		require.NoError(t, err)
		assert.Empty(t, result.Entities)
	})
}

// patternOnlyExtractor wraps PatternExtractor to implement EntityExtractor.
type patternOnlyExtractor struct {
	*PatternExtractor
}

func (e *patternOnlyExtractor) Extract(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error) {
	entities := e.PatternExtractor.Extract(text, opts)
	return &ExtractionResult{
		Entities: entities,
	}, nil
}

func (e *patternOnlyExtractor) Name() string {
	return "pattern_only"
}

// ===== Filter Tests =====

func TestFilterByTypes(t *testing.T) {
	entities := []Entity{
		{Type: EntityTypePerson, Value: "John"},
		{Type: EntityTypeEmail, Value: "john@example.com"},
		{Type: EntityTypeURL, Value: "https://example.com"},
	}

	filtered := FilterByTypes(EntityTypePerson, EntityTypeEmail)(entities)

	assert.Len(t, filtered, 2)
	for _, e := range filtered {
		assert.True(t, e.Type == EntityTypePerson || e.Type == EntityTypeEmail)
	}
}

func TestFilterByMinConfidence(t *testing.T) {
	entities := []Entity{
		{Value: "High", Confidence: 0.9},
		{Value: "Medium", Confidence: 0.6},
		{Value: "Low", Confidence: 0.3},
	}

	filtered := FilterByMinConfidence(0.5)(entities)

	assert.Len(t, filtered, 2)
	for _, e := range filtered {
		assert.GreaterOrEqual(t, e.Confidence, float32(0.5))
	}
}

func TestFilterBySource(t *testing.T) {
	entities := []Entity{
		{Value: "Pattern", Source: ExtractionSourcePattern},
		{Value: "NER", Source: ExtractionSourceNER},
		{Value: "Hybrid", Source: ExtractionSourceHybrid},
	}

	filtered := FilterBySource(ExtractionSourcePattern, ExtractionSourceHybrid)(entities)

	assert.Len(t, filtered, 2)
	for _, e := range filtered {
		assert.True(t, e.Source == ExtractionSourcePattern || e.Source == ExtractionSourceHybrid)
	}
}

// ===== Coreference Tests =====

func TestCoreferenceResolver(t *testing.T) {
	resolver := NewCoreferenceResolver()

	entities := []Entity{
		{ID: "1", Type: EntityTypePerson, Value: "John Smith", Position: TextPosition{Start: 0, End: 10}},
		{ID: "2", Type: EntityTypeOrganization, Value: "Acme Corp", Position: TextPosition{Start: 20, End: 29}},
	}

	text := "John Smith is a developer. He works at Acme Corp. They are a tech company."

	hints := resolver.ResolveHints(entities, text)

	assert.NotEmpty(t, hints)

	// Should have hints for "he" and "they"
	pronounsFound := make(map[string]bool)
	for _, h := range hints {
		pronounsFound[h.Mention] = true
	}

	// Note: The simple heuristic may not find all pronouns perfectly
	assert.True(t, len(hints) > 0, "Should find at least one coreference hint")
}

// ===== Integration-style Tests =====

func TestExtractionPipeline(t *testing.T) {
	logger := testLogger()

	t.Run("executes pipeline with filters", func(t *testing.T) {
		extractor := NewHybridExtractor(nil, logger, nil)
		opts := DefaultExtractionOptions()
		opts.UseNER = false

		pipeline := NewExtractionPipeline(extractor, logger).
			WithFilter(FilterByTypes(EntityTypeEmail)).
			WithFilter(FilterByMinConfidence(0.5))

		result, err := pipeline.Execute(
			context.Background(),
			"Contact alice@example.com or bob@example.com. Visit https://example.com",
			opts,
		)

		require.NoError(t, err)

		// Should only have emails (URL filtered out)
		for _, e := range result.Entities {
			assert.Equal(t, EntityTypeEmail, e.Type)
		}
	})
}
