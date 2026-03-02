// Package audit provides resolution trace recording and auditing capabilities.
package audit

import (
	"context"
	"sync"

	"github.com/otherjamesbrown/penfold/pkg/mentions/resolver"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracingTracer wraps a Tracer and emits OpenTelemetry spans alongside DB writes.
// This provides dual-write capability: PostgreSQL for durable audit + Langfuse for observability.
type TracingTracer struct {
	delegate   resolver.Tracer
	ctx        context.Context
	mu         sync.RWMutex
	traceSpans map[string]trace.Span       // traceID -> parent span
	stageSpans map[int64]trace.Span        // stageID -> stage span
	stageCtxs  map[int64]context.Context   // stageID -> context (for child spans)
}

// NewTracingTracer creates a new tracing-enabled tracer.
// ctx should be the parent context with existing OTEL trace context.
func NewTracingTracer(ctx context.Context, delegate resolver.Tracer) *TracingTracer {
	return &TracingTracer{
		delegate:   delegate,
		ctx:        ctx,
		traceSpans: make(map[string]trace.Span),
		stageSpans: make(map[int64]trace.Span),
		stageCtxs:  make(map[int64]context.Context),
	}
}

// SetContext updates the parent context for new traces.
// Call this when starting a new resolution to link OTEL spans properly.
func (t *TracingTracer) SetContext(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ctx = ctx
}

// StartTrace implements resolver.Tracer.
func (t *TracingTracer) StartTrace(
	tenantID string,
	contentID int64,
	contentType, contentSummary string,
	model string,
	level resolver.TraceLevel,
	config map[string]interface{},
) (string, error) {
	// Delegate to underlying tracer first
	traceID, err := t.delegate.StartTrace(tenantID, contentID, contentType, contentSummary, model, level, config)
	if err != nil {
		return "", err
	}

	// Create OTEL span for the trace
	t.mu.Lock()
	defer t.mu.Unlock()

	ctx, span := tracing.StartSpan(t.ctx, "mentions.audit.trace")
	tracing.SetAttributes(span,
		tracing.Attr("mentions.trace_id", traceID),
		tracing.Attr("mentions.tenant_id", tenantID),
		tracing.AttrInt64("mentions.content_id", contentID),
		tracing.Attr("mentions.content_type", contentType),
		tracing.Attr("mentions.model", model),
		tracing.Attr("mentions.trace_level", string(level)),
	)
	if contentSummary != "" {
		tracing.SetAttributes(span, tracing.Attr("mentions.content_summary", truncate(contentSummary, 100)))
	}

	t.traceSpans[traceID] = span
	// Update ctx for subsequent spans
	t.ctx = ctx

	return traceID, nil
}

// RecordStageStart implements resolver.Tracer.
func (t *TracingTracer) RecordStageStart(
	traceID string,
	stageNum int,
	stageName resolver.StageName,
	inputSummary string,
	inputData interface{},
) (int64, error) {
	// Delegate first
	stageID, err := t.delegate.RecordStageStart(traceID, stageNum, stageName, inputSummary, inputData)
	if err != nil {
		return 0, err
	}

	// Create OTEL span for the stage
	t.mu.Lock()
	defer t.mu.Unlock()

	// Use trace span context as parent if available
	parentCtx := t.ctx
	if parentSpan, ok := t.traceSpans[traceID]; ok {
		parentCtx = trace.ContextWithSpan(t.ctx, parentSpan)
	}

	ctx, span := tracing.StartSpan(parentCtx, "mentions.stage."+string(stageName))
	tracing.SetAttributes(span,
		tracing.Attr("mentions.trace_id", traceID),
		tracing.AttrInt64("mentions.stage_id", stageID),
		tracing.AttrInt("mentions.stage_num", stageNum),
		tracing.Attr("mentions.stage_name", string(stageName)),
	)
	if inputSummary != "" {
		tracing.SetAttributes(span, tracing.Attr("mentions.input_summary", truncate(inputSummary, 200)))
	}

	t.stageSpans[stageID] = span
	t.stageCtxs[stageID] = ctx

	return stageID, nil
}

// RecordStageComplete implements resolver.Tracer.
func (t *TracingTracer) RecordStageComplete(stageID int64, outputSummary string, outputData interface{}) error {
	// Delegate first
	if err := t.delegate.RecordStageComplete(stageID, outputSummary, outputData); err != nil {
		return err
	}

	// End the OTEL span
	t.mu.Lock()
	defer t.mu.Unlock()

	if span, ok := t.stageSpans[stageID]; ok {
		if outputSummary != "" {
			tracing.SetAttributes(span, tracing.Attr("mentions.output_summary", truncate(outputSummary, 200)))
		}
		tracing.SetAttributes(span, tracing.Attr("mentions.stage_status", "completed"))
		span.End()
		delete(t.stageSpans, stageID)
		delete(t.stageCtxs, stageID)
	}

	return nil
}

// RecordStageFailed implements resolver.Tracer.
func (t *TracingTracer) RecordStageFailed(stageID int64, errorMsg string) error {
	// Delegate first
	if err := t.delegate.RecordStageFailed(stageID, errorMsg); err != nil {
		return err
	}

	// End the OTEL span with error
	t.mu.Lock()
	defer t.mu.Unlock()

	if span, ok := t.stageSpans[stageID]; ok {
		tracing.SetAttributes(span,
			tracing.Attr("mentions.stage_status", "failed"),
			tracing.Attr("mentions.error", errorMsg),
		)
		span.End()
		delete(t.stageSpans, stageID)
		delete(t.stageCtxs, stageID)
	}

	return nil
}

// RecordStageSkipped implements resolver.Tracer.
func (t *TracingTracer) RecordStageSkipped(stageID int64, reason string) error {
	// Delegate first
	if err := t.delegate.RecordStageSkipped(stageID, reason); err != nil {
		return err
	}

	// End the OTEL span as skipped
	t.mu.Lock()
	defer t.mu.Unlock()

	if span, ok := t.stageSpans[stageID]; ok {
		tracing.SetAttributes(span,
			tracing.Attr("mentions.stage_status", "skipped"),
			tracing.Attr("mentions.skip_reason", reason),
		)
		span.End()
		delete(t.stageSpans, stageID)
		delete(t.stageCtxs, stageID)
	}

	return nil
}

// RecordLLMCall implements resolver.Tracer.
func (t *TracingTracer) RecordLLMCall(
	traceID string,
	stageID int64,
	model, promptTemplate, promptText string,
	promptTokens int,
	responseText string,
	responseTokens int,
	parsedOutput interface{},
	parseErrors []string,
	latencyMs, attemptNumber int,
	isFallback bool,
	fallbackReason string,
) error {
	// Delegate first
	if err := t.delegate.RecordLLMCall(traceID, stageID, model, promptTemplate, promptText,
		promptTokens, responseText, responseTokens, parsedOutput, parseErrors,
		latencyMs, attemptNumber, isFallback, fallbackReason); err != nil {
		return err
	}

	// Create OTEL span for LLM call
	t.mu.RLock()
	parentCtx := t.ctx
	if stageID > 0 {
		if stageCtx, ok := t.stageCtxs[stageID]; ok {
			parentCtx = stageCtx
		}
	} else if parentSpan, ok := t.traceSpans[traceID]; ok {
		parentCtx = trace.ContextWithSpan(t.ctx, parentSpan)
	}
	t.mu.RUnlock()

	// Create and immediately end the LLM call span (since it's already complete)
	_, span := tracing.StartLLMCall(parentCtx, "mentions.audit.llm_call", tracing.LLMCallOptions{
		Model:    model,
		System:   tracing.AISystemMLX, // Default, could be determined from model
		TaskType: "mention_resolution",
	})

	tracing.SetAttributes(span,
		tracing.Attr("mentions.trace_id", traceID),
		tracing.AttrInt("mentions.attempt_number", attemptNumber),
		tracing.AttrBool("mentions.is_fallback", isFallback),
	)
	if promptTemplate != "" {
		tracing.SetAttributes(span, tracing.Attr("mentions.prompt_template", truncate(promptTemplate, 50)))
	}
	if len(parseErrors) > 0 {
		tracing.SetAttributes(span, tracing.AttrInt("mentions.parse_errors", len(parseErrors)))
	}
	if isFallback && fallbackReason != "" {
		tracing.SetAttributes(span, tracing.Attr("mentions.fallback_reason", fallbackReason))
	}

	tracing.SetLLMResult(span, tracing.LLMResult{
		InputTokens:  promptTokens,
		OutputTokens: responseTokens,
		Model:        model,
		LatencyMs:    int64(latencyMs),
	})

	span.End()
	return nil
}

// RecordDecision implements resolver.Tracer.
func (t *TracingTracer) RecordDecision(
	traceID string,
	stageID int64,
	decisionType resolver.DecisionType,
	mentionID *int64,
	mentionText, chosenOption string,
	alternatives interface{},
	confidence float32,
	reasoning string,
	factors interface{},
) error {
	// Delegate first
	if err := t.delegate.RecordDecision(traceID, stageID, decisionType, mentionID, mentionText,
		chosenOption, alternatives, confidence, reasoning, factors); err != nil {
		return err
	}

	// Add decision as span event (not a full span since it's a point-in-time decision)
	t.mu.RLock()
	var span trace.Span
	if stageID > 0 {
		span = t.stageSpans[stageID]
	}
	if span == nil {
		span = t.traceSpans[traceID]
	}
	t.mu.RUnlock()

	if span != nil {
		// Add as event with decision details
		attrs := []attribute.KeyValue{
			attribute.String("decision.type", string(decisionType)),
			attribute.String("decision.mention_text", truncate(mentionText, 100)),
			attribute.String("decision.chosen_option", truncate(chosenOption, 100)),
			attribute.Float64("decision.confidence", float64(confidence)),
		}
		if reasoning != "" {
			attrs = append(attrs, attribute.String("decision.reasoning", truncate(reasoning, 200)))
		}
		if mentionID != nil {
			attrs = append(attrs, attribute.Int64("decision.mention_id", *mentionID))
		}

		tracing.AddEvent(span, "mention.decision", attrs...)
	}

	return nil
}

// CompleteTrace implements resolver.Tracer.
func (t *TracingTracer) CompleteTrace(traceID string, mentionsFound, autoResolved, queuedForReview, newEntitiesSuggested int) error {
	// Delegate first
	if err := t.delegate.CompleteTrace(traceID, mentionsFound, autoResolved, queuedForReview, newEntitiesSuggested); err != nil {
		return err
	}

	// End the trace span
	t.mu.Lock()
	defer t.mu.Unlock()

	if span, ok := t.traceSpans[traceID]; ok {
		tracing.SetAttributes(span,
			tracing.Attr("mentions.status", "completed"),
			tracing.AttrInt("mentions.found", mentionsFound),
			tracing.AttrInt("mentions.auto_resolved", autoResolved),
			tracing.AttrInt("mentions.queued_for_review", queuedForReview),
			tracing.AttrInt("mentions.new_entities_suggested", newEntitiesSuggested),
		)
		span.End()
		delete(t.traceSpans, traceID)
	}

	return nil
}

// FailTrace implements resolver.Tracer.
func (t *TracingTracer) FailTrace(traceID string, errorMsg string) error {
	// Delegate first
	if err := t.delegate.FailTrace(traceID, errorMsg); err != nil {
		return err
	}

	// End the trace span with error
	t.mu.Lock()
	defer t.mu.Unlock()

	if span, ok := t.traceSpans[traceID]; ok {
		tracing.SetAttributes(span,
			tracing.Attr("mentions.status", "failed"),
			tracing.Attr("mentions.error", errorMsg),
		)
		span.End()
		delete(t.traceSpans, traceID)
	}

	return nil
}

// GetTraceLevel returns the trace level from the delegate.
func (t *TracingTracer) GetTraceLevel() resolver.TraceLevel {
	if dl, ok := t.delegate.(interface{ GetTraceLevel() resolver.TraceLevel }); ok {
		return dl.GetTraceLevel()
	}
	return resolver.TraceLevelMinimal
}

// SetTraceLevel updates the trace level on the delegate.
func (t *TracingTracer) SetTraceLevel(level resolver.TraceLevel) {
	if dl, ok := t.delegate.(interface{ SetTraceLevel(resolver.TraceLevel) }); ok {
		dl.SetTraceLevel(level)
	}
}

// truncate limits a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Ensure TracingTracer implements resolver.Tracer at compile time.
var _ resolver.Tracer = (*TracingTracer)(nil)
