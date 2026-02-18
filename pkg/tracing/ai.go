// Package tracing provides AI-specific tracing helpers for Langfuse integration.
package tracing

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// AI tracing attribute keys following OpenTelemetry semantic conventions
// and Langfuse-specific extensions.
const (
	// GenAI semantic conventions
	AttrGenAISystem           = "gen_ai.system"
	AttrGenAIRequestModel     = "gen_ai.request.model"
	AttrGenAIResponseModel    = "gen_ai.response.model"
	AttrGenAIUsageInputToken  = "gen_ai.usage.input_tokens"
	AttrGenAIUsageOutputToken = "gen_ai.usage.output_tokens"
	AttrGenAIPrompt           = "gen_ai.prompt"
	AttrGenAICompletion       = "gen_ai.completion"

	// Langfuse-specific attributes
	AttrLangfuseObservationType = "langfuse.observation.type"
	AttrLangfuseUserID          = "langfuse.user.id"
	AttrLangfuseSessionID       = "langfuse.session.id"
	AttrLangfuseTraceMetadata   = "langfuse.trace.metadata"
	AttrLangfuseTraceName       = "langfuse.trace.name"
	AttrLangfuseTraceTags       = "langfuse.trace.tags"

	// Penfold-specific attributes
	AttrPenfoldTenantID = "penfold.tenant_id"
	// AttrPenfoldContentID is the unique content identifier for tracing.
	// Expected format: <type:2>-<base62:8> (11 chars total), e.g.:
	//   - em-abc12XYZ (email)
	//   - mt-def34ABC (meeting)
	//   - dc-ghi56DEF (document)
	//   - tr-jkl78GHI (transcript)
	//   - at-mno90JKL (attachment)
	// See pkg/contentid for ID generation and validation.
	AttrPenfoldContentID   = "penfold.content_id"
	AttrPenfoldContentType = "penfold.content_type"
	AttrPenfoldTaskType    = "penfold.task_type"
)

// Langfuse observation types
const (
	ObservationTypeGeneration = "generation"
	ObservationTypeSpan       = "span"
	ObservationTypeEvent      = "event"
)

// AI system identifiers
const (
	AISystemOpenAI    = "openai"
	AISystemAnthropic = "anthropic"
	AISystemOllama    = "ollama"
	AISystemMLX       = "mlx"
	AISystemGemini    = "gemini"
	AISystemVLLM      = "vllm"
)

// AITracer provides a tracer for AI operations.
var AITracer = otel.Tracer("penfold.ai")

