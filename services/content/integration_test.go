// Package main_test provides integration tests for the Content Processor components.
// These tests validate cross-component interactions and full pipeline processing.
package main_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/content/categorization"
	"github.com/otherjamesbrown/penfold/services/content/chunking"
	"github.com/otherjamesbrown/penfold/services/content/entity"
	"github.com/otherjamesbrown/penfold/services/content/summarization"
)

// =============================================================================
// Test Utilities
// =============================================================================

func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelError, // Suppress logs during tests
		ServiceName: "integration-test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// mockAIClient implements summarization.AIClient for testing.
type mockAIClient struct {
	completionFunc func(ctx context.Context, prompt string, maxTokens int) (string, error)
}

func (m *mockAIClient) GenerateCompletion(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if m.completionFunc != nil {
		return m.completionFunc(ctx, prompt, maxTokens)
	}
	// Return a generic summary
	return "This is a test summary of the provided content.", nil
}

func (m *mockAIClient) GenerateCompletionWithModel(ctx context.Context, prompt string, maxTokens int, model string) (string, error) {
	return m.GenerateCompletion(ctx, prompt, maxTokens)
}

func (m *mockAIClient) GetDefaultModel() string {
	return "test-model"
}

func (m *mockAIClient) EstimateTokens(text string) int {
	// Approximate: 1 token per 4 characters
	return len(text) / 4
}

// =============================================================================
// Full Content Pipeline Integration Tests
// =============================================================================

