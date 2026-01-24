// Package server provides the gRPC server implementation for the AI Coordinator service.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/services/ai/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AIServer implements the AICoordinatorService gRPC server.
type AIServer struct {
	aiv1.UnimplementedAICoordinatorServiceServer

	config  *config.Config
	logger  logging.Logger
	backend backend.Backend
}

// NewAIServer creates a new AI server instance.
func NewAIServer(cfg *config.Config, logger logging.Logger, be backend.Backend) *AIServer {
	return &AIServer{
		config:  cfg,
		logger:  logger.With(logging.F("component", "ai_server")),
		backend: be,
	}
}

// GenerateEmbedding creates a vector embedding for the given text.
// Used for semantic search and similarity matching.
func (s *AIServer) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	text := strings.TrimSpace(req.GetText())
	model := req.GetModel()

	// Start tracing span
	ctx, span := tracing.StartEmbedding(ctx, "ai.embedding", tracing.EmbeddingOptions{
		Model:    model,
		System:   tracing.AISystemMLX,
		TenantID: req.GetTenantId(),
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("GenerateEmbedding called",
		logging.F("text_length", len(text)),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
	)

	if text == "" {
		err := status.Error(codes.InvalidArgument, "text cannot be empty")
		tracing.SetError(span, err)
		return nil, err
	}

	result, err := s.backend.GenerateEmbedding(ctx, text, model)
	if err != nil {
		s.logger.Error("GenerateEmbedding failed",
			logging.F("text_length", len(text)),
			logging.Err(err),
		)
		tracing.SetError(span, err)
		return nil, s.convertError(err)
	}

	resp := &aiv1.EmbeddingResponse{
		Vector:     result.Vector,
		Dimensions: int32(result.Dimensions),
		ModelUsed:  result.Model,
	}

	tokenCount := 0
	if result.TokenCount > 0 {
		tc := int32(result.TokenCount)
		resp.TokenCount = &tc
		tokenCount = result.TokenCount
	}

	// Record tracing result
	tracing.SetEmbeddingResult(span, tracing.EmbeddingResult{
		Dimensions:  result.Dimensions,
		InputTokens: tokenCount,
		LatencyMs:   time.Since(startTime).Milliseconds(),
	})

	s.logger.Debug("GenerateEmbedding completed",
		logging.F("dimensions", result.Dimensions),
		logging.F("model_used", result.Model),
	)

	return resp, nil
}

// GenerateSummary produces a concise summary of the given content.
// Supports different summary styles and length constraints.
func (s *AIServer) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	model := req.GetModel()

	// Start tracing span
	ctx, span := tracing.StartLLMCall(ctx, "ai.summarize", tracing.LLMCallOptions{
		Model:    model,
		System:   tracing.AISystemMLX,
		TenantID: req.GetTenantId(),
		TaskType: "summarize",
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("GenerateSummary called",
		logging.F("content_length", len(content)),
		logging.F("style", req.GetStyle().String()),
		logging.F("max_length", req.GetMaxLength()),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
	)

	if content == "" {
		err := status.Error(codes.InvalidArgument, "content cannot be empty")
		tracing.SetError(span, err)
		return nil, err
	}

	// Build the system prompt based on style
	systemPrompt := s.buildSummarySystemPrompt(req.GetStyle(), req.GetMaxLength())

	messages := []backend.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: content},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.3, // Slightly creative for summaries
	}

	if req.GetMaxLength() > 0 {
		opts.MaxTokens = int(req.GetMaxLength()) * 2 // Allow room for key points
	}

	result, err := s.backend.ChatCompletion(ctx, messages, opts)
	if err != nil {
		s.logger.Error("GenerateSummary failed",
			logging.F("content_length", len(content)),
			logging.Err(err),
		)
		tracing.SetError(span, err)
		return nil, s.convertError(err)
	}

	// Parse the response to extract summary and key points
	summary, keyPoints := s.parseSummaryResponse(result.Content)

	resp := &aiv1.SummaryResponse{
		Summary:   summary,
		KeyPoints: keyPoints,
		ModelUsed: result.Model,
	}

	inputTokens := 0
	outputTokens := 0
	if result.InputTokens > 0 {
		it := int32(result.InputTokens)
		resp.InputTokens = &it
		inputTokens = result.InputTokens
	}
	if result.OutputTokens > 0 {
		ot := int32(result.OutputTokens)
		resp.OutputTokens = &ot
		outputTokens = result.OutputTokens
	}

	// Record tracing result
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
	})

	s.logger.Debug("GenerateSummary completed",
		logging.F("summary_length", len(summary)),
		logging.F("key_points_count", len(keyPoints)),
		logging.F("model_used", result.Model),
	)

	return resp, nil
}

