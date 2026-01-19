// Package conflict provides conflict detection and resolution for discovered relationships.
// It identifies duplicates, contradictions, cycles, and inconsistencies in the relationship
// graph and provides strategies for resolving them automatically or through human review.
package conflict

import (
	"time"

	"github.com/otherjamesbrown/penfold/services/relationship/extractor"
)

// ConflictType represents the category of conflict detected.
type ConflictType string

const (
	// ConflictTypeDuplicate indicates multiple relationships between the same entities
	// that may represent the same connection discovered from different sources.
	ConflictTypeDuplicate ConflictType = "duplicate"

	// ConflictTypeContradiction indicates relationships that express conflicting
	// information (e.g., "reports_to" vs "manages" between the same entities).
	ConflictTypeContradiction ConflictType = "contradiction"

	// ConflictTypeCycle indicates circular dependencies that may be invalid
	// (e.g., A reports_to B, B reports_to C, C reports_to A).
	ConflictTypeCycle ConflictType = "cycle"

	// ConflictTypeInconsistency indicates mismatches in relationship attributes
	// such as conflicting confidence scores, dates, or metadata.
	ConflictTypeInconsistency ConflictType = "inconsistency"
)

// String returns the string representation of the conflict type.
func (ct ConflictType) String() string {
	return string(ct)
}

// Severity represents the urgency level of a conflict.
type Severity string

const (
	// SeverityLow indicates a minor conflict that may not need immediate attention.
	SeverityLow Severity = "low"

	// SeverityMedium indicates a conflict that should be reviewed but is not critical.
	SeverityMedium Severity = "medium"

	// SeverityHigh indicates a significant conflict that could affect data quality.
	SeverityHigh Severity = "high"

	// SeverityCritical indicates a conflict that requires immediate attention.
	SeverityCritical Severity = "critical"
)

// String returns the string representation of the severity.
func (s Severity) String() string {
	return string(s)
}

// SeverityScore returns a numeric score for sorting (higher = more severe).
func (s Severity) SeverityScore() int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

// Conflict represents a detected conflict between relationships.
type Conflict struct {
	// ID is the unique identifier for this conflict.
	ID string

	// TenantID is the tenant this conflict belongs to.
	TenantID string

	// Type indicates the category of conflict.
	Type ConflictType

	// Severity indicates the urgency of resolution.
	Severity Severity

	// Relationships contains the conflicting relationships.
	Relationships []*extractor.Relationship

	// RelationshipIDs contains the IDs of the conflicting relationships.
	RelationshipIDs []string

	// Context provides additional information about the conflict.
	Context *ConflictContext

	// DetectedAt is when the conflict was detected.
	DetectedAt time.Time

	// ResolvedAt is when the conflict was resolved (nil if unresolved).
	ResolvedAt *time.Time

	// Resolution contains the applied resolution (nil if unresolved).
	Resolution *Resolution

	// Metadata contains additional key-value information.
	Metadata map[string]string
}

// IsResolved returns true if the conflict has been resolved.
func (c *Conflict) IsResolved() bool {
	return c.ResolvedAt != nil && c.Resolution != nil
}

// ConflictContext provides detailed context about a conflict.
type ConflictContext struct {
	// Description is a human-readable explanation of the conflict.
	Description string

	// SourceEntityID is the source entity involved.
	SourceEntityID string

	// TargetEntityID is the target entity involved.
	TargetEntityID string

	// SourceEntityName is the display name of the source entity.
	SourceEntityName string

	// TargetEntityName is the display name of the target entity.
	TargetEntityName string

	// RelationshipTypes lists the conflicting relationship types.
	RelationshipTypes []extractor.RelationshipType

	// ConfidenceRange shows the range of confidence scores.
	ConfidenceRange [2]float32

	// DateRange shows the range of discovery dates.
	DateRange [2]time.Time

	// CyclePath contains the entity IDs forming a cycle (for cycle conflicts).
	CyclePath []string

	// AttributeMismatches lists specific attribute conflicts (for inconsistencies).
	AttributeMismatches []AttributeMismatch
}

// AttributeMismatch describes a specific attribute conflict.
type AttributeMismatch struct {
	// Attribute is the name of the mismatched attribute.
	Attribute string

	// Values contains the conflicting values.
	Values []string

	// RelationshipIDs maps each value to its source relationship.
	RelationshipIDs map[string]string
}

// ResolutionAction specifies what action to take to resolve a conflict.
type ResolutionAction string

const (
	// ResolutionActionKeep indicates keeping one relationship and removing others.
	ResolutionActionKeep ResolutionAction = "keep"

	// ResolutionActionMerge indicates combining relationships into one.
	ResolutionActionMerge ResolutionAction = "merge"

	// ResolutionActionRemoveAll indicates removing all conflicting relationships.
	ResolutionActionRemoveAll ResolutionAction = "remove_all"

	// ResolutionActionFlagForReview indicates marking for human review.
	ResolutionActionFlagForReview ResolutionAction = "flag_for_review"

	// ResolutionActionIgnore indicates accepting the conflict without changes.
	ResolutionActionIgnore ResolutionAction = "ignore"

	// ResolutionActionSplit indicates splitting into separate relationships.
	ResolutionActionSplit ResolutionAction = "split"
)

// String returns the string representation of the action.
func (a ResolutionAction) String() string {
	return string(a)
}

