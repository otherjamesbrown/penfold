// Package pipeline provides the enrichment pipeline orchestrator.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/processors"
	"github.com/rs/zerolog"
)

// Pipeline orchestrates the content enrichment process.
type Pipeline struct {
	registry   processors.ProcessorRegistry
	config     *processors.Config
	repository *enrichment.Repository
	logger     zerolog.Logger
}

// Option configures the pipeline.
type Option func(*Pipeline)

// WithConfig sets a custom processor configuration.
func WithConfig(cfg *processors.Config) Option {
	return func(p *Pipeline) {
		p.config = cfg
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger zerolog.Logger) Option {
	return func(p *Pipeline) {
		p.logger = logger
	}
}

// New creates a new enrichment pipeline.
func New(registry processors.ProcessorRegistry, repo *enrichment.Repository, opts ...Option) *Pipeline {
	p := &Pipeline{
		registry:   registry,
		config:     processors.DefaultConfig(),
		repository: repo,
		logger:     zerolog.Nop(),
	}

	for _, opt := range opts {
		opt(p)
	}

	p.logger = p.logger.With().Str("component", "enrichment_pipeline").Logger()
	return p
}

// Process runs the enrichment pipeline for a source.
func (p *Pipeline) Process(ctx context.Context, source *processors.Source) (*enrichment.Enrichment, error) {
	p.logger.Info().
		Int64("source_id", source.ID).
		Str("tenant_id", source.TenantID).
		Msg("Starting enrichment pipeline")

	// Create initial enrichment record
	e := &enrichment.Enrichment{
		SourceID:      source.ID,
		TenantID:      source.TenantID,
		Status:        enrichment.StatusPending,
		ExtractedData: make(map[string]interface{}),
	}

	// Stage 1: Classification
	if err := p.runClassification(ctx, source, e); err != nil {
		return nil, p.handleError(ctx, e, "classification", err)
	}

	// Create enrichment record in database
	if err := p.repository.Create(ctx, e); err != nil {
		return nil, fmt.Errorf("failed to create enrichment record: %w", err)
	}

	// Stage 2: Common Enrichment
	if err := p.runCommonEnrichment(ctx, source, e); err != nil {
		return nil, p.handleError(ctx, e, "common_enrichment", err)
	}

	// Stage 3: Type-Specific Extraction
	if err := p.runTypeSpecificExtraction(ctx, source, e); err != nil {
		return nil, p.handleError(ctx, e, "type_specific", err)
	}

	// Stage 4 & 5: AI Routing and Processing
	if err := p.runAIProcessing(ctx, source, e); err != nil {
		return nil, p.handleError(ctx, e, "ai_processing", err)
	}

	// Mark completed
	e.Status = enrichment.StatusCompleted
	now := time.Now()
	e.CompletedAt = &now
	if err := p.repository.Update(ctx, e); err != nil {
		p.logger.Error().Err(err).Int64("id", e.ID).Msg("Failed to update enrichment as completed")
	}

	p.logger.Info().
		Int64("id", e.ID).
		Int64("source_id", source.ID).
		Str("content_type", string(e.Classification.ContentType)).
		Str("subtype", string(e.Classification.Subtype)).
		Bool("ai_processed", e.AIProcessed).
		Msg("Enrichment pipeline completed")

	return e, nil
}

// runClassification executes Stage 1: Classification.
func (p *Pipeline) runClassification(ctx context.Context, source *processors.Source, e *enrichment.Enrichment) error {
	e.Status = enrichment.StatusClassifying
	e.CurrentStage = "classification"

	classifiers := p.registry.GetByStage(processors.StageClassification)
	if len(classifiers) == 0 {
		return fmt.Errorf("no classification processor registered")
	}

	// Use the first (and typically only) classifier
	classifier, ok := classifiers[0].(processors.ClassificationProcessor)
	if !ok {
		return fmt.Errorf("classification processor does not implement ClassificationProcessor interface")
	}

	startTime := time.Now()
	classification, err := classifier.Classify(ctx, source)
	duration := time.Since(startTime)

	// Record stage result
	p.recordStage(ctx, e, "classification", classifier.Name(), err, duration, nil, classification)

	if err != nil {
		return fmt.Errorf("classification failed: %w", err)
	}

	e.Classification = *classification

	p.logger.Debug().
		Int64("source_id", source.ID).
		Str("content_type", string(classification.ContentType)).
		Str("subtype", string(classification.Subtype)).
		Str("profile", string(classification.Profile)).
		Dur("duration", duration).
		Msg("Classification completed")

	return nil
}

// runCommonEnrichment executes Stage 2: Common Enrichment.
func (p *Pipeline) runCommonEnrichment(ctx context.Context, source *processors.Source, e *enrichment.Enrichment) error {
	e.Status = enrichment.StatusEnriching
	e.CurrentStage = "common_enrichment"

	if err := p.repository.UpdateStatus(ctx, e.ID, e.Status, e.CurrentStage, ""); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to update status")
	}

	procs := p.registry.GetByStage(processors.StageCommonEnrichment)

	pctx := &processors.ProcessorContext{
		Source:     source,
		Enrichment: e,
		TenantID:   source.TenantID,
		Logger:     p.logger,
	}

	for _, proc := range procs {
		// Check if this processor should run
		if cep, ok := proc.(processors.CommonEnrichmentProcessor); ok {
			if !cep.CanProcess(&e.Classification) {
				p.logger.Debug().
					Str("processor", proc.Name()).
					Msg("Skipping processor - not applicable for classification")
				continue
			}
		}

		startTime := time.Now()
		err := proc.Process(ctx, pctx)
		duration := time.Since(startTime)

		p.recordStage(ctx, e, "common_enrichment", proc.Name(), err, duration, nil, nil)

		if err != nil {
			p.logger.Warn().
				Err(err).
				Str("processor", proc.Name()).
				Msg("Common enrichment processor failed, continuing")
			// Don't fail the whole pipeline for common enrichment errors
			continue
		}

		p.logger.Debug().
			Str("processor", proc.Name()).
			Dur("duration", duration).
			Msg("Common enrichment processor completed")
	}

	// Update the enrichment record with results
	if err := p.repository.Update(ctx, e); err != nil {
		return fmt.Errorf("failed to update enrichment: %w", err)
	}

	return nil
}