// ExtractAssertions identifies facts and claims from content.
// Returns structured subject-predicate-object triples with confidence scores.
func (s *AIServer) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	model := req.GetModel()

	// Start tracing span
	ctx, span := tracing.StartLLMCall(ctx, "ai.extract_assertions", tracing.LLMCallOptions{
		Model:    model,
		System:   tracing.AISystemMLX,
		TenantID: req.GetTenantId(),
		TaskType: "extraction",
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("ExtractAssertions called",
		logging.F("content_length", len(content)),
		logging.F("min_confidence", req.GetMinConfidence()),
		logging.F("max_assertions", req.GetMaxAssertions()),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
	)

	if content == "" {
		err := status.Error(codes.InvalidArgument, "content cannot be empty")
		tracing.SetError(span, err)
		return nil, err
	}

	systemPrompt := s.buildAssertionSystemPrompt()
	userPrompt := fmt.Sprintf("Extract assertions from the following content:\n\n%s", content)

	messages := []backend.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.1, // Low temperature for structured extraction
		MaxTokens:   4096,
		JSONMode:    true,
	}

	result, err := s.backend.ChatCompletion(ctx, messages, opts)
	if err != nil {
		s.logger.Error("ExtractAssertions failed",
			logging.F("content_length", len(content)),
			logging.Err(err),
		)
		tracing.SetError(span, err)
		return nil, s.convertError(err)
	}

	// Parse the JSON response
	assertions, err := s.parseAssertionsResponse(result.Content)
	if err != nil {
		s.logger.Warn("Failed to parse assertions JSON, attempting fallback",
			logging.Err(err),
		)
		assertions = s.parseAssertionsFallback(result.Content)
	}

	// Apply filters
	minConfidence := req.GetMinConfidence()
	if minConfidence == 0 {
		minConfidence = 0.5
	}

	maxAssertions := int(req.GetMaxAssertions())
	if maxAssertions == 0 {
		maxAssertions = 20
	}

	// Filter and sort assertions
	filtered := make([]*aiv1.Assertion, 0)
	totalFound := len(assertions)
	filteredCount := 0

	for _, a := range assertions {
		if a.Confidence >= minConfidence {
			filtered = append(filtered, a)
		} else {
			filteredCount++
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Confidence > filtered[j].Confidence
	})

	// Limit to max assertions
	if len(filtered) > maxAssertions {
		filteredCount += len(filtered) - maxAssertions
		filtered = filtered[:maxAssertions]
	}

	resp := &aiv1.AssertionResponse{
		Assertions:    filtered,
		ModelUsed:     result.Model,
		TotalFound:    int32(totalFound),
		FilteredCount: int32(filteredCount),
	}

	// Record tracing result
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
	})

	s.logger.Debug("ExtractAssertions completed",
		logging.F("assertions_returned", len(filtered)),
		logging.F("total_found", totalFound),
		logging.F("filtered_count", filteredCount),
		logging.F("model_used", result.Model),
	)

	return resp, nil
}

