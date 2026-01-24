// Package conflict provides conflict detection and resolution for discovered relationships.
package conflict

import (
	"fmt"
	"sort"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/relationship/extractor"
)

// StrategyRegistry holds all registered resolution strategies.
type StrategyRegistry struct {
	strategies map[string]Strategy
	logger     logging.Logger
}

// NewStrategyRegistry creates a new StrategyRegistry.
func NewStrategyRegistry(logger logging.Logger) *StrategyRegistry {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &StrategyRegistry{
		strategies: make(map[string]Strategy),
		logger:     logger.With(logging.F("component", "strategy_registry")),
	}
}

// Register adds a strategy to the registry.
func (r *StrategyRegistry) Register(strategy Strategy) {
	r.strategies[strategy.Name()] = strategy
	r.logger.Debug("Registered strategy", logging.F("name", strategy.Name()))
}

// RegisterDefaults registers all default strategies.
func (r *StrategyRegistry) RegisterDefaults() {
	r.Register(NewLatestWinsStrategy(r.logger))
	r.Register(NewHighConfidenceStrategy(r.logger))
	r.Register(NewMergeStrategy(r.logger))
	r.Register(NewHumanReviewStrategy(r.logger))
}

// Get retrieves a strategy by name.
func (r *StrategyRegistry) Get(name string) (Strategy, bool) {
	s, ok := r.strategies[name]
	return s, ok
}