func TestFullContentPipeline(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	// Sample content for testing
	sampleContent := `
Dear Team,

I hope this email finds you well. I wanted to follow up on our recent project discussion
regarding the Q4 marketing campaign for Acme Corporation.

As we discussed in yesterday's meeting, the key deliverables are:
- Complete brand refresh by October 15, 2024
- Launch social media campaign by October 20, 2024
- Finalize website updates by October 25, 2024

The total budget approved is $50,000 for this initiative. Please contact John Smith
(john.smith@acme.com) or call 555-123-4567 if you have any questions.

Best regards,
Jane Doe
Marketing Director
jane.doe@acme.com
`

	t.Run("chunk then summarize", func(t *testing.T) {
		// Create chunker
		config := chunking.ChunkSizeConfig{
			MinTokens:    20,
			TargetTokens: 100,
			MaxTokens:    200,
		}
		overlap := chunking.OverlapConfig{
			OverlapTokens:     10,
			OverlapPercentage: 0.1,
		}
		chunkr := chunking.NewSentenceChunker(config, overlap, nil)

		// Chunk the content
		chunks, err := chunkr.Chunk(ctx, sampleContent, "test-content-1")
		require.NoError(t, err)
		require.NotEmpty(t, chunks)

		// Create summarizer with mock AI client
		mockClient := &mockAIClient{
			completionFunc: func(ctx context.Context, prompt string, maxTokens int) (string, error) {
				return "Summary: Email about Q4 marketing campaign for Acme Corp with key deliverables and $50K budget.", nil
			},
		}

		summarizerCfg := &summarization.Config{
			DefaultStrategy:  summarization.StrategyTypeExtractive,
			EnableCache:      false,
			MinContentLength: 50,
			MaxContentLength: 10000,
		}
		summarizer := summarization.NewSummarizer(summarizerCfg, logger, mockClient)

		// Summarize each chunk
		for _, chunk := range chunks {
			if len(chunk.Text) > 50 {
				req := &summarization.SummaryRequest{
					Content:  chunk.Text,
					Strategy: summarization.StrategyTypeExtractive,
					Length:   summarization.SummaryLengthOneLine,
				}

				result, err := summarizer.Summarize(ctx, req)
				require.NoError(t, err)
				assert.NotEmpty(t, result.Summary)
				assert.Equal(t, summarization.StrategyTypeExtractive, result.Strategy)
			}
		}
	})

	t.Run("chunk with entity extraction", func(t *testing.T) {
		// Create chunker
		config := chunking.ChunkSizeConfig{
			MinTokens:    20,
			TargetTokens: 150,
			MaxTokens:    300,
		}
		overlap := chunking.OverlapConfig{
			OverlapTokens:     15,
			OverlapPercentage: 0.1,
		}
		chunkr := chunking.NewSentenceChunker(config, overlap, nil)

		// Chunk the content
		chunks, err := chunkr.Chunk(ctx, sampleContent, "test-content-2")
		require.NoError(t, err)
		require.NotEmpty(t, chunks)

		// Create entity extractor (pattern-based, no AI)
		extractor := entity.NewHybridExtractor(nil, logger, nil)
		opts := entity.DefaultExtractionOptions()
		opts.UseNER = false // Use patterns only for testing

		// Track entities across chunks
		chunkMap := entity.NewChunkEntityMap()

		for _, chunk := range chunks {
			result, err := extractor.Extract(ctx, chunk.Text, opts)
			require.NoError(t, err)
			chunkMap.Add(chunk.ID, result.Entities)
		}

		// Get all entities and deduplicate
		allEntities, _ := chunkMap.DeduplicateAcrossChunks()

		// Verify we found expected entities
		hasEmail := false
		hasPhone := false
		hasMoney := false
		hasDate := false

		for _, e := range allEntities {
			switch e.Type {
			case entity.EntityTypeEmail:
				hasEmail = true
			case entity.EntityTypePhone:
				hasPhone = true
			case entity.EntityTypeMoney:
				hasMoney = true
			case entity.EntityTypeDate:
				hasDate = true
			}
		}

		assert.True(t, hasEmail, "Should extract email addresses")
		assert.True(t, hasPhone, "Should extract phone numbers")
		assert.True(t, hasMoney, "Should extract money amounts")
		assert.True(t, hasDate, "Should extract dates")
	})

	t.Run("entity to categorization flow", func(t *testing.T) {
		// Extract entities first
		extractor := entity.NewHybridExtractor(nil, logger, nil)
		opts := entity.DefaultExtractionOptions()
		opts.UseNER = false

		result, err := extractor.Extract(ctx, sampleContent, opts)
		require.NoError(t, err)

		// Prepare content data for categorization using entities as keywords
		keywords := make([]string, 0)
		for _, e := range result.Entities {
			keywords = append(keywords, e.Value)
		}

		contentData := &categorization.ContentData{
			Text:     sampleContent,
			Title:    "Q4 Marketing Campaign Update",
			Keywords: keywords,
		}

		// Create categorizer with email taxonomy
		taxonomy := categorization.DefaultEmailTaxonomy()
		categorizer, err := categorization.NewRuleBasedCategorizer(&categorization.RuleBasedConfig{
			Taxonomy:      taxonomy,
			Logger:        logger,
			MinConfidence: 0.3,
			MaxCategories: 5,
		})
		require.NoError(t, err)

		// Categorize content
		catResult, err := categorizer.Categorize(ctx, contentData)
		require.NoError(t, err)
		assert.NotEmpty(t, catResult.Categories)
	})
}

// =============================================================================
// Chunk to Summary Integration Tests
// =============================================================================