// ClassifyContent categorizes content into predefined or dynamic categories.
// Returns classification labels with confidence scores.
func (s *AIServer) ClassifyContent(ctx context.Context, req *aiv1.ClassifyContentRequest) (*aiv1.ClassifyContentResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	model := req.GetModel()

	// Start tracing span
	ctx, span := tracing.StartLLMCall(ctx, "ai.classify", tracing.LLMCallOptions{
		Model:    model,
		System:   tracing.AISystemMLX,
		TenantID: req.GetTenantId(),
		TaskType: "classification",
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("ClassifyContent called",
		logging.F("content_length", len(content)),
		logging.F("categories", req.GetCategories()),
		logging.F("multi_label", req.GetMultiLabel()),
		logging.F("min_confidence", req.GetMinConfidence()),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
	)

	if content == "" {
		err := status.Error(codes.InvalidArgument, "content cannot be empty")
		tracing.SetError(span, err)
		return nil, err
	}

	categories := req.GetCategories()
	// Default to multi-label if not specified in the request
	// GetMultiLabel returns false if not set, so we check if the field was explicitly set
	multiLabel := true
	if req.MultiLabel != nil {
		multiLabel = req.GetMultiLabel()
	}

	systemPrompt := s.buildClassificationSystemPrompt(categories, multiLabel)
	userPrompt := fmt.Sprintf("Classify the following content:\n\n%s", content)

	messages := []backend.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.1,
		MaxTokens:   1024,
		JSONMode:    true,
	}

	result, err := s.backend.ChatCompletion(ctx, messages, opts)
	if err != nil {
		s.logger.Error("ClassifyContent failed",
			logging.F("content_length", len(content)),
			logging.Err(err),
		)
		tracing.SetError(span, err)
		return nil, s.convertError(err)
	}

	// Parse the JSON response
	classifications, err := s.parseClassificationsResponse(result.Content)
	if err != nil {
		s.logger.Warn("Failed to parse classifications JSON",
			logging.Err(err),
		)
		parseErr := status.Error(codes.Internal, "failed to parse classification response")
		tracing.SetError(span, parseErr)
		return nil, parseErr
	}

	// Apply minimum confidence filter
	minConfidence := req.GetMinConfidence()
	if minConfidence == 0 {
		minConfidence = 0.3
	}

	filtered := make([]*aiv1.Classification, 0)
	for _, c := range classifications {
		if c.Confidence >= minConfidence {
			filtered = append(filtered, c)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Confidence > filtered[j].Confidence
	})

	// If not multi-label, keep only the top one
	if !multiLabel && len(filtered) > 1 {
		filtered = filtered[:1]
	}

	var primary *aiv1.Classification
	if len(filtered) > 0 {
		primary = filtered[0]
	}

	resp := &aiv1.ClassifyContentResponse{
		Classifications: filtered,
		Primary:         primary,
		ModelUsed:       result.Model,
	}

	// Record tracing result
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
	})

	s.logger.Debug("ClassifyContent completed",
		logging.F("classifications_count", len(filtered)),
		logging.F("model_used", result.Model),
	)

	return resp, nil
}

// GetModelStatus checks the availability and health of AI models.
// Returns information about loaded models and their capabilities.
func (s *AIServer) GetModelStatus(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	s.logger.Debug("GetModelStatus called",
		logging.F("model_name", req.GetModelName()),
		logging.F("model_type", req.GetModelType().String()),
		logging.F("include_metrics", req.GetIncludeMetrics()),
	)

	models := make([]*aiv1.ModelInfo, 0)
	healthy := true
	var messages []string

	// Check embeddings backend
	embeddingErr := s.backend.CheckEmbeddingsHealth(ctx)
	embeddingStatus := aiv1.ModelStatus_MODEL_STATUS_READY
	var embeddingErrMsg *string
	if embeddingErr != nil {
		embeddingStatus = aiv1.ModelStatus_MODEL_STATUS_ERROR
		errStr := embeddingErr.Error()
		embeddingErrMsg = &errStr
		healthy = false
		messages = append(messages, fmt.Sprintf("embeddings: %v", embeddingErr))
	}

	// Add embedding model info
	if mlxBackend, ok := s.backend.(*backend.MLXBackend); ok {
		dims := int32(s.config.EmbeddingDimensions)
		embeddingModel := &aiv1.ModelInfo{
			Name:                mlxBackend.DefaultEmbeddingModel(),
			Type:                aiv1.ModelType_MODEL_TYPE_EMBEDDING,
			Status:              embeddingStatus,
			Capabilities:        []string{"embedding"},
			EmbeddingDimensions: &dims,
			IsLocal:             true,
			Provider:            "mlx",
			ErrorMessage:        embeddingErrMsg,
		}
		models = append(models, embeddingModel)
	}

	// Check LLM backend
	llmErr := s.backend.CheckLLMHealth(ctx)
	llmStatus := aiv1.ModelStatus_MODEL_STATUS_READY
	var llmErrMsg *string
	if llmErr != nil {
		llmStatus = aiv1.ModelStatus_MODEL_STATUS_ERROR
		errStr := llmErr.Error()
		llmErrMsg = &errStr
		healthy = false
		messages = append(messages, fmt.Sprintf("llm: %v", llmErr))
	}

	// Add LLM model info
	if mlxBackend, ok := s.backend.(*backend.MLXBackend); ok {
		llmModel := &aiv1.ModelInfo{
			Name:         mlxBackend.DefaultLLMModel(),
			Type:         aiv1.ModelType_MODEL_TYPE_LLM,
			Status:       llmStatus,
			Capabilities: []string{"chat", "summarization", "extraction", "classification"},
			IsLocal:      true,
			Provider:     "mlx",
			ErrorMessage: llmErrMsg,
		}
		models = append(models, llmModel)
	}

	// Filter by request parameters if specified
	if req.GetModelName() != "" || req.GetModelType() != aiv1.ModelType_MODEL_TYPE_UNSPECIFIED {
		filtered := make([]*aiv1.ModelInfo, 0)
		for _, m := range models {
			if req.GetModelName() != "" && m.Name != req.GetModelName() {
				continue
			}
			if req.GetModelType() != aiv1.ModelType_MODEL_TYPE_UNSPECIFIED && m.Type != req.GetModelType() {
				continue
			}
			filtered = append(filtered, m)
		}
		models = filtered
	}

	statusMessage := "All models healthy"
	if !healthy {
		statusMessage = strings.Join(messages, "; ")
	}

	resp := &aiv1.GetModelStatusResponse{
		Models:                models,
		DefaultEmbeddingModel: s.config.DefaultEmbeddingModel,
		DefaultLlmModel:       s.config.DefaultLLMModel,
		Healthy:               healthy,
		Message:               statusMessage,
	}

	s.logger.Debug("GetModelStatus completed",
		logging.F("models_count", len(models)),
		logging.F("healthy", healthy),
	)

	return resp, nil
}

