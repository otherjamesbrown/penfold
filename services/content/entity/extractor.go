package entity

import (
	"context"
	"fmt"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// EntityExtractor defines the interface for entity extraction implementations.
type EntityExtractor interface {
	// Extract extracts entities from the given text.
	Extract(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error)

	// Name returns the name of the extractor.
	Name() string
}

// ===== Hybrid Extractor =====

// HybridExtractor combines pattern-based and AI-based extraction.
type HybridExtractor struct {
	patternExtractor *PatternExtractor
	nerExtractor     *NERExtractor
	normalizer       *EntityNormalizer
	logger           logging.Logger
}

// HybridExtractorConfig configures the hybrid extractor.
type HybridExtractorConfig struct {
	// PatternPatterns are custom patterns to use (nil for defaults).
	PatternPatterns []*CompiledPattern

	// NERConfig configures the NER extractor.
	NERConfig *NERConfig

	// NormalizerConfig configures the normalizer.
	NormalizerConfig *NormalizerConfig
}

// NewHybridExtractor creates a new hybrid extractor.
func NewHybridExtractor(
	aiClient AIClient,
	logger logging.Logger,
	cfg *HybridExtractorConfig,
) *HybridExtractor {
	if cfg == nil {
		cfg = &HybridExtractorConfig{}
	}

	var nerExtractor *NERExtractor
	if aiClient != nil {
		nerExtractor = NewNERExtractor(aiClient, cfg.NERConfig)
	}

	return &HybridExtractor{
		patternExtractor: NewPatternExtractor(cfg.PatternPatterns),
		nerExtractor:     nerExtractor,
		normalizer:       NewEntityNormalizer(cfg.NormalizerConfig),
		logger:           logger.With(logging.F("component", "hybrid_entity_extractor")),
	}
}

// Extract extracts entities using both patterns and NER.
func (e *HybridExtractor) Extract(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error) {
	startTime := time.Now()

	e.logger.Debug("Starting hybrid entity extraction",
		logging.F("text_length", len(text)),
		logging.F("use_patterns", opts.UsePatterns),
		logging.F("use_ner", opts.UseNER),
	)

	if text == "" {
		return &ExtractionResult{
			Entities:         []Entity{},
			ProcessingTimeMs: 0,
		}, nil
	}

	result := &ExtractionResult{
		Entities: []Entity{},
		SourceText: truncateText(text, 500),
	}

	var patternEntities []Entity
	var nerEntities []Entity
	var errs []error

	// Extract with patterns
	if opts.UsePatterns {
		patternEntities = e.patternExtractor.Extract(text, opts)
		e.logger.Debug("Pattern extraction complete",
			logging.F("entities_found", len(patternEntities)),
		)
	}

	// Extract with NER
	if opts.UseNER && e.nerExtractor != nil {
		var err error
		nerEntities, err = e.nerExtractor.Extract(ctx, text, opts)
		if err != nil {
			e.logger.Warn("NER extraction failed",
				logging.F("error", err.Error()),
			)
			errs = append(errs, err)
		} else {
			e.logger.Debug("NER extraction complete",
				logging.F("entities_found", len(nerEntities)),
			)
			result.ModelUsed = e.nerExtractor.model
		}
	}

	// Merge entities from both sources
	allEntities := append(patternEntities, nerEntities...)
	result.TotalFound = len(allEntities)

	// Mark source for hybrid entities (those found by both)
	allEntities = markHybridEntities(patternEntities, nerEntities)

	// Normalize entities
	if opts.NormalizeEntities {
		e.normalizer.NormalizeEntities(allEntities)
	}

	// Deduplicate
	if opts.DeduplicateEntities {
		var duplicatesRemoved int
		allEntities, duplicatesRemoved = deduplicateAndCountEntities(allEntities)
		result.DuplicatesRemoved = duplicatesRemoved
	}

	// Filter by confidence
	var filtered []Entity
	for _, entity := range allEntities {
		if entity.Confidence >= opts.MinConfidence {
			filtered = append(filtered, entity)
		}
	}
	result.FilteredCount = len(allEntities) - len(filtered)
	allEntities = filtered

	// Limit results
	if opts.MaxEntities > 0 && len(allEntities) > opts.MaxEntities {
		allEntities = allEntities[:opts.MaxEntities]
	}

	result.Entities = allEntities
	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()
	result.Errors = errs

	e.logger.Info("Hybrid extraction completed",
		logging.F("entities_extracted", len(result.Entities)),
		logging.F("total_found", result.TotalFound),
		logging.F("filtered", result.FilteredCount),
		logging.F("duplicates_removed", result.DuplicatesRemoved),
		logging.F("processing_time_ms", result.ProcessingTimeMs),
	)

	return result, nil
}

// Name returns the extractor name.
func (e *HybridExtractor) Name() string {
	return "hybrid"
}

// markHybridEntities marks entities found by both extractors.
func markHybridEntities(patternEntities, nerEntities []Entity) []Entity {
	// Build a map of pattern entities by value+type
	patternMap := make(map[string]int) // value+type -> index
	for i, e := range patternEntities {
		key := string(e.Type) + ":" + e.Value
		patternMap[key] = i
	}

	// Check NER entities against pattern map
	for i, e := range nerEntities {
		key := string(e.Type) + ":" + e.Value
		if _, found := patternMap[key]; found {
			// Mark as hybrid - found by both
			nerEntities[i].Source = ExtractionSourceHybrid
			// Boost confidence for entities found by both
			nerEntities[i].Confidence = min(1.0, nerEntities[i].Confidence*1.1)
		}
	}

	// Combine all entities
	result := make([]Entity, 0, len(patternEntities)+len(nerEntities))
	result = append(result, patternEntities...)
	result = append(result, nerEntities...)

	return result
}

// deduplicateAndCountEntities deduplicates and returns the count of removed duplicates.
func deduplicateAndCountEntities(entities []Entity) ([]Entity, int) {
	seen := make(map[string]Entity)

	for _, e := range entities {
		key := string(e.Type) + ":" + normalizedKey(e)

		if existing, exists := seen[key]; exists {
			// Keep the one with higher confidence
			if e.Confidence > existing.Confidence {
				seen[key] = e
			}
		} else {
			seen[key] = e
		}
	}

	result := make([]Entity, 0, len(seen))
	for _, e := range seen {
		result = append(result, e)
	}

	return result, len(entities) - len(seen)
}

// normalizedKey returns a normalized key for deduplication.
func normalizedKey(e Entity) string {
	if e.NormalizedValue != "" {
		return e.NormalizedValue
	}
	return e.Value
}

// ===== Domain Extractor =====

// DomainExtractor extracts domain-specific entities.
type DomainExtractor struct {
	domain     string
	patterns   []*CompiledPattern
	entityMap  map[string]EntityType
	normalizer *EntityNormalizer
	logger     logging.Logger
}

// DomainConfig configures a domain extractor.
type DomainConfig struct {
	// Domain is the domain name (e.g., "finance", "legal", "medical").
	Domain string

	// Patterns are domain-specific patterns.
	Patterns []*CompiledPattern

	// EntityMap maps domain terms to entity types.
	EntityMap map[string]EntityType

	// NormalizerConfig configures normalization.
	NormalizerConfig *NormalizerConfig
}

// NewDomainExtractor creates a new domain-specific extractor.
func NewDomainExtractor(logger logging.Logger, cfg *DomainConfig) *DomainExtractor {
	if cfg == nil {
		cfg = &DomainConfig{Domain: "general"}
	}

	return &DomainExtractor{
		domain:     cfg.Domain,
		patterns:   cfg.Patterns,
		entityMap:  cfg.EntityMap,
		normalizer: NewEntityNormalizer(cfg.NormalizerConfig),
		logger:     logger.With(logging.F("component", "domain_entity_extractor"), logging.F("domain", cfg.Domain)),
	}
}

// Extract extracts domain-specific entities.
func (e *DomainExtractor) Extract(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error) {
	startTime := time.Now()

	if text == "" {
		return &ExtractionResult{
			Entities:         []Entity{},
			ProcessingTimeMs: 0,
		}, nil
	}

	result := &ExtractionResult{
		Entities:   []Entity{},
		SourceText: truncateText(text, 500),
	}

	// Extract using domain patterns
	patternExtractor := NewPatternExtractor(e.patterns)
	entities := patternExtractor.Extract(text, opts)

	// Mark as domain source
	for i := range entities {
		entities[i].Source = ExtractionSourceDomain
		entities[i].Metadata["domain"] = e.domain
	}

	// Normalize if enabled
	if opts.NormalizeEntities {
		e.normalizer.NormalizeEntities(entities)
	}

	// Deduplicate
	if opts.DeduplicateEntities {
		var duplicatesRemoved int
		entities, duplicatesRemoved = deduplicateAndCountEntities(entities)
		result.DuplicatesRemoved = duplicatesRemoved
	}

	result.TotalFound = len(entities)
	result.Entities = entities
	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()

	return result, nil
}

// Name returns the extractor name.
func (e *DomainExtractor) Name() string {
	return fmt.Sprintf("domain:%s", e.domain)
}

// ===== Composite Extractor =====

// CompositeExtractor runs multiple extractors and combines results.
type CompositeExtractor struct {
	extractors []EntityExtractor
	normalizer *EntityNormalizer
	logger     logging.Logger
}

// NewCompositeExtractor creates an extractor that combines multiple extractors.
func NewCompositeExtractor(logger logging.Logger, extractors ...EntityExtractor) *CompositeExtractor {
	return &CompositeExtractor{
		extractors: extractors,
		normalizer: NewEntityNormalizer(nil),
		logger:     logger.With(logging.F("component", "composite_entity_extractor")),
	}
}

// Extract runs all extractors and combines results.
func (e *CompositeExtractor) Extract(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error) {
	startTime := time.Now()

	if text == "" {
		return &ExtractionResult{
			Entities:         []Entity{},
			ProcessingTimeMs: 0,
		}, nil
	}

	result := &ExtractionResult{
		Entities:   []Entity{},
		SourceText: truncateText(text, 500),
	}

	var allEntities []Entity
	var errs []error

	for _, extractor := range e.extractors {
		extractorResult, err := extractor.Extract(ctx, text, opts)
		if err != nil {
			e.logger.Warn("Extractor failed",
				logging.F("extractor", extractor.Name()),
				logging.F("error", err.Error()),
			)
			errs = append(errs, fmt.Errorf("%s: %w", extractor.Name(), err))
			continue
		}

		allEntities = append(allEntities, extractorResult.Entities...)

		if extractorResult.ModelUsed != "" && result.ModelUsed == "" {
			result.ModelUsed = extractorResult.ModelUsed
		}
	}

	result.TotalFound = len(allEntities)

	// Deduplicate across all extractors
	if opts.DeduplicateEntities {
		var duplicatesRemoved int
		allEntities, duplicatesRemoved = deduplicateAndCountEntities(allEntities)
		result.DuplicatesRemoved = duplicatesRemoved
	}

	// Filter by confidence
	var filtered []Entity
	for _, entity := range allEntities {
		if entity.Confidence >= opts.MinConfidence {
			filtered = append(filtered, entity)
		}
	}
	result.FilteredCount = len(allEntities) - len(filtered)
	result.Entities = filtered

	result.ProcessingTimeMs = time.Since(startTime).Milliseconds()
	result.Errors = errs

	return result, nil
}

// Name returns the extractor name.
func (e *CompositeExtractor) Name() string {
	return "composite"
}

// AddExtractor adds an extractor to the composite.
func (e *CompositeExtractor) AddExtractor(extractor EntityExtractor) {
	e.extractors = append(e.extractors, extractor)
}

// ===== Helper Functions =====

// truncateText truncates text to a maximum length with ellipsis.
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// min returns the smaller of two float32 values.
func min(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// ===== Extraction Pipeline =====

// ExtractionPipeline provides a fluent interface for building extraction workflows.
type ExtractionPipeline struct {
	extractor   EntityExtractor
	normalizer  *EntityNormalizer
	postFilters []func([]Entity) []Entity
	logger      logging.Logger
}

// NewExtractionPipeline creates a new extraction pipeline.
func NewExtractionPipeline(extractor EntityExtractor, logger logging.Logger) *ExtractionPipeline {
	return &ExtractionPipeline{
		extractor:   extractor,
		normalizer:  NewEntityNormalizer(nil),
		postFilters: []func([]Entity) []Entity{},
		logger:      logger,
	}
}

// WithNormalizer sets a custom normalizer.
func (p *ExtractionPipeline) WithNormalizer(n *EntityNormalizer) *ExtractionPipeline {
	p.normalizer = n
	return p
}

// WithFilter adds a post-extraction filter.
func (p *ExtractionPipeline) WithFilter(filter func([]Entity) []Entity) *ExtractionPipeline {
	p.postFilters = append(p.postFilters, filter)
	return p
}

// Execute runs the extraction pipeline.
func (p *ExtractionPipeline) Execute(ctx context.Context, text string, opts ExtractionOptions) (*ExtractionResult, error) {
	// Run extraction
	result, err := p.extractor.Extract(ctx, text, opts)
	if err != nil {
		return nil, err
	}

	// Apply normalizer
	if p.normalizer != nil && opts.NormalizeEntities {
		p.normalizer.NormalizeEntities(result.Entities)
	}

	// Apply post-filters
	for _, filter := range p.postFilters {
		result.Entities = filter(result.Entities)
	}

	return result, nil
}

// ===== Common Filters =====

// FilterByTypes returns a filter that keeps only specified entity types.
func FilterByTypes(types ...EntityType) func([]Entity) []Entity {
	typeSet := make(map[EntityType]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	return func(entities []Entity) []Entity {
		var filtered []Entity
		for _, e := range entities {
			if typeSet[e.Type] {
				filtered = append(filtered, e)
			}
		}
		return filtered
	}
}

// FilterByMinConfidence returns a filter that keeps entities above a confidence threshold.
func FilterByMinConfidence(minConfidence float32) func([]Entity) []Entity {
	return func(entities []Entity) []Entity {
		var filtered []Entity
		for _, e := range entities {
			if e.Confidence >= minConfidence {
				filtered = append(filtered, e)
			}
		}
		return filtered
	}
}

// FilterBySource returns a filter that keeps entities from specified sources.
func FilterBySource(sources ...ExtractionSource) func([]Entity) []Entity {
	sourceSet := make(map[ExtractionSource]bool)
	for _, s := range sources {
		sourceSet[s] = true
	}

	return func(entities []Entity) []Entity {
		var filtered []Entity
		for _, e := range entities {
			if sourceSet[e.Source] {
				filtered = append(filtered, e)
			}
		}
		return filtered
	}
}

// Compile-time check that extractors implement EntityExtractor.
var (
	_ EntityExtractor = (*HybridExtractor)(nil)
	_ EntityExtractor = (*DomainExtractor)(nil)
	_ EntityExtractor = (*CompositeExtractor)(nil)
)
