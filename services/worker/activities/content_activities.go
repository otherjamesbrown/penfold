// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.temporal.io/sdk/activity"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ContentActivities holds dependencies for content processing activities.
type ContentActivities struct {
	logger        logging.Logger
	aiClient      AIClient
	contentRepo   ContentRepository
	embeddingRepo EmbeddingRepository
	entityRepo    EntityRepository
	summaryRepo   SummaryRepository
}

// NewContentActivities creates a new ContentActivities instance.
func NewContentActivities(
	logger logging.Logger,
	aiClient AIClient,
	contentRepo ContentRepository,
	embeddingRepo EmbeddingRepository,
	entityRepo EntityRepository,
	summaryRepo SummaryRepository,
) *ContentActivities {
	return &ContentActivities{
		logger:        logger.With(logging.F("component", "content_activities")),
		aiClient:      aiClient,
		contentRepo:   contentRepo,
		embeddingRepo: embeddingRepo,
		entityRepo:    entityRepo,
		summaryRepo:   summaryRepo,
	}
}

// ChunkContentInput is the input for the ChunkContent activity.
type ChunkContentInput struct {
	TenantID    string `json:"tenant_id"`
	SourceID    int64  `json:"source_id"`
	JobID       string `json:"job_id"`
	Content     string `json:"content"`
	ContentType string `json:"content_type"`
	ChunkSize   int    `json:"chunk_size"`   // Target chunk size in characters
	ChunkOverlap int   `json:"chunk_overlap"` // Overlap between chunks
}

// ChunkContentOutput is the output from the ChunkContent activity.
type ChunkContentOutput struct {
	Chunks      []ContentChunk `json:"chunks"`
	TotalChunks int            `json:"total_chunks"`
	TotalChars  int            `json:"total_chars"`
}