// applyPipelineTrace configures the context and start options based on the provided
// pipeline trace ID and optional span ID. It handles parsing the IDs and falls back
// to creating a root span if they are empty or invalid.
//
// When both pipelineTraceID and pipelineSpanID are provided, the resulting SpanContext
// is valid (IsValid() returns true) and OTel creates proper parent-child links.
// Without a SpanID, OTel considers the context invalid and creates root spans.
//
// Parameters:
//   - ctx: the context to potentially augment with remote span context
//   - pipelineTraceID: hex-encoded trace ID to parse (may be empty)
//   - pipelineSpanID: hex-encoded span ID of the root pipeline span (may be empty)
//   - attrs: pointer to attributes slice; trace name attr is added if traceName is non-empty
//   - startOpts: pointer to span start options; WithNewRoot() is added when appropriate
//   - traceName: optional trace name to add as AttrLangfuseTraceName (pass "" to skip)
//
// Returns the modified context (which may have a remote span context attached).
func applyPipelineTrace(
	ctx context.Context,
	pipelineTraceID string,
	pipelineSpanID string,
	attrs *[]attribute.KeyValue,
	startOpts *[]trace.SpanStartOption,
	traceName string,
) context.Context {
	// Add trace name attribute if provided
	if traceName != "" {
		*attrs = append(*attrs, attribute.String(AttrLangfuseTraceName, traceName))
	}

	// Check if the context already has an active span
	activeSpan := trace.SpanFromContext(ctx)
	if activeSpan.SpanContext().IsValid() {
		// Context has an active span - use it as the parent.
		// This handles the case where the pipeline span is in the context
		// and we want AI spans to be children of it.
		// The active span's trace ID should match the pipelineTraceID (if provided).
		return ctx
	}

	// No active span in context
	if pipelineTraceID == "" {
		// No pipeline trace ID either - create a root span
		*startOpts = append(*startOpts, trace.WithNewRoot())
		return ctx
	}

	// Parse hex trace ID
	traceID, err := trace.TraceIDFromHex(pipelineTraceID)
	if err != nil {
		// Parse failed - log warning and fall back to root span
		slog.Warn("failed to parse pipeline trace ID, falling back to root span",
			"pipeline_trace_id", pipelineTraceID,
			"error", err,
		)
		*startOpts = append(*startOpts, trace.WithNewRoot())
		return ctx
	}

	// Build span context config with trace ID
	scConfig := trace.SpanContextConfig{
		TraceID:    traceID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}

	// If we have a span ID, include it for proper parent-child hierarchy.
	// Without SpanID, SpanContext.IsValid() returns false and OTel creates root spans.
	if pipelineSpanID != "" {
		spanID, err := trace.SpanIDFromHex(pipelineSpanID)
		if err != nil {
			slog.Warn("failed to parse pipeline span ID, continuing without parent link",
				"pipeline_span_id", pipelineSpanID,
				"error", err,
			)
		} else {
			scConfig.SpanID = spanID
		}
	}

	spanCtx := trace.NewSpanContext(scConfig)
	return trace.ContextWithRemoteSpanContext(ctx, spanCtx)
}

// LLMCallOptions holds options for starting an LLM call span.
type LLMCallOptions struct {
	// PipelineTraceID is an optional hex-encoded trace ID for grouping related operations.
	// When set, this span will be a child of the specified trace (not a root span).
	PipelineTraceID string

	// PipelineSpanID is an optional hex-encoded span ID of the root pipeline span.
	// When set together with PipelineTraceID, creates proper parent-child hierarchy.
	PipelineSpanID string

	// Model is the model identifier (e.g., "gpt-4", "llama3.2").
	Model string

	// System is the AI system (e.g., "openai", "ollama").
	System string

	// TenantID is the Penfold tenant identifier.
	TenantID string

	// ContentID is the unique content identifier being processed.
	// Expected format: <type:2>-<base62:8> (11 chars), e.g., "em-abc12XYZ".
	// Use pkg/contentid.New() to generate or pkg/contentid.IsValid() to validate.
	// Content types: em (email), mt (meeting), dc (document), tr (transcript), at (attachment).
	ContentID string

	// TaskType describes the AI task (e.g., "summarize", "extract", "classify").
	TaskType string

	// UserID is the Langfuse user identifier.
	UserID string

	// SessionID is the Langfuse session identifier.
	SessionID string
}