// List returns all registered strategy names.
func (r *StrategyRegistry) List() []string {
	names := make([]string, 0, len(r.strategies))
	for name := range r.strategies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetBestStrategy returns the best strategy for the given conflict type.
func (r *StrategyRegistry) GetBestStrategy(conflictType ConflictType) Strategy {
	// Strategy preferences by conflict type
	preferences := map[ConflictType][]string{
		ConflictTypeDuplicate:     {"high_confidence", "latest_wins", "merge"},
		ConflictTypeContradiction: {"high_confidence", "human_review"},
		ConflictTypeCycle:         {"high_confidence", "human_review"},
		ConflictTypeInconsistency: {"merge", "high_confidence", "latest_wins"},
	}

	if prefs, ok := preferences[conflictType]; ok {
		for _, name := range prefs {
			if s, ok := r.strategies[name]; ok {
				return s
			}
		}
	}

	// Default to high_confidence if available, otherwise first registered
	if s, ok := r.strategies["high_confidence"]; ok {
		return s
	}
	for _, s := range r.strategies {
		return s
	}
	return nil
}

// LatestWinsStrategy resolves conflicts by keeping the most recent relationship.
type LatestWinsStrategy struct {
	logger           logging.Logger
	minConfidence    float64
	supportedTypes   map[ConflictType]bool
}

// NewLatestWinsStrategy creates a new LatestWinsStrategy.
func NewLatestWinsStrategy(logger logging.Logger) *LatestWinsStrategy {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &LatestWinsStrategy{
		logger:        logger.With(logging.F("strategy", "latest_wins")),
		minConfidence: 0.6,
		supportedTypes: map[ConflictType]bool{
			ConflictTypeDuplicate:     true,
			ConflictTypeInconsistency: true,
		},
	}
}

// Name returns the strategy name.
func (s *LatestWinsStrategy) Name() string {
	return "latest_wins"
}

// CanResolve checks if this strategy can handle the conflict type.
func (s *LatestWinsStrategy) CanResolve(conflictType ConflictType) bool {
	return s.supportedTypes[conflictType]
}

// MinConfidenceForAuto returns the minimum confidence for automatic resolution.
func (s *LatestWinsStrategy) MinConfidenceForAuto() float64 {
	return s.minConfidence
}

// Resolve applies the latest-wins strategy to the conflict.
func (s *LatestWinsStrategy) Resolve(conflict *Conflict) (*Resolution, error) {
	if conflict == nil || len(conflict.Relationships) == 0 {
		return nil, fmt.Errorf("conflict has no relationships")
	}

	if !s.CanResolve(conflict.Type) {
		return nil, fmt.Errorf("strategy cannot resolve conflict type: %s", conflict.Type)
	}

	// Find the most recent relationship
	var latest *extractor.Relationship
	for _, rel := range conflict.Relationships {
		if latest == nil || rel.UpdatedAt.After(latest.UpdatedAt) {
			latest = rel
		}
	}

	// Calculate confidence based on time difference
	confidence := s.calculateConfidence(conflict.Relationships, latest)

	// Build list of relationships to remove
	removed := make([]string, 0, len(conflict.Relationships)-1)
	for _, rel := range conflict.Relationships {
		if rel.ID != latest.ID {
			removed = append(removed, rel.ID)
		}
	}

	return &Resolution{
		Action:                 ResolutionActionKeep,
		Confidence:             confidence,
		Explanation:            fmt.Sprintf("Kept most recent relationship (updated: %s)", latest.UpdatedAt.Format(time.RFC3339)),
		Strategy:               s.Name(),
		KeptRelationshipID:     latest.ID,
		RemovedRelationshipIDs: removed,
		AppliedAt:              time.Now(),
		AppliedBy:              "system:" + s.Name(),
		Metadata:               make(map[string]string),
	}, nil
}

// ProposeOptions returns resolution options for the conflict.
func (s *LatestWinsStrategy) ProposeOptions(conflict *Conflict) ([]ResolutionOption, error) {
	if conflict == nil || len(conflict.Relationships) == 0 {
		return nil, fmt.Errorf("conflict has no relationships")
	}

	// Sort by update time (most recent first)
	sorted := make([]*extractor.Relationship, len(conflict.Relationships))
	copy(sorted, conflict.Relationships)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UpdatedAt.After(sorted[j].UpdatedAt)
	})

	options := make([]ResolutionOption, 0, len(sorted))

	for i, rel := range sorted {
		removed := make([]string, 0, len(sorted)-1)
		for _, other := range sorted {
			if other.ID != rel.ID {
				removed = append(removed, other.ID)
			}
		}

		confidence := s.calculateConfidence(conflict.Relationships, rel)

		option := ResolutionOption{
			Resolution: &Resolution{
				Action:                 ResolutionActionKeep,
				Confidence:             confidence,
				Explanation:            fmt.Sprintf("Keep relationship from %s", rel.UpdatedAt.Format(time.RFC3339)),
				Strategy:               s.Name(),
				KeptRelationshipID:     rel.ID,
				RemovedRelationshipIDs: removed,
				Metadata:               make(map[string]string),
			},
			Score:       1.0 - float64(i)*0.2, // Decrease score for older relationships
			Recommended: i == 0,               // Most recent is recommended
		}

		if i == 0 {
			option.Pros = []string{"Most recent data", "Reflects current state"}
			option.Cons = []string{"May discard valuable historical context"}
		} else {
			option.Pros = []string{"Preserves older relationship"}
			option.Cons = []string{"Data may be outdated"}
		}

		options = append(options, option)
	}

	return options, nil
}

// calculateConfidence calculates resolution confidence based on time difference.
func (s *LatestWinsStrategy) calculateConfidence(rels []*extractor.Relationship, latest *extractor.Relationship) float64 {
	if len(rels) <= 1 {
		return 1.0
	}

	// Find the second most recent
	var secondLatest time.Time
	for _, rel := range rels {
		if rel.ID != latest.ID && rel.UpdatedAt.After(secondLatest) {
			secondLatest = rel.UpdatedAt
		}
	}

	// Calculate confidence based on time gap
	gap := latest.UpdatedAt.Sub(secondLatest)

	// More confident if the gap is larger
	if gap >= 7*24*time.Hour { // 7+ days
		return 0.9
	}
	if gap >= 24*time.Hour { // 1+ days
		return 0.8
	}
	if gap >= time.Hour { // 1+ hours
		return 0.7
	}

	return 0.6
}