// ContentChunk represents a single chunk of content.
type ContentChunk struct {
	Index       int    `json:"index"`
	Content     string `json:"content"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
	TokenCount  int    `json:"token_count,omitempty"`
}

// ChunkContentActivity chunks content into smaller pieces for processing.
// This is useful for handling large documents that exceed LLM context limits.
func (a *ContentActivities) ChunkContentActivity(ctx context.Context, input ChunkContentInput) (*ChunkContentOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ChunkContentActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting content chunking")

	logger.Info("Chunking content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Content == "" {
		return &ChunkContentOutput{
			Chunks:      []ContentChunk{},
			TotalChunks: 0,
			TotalChars:  0,
		}, nil
	}

	// Set defaults
	chunkSize := input.ChunkSize
	if chunkSize == 0 {
		chunkSize = 1000 // Default 1000 characters
	}
	chunkOverlap := input.ChunkOverlap
	if chunkOverlap == 0 {
		chunkOverlap = 100 // Default 100 character overlap
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 10 // Overlap should be less than chunk size
	}

	startTime := time.Now()

	// Split content into chunks
	chunks := splitIntoChunks(input.Content, chunkSize, chunkOverlap)

	output := &ChunkContentOutput{
		Chunks:      chunks,
		TotalChunks: len(chunks),
		TotalChars:  utf8.RuneCountInString(input.Content),
	}

	logger.Info("Content chunking completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("total_chunks", output.TotalChunks),
		logging.F("total_chars", output.TotalChars),
	)

	return output, nil
}

// splitIntoChunks splits content into overlapping chunks.
func splitIntoChunks(content string, chunkSize, overlap int) []ContentChunk {
	runes := []rune(content)
	totalLen := len(runes)

	if totalLen <= chunkSize {
		return []ContentChunk{{
			Index:       0,
			Content:     content,
			StartOffset: 0,
			EndOffset:   totalLen,
		}}
	}

	chunks := make([]ContentChunk, 0)
	step := chunkSize - overlap
	if step < 1 {
		step = 1
	}

	index := 0
	for start := 0; start < totalLen; start += step {
		end := start + chunkSize
		if end > totalLen {
			end = totalLen
		}

		// Try to break at sentence boundaries
		chunkRunes := runes[start:end]
		chunkStr := string(chunkRunes)

		// If we're not at the end, try to find a good break point
		if end < totalLen {
			// Look for sentence endings near the end of the chunk
			lastSentence := findLastSentenceBreak(chunkStr)
			if lastSentence > chunkSize/2 {
				chunkStr = chunkStr[:lastSentence]
				end = start + lastSentence
			}
		}

		chunks = append(chunks, ContentChunk{
			Index:       index,
			Content:     chunkStr,
			StartOffset: start,
			EndOffset:   end,
		})

		index++

		// Stop if we've reached the end
		if end >= totalLen {
			break
		}
	}

	return chunks
}

// findLastSentenceBreak finds the position of the last sentence ending.
func findLastSentenceBreak(s string) int {
	lastBreak := -1
	for i := len(s) - 1; i >= len(s)/2; i-- {
		if i < len(s) && (s[i] == '.' || s[i] == '!' || s[i] == '?') {
			// Check if followed by space or end
			if i+1 >= len(s) || s[i+1] == ' ' || s[i+1] == '\n' {
				lastBreak = i + 1
				break
			}
		}
	}
	return lastBreak
}

// ExtractEntitiesActivityInput is the input for the ExtractEntitiesActivity activity.
type ExtractEntitiesActivityInput struct {
	TenantID    string   `json:"tenant_id"`
	SourceID    int64    `json:"source_id"`
	JobID       string   `json:"job_id"`
	Content     string   `json:"content"`
	EntityTypes []string `json:"entity_types,omitempty"` // Filter by types: person, organization, location, etc.
}

// ExtractEntitiesActivityOutput is the output from the ExtractEntitiesActivity activity.
type ExtractEntitiesActivityOutput struct {
	EntityCount int               `json:"entity_count"`
	Entities    []ExtractedEntity `json:"entities"`
	ModelUsed   string            `json:"model_used"`
}

// ExtractEntitiesActivity extracts named entities from content.
// This activity identifies people, organizations, locations, dates, and other entities.
func (a *ContentActivities) ExtractEntitiesActivity(ctx context.Context, input ExtractEntitiesActivityInput) (*ExtractEntitiesActivityOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ExtractEntitiesActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting entity extraction")

	logger.Info("Extracting entities from content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Content == "" {
		return &ExtractEntitiesActivityOutput{
			EntityCount: 0,
			Entities:    []ExtractedEntity{},
		}, nil
	}

	// Check if AI client is available
	if a.aiClient == nil {
		return nil, NewConfigurationError("AI client not configured")
	}

	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service for entity extraction")

	// Use assertion extraction as proxy for entity extraction
	// In production, this would use a dedicated NER model
	minConfidence := float32(0.6)
	maxAssertions := int32(100)

	req := &aiv1.AssertionRequest{
		Content:       input.Content,
		MinConfidence: &minConfidence,
		MaxAssertions: &maxAssertions,
		TenantId:      &input.TenantID,
	}

	resp, err := a.aiClient.ExtractAssertions(ctx, req)
	if err != nil {
		logger.Error("Failed to extract entities from AI service", logging.Err(err))
		return nil, fmt.Errorf("failed to extract entities: %w", err)
	}

	activity.RecordHeartbeat(ctx, "processing extraction results")

	// Extract unique entities from assertions
	entityMap := make(map[string]*ExtractedEntity)
	for _, assertion := range resp.Assertions {
		// Add subject as entity
		if assertion.Subject != "" {
			key := strings.ToLower(assertion.Subject)
			if existing, ok := entityMap[key]; ok {
				if assertion.Confidence > existing.Confidence {
					existing.Confidence = assertion.Confidence
				}
			} else {
				entityMap[key] = &ExtractedEntity{
					Name:       assertion.Subject,
					Type:       inferEntityTypeFromAssertion(assertion),
					Confidence: assertion.Confidence,
				}
			}
		}
		// Add object as entity
		if assertion.Object != "" {
			key := strings.ToLower(assertion.Object)
			if existing, ok := entityMap[key]; ok {
				if assertion.Confidence > existing.Confidence {
					existing.Confidence = assertion.Confidence
				}
			} else {
				entityMap[key] = &ExtractedEntity{
					Name:       assertion.Object,
					Type:       inferEntityTypeFromAssertion(assertion),
					Confidence: assertion.Confidence,
				}
			}
		}
	}

	// Filter by entity types if specified
	entities := make([]ExtractedEntity, 0, len(entityMap))
	for _, e := range entityMap {
		if len(input.EntityTypes) == 0 || containsString(input.EntityTypes, e.Type) {
			entities = append(entities, *e)
		}
	}

	// Store entities if repository is available
	if a.entityRepo != nil && len(entities) > 0 {
		entityPtrs := make([]*Entity, len(entities))
		for i := range entities {
			entityPtrs[i] = &Entity{
				Name:       entities[i].Name,
				Type:       entities[i].Type,
				Confidence: entities[i].Confidence,
			}
		}
		if _, err := a.entityRepo.StoreEntities(ctx, input.TenantID, input.SourceID, entityPtrs); err != nil {
			logger.Warn("Failed to store entities", logging.Err(err))
		}
	}

	output := &ExtractEntitiesActivityOutput{
		EntityCount: len(entities),
		Entities:    entities,
		ModelUsed:   resp.ModelUsed,
	}

	logger.Info("Entity extraction completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("entity_count", output.EntityCount),
		logging.F("model_used", output.ModelUsed),
	)

	return output, nil
}

// inferEntityTypeFromAssertion infers entity type from assertion data.
func inferEntityTypeFromAssertion(assertion *aiv1.Assertion) string {
	category := assertion.GetCategory()
	switch strings.ToLower(category) {
	case "person", "personnel":
		return "person"
	case "organization", "company", "org":
		return "organization"
	case "location", "place", "geographical":
		return "location"
	case "date", "time", "temporal":
		return "date"
	case "product", "service":
		return "product"
	case "event":
		return "event"
	}
	return "unknown"
}

// CategorizeContentInput is the input for the CategorizeContent activity.
type CategorizeContentInput struct {
	TenantID   string   `json:"tenant_id"`
	SourceID   int64    `json:"source_id"`
	JobID      string   `json:"job_id"`
	Content    string   `json:"content"`
	Categories []string `json:"categories,omitempty"` // Optional predefined categories
}

// CategorizeContentOutput is the output from the CategorizeContent activity.
type CategorizeContentOutput struct {
	PrimaryCategory   string               `json:"primary_category"`
	SecondaryCategories []string           `json:"secondary_categories,omitempty"`
	Confidence        float32              `json:"confidence"`
	Scores            []CategoryScore      `json:"scores"`
	ModelUsed         string               `json:"model_used"`
}

// CategoryScore represents a category with its confidence score.
type CategoryScore struct {
	Category   string  `json:"category"`
	Score      float32 `json:"score"`
}

// CategorizeContentActivity categorizes content into predefined or inferred categories.
// This is useful for organizing and routing content.
func (a *ContentActivities) CategorizeContentActivity(ctx context.Context, input CategorizeContentInput) (*CategorizeContentOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "CategorizeContentActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting content categorization")

	logger.Info("Categorizing content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Content == "" {
		return &CategorizeContentOutput{
			PrimaryCategory: "uncategorized",
			Confidence:      0,
			Scores:          []CategoryScore{},
		}, nil
	}

	// Check if AI client is available
	if a.aiClient == nil {
		return nil, NewConfigurationError("AI client not configured")
	}

	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service for categorization")

	// Use summary generation with a categorization prompt
	// In production, this would use a dedicated classification model
	maxLength := int32(50)
	req := &aiv1.SummaryRequest{
		Content:   "Categorize this content into one of these categories: work, personal, finance, health, travel, technology, news, social. Content: " + truncateContent(input.Content, 2000),
		MaxLength: &maxLength,
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		TenantId:  &input.TenantID,
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		logger.Error("Failed to categorize content", logging.Err(err))
		return nil, fmt.Errorf("failed to categorize content: %w", err)
	}

	activity.RecordHeartbeat(ctx, "processing categorization results")

	// Parse category from response
	category := parseCategory(resp.Summary, input.Categories)

	output := &CategorizeContentOutput{
		PrimaryCategory: category,
		Confidence:      0.8, // Placeholder confidence
		Scores: []CategoryScore{
			{Category: category, Score: 0.8},
		},
		ModelUsed: resp.ModelUsed,
	}

	logger.Info("Content categorization completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("category", output.PrimaryCategory),
		logging.F("confidence", output.Confidence),
	)

	return output, nil
}

// parseCategory extracts a category from the LLM response.
func parseCategory(response string, allowedCategories []string) string {
	response = strings.ToLower(response)

	defaultCategories := []string{"work", "personal", "finance", "health", "travel", "technology", "news", "social"}
	categories := allowedCategories
	if len(categories) == 0 {
		categories = defaultCategories
	}

	for _, cat := range categories {
		if strings.Contains(response, strings.ToLower(cat)) {
			return cat
		}
	}
	return "uncategorized"
}

// truncateContent truncates content to a maximum length.
func truncateContent(content string, maxLen int) string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}

// SummarizeContentInput is the input for the SummarizeContent activity.
type SummarizeContentInput struct {
	TenantID        string `json:"tenant_id"`
	SourceID        int64  `json:"source_id"`
	JobID           string `json:"job_id"`
	Content         string `json:"content"`
	MaxLength       int32  `json:"max_length,omitempty"`
	Style           string `json:"style,omitempty"` // brief, detailed, bullet_points
	PipelineTraceID string `json:"pipeline_trace_id,omitempty"` // Pipeline trace ID for Langfuse grouping
	PipelineSpanID  string `json:"pipeline_span_id,omitempty"` // Pipeline span ID for parent-child hierarchy
}

// SummarizeContentOutput is the output from the SummarizeContent activity.
type SummarizeContentOutput struct {
	SummaryID    int64    `json:"summary_id"`
	Summary      string   `json:"summary"`
	KeyPoints    []string `json:"key_points"`
	ModelUsed    string   `json:"model_used"`
	InputTokens  int32    `json:"input_tokens,omitempty"`
	OutputTokens int32    `json:"output_tokens,omitempty"`
}

// SummarizeContentActivity generates a summary of the content.
// This activity uses an LLM to create concise summaries with key points.
func (a *ContentActivities) SummarizeContentActivity(ctx context.Context, input SummarizeContentInput) (*SummarizeContentOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "SummarizeContentActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting content summarization")

	logger.Info("Summarizing content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Content == "" {
		return nil, NewValidationError("content is required")
	}

	// Check if AI client is available
	if a.aiClient == nil {
		return nil, NewConfigurationError("AI client not configured")
	}

	startTime := time.Now()
	activity.RecordHeartbeat(ctx, "calling AI service for summarization")

	// Set defaults
	maxLength := input.MaxLength
	if maxLength == 0 {
		maxLength = 150
	}

	style := aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF
	switch strings.ToLower(input.Style) {
	case "detailed":
		style = aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED
	case "bullet_points":
		style = aiv1.SummaryStyle_SUMMARY_STYLE_BULLET_POINTS
	}

	req := &aiv1.SummaryRequest{
		Content:   input.Content,
		MaxLength: &maxLength,
		Style:     style,
		TenantId:  &input.TenantID,
	}
	if input.PipelineTraceID != "" {
		req.PipelineTraceId = &input.PipelineTraceID
	}
	if input.PipelineSpanID != "" {
		req.PipelineSpanId = &input.PipelineSpanID
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		logger.Error("Failed to generate summary", logging.Err(err))
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	activity.RecordHeartbeat(ctx, "storing summary")

	// Store summary if repository is available
	var summaryID int64
	if a.summaryRepo != nil {
		summaryID, err = a.summaryRepo.StoreSummary(
			ctx,
			input.TenantID,
			input.SourceID,
			resp.Summary,
			resp.KeyPoints,
			resp.ModelUsed,
		)
		if err != nil {
			logger.Warn("Failed to store summary", logging.Err(err))
		}
	}

	output := &SummarizeContentOutput{
		SummaryID:    summaryID,
		Summary:      resp.Summary,
		KeyPoints:    resp.KeyPoints,
		ModelUsed:    resp.ModelUsed,
		InputTokens:  resp.GetInputTokens(),
		OutputTokens: resp.GetOutputTokens(),
	}

	logger.Info("Content summarization completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("summary_id", summaryID),
		logging.F("summary_length", len(resp.Summary)),
		logging.F("key_points", len(resp.KeyPoints)),
	)

	return output, nil
}

// GenerateEmbeddingsInput is the input for the GenerateEmbeddings activity.
type GenerateEmbeddingsInput struct {
	TenantID    string   `json:"tenant_id"`
	SourceID    int64    `json:"source_id"`
	JobID       string   `json:"job_id"`
	Contents    []string `json:"contents"` // Multiple content items to embed
	Model       string   `json:"model,omitempty"`
}

// GenerateEmbeddingsOutput is the output from the GenerateEmbeddings activity.
type GenerateEmbeddingsOutput struct {
	EmbeddingIDs []int64 `json:"embedding_ids"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	Dimensions   int32   `json:"dimensions"`
	ModelUsed    string  `json:"model_used"`
}

