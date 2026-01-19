// Package lifecycle provides relationship lifecycle management capabilities.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
)

// ValidationError represents a validation failure with detailed context.
type ValidationError struct {
	// Field is the field that failed validation.
	Field string

	// Message describes the validation failure.
	Message string

	// Code is a machine-readable error code.
	Code string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidationResult contains the outcome of a validation operation.
type ValidationResult struct {
	// Valid indicates whether validation passed.
	Valid bool

	// Errors contains all validation errors encountered.
	Errors []ValidationError

	// Warnings contains non-fatal validation issues.
	Warnings []string
}

// AddError adds a validation error to the result.
func (r *ValidationResult) AddError(field, message, code string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
	})
}

// AddWarning adds a warning to the result.
func (r *ValidationResult) AddWarning(message string) {
	r.Warnings = append(r.Warnings, message)
}

// ToError converts the validation result to an error if invalid.
func (r *ValidationResult) ToError() error {
	if r.Valid {
		return nil
	}
	if len(r.Errors) == 0 {
		return errors.New("validation failed")
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return errors.New(strings.Join(msgs, "; "))
}

// Validator provides validation logic for lifecycle operations.
type Validator struct {
	// DuplicateChecker checks for duplicate relationships.
	DuplicateChecker DuplicateChecker

	// MinConfidenceForAutoActive is the minimum confidence for automatic activation.
	MinConfidenceForAutoActive float32

	// MaxEvidenceAge is the maximum age for evidence to be considered valid.
	MaxEvidenceAge time.Duration

	// RequireEvidence indicates if relationships must have evidence.
	RequireEvidence bool
}

// DuplicateChecker is an interface for checking duplicate relationships.
type DuplicateChecker interface {
	// FindDuplicates returns potential duplicates for a relationship.
	FindDuplicates(ctx context.Context, tenantID string, rel *relationshipv1.Relationship) ([]*relationshipv1.Relationship, error)
}

// DefaultValidator creates a Validator with default settings.
func DefaultValidator() *Validator {
	return &Validator{
		MinConfidenceForAutoActive: 0.8,
		MaxEvidenceAge:             365 * 24 * time.Hour, // 1 year
		RequireEvidence:            false,
	}
}

// ValidateCreate validates a relationship for creation.
func (v *Validator) ValidateCreate(ctx context.Context, rel *relationshipv1.Relationship) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Validate required fields
	if rel == nil {
		result.AddError("", "relationship cannot be nil", "ERR_NIL_RELATIONSHIP")
		return result
	}

	if rel.TenantId == "" {
		result.AddError("tenant_id", "tenant ID is required", "ERR_MISSING_TENANT_ID")
	}

	if rel.SourceEntity == nil {
		result.AddError("source_entity", "source entity is required", "ERR_MISSING_SOURCE_ENTITY")
	} else {
		v.validateEntity(rel.SourceEntity, "source_entity", result)
	}

	if rel.TargetEntity == nil {
		result.AddError("target_entity", "target entity is required", "ERR_MISSING_TARGET_ENTITY")
	} else {
		v.validateEntity(rel.TargetEntity, "target_entity", result)
	}

	// Validate relationship type
	if rel.RelationshipType == relationshipv1.RelationshipType_RELATIONSHIP_TYPE_UNSPECIFIED {
		result.AddError("relationship_type", "relationship type must be specified", "ERR_MISSING_RELATIONSHIP_TYPE")
	}

	// Validate confidence
	if rel.Confidence < 0 || rel.Confidence > 1 {
		result.AddError("confidence", "confidence must be between 0 and 1", "ERR_INVALID_CONFIDENCE")
	}

	// Validate self-referential relationship
	if rel.SourceEntity != nil && rel.TargetEntity != nil {
		if rel.SourceEntity.Id == rel.TargetEntity.Id && rel.SourceEntity.Id != "" {
			result.AddError("", "relationship cannot reference the same entity as source and target", "ERR_SELF_REFERENTIAL")
		}
	}

	// Validate evidence if required
	if v.RequireEvidence && len(rel.Evidence) == 0 {
		result.AddError("evidence", "at least one piece of evidence is required", "ERR_MISSING_EVIDENCE")
	}

	// Validate each piece of evidence
	for i, ev := range rel.Evidence {
		v.validateEvidence(ev, fmt.Sprintf("evidence[%d]", i), result)
	}

	// Check for duplicates if checker is available
	if v.DuplicateChecker != nil && result.Valid {
		duplicates, err := v.DuplicateChecker.FindDuplicates(ctx, rel.TenantId, rel)
		if err != nil {
			result.AddWarning(fmt.Sprintf("could not check for duplicates: %v", err))
		} else if len(duplicates) > 0 {
			result.AddWarning(fmt.Sprintf("potential duplicate(s) found: %d existing relationship(s) may match", len(duplicates)))
		}
	}

	return result
}

