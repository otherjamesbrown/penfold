package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/types/known/timestamppb"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/contentv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// testLogger creates a logger for testing.
func testLogger() logging.Logger {
	return logging.NewLogger(&logging.Config{
		Level:       logging.LevelDebug,
		ServiceName: "pipeline-test",
		Environment: "test",
		JSONFormat:  false,
	})
}

// testContentItem creates a sample content item for testing.
func testContentItem(id, content string) *contentv1.ContentItem {
	return &contentv1.ContentItem{
		Id:         id,
		SourceType: "test",
		SourceId:   "test-source-1",
		TenantId:   "tenant-1",
		RawContent: content,
		CreatedAt:  timestamppb.Now(),
		UpdatedAt:  timestamppb.Now(),
	}
}

// =============================================================================
// Pipeline Tests
// =============================================================================

func TestNewPipeline(t *testing.T) {
	logger := testLogger()
	metrics := NewPipelineMetrics("test", "content")

	t.Run("with default config", func(t *testing.T) {
		p := NewPipeline(nil, logger, metrics)
		if p == nil {
			t.Fatal("expected non-nil pipeline")
		}
		if !p.IsRunning() {
			t.Error("expected pipeline to be running")
		}
		if len(p.stageOrder) != 5 {
			t.Errorf("expected 5 default stages, got %d", len(p.stageOrder))
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &Config{
			MaxConcurrentProcessing: 5,
			StageTimeout:            time.Minute,
			StageOrder: []StageType{
				StageTypePreprocess,
				StageTypeChunk,
			},
		}
		p := NewPipeline(cfg, logger, metrics)
		if p.config.MaxConcurrentProcessing != 5 {
			t.Errorf("expected MaxConcurrentProcessing=5, got %d", p.config.MaxConcurrentProcessing)
		}
		if len(p.stageOrder) != 2 {
			t.Errorf("expected 2 stages, got %d", len(p.stageOrder))
		}
	})
}

func TestPipelineRegisterStage(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)

	// Register stages
	preprocess := NewNoOpStage("test-preprocess", StageTypePreprocess)
	chunk := NewNoOpStage("test-chunk", StageTypeChunk)

	p.RegisterStage(preprocess)
	p.RegisterStage(chunk)

	stages := p.GetStages()
	if len(stages[StageTypePreprocess]) != 1 {
		t.Errorf("expected 1 preprocess stage, got %d", len(stages[StageTypePreprocess]))
	}
	if len(stages[StageTypeChunk]) != 1 {
		t.Errorf("expected 1 chunk stage, got %d", len(stages[StageTypeChunk]))
	}
}

func TestPipelineProcess(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(DefaultConfig(), logger, nil)

	// Register all stages
	p.RegisterStage(NewPreprocessStage(nil, logger))
	p.RegisterStage(NewChunkStage(nil, logger))
	p.RegisterStage(NewAnalyzeStage(nil, logger))
	p.RegisterStage(NewScoreStage(nil, logger))
	p.RegisterStage(NewStoreStage(nil, nil, logger))

	ctx := context.Background()
	item := testContentItem("test-1", "This is a test document. It has multiple sentences. The content should be processed through all stages.")

	result, err := p.Process(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.NormalizedText == "" {
		t.Error("expected normalized text")
	}

	if len(result.Chunks) == 0 {
		t.Error("expected chunks")
	}

	if result.Analysis == nil {
		t.Error("expected analysis")
	}

	if result.Scores == nil {
		t.Error("expected scores")
	}

	if len(result.CompletedStages) != 5 {
		t.Errorf("expected 5 completed stages, got %d", len(result.CompletedStages))
	}
}

func TestPipelineProcessEmptyContent(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))

	ctx := context.Background()

	t.Run("nil item", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if !errors.Is(err, ErrEmptyContent) {
			t.Errorf("expected ErrEmptyContent, got %v", err)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		item := testContentItem("test-empty", "")
		_, err := p.Process(ctx, item)
		if !errors.Is(err, ErrEmptyContent) {
			t.Errorf("expected ErrEmptyContent, got %v", err)
		}
	})
}