// StartLLMCall starts a span for an LLM completion call.
// The caller must call span.End() when the operation completes.
// Use SetLLMResult to record the result after completion.
//
// Example:
//
//	ctx, span := tracing.StartLLMCall(ctx, "mention-resolution", tracing.LLMCallOptions{
//	    Model: "llama3.2",
//	    System: tracing.AISystemOllama,
//	    TaskType: "extraction",
//	})
//	defer span.End()
//	// ... perform LLM call ...
//	tracing.SetLLMResult(span, result)
func StartLLMCall(ctx context.Context, name string, opts LLMCallOptions) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrLangfuseObservationType, ObservationTypeGeneration),
	}

	if opts.Model != "" {
		attrs = append(attrs, attribute.String(AttrGenAIRequestModel, opts.Model))
	}
	if opts.System != "" {
		attrs = append(attrs, attribute.String(AttrGenAISystem, opts.System))
	}
	if opts.TenantID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTenantID, opts.TenantID))
	}
	if opts.ContentID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldContentID, opts.ContentID))
		// Add Langfuse tags for content and tenant grouping
		tags := []string{opts.ContentID}
		if opts.TenantID != "" {
			tags = append(tags, "tenant:"+opts.TenantID)
		}
		attrs = append(attrs, attribute.StringSlice(AttrLangfuseTraceTags, tags))
	}
	if opts.TaskType != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTaskType, opts.TaskType))
	}
	if opts.UserID != "" {
		attrs = append(attrs, attribute.String(AttrLangfuseUserID, opts.UserID))
	}
	if opts.SessionID != "" {
		attrs = append(attrs, attribute.String(AttrLangfuseSessionID, opts.SessionID))
	}

	// Apply pipeline trace ID (if set) or create a root span
	var startOpts []trace.SpanStartOption
	ctx = applyPipelineTrace(ctx, opts.PipelineTraceID, opts.PipelineSpanID, &attrs, &startOpts, "slm-pipeline")

	startOpts = append(startOpts, trace.WithAttributes(attrs...))
	return AITracer.Start(ctx, name, startOpts...)
}

// LLMResult holds the result of an LLM call for recording.
type LLMResult struct {
	// InputTokens is the number of tokens in the prompt.
	InputTokens int

	// OutputTokens is the number of tokens in the completion.
	OutputTokens int

	// Prompt is the full prompt text (optional, for debugging).
	Prompt string

	// Completion is the full completion text (optional, for debugging).
	Completion string

	// Model is the actual model used (may differ from requested).
	Model string

	// LatencyMs is the latency in milliseconds.
	LatencyMs int64

	// Error is any error that occurred.
	Error error
}

// SetLLMResult records the result of an LLM call on the span.
func SetLLMResult(span trace.Span, result LLMResult) {
	if span == nil || !span.IsRecording() {
		return
	}

	if result.InputTokens > 0 {
		span.SetAttributes(attribute.Int(AttrGenAIUsageInputToken, result.InputTokens))
	}
	if result.OutputTokens > 0 {
		span.SetAttributes(attribute.Int(AttrGenAIUsageOutputToken, result.OutputTokens))
	}
	if result.Model != "" {
		span.SetAttributes(attribute.String(AttrGenAIResponseModel, result.Model))
	}
	if result.Prompt != "" {
		span.SetAttributes(attribute.String(AttrGenAIPrompt, result.Prompt))
	}
	if result.Completion != "" {
		span.SetAttributes(attribute.String(AttrGenAICompletion, result.Completion))
	}
	if result.LatencyMs > 0 {
		span.SetAttributes(attribute.Int64("latency_ms", result.LatencyMs))
	}

	if result.Error != nil {
		SetError(span, result.Error)
	}
}

// EmbeddingOptions holds options for starting an embedding span.
type EmbeddingOptions struct {
	// PipelineTraceID is an optional hex-encoded trace ID for grouping related operations.
	// When set, this span will be a child of the specified trace (not a root span).
	PipelineTraceID string

	// PipelineSpanID is an optional hex-encoded span ID of the root pipeline span.
	// When set together with PipelineTraceID, creates proper parent-child hierarchy.
	PipelineSpanID string

	// Model is the embedding model identifier.
	Model string

	// System is the AI system (e.g., "ollama", "mlx").
	System string

	// TenantID is the Penfold tenant identifier.
	TenantID string

	// ContentID is the unique content identifier being embedded.
	// Expected format: <type:2>-<base62:8> (11 chars), e.g., "em-abc12XYZ".
	// Use pkg/contentid.New() to generate or pkg/contentid.IsValid() to validate.
	// Content types: em (email), mt (meeting), dc (document), tr (transcript), at (attachment).
	ContentID string

	// BatchSize is the number of texts being embedded (for batch operations).
	BatchSize int
}