func TestChunkToSummary(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	// Large content that requires chunking
	largeContent := `The artificial intelligence landscape continues to evolve rapidly. Machine learning models are becoming increasingly sophisticated, enabling new applications across various industries. Natural language processing has seen particular advancement with large language models. Key developments include improved understanding of context, better reasoning capabilities, and more efficient training methods. Companies are investing heavily in AI research and development to stay competitive in this rapidly changing field. The field of computer vision has also made significant strides, with deep learning models achieving near-human performance on image recognition tasks. Autonomous vehicles rely heavily on these advances to navigate complex environments safely. Healthcare applications are transforming patient care through diagnostic imaging and predictive analytics. Financial services use AI for fraud detection and algorithmic trading strategies. Manufacturing benefits from predictive maintenance and quality control automation.`

	t.Run("chunk preserves semantic boundaries", func(t *testing.T) {
		config := chunking.ChunkSizeConfig{
			MinTokens:    50,
			TargetTokens: 150,
			MaxTokens:    300,
		}
		overlap := chunking.OverlapConfig{
			OverlapTokens:     20,
			OverlapPercentage: 0.15,
		}
		chunkr := chunking.NewSentenceChunker(config, overlap, nil)

		chunks, err := chunkr.Chunk(ctx, largeContent, "test-large")
		require.NoError(t, err)
		require.NotEmpty(t, chunks)

		// Verify chunks maintain sentence boundaries
		for _, chunk := range chunks {
			// Each chunk should end with proper punctuation or be the last chunk
			text := strings.TrimSpace(chunk.Text)
			if len(text) > 0 {
				lastChar := text[len(text)-1]
				isValidEnding := lastChar == '.' || lastChar == '!' || lastChar == '?' ||
					chunk.Index == len(chunks)-1
				assert.True(t, isValidEnding || chunk.Metadata.BoundaryType == "sentence",
					"Chunk should end with sentence boundary: %s", text)
			}
		}
	})

	t.Run("summarize chunked content", func(t *testing.T) {
		// Chunk the content
		config := chunking.ChunkSizeConfig{
			MinTokens:    50,
			TargetTokens: 200,
			MaxTokens:    400,
		}
		chunkr := chunking.NewParagraphChunker(config, chunking.OverlapConfig{}, nil)

		chunks, err := chunkr.Chunk(ctx, largeContent, "test-summary")
		require.NoError(t, err)

		// Create summarizer
		summarizer := summarization.NewSummarizer(&summarization.Config{
			DefaultStrategy:  summarization.StrategyTypeExtractive,
			EnableCache:      false,
			MinContentLength: 50,
			MaxContentLength: 10000,
		}, logger, nil)

		// Summarize each chunk
		summaries := make([]string, 0)
		for _, chunk := range chunks {
			if len(chunk.Text) >= 50 {
				req := &summarization.SummaryRequest{
					Content:  chunk.Text,
					Strategy: summarization.StrategyTypeExtractive,
					Length:   summarization.SummaryLengthOneLine,
				}

				result, err := summarizer.Summarize(ctx, req)
				require.NoError(t, err)
				summaries = append(summaries, result.Summary)
			}
		}

		// All chunks should produce summaries
		assert.NotEmpty(t, summaries)

		// Combined summary should be shorter than original
		combinedSummary := strings.Join(summaries, " ")
		assert.Less(t, len(combinedSummary), len(largeContent),
			"Combined summaries should be shorter than original content")
	})
}

// =============================================================================
// Entity Extraction Flow Integration Tests
// =============================================================================

func TestEntityExtractionFlow(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	content := `
Contact Information:
- Email: support@example.com or sales@example.org
- Phone: +1-555-123-4567 or (555) 987-6543
- Website: https://www.example.com

Financial Details:
- Total Investment: $1,250,000.00
- Monthly Budget: $50,000
- Expected ROI: 15%

Important Dates:
- Project Start: 2024-01-15
- Milestone 1: 2024-03-30
- Project End: 2024-12-31
`

	t.Run("pattern extraction comprehensive", func(t *testing.T) {
		extractor := entity.NewPatternExtractor(nil)
		opts := entity.DefaultExtractionOptions()
		opts.MinConfidence = 0.5

		entities := extractor.Extract(content, opts)
		require.NotEmpty(t, entities)

		// Count entity types
		typeCounts := make(map[entity.EntityType]int)
		for _, e := range entities {
			typeCounts[e.Type]++
		}

		// Verify expected extractions
		assert.GreaterOrEqual(t, typeCounts[entity.EntityTypeEmail], 2, "Should find 2+ emails")
		assert.GreaterOrEqual(t, typeCounts[entity.EntityTypePhone], 2, "Should find 2+ phones")
		assert.GreaterOrEqual(t, typeCounts[entity.EntityTypeURL], 1, "Should find 1+ URLs")
		assert.GreaterOrEqual(t, typeCounts[entity.EntityTypeMoney], 2, "Should find 2+ money amounts")
		assert.GreaterOrEqual(t, typeCounts[entity.EntityTypeDate], 3, "Should find 3+ dates")
	})

	t.Run("entity normalization", func(t *testing.T) {
		normalizer := entity.NewEntityNormalizer(nil)

		// Test email normalization
		emailEntity := &entity.Entity{
			Type:  entity.EntityTypeEmail,
			Value: "  SUPPORT@EXAMPLE.COM  ",
		}
		normalizer.Normalize(emailEntity)
		assert.Equal(t, "support@example.com", emailEntity.NormalizedValue)

		// Test phone normalization
		phoneEntity := &entity.Entity{
			Type:  entity.EntityTypePhone,
			Value: "(555) 123-4567",
		}
		normalizer.Normalize(phoneEntity)
		assert.NotEmpty(t, phoneEntity.NormalizedValue)
		// Normalized phone should be in E.164 format
		assert.True(t, strings.HasPrefix(phoneEntity.NormalizedValue, "+"),
			"Phone should be normalized to E.164 format")
	})

	t.Run("filter entities by type and confidence", func(t *testing.T) {
		extractor := entity.NewHybridExtractor(nil, logger, nil)
		opts := entity.DefaultExtractionOptions()
		opts.UseNER = false

		result, err := extractor.Extract(ctx, content, opts)
		require.NoError(t, err)

		// Filter to only emails
		emails := entity.FilterByTypes(entity.EntityTypeEmail)(result.Entities)
		for _, e := range emails {
			assert.Equal(t, entity.EntityTypeEmail, e.Type)
		}

		// Filter by confidence
		highConfidence := entity.FilterByMinConfidence(0.8)(result.Entities)
		for _, e := range highConfidence {
			assert.GreaterOrEqual(t, e.Confidence, float32(0.8))
		}
	})
}

