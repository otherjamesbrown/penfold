// Package observability provides observability utilities for the Temporal worker.
package observability

import (
	"context"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "penfold.pipeline"
)

// PipelineSpan represents a parent span for a content processing pipeline.
type PipelineSpan struct {
	ctx  context.Context
	span trace.Span
}

// PipelineOptions holds options for starting a pipeline span.
type PipelineOptions struct {
	// PipelineName is the name of the pipeline (e.g., "email", "content").
	PipelineName string

	// ContentID is the identifier of the content being processed.
	ContentID string

	// ContentType is the type of content (e.g., "email", "document").
	ContentType string

	// TenantID is the tenant identifier.
	TenantID string

	// WorkflowID is the Temporal workflow ID for correlation.
	WorkflowID string

	// Additional attributes to add to the span.
	Attributes []attribute.KeyValue
}

// StartPipeline creates a new pipeline span that can serve as a parent for all
// activities and AI operations in a content processing pipeline.
//
// Example:
//
//	pipelineSpan := observability.StartPipeline(ctx, observability.PipelineOptions{
//	    PipelineName: "email",
//	    ContentID:    fmt.Sprintf("%d", sourceID),
//	    ContentType:  "email",
//	    TenantID:     tenantID,
//	    WorkflowID:   workflowID,
//	})
//	defer pipelineSpan.End()
//	ctx = pipelineSpan.Context()
func StartPipeline(ctx context.Context, opts PipelineOptions) *PipelineSpan {
	tracer := otel.Tracer(tracerName)

	attrs := []attribute.KeyValue{
		attribute.String("penfold.pipeline", "pipeline."+opts.PipelineName),
		attribute.String("langfuse.observation.type", "span"),
	}

	if opts.ContentID != "" {
		attrs = append(attrs, attribute.String("penfold.content_id", opts.ContentID))
	}
	if opts.ContentType != "" {
		attrs = append(attrs, attribute.String("penfold.content_type", opts.ContentType))
	}
	if opts.TenantID != "" {
		attrs = append(attrs, attribute.String("penfold.tenant_id", opts.TenantID))
	}
	if opts.WorkflowID != "" {
		attrs = append(attrs,
			attribute.String("penfold.pipeline_id", opts.WorkflowID),
			attribute.String("langfuse.session.id", opts.WorkflowID),
		)
	}
	attrs = append(attrs, opts.Attributes...)

	ctx, span := tracer.Start(ctx, "pipeline."+opts.PipelineName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)

	return &PipelineSpan{ctx: ctx, span: span}
}

// Context returns the context with the pipeline span attached.
// Use this context for child operations to link them to the pipeline.
func (p *PipelineSpan) Context() context.Context {
	return p.ctx
}

// Span returns the underlying trace span.
func (p *PipelineSpan) Span() trace.Span {
	return p.span
}

// End completes the pipeline span.
func (p *PipelineSpan) End() {
	if p.span != nil {
		p.span.End()
	}
}

// SetError records an error on the pipeline span.
func (p *PipelineSpan) SetError(err error) {
	if p.span != nil && err != nil {
		p.span.RecordError(err)
	}
}

// AddAttribute adds an attribute to the pipeline span.
func (p *PipelineSpan) AddAttribute(key, value string) {
	if p.span != nil {
		p.span.SetAttributes(attribute.String(key, value))
	}
}

// RecordStage records a stage completion event on the pipeline.
func (p *PipelineSpan) RecordStage(stageName string, attrs ...attribute.KeyValue) {
	if p.span != nil {
		allAttrs := append([]attribute.KeyValue{
			attribute.String("stage.name", stageName),
		}, attrs...)
		p.span.AddEvent("pipeline.stage.completed", trace.WithAttributes(allAttrs...))
	}
}

// EmailPipelineOptions creates PipelineOptions for email processing.
func EmailPipelineOptions(tenantID string, sourceID int64, workflowID string) PipelineOptions {
	return PipelineOptions{
		PipelineName: "email",
		ContentID:    formatInt64(sourceID),
		ContentType:  "email",
		TenantID:     tenantID,
		WorkflowID:   workflowID,
	}
}

// ContentPipelineOptions creates PipelineOptions for content ingestion.
func ContentPipelineOptions(tenantID string, contentID int64, contentType, workflowID string) PipelineOptions {
	return PipelineOptions{
		PipelineName: "content",
		ContentID:    formatInt64(contentID),
		ContentType:  contentType,
		TenantID:     tenantID,
		WorkflowID:   workflowID,
	}
}

// AnalysisPipelineOptions creates PipelineOptions for content analysis.
func AnalysisPipelineOptions(tenantID string, contentID int64, workflowID string) PipelineOptions {
	return PipelineOptions{
		PipelineName: "analysis",
		ContentID:    formatInt64(contentID),
		ContentType:  "analysis",
		TenantID:     tenantID,
		WorkflowID:   workflowID,
	}
}

// formatInt64 converts an int64 to string.
func formatInt64(n int64) string {
	return strconv.FormatInt(n, 10)
}
