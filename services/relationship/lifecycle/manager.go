// Package lifecycle provides relationship lifecycle management capabilities.
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RelationshipStore provides persistence operations for relationships.
type RelationshipStore interface {
	// Get retrieves a relationship by ID.
	Get(ctx context.Context, tenantID, id string) (*relationshipv1.Relationship, error)

	// Save persists a relationship.
	Save(ctx context.Context, rel *relationshipv1.Relationship) error

	// Delete removes a relationship (hard delete).
	Delete(ctx context.Context, tenantID, id string) error

	// List returns relationships matching the given criteria.
	List(ctx context.Context, tenantID string, filter *ListFilter) ([]*relationshipv1.Relationship, error)
}

// ListFilter specifies filtering criteria for listing relationships.
type ListFilter struct {
	// Status filters by relationship status.
	Status *relationshipv1.RelationshipStatus

	// RelationshipType filters by relationship type.
	RelationshipType *relationshipv1.RelationshipType

	// EntityID filters relationships involving a specific entity.
	EntityID *string

	// MinConfidence filters by minimum confidence.
	MinConfidence *float32

	// UpdatedAfter filters relationships updated after this time.
	UpdatedAfter *time.Time

	// UpdatedBefore filters relationships updated before this time.
	UpdatedBefore *time.Time

	// Limit is the maximum number of results to return.
	Limit int

	// Offset is the number of results to skip.
	Offset int
}

// ManagerConfig configures the lifecycle manager.
type ManagerConfig struct {
	// Logger is used for logging.
	Logger logging.Logger

	// Store is the relationship persistence layer.
	Store RelationshipStore

	// EventPublisher publishes lifecycle events.
	EventPublisher EventPublisher

	// Validator validates lifecycle operations.
	Validator *Validator

	// StateMachine manages state transitions.
	StateMachine *StateMachine

	// DecayDetector detects inactive relationships.
	DecayDetector *DecayDetector

	// DefaultActor is the default actor for system operations.
	DefaultActor string

	// AutoActivateConfidence is the confidence threshold for automatic activation.
	AutoActivateConfidence float32
}

// Manager provides lifecycle management for relationships.
type Manager struct {
	mu             sync.RWMutex
	logger         logging.Logger
	store          RelationshipStore
	eventPublisher EventPublisher
	validator      *Validator
	stateMachine   *StateMachine
	decayDetector  *DecayDetector
	eventFactory   *EventFactory
	now            func() time.Time

	autoActivateConfidence float32
}

// NewManager creates a new lifecycle Manager.
func NewManager(cfg *ManagerConfig) *Manager {
	if cfg == nil {
		cfg = &ManagerConfig{}
	}

	m := &Manager{
		logger:                 cfg.Logger,
		store:                  cfg.Store,
		eventPublisher:         cfg.EventPublisher,
		validator:              cfg.Validator,
		stateMachine:           cfg.StateMachine,
		decayDetector:          cfg.DecayDetector,
		autoActivateConfidence: cfg.AutoActivateConfidence,
		now:                    time.Now,
	}

	// Set defaults
	if m.logger == nil {
		m.logger = logging.MustGlobal()
	}

	if m.validator == nil {
		m.validator = DefaultValidator()
	}

	if m.stateMachine == nil {
		m.stateMachine = NewStateMachine(&StateMachineConfig{Logger: m.logger})
	}

	if m.decayDetector == nil {
		m.decayDetector = DefaultDecayDetector()
	}

	if m.autoActivateConfidence <= 0 {
		m.autoActivateConfidence = 0.8
	}

	defaultActor := cfg.DefaultActor
	if defaultActor == "" {
		defaultActor = "system"
	}
	m.eventFactory = NewEventFactory(defaultActor)

	return m
}

// Create creates a new relationship in the proposed state.
func (m *Manager) Create(ctx context.Context, rel *relationshipv1.Relationship) error {
	if rel == nil {
		return fmt.Errorf("relationship cannot be nil")
	}

	m.logger.Info("Creating relationship",
		logging.F("tenant_id", rel.TenantId),
	)

	// Validate the relationship
	result := m.validator.ValidateCreate(ctx, rel)
	if !result.Valid {
		return result.ToError()
	}

	// Log warnings
	for _, warning := range result.Warnings {
		m.logger.Warn("Create validation warning",
			logging.F("warning", warning),
		)
	}

	// Set defaults
	now := timestamppb.New(m.now())
	if rel.Id == "" {
		rel.Id = uuid.New().String()
	}
	if rel.CreatedAt == nil {
		rel.CreatedAt = now
	}
	rel.UpdatedAt = now

	// Determine initial state based on confidence
	if rel.Confidence >= m.autoActivateConfidence {
		rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED
		m.logger.Info("Auto-activating high confidence relationship",
			logging.F("relationship_id", rel.Id),
			logging.F("confidence", rel.Confidence),
		)
	} else {
		rel.Status = relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED
	}

	// Persist the relationship
	if m.store != nil {
		if err := m.store.Save(ctx, rel); err != nil {
			return fmt.Errorf("failed to save relationship: %w", err)
		}
	}

	// Publish created event
	if m.eventPublisher != nil {
		event := m.eventFactory.CreatedEvent(rel, "")
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish created event",
				logging.F("relationship_id", rel.Id),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationship created",
		logging.F("relationship_id", rel.Id),
		logging.F("tenant_id", rel.TenantId),
		logging.F("status", rel.Status.String()),
	)

	return nil
}