// =============================================================================
// Cross-Component Integration Tests
// =============================================================================

func TestCrossComponentIntegration(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	t.Run("chunking with categorization", func(t *testing.T) {
		// Sample content
		content := `
Technical Documentation: API Integration Guide

This document describes how to integrate with our REST API. The API supports JSON
requests and responses. Authentication is done via Bearer tokens.

## Getting Started

First, obtain your API key from the developer portal. Then make requests to:
https://api.example.com/v1/

For support, contact api-support@example.com or call 1-800-555-1234.
`

		// Chunk the content
		config := chunking.ChunkSizeConfig{
			MinTokens:    30,
			TargetTokens: 100,
			MaxTokens:    200,
		}
		chunkr := chunking.NewSentenceChunker(config, chunking.OverlapConfig{}, nil)

		chunks, err := chunkr.Chunk(ctx, content, "doc-test")
		require.NoError(t, err)
		require.NotEmpty(t, chunks)

		// Categorize each chunk
		taxonomy := categorization.DefaultDocumentTaxonomy()
		categorizer, err := categorization.NewRuleBasedCategorizer(&categorization.RuleBasedConfig{
			Taxonomy:      taxonomy,
			Logger:        logger,
			MinConfidence: 0.2,
			MaxCategories: 3,
		})
		require.NoError(t, err)

		totalCategories := 0
		for _, chunk := range chunks {
			contentData := &categorization.ContentData{
				Text: chunk.Text,
			}

			result, err := categorizer.Categorize(ctx, contentData)
			require.NoError(t, err)
			totalCategories += len(result.Categories)
		}

		// Should categorize some chunks
		assert.GreaterOrEqual(t, totalCategories, 1, "Should categorize at least one chunk")
	})

	t.Run("entity extraction with summarization", func(t *testing.T) {
		content := `
Meeting Notes - January 15, 2024

Attendees: John Smith (john@example.com), Jane Doe (jane@example.com)

Action Items:
1. John to prepare budget proposal by January 20, 2024
2. Jane to contact vendor at sales@vendor.com
3. Team to review proposal, estimated cost $25,000

Next meeting: January 22, 2024 at 2:00 PM
Call-in number: 555-123-4567
`

		// Extract entities
		extractor := entity.NewHybridExtractor(nil, logger, nil)
		opts := entity.DefaultExtractionOptions()
		opts.UseNER = false

		entityResult, err := extractor.Extract(ctx, content, opts)
		require.NoError(t, err)
		require.NotEmpty(t, entityResult.Entities)

		// Summarize with entity context
		summarizer := summarization.NewSummarizer(&summarization.Config{
			DefaultStrategy:  summarization.StrategyTypeExtractive,
			EnableCache:      false,
			MinContentLength: 50,
			MaxContentLength: 10000,
		}, logger, nil)

		summaryReq := &summarization.SummaryRequest{
			Content:          content,
			Strategy:         summarization.StrategyTypeExtractive,
			Length:           summarization.SummaryLengthParagraph,
			PreserveEntities: true,
		}

		summaryResult, err := summarizer.Summarize(ctx, summaryReq)
		require.NoError(t, err)
		assert.NotEmpty(t, summaryResult.Summary)

		// Verify summary is shorter than original
		assert.Less(t, len(summaryResult.Summary), len(content),
			"Summary should be shorter than original")
	})
}