// GenerateEmbeddingsActivity generates embeddings for one or more content items.
// This activity supports batch embedding generation for efficiency.
func (a *ContentActivities) GenerateEmbeddingsActivity(ctx context.Context, input GenerateEmbeddingsInput) (*GenerateEmbeddingsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "GenerateEmbeddingsActivity"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("job_id", input.JobID),
		logging.F("content_count", len(input.Contents)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting embedding generation")

	logger.Info("Generating embeddings")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if len(input.Contents) == 0 {
		return &GenerateEmbeddingsOutput{
			EmbeddingIDs: []int64{},
			SuccessCount: 0,
			FailureCount: 0,
		}, nil
	}

	// Check if AI client is available
	if a.aiClient == nil {
		return nil, NewConfigurationError("AI client not configured")
	}

	startTime := time.Now()
	embeddingIDs := make([]int64, 0, len(input.Contents))
	successCount := 0
	failureCount := 0
	var dimensions int32
	var modelUsed string

	for i, content := range input.Contents {
		// Check for cancellation between items
		if ctx.Err() != nil {
			failureCount += len(input.Contents) - i
			break
		}

		// Record heartbeat
		activity.RecordHeartbeat(ctx, fmt.Sprintf("generating embedding %d/%d", i+1, len(input.Contents)))

		// Generate embedding
		req := &aiv1.EmbeddingRequest{
			Text:     content,
			TenantId: &input.TenantID,
		}
		if input.Model != "" {
			req.Model = &input.Model
		}

		resp, err := a.aiClient.GenerateEmbedding(ctx, req)
		if err != nil {
			logger.Warn("Failed to generate embedding", logging.Err(err), logging.F("index", i))
			failureCount++
			continue
		}

		dimensions = resp.Dimensions
		modelUsed = resp.ModelUsed

		// Store embedding if repository is available
		var embeddingID int64
		if a.embeddingRepo != nil {
			embeddingID, err = a.embeddingRepo.StoreEmbedding(
				ctx,
				input.TenantID,
				input.SourceID,
				resp.Vector,
				resp.ModelUsed,
				resp.Dimensions,
			)
			if err != nil {
				logger.Warn("Failed to store embedding", logging.Err(err), logging.F("index", i))
				failureCount++
				continue
			}
		}

		embeddingIDs = append(embeddingIDs, embeddingID)
		successCount++
	}

	output := &GenerateEmbeddingsOutput{
		EmbeddingIDs: embeddingIDs,
		SuccessCount: successCount,
		FailureCount: failureCount,
		Dimensions:   dimensions,
		ModelUsed:    modelUsed,
	}

	logger.Info("Embedding generation completed",
		logging.F("duration", time.Since(startTime)),
		logging.F("success_count", successCount),
		logging.F("failure_count", failureCount),
		logging.F("dimensions", dimensions),
	)

	return output, nil
}

// Ensure ContentActivities implements required interfaces at compile time.
var _ interface {
	ChunkContentActivity(ctx context.Context, input ChunkContentInput) (*ChunkContentOutput, error)
	ExtractEntitiesActivity(ctx context.Context, input ExtractEntitiesActivityInput) (*ExtractEntitiesActivityOutput, error)
	CategorizeContentActivity(ctx context.Context, input CategorizeContentInput) (*CategorizeContentOutput, error)
	SummarizeContentActivity(ctx context.Context, input SummarizeContentInput) (*SummarizeContentOutput, error)
	GenerateEmbeddingsActivity(ctx context.Context, input GenerateEmbeddingsInput) (*GenerateEmbeddingsOutput, error)
} = (*ContentActivities)(nil)
