// Package scorer provides confidence scoring for discovered relationships.
// It implements multiple scoring factors that evaluate relationship quality
// based on frequency, recency, source trustworthiness, and cross-source consistency.
package scorer

import (
	"context"
	"math"
	"time"
)

// ScoringFactor defines the interface for individual scoring factors.
// Each factor evaluates a specific aspect of relationship confidence.
type ScoringFactor interface {
	// Name returns the unique name of this scoring factor.
	Name() string

	// Score evaluates the factor for the given relationship and evidence.
	// Returns a score between 0.0 and 1.0.
	Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64

	// Weight returns the relative importance of this factor in the overall score.
	Weight() float64

	// Explanation provides a human-readable explanation of the score.
	Explanation(score float64) string
}

// Relationship represents a discovered relationship between entities.
// This is a simplified internal representation for scoring purposes.
type Relationship struct {
	// ID is the unique identifier for this relationship.
	ID string

	// SourceEntityID is the ID of the source entity.
	SourceEntityID string

	// TargetEntityID is the ID of the target entity.
	TargetEntityID string

	// Type is the relationship type (e.g., "WORKS_AT", "KNOWS").
	Type string

	// CreatedAt is when the relationship was first discovered.
	CreatedAt time.Time

	// UpdatedAt is when the relationship was last updated.
	UpdatedAt time.Time

	// CurrentConfidence is the existing confidence score (if any).
	CurrentConfidence float64

	// Status is the current validation status.
	Status string

	// Metadata contains additional relationship properties.
	Metadata map[string]string
}

// Evidence represents supporting evidence for a relationship.
type Evidence struct {
	// ID is the unique identifier for this evidence.
	ID string

	// SourceID is the content source where evidence was found.
	SourceID string

	// SourceType is the type of source (e.g., "email", "meeting", "document").
	SourceType string

	// Excerpt is the text excerpt supporting the relationship.
	Excerpt string

	// DiscoveredAt is when this evidence was discovered.
	DiscoveredAt time.Time

	// ConfidenceContribution is the confidence from this evidence alone.
	ConfidenceContribution float64

	// SourceQuality is the trustworthiness of this source (0.0 to 1.0).
	SourceQuality float64

	// Metadata contains additional evidence properties.
	Metadata map[string]string
}

// BaseFactor provides common functionality for scoring factors.
type BaseFactor struct {
	name   string
	weight float64
}

// Name returns the factor name.
func (f *BaseFactor) Name() string {
	return f.name
}

// Weight returns the factor weight.
func (f *BaseFactor) Weight() float64 {
	return f.weight
}

// FrequencyFactor scores relationships based on how often they appear in evidence.
// More occurrences indicate a stronger, more established relationship.
type FrequencyFactor struct {
	BaseFactor

	// MinOccurrences is the minimum occurrences for any confidence.
	MinOccurrences int

	// OptimalOccurrences is the count where confidence plateaus at 1.0.
	OptimalOccurrences int
}

// NewFrequencyFactor creates a new FrequencyFactor with the given configuration.
func NewFrequencyFactor(weight float64, minOccurrences, optimalOccurrences int) *FrequencyFactor {
	if minOccurrences < 1 {
		minOccurrences = 1
	}
	if optimalOccurrences < minOccurrences {
		optimalOccurrences = minOccurrences * 5
	}
	return &FrequencyFactor{
		BaseFactor:         BaseFactor{name: "frequency", weight: weight},
		MinOccurrences:     minOccurrences,
		OptimalOccurrences: optimalOccurrences,
	}
}

// DefaultFrequencyFactor creates a FrequencyFactor with sensible defaults.
func DefaultFrequencyFactor() *FrequencyFactor {
	return NewFrequencyFactor(0.25, 1, 10)
}

