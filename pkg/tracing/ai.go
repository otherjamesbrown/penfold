// Package tracing provides AI-specific tracing helpers for Langfuse integration.
package tracing

import (
	"context"
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
	AttrLangfuseTag             = "langfuse.tag"

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

// LLMCallOptions holds options for starting an LLM call span.
type LLMCallOptions struct {
	// PipelineTraceID is an optional hex-encoded trace ID for grouping related operations.
	// When set, this span will be a child of the specified trace (not a root span).
	PipelineTraceID string

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
		// Add Langfuse tag for content grouping
		attrs = append(attrs, attribute.String(AttrLangfuseTag, opts.ContentID))
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

	// If PipelineTraceID is set, create span as child of that trace
	// Otherwise, create a root span (backward compatibility)
	var startOpts []trace.SpanStartOption
	if opts.PipelineTraceID != "" {
		// Parse hex trace ID and create remote span context
		traceID, err := trace.TraceIDFromHex(opts.PipelineTraceID)
		if err == nil {
			spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
			ctx = trace.ContextWithRemoteSpanContext(ctx, spanCtx)
			// Don't use WithNewRoot() - span will be child of remote context
		}
		// If parse error, fall through to WithNewRoot() behavior
	} else {
		// No PipelineTraceID - create root span (backward compatibility)
		startOpts = append(startOpts, trace.WithNewRoot())
	}

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
		// Add Langfuse tag for content grouping
		attrs = append(attrs, attribute.String(AttrLangfuseTag, opts.ContentID))
	}
	if opts.BatchSize > 0 {
		attrs = append(attrs, attribute.Int("batch_size", opts.BatchSize))
	}

	// If PipelineTraceID is set, create span as child of that trace
	// Otherwise, create a root span (backward compatibility)
	var startOpts []trace.SpanStartOption
	if opts.PipelineTraceID != "" {
		// Parse hex trace ID and create remote span context
		traceID, err := trace.TraceIDFromHex(opts.PipelineTraceID)
		if err == nil {
			spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
			ctx = trace.ContextWithRemoteSpanContext(ctx, spanCtx)
			// Don't use WithNewRoot() - span will be child of remote context
		}
		// If parse error, fall through to WithNewRoot() behavior
	} else {
		// No PipelineTraceID - create root span (backward compatibility)
		startOpts = append(startOpts, trace.WithNewRoot())
	}

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

	if result.Error != nil {
		SetError(span, result.Error)
	}
}

// AIProcessingOptions holds options for starting a general AI processing span.
type AIProcessingOptions struct {
	// PipelineTraceID is an optional hex-encoded trace ID for grouping related operations.
	// When set, this span will be a child of the specified trace (not a root span).
	PipelineTraceID string

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
		// Add Langfuse tag for content grouping
		attrs = append(attrs, attribute.String(AttrLangfuseTag, opts.ContentID))
	}
	if opts.ContentType != "" {
		attrs = append(attrs, attribute.String(AttrPenfoldContentType, opts.ContentType))
	}

	// If PipelineTraceID is set, create span as child of that trace
	// Otherwise, create a root span (backward compatibility)
	var startOpts []trace.SpanStartOption
	if opts.PipelineTraceID != "" {
		// Parse hex trace ID and create remote span context
		traceID, err := trace.TraceIDFromHex(opts.PipelineTraceID)
		if err == nil {
			spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
			ctx = trace.ContextWithRemoteSpanContext(ctx, spanCtx)
			// Don't use WithNewRoot() - span will be child of remote context
		}
		// If parse error, fall through to WithNewRoot() behavior
	} else {
		// No PipelineTraceID - create root span (backward compatibility)
		startOpts = append(startOpts, trace.WithNewRoot())
	}

	startOpts = append(startOpts, trace.WithAttributes(attrs...))
	return AITracer.Start(ctx, name, startOpts...)
}

// StartPipeline starts a parent span for an AI processing pipeline.
// Use this to group multiple AI operations under a single trace.
//
// The contentID should be in the standard format: <type:2>-<base62:8> (11 chars),
// e.g., "em-abc12XYZ" for email. Use pkg/contentid.New() to generate IDs.
//
// The pipelineTraceID is an optional hex-encoded trace ID. If provided, this pipeline
// span will continue that trace instead of creating a new root. If empty, a new root
// trace will be created.
//
// Example:
//
//	contentID := contentid.New(contentid.TypeEmail) // e.g., "em-abc12XYZ"
//	ctx, span := tracing.StartPipeline(ctx, "email-enrichment", contentID, "email", "")
//	defer span.End()
//	// ... perform multiple AI operations ...
func StartPipeline(ctx context.Context, name, contentID, contentType, pipelineTraceID string) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String(AttrLangfuseObservationType, ObservationTypeSpan),
		attribute.String(AttrPenfoldContentID, contentID),
		attribute.String(AttrPenfoldContentType, contentType),
	}

	// Add Langfuse tag for content grouping
	if contentID != "" {
		attrs = append(attrs, attribute.String(AttrLangfuseTag, contentID))
	}

	// If PipelineTraceID is set, create span as child of that trace
	// Otherwise, create a root span
	var startOpts []trace.SpanStartOption
	if pipelineTraceID != "" {
		// Parse hex trace ID and create remote span context
		traceID, err := trace.TraceIDFromHex(pipelineTraceID)
		if err == nil {
			spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
			ctx = trace.ContextWithRemoteSpanContext(ctx, spanCtx)
			// Don't use WithNewRoot() - span will be child of remote context
		}
		// If parse error, fall through to WithNewRoot() behavior
	} else {
		// No PipelineTraceID - create root span
		startOpts = append(startOpts, trace.WithNewRoot())
	}

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