// =============================================================================
// Performance Integration Tests
// =============================================================================

func TestPerformanceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	logger := testLogger()
	ctx := context.Background()

	t.Run("chunking performance", func(t *testing.T) {
		// Generate large content
		largeContent := strings.Repeat("This is a test sentence with multiple words. ", 1000)

		config := chunking.ChunkSizeConfig{
			MinTokens:    50,
			TargetTokens: 200,
			MaxTokens:    400,
		}
		chunkr := chunking.NewSentenceChunker(config, chunking.OverlapConfig{}, nil)

		start := time.Now()
		chunks, err := chunkr.Chunk(ctx, largeContent, "perf-test")
		elapsed := time.Since(start)

		require.NoError(t, err)
		require.NotEmpty(t, chunks)

		// Should complete in reasonable time
		assert.Less(t, elapsed, 1*time.Second, "Chunking should complete within 1 second")

		t.Logf("Chunked %d characters into %d chunks in %v", len(largeContent), len(chunks), elapsed)
	})

	t.Run("entity extraction performance", func(t *testing.T) {
		// Content with many entities
		contentBuilder := strings.Builder{}
		for i := 0; i < 100; i++ {
			contentBuilder.WriteString("Contact user" + string(rune('0'+i%10)) + "@example.com ")
			contentBuilder.WriteString("or call 555-" + string(rune('0'+i%10)) + "00-" + string(rune('0'+i%10)) + "000. ")
			contentBuilder.WriteString("Total: $" + string(rune('0'+i%10)) + ",000.00. ")
			contentBuilder.WriteString("Due date: 2024-0" + string(rune('1'+i%9)) + "-15. ")
		}
		content := contentBuilder.String()

		extractor := entity.NewHybridExtractor(nil, logger, nil)
		opts := entity.DefaultExtractionOptions()
		opts.UseNER = false

		start := time.Now()
		result, err := extractor.Extract(ctx, content, opts)
		elapsed := time.Since(start)

		require.NoError(t, err)
		require.NotEmpty(t, result.Entities)

		// Should complete in reasonable time
		assert.Less(t, elapsed, 2*time.Second, "Entity extraction should complete within 2 seconds")

		t.Logf("Extracted %d entities from %d characters in %v", len(result.Entities), len(content), elapsed)
	})

	t.Run("categorization throughput", func(t *testing.T) {
		taxonomy := categorization.DefaultEmailTaxonomy()
		categorizer, err := categorization.NewRuleBasedCategorizer(&categorization.RuleBasedConfig{
			Taxonomy:      taxonomy,
			Logger:        logger,
			MinConfidence: 0.3,
		})
		require.NoError(t, err)

		contents := make([]*categorization.ContentData, 100)
		for i := range contents {
			contents[i] = &categorization.ContentData{
				Text:  "Meeting agenda for project deadline discussion with team.",
				Title: "Project Meeting",
			}
		}

		start := time.Now()
		for _, content := range contents {
			_, err := categorizer.Categorize(ctx, content)
			require.NoError(t, err)
		}
		elapsed := time.Since(start)

		throughput := float64(len(contents)) / elapsed.Seconds()
		t.Logf("Categorized %d items in %v (%.1f items/sec)", len(contents), elapsed, throughput)

		// Should achieve reasonable throughput
		assert.Greater(t, throughput, 50.0, "Should process at least 50 items per second")
	})
}

// =============================================================================
// Error Handling Integration Tests
// =============================================================================

