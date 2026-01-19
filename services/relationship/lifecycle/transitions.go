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
)

// TransitionHook is a function that is called during a state transition.
// Hooks can perform side effects like logging, notifications, or cache invalidation.
type TransitionHook func(ctx context.Context, transition *StateTransition, rel *relationshipv1.Relationship) error

// TransitionHookType indicates when a hook should be executed.
type TransitionHookType string

const (
	// HookBefore is executed before the transition is applied.
	HookBefore TransitionHookType = "before"

	// HookAfter is executed after the transition is applied.
	HookAfter TransitionHookType = "after"
)

// StateMachine manages state transitions for relationships.
type StateMachine struct {
	mu          sync.RWMutex
	logger      logging.Logger
	beforeHooks map[State][]TransitionHook
	afterHooks  map[State][]TransitionHook
	globalHooks struct {
		before []TransitionHook
		after  []TransitionHook
	}
	historyStore TransitionHistoryStore
	now          func() time.Time
}

// TransitionHistoryStore persists transition history.
type TransitionHistoryStore interface {
	// SaveTransition persists a state transition.
	SaveTransition(ctx context.Context, transition *StateTransition) error

	// GetHistory retrieves the transition history for a relationship.
	GetHistory(ctx context.Context, tenantID, relationshipID string) (*TransitionHistory, error)
}

// StateMachineConfig configures the state machine.
type StateMachineConfig struct {
	// Logger is used for logging state transitions.
	Logger logging.Logger

	// HistoryStore persists transition history.
	HistoryStore TransitionHistoryStore
}

// NewStateMachine creates a new StateMachine.
func NewStateMachine(cfg *StateMachineConfig) *StateMachine {
	sm := &StateMachine{
		beforeHooks:  make(map[State][]TransitionHook),
		afterHooks:   make(map[State][]TransitionHook),
		historyStore: nil,
		now:          time.Now,
	}

	if cfg != nil {
		sm.logger = cfg.Logger
		sm.historyStore = cfg.HistoryStore
	}

	if sm.logger == nil {
		sm.logger = logging.MustGlobal()
	}

	return sm
}

// CanTransition checks if a transition from the current state to the target state is valid.
func (sm *StateMachine) CanTransition(from, to State) bool {
	// Same state is always valid (no-op)
	if from == to {
		return true
	}

	// Check if from state is valid
	if !from.IsValid() {
		return false
	}

	// Check if to state is valid
	if !to.IsValid() {
		return false
	}

	// Check valid transitions map
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

// ExecuteTransition performs a state transition on a relationship.
func (sm *StateMachine) ExecuteTransition(ctx context.Context, rel *relationshipv1.Relationship, newState State, reason string, actor string) (*StateTransition, error) {
	if rel == nil {
		return nil, fmt.Errorf("relationship cannot be nil")
	}

	currentState := statusToState(rel.Status)

	// Check if transition is valid
	if !sm.CanTransition(currentState, newState) {
		return nil, fmt.Errorf("invalid transition from %s to %s", currentState, newState)
	}

	// Create transition record
	transition := &StateTransition{
		ID:             uuid.New().String(),
		RelationshipID: rel.Id,
		TenantID:       rel.TenantId,
		From:           currentState,
		To:             newState,
		Timestamp:      sm.now(),
		Reason:         reason,
		Actor:          actor,
		Metadata:       make(map[string]string),
	}

	sm.logger.Info("Executing state transition",
		logging.F("relationship_id", rel.Id),
		logging.F("tenant_id", rel.TenantId),
		logging.F("from_state", currentState.String()),
		logging.F("to_state", newState.String()),
		logging.F("reason", reason),
		logging.F("actor", actor),
	)

	// Execute before hooks
	if err := sm.executeHooks(ctx, HookBefore, transition, rel); err != nil {
		sm.logger.Error("Before hook failed",
			logging.F("relationship_id", rel.Id),
			logging.Err(err),
		)
		return nil, fmt.Errorf("before hook failed: %w", err)
	}

	// Apply the transition (update status)
	rel.Status = stateToStatus(newState)

	// Save transition to history if store is available
	if sm.historyStore != nil {
		if err := sm.historyStore.SaveTransition(ctx, transition); err != nil {
			sm.logger.Error("Failed to save transition history",
				logging.F("relationship_id", rel.Id),
				logging.Err(err),
			)
			// Non-fatal, continue with transition
		}
	}

	// Execute after hooks
	if err := sm.executeHooks(ctx, HookAfter, transition, rel); err != nil {
		sm.logger.Warn("After hook failed",
			logging.F("relationship_id", rel.Id),
			logging.Err(err),
		)
		// After hooks failing is non-fatal
	}

	sm.logger.Info("State transition completed",
		logging.F("relationship_id", rel.Id),
		logging.F("transition_id", transition.ID),
	)

	return transition, nil
}

// RegisterHook registers a hook to be executed during transitions.
func (sm *StateMachine) RegisterHook(hookType TransitionHookType, targetState State, hook TransitionHook) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	switch hookType {
	case HookBefore:
		if targetState == "" {
			sm.globalHooks.before = append(sm.globalHooks.before, hook)
		} else {
			sm.beforeHooks[targetState] = append(sm.beforeHooks[targetState], hook)
		}
	case HookAfter:
		if targetState == "" {
			sm.globalHooks.after = append(sm.globalHooks.after, hook)
		} else {
			sm.afterHooks[targetState] = append(sm.afterHooks[targetState], hook)
		}
	}
}