// validateEntity validates an entity's fields.
func (v *Validator) validateEntity(entity *relationshipv1.Entity, fieldPrefix string, result *ValidationResult) {
	if entity.Name == "" {
		result.AddError(fieldPrefix+".name", "entity name is required", "ERR_MISSING_ENTITY_NAME")
	}

	if entity.Type == relationshipv1.EntityType_ENTITY_TYPE_UNSPECIFIED {
		result.AddError(fieldPrefix+".type", "entity type must be specified", "ERR_MISSING_ENTITY_TYPE")
	}
}

// validateEvidence validates an evidence item's fields.
func (v *Validator) validateEvidence(ev *relationshipv1.Evidence, fieldPrefix string, result *ValidationResult) {
	if ev == nil {
		result.AddError(fieldPrefix, "evidence cannot be nil", "ERR_NIL_EVIDENCE")
		return
	}

	if ev.SourceId == "" {
		result.AddError(fieldPrefix+".source_id", "evidence source ID is required", "ERR_MISSING_EVIDENCE_SOURCE_ID")
	}

	if ev.SourceType == "" {
		result.AddError(fieldPrefix+".source_type", "evidence source type is required", "ERR_MISSING_EVIDENCE_SOURCE_TYPE")
	}

	if ev.ConfidenceContribution < 0 || ev.ConfidenceContribution > 1 {
		result.AddError(fieldPrefix+".confidence_contribution", "confidence contribution must be between 0 and 1", "ERR_INVALID_CONFIDENCE_CONTRIBUTION")
	}

	// Check evidence age
	if ev.DiscoveredAt != nil && v.MaxEvidenceAge > 0 {
		age := time.Since(ev.DiscoveredAt.AsTime())
		if age > v.MaxEvidenceAge {
			result.AddWarning(fmt.Sprintf("%s: evidence is older than maximum age (%v)", fieldPrefix, v.MaxEvidenceAge))
		}
	}
}

// RelationshipUpdate represents an update to a relationship.
type RelationshipUpdate struct {
	// Confidence is the new confidence value (if set).
	Confidence *float32

	// Status is the new status (if set).
	Status *relationshipv1.RelationshipStatus

	// Notes is the new notes value (if set).
	Notes *string

	// AddEvidence is evidence to add.
	AddEvidence []*relationshipv1.Evidence

	// RemoveEvidenceIDs are IDs of evidence to remove.
	RemoveEvidenceIDs []string

	// Metadata contains additional update context.
	Metadata map[string]string
}

// ValidateUpdate validates an update operation.
func (v *Validator) ValidateUpdate(ctx context.Context, rel *relationshipv1.Relationship, update *RelationshipUpdate) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if rel == nil {
		result.AddError("", "existing relationship cannot be nil", "ERR_NIL_RELATIONSHIP")
		return result
	}

	if update == nil {
		result.AddError("", "update cannot be nil", "ERR_NIL_UPDATE")
		return result
	}

	// Validate confidence if being updated
	if update.Confidence != nil {
		if *update.Confidence < 0 || *update.Confidence > 1 {
			result.AddError("confidence", "confidence must be between 0 and 1", "ERR_INVALID_CONFIDENCE")
		}
	}

	// Validate status transition if being updated
	if update.Status != nil {
		currentState := statusToState(rel.Status)
		newState := statusToState(*update.Status)

		if !canTransitionTo(currentState, newState) {
			result.AddError("status", fmt.Sprintf("cannot transition from %s to %s", currentState, newState), "ERR_INVALID_TRANSITION")
		}
	}

	// Validate new evidence
	for i, ev := range update.AddEvidence {
		v.validateEvidence(ev, fmt.Sprintf("add_evidence[%d]", i), result)
	}

	return result
}