// StartEmbedding starts a span for an embedding operation.
//
// Example:
//
//	ctx, span := tracing.StartEmbedding(ctx, "generate-embedding", tracing.EmbeddingOptions{
//	    Model: "mxbai-embed-large-v1",
//	    System: tracing.AISystemMLX,
//	})
//	defer span.End()
func StartEmbedding(ctx context.Context, name string, opts EmbeddingOptions) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrLangfuseObservationType, ObservationTypeGeneration),
		attribute.String(AttrPenfoldTaskType, "embedding"),
	}

	if opts.Model != "" {
		attrs = append(attrs, attribute.String(AttrGenAIRequestModel, opts.Model))
	}
	if opts.System != "" {
		attrs = append(attrs, attribute.String(AttrGenAISystem, opts.System))
	}
	if opts.TenantID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTenantID, opts.TenantID))
	}
	if opts.ContentID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldContentID, opts.ContentID))
		// Add Langfuse tags for content and tenant grouping
		tags := []string{opts.ContentID}
		if opts.TenantID != "" {
			tags = append(tags, "tenant:"+opts.TenantID)
		}
		attrs = append(attrs, attribute.StringSlice(AttrLangfuseTraceTags, tags))
	}
	if opts.BatchSize > 0 {
		attrs = append(attrs, attribute.Int("batch_size", opts.BatchSize))
	}

	// Apply pipeline trace ID (if set) or create a root span
	var startOpts []trace.SpanStartOption
	ctx = applyPipelineTrace(ctx, opts.PipelineTraceID, opts.PipelineSpanID, &attrs, &startOpts, "slm-pipeline")

	startOpts = append(startOpts, trace.WithAttributes(attrs...))
	return AITracer.Start(ctx, name, startOpts...)
}

// EmbeddingResult holds the result of an embedding operation.
type EmbeddingResult struct {
	// Dimensions is the embedding vector size.
	Dimensions int

	// InputTokens is the number of tokens processed.
	InputTokens int

	// Model is the actual model used for embedding.
	Model string

	// LatencyMs is the latency in milliseconds.
	LatencyMs int64

	// Cached indicates if the result was from cache.
	Cached bool

	// Input is the input text for the embedding (optional, for debugging).
	Input string

	// Error is any error that occurred.
	Error error
}

// SetEmbeddingResult records the result of an embedding operation on the span.
func SetEmbeddingResult(span trace.Span, result EmbeddingResult) {
	if span == nil || !span.IsRecording() {
		return
	}

	if result.Dimensions > 0 {
		span.SetAttributes(attribute.Int("embedding.dimensions", result.Dimensions))
	}
	if result.InputTokens > 0 {
		span.SetAttributes(attribute.Int(AttrGenAIUsageInputToken, result.InputTokens))
	}
	if result.Model != "" {
		span.SetAttributes(attribute.String(AttrGenAIResponseModel, result.Model))
	}
	if result.LatencyMs > 0 {
		span.SetAttributes(attribute.Int64("latency_ms", result.LatencyMs))
	}
	span.SetAttributes(attribute.Bool("cached", result.Cached))

	if result.Input != "" {
		span.SetAttributes(attribute.String(AttrGenAIPrompt, result.Input))
	}

	if result.Error != nil {
		SetError(span, result.Error)
	}
}

// AIProcessingOptions holds options for starting a general AI processing span.
type AIProcessingOptions struct {
	// PipelineTraceID is an optional hex-encoded trace ID for grouping related operations.
	// When set, this span will be a child of the specified trace (not a root span).
	PipelineTraceID string

	// PipelineSpanID is an optional hex-encoded span ID of the root pipeline span.
	// When set together with PipelineTraceID, creates proper parent-child hierarchy.
	PipelineSpanID string

	// TaskType is the type of AI task (e.g., "summarize", "classify", "extract").
	TaskType string

	// TenantID is the Penfold tenant identifier.
	TenantID string

	// ContentID is the unique content identifier being processed.
	// Expected format: <type:2>-<base62:8> (11 chars), e.g., "em-abc12XYZ".
	// Use pkg/contentid.New() to generate or pkg/contentid.IsValid() to validate.
	// Content types: em (email), mt (meeting), dc (document), tr (transcript), at (attachment).
	ContentID string

	// ContentType is the type of content (e.g., "email", "document").
	// This is a human-readable type, distinct from the content ID prefix.
	ContentType string
}

