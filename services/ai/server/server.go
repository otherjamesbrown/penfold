// Package server provides the gRPC server implementation for the AI Coordinator service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
	"github.com/otherjamesbrown/penfold/pkg/langfuse"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"github.com/otherjamesbrown/penfold/services/ai/backend"
	"github.com/otherjamesbrown/penfold/services/ai/config"
	"github.com/otherjamesbrown/penfold/services/ai/registry"
	"github.com/otherjamesbrown/penfold/services/ai/router"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// PromptStore is a minimal interface for retrieving active prompt templates by stage.
// Version 0 means "return the active prompt"; version > 0 returns a specific version.
type PromptStore interface {
	GetPromptByStage(ctx context.Context, stage string, version int) (*pipeline.PromptTemplate, error)
}

// StageParamsProvider retrieves per-stage LLM parameters from pipeline_definitions.
type StageParamsProvider interface {
	GetStageLLMParams(ctx context.Context, stage string) (*pipeline.StageParams, error)
}

// AIServer implements the AICoordinatorService gRPC server.
type AIServer struct {
	aiv1.UnimplementedAICoordinatorServiceServer

	config         *config.Config
	logger         logging.Logger
	backend        backend.Backend          // Direct backend for backwards compatibility
	router         *router.ModelRouter      // Multi-model routing (optional)
	registry       *registry.DBRegistry     // Model registry (optional)
	langfuse       *langfuse.Ingestion      // Langfuse generation reporting (optional, nil = disabled)
	promptStore    PromptStore              // DB-backed prompt store (optional, nil = use hardcoded fallbacks)
	configResolver *config.DBConfigResolver // DB-backed model config resolver (optional, nil = env vars only)
	stageParams    StageParamsProvider     // Per-stage LLM params from pipeline_definitions (optional, nil = hardcoded defaults)
}

// NewAIServer creates a new AI server instance with a single backend.
// For backwards compatibility with existing code.
func NewAIServer(cfg *config.Config, logger logging.Logger, be backend.Backend) *AIServer {
	return &AIServer{
		config:  cfg,
		logger:  logger.With(logging.F("component", "ai_server")),
		backend: be,
	}
}

// NewAIServerWithRouter creates a new AI server instance with multi-model routing support.
// The router handles model selection, circuit breaking, and fallback logic.
// The registry provides model configuration storage and retrieval.
func NewAIServerWithRouter(
	cfg *config.Config,
	logger logging.Logger,
	r *router.ModelRouter,
	reg *registry.DBRegistry,
) *AIServer {
	return &AIServer{
		config:   cfg,
		logger:   logger.With(logging.F("component", "ai_server")),
		router:   r,
		registry: reg,
	}
}

// NewAIServerWithAll creates a new AI server instance with both direct backend and router.
// This allows the server to use the router when available but fall back to the direct backend.
func NewAIServerWithAll(
	cfg *config.Config,
	logger logging.Logger,
	be backend.Backend,
	r *router.ModelRouter,
	reg *registry.DBRegistry,
) *AIServer {
	return &AIServer{
		config:   cfg,
		logger:   logger.With(logging.F("component", "ai_server")),
		backend:  be,
		router:   r,
		registry: reg,
	}
}

// NewAIServerWithLangfuse creates a new AI server instance with Langfuse generation reporting.
// Pass a nil ingestion to disable generation reporting (all handlers remain functional).
func NewAIServerWithLangfuse(
	cfg *config.Config,
	logger logging.Logger,
	be backend.Backend,
	lf *langfuse.Ingestion,
) *AIServer {
	s := NewAIServer(cfg, logger, be)
	s.langfuse = lf
	return s
}

// WithPromptStore sets the DB-backed prompt store on an existing AIServer.
// Call this after constructing the server to enable DB-driven prompts with
// hardcoded fallbacks when DB is unavailable.
func (s *AIServer) WithPromptStore(ps PromptStore) *AIServer {
	s.promptStore = ps
	return s
}

// WithDBConfigResolver sets the DB-backed model config resolver on an existing AIServer.
// Call this after constructing the server to enable DB-driven model assignments per pipeline stage.
// When nil, all handlers fall back to env var config (Config.ModelForStage).
func (s *AIServer) WithDBConfigResolver(r *config.DBConfigResolver) *AIServer {
	s.configResolver = r
	return s
}

// WithStageParams sets the per-stage LLM parameters provider on an existing AIServer.
// When nil, all handlers use hardcoded fallback values for temperature, max_tokens, and max_retries.
func (s *AIServer) WithStageParams(sp StageParamsProvider) *AIServer {
	s.stageParams = sp
	return s
}

// getStageParams retrieves per-stage LLM parameters from pipeline_definitions.
// Returns nil when the provider is not configured or the stage is not found (caller uses defaults).
func (s *AIServer) getStageParams(ctx context.Context, stage string) *pipeline.StageParams {
	if s.stageParams == nil {
		return nil
	}
	params, err := s.stageParams.GetStageLLMParams(ctx, stage)
	if err != nil {
		s.logger.Warn("stage params lookup failed, using hardcoded defaults",
			logging.F("stage", stage),
			logging.Err(err),
		)
		return nil
	}
	return params
}

