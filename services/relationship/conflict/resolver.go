// Package conflict provides conflict detection and resolution for discovered relationships.
package conflict

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/relationship/extractor"
)

// ConflictResolver provides conflict detection and resolution capabilities.
// It coordinates between detectors and strategies to identify and resolve
// relationship conflicts.
type ConflictResolver struct {
	config     *ResolverConfig
	detectors  *DetectorRegistry
	strategies *StrategyRegistry
	queue      ReviewQueueIntegration
	logger     logging.Logger
	mu         sync.RWMutex

	// Cache of detected conflicts
	conflicts     map[string]*Conflict
	conflictsByID map[string]*Conflict

	// Resolution history for tracking
	history []ResolutionHistory
}

// ResolutionHistory tracks a resolution event.
type ResolutionHistory struct {
	ConflictID  string
	Resolution  *Resolution
	ResolvedAt  time.Time
	WasAutomatic bool
}

// ReviewQueueIntegration defines the interface for review queue integration.
type ReviewQueueIntegration interface {
	// QueueForReview adds a conflict to the review queue.
	QueueForReview(ctx context.Context, conflict *Conflict) error

	// GetPendingConflicts retrieves pending conflicts for a tenant.
	GetPendingConflicts(ctx context.Context, tenantID string, limit int) ([]*Conflict, error)

	// ApplyResolution applies a resolution to a queued conflict.
	ApplyResolution(ctx context.Context, conflictID string, resolution *Resolution) error

	// RemoveFromQueue removes a conflict from the queue.
	RemoveFromQueue(ctx context.Context, conflictID string) error
}

// NewConflictResolver creates a new ConflictResolver.
func NewConflictResolver(config *ResolverConfig, logger logging.Logger) *ConflictResolver {
	if config == nil {
		config = DefaultResolverConfig()
	}
	if logger == nil {
		logger = &noopLogger{}
	}

	resolver := &ConflictResolver{
		config:        config,
		detectors:     NewDetectorRegistry(config.DetectorConfig, logger),
		strategies:    NewStrategyRegistry(logger),
		logger:        logger.With(logging.F("component", "conflict_resolver")),
		conflicts:     make(map[string]*Conflict),
		conflictsByID: make(map[string]*Conflict),
		history:       make([]ResolutionHistory, 0),
	}

	// Register default detectors and strategies
	resolver.detectors.RegisterDefaults()
	resolver.strategies.RegisterDefaults()

	return resolver
}

// SetReviewQueue sets the review queue integration.
func (r *ConflictResolver) SetReviewQueue(queue ReviewQueueIntegration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = queue
}

// DetectConflicts analyzes relationships and returns detected conflicts.
func (r *ConflictResolver) DetectConflicts(relationships []*extractor.Relationship) []*Conflict {
	r.logger.Info("Detecting conflicts",
		logging.F("relationship_count", len(relationships)),
	)

	conflicts := r.detectors.DetectAll(relationships)

	// Update internal cache
	r.mu.Lock()
	for _, conflict := range conflicts {
		r.conflictsByID[conflict.ID] = conflict
		key := r.conflictKey(conflict)
		r.conflicts[key] = conflict
	}
	r.mu.Unlock()

	r.logger.Info("Conflict detection complete",
		logging.F("conflicts_found", len(conflicts)),
	)

	return conflicts
}

// Resolve attempts to resolve a conflict using the specified strategy.
func (r *ConflictResolver) Resolve(conflict *Conflict, strategyName string) (*Resolution, error) {
	if conflict == nil {
		return nil, fmt.Errorf("conflict is nil")
	}

	r.logger.Debug("Resolving conflict",
		logging.F("conflict_id", conflict.ID),
		logging.F("type", conflict.Type),
		logging.F("strategy", strategyName),
	)

	strategy, ok := r.strategies.Get(strategyName)
	if !ok {
		return nil, fmt.Errorf("strategy not found: %s", strategyName)
	}

	if !strategy.CanResolve(conflict.Type) {
		return nil, fmt.Errorf("strategy '%s' cannot resolve conflict type: %s", strategyName, conflict.Type)
	}

	resolution, err := strategy.Resolve(conflict)
	if err != nil {
		return nil, fmt.Errorf("resolution failed: %w", err)
	}

	// Update conflict state
	r.mu.Lock()
	now := time.Now()
	conflict.ResolvedAt = &now
	conflict.Resolution = resolution
	r.history = append(r.history, ResolutionHistory{
		ConflictID:   conflict.ID,
		Resolution:   resolution,
		ResolvedAt:   now,
		WasAutomatic: false,
	})
	r.mu.Unlock()

	r.logger.Info("Conflict resolved",
		logging.F("conflict_id", conflict.ID),
		logging.F("action", resolution.Action),
		logging.F("confidence", resolution.Confidence),
	)

	return resolution, nil
}