// RegisterBeforeHook registers a hook to be executed before transitioning to a state.
func (sm *StateMachine) RegisterBeforeHook(targetState State, hook TransitionHook) {
	sm.RegisterHook(HookBefore, targetState, hook)
}

// RegisterAfterHook registers a hook to be executed after transitioning to a state.
func (sm *StateMachine) RegisterAfterHook(targetState State, hook TransitionHook) {
	sm.RegisterHook(HookAfter, targetState, hook)
}

// RegisterGlobalBeforeHook registers a hook to be executed before any transition.
func (sm *StateMachine) RegisterGlobalBeforeHook(hook TransitionHook) {
	sm.RegisterHook(HookBefore, "", hook)
}

// RegisterGlobalAfterHook registers a hook to be executed after any transition.
func (sm *StateMachine) RegisterGlobalAfterHook(hook TransitionHook) {
	sm.RegisterHook(HookAfter, "", hook)
}

// executeHooks executes hooks for a transition.
func (sm *StateMachine) executeHooks(ctx context.Context, hookType TransitionHookType, transition *StateTransition, rel *relationshipv1.Relationship) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var hooks []TransitionHook

	switch hookType {
	case HookBefore:
		// Add global hooks first
		hooks = append(hooks, sm.globalHooks.before...)
		// Add state-specific hooks
		if stateHooks, ok := sm.beforeHooks[transition.To]; ok {
			hooks = append(hooks, stateHooks...)
		}
	case HookAfter:
		// Add global hooks first
		hooks = append(hooks, sm.globalHooks.after...)
		// Add state-specific hooks
		if stateHooks, ok := sm.afterHooks[transition.To]; ok {
			hooks = append(hooks, stateHooks...)
		}
	}

	for _, hook := range hooks {
		if err := hook(ctx, transition, rel); err != nil {
			return err
		}
	}

	return nil
}

// GetValidTransitions returns the list of valid target states from the current state.
func (sm *StateMachine) GetValidTransitions(currentState State) []State {
	validTargets, ok := ValidTransitions[currentState]
	if !ok {
		return nil
	}
	return validTargets
}

// GetHistory retrieves the transition history for a relationship.
func (sm *StateMachine) GetHistory(ctx context.Context, tenantID, relationshipID string) (*TransitionHistory, error) {
	if sm.historyStore == nil {
		return nil, fmt.Errorf("history store not configured")
	}
	return sm.historyStore.GetHistory(ctx, tenantID, relationshipID)
}