// resolveModel returns the model for a pipeline stage, preferring DB config when available.
// Priority: explicit request model (caller's responsibility) → DB config → env var stage config →
// global env var default → hardcoded fallback. The DB config resolver handles levels 2–5;
// this method delegates to it when present, otherwise falls back to Config.ModelForStage.
func (s *AIServer) resolveModel(ctx context.Context, stage string) string {
	if s.configResolver != nil {
		model, err := s.configResolver.ModelForStage(ctx, stage)
		if err != nil {
			s.logger.Warn("DB config resolution failed, falling back to env",
				logging.F("stage", stage), logging.Err(err))
			return s.config.ModelForStage(stage)
		}
		return model
	}
	return s.config.ModelForStage(stage)
}

// getPrompt retrieves the active prompt for the given stage from the DB prompt store.
// If the store is not configured, the DB is unreachable, or no active prompt exists,
// it falls back to the provided hardcoded default without failing the request.
// Returns the prompt content and the prompt version (0 when using the hardcoded fallback).
func (s *AIServer) getPrompt(ctx context.Context, stage, hardcoded string, version int32) (string, int32) {
	if s.promptStore == nil {
		return hardcoded, 0
	}
	pt, err := s.promptStore.GetPromptByStage(ctx, stage, int(version))
	if err != nil {
		s.logger.Warn("prompt store lookup failed, using hardcoded fallback",
			logging.F("stage", stage),
			logging.Err(err),
		)
		return hardcoded, 0
	}
	if pt == nil {
		return hardcoded, 0
	}
	return pt.Content, int32(pt.Version)
}

// extractLangfuseMetadata reads x-langfuse-trace-id and x-langfuse-phase-id from
// incoming gRPC metadata. Returns empty strings if metadata is absent or the keys
// are not present.
func extractLangfuseMetadata(ctx context.Context) (traceID, phaseID string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ""
	}
	if vals := md.Get("x-langfuse-trace-id"); len(vals) > 0 {
		traceID = vals[0]
	}
	if vals := md.Get("x-langfuse-phase-id"); len(vals) > 0 {
		phaseID = vals[0]
	}
	return traceID, phaseID
}