// Update updates an existing relationship.
func (m *Manager) Update(ctx context.Context, tenantID, id string, update *RelationshipUpdate) error {
	if update == nil {
		return fmt.Errorf("update cannot be nil")
	}

	m.logger.Info("Updating relationship",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	// Get existing relationship
	var rel *relationshipv1.Relationship
	if m.store != nil {
		var err error
		rel, err = m.store.Get(ctx, tenantID, id)
		if err != nil {
			return fmt.Errorf("failed to get relationship: %w", err)
		}
	} else {
		return fmt.Errorf("no store configured")
	}

	// Validate update
	result := m.validator.ValidateUpdate(ctx, rel, update)
	if !result.Valid {
		return result.ToError()
	}

	// Track changes for event
	changes := make(map[string]interface{})

	// Apply updates
	if update.Confidence != nil {
		changes["old_confidence"] = rel.Confidence
		changes["new_confidence"] = *update.Confidence
		rel.Confidence = *update.Confidence
	}

	if update.Notes != nil {
		rel.Notes = update.Notes
	}

	// Handle status change
	if update.Status != nil && *update.Status != rel.Status {
		oldState := statusToState(rel.Status)
		newState := statusToState(*update.Status)

		// Use state machine for transition
		_, err := m.stateMachine.ExecuteTransition(ctx, rel, newState, "user_update", "")
		if err != nil {
			return fmt.Errorf("failed to transition state: %w", err)
		}

		changes["old_status"] = oldState.String()
		changes["new_status"] = newState.String()
	}

	// Add new evidence
	if len(update.AddEvidence) > 0 {
		rel.Evidence = append(rel.Evidence, update.AddEvidence...)
		changes["evidence_added"] = len(update.AddEvidence)
	}

	// Remove evidence
	if len(update.RemoveEvidenceIDs) > 0 {
		rel.Evidence = filterEvidence(rel.Evidence, update.RemoveEvidenceIDs)
		changes["evidence_removed"] = len(update.RemoveEvidenceIDs)
	}

	// Update timestamp
	rel.UpdatedAt = timestamppb.New(m.now())

	// Persist changes
	if err := m.store.Save(ctx, rel); err != nil {
		return fmt.Errorf("failed to save relationship: %w", err)
	}

	// Publish updated event
	if m.eventPublisher != nil {
		event := m.eventFactory.UpdatedEvent(rel, "", changes)
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish updated event",
				logging.F("relationship_id", id),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationship updated",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	return nil
}

// Merge merges the source relationship into the target relationship.
func (m *Manager) Merge(ctx context.Context, tenantID, sourceID, targetID string) error {
	m.logger.Info("Merging relationships",
		logging.F("source_id", sourceID),
		logging.F("target_id", targetID),
		logging.F("tenant_id", tenantID),
	)

	if m.store == nil {
		return fmt.Errorf("no store configured")
	}

	// Get both relationships
	source, err := m.store.Get(ctx, tenantID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source relationship: %w", err)
	}

	target, err := m.store.Get(ctx, tenantID, targetID)
	if err != nil {
		return fmt.Errorf("failed to get target relationship: %w", err)
	}

	// Validate merge
	result := m.validator.ValidateMerge(ctx, source, target)
	if !result.Valid {
		return result.ToError()
	}

	// Transfer evidence from source to target
	evidenceTransferred := len(source.Evidence)
	target.Evidence = append(target.Evidence, source.Evidence...)

	// Combine confidence (take maximum or aggregate)
	if source.Confidence > target.Confidence {
		target.Confidence = source.Confidence
	}

	// Update target timestamp
	target.UpdatedAt = timestamppb.New(m.now())

	// Save target with merged data
	if err := m.store.Save(ctx, target); err != nil {
		return fmt.Errorf("failed to save merged target: %w", err)
	}

	// Transition source to merged state
	_, err = m.stateMachine.ExecuteTransition(ctx, source, StateMerged, string(ReasonMerged), "")
	if err != nil {
		return fmt.Errorf("failed to transition source to merged: %w", err)
	}

	// Mark source with merge info in notes
	mergeNote := fmt.Sprintf("Merged into relationship %s at %s", targetID, m.now().Format(time.RFC3339))
	source.Notes = &mergeNote
	source.UpdatedAt = timestamppb.New(m.now())

	if err := m.store.Save(ctx, source); err != nil {
		return fmt.Errorf("failed to save merged source: %w", err)
	}

	// Publish merge event
	if m.eventPublisher != nil {
		event := m.eventFactory.MergedEvent(sourceID, targetID, tenantID, "manual_merge", "", evidenceTransferred)
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish merge event",
				logging.F("source_id", sourceID),
				logging.F("target_id", targetID),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationships merged",
		logging.F("source_id", sourceID),
		logging.F("target_id", targetID),
		logging.F("evidence_transferred", evidenceTransferred),
	)

	return nil
}

// Archive soft-deletes a relationship.
func (m *Manager) Archive(ctx context.Context, tenantID, id, reason string) error {
	m.logger.Info("Archiving relationship",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
		logging.F("reason", reason),
	)

	if m.store == nil {
		return fmt.Errorf("no store configured")
	}

	// Get the relationship
	rel, err := m.store.Get(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get relationship: %w", err)
	}

	// Validate archive
	result := m.validator.ValidateArchive(ctx, rel, reason)
	if !result.Valid {
		return result.ToError()
	}

	// Transition to archived state
	_, err = m.stateMachine.ExecuteTransition(ctx, rel, StateArchived, reason, "")
	if err != nil {
		return fmt.Errorf("failed to transition to archived: %w", err)
	}

	// Update timestamp
	rel.UpdatedAt = timestamppb.New(m.now())

	// Persist changes
	if err := m.store.Save(ctx, rel); err != nil {
		return fmt.Errorf("failed to save archived relationship: %w", err)
	}

	// Publish archived event
	if m.eventPublisher != nil {
		event := m.eventFactory.ArchivedEvent(rel, reason, "")
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish archived event",
				logging.F("relationship_id", id),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationship archived",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	return nil
}

// Restore unarchives a relationship.
func (m *Manager) Restore(ctx context.Context, tenantID, id string) error {
	m.logger.Info("Restoring relationship",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	if m.store == nil {
		return fmt.Errorf("no store configured")
	}

	// Get the relationship
	rel, err := m.store.Get(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get relationship: %w", err)
	}

	// Validate restore
	result := m.validator.ValidateRestore(ctx, rel)
	if !result.Valid {
		return result.ToError()
	}

	// Determine target state (back to active or proposed based on confidence)
	targetState := StateActive
	if rel.Confidence < m.autoActivateConfidence {
		targetState = StateProposed
	}

	// Transition to target state
	_, err = m.stateMachine.ExecuteTransition(ctx, rel, targetState, string(ReasonUserRestored), "")
	if err != nil {
		return fmt.Errorf("failed to transition to %s: %w", targetState, err)
	}

	// Update timestamp
	rel.UpdatedAt = timestamppb.New(m.now())

	// Persist changes
	if err := m.store.Save(ctx, rel); err != nil {
		return fmt.Errorf("failed to save restored relationship: %w", err)
	}

	// Publish restored event
	if m.eventPublisher != nil {
		event := m.eventFactory.RestoredEvent(rel, "")
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish restored event",
				logging.F("relationship_id", id),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationship restored",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
		logging.F("new_state", targetState.String()),
	)

	return nil
}

// Confirm confirms a proposed relationship.
func (m *Manager) Confirm(ctx context.Context, tenantID, id, actor string) error {
	m.logger.Info("Confirming relationship",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	if m.store == nil {
		return fmt.Errorf("no store configured")
	}

	// Get the relationship
	rel, err := m.store.Get(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get relationship: %w", err)
	}

	// Transition to active state
	_, err = m.stateMachine.ExecuteTransition(ctx, rel, StateActive, string(ReasonUserConfirmed), actor)
	if err != nil {
		return fmt.Errorf("failed to transition to active: %w", err)
	}

	// Update timestamp
	rel.UpdatedAt = timestamppb.New(m.now())

	// Persist changes
	if err := m.store.Save(ctx, rel); err != nil {
		return fmt.Errorf("failed to save confirmed relationship: %w", err)
	}

	// Publish confirmed event
	if m.eventPublisher != nil {
		event := m.eventFactory.ConfirmedEvent(rel, actor)
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish confirmed event",
				logging.F("relationship_id", id),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationship confirmed",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	return nil
}

// Reject rejects a proposed relationship.
func (m *Manager) Reject(ctx context.Context, tenantID, id, reason, actor string) error {
	m.logger.Info("Rejecting relationship",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
		logging.F("reason", reason),
	)

	if m.store == nil {
		return fmt.Errorf("no store configured")
	}

	// Get the relationship
	rel, err := m.store.Get(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("failed to get relationship: %w", err)
	}

	// Transition to archived state
	_, err = m.stateMachine.ExecuteTransition(ctx, rel, StateArchived, string(ReasonUserRejected), actor)
	if err != nil {
		return fmt.Errorf("failed to transition to archived: %w", err)
	}

	// Update timestamp
	rel.UpdatedAt = timestamppb.New(m.now())

	// Persist changes
	if err := m.store.Save(ctx, rel); err != nil {
		return fmt.Errorf("failed to save rejected relationship: %w", err)
	}

	// Publish rejected event
	if m.eventPublisher != nil {
		event := m.eventFactory.RejectedEvent(rel, reason, actor)
		if err := m.eventPublisher.Publish(ctx, event); err != nil {
			m.logger.Error("Failed to publish rejected event",
				logging.F("relationship_id", id),
				logging.Err(err),
			)
		}
	}

	m.logger.Info("Relationship rejected",
		logging.F("relationship_id", id),
		logging.F("tenant_id", tenantID),
	)

	return nil
}

// DetectDecayedRelationships finds relationships that have become inactive.
func (m *Manager) DetectDecayedRelationships(ctx context.Context, tenantID string) ([]*DecayInfo, error) {
	if m.store == nil {
		return nil, fmt.Errorf("no store configured")
	}

	// List active relationships
	activeStatus := relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED
	filter := &ListFilter{
		Status: &activeStatus,
	}

	relationships, err := m.store.List(ctx, tenantID, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list relationships: %w", err)
	}

	// Check each for decay
	decayed := m.decayDetector.BatchCheckDecay(relationships)

	m.logger.Info("Decay detection completed",
		logging.F("tenant_id", tenantID),
		logging.F("total_checked", len(relationships)),
		logging.F("decayed_count", len(decayed)),
	)

	return decayed, nil
}

// ProcessDecayedRelationships transitions decayed relationships to inactive.
func (m *Manager) ProcessDecayedRelationships(ctx context.Context, tenantID string) (int, error) {
	decayed, err := m.DetectDecayedRelationships(ctx, tenantID)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, info := range decayed {
		rel, err := m.store.Get(ctx, tenantID, info.RelationshipID)
		if err != nil {
			m.logger.Error("Failed to get decayed relationship",
				logging.F("relationship_id", info.RelationshipID),
				logging.Err(err),
			)
			continue
		}

		// Transition to inactive
		_, err = m.stateMachine.ExecuteTransition(ctx, rel, StateInactive, string(ReasonDecay), "system")
		if err != nil {
			m.logger.Error("Failed to transition decayed relationship",
				logging.F("relationship_id", info.RelationshipID),
				logging.Err(err),
			)
			continue
		}

		// Update and save
		rel.UpdatedAt = timestamppb.New(m.now())
		if err := m.store.Save(ctx, rel); err != nil {
			m.logger.Error("Failed to save decayed relationship",
				logging.F("relationship_id", info.RelationshipID),
				logging.Err(err),
			)
			continue
		}

		// Publish decay event
		if m.eventPublisher != nil {
			event := m.eventFactory.DecayedEvent(rel, info)
			if err := m.eventPublisher.Publish(ctx, event); err != nil {
				m.logger.Error("Failed to publish decay event",
					logging.F("relationship_id", info.RelationshipID),
					logging.Err(err),
				)
			}
		}

		processed++
	}

	m.logger.Info("Processed decayed relationships",
		logging.F("tenant_id", tenantID),
		logging.F("processed_count", processed),
	)

	return processed, nil
}

// GetTransitionHistory retrieves the state transition history for a relationship.
func (m *Manager) GetTransitionHistory(ctx context.Context, tenantID, relationshipID string) (*TransitionHistory, error) {
	return m.stateMachine.GetHistory(ctx, tenantID, relationshipID)
}

// filterEvidence removes evidence items by ID from a slice.
func filterEvidence(evidence []*relationshipv1.Evidence, removeIDs []string) []*relationshipv1.Evidence {
	removeSet := make(map[string]bool)
	for _, id := range removeIDs {
		removeSet[id] = true
	}

	var result []*relationshipv1.Evidence
	for _, ev := range evidence {
		if !removeSet[ev.SourceId] {
			result = append(result, ev)
		}
	}
	return result
}