// HighConfidenceStrategy resolves conflicts by keeping the highest confidence relationship.
type HighConfidenceStrategy struct {
	logger           logging.Logger
	minConfidence    float64
	supportedTypes   map[ConflictType]bool
}

// NewHighConfidenceStrategy creates a new HighConfidenceStrategy.
func NewHighConfidenceStrategy(logger logging.Logger) *HighConfidenceStrategy {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &HighConfidenceStrategy{
		logger:        logger.With(logging.F("strategy", "high_confidence")),
		minConfidence: 0.7,
		supportedTypes: map[ConflictType]bool{
			ConflictTypeDuplicate:     true,
			ConflictTypeContradiction: true,
			ConflictTypeCycle:         true,
			ConflictTypeInconsistency: true,
		},
	}
}

// Name returns the strategy name.
func (s *HighConfidenceStrategy) Name() string {
	return "high_confidence"
}

// CanResolve checks if this strategy can handle the conflict type.
func (s *HighConfidenceStrategy) CanResolve(conflictType ConflictType) bool {
	return s.supportedTypes[conflictType]
}

// MinConfidenceForAuto returns the minimum confidence for automatic resolution.
func (s *HighConfidenceStrategy) MinConfidenceForAuto() float64 {
	return s.minConfidence
}

// Resolve applies the high-confidence strategy to the conflict.
func (s *HighConfidenceStrategy) Resolve(conflict *Conflict) (*Resolution, error) {
	if conflict == nil || len(conflict.Relationships) == 0 {
		return nil, fmt.Errorf("conflict has no relationships")
	}

	// Find the highest confidence relationship
	var highest *extractor.Relationship
	for _, rel := range conflict.Relationships {
		if highest == nil || rel.Confidence > highest.Confidence {
			highest = rel
		}
	}

	// Calculate resolution confidence based on confidence gap
	confidence := s.calculateConfidence(conflict.Relationships, highest)

	// Check if confidence is sufficient for auto-resolution
	if confidence < s.minConfidence {
		return &Resolution{
			Action:      ResolutionActionFlagForReview,
			Confidence:  confidence,
			Explanation: fmt.Sprintf("Confidence gap insufficient for auto-resolution (%.2f < %.2f)", confidence, s.minConfidence),
			Strategy:    s.Name(),
			AppliedAt:   time.Now(),
			AppliedBy:   "system:" + s.Name(),
			Metadata:    make(map[string]string),
		}, nil
	}

	// Build list of relationships to remove
	removed := make([]string, 0, len(conflict.Relationships)-1)
	for _, rel := range conflict.Relationships {
		if rel.ID != highest.ID {
			removed = append(removed, rel.ID)
		}
	}

	return &Resolution{
		Action:                 ResolutionActionKeep,
		Confidence:             confidence,
		Explanation:            fmt.Sprintf("Kept highest confidence relationship (%.2f)", highest.Confidence),
		Strategy:               s.Name(),
		KeptRelationshipID:     highest.ID,
		RemovedRelationshipIDs: removed,
		AppliedAt:              time.Now(),
		AppliedBy:              "system:" + s.Name(),
		Metadata:               make(map[string]string),
	}, nil
}

