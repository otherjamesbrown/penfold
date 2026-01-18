// Package queue provides priority calculation strategies for review items.
package queue

import (
	"math"
	"time"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/reviewv1"
)

// PriorityCalculator defines the interface for calculating item priority.
type PriorityCalculator interface {
	// Calculate returns a priority score for the given item.
	// Higher scores indicate higher priority (should be processed first).
	Calculate(item *QueueItem) float64

	// Name returns the name of this priority calculator for logging/debugging.
	Name() string
}

// TimePriority calculates priority based on age - older items get higher priority.
// This implements a FIFO-like ordering with priority adjustment.
type TimePriority struct {
	// BaseWeight is the initial weight applied to time-based priority.
	BaseWeight float64

	// DecayHalfLife is how long it takes for the time factor to halve.
	// Smaller values mean older items get higher priority faster.
	DecayHalfLife time.Duration
}

// NewTimePriority creates a TimePriority calculator with default settings.
func NewTimePriority() *TimePriority {
	return &TimePriority{
		BaseWeight:    1.0,
		DecayHalfLife: 24 * time.Hour, // Items double in priority every 24 hours
	}
}

// NewTimePriorityWithConfig creates a TimePriority calculator with custom settings.
func NewTimePriorityWithConfig(baseWeight float64, halfLife time.Duration) *TimePriority {
	if baseWeight <= 0 {
		baseWeight = 1.0
	}
	if halfLife <= 0 {
		halfLife = 24 * time.Hour
	}
	return &TimePriority{
		BaseWeight:    baseWeight,
		DecayHalfLife: halfLife,
	}
}

// Calculate returns priority based on how long the item has been in queue.
func (p *TimePriority) Calculate(item *QueueItem) float64 {
	if item == nil {
		return 0
	}

	age := time.Since(item.EnqueuedAt)
	if age < 0 {
		age = 0
	}

	// Use exponential growth: priority = BaseWeight * 2^(age/halfLife)
	// This means items double in priority every halfLife duration
	ageHalfLives := float64(age) / float64(p.DecayHalfLife)
	return p.BaseWeight * math.Pow(2, ageHalfLives)
}

// Name returns the calculator name.
func (p *TimePriority) Name() string {
	return "time_priority"
}

// ImportancePriority calculates priority based on item importance/urgency.
// This uses the original priority from the review item plus content-type weighting.
type ImportancePriority struct {
	// PriorityWeights maps priority levels to numeric weights.
	PriorityWeights map[reviewv1.Priority]float64

	// ContentTypeWeights maps content types to additional weight multipliers.
	ContentTypeWeights map[string]float64

	// CategoryWeights maps categories to additional weight multipliers.
	CategoryWeights map[string]float64
}

// NewImportancePriority creates an ImportancePriority calculator with default weights.
func NewImportancePriority() *ImportancePriority {
	return &ImportancePriority{
		PriorityWeights: map[reviewv1.Priority]float64{
			reviewv1.Priority_PRIORITY_UNSPECIFIED: 1.0,
			reviewv1.Priority_PRIORITY_LOW:         2.0,
			reviewv1.Priority_PRIORITY_MEDIUM:      4.0,
			reviewv1.Priority_PRIORITY_HIGH:        8.0,
			reviewv1.Priority_PRIORITY_URGENT:      16.0,
		},
		ContentTypeWeights: map[string]float64{
			"email":        1.0,
			"document":     0.8,
			"relationship": 1.2,
			"task":         1.5,
			"meeting":      1.3,
			"alert":        2.0,
		},
		CategoryWeights: map[string]float64{
			"communications": 1.0,
			"tasks":          1.2,
			"insights":       0.8,
			"relationships":  1.1,
			"alerts":         1.5,
		},
	}
}

// NewImportancePriorityWithWeights creates an ImportancePriority with custom weights.
func NewImportancePriorityWithWeights(
	priorityWeights map[reviewv1.Priority]float64,
	contentTypeWeights map[string]float64,
	categoryWeights map[string]float64,
) *ImportancePriority {
	p := NewImportancePriority()
	if priorityWeights != nil {
		p.PriorityWeights = priorityWeights
	}
	if contentTypeWeights != nil {
		p.ContentTypeWeights = contentTypeWeights
	}
	if categoryWeights != nil {
		p.CategoryWeights = categoryWeights
	}
	return p
}

// Calculate returns priority based on item importance.
func (p *ImportancePriority) Calculate(item *QueueItem) float64 {
	if item == nil {
		return 0
	}

	// Base priority from the item's original priority level
	basePriority := p.PriorityWeights[item.OriginalPriority]
	if basePriority == 0 {
		basePriority = 1.0
	}

	// Content type multiplier
	contentMultiplier := p.ContentTypeWeights[item.ContentType]
	if contentMultiplier == 0 {
		contentMultiplier = 1.0
	}

	// Category multiplier
	categoryMultiplier := p.CategoryWeights[item.Category]
	if categoryMultiplier == 0 {
		categoryMultiplier = 1.0
	}

	return basePriority * contentMultiplier * categoryMultiplier
}