func TestPipelineProcessDryRun(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))
	p.RegisterStage(NewChunkStage(nil, logger))
	p.RegisterStage(NewStoreStage(nil, nil, logger))

	ctx := context.Background()
	item := testContentItem("test-dry-run", "Test content for dry run mode.")

	result, err := p.ProcessWithOptions(ctx, item, &ProcessOptions{
		DryRun: true,
		JobID:  "dry-run-job",
	})

	if !errors.Is(err, ErrDryRunMode) {
		t.Errorf("expected ErrDryRunMode, got %v", err)
	}

	if result == nil {
		t.Fatal("expected result even in dry run mode")
	}

	if !result.DryRun {
		t.Error("expected DryRun flag to be true")
	}

	if result.JobID != "dry-run-job" {
		t.Errorf("expected job ID 'dry-run-job', got '%s'", result.JobID)
	}
}

func TestPipelineProcessSkipStages(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))
	p.RegisterStage(NewChunkStage(nil, logger))
	p.RegisterStage(NewAnalyzeStage(nil, logger))

	ctx := context.Background()
	item := testContentItem("test-skip", "Test content.")

	result, err := p.ProcessWithOptions(ctx, item, &ProcessOptions{
		SkipStages: []StageType{StageTypeAnalyze},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have completed preprocess and chunk, but not analyze
	if result.Analysis != nil {
		t.Error("expected Analysis to be nil when skipped")
	}

	if !contains(result.CompletedStages, StageTypePreprocess) {
		t.Error("expected preprocess stage to be completed")
	}
	if !contains(result.CompletedStages, StageTypeChunk) {
		t.Error("expected chunk stage to be completed")
	}
	if contains(result.CompletedStages, StageTypeAnalyze) {
		t.Error("expected analyze stage to be skipped")
	}
}

func TestPipelineProcessOnlyStages(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))
	p.RegisterStage(NewChunkStage(nil, logger))
	p.RegisterStage(NewAnalyzeStage(nil, logger))

	ctx := context.Background()
	item := testContentItem("test-only", "Test content.")

	result, err := p.ProcessWithOptions(ctx, item, &ProcessOptions{
		OnlyStages: []StageType{StageTypePreprocess},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.CompletedStages) != 1 {
		t.Errorf("expected 1 completed stage, got %d", len(result.CompletedStages))
	}
	if result.CompletedStages[0] != StageTypePreprocess {
		t.Errorf("expected preprocess stage, got %s", result.CompletedStages[0])
	}
}

func TestPipelineStageFailureAndRollback(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)

	rollbackCalled := false

	// Create a stage that tracks rollback
	successStage := NewNoOpStage("success", StageTypePreprocess).
		WithRollbackFunc(func(ctx context.Context, content *ProcessedContent) error {
			rollbackCalled = true
			return nil
		})

	// Create a failing stage
	failingStage := NewNoOpStage("failing", StageTypeChunk).
		WithProcessFunc(func(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
			return nil, errors.New("stage failure")
		})

	p.RegisterStage(successStage)
	p.RegisterStage(failingStage)

	ctx := context.Background()
	item := testContentItem("test-fail", "Test content.")

	result, err := p.Process(ctx, item)
	if err == nil {
		t.Error("expected error from failing stage")
	}

	if !errors.Is(err, ErrStageFailed) {
		t.Errorf("expected ErrStageFailed, got %v", err)
	}

	if !rollbackCalled {
		t.Error("expected rollback to be called")
	}

	if result == nil {
		t.Fatal("expected result even on failure")
	}

	if len(result.Errors) == 0 {
		t.Error("expected errors to be recorded")
	}
}

func TestPipelineStop(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))

	p.Stop()

	if p.IsRunning() {
		t.Error("expected pipeline to be stopped")
	}

	ctx := context.Background()
	item := testContentItem("test-stop", "Test content.")

	_, err := p.Process(ctx, item)
	if !errors.Is(err, ErrPipelineNotRunning) {
		t.Errorf("expected ErrPipelineNotRunning, got %v", err)
	}
}