// ProposeOptions returns resolution options for the conflict.
func (s *HighConfidenceStrategy) ProposeOptions(conflict *Conflict) ([]ResolutionOption, error) {
	if conflict == nil || len(conflict.Relationships) == 0 {
		return nil, fmt.Errorf("conflict has no relationships")
	}

	// Sort by confidence (highest first)
	sorted := make([]*extractor.Relationship, len(conflict.Relationships))
	copy(sorted, conflict.Relationships)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Confidence > sorted[j].Confidence
	})

	options := make([]ResolutionOption, 0, len(sorted))

	for i, rel := range sorted {
		removed := make([]string, 0, len(sorted)-1)
		for _, other := range sorted {
			if other.ID != rel.ID {
				removed = append(removed, other.ID)
			}
		}

		confidence := s.calculateConfidence(conflict.Relationships, rel)

		option := ResolutionOption{
			Resolution: &Resolution{
				Action:                 ResolutionActionKeep,
				Confidence:             confidence,
				Explanation:            fmt.Sprintf("Keep relationship with confidence %.2f", rel.Confidence),
				Strategy:               s.Name(),
				KeptRelationshipID:     rel.ID,
				RemovedRelationshipIDs: removed,
				Metadata:               make(map[string]string),
			},
			Score:       float64(rel.Confidence),
			Recommended: i == 0,
		}

		if i == 0 {
			option.Pros = []string{"Highest confidence score", "Most reliable evidence"}
			option.Cons = []string{"May discard relationships with useful context"}
		} else {
			option.Pros = []string{fmt.Sprintf("Confidence: %.2f", rel.Confidence)}
			option.Cons = []string{"Lower confidence than other options"}
		}

		options = append(options, option)
	}

	return options, nil
}

// calculateConfidence calculates resolution confidence based on confidence gap.
func (s *HighConfidenceStrategy) calculateConfidence(rels []*extractor.Relationship, highest *extractor.Relationship) float64 {
	if len(rels) <= 1 {
		return 1.0
	}

	// Find the second highest confidence
	var secondHighest float32
	for _, rel := range rels {
		if rel.ID != highest.ID && rel.Confidence > secondHighest {
			secondHighest = rel.Confidence
		}
	}

	// Calculate confidence based on gap
	gap := highest.Confidence - secondHighest

	// Larger gap = higher confidence in resolution
	if gap >= 0.3 {
		return 0.95
	}
	if gap >= 0.2 {
		return 0.85
	}
	if gap >= 0.1 {
		return 0.75
	}
	if gap >= 0.05 {
		return 0.65
	}

	return 0.5 // Too close to auto-resolve confidently
}

// MergeStrategy resolves conflicts by combining relationships into one.
type MergeStrategy struct {
	logger           logging.Logger
	minConfidence    float64
	supportedTypes   map[ConflictType]bool
}

// NewMergeStrategy creates a new MergeStrategy.
func NewMergeStrategy(logger logging.Logger) *MergeStrategy {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &MergeStrategy{
		logger:        logger.With(logging.F("strategy", "merge")),
		minConfidence: 0.6,
		supportedTypes: map[ConflictType]bool{
			ConflictTypeDuplicate:     true,
			ConflictTypeInconsistency: true,
		},
	}
}

// Name returns the strategy name.
func (s *MergeStrategy) Name() string {
	return "merge"
}

// CanResolve checks if this strategy can handle the conflict type.
func (s *MergeStrategy) CanResolve(conflictType ConflictType) bool {
	return s.supportedTypes[conflictType]
}

// MinConfidenceForAuto returns the minimum confidence for automatic resolution.
func (s *MergeStrategy) MinConfidenceForAuto() float64 {
	return s.minConfidence
}

// Resolve applies the merge strategy to the conflict.
func (s *MergeStrategy) Resolve(conflict *Conflict) (*Resolution, error) {
	if conflict == nil || len(conflict.Relationships) == 0 {
		return nil, fmt.Errorf("conflict has no relationships")
	}

	if !s.CanResolve(conflict.Type) {
		return nil, fmt.Errorf("strategy cannot resolve conflict type: %s", conflict.Type)
	}

	// Create merged relationship
	merged := s.mergeRelationships(conflict.Relationships)

	// All original relationships will be replaced
	removed := make([]string, len(conflict.Relationships))
	for i, rel := range conflict.Relationships {
		removed[i] = rel.ID
	}

	return &Resolution{
		Action:                 ResolutionActionMerge,
		Confidence:             0.8, // Merge is generally reliable
		Explanation:            fmt.Sprintf("Merged %d relationships into one with combined evidence", len(conflict.Relationships)),
		Strategy:               s.Name(),
		MergedRelationship:     merged,
		RemovedRelationshipIDs: removed,
		AppliedAt:              time.Now(),
		AppliedBy:              "system:" + s.Name(),
		Metadata:               make(map[string]string),
	}, nil
}