// Helper methods

func (s *AIServer) buildSummarySystemPrompt(style aiv1.SummaryStyle, maxLength int32) string {
	var styleInstruction string
	switch style {
	case aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF:
		styleInstruction = "Create a brief, executive-style summary focusing only on the most critical points. Be concise."
	case aiv1.SummaryStyle_SUMMARY_STYLE_DETAILED:
		styleInstruction = "Create a detailed summary that preserves important context and nuance. Include relevant details."
	case aiv1.SummaryStyle_SUMMARY_STYLE_BULLET_POINTS:
		styleInstruction = "Create a summary using bullet points. Each bullet should capture a distinct main topic or finding."
	case aiv1.SummaryStyle_SUMMARY_STYLE_TECHNICAL:
		styleInstruction = "Create a technical summary focusing on facts, data, and specific details. Avoid subjective interpretations."
	default:
		styleInstruction = "Create a balanced, informative summary that captures the main points."
	}

	lengthInstruction := ""
	if maxLength > 0 {
		lengthInstruction = fmt.Sprintf(" Keep the summary under %d words.", maxLength)
	}

	return fmt.Sprintf(`You are a summarization assistant. %s%s

After the summary, provide 3-5 key points as a JSON array.

Format your response as:
SUMMARY:
[Your summary here]

KEY_POINTS:
["point 1", "point 2", "point 3"]`, styleInstruction, lengthInstruction)
}

func (s *AIServer) parseSummaryResponse(content string) (string, []string) {
	// Try to parse structured response
	content = strings.TrimSpace(content)

	// Look for SUMMARY: and KEY_POINTS: markers
	summaryStart := strings.Index(strings.ToUpper(content), "SUMMARY:")
	keyPointsStart := strings.Index(strings.ToUpper(content), "KEY_POINTS:")

	var summary string
	var keyPoints []string

	if summaryStart >= 0 && keyPointsStart > summaryStart {
		// Extract summary
		summaryText := content[summaryStart+8 : keyPointsStart]
		summary = strings.TrimSpace(summaryText)

		// Extract key points JSON
		keyPointsText := content[keyPointsStart+11:]
		keyPointsText = strings.TrimSpace(keyPointsText)

		// Find JSON array
		start := strings.Index(keyPointsText, "[")
		end := strings.LastIndex(keyPointsText, "]")
		if start >= 0 && end > start {
			jsonStr := keyPointsText[start : end+1]
			if err := json.Unmarshal([]byte(jsonStr), &keyPoints); err != nil {
				// Fallback: split by newlines
				keyPoints = s.extractKeyPointsFallback(keyPointsText)
			}
		}
	} else {
		// No structured format, use entire content as summary
		summary = content
	}

	return summary, keyPoints
}