// Resolution represents the result of resolving a conflict.
type Resolution struct {
	// Action is the resolution action taken.
	Action ResolutionAction

	// Confidence indicates how certain we are this resolution is correct (0.0 to 1.0).
	Confidence float64

	// Explanation provides a human-readable explanation of the resolution.
	Explanation string

	// Strategy is the name of the strategy that produced this resolution.
	Strategy string

	// KeptRelationshipID is the ID of the relationship to keep (for Keep action).
	KeptRelationshipID string

	// MergedRelationship is the resulting merged relationship (for Merge action).
	MergedRelationship *extractor.Relationship

	// RemovedRelationshipIDs are the IDs of relationships to remove.
	RemovedRelationshipIDs []string

	// AppliedAt is when the resolution was applied.
	AppliedAt time.Time

	// AppliedBy indicates who/what applied the resolution (system or user ID).
	AppliedBy string

	// Metadata contains additional resolution details.
	Metadata map[string]string
}

// ResolutionOption represents a possible resolution for a conflict.
type ResolutionOption struct {
	// Resolution is the proposed resolution.
	Resolution *Resolution

	// Score indicates how good this option is (higher is better).
	Score float64

	// Pros lists advantages of this resolution.
	Pros []string

	// Cons lists disadvantages of this resolution.
	Cons []string

	// Recommended indicates if this is the recommended option.
	Recommended bool
}

// Strategy defines the interface for conflict resolution strategies.
type Strategy interface {
	// Name returns the unique name of this strategy.
	Name() string

	// CanResolve checks if this strategy can handle the given conflict type.
	CanResolve(conflictType ConflictType) bool

	// Resolve attempts to resolve the conflict and returns a resolution.
	Resolve(conflict *Conflict) (*Resolution, error)

	// ProposeOptions returns multiple resolution options ranked by preference.
	ProposeOptions(conflict *Conflict) ([]ResolutionOption, error)

	// MinConfidenceForAuto returns the minimum confidence for automatic resolution.
	MinConfidenceForAuto() float64
}

// DetectorConfig holds configuration for conflict detectors.
type DetectorConfig struct {
	// DuplicateThreshold is the similarity threshold for duplicate detection (0.0 to 1.0).
	DuplicateThreshold float64

	// ContradictionPairs maps relationship types that are considered contradictory.
	ContradictionPairs map[extractor.RelationshipType][]extractor.RelationshipType

	// MaxCycleLength is the maximum cycle length to detect.
	MaxCycleLength int

	// InconsistencyAttributes lists attributes to check for inconsistencies.
	InconsistencyAttributes []string

	// MinConfidenceDiff is the minimum confidence difference to flag as inconsistent.
	MinConfidenceDiff float32
}

// DefaultDetectorConfig returns a DetectorConfig with sensible defaults.
func DefaultDetectorConfig() *DetectorConfig {
	return &DetectorConfig{
		DuplicateThreshold: 0.85,
		ContradictionPairs: map[extractor.RelationshipType][]extractor.RelationshipType{
			extractor.RelationshipTypeReportsTo: {extractor.RelationshipTypeManages},
			extractor.RelationshipTypeManages:   {extractor.RelationshipTypeReportsTo},
			extractor.RelationshipTypePartOf:    {extractor.RelationshipTypePartOf}, // Bidirectional part_of is a contradiction
		},
		MaxCycleLength: 10,
		InconsistencyAttributes: []string{
			"confidence",
			"strength",
			"context",
		},
		MinConfidenceDiff: 0.3,
	}
}

// ResolverConfig holds configuration for the ConflictResolver.
type ResolverConfig struct {
	// AutoResolveMinConfidence is the minimum confidence for automatic resolution.
	AutoResolveMinConfidence float64

	// DefaultStrategy is the default strategy to use if none specified.
	DefaultStrategy string

	// EnableHumanReview enables queueing conflicts for human review.
	EnableHumanReview bool

	// ReviewQueuePriority is the default priority for review queue items.
	ReviewQueuePriority string

	// MaxBatchSize is the maximum number of conflicts to process in a batch.
	MaxBatchSize int

	// DetectorConfig holds detector-specific configuration.
	DetectorConfig *DetectorConfig
}

// DefaultResolverConfig returns a ResolverConfig with sensible defaults.
func DefaultResolverConfig() *ResolverConfig {
	return &ResolverConfig{
		AutoResolveMinConfidence: 0.8,
		DefaultStrategy:          "high_confidence",
		EnableHumanReview:        true,
		ReviewQueuePriority:      "medium",
		MaxBatchSize:             100,
		DetectorConfig:           DefaultDetectorConfig(),
	}
}

// ConflictFilter specifies criteria for filtering conflicts.
type ConflictFilter struct {
	// Types filters by conflict types.
	Types []ConflictType

	// Severities filters by severity levels.
	Severities []Severity

	// TenantID filters by tenant.
	TenantID string

	// Resolved filters by resolution status (nil = all, true = resolved, false = unresolved).
	Resolved *bool

	// DetectedAfter filters conflicts detected after this time.
	DetectedAfter *time.Time

	// DetectedBefore filters conflicts detected before this time.
	DetectedBefore *time.Time

	// Limit is the maximum number of conflicts to return.
	Limit int

	// Offset is the number of conflicts to skip.
	Offset int
}

// ConflictStats provides statistics about conflicts.
type ConflictStats struct {
	// TenantID is the tenant these stats are for.
	TenantID string

	// TotalConflicts is the total number of conflicts.
	TotalConflicts int64

	// UnresolvedConflicts is the number of unresolved conflicts.
	UnresolvedConflicts int64

	// ResolvedConflicts is the number of resolved conflicts.
	ResolvedConflicts int64

	// ByType breaks down counts by conflict type.
	ByType map[ConflictType]int64

	// BySeverity breaks down counts by severity.
	BySeverity map[Severity]int64

	// AutoResolvedCount is conflicts resolved automatically.
	AutoResolvedCount int64

	// ManuallyResolvedCount is conflicts resolved by humans.
	ManuallyResolvedCount int64

	// AverageResolutionTime is the average time to resolve conflicts.
	AverageResolutionTime time.Duration
}