// AutoResolve attempts automatic resolution using the best strategy.
func (r *ConflictResolver) AutoResolve(conflict *Conflict) (*Resolution, error) {
	if conflict == nil {
		return nil, fmt.Errorf("conflict is nil")
	}

	r.logger.Debug("Attempting auto-resolution",
		logging.F("conflict_id", conflict.ID),
		logging.F("type", conflict.Type),
	)

	// Get the best strategy for this conflict type
	strategy := r.strategies.GetBestStrategy(conflict.Type)
	if strategy == nil {
		return nil, fmt.Errorf("no suitable strategy found for conflict type: %s", conflict.Type)
	}

	// Attempt resolution
	resolution, err := strategy.Resolve(conflict)
	if err != nil {
		return nil, fmt.Errorf("auto-resolution failed: %w", err)
	}

	// Check if confidence meets threshold for auto-resolution
	if resolution.Confidence < r.config.AutoResolveMinConfidence {
		r.logger.Debug("Auto-resolution confidence too low",
			logging.F("conflict_id", conflict.ID),
			logging.F("confidence", resolution.Confidence),
			logging.F("threshold", r.config.AutoResolveMinConfidence),
		)

		// Flag for human review if configured
		if r.config.EnableHumanReview && resolution.Action != ResolutionActionFlagForReview {
			resolution.Action = ResolutionActionFlagForReview
			resolution.Explanation = fmt.Sprintf(
				"Auto-resolution confidence (%.2f) below threshold (%.2f). %s",
				resolution.Confidence, r.config.AutoResolveMinConfidence, resolution.Explanation,
			)
		}
	}

	// Update conflict state
	r.mu.Lock()
	now := time.Now()
	conflict.ResolvedAt = &now
	conflict.Resolution = resolution
	r.history = append(r.history, ResolutionHistory{
		ConflictID:   conflict.ID,
		Resolution:   resolution,
		ResolvedAt:   now,
		WasAutomatic: true,
	})
	r.mu.Unlock()

	r.logger.Info("Auto-resolution complete",
		logging.F("conflict_id", conflict.ID),
		logging.F("action", resolution.Action),
		logging.F("confidence", resolution.Confidence),
		logging.F("strategy", strategy.Name()),
	)

	return resolution, nil
}

// ProposeResolutions generates resolution options for a conflict.
func (r *ConflictResolver) ProposeResolutions(conflict *Conflict) ([]ResolutionOption, error) {
	if conflict == nil {
		return nil, fmt.Errorf("conflict is nil")
	}

	r.logger.Debug("Proposing resolutions",
		logging.F("conflict_id", conflict.ID),
		logging.F("type", conflict.Type),
	)

	allOptions := make([]ResolutionOption, 0)

	// Get options from all applicable strategies
	for _, name := range r.strategies.List() {
		strategy, _ := r.strategies.Get(name)
		if !strategy.CanResolve(conflict.Type) {
			continue
		}

		options, err := strategy.ProposeOptions(conflict)
		if err != nil {
			r.logger.Warn("Strategy failed to propose options",
				logging.F("strategy", name),
				logging.Err(err),
			)
			continue
		}

		allOptions = append(allOptions, options...)
	}

	// Sort by score (highest first)
	sort.Slice(allOptions, func(i, j int) bool {
		return allOptions[i].Score > allOptions[j].Score
	})

	// Mark the top option as recommended if none is marked
	hasRecommended := false
	for _, opt := range allOptions {
		if opt.Recommended {
			hasRecommended = true
			break
		}
	}
	if !hasRecommended && len(allOptions) > 0 {
		allOptions[0].Recommended = true
	}

	r.logger.Debug("Resolutions proposed",
		logging.F("conflict_id", conflict.ID),
		logging.F("options_count", len(allOptions)),
	)

	return allOptions, nil
}

// BatchDetect processes relationships in batches for memory efficiency.
func (r *ConflictResolver) BatchDetect(ctx context.Context, relationships []*extractor.Relationship) ([]*Conflict, error) {
	batchSize := r.config.MaxBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	allConflicts := make([]*Conflict, 0)

	for i := 0; i < len(relationships); i += batchSize {
		select {
		case <-ctx.Done():
			return allConflicts, ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(relationships) {
			end = len(relationships)
		}

		batch := relationships[i:end]
		conflicts := r.DetectConflicts(batch)
		allConflicts = append(allConflicts, conflicts...)
	}

	return allConflicts, nil
}

// BatchAutoResolve attempts automatic resolution on multiple conflicts.
func (r *ConflictResolver) BatchAutoResolve(ctx context.Context, conflicts []*Conflict) ([]ResolutionResult, error) {
	results := make([]ResolutionResult, 0, len(conflicts))

	for _, conflict := range conflicts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		resolution, err := r.AutoResolve(conflict)

		result := ResolutionResult{
			ConflictID: conflict.ID,
			Resolution: resolution,
			Error:      err,
		}

		if err == nil && resolution.Action == ResolutionActionFlagForReview {
			// Queue for human review if configured
			if r.queue != nil && r.config.EnableHumanReview {
				if qErr := r.queue.QueueForReview(ctx, conflict); qErr != nil {
					r.logger.Warn("Failed to queue conflict for review",
						logging.F("conflict_id", conflict.ID),
						logging.Err(qErr),
					)
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// ResolutionResult holds the result of a batch resolution operation.
type ResolutionResult struct {
	ConflictID string
	Resolution *Resolution
	Error      error
}

// GetConflict retrieves a conflict by ID.
func (r *ConflictResolver) GetConflict(id string) (*Conflict, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conflict, ok := r.conflictsByID[id]
	return conflict, ok
}

// GetConflicts returns conflicts matching the filter.
func (r *ConflictResolver) GetConflicts(filter *ConflictFilter) []*Conflict {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]*Conflict, 0)

	for _, conflict := range r.conflictsByID {
		if r.matchesFilter(conflict, filter) {
			results = append(results, conflict)
		}
	}

	// Apply sorting (by severity, then detection time)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Severity.SeverityScore() != results[j].Severity.SeverityScore() {
			return results[i].Severity.SeverityScore() > results[j].Severity.SeverityScore()
		}
		return results[i].DetectedAt.Before(results[j].DetectedAt)
	})

	// Apply limit and offset
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(results) {
			results = results[filter.Offset:]
		} else if filter.Offset >= len(results) {
			return []*Conflict{}
		}

		if filter.Limit > 0 && filter.Limit < len(results) {
			results = results[:filter.Limit]
		}
	}

	return results
}