// runTypeSpecificExtraction executes Stage 3: Type-Specific Extraction.
func (p *Pipeline) runTypeSpecificExtraction(ctx context.Context, source *processors.Source, e *enrichment.Enrichment) error {
	e.Status = enrichment.StatusExtracting
	e.CurrentStage = "type_specific"

	if err := p.repository.UpdateStatus(ctx, e.ID, e.Status, e.CurrentStage, ""); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to update status")
	}

	// Find the processor for this subtype
	proc, ok := p.registry.GetTypeSpecificProcessor(e.Classification.Subtype)
	if !ok {
		p.logger.Debug().
			Str("subtype", string(e.Classification.Subtype)).
			Msg("No type-specific processor for subtype")
		return nil
	}

	pctx := &processors.ProcessorContext{
		Source:     source,
		Enrichment: e,
		TenantID:   source.TenantID,
		Logger:     p.logger,
	}

	startTime := time.Now()
	err := proc.Extract(ctx, pctx)
	duration := time.Since(startTime)

	p.recordStage(ctx, e, "type_specific", proc.Name(), err, duration, nil, e.ExtractedData)

	if err != nil {
		p.logger.Warn().
			Err(err).
			Str("processor", proc.Name()).
			Str("subtype", string(e.Classification.Subtype)).
			Msg("Type-specific extraction failed")
		// Continue - extraction failure shouldn't stop the pipeline
	} else {
		p.logger.Debug().
			Str("processor", proc.Name()).
			Str("subtype", string(e.Classification.Subtype)).
			Dur("duration", duration).
			Msg("Type-specific extraction completed")
	}

	// Update the enrichment record
	if err := p.repository.Update(ctx, e); err != nil {
		return fmt.Errorf("failed to update enrichment: %w", err)
	}

	return nil
}