func (s *AIServer) extractKeyPointsFallback(text string) []string {
	lines := strings.Split(text, "\n")
	points := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimPrefix(line, "•")
		line = strings.TrimSpace(line)
		if line != "" && line != "[" && line != "]" {
			points = append(points, line)
		}
	}
	return points
}

func (s *AIServer) buildAssertionSystemPrompt() string {
	return `You are an assertion extraction assistant. Extract factual claims and statements from the content as subject-predicate-object triples.

For each assertion, provide:
- subject: The entity being discussed
- predicate: The relationship or action
- object: What the subject relates to
- confidence: Your confidence score (0.0-1.0)
- source_text: The exact text supporting this assertion
- category: One of: temporal, organizational, factual, relational, location, quantity

Respond with a JSON object:
{
  "assertions": [
    {
      "subject": "...",
      "predicate": "...",
      "object": "...",
      "confidence": 0.85,
      "source_text": "...",
      "category": "..."
    }
  ]
}

Focus on explicit, verifiable facts. Avoid speculation or inference.`
}

type assertionsJSON struct {
	Assertions []struct {
		Subject    string  `json:"subject"`
		Predicate  string  `json:"predicate"`
		Object     string  `json:"object"`
		Confidence float32 `json:"confidence"`
		SourceText string  `json:"source_text"`
		Category   string  `json:"category"`
	} `json:"assertions"`
}

func (s *AIServer) parseAssertionsResponse(content string) ([]*aiv1.Assertion, error) {
	// Clean up the response
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed assertionsJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	assertions := make([]*aiv1.Assertion, len(parsed.Assertions))
	for i, a := range parsed.Assertions {
		var sourceText *string
		if a.SourceText != "" {
			sourceText = &a.SourceText
		}
		var category *string
		if a.Category != "" {
			category = &a.Category
		}

		assertions[i] = &aiv1.Assertion{
			Subject:    a.Subject,
			Predicate:  a.Predicate,
			Object:     a.Object,
			Confidence: a.Confidence,
			SourceText: sourceText,
			Category:   category,
		}
	}

	return assertions, nil
}

func (s *AIServer) parseAssertionsFallback(content string) []*aiv1.Assertion {
	// Return empty list if we can't parse
	return []*aiv1.Assertion{}
}

func (s *AIServer) buildClassificationSystemPrompt(categories []string, multiLabel bool) string {
	categoryInstruction := ""
	if len(categories) > 0 {
		categoryInstruction = fmt.Sprintf("Classify into these categories: %s\n", strings.Join(categories, ", "))
	} else {
		categoryInstruction = "Use appropriate general categories like: work, personal, finance, health, technology, news, entertainment, education.\n"
	}

	multiLabelInstruction := ""
	if multiLabel {
		multiLabelInstruction = "Multiple categories can apply to the same content."
	} else {
		multiLabelInstruction = "Choose only the single most appropriate category."
	}

	return fmt.Sprintf(`You are a content classification assistant. %s%s

Respond with a JSON object:
{
  "classifications": [
    {
      "label": "category_name",
      "confidence": 0.85,
      "explanation": "Brief reason for this classification"
    }
  ]
}

Order classifications by confidence (highest first).`, categoryInstruction, multiLabelInstruction)
}

type classificationsJSON struct {
	Classifications []struct {
		Label       string  `json:"label"`
		Confidence  float32 `json:"confidence"`
		Explanation string  `json:"explanation"`
	} `json:"classifications"`
}

func (s *AIServer) parseClassificationsResponse(content string) ([]*aiv1.Classification, error) {
	// Clean up the response
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var parsed classificationsJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	classifications := make([]*aiv1.Classification, len(parsed.Classifications))
	for i, c := range parsed.Classifications {
		var explanation *string
		if c.Explanation != "" {
			explanation = &c.Explanation
		}

		classifications[i] = &aiv1.Classification{
			Label:       c.Label,
			Confidence:  c.Confidence,
			Explanation: explanation,
		}
	}

	return classifications, nil
}

func (s *AIServer) convertError(err error) error {
	switch {
	case strings.Contains(err.Error(), "context canceled"):
		return status.Error(codes.Canceled, err.Error())
	case strings.Contains(err.Error(), "service unavailable"):
		return status.Error(codes.Unavailable, err.Error())
	case strings.Contains(err.Error(), "text cannot be empty"),
		strings.Contains(err.Error(), "messages cannot be empty"):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