func TestErrorHandlingIntegration(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	t.Run("empty content handling", func(t *testing.T) {
		// Chunking with empty content
		chunkr := chunking.NewSentenceChunker(
			chunking.ChunkSizeConfig{MinTokens: 10, TargetTokens: 100, MaxTokens: 200},
			chunking.OverlapConfig{},
			nil,
		)
		chunks, err := chunkr.Chunk(ctx, "", "empty-test")
		require.NoError(t, err)
		assert.Empty(t, chunks)

		// Entity extraction with empty content
		extractor := entity.NewHybridExtractor(nil, logger, nil)
		result, err := extractor.Extract(ctx, "", entity.DefaultExtractionOptions())
		require.NoError(t, err)
		assert.Empty(t, result.Entities)

		// Summarization with empty content
		summarizer := summarization.NewSummarizer(&summarization.Config{
			EnableCache:      false,
			MinContentLength: 10,
		}, logger, nil)
		_, err = summarizer.Summarize(ctx, &summarization.SummaryRequest{Content: ""})
		assert.Error(t, err)
	})

	t.Run("categorization nil content handling", func(t *testing.T) {
		taxonomy := categorization.DefaultEmailTaxonomy()
		categorizer, err := categorization.NewRuleBasedCategorizer(&categorization.RuleBasedConfig{
			Taxonomy:      taxonomy,
			Logger:        logger,
			MinConfidence: 0.3,
		})
		require.NoError(t, err)

		_, err = categorizer.Categorize(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Large content for slower processing
		largeContent := strings.Repeat("Test content for cancellation. ", 1000)

		config := chunking.ChunkSizeConfig{
			MinTokens:    10,
			TargetTokens: 50,
			MaxTokens:    100,
		}
		chunkr := chunking.NewSentenceChunker(config, chunking.OverlapConfig{}, nil)

		// Create a cancelled context
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		_, err := chunkr.Chunk(cancelledCtx, largeContent, "cancelled-test")
		// Should either complete quickly or return context error
		if err != nil {
			assert.ErrorIs(t, err, context.Canceled)
		}
	})
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkChunkingIntegration(b *testing.B) {
	ctx := context.Background()
	content := strings.Repeat("This is a test sentence for benchmarking. ", 100)

	config := chunking.ChunkSizeConfig{
		MinTokens:    50,
		TargetTokens: 200,
		MaxTokens:    400,
	}
	chunkr := chunking.NewSentenceChunker(config, chunking.OverlapConfig{}, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunkr.Chunk(ctx, content, "bench-content")
	}
}

func BenchmarkEntityExtractionIntegration(b *testing.B) {
	ctx := context.Background()
	logger := testLogger()
	content := `
Contact: test@example.com, support@test.org
Phone: 555-123-4567, (555) 987-6543
URL: https://example.com, http://test.org
Money: $1,000.00, $50,000
Dates: 2024-01-15, 2024-12-31
`

	extractor := entity.NewHybridExtractor(nil, logger, nil)
	opts := entity.DefaultExtractionOptions()
	opts.UseNER = false

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractor.Extract(ctx, content, opts)
	}
}

func BenchmarkCategorizationIntegration(b *testing.B) {
	ctx := context.Background()
	logger := testLogger()
	taxonomy := categorization.DefaultEmailTaxonomy()

	categorizer, _ := categorization.NewRuleBasedCategorizer(&categorization.RuleBasedConfig{
		Taxonomy:      taxonomy,
		Logger:        logger,
		MinConfidence: 0.3,
	})

	content := &categorization.ContentData{
		Text:  "Meeting agenda for the project deadline discussion. Please review the attached document.",
		Title: "Project Meeting",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		categorizer.Categorize(ctx, content)
	}
}

func BenchmarkSummarizationIntegration(b *testing.B) {
	ctx := context.Background()
	logger := testLogger()

	summarizer := summarization.NewSummarizer(&summarization.Config{
		DefaultStrategy:  summarization.StrategyTypeExtractive,
		EnableCache:      false,
		MinContentLength: 50,
		MaxContentLength: 10000,
	}, logger, nil)

	content := strings.Repeat("This is a test sentence for summarization benchmarking. ", 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &summarization.SummaryRequest{
			Content:  content,
			Strategy: summarization.StrategyTypeExtractive,
			Length:   summarization.SummaryLengthOneLine,
		}
		summarizer.Summarize(ctx, req)
	}
}