// DecayDetector detects relationships that should transition to inactive.
type DecayDetector struct {
	// InactivePeriod is the duration after which a relationship is considered inactive.
	InactivePeriod time.Duration

	// DecayThreshold is the confidence decay percentage that triggers inactivity.
	DecayThreshold float32

	// Now allows overriding the current time for testing.
	Now func() time.Time
}

// DefaultDecayDetector creates a DecayDetector with default settings.
func DefaultDecayDetector() *DecayDetector {
	return &DecayDetector{
		InactivePeriod: 180 * 24 * time.Hour, // 6 months
		DecayThreshold: 0.5,                  // 50% decay
		Now:            time.Now,
	}
}

// CheckDecay evaluates whether a relationship should be marked as inactive.
func (d *DecayDetector) CheckDecay(rel *relationshipv1.Relationship) *DecayInfo {
	info := &DecayInfo{
		RelationshipID: rel.Id,
		DetectedAt:     d.Now(),
	}

	// Determine last activity time
	var lastActivity time.Time
	if rel.UpdatedAt != nil {
		lastActivity = rel.UpdatedAt.AsTime()
	} else if rel.CreatedAt != nil {
		lastActivity = rel.CreatedAt.AsTime()
	} else {
		lastActivity = d.Now().Add(-d.InactivePeriod * 2) // Assume very old
	}

	info.LastActivityAt = lastActivity

	// Calculate days since activity
	daysSince := int(d.Now().Sub(lastActivity).Hours() / 24)
	info.DaysSinceActivity = daysSince

	// Calculate confidence decay
	if daysSince > 0 {
		inactiveDays := int(d.InactivePeriod.Hours() / 24)
		decayRatio := float32(daysSince) / float32(inactiveDays)
		if decayRatio > 1.0 {
			decayRatio = 1.0
		}
		info.ConfidenceDecay = decayRatio * d.DecayThreshold
	}

	// Determine if should transition
	info.ShouldTransition = daysSince >= int(d.InactivePeriod.Hours()/24)

	return info
}

// BatchCheckDecay checks multiple relationships for decay.
func (d *DecayDetector) BatchCheckDecay(relationships []*relationshipv1.Relationship) []*DecayInfo {
	var decayed []*DecayInfo
	for _, rel := range relationships {
		info := d.CheckDecay(rel)
		if info.ShouldTransition {
			decayed = append(decayed, info)
		}
	}
	return decayed
}

// InMemoryHistoryStore is an in-memory implementation of TransitionHistoryStore.
type InMemoryHistoryStore struct {
	mu        sync.RWMutex
	histories map[string]*TransitionHistory // key: tenantID:relationshipID
}

// NewInMemoryHistoryStore creates a new in-memory history store.
func NewInMemoryHistoryStore() *InMemoryHistoryStore {
	return &InMemoryHistoryStore{
		histories: make(map[string]*TransitionHistory),
	}
}

// SaveTransition saves a transition to the in-memory store.
func (s *InMemoryHistoryStore) SaveTransition(ctx context.Context, transition *StateTransition) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fmt.Sprintf("%s:%s", transition.TenantID, transition.RelationshipID)
	history, ok := s.histories[key]
	if !ok {
		history = &TransitionHistory{
			RelationshipID: transition.RelationshipID,
			TenantID:       transition.TenantID,
			CurrentState:   transition.From,
			CreatedAt:      transition.Timestamp,
		}
		s.histories[key] = history
	}

	history.AddTransition(*transition)
	return nil
}

// GetHistory retrieves the transition history from the in-memory store.
func (s *InMemoryHistoryStore) GetHistory(ctx context.Context, tenantID, relationshipID string) (*TransitionHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", tenantID, relationshipID)
	history, ok := s.histories[key]
	if !ok {
		return nil, fmt.Errorf("history not found for relationship %s", relationshipID)
	}

	return history, nil
}
