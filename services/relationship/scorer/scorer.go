// Package scorer provides confidence scoring for discovered relationships.
// The ConfidenceScorer aggregates multiple scoring factors to produce an overall
// confidence score that reflects the reliability of a discovered relationship.
package scorer

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ScoreResult contains the detailed scoring result for a relationship.
type ScoreResult struct {
	// Confidence is the final aggregated confidence score (0.0 to 1.0).
	Confidence float64

	// FactorScores contains individual scores from each factor.
	FactorScores map[string]FactorScore

	// Explanation is a human-readable summary of the score.
	Explanation string

	// RawScore is the weighted sum before normalization.
	RawScore float64

	// TotalWeight is the sum of all factor weights.
	TotalWeight float64

	// ScoredAt is when the score was calculated.
	ScoredAt time.Time

	// DecayedFrom indicates the original score if decay was applied.
	DecayedFrom *float64

	// PassesThreshold indicates if the score passes the minimum threshold.
	PassesThreshold bool
}

// FactorScore contains the scoring details for a single factor.
type FactorScore struct {
	// Name is the factor name.
	Name string

	// Score is the raw score from this factor (0.0 to 1.0).
	Score float64

	// Weight is the factor's weight in the overall calculation.
	Weight float64

	// WeightedScore is Score * Weight.
	WeightedScore float64

	// Explanation is a human-readable explanation of this factor's score.
	Explanation string
}

// ScorerConfig holds configuration for the ConfidenceScorer.
type ScorerConfig struct {
	// Factors is the list of scoring factors to use.
	Factors []ScoringFactor

	// MinimumConfidence is the threshold below which relationships are filtered.
	MinimumConfidence float64

	// DecayEnabled enables time-based score decay.
	DecayEnabled bool

	// DecayHalfLife is the duration for score to decay by 50%.
	DecayHalfLife time.Duration

	// DecayMinimum is the floor for decayed scores.
	DecayMinimum float64

	// ValidationBoostEnabled enables boost/penalty for validated relationships.
	ValidationBoostEnabled bool

	// ValidationBoostFactor is used when validation boost is enabled.
	ValidationBoostFactor *ValidationBoostFactor

	// NormalizationMethod specifies how to normalize the final score.
	NormalizationMethod NormalizationMethod
}

// NormalizationMethod specifies how to combine factor scores.
type NormalizationMethod string

const (
	// NormalizationWeightedAverage uses weighted average of factor scores.
	NormalizationWeightedAverage NormalizationMethod = "weighted_average"

	// NormalizationGeometricMean uses geometric mean for multiplicative combination.
	NormalizationGeometricMean NormalizationMethod = "geometric_mean"

	// NormalizationMinimum uses the minimum factor score (conservative).
	NormalizationMinimum NormalizationMethod = "minimum"
)

// DefaultScorerConfig returns a configuration with sensible defaults.
func DefaultScorerConfig() *ScorerConfig {
	return &ScorerConfig{
		Factors: []ScoringFactor{
			DefaultFrequencyFactor(),
			DefaultRecencyFactor(),
			DefaultSourceQualityFactor(),
			DefaultConsistencyFactor(),
		},
		MinimumConfidence:      0.3,
		DecayEnabled:           true,
		DecayHalfLife:          180 * 24 * time.Hour, // 6 months
		DecayMinimum:           0.1,
		ValidationBoostEnabled: true,
		ValidationBoostFactor:  DefaultValidationBoostFactor(),
		NormalizationMethod:    NormalizationWeightedAverage,
	}
}

// ConfidenceScorer calculates confidence scores for relationships.
// It combines multiple scoring factors with configurable weights to produce
// an overall confidence score reflecting relationship reliability.
type ConfidenceScorer struct {
	config *ScorerConfig
	logger logging.Logger
	now    func() time.Time
}

// NewConfidenceScorer creates a new ConfidenceScorer with the given configuration.
func NewConfidenceScorer(cfg *ScorerConfig, logger logging.Logger) *ConfidenceScorer {
	if cfg == nil {
		cfg = DefaultScorerConfig()
	}
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &ConfidenceScorer{
		config: cfg,
		logger: logger.With(logging.F("component", "confidence_scorer")),
		now:    time.Now,
	}
}

// Score calculates the confidence score for a relationship based on its current state.
// This method uses only the relationship's existing evidence.
func (s *ConfidenceScorer) Score(ctx context.Context, rel *Relationship) (*ScoreResult, error) {
	if rel == nil {
		return nil, fmt.Errorf("relationship cannot be nil")
	}

	// Score without additional evidence
	return s.ScoreWithEvidence(ctx, rel, nil)
}

