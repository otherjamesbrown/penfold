// Package lifecycle provides relationship lifecycle management capabilities.
// It handles state transitions, validation, and history tracking for relationships
// as they move through their lifecycle from discovery to archival or merger.
package lifecycle

import (
	"time"
)

// State represents the lifecycle state of a relationship.
type State string

const (
	// StateProposed indicates a newly discovered relationship pending validation.
	StateProposed State = "proposed"

	// StateActive indicates a confirmed, active relationship.
	StateActive State = "active"

	// StateInactive indicates a relationship with no recent activity (decayed).
	StateInactive State = "inactive"

	// StateArchived indicates a soft-deleted relationship that can be restored.
	StateArchived State = "archived"

	// StateMerged indicates a relationship that was merged into another.
	StateMerged State = "merged"
)

// String returns the string representation of the state.
func (s State) String() string {
	return string(s)
}

// IsValid returns true if the state is a recognized lifecycle state.
func (s State) IsValid() bool {
	switch s {
	case StateProposed, StateActive, StateInactive, StateArchived, StateMerged:
		return true
	default:
		return false
	}
}

// IsTerminal returns true if the state is a terminal state (no further transitions).
func (s State) IsTerminal() bool {
	return s == StateMerged
}

// StateTransition represents a state change in the relationship lifecycle.
type StateTransition struct {
	// ID is a unique identifier for this transition.
	ID string

	// RelationshipID identifies the relationship that transitioned.
	RelationshipID string

	// TenantID is the tenant this transition belongs to.
	TenantID string

	// From is the previous state.
	From State

	// To is the new state.
	To State

	// Timestamp is when the transition occurred.
	Timestamp time.Time

	// Reason describes why the transition was made.
	Reason string

	// Actor is who or what triggered the transition (user ID, system, etc.).
	Actor string

	// Metadata contains additional context about the transition.
	Metadata map[string]string
}

// ValidTransitions defines the allowed state transitions.
// The map key is the source state, and the value is a slice of valid target states.
var ValidTransitions = map[State][]State{
	StateProposed: {
		StateActive,   // Confirmed by user or high confidence
		StateArchived, // Rejected by user
		StateMerged,   // Identified as duplicate
	},
	StateActive: {
		StateInactive, // Decay due to no recent activity
		StateArchived, // User archived or relationship ended
		StateMerged,   // Merged with another relationship
	},
	StateInactive: {
		StateActive,   // Reactivated due to new evidence
		StateArchived, // User archived
		StateMerged,   // Merged with another relationship
	},
	StateArchived: {
		StateActive,   // Restored by user
		StateProposed, // Restored for re-evaluation
		StateMerged,   // Merged during cleanup
	},
	StateMerged: {
		// Terminal state - no outgoing transitions
	},
}

// TransitionReason provides standard reasons for state transitions.
type TransitionReason string

const (
	// ReasonUserConfirmed indicates a user confirmed the relationship.
	ReasonUserConfirmed TransitionReason = "user_confirmed"

	// ReasonUserRejected indicates a user rejected the relationship.
	ReasonUserRejected TransitionReason = "user_rejected"

	// ReasonUserArchived indicates a user manually archived the relationship.
	ReasonUserArchived TransitionReason = "user_archived"

	// ReasonUserRestored indicates a user restored an archived relationship.
	ReasonUserRestored TransitionReason = "user_restored"

	// ReasonHighConfidence indicates automatic confirmation due to high confidence.
	ReasonHighConfidence TransitionReason = "high_confidence"

	// ReasonDecay indicates automatic inactivation due to lack of recent activity.
	ReasonDecay TransitionReason = "decay"

	// ReasonReactivated indicates reactivation due to new evidence.
	ReasonReactivated TransitionReason = "reactivated"

	// ReasonMerged indicates the relationship was merged with another.
	ReasonMerged TransitionReason = "merged"

	// ReasonDuplicate indicates the relationship was identified as a duplicate.
	ReasonDuplicate TransitionReason = "duplicate"

	// ReasonSystemCleanup indicates automatic cleanup by the system.
	ReasonSystemCleanup TransitionReason = "system_cleanup"
)

// String returns the string representation of the reason.
func (r TransitionReason) String() string {
	return string(r)
}

// TransitionHistory maintains a history of state transitions for a relationship.
type TransitionHistory struct {
	// RelationshipID identifies the relationship.
	RelationshipID string

	// TenantID is the tenant this history belongs to.
	TenantID string

	// CurrentState is the current state of the relationship.
	CurrentState State

	// Transitions is the ordered list of transitions (oldest first).
	Transitions []StateTransition

	// CreatedAt is when the relationship was first created.
	CreatedAt time.Time

	// LastTransitionAt is when the last transition occurred.
	LastTransitionAt time.Time
}

// AddTransition appends a transition to the history.
func (h *TransitionHistory) AddTransition(t StateTransition) {
	h.Transitions = append(h.Transitions, t)
	h.CurrentState = t.To
	h.LastTransitionAt = t.Timestamp
}

// GetLatestTransition returns the most recent transition, or nil if none.
func (h *TransitionHistory) GetLatestTransition() *StateTransition {
	if len(h.Transitions) == 0 {
		return nil
	}
	return &h.Transitions[len(h.Transitions)-1]
}

// GetTransitionCount returns the number of transitions in the history.
func (h *TransitionHistory) GetTransitionCount() int {
	return len(h.Transitions)
}

// GetTransitionsByState returns all transitions that resulted in the given state.
func (h *TransitionHistory) GetTransitionsByState(state State) []StateTransition {
	var result []StateTransition
	for _, t := range h.Transitions {
		if t.To == state {
			result = append(result, t)
		}
	}
	return result
}

// MergeInfo contains information about a merge operation.
type MergeInfo struct {
	// SourceID is the relationship that was merged (the one being removed).
	SourceID string

	// TargetID is the relationship that absorbed the source.
	TargetID string

	// MergedAt is when the merge occurred.
	MergedAt time.Time

	// Reason describes why the merge was performed.
	Reason string

	// Actor is who or what triggered the merge.
	Actor string

	// EvidenceTransferred indicates how many evidence items were transferred.
	EvidenceTransferred int

	// ConfidenceAdjustment indicates how the target's confidence was adjusted.
	ConfidenceAdjustment float32
}

// ArchiveInfo contains information about an archive operation.
type ArchiveInfo struct {
	// RelationshipID is the archived relationship.
	RelationshipID string

	// ArchivedAt is when the archive occurred.
	ArchivedAt time.Time

	// Reason describes why the relationship was archived.
	Reason string

	// Actor is who or what triggered the archive.
	Actor string

	// RestoredAt is when the relationship was restored, if applicable.
	RestoredAt *time.Time

	// RestoredBy is who restored the relationship, if applicable.
	RestoredBy *string
}

// DecayInfo contains information about relationship decay detection.
type DecayInfo struct {
	// RelationshipID is the decaying relationship.
	RelationshipID string

	// LastActivityAt is the timestamp of the last detected activity.
	LastActivityAt time.Time

	// DaysSinceActivity is the number of days since last activity.
	DaysSinceActivity int

	// ConfidenceDecay is the amount the confidence has decayed.
	ConfidenceDecay float32

	// ShouldTransition indicates if the relationship should transition to inactive.
	ShouldTransition bool

	// DetectedAt is when the decay was detected.
	DetectedAt time.Time
}