// Score calculates the frequency-based confidence score.
// Uses a logarithmic scale to prevent excessive bonus from high frequency.
func (f *FrequencyFactor) Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64 {
	count := len(evidence)
	if count < f.MinOccurrences {
		return 0.0
	}

	// Use logarithmic scaling to give diminishing returns
	// log(count + 1) / log(optimal + 1) provides smooth scaling
	logCount := math.Log(float64(count) + 1)
	logOptimal := math.Log(float64(f.OptimalOccurrences) + 1)

	score := logCount / logOptimal
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// Explanation provides a human-readable explanation of the frequency score.
func (f *FrequencyFactor) Explanation(score float64) string {
	switch {
	case score >= 0.9:
		return "Very frequent: appears consistently across many sources"
	case score >= 0.7:
		return "Frequent: appears in multiple sources"
	case score >= 0.5:
		return "Moderate: appears in several sources"
	case score >= 0.3:
		return "Occasional: appears in a few sources"
	case score > 0:
		return "Rare: appears in minimal sources"
	default:
		return "Insufficient evidence: below minimum occurrence threshold"
	}
}

// RecencyFactor scores relationships based on how recent the evidence is.
// Recent evidence indicates an active, current relationship.
type RecencyFactor struct {
	BaseFactor

	// HalfLife is the duration after which score decays to 50%.
	HalfLife time.Duration

	// MinimumScore is the floor score for very old evidence.
	MinimumScore float64

	// Now allows overriding current time for testing.
	Now func() time.Time
}

// NewRecencyFactor creates a new RecencyFactor with the given configuration.
func NewRecencyFactor(weight float64, halfLife time.Duration, minimumScore float64) *RecencyFactor {
	if halfLife <= 0 {
		halfLife = 90 * 24 * time.Hour // 90 days default
	}
	if minimumScore < 0 {
		minimumScore = 0
	}
	if minimumScore > 1 {
		minimumScore = 0.1
	}
	return &RecencyFactor{
		BaseFactor:   BaseFactor{name: "recency", weight: weight},
		HalfLife:     halfLife,
		MinimumScore: minimumScore,
		Now:          time.Now,
	}
}

// DefaultRecencyFactor creates a RecencyFactor with sensible defaults.
// Default half-life of 90 days means evidence loses half its recency value after 3 months.
func DefaultRecencyFactor() *RecencyFactor {
	return NewRecencyFactor(0.25, 90*24*time.Hour, 0.1)
}

// Score calculates the recency-based confidence score.
// Uses exponential decay based on the most recent evidence.
func (f *RecencyFactor) Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64 {
	if len(evidence) == 0 {
		return 0.0
	}

	now := f.Now()

	// Find the most recent evidence
	var mostRecent time.Time
	for _, e := range evidence {
		if e.DiscoveredAt.After(mostRecent) {
			mostRecent = e.DiscoveredAt
		}
	}

	if mostRecent.IsZero() {
		return f.MinimumScore
	}

	// Calculate age and apply exponential decay
	age := now.Sub(mostRecent)
	if age < 0 {
		// Future evidence (data issue) - treat as current
		return 1.0
	}

	// Exponential decay: score = 0.5^(age/half_life)
	decay := math.Pow(0.5, float64(age)/float64(f.HalfLife))

	// Apply minimum score floor
	score := decay
	if score < f.MinimumScore {
		score = f.MinimumScore
	}

	return score
}

// Explanation provides a human-readable explanation of the recency score.
func (f *RecencyFactor) Explanation(score float64) string {
	switch {
	case score >= 0.9:
		return "Very recent: evidence from the last few days"
	case score >= 0.7:
		return "Recent: evidence from the last few weeks"
	case score >= 0.5:
		return "Moderate: evidence from the last few months"
	case score >= 0.3:
		return "Aging: evidence is several months old"
	case score > f.MinimumScore:
		return "Stale: evidence is quite old"
	default:
		return "Historical: evidence is very old but still relevant"
	}
}

// SourceQualityFactor scores relationships based on source trustworthiness.
// Higher quality sources (direct communications) are more reliable than indirect sources.
type SourceQualityFactor struct {
	BaseFactor

	// SourceQualityMap provides quality scores for different source types.
	// Keys are source types, values are quality scores (0.0 to 1.0).
	SourceQualityMap map[string]float64

	// DefaultQuality is used when a source type is not in the map.
	DefaultQuality float64
}

// NewSourceQualityFactor creates a new SourceQualityFactor with the given configuration.
func NewSourceQualityFactor(weight float64, qualityMap map[string]float64, defaultQuality float64) *SourceQualityFactor {
	if qualityMap == nil {
		qualityMap = defaultSourceQualityMap()
	}
	if defaultQuality <= 0 || defaultQuality > 1 {
		defaultQuality = 0.5
	}
	return &SourceQualityFactor{
		BaseFactor:       BaseFactor{name: "source_quality", weight: weight},
		SourceQualityMap: qualityMap,
		DefaultQuality:   defaultQuality,
	}
}

// DefaultSourceQualityFactor creates a SourceQualityFactor with sensible defaults.
func DefaultSourceQualityFactor() *SourceQualityFactor {
	return NewSourceQualityFactor(0.25, defaultSourceQualityMap(), 0.5)
}

// defaultSourceQualityMap returns the default source quality mappings.
func defaultSourceQualityMap() map[string]float64 {
	return map[string]float64{
		// Direct communication sources - highest quality
		"email":           0.9,
		"direct_message":  0.9,
		"chat":            0.85,
		"meeting":         0.95,
		"calendar_invite": 0.9,

		// Semi-direct sources
		"slack_channel":  0.75,
		"teams_channel":  0.75,
		"forum_post":     0.7,
		"comment":        0.65,

		// Document sources - variable quality
		"document":      0.8,
		"presentation":  0.75,
		"spreadsheet":   0.7,
		"wiki_page":     0.7,
		"knowledge_base": 0.75,

		// Indirect sources - lower quality
		"web_page":      0.5,
		"social_media":  0.4,
		"news_article":  0.5,
		"external_link": 0.3,

		// System-generated - context dependent
		"org_chart":     0.85,
		"directory":     0.9,
		"hr_system":     0.95,
		"crm":           0.8,
	}
}

// Score calculates the source quality-based confidence score.
// Uses weighted average of evidence source qualities.
func (f *SourceQualityFactor) Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64 {
	if len(evidence) == 0 {
		return 0.0
	}

	var totalQuality float64
	var totalWeight float64

	for _, e := range evidence {
		// Get quality from evidence if set, otherwise look up by source type
		quality := e.SourceQuality
		if quality <= 0 || quality > 1 {
			if q, ok := f.SourceQualityMap[e.SourceType]; ok {
				quality = q
			} else {
				quality = f.DefaultQuality
			}
		}

		// Weight by confidence contribution if available
		weight := e.ConfidenceContribution
		if weight <= 0 {
			weight = 1.0
		}

		totalQuality += quality * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return f.DefaultQuality
	}

	return totalQuality / totalWeight
}

// Explanation provides a human-readable explanation of the source quality score.
func (f *SourceQualityFactor) Explanation(score float64) string {
	switch {
	case score >= 0.9:
		return "Highly reliable: from authoritative, verified sources"
	case score >= 0.75:
		return "Reliable: from trusted communication channels"
	case score >= 0.6:
		return "Moderate: from standard business sources"
	case score >= 0.4:
		return "Lower confidence: from indirect or informal sources"
	default:
		return "Uncertain: from unverified or external sources"
	}
}

// ConsistencyFactor scores relationships based on agreement across multiple sources.
// Relationships corroborated by independent sources are more reliable.
type ConsistencyFactor struct {
	BaseFactor

	// MinSourcesForBonus is the number of unique sources needed for any consistency bonus.
	MinSourcesForBonus int

	// OptimalSources is the number of sources where consistency score plateaus.
	OptimalSources int
}

// NewConsistencyFactor creates a new ConsistencyFactor with the given configuration.
func NewConsistencyFactor(weight float64, minSources, optimalSources int) *ConsistencyFactor {
	if minSources < 2 {
		minSources = 2
	}
	if optimalSources < minSources {
		optimalSources = minSources * 2
	}
	return &ConsistencyFactor{
		BaseFactor:         BaseFactor{name: "consistency", weight: weight},
		MinSourcesForBonus: minSources,
		OptimalSources:     optimalSources,
	}
}

// DefaultConsistencyFactor creates a ConsistencyFactor with sensible defaults.
func DefaultConsistencyFactor() *ConsistencyFactor {
	return NewConsistencyFactor(0.25, 2, 5)
}

// Score calculates the cross-source consistency score.
// Evaluates how many independent sources corroborate the relationship.
func (f *ConsistencyFactor) Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64 {
	if len(evidence) == 0 {
		return 0.0
	}

	// Count unique source types (independent corroboration)
	sourceTypes := make(map[string]bool)
	sourceIDs := make(map[string]bool)

	for _, e := range evidence {
		if e.SourceType != "" {
			sourceTypes[e.SourceType] = true
		}
		if e.SourceID != "" {
			sourceIDs[e.SourceID] = true
		}
	}

	// Use unique source IDs as primary metric, source types as secondary
	uniqueSources := len(sourceIDs)
	uniqueTypes := len(sourceTypes)

	// Combine both metrics - more unique IDs is better, diverse types also helps
	effectiveCount := uniqueSources
	if uniqueTypes > 1 {
		// Bonus for diversity in source types
		effectiveCount += uniqueTypes - 1
	}

	if effectiveCount < f.MinSourcesForBonus {
		// Single source - base consistency score
		return 0.5
	}

	// Scale from 0.5 (single source) to 1.0 (optimal sources)
	progress := float64(effectiveCount-f.MinSourcesForBonus) /
		float64(f.OptimalSources-f.MinSourcesForBonus)

	if progress > 1.0 {
		progress = 1.0
	}

	// Map to 0.5 - 1.0 range
	return 0.5 + (progress * 0.5)
}

// Explanation provides a human-readable explanation of the consistency score.
func (f *ConsistencyFactor) Explanation(score float64) string {
	switch {
	case score >= 0.95:
		return "Highly corroborated: confirmed by many independent sources"
	case score >= 0.8:
		return "Well corroborated: confirmed by multiple diverse sources"
	case score >= 0.65:
		return "Moderately corroborated: confirmed by several sources"
	case score > 0.5:
		return "Partially corroborated: some independent confirmation"
	default:
		return "Single source: not independently corroborated"
	}
}

// ValidationBoostFactor provides a boost for user-validated relationships.
// This is a special factor that applies when relationships have been manually confirmed.
type ValidationBoostFactor struct {
	BaseFactor

	// ConfirmedBoost is the multiplier applied to confirmed relationships.
	ConfirmedBoost float64

	// RejectedPenalty is the multiplier applied to rejected relationships.
	RejectedPenalty float64
}

// NewValidationBoostFactor creates a new ValidationBoostFactor.
func NewValidationBoostFactor(confirmedBoost, rejectedPenalty float64) *ValidationBoostFactor {
	if confirmedBoost < 1.0 {
		confirmedBoost = 1.0
	}
	if confirmedBoost > 2.0 {
		confirmedBoost = 2.0
	}
	if rejectedPenalty < 0 {
		rejectedPenalty = 0
	}
	if rejectedPenalty > 1.0 {
		rejectedPenalty = 1.0
	}
	return &ValidationBoostFactor{
		BaseFactor:      BaseFactor{name: "validation_boost", weight: 1.0},
		ConfirmedBoost:  confirmedBoost,
		RejectedPenalty: rejectedPenalty,
	}
}

// DefaultValidationBoostFactor creates a ValidationBoostFactor with sensible defaults.
func DefaultValidationBoostFactor() *ValidationBoostFactor {
	return NewValidationBoostFactor(1.2, 0.1)
}

// Score returns a multiplier based on validation status.
// Returns 1.0 for unvalidated, boost for confirmed, penalty for rejected.
func (f *ValidationBoostFactor) Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64 {
	switch rel.Status {
	case "confirmed", "CONFIRMED", "validated", "VALIDATED":
		return f.ConfirmedBoost
	case "rejected", "REJECTED":
		return f.RejectedPenalty
	default:
		return 1.0 // Neutral for unvalidated
	}
}

// Explanation provides a human-readable explanation of the validation status.
func (f *ValidationBoostFactor) Explanation(score float64) string {
	switch {
	case score > 1.0:
		return "User confirmed: manually validated as correct"
	case score < 1.0:
		return "User rejected: manually marked as incorrect"
	default:
		return "Pending validation: not yet reviewed"
	}
}

// TimeSinceLastInteractionFactor scores based on evidence of ongoing interaction.
// This measures relationship activity over time, not just recency.
type TimeSinceLastInteractionFactor struct {
	BaseFactor

	// ActivePeriod is the time window considered "active" for full score.
	ActivePeriod time.Duration

	// InactivePeriod is the time window after which relationship is considered dormant.
	InactivePeriod time.Duration

	// Now allows overriding current time for testing.
	Now func() time.Time
}

// NewTimeSinceLastInteractionFactor creates a new TimeSinceLastInteractionFactor.
func NewTimeSinceLastInteractionFactor(weight float64, activePeriod, inactivePeriod time.Duration) *TimeSinceLastInteractionFactor {
	if activePeriod <= 0 {
		activePeriod = 30 * 24 * time.Hour // 30 days
	}
	if inactivePeriod <= activePeriod {
		inactivePeriod = 6 * 30 * 24 * time.Hour // 6 months
	}
	return &TimeSinceLastInteractionFactor{
		BaseFactor:     BaseFactor{name: "interaction_recency", weight: weight},
		ActivePeriod:   activePeriod,
		InactivePeriod: inactivePeriod,
		Now:            time.Now,
	}
}

// DefaultTimeSinceLastInteractionFactor creates with sensible defaults.
func DefaultTimeSinceLastInteractionFactor() *TimeSinceLastInteractionFactor {
	return NewTimeSinceLastInteractionFactor(0.0, 30*24*time.Hour, 180*24*time.Hour)
}

// Score calculates an activity-based score.
func (f *TimeSinceLastInteractionFactor) Score(ctx context.Context, rel *Relationship, evidence []Evidence) float64 {
	if len(evidence) == 0 {
		return 0.0
	}

	now := f.Now()

	// Find the most recent evidence
	var mostRecent time.Time
	for _, e := range evidence {
		if e.DiscoveredAt.After(mostRecent) {
			mostRecent = e.DiscoveredAt
		}
	}

	if mostRecent.IsZero() {
		return 0.0
	}

	age := now.Sub(mostRecent)
	if age < 0 {
		return 1.0 // Future evidence treated as current
	}

	if age <= f.ActivePeriod {
		return 1.0 // Full score within active period
	}

	if age >= f.InactivePeriod {
		return 0.1 // Minimum score for dormant relationships
	}

	// Linear interpolation between active and inactive periods
	progress := float64(age-f.ActivePeriod) / float64(f.InactivePeriod-f.ActivePeriod)
	return 1.0 - (progress * 0.9) // Scale from 1.0 to 0.1
}

// Explanation provides a human-readable explanation of the interaction score.
func (f *TimeSinceLastInteractionFactor) Explanation(score float64) string {
	switch {
	case score >= 0.9:
		return "Active relationship: recent interactions observed"
	case score >= 0.5:
		return "Moderately active: some recent activity"
	case score > 0.1:
		return "Declining activity: becoming dormant"
	default:
		return "Dormant relationship: no recent interactions"
	}
}