// ScoreWithEvidence calculates the confidence score using provided evidence.
// The evidence slice can include new evidence not yet associated with the relationship.
func (s *ConfidenceScorer) ScoreWithEvidence(ctx context.Context, rel *Relationship, evidence []Evidence) (*ScoreResult, error) {
	if rel == nil {
		return nil, fmt.Errorf("relationship cannot be nil")
	}

	s.logger.Debug("Scoring relationship",
		logging.F("relationship_id", rel.ID),
		logging.F("evidence_count", len(evidence)),
	)

	result := &ScoreResult{
		FactorScores: make(map[string]FactorScore),
		ScoredAt:     s.now(),
	}

	// Calculate individual factor scores
	var totalWeight float64
	var weightedSum float64

	for _, factor := range s.config.Factors {
		score := factor.Score(ctx, rel, evidence)
		weight := factor.Weight()

		// Clamp score to valid range
		if score < 0 {
			score = 0
		}
		if score > 1 {
			score = 1
		}

		fs := FactorScore{
			Name:          factor.Name(),
			Score:         score,
			Weight:        weight,
			WeightedScore: score * weight,
			Explanation:   factor.Explanation(score),
		}

		result.FactorScores[factor.Name()] = fs

		weightedSum += fs.WeightedScore
		totalWeight += weight
	}

	result.RawScore = weightedSum
	result.TotalWeight = totalWeight

	// Calculate final confidence using the configured normalization method
	result.Confidence = s.normalizeScore(result, evidence)

	// Apply validation boost if enabled
	if s.config.ValidationBoostEnabled && s.config.ValidationBoostFactor != nil {
		boost := s.config.ValidationBoostFactor.Score(ctx, rel, evidence)
		if boost != 1.0 {
			result.Confidence *= boost
			// Clamp after boost
			if result.Confidence > 1.0 {
				result.Confidence = 1.0
			}
			if result.Confidence < 0 {
				result.Confidence = 0
			}
		}
	}

	// Apply decay if enabled
	if s.config.DecayEnabled {
		original := result.Confidence
		result.Confidence = s.applyDecay(rel, result.Confidence)
		if result.Confidence != original {
			result.DecayedFrom = &original
		}
	}

	// Check threshold
	result.PassesThreshold = result.Confidence >= s.config.MinimumConfidence

	// Generate explanation
	result.Explanation = s.generateExplanation(result)

	s.logger.Debug("Scored relationship",
		logging.F("relationship_id", rel.ID),
		logging.F("confidence", result.Confidence),
		logging.F("passes_threshold", result.PassesThreshold),
	)

	return result, nil
}

// normalizeScore applies the configured normalization method to factor scores.
func (s *ConfidenceScorer) normalizeScore(result *ScoreResult, evidence []Evidence) float64 {
	switch s.config.NormalizationMethod {
	case NormalizationGeometricMean:
		return s.geometricMeanNormalization(result)
	case NormalizationMinimum:
		return s.minimumNormalization(result)
	default: // NormalizationWeightedAverage
		return s.weightedAverageNormalization(result)
	}
}

// weightedAverageNormalization calculates weighted average of factor scores.
func (s *ConfidenceScorer) weightedAverageNormalization(result *ScoreResult) float64 {
	if result.TotalWeight == 0 {
		return 0
	}
	return result.RawScore / result.TotalWeight
}

// geometricMeanNormalization uses geometric mean for multiplicative combination.
// This penalizes relationships that have any very low factor score.
func (s *ConfidenceScorer) geometricMeanNormalization(result *ScoreResult) float64 {
	if len(result.FactorScores) == 0 {
		return 0
	}

	// Weighted geometric mean: exp(sum(weight * log(score)) / total_weight)
	var weightedLogSum float64
	var totalWeight float64

	for _, fs := range result.FactorScores {
		if fs.Score <= 0 {
			// Handle zero scores - use a small epsilon
			fs.Score = 0.001
		}
		weightedLogSum += fs.Weight * math.Log(fs.Score)
		totalWeight += fs.Weight
	}

	if totalWeight == 0 {
		return 0
	}

	return math.Exp(weightedLogSum / totalWeight)
}

// minimumNormalization uses the minimum weighted score (conservative approach).
func (s *ConfidenceScorer) minimumNormalization(result *ScoreResult) float64 {
	if len(result.FactorScores) == 0 {
		return 0
	}

	minScore := 1.0
	for _, fs := range result.FactorScores {
		if fs.Score < minScore {
			minScore = fs.Score
		}
	}

	return minScore
}