// ProposeOptions returns resolution options for the conflict.
func (s *MergeStrategy) ProposeOptions(conflict *Conflict) ([]ResolutionOption, error) {
	if conflict == nil || len(conflict.Relationships) == 0 {
		return nil, fmt.Errorf("conflict has no relationships")
	}

	merged := s.mergeRelationships(conflict.Relationships)

	removed := make([]string, len(conflict.Relationships))
	for i, rel := range conflict.Relationships {
		removed[i] = rel.ID
	}

	return []ResolutionOption{
		{
			Resolution: &Resolution{
				Action:                 ResolutionActionMerge,
				Confidence:             0.8,
				Explanation:            fmt.Sprintf("Merge all %d relationships into one", len(conflict.Relationships)),
				Strategy:               s.Name(),
				MergedRelationship:     merged,
				RemovedRelationshipIDs: removed,
				Metadata:               make(map[string]string),
			},
			Score:       0.8,
			Pros:        []string{"Preserves all evidence", "Combines confidence scores", "No data loss"},
			Cons:        []string{"May combine incorrect data", "Creates new relationship"},
			Recommended: true,
		},
	}, nil
}

// mergeRelationships combines multiple relationships into one.
func (s *MergeStrategy) mergeRelationships(rels []*extractor.Relationship) *extractor.Relationship {
	if len(rels) == 0 {
		return nil
	}

	// Use the first relationship as base
	base := rels[0]

	// Combine evidence from all relationships
	allEvidence := make([]extractor.Evidence, 0)
	for _, rel := range rels {
		allEvidence = append(allEvidence, rel.Evidence...)
	}

	// Calculate combined confidence using probabilistic combination
	combinedConfidence := float32(0)
	for _, rel := range rels {
		combinedConfidence = combinedConfidence + rel.Confidence - (combinedConfidence * rel.Confidence)
	}
	// Cap at 1.0
	if combinedConfidence > 1.0 {
		combinedConfidence = 1.0
	}

	// Calculate combined strength (weighted average by confidence)
	totalWeight := float32(0)
	weightedStrength := float32(0)
	for _, rel := range rels {
		weightedStrength += rel.Strength * rel.Confidence
		totalWeight += rel.Confidence
	}
	var combinedStrength float32
	if totalWeight > 0 {
		combinedStrength = weightedStrength / totalWeight
	}

	// Find the most recent timestamps
	latestCreated := base.CreatedAt
	latestUpdated := base.UpdatedAt
	for _, rel := range rels {
		if rel.CreatedAt.After(latestCreated) {
			latestCreated = rel.CreatedAt
		}
		if rel.UpdatedAt.After(latestUpdated) {
			latestUpdated = rel.UpdatedAt
		}
	}

	// Build merged context
	contexts := make([]string, 0)
	for _, rel := range rels {
		if rel.Context != "" {
			contexts = append(contexts, rel.Context)
		}
	}
	mergedContext := ""
	if len(contexts) > 0 {
		mergedContext = contexts[0]
		if len(contexts) > 1 {
			mergedContext += fmt.Sprintf(" (merged from %d sources)", len(rels))
		}
	}

	return &extractor.Relationship{
		ID:         base.ID + "-merged",
		Source:     base.Source,
		Target:     base.Target,
		Type:       base.Type,
		Strength:   combinedStrength,
		Confidence: combinedConfidence,
		Context:    mergedContext,
		Evidence:   allEvidence,
		TenantID:   base.TenantID,
		CreatedAt:  latestCreated,
		UpdatedAt:  latestUpdated,
	}
}