// matchesFilter checks if a conflict matches the filter criteria.
func (r *ConflictResolver) matchesFilter(conflict *Conflict, filter *ConflictFilter) bool {
	if filter == nil {
		return true
	}

	// Filter by types
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if conflict.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by severities
	if len(filter.Severities) > 0 {
		found := false
		for _, s := range filter.Severities {
			if conflict.Severity == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by tenant
	if filter.TenantID != "" && conflict.TenantID != filter.TenantID {
		return false
	}

	// Filter by resolved status
	if filter.Resolved != nil {
		isResolved := conflict.IsResolved()
		if *filter.Resolved != isResolved {
			return false
		}
	}

	// Filter by detection time
	if filter.DetectedAfter != nil && conflict.DetectedAt.Before(*filter.DetectedAfter) {
		return false
	}
	if filter.DetectedBefore != nil && conflict.DetectedAt.After(*filter.DetectedBefore) {
		return false
	}

	return true
}

// GetStats returns conflict statistics.
func (r *ConflictResolver) GetStats(tenantID string) *ConflictStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &ConflictStats{
		TenantID:   tenantID,
		ByType:     make(map[ConflictType]int64),
		BySeverity: make(map[Severity]int64),
	}

	var totalResolutionTime time.Duration
	resolvedCount := 0

	for _, conflict := range r.conflictsByID {
		if tenantID != "" && conflict.TenantID != tenantID {
			continue
		}

		stats.TotalConflicts++
		stats.ByType[conflict.Type]++
		stats.BySeverity[conflict.Severity]++

		if conflict.IsResolved() {
			stats.ResolvedConflicts++
			if conflict.Resolution.AppliedBy != "" && conflict.Resolution.AppliedBy[:7] == "system:" {
				stats.AutoResolvedCount++
			} else {
				stats.ManuallyResolvedCount++
			}

			// Calculate resolution time
			if conflict.ResolvedAt != nil {
				duration := conflict.ResolvedAt.Sub(conflict.DetectedAt)
				totalResolutionTime += duration
				resolvedCount++
			}
		} else {
			stats.UnresolvedConflicts++
		}
	}

	if resolvedCount > 0 {
		stats.AverageResolutionTime = totalResolutionTime / time.Duration(resolvedCount)
	}

	return stats
}

// GetResolutionHistory returns the resolution history.
func (r *ConflictResolver) GetResolutionHistory(limit int) []ResolutionHistory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > len(r.history) {
		limit = len(r.history)
	}

	// Return most recent first
	result := make([]ResolutionHistory, limit)
	for i := 0; i < limit; i++ {
		result[i] = r.history[len(r.history)-1-i]
	}

	return result
}

// ClearResolved removes resolved conflicts from the cache.
func (r *ConflictResolver) ClearResolved() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cleared := 0
	for key, conflict := range r.conflicts {
		if conflict.IsResolved() {
			delete(r.conflicts, key)
			delete(r.conflictsByID, conflict.ID)
			cleared++
		}
	}

	r.logger.Info("Cleared resolved conflicts",
		logging.F("count", cleared),
	)

	return cleared
}

// RegisterDetector adds a custom detector.
func (r *ConflictResolver) RegisterDetector(detector Detector) {
	r.detectors.Register(detector)
}

// RegisterStrategy adds a custom strategy.
func (r *ConflictResolver) RegisterStrategy(strategy Strategy) {
	r.strategies.Register(strategy)
}

// conflictKey generates a unique key for deduplicating conflicts.
func (r *ConflictResolver) conflictKey(conflict *Conflict) string {
	if len(conflict.RelationshipIDs) > 0 {
		// Use sorted relationship IDs
		ids := make([]string, len(conflict.RelationshipIDs))
		copy(ids, conflict.RelationshipIDs)
		sort.Strings(ids)
		return fmt.Sprintf("%s:%s", conflict.Type, ids)
	}
	return conflict.ID
}