func TestPipelineContextCancellation(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(nil, logger, nil)

	// Create a slow stage
	slowStage := NewNoOpStage("slow", StageTypePreprocess).
		WithProcessFunc(func(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return content, nil
			}
		})

	p.RegisterStage(slowStage)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	item := testContentItem("test-timeout", "Test content.")

	_, err := p.Process(ctx, item)
	// The error is wrapped, so we check if it contains a deadline exceeded error
	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline exceeded")) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestPipelineMetrics(t *testing.T) {
	logger := testLogger()
	reg := prometheus.NewRegistry()
	metrics := NewPipelineMetrics("test", "content")

	if err := metrics.RegisterMetrics(reg); err != nil {
		t.Fatalf("failed to register metrics: %v", err)
	}

	p := NewPipeline(nil, logger, metrics)
	p.RegisterStage(NewPreprocessStage(nil, logger))

	ctx := context.Background()
	item := testContentItem("test-metrics", "Test content for metrics.")

	_, err := p.Process(ctx, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify metrics were recorded
	// Note: In a real test, you might gather metrics and check values
}

// =============================================================================
// Preprocess Stage Tests
// =============================================================================

func TestPreprocessStage(t *testing.T) {
	logger := testLogger()
	stage := NewPreprocessStage(nil, logger)

	if stage.Name() != "preprocess" {
		t.Errorf("expected name 'preprocess', got '%s'", stage.Name())
	}
	if stage.Type() != StageTypePreprocess {
		t.Errorf("expected type %s, got %s", StageTypePreprocess, stage.Type())
	}
}

func TestPreprocessStageStripHTML(t *testing.T) {
	logger := testLogger()
	stage := NewPreprocessStage(&PreprocessConfig{
		StripHTML:           true,
		NormalizeWhitespace: true,
		TrimContent:         true,
	}, logger)

	ctx := context.Background()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple HTML",
			input:    "<p>Hello World</p>",
			expected: "Hello World",
		},
		{
			name:     "nested HTML",
			input:    "<div><p>Hello</p><p>World</p></div>",
			expected: "Hello World",
		},
		{
			name:     "HTML with script",
			input:    "<p>Hello</p><script>alert('x')</script><p>World</p>",
			expected: "Hello World",
		},
		{
			name:     "HTML entities",
			input:    "<p>Hello &amp; World &lt;test&gt;</p>",
			expected: "Hello & World <test>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := &ProcessedContent{
				Item: testContentItem("test", tt.input),
			}

			result, err := stage.Process(ctx, content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Normalize whitespace for comparison
			got := strings.TrimSpace(result.NormalizedText)
			want := strings.TrimSpace(tt.expected)
			if got != want {
				t.Errorf("expected '%s', got '%s'", want, got)
			}
		})
	}
}

func TestPreprocessStageNormalizeWhitespace(t *testing.T) {
	logger := testLogger()
	stage := NewPreprocessStage(&PreprocessConfig{
		NormalizeWhitespace: true,
		PreserveStructure:   false,
		TrimContent:         true,
	}, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item: testContentItem("test", "Hello   World\n\n\nTest"),
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Hello World Test"
	if result.NormalizedText != expected {
		t.Errorf("expected '%s', got '%s'", expected, result.NormalizedText)
	}
}

func TestPreprocessStagePreserveStructure(t *testing.T) {
	logger := testLogger()
	stage := NewPreprocessStage(&PreprocessConfig{
		NormalizeWhitespace: true,
		PreserveStructure:   true,
		TrimContent:         true,
	}, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item: testContentItem("test", "Paragraph one.\n\nParagraph two."),
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.NormalizedText, "\n") {
		t.Error("expected paragraph breaks to be preserved")
	}
}

func TestPreprocessStageMaxLength(t *testing.T) {
	logger := testLogger()
	stage := NewPreprocessStage(&PreprocessConfig{
		MaxLength:   20,
		TrimContent: true,
	}, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item: testContentItem("test", "This is a very long text that should be truncated"),
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.NormalizedText) > 25 { // Allow for "..."
		t.Errorf("expected truncated text, got length %d", len(result.NormalizedText))
	}
}

// =============================================================================
// Chunk Stage Tests
// =============================================================================

func TestChunkStage(t *testing.T) {
	logger := testLogger()
	stage := NewChunkStage(nil, logger)

	if stage.Name() != "chunk" {
		t.Errorf("expected name 'chunk', got '%s'", stage.Name())
	}
	if stage.Type() != StageTypeChunk {
		t.Errorf("expected type %s, got %s", StageTypeChunk, stage.Type())
	}
}

func TestChunkStageSmallContent(t *testing.T) {
	logger := testLogger()
	stage := NewChunkStage(&ChunkConfig{
		MaxChunkSize: 1000,
		MinChunkSize: 50,
	}, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item:           testContentItem("test", "Small content"),
		NormalizedText: "Small content",
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(result.Chunks))
	}
}