// GenerateEmbedding creates a vector embedding for the given text.
// Used for semantic search and similarity matching.
func (s *AIServer) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	text := strings.TrimSpace(req.GetText())
	model := req.GetModel()

	// Resolve model: explicit request → DB config → stage env var → global default → hardcoded fallback
	if model == "" {
		model = s.resolveModel(ctx, "embed")
	}

	// Start tracing span for the embedding generation.
	// The worker creates the stage.embedding span; this handler only creates ai.embedding.
	ctx, span := tracing.StartEmbedding(ctx, "ai.embedding", tracing.EmbeddingOptions{
		Model:     model,
		System:    tracing.AISystemMLX,
		TenantID:  req.GetTenantId(),
		ContentID: req.GetContentId(),
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
		Model:       result.Model,
		LatencyMs:   time.Since(startTime).Milliseconds(),
		Input:       text,
	})

	// Report generation to Langfuse if configured and trace metadata is present.
	if s.langfuse != nil {
		lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
		if lfTraceID != "" {
			s.langfuse.CreateGeneration(langfuse.GenerationEvent{
				ID:           uuid.New().String(),
				TraceID:      lfTraceID,
				ParentID:     lfPhaseID,
				Name:         "ai.embedding",
				Model:        result.Model,
				Input:        text,
				Output:       fmt.Sprintf("vector[%d]", result.Dimensions),
				PromptTokens: tokenCount,
				StartTime:    startTime,
				EndTime:      time.Now(),
			})
			if err := s.langfuse.Flush(ctx); err != nil {
				s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
			}
		}
	}

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

	// Start tracing span for the summary generation.
	// The worker creates the stage.summarize span; this handler only creates ai.summarize.
	ctx, span := tracing.StartLLMCall(ctx, "ai.summarize", tracing.LLMCallOptions{
		Model:     model,
		System:    tracing.AISystemMLX,
		TenantID:  req.GetTenantId(),
		TaskType:  "summarize",
		ContentID: req.GetContentId(),
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

	// When json_mode is true, the caller provides a self-contained prompt
	// (e.g. consolidation template from DB) that expects JSON output.
	// The default summary system prompt demands SUMMARY:/KEY_POINTS: format
	// which conflicts with JSON mode — skip it.
	var messages []backend.Message
	if req.GetJsonMode() {
		messages = []backend.Message{
			{Role: "user", Content: content},
		}
	} else {
		systemPrompt := s.buildSummarySystemPrompt(ctx, req.GetStyle(), req.GetMaxLength(), req.GetPromptVersion())
		messages = []backend.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: content},
		}
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.3, // Slightly creative for summaries
		JSONMode:    req.GetJsonMode(), // Pass through JSON mode flag
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

	// Report generation to Langfuse if configured and trace metadata is present.
	if s.langfuse != nil {
		lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
		if lfTraceID != "" {
			s.langfuse.CreateGeneration(langfuse.GenerationEvent{
				ID:               uuid.New().String(),
				TraceID:          lfTraceID,
				ParentID:         lfPhaseID,
				Name:             "ai.summarize",
				Model:            result.Model,
				Input:            messages,
				Output:           result.Content,
				PromptTokens:     result.InputTokens,
				CompletionTokens: result.OutputTokens,
				StartTime:        startTime,
				EndTime:          time.Now(),
			})
			if err := s.langfuse.Flush(ctx); err != nil {
				s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
			}
		}
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
	if result.FinishReason != "" {
		resp.FinishReason = &result.FinishReason
	}

	// Record tracing result with prompt/completion for Langfuse visibility
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Prompt:       tracePromptFromMessages(messages),
		Completion:   result.Content,
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

	// Resolve model: explicit request → DB config → stage env var → global default → hardcoded fallback
	if model == "" {
		model = s.resolveModel(ctx, "extract_assertions")
	}

	// Start tracing span for the assertion extraction.
	// The worker creates the stage.extract_assertions span; this handler only creates ai.extract_assertions.
	ctx, span := tracing.StartLLMCall(ctx, "ai.extract_assertions", tracing.LLMCallOptions{
		Model:     model,
		System:    tracing.AISystemMLX,
		TenantID:  req.GetTenantId(),
		TaskType:  "extraction",
		ContentID: req.GetContentId(),
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

	systemPrompt, promptVersion := s.buildAssertionSystemPrompt(ctx)
	userPrompt := fmt.Sprintf("Extract assertions from the following content:\n\n%s", content)

	messages := []backend.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.1, // Low temperature for structured extraction
		MaxTokens:   8192, // gemini-2.5-flash thinking tokens consume output budget
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

	// Report generation to Langfuse if configured and trace metadata is present.
	if s.langfuse != nil {
		lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
		if lfTraceID != "" {
			s.langfuse.CreateGeneration(langfuse.GenerationEvent{
				ID:               uuid.New().String(),
				TraceID:          lfTraceID,
				ParentID:         lfPhaseID,
				Name:             "ai.extract_assertions",
				Model:            result.Model,
				Input:            messages,
				Output:           result.Content,
				PromptTokens:     result.InputTokens,
				CompletionTokens: result.OutputTokens,
				StartTime:        startTime,
				EndTime:          time.Now(),
			})
			if err := s.langfuse.Flush(ctx); err != nil {
				s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
			}
		}
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

	inputTokens := int32(result.InputTokens)
	outputTokens := int32(result.OutputTokens)
	resp := &aiv1.AssertionResponse{
		Assertions:    filtered,
		ModelUsed:     result.Model,
		TotalFound:    int32(totalFound),
		FilteredCount: int32(filteredCount),
		PromptVersion: promptVersion,
		InputTokens:   &inputTokens,
		OutputTokens:  &outputTokens,
	}

	// Record tracing result with prompt/completion for Langfuse visibility
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Prompt:       systemPrompt + "\n\n" + userPrompt,
		Completion:   result.Content,
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
		Model:     model,
		System:    tracing.AISystemMLX,
		TenantID:  req.GetTenantId(),
		TaskType:  "classification",
		ContentID: req.GetContentId(),
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

	systemPrompt := s.buildClassificationSystemPrompt(ctx, categories, multiLabel)
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

	// Record tracing result with prompt/completion for Langfuse visibility
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Prompt:       systemPrompt + "\n\n" + userPrompt,
		Completion:   result.Content,
	})

	s.logger.Debug("ClassifyContent completed",
		logging.F("classifications_count", len(filtered)),
		logging.F("model_used", result.Model),
	)

	return resp, nil
}

// TriageContent classifies content into categories and rates importance.
// Returns classification with category, importance, and reasoning.
func (s *AIServer) TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	content := strings.TrimSpace(req.GetContent())
	model := req.GetModel()

	// Resolve model: explicit request → DB config → stage env var → global default → hardcoded fallback
	if model == "" {
		model = s.resolveModel(ctx, "triage")
	}

	// Start tracing span for the triage generation.
	// The worker creates the stage.triage span; this handler only creates ai.triage.
	ctx, span := tracing.StartLLMCall(ctx, "ai.triage", tracing.LLMCallOptions{
		Model:     model,
		System:    tracing.AISystemMLX,
		TenantID:  req.GetTenantId(),
		TaskType:  "triage",
		ContentID: req.GetContentId(),
	})
	defer span.End()
	startTime := time.Now()

	s.logger.Debug("TriageContent called",
		logging.F("content_length", len(content)),
		logging.F("subject", req.GetSubject()),
		logging.F("sender", req.GetSender()),
		logging.F("model", model),
		logging.F("tenant_id", req.GetTenantId()),
		logging.F("source_id", req.GetSourceId()),
	)

	if content == "" {
		err := status.Error(codes.InvalidArgument, "content cannot be empty")
		tracing.SetError(span, err)
		return nil, err
	}

	// Build the triage prompt
	systemPrompt, userPrompt, triagePromptVersion := s.buildTriagePrompt(ctx, req.GetSubject(), req.GetSender(), content, req.GetPromptVersion())

	messages := []backend.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	opts := backend.CompletionOptions{
		Model:       model,
		Temperature: 0.1, // Low temperature for consistent classification
		MaxTokens:   256, // Small response: just category, importance, reason
		JSONMode:    true,
	}

	// Retry loop: up to 2 retries on malformed output
	const maxTriageRetries = 2
	var triageResult *triageResult
	var result *backend.CompletionResult
	var lastErr error
	retryCount := 0

	for attempt := 0; attempt <= maxTriageRetries; attempt++ {
		result, lastErr = s.backend.ChatCompletion(ctx, messages, opts)
		if lastErr != nil {
			s.logger.Error("TriageContent ChatCompletion failed",
				logging.F("attempt", attempt),
				logging.Err(lastErr),
			)
			tracing.SetError(span, lastErr)
			return nil, s.convertError(lastErr)
		}

		// Try to parse the response
		triageResult, lastErr = parseTriageResponse(result.Content)
		if lastErr == nil {
			// Successfully parsed and validated
			break
		}

		s.logger.Warn("TriageContent response parsing failed, retrying",
			logging.F("attempt", attempt),
			logging.F("max_retries", maxTriageRetries),
			logging.Err(lastErr),
		)

		retryCount++

		if attempt >= maxTriageRetries {
			// Exhausted retries
			parseErr := status.Error(codes.Internal, fmt.Sprintf("failed to parse triage response after %d retries: %v", maxTriageRetries, lastErr))
			tracing.SetError(span, parseErr)
			return nil, parseErr
		}
	}

	// Report generation to Langfuse if configured and trace metadata is present.
	if s.langfuse != nil {
		lfTraceID, lfPhaseID := extractLangfuseMetadata(ctx)
		if lfTraceID != "" {
			s.langfuse.CreateGeneration(langfuse.GenerationEvent{
				ID:               uuid.New().String(),
				TraceID:          lfTraceID,
				ParentID:         lfPhaseID,
				Name:             "ai.triage",
				Model:            result.Model,
				Input:            messages,
				Output:           result.Content,
				PromptTokens:     result.InputTokens,
				CompletionTokens: result.OutputTokens,
				StartTime:        startTime,
				EndTime:          time.Now(),
			})
			if err := s.langfuse.Flush(ctx); err != nil {
				s.logger.Warn("Langfuse generation flush failed", logging.Err(err))
			}
		}
	}

	// Build response
	resp := &aiv1.TriageContentResponse{
		Category:      triageResult.Category,
		Importance:    triageResult.Importance,
		Reason:        triageResult.Reason,
		ModelUsed:     result.Model,
		Retries:       int32(retryCount),
		PromptVersion: triagePromptVersion,
	}

	// Add content_contribution fields if present
	if triageResult.ContentContribution != "" {
		resp.ContentContribution = &triageResult.ContentContribution
	}
	if triageResult.ContributionReason != "" {
		resp.ContributionReason = &triageResult.ContributionReason
	}

	if result.InputTokens > 0 {
		it := int32(result.InputTokens)
		resp.InputTokens = &it
	}
	if result.OutputTokens > 0 {
		ot := int32(result.OutputTokens)
		resp.OutputTokens = &ot
	}

	// Record tracing result with prompt/completion for Langfuse visibility
	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Model:        result.Model,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Prompt:       systemPrompt + "\n\n" + userPrompt,
		Completion:   result.Content,
	})

	// Record pipeline_runs provenance if source_id and registry are available
	if req.SourceId != nil && s.registry != nil {
		sourceID := req.GetSourceId()
		durationMs := int(time.Since(startTime).Milliseconds())

		// Get the repository from the registry for direct DB access
		// The registry doesn't expose pipeline_runs insert, so we'll log and skip
		s.logger.Debug("Pipeline provenance tracking",
			logging.F("source_id", sourceID),
			logging.F("stage", "triage"),
			logging.F("model", result.Model),
			logging.F("duration_ms", durationMs),
			logging.F("status", "completed"),
		)
		// Note: Direct pipeline_runs insert would require exposing DB from registry
		// For now, this will be handled at the gateway/workflow level
	}

	s.logger.Debug("TriageContent completed",
		logging.F("category", triageResult.Category),
		logging.F("importance", triageResult.Importance),
		logging.F("content_contribution", triageResult.ContentContribution),
		logging.F("retries", retryCount),
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

	// If registry is available, use it for comprehensive model status
	if s.registry != nil {
		return s.getModelStatusFromRegistry(ctx, req)
	}

	// Fall back to direct backend checks for backwards compatibility
	return s.getModelStatusFromBackend(ctx, req)
}