// Name returns the calculator name.
func (p *ImportancePriority) Name() string {
	return "importance_priority"
}

// DeferralPenalty calculates priority adjustment based on deferral history.
// Items that have been deferred multiple times get lower priority.
type DeferralPenalty struct {
	// PenaltyPerDefer is the multiplicative penalty for each deferral (0-1).
	// A value of 0.8 means each deferral reduces priority by 20%.
	PenaltyPerDefer float64

	// MaxPenalty is the minimum multiplier (floor).
	MaxPenalty float64
}

// NewDeferralPenalty creates a DeferralPenalty calculator with default settings.
func NewDeferralPenalty() *DeferralPenalty {
	return &DeferralPenalty{
		PenaltyPerDefer: 0.8,  // 20% penalty per deferral
		MaxPenalty:      0.25, // Minimum 25% of original priority
	}
}

// NewDeferralPenaltyWithConfig creates a DeferralPenalty with custom settings.
func NewDeferralPenaltyWithConfig(penaltyPerDefer, maxPenalty float64) *DeferralPenalty {
	if penaltyPerDefer <= 0 || penaltyPerDefer > 1 {
		penaltyPerDefer = 0.8
	}
	if maxPenalty <= 0 || maxPenalty > 1 {
		maxPenalty = 0.25
	}
	return &DeferralPenalty{
		PenaltyPerDefer: penaltyPerDefer,
		MaxPenalty:      maxPenalty,
	}
}

// Calculate returns a multiplier based on deferral count.
func (p *DeferralPenalty) Calculate(item *QueueItem) float64 {
	if item == nil || item.DeferCount <= 0 {
		return 1.0
	}

	// Calculate penalty: PenaltyPerDefer^DeferCount
	penalty := math.Pow(p.PenaltyPerDefer, float64(item.DeferCount))

	// Ensure we don't go below the maximum penalty
	if penalty < p.MaxPenalty {
		return p.MaxPenalty
	}
	return penalty
}

// Name returns the calculator name.
func (p *DeferralPenalty) Name() string {
	return "deferral_penalty"
}

// MixedPriority combines multiple priority factors with configurable weights.
type MixedPriority struct {
	// TimeWeight is the weight for time-based priority (0-1).
	TimeWeight float64

	// ImportanceWeight is the weight for importance-based priority (0-1).
	ImportanceWeight float64

	// DeferralWeight is the weight for deferral penalty (0-1).
	DeferralWeight float64

	// Individual calculators
	timePriority       *TimePriority
	importancePriority *ImportancePriority
	deferralPenalty    *DeferralPenalty
}

// NewMixedPriority creates a MixedPriority calculator with the given weights.
// Weights should sum to 1.0 but will be normalized if they don't.
func NewMixedPriority(timeWeight, importanceWeight, deferralWeight float64) *MixedPriority {
	// Normalize weights
	total := timeWeight + importanceWeight + deferralWeight
	if total <= 0 {
		// Default to equal weights
		timeWeight = 0.33
		importanceWeight = 0.34
		deferralWeight = 0.33
		total = 1.0
	}

	return &MixedPriority{
		TimeWeight:         timeWeight / total,
		ImportanceWeight:   importanceWeight / total,
		DeferralWeight:     deferralWeight / total,
		timePriority:       NewTimePriority(),
		importancePriority: NewImportancePriority(),
		deferralPenalty:    NewDeferralPenalty(),
	}
}

// NewMixedPriorityWithCalculators creates a MixedPriority with custom calculators.
func NewMixedPriorityWithCalculators(
	timeWeight, importanceWeight, deferralWeight float64,
	timePriority *TimePriority,
	importancePriority *ImportancePriority,
	deferralPenalty *DeferralPenalty,
) *MixedPriority {
	m := NewMixedPriority(timeWeight, importanceWeight, deferralWeight)

	if timePriority != nil {
		m.timePriority = timePriority
	}
	if importancePriority != nil {
		m.importancePriority = importancePriority
	}
	if deferralPenalty != nil {
		m.deferralPenalty = deferralPenalty
	}

	return m
}

// Calculate combines time, importance, and deferral factors into a final priority.
func (p *MixedPriority) Calculate(item *QueueItem) float64 {
	if item == nil {
		return 0
	}

	// Calculate individual components
	timePriority := p.timePriority.Calculate(item)
	importancePriority := p.importancePriority.Calculate(item)
	deferralMultiplier := p.deferralPenalty.Calculate(item)

	// Weighted combination
	// Note: deferral is used as a multiplier rather than an additive component
	basePriority := (p.TimeWeight * timePriority) + (p.ImportanceWeight * importancePriority)

	// Apply deferral penalty if weight is set
	if p.DeferralWeight > 0 {
		// Blend in the deferral penalty
		effectiveMultiplier := (1.0 - p.DeferralWeight) + (p.DeferralWeight * deferralMultiplier)
		return basePriority * effectiveMultiplier
	}

	return basePriority
}