// runAIProcessing executes Stage 4 (AI Routing) and Stage 5 (AI Processing).
func (p *Pipeline) runAIProcessing(ctx context.Context, source *processors.Source, e *enrichment.Enrichment) error {
	// Stage 4: AI Routing - decide if we should process
	if p.config.ShouldSkipAI(e.Classification.Profile) {
		e.AIProcessed = false
		e.AISkipReason = fmt.Sprintf("profile:%s - %s",
			e.Classification.Profile,
			p.config.GetAISkipReason(e.Classification.Profile))

		p.logger.Debug().
			Str("profile", string(e.Classification.Profile)).
			Str("reason", e.AISkipReason).
			Msg("Skipping AI processing")

		return nil
	}

	// Stage 5: AI Processing
	e.Status = enrichment.StatusAIProcessing
	e.CurrentStage = "ai_processing"

	if err := p.repository.UpdateStatus(ctx, e.ID, e.Status, e.CurrentStage, ""); err != nil {
		p.logger.Warn().Err(err).Msg("Failed to update status")
	}

	aiProcessors := p.registry.GetByStage(processors.StageAIProcessing)

	pctx := &processors.ProcessorContext{
		Source:     source,
		Enrichment: e,
		TenantID:   source.TenantID,
		Logger:     p.logger,
	}

	for _, proc := range aiProcessors {
		// Check if this AI processor should run
		if aip, ok := proc.(processors.AIProcessor); ok {
			if !aip.ShouldProcess(e) {
				continue
			}
		}

		startTime := time.Now()
		err := proc.Process(ctx, pctx)
		duration := time.Since(startTime)

		p.recordStage(ctx, e, "ai_processing", proc.Name(), err, duration, nil, nil)

		if err != nil {
			p.logger.Error().
				Err(err).
				Str("processor", proc.Name()).
				Msg("AI processor failed")
			return fmt.Errorf("AI processing failed: %w", err)
		}

		p.logger.Debug().
			Str("processor", proc.Name()).
			Dur("duration", duration).
			Msg("AI processor completed")
	}

	now := time.Now()
	e.AIProcessed = true
	e.AIProcessedAt = &now

	return nil
}

// handleError handles pipeline errors and updates the enrichment record.
func (p *Pipeline) handleError(ctx context.Context, e *enrichment.Enrichment, stage string, err error) error {
	e.Status = enrichment.StatusFailed
	e.ErrorMessage = err.Error()

	p.logger.Error().
		Err(err).
		Int64("source_id", e.SourceID).
		Str("stage", stage).
		Msg("Pipeline failed")

	if e.ID > 0 {
		if updateErr := p.repository.MarkFailed(ctx, e.ID, err.Error()); updateErr != nil {
			p.logger.Error().Err(updateErr).Msg("Failed to update enrichment as failed")
		}
	}

	return fmt.Errorf("pipeline failed at %s: %w", stage, err)
}

// recordStage records a stage result for observability.
func (p *Pipeline) recordStage(ctx context.Context, e *enrichment.Enrichment, stageName, processorName string, err error, duration time.Duration, input, output interface{}) {
	if e.ID == 0 {
		// Enrichment not yet persisted, skip recording
		return
	}

	status := "completed"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}

	var inputData, outputData json.RawMessage
	if input != nil {
		if data, err := json.Marshal(input); err == nil {
			inputData = data
		}
	}
	if output != nil {
		if data, err := json.Marshal(output); err == nil {
			outputData = data
		}
	}

	now := time.Now()
	startedAt := now.Add(-duration)

	stage := &enrichment.StageResult{
		EnrichmentID:  e.ID,
		StageName:     stageName,
		ProcessorName: processorName,
		Status:        status,
		InputData:     inputData,
		OutputData:    outputData,
		ErrorMessage:  errMsg,
		StartedAt:     &startedAt,
		CompletedAt:   &now,
		DurationMs:    int(duration.Milliseconds()),
	}

	if err := p.repository.RecordStage(ctx, stage); err != nil {
		p.logger.Warn().Err(err).Str("stage", stageName).Msg("Failed to record stage")
	}
}

// ProcessBatch processes multiple sources in sequence.
func (p *Pipeline) ProcessBatch(ctx context.Context, sources []*processors.Source) ([]*enrichment.Enrichment, error) {
	results := make([]*enrichment.Enrichment, 0, len(sources))
	var lastErr error

	for _, source := range sources {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := p.Process(ctx, source)
		if err != nil {
			p.logger.Error().
				Err(err).
				Int64("source_id", source.ID).
				Msg("Failed to process source")
			lastErr = err
			continue
		}

		results = append(results, result)
	}

	return results, lastErr
}