func TestChunkStageLargeContent(t *testing.T) {
	logger := testLogger()
	stage := NewChunkStage(&ChunkConfig{
		MaxChunkSize:   100,
		MinChunkSize:   10,
		ChunkOverlap:   20,
		SplitOnSentences: true,
	}, logger)

	ctx := context.Background()

	// Create content larger than MaxChunkSize
	text := "First sentence here. Second sentence there. Third sentence follows. Fourth sentence continues. Fifth sentence ends."
	content := &ProcessedContent{
		Item:           testContentItem("test", text),
		NormalizedText: text,
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(result.Chunks))
	}

	// Verify chunk IDs are set
	for i, chunk := range result.Chunks {
		if chunk.ID == "" {
			t.Errorf("chunk %d has no ID", i)
		}
		if chunk.Index != i {
			t.Errorf("chunk %d has wrong index: %d", i, chunk.Index)
		}
	}
}

func TestChunkStageParagraphBased(t *testing.T) {
	logger := testLogger()
	stage := NewChunkStage(&ChunkConfig{
		MaxChunkSize:      200,
		MinChunkSize:      20,
		SplitOnParagraphs: true,
	}, logger)

	ctx := context.Background()

	text := "First paragraph with some content.\n\nSecond paragraph with more content.\n\nThird paragraph with even more content."
	content := &ProcessedContent{
		Item:           testContentItem("test", text),
		NormalizedText: text,
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Chunks) < 1 {
		t.Error("expected at least 1 chunk")
	}
}

// =============================================================================
// Analyze Stage Tests
// =============================================================================

func TestAnalyzeStage(t *testing.T) {
	logger := testLogger()
	stage := NewAnalyzeStage(nil, logger)

	if stage.Name() != "analyze" {
		t.Errorf("expected name 'analyze', got '%s'", stage.Name())
	}
	if stage.Type() != StageTypeAnalyze {
		t.Errorf("expected type %s, got %s", StageTypeAnalyze, stage.Type())
	}
}