// getModelStatusFromRegistry retrieves model status from the registry.
func (s *AIServer) getModelStatusFromRegistry(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	// Build filter from request
	filter := &registry.ModelFilter{}
	if req.GetModelType() == aiv1.ModelType_MODEL_TYPE_EMBEDDING {
		filter.Capability = registry.CapabilityEmbedding
	} else if req.GetModelType() == aiv1.ModelType_MODEL_TYPE_LLM {
		filter.Capability = registry.CapabilityChat
	}

	// List models from registry
	regModels := s.registry.List(filter)

	// Convert to proto and check health
	models := make([]*aiv1.ModelInfo, 0, len(regModels))
	healthy := true
	var messages []string

	for _, m := range regModels {
		// Filter by model name if specified
		if req.GetModelName() != "" && m.Name != req.GetModelName() && m.ModelName != req.GetModelName() {
			continue
		}

		info := s.modelConfigToProto(m)
		models = append(models, info)

		// Track overall health
		if !m.Health.IsAvailable() {
			healthy = false
			messages = append(messages, fmt.Sprintf("%s: %s", m.Name, m.Health.ErrorMessage))
		}
	}

	// Also check router health if available
	if s.router != nil {
		routerHealth := s.router.GetAllHealth()
		for name, isHealthy := range routerHealth {
			if !isHealthy {
				healthy = false
				messages = append(messages, fmt.Sprintf("router/%s: unhealthy", name))
			}
		}
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

	s.logger.Debug("GetModelStatus completed (registry)",
		logging.F("models_count", len(models)),
		logging.F("healthy", healthy),
	)

	return resp, nil
}

// getModelStatusFromBackend retrieves model status directly from the backend.
func (s *AIServer) getModelStatusFromBackend(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	models := make([]*aiv1.ModelInfo, 0)
	healthy := true
	var messages []string

	// Check embeddings backend
	if s.backend != nil {
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

	s.logger.Debug("GetModelStatus completed (backend)",
		logging.F("models_count", len(models)),
		logging.F("healthy", healthy),
	)

	return resp, nil
}

// Helper methods

// selectBackend determines which backend to use based on the model name.
// Tries DB-backed provider lookup first via registry, falls back to string heuristic.
func (s *AIServer) selectBackend(ctx context.Context, model string) string {
	// Try DB first via registry (pf-b158ef)
	if s.registry != nil {
		provider, err := s.registry.GetModelProviderByName(ctx, model)
		if err == nil && provider != "" {
			return provider
		}
	}
	// Fallback: string heuristic for unknown models
	if strings.Contains(strings.ToLower(model), "gemini") {
		return "gemini"
	}
	return "ollama"
}

// tracePromptFromMessages concatenates message contents for tracing.
func tracePromptFromMessages(messages []backend.Message) string {
	var parts []string
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return strings.Join(parts, "\n\n")
}

func (s *AIServer) buildSummarySystemPrompt(ctx context.Context, style aiv1.SummaryStyle, maxLength int32, version int32) string {
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

	hardcoded := fmt.Sprintf(`You are a summarization assistant. %s%s

After the summary, provide 3-5 key points as a JSON array.

Format your response as:
SUMMARY:
[Your summary here]

KEY_POINTS:
["point 1", "point 2", "point 3"]`, styleInstruction, lengthInstruction)

	if s.promptStore != nil {
		pt, err := s.promptStore.GetPromptByStage(ctx, "summary", int(version))
		if err == nil && pt != nil {
			// Replace runtime placeholders in the DB template.
			r := strings.NewReplacer(
				"{style_instruction}", styleInstruction,
				"{length_instruction}", lengthInstruction,
			)
			return r.Replace(pt.Content)
		}
		if err != nil {
			s.logger.Warn("prompt store lookup failed for summary, using hardcoded fallback", logging.Err(err))
		}
	}
	return hardcoded
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

func (s *AIServer) buildAssertionSystemPrompt(ctx context.Context) (string, int32) {
	const hardcoded = `You are a business intelligence extraction assistant. Extract meaningful business assertions from the content as subject-predicate-object triples.

For each assertion, provide:
- subject: The entity being discussed (a person, team, project, or system)
- predicate: The relationship or action (what was decided, committed to, assigned, etc.)
- object: What the subject relates to
- confidence: Your confidence score (0.0-1.0)
- source_text: The exact text supporting this assertion
- category: One of: decision, risk, commitment, milestone, outcome, dependency, assumption, issue, action, question

Category definitions:
- decision: A choice or determination that was made ("we decided to use Kafka")
- risk: A potential problem or threat identified ("data loss if migration fails")
- commitment: A promise or obligation undertaken ("James will deliver the API by Friday")
- milestone: A significant achievement or deadline ("v2.0 launch on March 1")
- outcome: A result or consequence of an action ("migration reduced latency by 40%")
- dependency: A reliance on another system, team, or deliverable ("blocked on auth team")
- assumption: An unstated belief underpinning a plan ("assumes 1000 concurrent users")
- issue: A current problem or concern ("builds are failing on CI")
- action: A task or follow-up assigned to someone ("Sarah to update the docs")
- question: An unresolved question requiring an answer ("do we need HIPAA compliance?")

Focus on business-meaningful assertions: decisions made, risks identified, actions assigned, commitments given, and dependencies noted. Skip trivial metadata like attendee lists or meeting logistics.

Deduplicate before responding: do not extract multiple assertions that describe the same action, decision, or fact in different words. If two assertions are semantically equivalent, keep only the more specific one.

Always include a confidence score for every assertion. Never omit the confidence field.

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
}`
	content, version := s.getPrompt(ctx, "extract_assertions", hardcoded, 0)
	return content, version
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

func (s *AIServer) buildClassificationSystemPrompt(ctx context.Context, categories []string, multiLabel bool) string {
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

	hardcoded := fmt.Sprintf(`You are a content classification assistant. %s%s

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

	if s.promptStore != nil {
		pt, err := s.promptStore.GetPromptByStage(ctx, "classification", 0)
		if err == nil && pt != nil {
			// Replace runtime placeholders in the DB template.
			r := strings.NewReplacer(
				"{category_instruction}", categoryInstruction,
				"{multi_label_instruction}", multiLabelInstruction,
			)
			return r.Replace(pt.Content)
		}
		if err != nil {
			s.logger.Warn("prompt store lookup failed for classification, using hardcoded fallback", logging.Err(err))
		}
	}
	return hardcoded
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
	case errors.Is(err, context.Canceled), errors.Is(err, backend.ErrContextCanceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, backend.ErrServiceUnavailable):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, backend.ErrEmptyText), errors.Is(err, backend.ErrEmptyMessages):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, backend.ErrRequestTimeout):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, registry.ErrModelNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, registry.ErrModelAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, registry.ErrInvalidModelID), errors.Is(err, registry.ErrInvalidModelName),
		errors.Is(err, registry.ErrInvalidProvider), errors.Is(err, registry.ErrNoCapabilities),
		errors.Is(err, registry.ErrInvalidRoutingRule):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, registry.ErrRoutingRuleNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// =============================================================================
// Model Management RPCs
// =============================================================================

// ListModels returns all registered models with optional filtering.
func (s *AIServer) ListModels(ctx context.Context, req *aiv1.ListModelsRequest) (*aiv1.ListModelsResponse, error) {
	s.logger.Debug("ListModels called",
		logging.F("provider", req.GetProvider()),
		logging.F("capability", req.GetCapability()),
		logging.F("is_local", req.IsLocal),
		logging.F("is_enabled", req.IsEnabled),
	)

	if s.registry == nil {
		return nil, status.Error(codes.Unimplemented, "model registry not configured")
	}

	// Build filter from request
	filter := &registry.ModelFilter{}
	if req.Provider != nil {
		filter.Provider = registry.Provider(req.GetProvider())
	}
	if req.Capability != nil {
		filter.Capability = registry.Capability(req.GetCapability())
	}
	if req.IsLocal != nil {
		isLocal := req.GetIsLocal()
		filter.IsLocal = &isLocal
	}

	// List models from registry
	models := s.registry.List(filter)

	// Convert to proto
	protoModels := make([]*aiv1.ModelInfo, 0, len(models))
	for _, m := range models {
		protoModels = append(protoModels, s.modelConfigToProto(m))
	}

	s.logger.Debug("ListModels completed",
		logging.F("models_count", len(protoModels)),
	)

	return &aiv1.ListModelsResponse{
		Models:     protoModels,
		TotalCount: int32(len(protoModels)),
	}, nil
}

// RegisterModel adds a new model to the registry.
func (s *AIServer) RegisterModel(ctx context.Context, req *aiv1.RegisterModelRequest) (*aiv1.RegisterModelResponse, error) {
	s.logger.Debug("RegisterModel called",
		logging.F("name", req.GetName()),
		logging.F("provider", req.GetProvider()),
		logging.F("model_name", req.GetModelName()),
		logging.F("type", req.GetType().String()),
	)

	if s.registry == nil {
		return nil, status.Error(codes.Unimplemented, "model registry not configured")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetProvider() == "" {
		return nil, status.Error(codes.InvalidArgument, "provider is required")
	}
	if req.GetModelName() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_name is required")
	}
	if len(req.GetCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one capability is required")
	}

	// Generate ID
	modelID := fmt.Sprintf("%s/%s", req.GetProvider(), req.GetModelName())

	// Convert capabilities
	caps := make([]registry.Capability, len(req.GetCapabilities()))
	for i, c := range req.GetCapabilities() {
		caps[i] = registry.Capability(c)
	}

	// Create model config
	model := &registry.ModelConfig{
		ID:        modelID,
		Name:      req.GetName(),
		Provider:  registry.Provider(req.GetProvider()),
		ModelName: req.GetModelName(),
		Endpoint:  req.GetEndpoint(),
		Capabilities: registry.ModelCapabilities{
			Capabilities:        caps,
			EmbeddingDimensions: int(req.GetEmbeddingDimensions()),
		},
		Limits: registry.ModelLimits{
			ContextWindow: int(req.GetMaxContextLength()),
		},
		Cost: registry.ModelCost{
			InputCostPer1K:  req.GetInputCostPer_1K(),
			OutputCostPer1K: req.GetOutputCostPer_1K(),
			Currency:        "USD",
		},
		IsLocal:  req.GetIsLocal(),
		Priority: int(req.GetPriority()),
	}

	// Register in registry
	if err := s.registry.Register(model); err != nil {
		s.logger.Error("RegisterModel failed",
			logging.Err(err),
			logging.F("model_id", modelID),
		)
		return nil, s.convertError(err)
	}

	s.logger.Info("Model registered",
		logging.F("model_id", modelID),
		logging.F("name", req.GetName()),
	)

	return &aiv1.RegisterModelResponse{
		Model: s.modelConfigToProto(model),
	}, nil
}

// UpdateModel modifies an existing model's configuration.
func (s *AIServer) UpdateModel(ctx context.Context, req *aiv1.UpdateModelRequest) (*aiv1.UpdateModelResponse, error) {
	s.logger.Debug("UpdateModel called",
		logging.F("model_id", req.GetModelId()),
	)

	if s.registry == nil {
		return nil, status.Error(codes.Unimplemented, "model registry not configured")
	}

	if req.GetModelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id is required")
	}

	// Get existing model
	existing, err := s.registry.Get(req.GetModelId())
	if err != nil {
		return nil, s.convertError(err)
	}

	// Apply updates
	if req.Name != nil {
		existing.Name = req.GetName()
	}
	if req.Endpoint != nil {
		existing.Endpoint = req.GetEndpoint()
	}
	if len(req.GetCapabilities()) > 0 {
		caps := make([]registry.Capability, len(req.GetCapabilities()))
		for i, c := range req.GetCapabilities() {
			caps[i] = registry.Capability(c)
		}
		existing.Capabilities.Capabilities = caps
	}
	if req.MaxContextLength != nil {
		existing.Limits.ContextWindow = int(req.GetMaxContextLength())
	}
	if req.EmbeddingDimensions != nil {
		existing.Capabilities.EmbeddingDimensions = int(req.GetEmbeddingDimensions())
	}
	if req.InputCostPer_1K != nil {
		existing.Cost.InputCostPer1K = req.GetInputCostPer_1K()
	}
	if req.OutputCostPer_1K != nil {
		existing.Cost.OutputCostPer1K = req.GetOutputCostPer_1K()
	}
	if req.Priority != nil {
		existing.Priority = int(req.GetPriority())
	}

	// Update in registry
	if err := s.registry.Update(existing); err != nil {
		s.logger.Error("UpdateModel failed",
			logging.Err(err),
			logging.F("model_id", req.GetModelId()),
		)
		return nil, s.convertError(err)
	}

	s.logger.Info("Model updated",
		logging.F("model_id", req.GetModelId()),
	)

	return &aiv1.UpdateModelResponse{
		Model: s.modelConfigToProto(existing),
	}, nil
}

// DeleteModel removes a model from the registry.
func (s *AIServer) DeleteModel(ctx context.Context, req *aiv1.DeleteModelRequest) (*aiv1.DeleteModelResponse, error) {
	s.logger.Debug("DeleteModel called",
		logging.F("model_id", req.GetModelId()),
	)

	if s.registry == nil {
		return nil, status.Error(codes.Unimplemented, "model registry not configured")
	}

	if req.GetModelId() == "" {
		return nil, status.Error(codes.InvalidArgument, "model_id is required")
	}

	// Delete from registry
	if err := s.registry.Unregister(req.GetModelId()); err != nil {
		if errors.Is(err, registry.ErrModelNotFound) {
			return &aiv1.DeleteModelResponse{
				Deleted: false,
				Message: "model not found",
			}, nil
		}
		s.logger.Error("DeleteModel failed",
			logging.Err(err),
			logging.F("model_id", req.GetModelId()),
		)
		return nil, s.convertError(err)
	}

	s.logger.Info("Model deleted",
		logging.F("model_id", req.GetModelId()),
	)

	return &aiv1.DeleteModelResponse{
		Deleted: true,
		Message: "model deleted successfully",
	}, nil
}

// =============================================================================
// Routing Rule RPCs
// =============================================================================

// GetRoutingRules returns routing rules with optional filtering.
func (s *AIServer) GetRoutingRules(ctx context.Context, req *aiv1.GetRoutingRulesRequest) (*aiv1.GetRoutingRulesResponse, error) {
	s.logger.Debug("GetRoutingRules called",
		logging.F("task_type", req.GetTaskType()),
		logging.F("is_enabled", req.IsEnabled),
	)

	if s.registry == nil {
		return nil, status.Error(codes.Unimplemented, "model registry not configured")
	}

	var rules []*registry.RoutingRule
	var err error

	if req.TaskType != nil && req.GetTaskType() != "" {
		rules, err = s.registry.GetRoutingRulesByTask(ctx, req.GetTaskType())
	} else {
		rules, err = s.registry.GetRoutingRules(ctx)
	}

	if err != nil {
		s.logger.Error("GetRoutingRules failed",
			logging.Err(err),
		)
		return nil, s.convertError(err)
	}

	// Convert to proto
	protoRules := make([]*aiv1.RoutingRule, 0, len(rules))
	for _, r := range rules {
		// Apply enabled filter if specified
		if req.IsEnabled != nil && r.IsEnabled != req.GetIsEnabled() {
			continue
		}
		protoRules = append(protoRules, s.routingRuleToProto(r))
	}

	s.logger.Debug("GetRoutingRules completed",
		logging.F("rules_count", len(protoRules)),
	)

	return &aiv1.GetRoutingRulesResponse{
		Rules: protoRules,
	}, nil
}

// UpdateRoutingRule creates or updates a routing rule.
func (s *AIServer) UpdateRoutingRule(ctx context.Context, req *aiv1.UpdateRoutingRuleRequest) (*aiv1.UpdateRoutingRuleResponse, error) {
	s.logger.Debug("UpdateRoutingRule called",
		logging.F("name", req.GetName()),
		logging.F("task_type", req.GetTaskType()),
	)

	if s.registry == nil {
		return nil, status.Error(codes.Unimplemented, "model registry not configured")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetTaskType() == "" {
		return nil, status.Error(codes.InvalidArgument, "task_type is required")
	}
	if len(req.GetPreferredModelIds()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one preferred_model_id is required")
	}

	// Convert optimization mode
	optMode := s.protoOptimizationModeToString(req.GetOptimizationMode())

	// Check if rule exists by name using direct lookup
	existingRule, err := s.registry.GetRoutingRuleByName(ctx, req.GetName())
	if err != nil && !errors.Is(err, registry.ErrRoutingRuleNotFound) {
		s.logger.Error("Failed to check existing rule",
			logging.Err(err),
		)
		return nil, s.convertError(err)
	}

	created := false
	rule := &registry.RoutingRule{
		Name:            req.GetName(),
		TaskType:        req.GetTaskType(),
		PreferredModels: req.GetPreferredModelIds(),
		FallbackModels:  req.GetFallbackModelIds(),
		OptimizationMode: optMode,
		IsEnabled:       req.GetIsEnabled(),
	}

	if existingRule != nil {
		// Update existing rule
		rule.ID = existingRule.ID
		rule.CreatedAt = existingRule.CreatedAt
		if err := s.registry.UpdateRoutingRule(ctx, rule); err != nil {
			s.logger.Error("UpdateRoutingRule failed",
				logging.Err(err),
				logging.F("name", req.GetName()),
			)
			return nil, s.convertError(err)
		}
		s.logger.Info("Routing rule updated",
			logging.F("rule_id", rule.ID),
			logging.F("name", rule.Name),
		)
	} else {
		// Create new rule
		rule.ID = uuid.New().String()
		// For new rules, use the is_enabled value from request (default false becomes true if not set explicitly)
		rule.IsEnabled = req.GetIsEnabled()
		if err := s.registry.CreateRoutingRule(ctx, rule); err != nil {
			s.logger.Error("CreateRoutingRule failed",
				logging.Err(err),
				logging.F("name", req.GetName()),
			)
			return nil, s.convertError(err)
		}
		created = true
		s.logger.Info("Routing rule created",
			logging.F("rule_id", rule.ID),
			logging.F("name", rule.Name),
		)
	}

	return &aiv1.UpdateRoutingRuleResponse{
		Rule:    s.routingRuleToProto(rule),
		Created: created,
	}, nil
}

// =============================================================================
// Proto Conversion Helpers
// =============================================================================

// modelConfigToProto converts a registry.ModelConfig to aiv1.ModelInfo.
func (s *AIServer) modelConfigToProto(m *registry.ModelConfig) *aiv1.ModelInfo {
	info := &aiv1.ModelInfo{
		Id:       m.ID,
		Name:     m.Name,
		Provider: string(m.Provider),
		ModelName: m.ModelName,
		IsLocal:  m.IsLocal,
		Priority: int32(m.Priority),
	}

	// Map provider to model type
	switch {
	case m.Capabilities.HasCapability(registry.CapabilityEmbedding):
		info.Type = aiv1.ModelType_MODEL_TYPE_EMBEDDING
		dims := int32(m.Capabilities.EmbeddingDimensions)
		info.EmbeddingDimensions = &dims
	case m.Capabilities.HasCapability(registry.CapabilityClassification):
		info.Type = aiv1.ModelType_MODEL_TYPE_CLASSIFIER
	default:
		info.Type = aiv1.ModelType_MODEL_TYPE_LLM
	}

	// Map health status
	switch m.Health.Status {
	case registry.HealthStatusHealthy:
		info.Status = aiv1.ModelStatus_MODEL_STATUS_READY
	case registry.HealthStatusDegraded:
		info.Status = aiv1.ModelStatus_MODEL_STATUS_LOADING
	case registry.HealthStatusUnhealthy:
		info.Status = aiv1.ModelStatus_MODEL_STATUS_ERROR
		errMsg := m.Health.ErrorMessage
		info.ErrorMessage = &errMsg
	case registry.HealthStatusLoading:
		info.Status = aiv1.ModelStatus_MODEL_STATUS_LOADING
	default:
		info.Status = aiv1.ModelStatus_MODEL_STATUS_UNSPECIFIED
	}

	// Capabilities
	caps := make([]string, len(m.Capabilities.Capabilities))
	for i, c := range m.Capabilities.Capabilities {
		caps[i] = string(c)
	}
	info.Capabilities = caps

	// Limits
	if m.Limits.ContextWindow > 0 {
		ctx := int32(m.Limits.ContextWindow)
		info.MaxContextLength = &ctx
	}

	// Cost
	if m.Cost.InputCostPer1K > 0 {
		info.InputCostPer_1K = &m.Cost.InputCostPer1K
	}
	if m.Cost.OutputCostPer1K > 0 {
		info.OutputCostPer_1K = &m.Cost.OutputCostPer1K
	}

	// Endpoint
	if m.Endpoint != "" {
		info.Endpoint = &m.Endpoint
	}

	// Metrics
	if m.Health.AvgLatencyMs > 0 {
		info.AvgLatencyMs = &m.Health.AvgLatencyMs
	}
	if m.Health.RequestCount > 0 {
		info.RequestsLastHour = &m.Health.RequestCount
	}

	return info
}

// routingRuleToProto converts a registry.RoutingRule to aiv1.RoutingRule.
func (s *AIServer) routingRuleToProto(r *registry.RoutingRule) *aiv1.RoutingRule {
	rule := &aiv1.RoutingRule{
		Id:                r.ID,
		Name:              r.Name,
		TaskType:          r.TaskType,
		PreferredModelIds: r.PreferredModels,
		FallbackModelIds:  r.FallbackModels,
		OptimizationMode:  s.stringToProtoOptimizationMode(r.OptimizationMode),
		IsEnabled:         r.IsEnabled,
		CreatedAt:         r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         r.CreatedAt.Format(time.RFC3339), // Use CreatedAt if no UpdatedAt
	}

	return rule
}

// stringToProtoOptimizationMode converts a string optimization mode to proto enum.
func (s *AIServer) stringToProtoOptimizationMode(mode string) aiv1.OptimizationMode {
	switch mode {
	case registry.OptimizationModeLatency:
		return aiv1.OptimizationMode_OPTIMIZATION_MODE_LATENCY
	case registry.OptimizationModeQuality:
		return aiv1.OptimizationMode_OPTIMIZATION_MODE_QUALITY
	case registry.OptimizationModeCost:
		return aiv1.OptimizationMode_OPTIMIZATION_MODE_COST
	case registry.OptimizationModeBalanced:
		return aiv1.OptimizationMode_OPTIMIZATION_MODE_BALANCED
	default:
		return aiv1.OptimizationMode_OPTIMIZATION_MODE_UNSPECIFIED
	}
}

// protoOptimizationModeToString converts a proto optimization mode to string.
func (s *AIServer) protoOptimizationModeToString(mode aiv1.OptimizationMode) string {
	switch mode {
	case aiv1.OptimizationMode_OPTIMIZATION_MODE_LATENCY:
		return registry.OptimizationModeLatency
	case aiv1.OptimizationMode_OPTIMIZATION_MODE_QUALITY:
		return registry.OptimizationModeQuality
	case aiv1.OptimizationMode_OPTIMIZATION_MODE_COST:
		return registry.OptimizationModeCost
	case aiv1.OptimizationMode_OPTIMIZATION_MODE_BALANCED:
		return registry.OptimizationModeBalanced
	default:
		return registry.OptimizationModeBalanced
	}
}