// Name returns the calculator name.
func (p *MixedPriority) Name() string {
	return "mixed_priority"
}

// SourcePriority calculates priority based on the source of the item.
// Some sources may be more urgent than others (e.g., real-time alerts vs batch imports).
type SourcePriority struct {
	// SourceWeights maps source names to priority weights.
	SourceWeights map[string]float64

	// DefaultWeight is used for unknown sources.
	DefaultWeight float64
}

// NewSourcePriority creates a SourcePriority calculator with default weights.
func NewSourcePriority() *SourcePriority {
	return &SourcePriority{
		SourceWeights: map[string]float64{
			"gmail":    1.2, // Email is fairly urgent
			"slack":    1.5, // Slack messages are often time-sensitive
			"manual":   1.0, // User-initiated, normal priority
			"calendar": 1.4, // Calendar-related items are often time-bound
			"batch":    0.6, // Batch imports are lower priority
			"system":   0.8, // System-generated items
			"ai":       1.0, // AI-suggested items
		},
		DefaultWeight: 1.0,
	}
}

// NewSourcePriorityWithWeights creates a SourcePriority with custom weights.
func NewSourcePriorityWithWeights(weights map[string]float64, defaultWeight float64) *SourcePriority {
	p := NewSourcePriority()
	if weights != nil {
		p.SourceWeights = weights
	}
	if defaultWeight > 0 {
		p.DefaultWeight = defaultWeight
	}
	return p
}

// Calculate returns priority based on the item's source.
func (p *SourcePriority) Calculate(item *QueueItem) float64 {
	if item == nil {
		return p.DefaultWeight
	}

	weight := p.SourceWeights[item.Source]
	if weight == 0 {
		return p.DefaultWeight
	}
	return weight
}

// Name returns the calculator name.
func (p *SourcePriority) Name() string {
	return "source_priority"
}

// CompositePriority allows combining arbitrary calculators with custom weights.
type CompositePriority struct {
	// Calculators is the list of calculators with their weights.
	Calculators []WeightedCalculator

	// Normalizer determines how scores are combined.
	Normalizer PriorityNormalizer
}

// WeightedCalculator pairs a calculator with its weight.
type WeightedCalculator struct {
	Calculator PriorityCalculator
	Weight     float64
}

// PriorityNormalizer defines how individual scores are combined.
type PriorityNormalizer int

const (
	// NormalizeWeightedSum sums weighted scores.
	NormalizeWeightedSum PriorityNormalizer = iota

	// NormalizeMax takes the maximum score.
	NormalizeMax

	// NormalizeMin takes the minimum score.
	NormalizeMin

	// NormalizeProduct multiplies all scores.
	NormalizeProduct
)

// NewCompositePriority creates a CompositePriority calculator.
func NewCompositePriority(normalizer PriorityNormalizer, calculators ...WeightedCalculator) *CompositePriority {
	return &CompositePriority{
		Calculators: calculators,
		Normalizer:  normalizer,
	}
}

// AddCalculator adds a calculator with the given weight.
func (p *CompositePriority) AddCalculator(calc PriorityCalculator, weight float64) {
	p.Calculators = append(p.Calculators, WeightedCalculator{
		Calculator: calc,
		Weight:     weight,
	})
}

// Calculate combines all calculator scores using the configured normalizer.
func (p *CompositePriority) Calculate(item *QueueItem) float64 {
	if item == nil || len(p.Calculators) == 0 {
		return 0
	}

	scores := make([]float64, len(p.Calculators))
	weights := make([]float64, len(p.Calculators))

	for i, wc := range p.Calculators {
		scores[i] = wc.Calculator.Calculate(item)
		weights[i] = wc.Weight
	}

	switch p.Normalizer {
	case NormalizeWeightedSum:
		return weightedSum(scores, weights)
	case NormalizeMax:
		return maxScore(scores)
	case NormalizeMin:
		return minScore(scores)
	case NormalizeProduct:
		return productScore(scores)
	default:
		return weightedSum(scores, weights)
	}
}

// Name returns the calculator name.
func (p *CompositePriority) Name() string {
	return "composite_priority"
}

// Helper functions for score combination.

func weightedSum(scores, weights []float64) float64 {
	totalWeight := 0.0
	for _, w := range weights {
		totalWeight += w
	}
	if totalWeight == 0 {
		totalWeight = float64(len(weights))
		for i := range weights {
			weights[i] = 1.0
		}
	}

	sum := 0.0
	for i, score := range scores {
		sum += (weights[i] / totalWeight) * score
	}
	return sum
}

func maxScore(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	max := scores[0]
	for _, s := range scores[1:] {
		if s > max {
			max = s
		}
	}
	return max
}

func minScore(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	min := scores[0]
	for _, s := range scores[1:] {
		if s < min {
			min = s
		}
	}
	return min
}

func productScore(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	product := 1.0
	for _, s := range scores {
		product *= s
	}
	return product
}