// ValidateMerge validates a merge operation between two relationships.
func (v *Validator) ValidateMerge(ctx context.Context, source, target *relationshipv1.Relationship) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if source == nil {
		result.AddError("source", "source relationship cannot be nil", "ERR_NIL_SOURCE")
		return result
	}

	if target == nil {
		result.AddError("target", "target relationship cannot be nil", "ERR_NIL_TARGET")
		return result
	}

	// Cannot merge a relationship with itself
	if source.Id == target.Id {
		result.AddError("", "cannot merge a relationship with itself", "ERR_SELF_MERGE")
		return result
	}

	// Must be in the same tenant
	if source.TenantId != target.TenantId {
		result.AddError("tenant_id", "source and target must be in the same tenant", "ERR_TENANT_MISMATCH")
	}

	// Must have the same relationship type
	if source.RelationshipType != target.RelationshipType {
		result.AddWarning(fmt.Sprintf("merging relationships of different types: %s and %s",
			source.RelationshipType.String(), target.RelationshipType.String()))
	}

	// Check that entities are compatible for merge
	if source.SourceEntity != nil && target.SourceEntity != nil {
		if !entitiesAreCompatible(source.SourceEntity, target.SourceEntity) {
			result.AddError("source_entity", "source entities are not compatible for merge", "ERR_INCOMPATIBLE_SOURCE_ENTITIES")
		}
	}

	if source.TargetEntity != nil && target.TargetEntity != nil {
		if !entitiesAreCompatible(source.TargetEntity, target.TargetEntity) {
			result.AddError("target_entity", "target entities are not compatible for merge", "ERR_INCOMPATIBLE_TARGET_ENTITIES")
		}
	}

	// Cannot merge into an archived or merged relationship
	targetState := statusToState(target.Status)
	if targetState == StateArchived {
		result.AddError("target.status", "cannot merge into an archived relationship", "ERR_MERGE_INTO_ARCHIVED")
	}
	if targetState == StateMerged {
		result.AddError("target.status", "cannot merge into an already merged relationship", "ERR_MERGE_INTO_MERGED")
	}

	// Cannot merge a merged relationship
	sourceState := statusToState(source.Status)
	if sourceState == StateMerged {
		result.AddError("source.status", "cannot merge an already merged relationship", "ERR_MERGE_FROM_MERGED")
	}

	return result
}

// ValidateArchive validates an archive operation.
func (v *Validator) ValidateArchive(ctx context.Context, rel *relationshipv1.Relationship, reason string) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if rel == nil {
		result.AddError("", "relationship cannot be nil", "ERR_NIL_RELATIONSHIP")
		return result
	}

	// Check current state allows archival
	currentState := statusToState(rel.Status)
	if !canTransitionTo(currentState, StateArchived) {
		result.AddError("status", fmt.Sprintf("cannot archive relationship in state %s", currentState), "ERR_CANNOT_ARCHIVE")
	}

	// Reason is recommended but not required
	if reason == "" {
		result.AddWarning("archive reason not provided")
	}

	return result
}

// ValidateRestore validates a restore operation.
func (v *Validator) ValidateRestore(ctx context.Context, rel *relationshipv1.Relationship) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if rel == nil {
		result.AddError("", "relationship cannot be nil", "ERR_NIL_RELATIONSHIP")
		return result
	}

	// Check current state is archived
	currentState := statusToState(rel.Status)
	if currentState != StateArchived {
		result.AddError("status", fmt.Sprintf("can only restore archived relationships, current state is %s", currentState), "ERR_NOT_ARCHIVED")
	}

	return result
}

// statusToState converts a protobuf RelationshipStatus to a lifecycle State.
func statusToState(status relationshipv1.RelationshipStatus) State {
	switch status {
	case relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED:
		return StateProposed
	case relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED:
		return StateActive
	case relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_REJECTED:
		return StateArchived
	case relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED:
		return StateArchived
	default:
		return StateProposed
	}
}

// stateToStatus converts a lifecycle State to a protobuf RelationshipStatus.
func stateToStatus(state State) relationshipv1.RelationshipStatus {
	switch state {
	case StateProposed:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED
	case StateActive:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_CONFIRMED
	case StateInactive:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_DISCOVERED // No direct mapping, use discovered
	case StateArchived:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED
	case StateMerged:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_ARCHIVED // Merged relationships are effectively archived
	default:
		return relationshipv1.RelationshipStatus_RELATIONSHIP_STATUS_UNSPECIFIED
	}
}

// canTransitionTo checks if a transition from one state to another is valid.
func canTransitionTo(from, to State) bool {
	if from == to {
		return true // Same state is always valid
	}

	validTargets, ok := ValidTransitions[from]
	if !ok {
		return false
	}

	for _, target := range validTargets {
		if target == to {
			return true
		}
	}

	return false
}

// entitiesAreCompatible checks if two entities can be considered the same for merge purposes.
func entitiesAreCompatible(a, b *relationshipv1.Entity) bool {
	// Same ID is always compatible
	if a.Id != "" && a.Id == b.Id {
		return true
	}

	// Same type and matching canonical names are compatible
	if a.Type == b.Type {
		canonA := a.GetCanonicalName()
		canonB := b.GetCanonicalName()
		if canonA != "" && canonA == canonB {
			return true
		}

		// Fall back to name comparison (case-insensitive)
		if strings.EqualFold(a.Name, b.Name) {
			return true
		}
	}

	return false
}