// HumanReviewStrategy queues conflicts for human review.
type HumanReviewStrategy struct {
	logger           logging.Logger
	minConfidence    float64
	supportedTypes   map[ConflictType]bool
}

// NewHumanReviewStrategy creates a new HumanReviewStrategy.
func NewHumanReviewStrategy(logger logging.Logger) *HumanReviewStrategy {
	if logger == nil {
		logger = logging.MustGlobal()
	}
	return &HumanReviewStrategy{
		logger:        logger.With(logging.F("strategy", "human_review")),
		minConfidence: 0.0, // Always applicable as fallback
		supportedTypes: map[ConflictType]bool{
			ConflictTypeDuplicate:     true,
			ConflictTypeContradiction: true,
			ConflictTypeCycle:         true,
			ConflictTypeInconsistency: true,
		},
	}
}

// Name returns the strategy name.
func (s *HumanReviewStrategy) Name() string {
	return "human_review"
}

// CanResolve checks if this strategy can handle the conflict type.
func (s *HumanReviewStrategy) CanResolve(conflictType ConflictType) bool {
	return s.supportedTypes[conflictType]
}

// MinConfidenceForAuto returns the minimum confidence for automatic resolution.
// Human review never auto-resolves.
func (s *HumanReviewStrategy) MinConfidenceForAuto() float64 {
	return 2.0 // Never auto-resolve
}

// Resolve creates a resolution that flags the conflict for human review.
func (s *HumanReviewStrategy) Resolve(conflict *Conflict) (*Resolution, error) {
	if conflict == nil {
		return nil, fmt.Errorf("conflict is nil")
	}

	return &Resolution{
		Action:      ResolutionActionFlagForReview,
		Confidence:  1.0, // High confidence in the decision to request review
		Explanation: "Conflict queued for human review due to complexity or low auto-resolution confidence",
		Strategy:    s.Name(),
		AppliedAt:   time.Now(),
		AppliedBy:   "system:" + s.Name(),
		Metadata: map[string]string{
			"review_priority": s.calculatePriority(conflict),
			"conflict_type":   string(conflict.Type),
			"severity":        string(conflict.Severity),
		},
	}, nil
}

// ProposeOptions returns resolution options including human review.
func (s *HumanReviewStrategy) ProposeOptions(conflict *Conflict) ([]ResolutionOption, error) {
	if conflict == nil {
		return nil, fmt.Errorf("conflict is nil")
	}

	options := []ResolutionOption{
		{
			Resolution: &Resolution{
				Action:      ResolutionActionFlagForReview,
				Confidence:  1.0,
				Explanation: "Queue for expert human review",
				Strategy:    s.Name(),
				Metadata: map[string]string{
					"review_priority": s.calculatePriority(conflict),
				},
			},
			Score:       1.0,
			Pros:        []string{"Human expertise applied", "Best for complex cases", "No incorrect auto-resolution"},
			Cons:        []string{"Requires human time", "Resolution delayed"},
			Recommended: conflict.Severity == SeverityCritical || conflict.Severity == SeverityHigh,
		},
		{
			Resolution: &Resolution{
				Action:      ResolutionActionIgnore,
				Confidence:  0.5,
				Explanation: "Accept the conflict without changes",
				Strategy:    s.Name(),
				Metadata:    make(map[string]string),
			},
			Score:       0.3,
			Pros:        []string{"No changes required", "Fast resolution"},
			Cons:        []string{"Conflict persists", "Data quality not improved"},
			Recommended: false,
		},
	}

	return options, nil
}

// calculatePriority determines review priority based on conflict characteristics.
func (s *HumanReviewStrategy) calculatePriority(conflict *Conflict) string {
	switch conflict.Severity {
	case SeverityCritical:
		return "urgent"
	case SeverityHigh:
		return "high"
	case SeverityMedium:
		return "medium"
	default:
		return "low"
	}
}