func TestAnalyzeStageLanguageDetection(t *testing.T) {
	logger := testLogger()
	stage := NewAnalyzeStage(&AnalyzeConfig{
		DetectLanguage: true,
	}, logger)

	ctx := context.Background()

	// English text
	content := &ProcessedContent{
		Item:           testContentItem("test", "This is English text."),
		NormalizedText: "This is English text with some more words to analyze.",
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Analysis == nil {
		t.Fatal("expected analysis result")
	}

	if result.Analysis.Language != "en" {
		t.Errorf("expected language 'en', got '%s'", result.Analysis.Language)
	}
}

func TestAnalyzeStageEntityExtraction(t *testing.T) {
	logger := testLogger()
	stage := NewAnalyzeStage(&AnalyzeConfig{
		ExtractEntities:     true,
		MinEntityConfidence: 0.5,
	}, logger)

	ctx := context.Background()

	text := "Contact john@example.com or call 555-123-4567. Meeting on 2024-01-15."
	content := &ProcessedContent{
		Item:           testContentItem("test", text),
		NormalizedText: text,
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Analysis == nil {
		t.Fatal("expected analysis result")
	}

	// Check for email entity
	hasEmail := false
	hasPhone := false
	hasDate := false

	for _, entity := range result.Analysis.Entities {
		switch entity.Type {
		case "email":
			hasEmail = true
			if entity.Value != "john@example.com" {
				t.Errorf("expected email 'john@example.com', got '%s'", entity.Value)
			}
		case "phone":
			hasPhone = true
		case "date":
			hasDate = true
		}
	}

	if !hasEmail {
		t.Error("expected email entity to be extracted")
	}
	if !hasPhone {
		t.Error("expected phone entity to be extracted")
	}
	if !hasDate {
		t.Error("expected date entity to be extracted")
	}
}

func TestAnalyzeStageKeywordExtraction(t *testing.T) {
	logger := testLogger()
	stage := NewAnalyzeStage(&AnalyzeConfig{
		ExtractKeywords: true,
		MaxKeywords:     5,
	}, logger)

	ctx := context.Background()

	text := "Machine learning is a subset of artificial intelligence. Machine learning algorithms learn from data. Machine learning is used in many applications."
	content := &ProcessedContent{
		Item:           testContentItem("test", text),
		NormalizedText: text,
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Analysis == nil {
		t.Fatal("expected analysis result")
	}

	if len(result.Analysis.Keywords) == 0 {
		t.Error("expected keywords to be extracted")
	}

	// "machine" and "learning" should be top keywords
	hasKeyword := false
	for _, kw := range result.Analysis.Keywords {
		if kw == "machine" || kw == "learning" {
			hasKeyword = true
			break
		}
	}
	if !hasKeyword {
		t.Errorf("expected 'machine' or 'learning' in keywords, got %v", result.Analysis.Keywords)
	}
}

func TestAnalyzeStageContentTypeDetection(t *testing.T) {
	logger := testLogger()
	stage := NewAnalyzeStage(nil, logger)

	ctx := context.Background()

	tests := []struct {
		name         string
		text         string
		expectedType string
	}{
		{
			name:         "email detection",
			text:         "Dear John, I wanted to follow up on our meeting. Best regards, Jane",
			expectedType: "email",
		},
		{
			name:         "meeting detection",
			text:         "Meeting Agenda: 1. Review Q1 results. Attendees: John, Jane, Bob",
			expectedType: "meeting",
		},
		{
			name:         "document fallback",
			text:         "This is just a regular document with some text content.",
			expectedType: "document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create item with empty SourceType to trigger detection
			item := &contentv1.ContentItem{
				Id:         "test",
				SourceType: "", // Empty to allow auto-detection
				SourceId:   "test-source-1",
				TenantId:   "tenant-1",
				RawContent: tt.text,
				CreatedAt:  timestamppb.Now(),
				UpdatedAt:  timestamppb.Now(),
			}
			content := &ProcessedContent{
				Item:           item,
				NormalizedText: tt.text,
			}

			result, err := stage.Process(ctx, content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Analysis.ContentType != tt.expectedType {
				t.Errorf("expected content type '%s', got '%s'", tt.expectedType, result.Analysis.ContentType)
			}
		})
	}
}

// =============================================================================
// Score Stage Tests
// =============================================================================

func TestScoreStage(t *testing.T) {
	logger := testLogger()
	stage := NewScoreStage(nil, logger)

	if stage.Name() != "score" {
		t.Errorf("expected name 'score', got '%s'", stage.Name())
	}
	if stage.Type() != StageTypeScore {
		t.Errorf("expected type %s, got %s", StageTypeScore, stage.Type())
	}
}

func TestScoreStageBasicScoring(t *testing.T) {
	logger := testLogger()
	stage := NewScoreStage(nil, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item:           testContentItem("test", "Test content"),
		NormalizedText: "This is test content with reasonable length for scoring.",
		Chunks:         []Chunk{{ID: "chunk-1", Text: "chunk content"}},
		Analysis: &AnalysisResult{
			Language:         "en",
			DetectedLanguage: 0.9,
			Entities:         []Entity{{Type: "email", Value: "test@example.com"}},
		},
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Scores == nil {
		t.Fatal("expected scores result")
	}

	// Check that all scores are in valid range
	if result.Scores.ImportanceScore < 0 || result.Scores.ImportanceScore > 1 {
		t.Errorf("importance score out of range: %f", result.Scores.ImportanceScore)
	}
	if result.Scores.QualityScore < 0 || result.Scores.QualityScore > 1 {
		t.Errorf("quality score out of range: %f", result.Scores.QualityScore)
	}
	if result.Scores.RecencyScore < 0 || result.Scores.RecencyScore > 1 {
		t.Errorf("recency score out of range: %f", result.Scores.RecencyScore)
	}
	if result.Scores.OverallScore < 0 || result.Scores.OverallScore > 1 {
		t.Errorf("overall score out of range: %f", result.Scores.OverallScore)
	}
}

func TestScoreStageRecencyDecay(t *testing.T) {
	logger := testLogger()
	stage := NewScoreStage(&ScoreConfig{
		RecencyDecayDays: 30,
		RecencyWeight:    1.0, // Only use recency for this test
	}, logger)

	ctx := context.Background()

	// Recent content
	recentItem := testContentItem("recent", "content")
	recentItem.CreatedAt = timestamppb.Now()

	recentContent := &ProcessedContent{
		Item:           recentItem,
		NormalizedText: "content",
	}

	recentResult, _ := stage.Process(ctx, recentContent)

	// Old content
	oldItem := testContentItem("old", "content")
	oldTime := time.Now().Add(-60 * 24 * time.Hour) // 60 days ago
	oldItem.CreatedAt = timestamppb.New(oldTime)

	oldContent := &ProcessedContent{
		Item:           oldItem,
		NormalizedText: "content",
	}

	oldResult, _ := stage.Process(ctx, oldContent)

	if recentResult.Scores.RecencyScore <= oldResult.Scores.RecencyScore {
		t.Error("recent content should have higher recency score than old content")
	}
}

// =============================================================================
// Store Stage Tests
// =============================================================================

func TestStoreStage(t *testing.T) {
	logger := testLogger()
	stage := NewStoreStage(nil, nil, logger)

	if stage.Name() != "store" {
		t.Errorf("expected name 'store', got '%s'", stage.Name())
	}
	if stage.Type() != StageTypeStore {
		t.Errorf("expected type %s, got %s", StageTypeStore, stage.Type())
	}
}

func TestStoreStageDryRun(t *testing.T) {
	logger := testLogger()

	// Create a mock store that fails if called
	mockStore := &mockContentStore{
		storeFunc: func(ctx context.Context, content *ProcessedContent) error {
			t.Error("store should not be called in dry run mode")
			return errors.New("should not be called")
		},
	}

	stage := NewStoreStage(nil, mockStore, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item:   testContentItem("test", "content"),
		DryRun: true,
	}

	_, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStoreStageWithStore(t *testing.T) {
	logger := testLogger()

	storeCalled := false
	chunksCalled := false

	mockStore := &mockContentStore{
		storeFunc: func(ctx context.Context, content *ProcessedContent) error {
			storeCalled = true
			return nil
		},
		storeChunksFunc: func(ctx context.Context, contentID string, chunks []Chunk) error {
			chunksCalled = true
			return nil
		},
	}

	stage := NewStoreStage(&StoreConfig{
		StoreChunks: true,
	}, mockStore, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item:   testContentItem("test", "content"),
		DryRun: false,
		Chunks: []Chunk{{ID: "chunk-1", Text: "chunk"}},
	}

	_, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !storeCalled {
		t.Error("expected store to be called")
	}
	if !chunksCalled {
		t.Error("expected storeChunks to be called")
	}
}

func TestStoreStageRollback(t *testing.T) {
	logger := testLogger()

	deleteCalled := false
	mockStore := &mockContentStore{
		deleteFunc: func(ctx context.Context, contentID string) error {
			deleteCalled = true
			if contentID != "test-id" {
				t.Errorf("expected contentID 'test-id', got '%s'", contentID)
			}
			return nil
		},
	}

	stage := NewStoreStage(nil, mockStore, logger)

	ctx := context.Background()
	content := &ProcessedContent{
		Item:   testContentItem("test-id", "content"),
		DryRun: false,
	}

	err := stage.Rollback(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("expected delete to be called during rollback")
	}
}

// =============================================================================
// NoOp Stage Tests
// =============================================================================

func TestNoOpStage(t *testing.T) {
	stage := NewNoOpStage("test-noop", StageTypePreprocess)

	if stage.Name() != "test-noop" {
		t.Errorf("expected name 'test-noop', got '%s'", stage.Name())
	}

	ctx := context.Background()
	content := &ProcessedContent{
		Item:           testContentItem("test", "content"),
		NormalizedText: "content",
	}

	result, err := stage.Process(ctx, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != content {
		t.Error("expected content to be passed through unchanged")
	}
}

func TestNoOpStageCustomFunctions(t *testing.T) {
	processCalled := false
	rollbackCalled := false

	stage := NewNoOpStage("custom", StageTypeAnalyze).
		WithProcessFunc(func(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
			processCalled = true
			return content, nil
		}).
		WithRollbackFunc(func(ctx context.Context, content *ProcessedContent) error {
			rollbackCalled = true
			return nil
		}).
		WithParallel(true)

	ctx := context.Background()
	content := &ProcessedContent{}

	stage.Process(ctx, content)
	if !processCalled {
		t.Error("expected custom process function to be called")
	}

	stage.Rollback(ctx, content)
	if !rollbackCalled {
		t.Error("expected custom rollback function to be called")
	}

	if !stage.SupportsParallel() {
		t.Error("expected stage to support parallel execution")
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestFullPipelineIntegration(t *testing.T) {
	logger := testLogger()
	p := NewPipeline(DefaultConfig(), logger, nil)

	// Register all stages
	p.RegisterStage(NewPreprocessStage(DefaultPreprocessConfig(), logger))
	p.RegisterStage(NewChunkStage(DefaultChunkConfig(), logger))
	p.RegisterStage(NewAnalyzeStage(DefaultAnalyzeConfig(), logger))
	p.RegisterStage(NewScoreStage(DefaultScoreConfig(), logger))
	p.RegisterStage(NewStoreStage(DefaultStoreConfig(), nil, logger))

	ctx := context.Background()

	// Create a realistic content item
	htmlContent := `
		<html>
		<head><title>Test Document</title></head>
		<body>
			<h1>Important Meeting Notes</h1>
			<p>Dear Team,</p>
			<p>Please find below the notes from our meeting on 2024-01-15.</p>
			<p>Attendees: john@example.com, jane@example.com</p>
			<p>Key discussion points:</p>
			<ul>
				<li>Project timeline review</li>
				<li>Budget allocation for Q2</li>
				<li>Resource planning</li>
			</ul>
			<p>Next steps: Schedule follow-up meeting. Contact 555-123-4567 for questions.</p>
			<p>Best regards,<br>Meeting Organizer</p>
		</body>
		</html>
	`

	item := testContentItem("integration-test", htmlContent)
	item.SourceType = "meeting"

	result, err := p.Process(ctx, item)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	// Verify all stages completed
	if len(result.CompletedStages) != 5 {
		t.Errorf("expected 5 completed stages, got %d: %v", len(result.CompletedStages), result.CompletedStages)
	}

	// Verify HTML was stripped
	if strings.Contains(result.NormalizedText, "<") {
		t.Error("expected HTML to be stripped")
	}

	// Verify chunks were created
	if len(result.Chunks) == 0 {
		t.Error("expected chunks to be created")
	}

	// Verify analysis was performed
	if result.Analysis == nil {
		t.Error("expected analysis results")
	}
	if len(result.Analysis.Entities) == 0 {
		t.Error("expected entities to be extracted")
	}

	// Verify scores were calculated
	if result.Scores == nil {
		t.Error("expected scores")
	}
	if result.Scores.OverallScore <= 0 {
		t.Error("expected positive overall score")
	}

	// Log results for manual verification
	t.Logf("Normalized text length: %d", len(result.NormalizedText))
	t.Logf("Number of chunks: %d", len(result.Chunks))
	t.Logf("Language detected: %s (confidence: %.2f)", result.Analysis.Language, result.Analysis.DetectedLanguage)
	t.Logf("Entities found: %d", len(result.Analysis.Entities))
	t.Logf("Keywords: %v", result.Analysis.Keywords)
	t.Logf("Overall score: %.2f", result.Scores.OverallScore)
}

func TestConcurrentProcessing(t *testing.T) {
	logger := testLogger()
	cfg := DefaultConfig()
	cfg.MaxConcurrentProcessing = 5

	p := NewPipeline(cfg, logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))
	p.RegisterStage(NewChunkStage(nil, logger))

	ctx := context.Background()
	numItems := 20

	// Process items concurrently
	results := make(chan error, numItems)

	for i := 0; i < numItems; i++ {
		go func(idx int) {
			item := testContentItem(
				fmt.Sprintf("concurrent-%d", idx),
				fmt.Sprintf("Content item %d with some text to process.", idx),
			)
			_, err := p.Process(ctx, item)
			results <- err
		}(i)
	}

	// Collect results
	var errors []error
	for i := 0; i < numItems; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("concurrent processing had %d errors: %v", len(errors), errors)
	}
}

// =============================================================================
// Mock Implementations
// =============================================================================

// mockContentStore implements ContentStore for testing.
type mockContentStore struct {
	storeFunc       func(ctx context.Context, content *ProcessedContent) error
	storeChunksFunc func(ctx context.Context, contentID string, chunks []Chunk) error
	deleteFunc      func(ctx context.Context, contentID string) error
}

func (m *mockContentStore) Store(ctx context.Context, content *ProcessedContent) error {
	if m.storeFunc != nil {
		return m.storeFunc(ctx, content)
	}
	return nil
}

func (m *mockContentStore) StoreChunks(ctx context.Context, contentID string, chunks []Chunk) error {
	if m.storeChunksFunc != nil {
		return m.storeChunksFunc(ctx, contentID, chunks)
	}
	return nil
}

func (m *mockContentStore) Delete(ctx context.Context, contentID string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, contentID)
	}
	return nil
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkPreprocessStage(b *testing.B) {
	logger := testLogger()
	stage := NewPreprocessStage(DefaultPreprocessConfig(), logger)
	ctx := context.Background()

	// Create test content with HTML
	htmlContent := strings.Repeat("<p>This is a paragraph with some text.</p>", 100)
	item := testContentItem("bench", htmlContent)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		content := &ProcessedContent{Item: item}
		stage.Process(ctx, content)
	}
}

func BenchmarkChunkStage(b *testing.B) {
	logger := testLogger()
	stage := NewChunkStage(DefaultChunkConfig(), logger)
	ctx := context.Background()

	// Create large text content
	text := strings.Repeat("This is a sentence. ", 500)
	item := testContentItem("bench", text)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		content := &ProcessedContent{
			Item:           item,
			NormalizedText: text,
		}
		stage.Process(ctx, content)
	}
}

func BenchmarkAnalyzeStage(b *testing.B) {
	logger := testLogger()
	stage := NewAnalyzeStage(DefaultAnalyzeConfig(), logger)
	ctx := context.Background()

	text := "Contact john@example.com for more info. Call 555-123-4567. Meeting on 2024-01-15."
	text = strings.Repeat(text+" ", 50)
	item := testContentItem("bench", text)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		content := &ProcessedContent{
			Item:           item,
			NormalizedText: text,
		}
		stage.Process(ctx, content)
	}
}

func BenchmarkFullPipeline(b *testing.B) {
	logger := testLogger()
	p := NewPipeline(DefaultConfig(), logger, nil)
	p.RegisterStage(NewPreprocessStage(nil, logger))
	p.RegisterStage(NewChunkStage(nil, logger))
	p.RegisterStage(NewAnalyzeStage(nil, logger))
	p.RegisterStage(NewScoreStage(nil, logger))
	p.RegisterStage(NewStoreStage(nil, nil, logger))

	ctx := context.Background()
	htmlContent := strings.Repeat("<p>Paragraph with content. Contact test@example.com.</p>", 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item := testContentItem(fmt.Sprintf("bench-%d", i), htmlContent)
		p.Process(ctx, item)
	}
}