// applyDecay applies time-based decay to the confidence score.
func (s *ConfidenceScorer) applyDecay(rel *Relationship, confidence float64) float64 {
	if !s.config.DecayEnabled || s.config.DecayHalfLife <= 0 {
		return confidence
	}

	// Decay based on time since last update
	age := s.now().Sub(rel.UpdatedAt)
	if age <= 0 {
		return confidence
	}

	// Exponential decay: score = original * 0.5^(age/half_life)
	decay := math.Pow(0.5, float64(age)/float64(s.config.DecayHalfLife))
	decayed := confidence * decay

	// Apply minimum floor
	if decayed < s.config.DecayMinimum {
		decayed = s.config.DecayMinimum
	}

	return decayed
}

// generateExplanation creates a human-readable summary of the score.
func (s *ConfidenceScorer) generateExplanation(result *ScoreResult) string {
	var parts []string

	// Overall assessment
	switch {
	case result.Confidence >= 0.9:
		parts = append(parts, "Very high confidence relationship.")
	case result.Confidence >= 0.7:
		parts = append(parts, "High confidence relationship.")
	case result.Confidence >= 0.5:
		parts = append(parts, "Moderate confidence relationship.")
	case result.Confidence >= 0.3:
		parts = append(parts, "Low confidence relationship.")
	default:
		parts = append(parts, "Very low confidence relationship.")
	}

	// Sort factor scores by weight for consistent output
	factors := make([]FactorScore, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factors = append(factors, fs)
	}
	sort.Slice(factors, func(i, j int) bool {
		return factors[i].Weight > factors[j].Weight
	})

	// Add key factor summaries
	for _, fs := range factors {
		if fs.Weight > 0 {
			parts = append(parts, fmt.Sprintf("%s: %s", fs.Name, fs.Explanation))
		}
	}

	// Note decay if applied
	if result.DecayedFrom != nil {
		parts = append(parts, fmt.Sprintf("Score decayed from %.2f due to age.", *result.DecayedFrom))
	}

	// Note threshold status
	if !result.PassesThreshold {
		parts = append(parts, fmt.Sprintf("Below minimum threshold of %.2f.", s.config.MinimumConfidence))
	}

	return strings.Join(parts, " ")
}

// AggregateScores combines multiple confidence scores into a single score.
// This is useful when scoring relationships with evidence from multiple discovery runs.
func (s *ConfidenceScorer) AggregateScores(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}

	if len(scores) == 1 {
		return scores[0]
	}

	// Use probabilistic combination: P(A or B) = P(A) + P(B) - P(A)*P(B)
	// Extended to multiple values for "belief" aggregation
	combined := scores[0]
	for i := 1; i < len(scores); i++ {
		combined = combined + scores[i] - (combined * scores[i])
	}

	// Clamp to valid range
	if combined > 1.0 {
		combined = 1.0
	}
	if combined < 0 {
		combined = 0
	}

	return combined
}

// FilterByThreshold returns only relationships that pass the minimum confidence threshold.
func (s *ConfidenceScorer) FilterByThreshold(results []*ScoreResult) []*ScoreResult {
	filtered := make([]*ScoreResult, 0, len(results))
	for _, r := range results {
		if r.PassesThreshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// FilterByConfidence returns only relationships with confidence >= the specified threshold.
func (s *ConfidenceScorer) FilterByConfidence(results []*ScoreResult, threshold float64) []*ScoreResult {
	filtered := make([]*ScoreResult, 0, len(results))
	for _, r := range results {
		if r.Confidence >= threshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// ScoreBatch scores multiple relationships efficiently.
func (s *ConfidenceScorer) ScoreBatch(ctx context.Context, relationships []*Relationship, evidenceMap map[string][]Evidence) ([]*ScoreResult, error) {
	results := make([]*ScoreResult, 0, len(relationships))

	for _, rel := range relationships {
		var evidence []Evidence
		if evidenceMap != nil {
			evidence = evidenceMap[rel.ID]
		}

		result, err := s.ScoreWithEvidence(ctx, rel, evidence)
		if err != nil {
			s.logger.Warn("Failed to score relationship",
				logging.F("relationship_id", rel.ID),
				logging.Err(err),
			)
			continue
		}

		results = append(results, result)
	}

	return results, nil
}

// GetMinimumConfidence returns the configured minimum confidence threshold.
func (s *ConfidenceScorer) GetMinimumConfidence() float64 {
	return s.config.MinimumConfidence
}

// SetMinimumConfidence updates the minimum confidence threshold.
func (s *ConfidenceScorer) SetMinimumConfidence(threshold float64) {
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}
	s.config.MinimumConfidence = threshold
}

// GetFactorWeights returns a map of factor names to their weights.
func (s *ConfidenceScorer) GetFactorWeights() map[string]float64 {
	weights := make(map[string]float64)
	for _, factor := range s.config.Factors {
		weights[factor.Name()] = factor.Weight()
	}
	return weights
}

// noopLogger is a no-op implementation of logging.Logger for when no logger is provided.