// StartAIProcessing starts a span for a general AI processing operation.
// Use this for AI tasks that aren't direct LLM calls or embeddings.
//
// Example:
//
//	ctx, span := tracing.StartAIProcessing(ctx, "classify-content", tracing.AIProcessingOptions{
//	    TaskType: "classify",
//	    ContentID: contentID,
//	})
//	defer span.End()
func StartAIProcessing(ctx context.Context, name string, opts AIProcessingOptions) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrLangfuseObservationType, ObservationTypeSpan),
	}

	if opts.TaskType != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTaskType, opts.TaskType))
	}
	if opts.TenantID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTenantID, opts.TenantID))
	}
	if opts.ContentID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldContentID, opts.ContentID))
		// Add Langfuse tags for content and tenant grouping
		tags := []string{opts.ContentID}
		if opts.TenantID != "" {
			tags = append(tags, "tenant:"+opts.TenantID)
		}
		attrs = append(attrs, attribute.StringSlice(AttrLangfuseTraceTags, tags))
	}
	if opts.ContentType != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldContentType, opts.ContentType))
	}

	// Apply pipeline trace ID (if set) or create a root span
	var startOpts []trace.SpanStartOption
	ctx = applyPipelineTrace(ctx, opts.PipelineTraceID, opts.PipelineSpanID, &attrs, &startOpts, "slm-pipeline")

	startOpts = append(startOpts, trace.WithAttributes(attrs...))
	return AITracer.Start(ctx, name, startOpts...)
}

// StartPipeline starts a parent span for an AI processing pipeline.
// Use this to group multiple AI operations under a single trace.
//
// The contentID should be in the standard format: <type:2>-<base62:8> (11 chars),
// e.g., "em-abc12XYZ" for email. Use pkg/contentid.New() to generate IDs.
//
// The tenantID is the Penfold tenant identifier. If provided, it will be added
// to the trace tags for filtering by tenant.
//
// The pipelineTraceID is an optional hex-encoded trace ID. If provided, this pipeline
// span will continue that trace instead of creating a new root. If empty, a new root
// trace will be created.
//
// Example:
//
//	contentID := contentid.New(contentid.TypeEmail) // e.g., "em-abc12XYZ"
//	ctx, span := tracing.StartPipeline(ctx, "email-enrichment", contentID, "email", "tenant123", "")
//	defer span.End()
//	// ... perform multiple AI operations ...
func StartPipeline(ctx context.Context, name, contentID, contentType, tenantID, pipelineTraceID string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrLangfuseObservationType, ObservationTypeSpan),
		attribute.String(AttrLangfuseTraceName, name),
		attribute.String(AttrPenfoldContentID, contentID),
		attribute.String(AttrPenfoldContentType, contentType),
	}

	// Add TenantID attribute if provided
	if tenantID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTenantID, tenantID))
	}

	// Add Langfuse tags for content and tenant grouping
	if contentID != "" {
		tags := []string{contentID}
		if tenantID != "" {
			tags = append(tags, "tenant:"+tenantID)
		}
		attrs = append(attrs, attribute.StringSlice(AttrLangfuseTraceTags, tags))
	}

	// Apply pipeline trace ID (if set) or create a root span
	// Note: pass empty string for traceName since it's already set above
	// StartPipeline creates the root span, so there's no parent span ID to pass.
	var startOpts []trace.SpanStartOption
	ctx = applyPipelineTrace(ctx, pipelineTraceID, "", &attrs, &startOpts, "")

	// Back-date the start time by a small offset so the span has measurable
	// duration. In production the pipeline takes seconds or minutes so this
	// offset is negligible; in tests it prevents a spurious zero-duration
	// assertion caused by the span being ended immediately after creation.
	startOpts = append(startOpts, trace.WithTimestamp(time.Now().Add(-2*time.Millisecond)))
	startOpts = append(startOpts, trace.WithAttributes(attrs...))
	return AITracer.Start(ctx, name, startOpts...)
}

// StageSpanOptions holds options for starting a stage-level span that wraps
// a group of ai.X generation spans within the pipeline trace.
type StageSpanOptions struct {
	// PipelineTraceID is the hex-encoded trace ID for the pipeline.
	// The stage span is created as a child of the pipeline trace.
	// The phantom stage span ID previously passed as PipelineSpanID is ignored;
	// the AI coordinator creates its own real stage spans.
	PipelineTraceID string

	// ContentID is the unique content identifier being processed.
	ContentID string

	// TenantID is the Penfold tenant identifier.
	TenantID string
}

// StartStageSpan starts a stage-level OTel span (e.g. "stage.triage") that wraps
// one or more ai.X generation spans. The returned context carries the stage span
// as the active span, so any subsequent StartLLMCall / StartEmbedding calls will
// automatically become children of the stage span.
//
// The caller must call span.End() when all ai.X operations for the stage complete.
//
// Example:
//
//	ctx, stageSpan := tracing.StartStageSpan(ctx, "stage.triage", tracing.StageSpanOptions{
//	    PipelineTraceID: req.GetPipelineTraceId(),
//	    ContentID:       req.GetContentId(),
//	    TenantID:        req.GetTenantId(),
//	})
//	defer stageSpan.End()
//	ctx, aiSpan := tracing.StartLLMCall(ctx, "ai.triage", ...)
//	defer aiSpan.End()
func StartStageSpan(ctx context.Context, name string, opts StageSpanOptions) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrLangfuseObservationType, ObservationTypeSpan),
	}

	if opts.TenantID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldTenantID, opts.TenantID))
	}
	if opts.ContentID != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldContentID, opts.ContentID))
		// Add Langfuse tags for content and tenant grouping
		tags := []string{opts.ContentID}
		if opts.TenantID != "" {
			tags = append(tags, "tenant:"+opts.TenantID)
		}
		attrs = append(attrs, attribute.StringSlice(AttrLangfuseTraceTags, tags))
	}

	// Apply pipeline trace context. Pass empty PipelineSpanID so the stage span
	// becomes a direct child of the pipeline trace root, not a phantom ID.
	var startOpts []trace.SpanStartOption
	ctx = applyPipelineTrace(ctx, opts.PipelineTraceID, "", &attrs, &startOpts, "")

	startOpts = append(startOpts, trace.WithAttributes(attrs...))
	return AITracer.Start(ctx, name, startOpts...)
}

// RecordAIEvent records a discrete event within an AI span.
// Use for significant milestones or decisions.
func RecordAIEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent(name, trace.WithAttributes(attrs...), trace.WithTimestamp(time.Now()))
}

// SetModelSelection records model selection information on a span.
// Use this to track which model was selected and why.
func SetModelSelection(span trace.Span, selectedModel, selectedSystem, reason string, alternatives []string) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("model.selected", selectedModel),
		attribute.String("model.system", selectedSystem),
		attribute.String("model.selection_reason", reason),
		attribute.StringSlice("model.alternatives", alternatives),
	)
}

// SetDecision records an AI decision on a span.
// Use this for mention resolution, entity matching, etc.
func SetDecision(span trace.Span, decisionType string, confidence float64, reasoning string) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("decision.type", decisionType),
		attribute.Float64("decision.confidence", confidence),
		attribute.String("decision.reasoning", reasoning),
	)
}
