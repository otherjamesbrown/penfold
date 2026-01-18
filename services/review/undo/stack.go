// Package undo provides undo/redo functionality for review actions.
package undo

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ErrStackEmpty is returned when trying to pop/undo from an empty stack.
var ErrStackEmpty = errors.New("undo stack is empty")

// ErrRedoStackEmpty is returned when trying to redo with no actions to redo.
var ErrRedoStackEmpty = errors.New("redo stack is empty")

// ErrStackFull is returned when the stack has reached its maximum depth.
var ErrStackFull = errors.New("undo stack is full")

// Stack represents an undo/redo stack for a session.
type Stack struct {
	mu          sync.RWMutex
	undoStack   []UndoableAction
	redoStack   []UndoableAction
	limits      *Limits
	logger      logging.Logger
	tenantID    string
	userID      string
	sessionID   string
	createdAt   time.Time
	lastUpdated time.Time
	onPush      func(action UndoableAction)
	onUndo      func(action UndoableAction)
	onRedo      func(action UndoableAction)
}

// StackConfig holds configuration for creating an undo stack.
type StackConfig struct {
	TenantID  string
	UserID    string
	SessionID string
	Limits    *Limits
	Logger    logging.Logger
}

// NewStack creates a new undo stack with the given configuration.
func NewStack(cfg *StackConfig) *Stack {
	if cfg == nil {
		cfg = &StackConfig{}
	}
	if cfg.Limits == nil {
		cfg.Limits = DefaultLimits()
	}
	now := time.Now().UTC()
	return &Stack{
		undoStack:   make([]UndoableAction, 0),
		redoStack:   make([]UndoableAction, 0),
		limits:      cfg.Limits,
		logger:      cfg.Logger,
		tenantID:    cfg.TenantID,
		userID:      cfg.UserID,
		sessionID:   cfg.SessionID,
		createdAt:   now,
		lastUpdated: now,
	}
}

// Push adds an action to the undo stack.
// If the stack is at maximum depth, the oldest action is removed.
func (s *Stack) Push(action UndoableAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we're at max depth
	if len(s.undoStack) >= s.limits.MaxStackDepth {
		// Remove the oldest action
		s.undoStack = s.undoStack[1:]
	}

	s.undoStack = append(s.undoStack, action)
	// Clear redo stack when a new action is pushed
	s.redoStack = make([]UndoableAction, 0)
	s.lastUpdated = time.Now().UTC()

	if s.onPush != nil {
		s.onPush(action)
	}

	if s.logger != nil {
		s.logger.Debug("action pushed to undo stack",
			logging.F("action_id", action.ID()),
			logging.F("action_type", action.Type()),
			logging.F("stack_size", len(s.undoStack)),
		)
	}

	return nil
}

// Pop removes and returns the most recent action from the undo stack.
func (s *Stack) Pop() (UndoableAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.undoStack) == 0 {
		return nil, ErrStackEmpty
	}

	action := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]
	s.lastUpdated = time.Now().UTC()

	return action, nil
}

// Undo undoes the most recent action and moves it to the redo stack.
func (s *Stack) Undo(ctx context.Context) (UndoableAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.undoStack) == 0 {
		return nil, ErrStackEmpty
	}

	action := s.undoStack[len(s.undoStack)-1]

	// Check if action has expired
	if s.limits.ActionTTL > 0 {
		if time.Since(action.CreatedAt()) > s.limits.ActionTTL {
			return nil, ErrActionExpired
		}
	}

	// Perform the undo
	if err := action.Undo(ctx); err != nil {
		return nil, err
	}

	// Move to redo stack
	s.undoStack = s.undoStack[:len(s.undoStack)-1]
	s.redoStack = append(s.redoStack, action)
	s.lastUpdated = time.Now().UTC()

	if s.onUndo != nil {
		s.onUndo(action)
	}

	if s.logger != nil {
		s.logger.Info("action undone",
			logging.F("action_id", action.ID()),
			logging.F("action_type", action.Type()),
			logging.F("description", action.Description()),
		)
	}

	return action, nil
}

// Redo redoes the most recently undone action.
func (s *Stack) Redo(ctx context.Context) (UndoableAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.redoStack) == 0 {
		return nil, ErrRedoStackEmpty
	}

	action := s.redoStack[len(s.redoStack)-1]

	// Perform the redo
	if err := action.Redo(ctx); err != nil {
		return nil, err
	}

	// Move back to undo stack
	s.redoStack = s.redoStack[:len(s.redoStack)-1]
	s.undoStack = append(s.undoStack, action)
	s.lastUpdated = time.Now().UTC()

	if s.onRedo != nil {
		s.onRedo(action)
	}

	if s.logger != nil {
		s.logger.Info("action redone",
			logging.F("action_id", action.ID()),
			logging.F("action_type", action.Type()),
			logging.F("description", action.Description()),
		)
	}

	return action, nil
}

// UndoN undoes the last n actions.
func (s *Stack) UndoN(ctx context.Context, n int) ([]UndoableAction, error) {
	actions := make([]UndoableAction, 0, n)
	for i := 0; i < n; i++ {
		action, err := s.Undo(ctx)
		if err != nil {
			// Return what we undid so far
			return actions, err
		}
		actions = append(actions, action)
	}
	return actions, nil
}

// RedoN redoes the last n undone actions.
func (s *Stack) RedoN(ctx context.Context, n int) ([]UndoableAction, error) {
	actions := make([]UndoableAction, 0, n)
	for i := 0; i < n; i++ {
		action, err := s.Redo(ctx)
		if err != nil {
			// Return what we redid so far
			return actions, err
		}
		actions = append(actions, action)
	}
	return actions, nil
}

// Clear removes all actions from both undo and redo stacks.
func (s *Stack) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.undoStack = make([]UndoableAction, 0)
	s.redoStack = make([]UndoableAction, 0)
	s.lastUpdated = time.Now().UTC()

	if s.logger != nil {
		s.logger.Debug("undo stack cleared",
			logging.F("session_id", s.sessionID),
		)
	}
}

// ClearRedo clears only the redo stack.
func (s *Stack) ClearRedo() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.redoStack = make([]UndoableAction, 0)
	s.lastUpdated = time.Now().UTC()
}

// UndoSize returns the number of actions that can be undone.
func (s *Stack) UndoSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.undoStack)
}

// RedoSize returns the number of actions that can be redone.
func (s *Stack) RedoSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.redoStack)
}

// CanUndo returns true if there are actions that can be undone.
func (s *Stack) CanUndo() bool {
	return s.UndoSize() > 0
}

// CanRedo returns true if there are actions that can be redone.
func (s *Stack) CanRedo() bool {
	return s.RedoSize() > 0
}

// PeekUndo returns the next action to be undone without removing it.
func (s *Stack) PeekUndo() (UndoableAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.undoStack) == 0 {
		return nil, ErrStackEmpty
	}

	return s.undoStack[len(s.undoStack)-1], nil
}

// PeekRedo returns the next action to be redone without removing it.
func (s *Stack) PeekRedo() (UndoableAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.redoStack) == 0 {
		return nil, ErrRedoStackEmpty
	}

	return s.redoStack[len(s.redoStack)-1], nil
}

// GetUndoActions returns all actions in the undo stack (oldest first).
func (s *Stack) GetUndoActions() []UndoableAction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	actions := make([]UndoableAction, len(s.undoStack))
	copy(actions, s.undoStack)
	return actions
}

// GetRedoActions returns all actions in the redo stack (oldest first).
func (s *Stack) GetRedoActions() []UndoableAction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	actions := make([]UndoableAction, len(s.redoStack))
	copy(actions, s.redoStack)
	return actions
}

// TenantID returns the tenant ID for this stack.
func (s *Stack) TenantID() string {
	return s.tenantID
}

// UserID returns the user ID for this stack.
func (s *Stack) UserID() string {
	return s.userID
}

// SessionID returns the session ID for this stack.
func (s *Stack) SessionID() string {
	return s.sessionID
}

// CreatedAt returns when the stack was created.
func (s *Stack) CreatedAt() time.Time {
	return s.createdAt
}

// LastUpdated returns when the stack was last updated.
func (s *Stack) LastUpdated() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastUpdated
}

// SetOnPush sets a callback for when actions are pushed.
func (s *Stack) SetOnPush(fn func(action UndoableAction)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPush = fn
}

// SetOnUndo sets a callback for when actions are undone.
func (s *Stack) SetOnUndo(fn func(action UndoableAction)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUndo = fn
}

// SetOnRedo sets a callback for when actions are redone.
func (s *Stack) SetOnRedo(fn func(action UndoableAction)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRedo = fn
}

// PruneExpired removes actions that have exceeded the TTL.
func (s *Stack) PruneExpired() int {
	if s.limits.ActionTTL == 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-s.limits.ActionTTL)
	pruned := 0

	// Prune from undo stack
	validUndo := make([]UndoableAction, 0, len(s.undoStack))
	for _, action := range s.undoStack {
		if action.CreatedAt().After(cutoff) {
			validUndo = append(validUndo, action)
		} else {
			pruned++
		}
	}
	s.undoStack = validUndo

	// Prune from redo stack
	validRedo := make([]UndoableAction, 0, len(s.redoStack))
	for _, action := range s.redoStack {
		if action.CreatedAt().After(cutoff) {
			validRedo = append(validRedo, action)
		} else {
			pruned++
		}
	}
	s.redoStack = validRedo

	if pruned > 0 {
		s.lastUpdated = now
		if s.logger != nil {
			s.logger.Debug("pruned expired actions",
				logging.F("pruned_count", pruned),
				logging.F("session_id", s.sessionID),
			)
		}
	}

	return pruned
}

// SelectiveUndo undoes a specific action by ID, if possible.
// This only works if the action is at the top of the stack.
func (s *Stack) SelectiveUndo(ctx context.Context, actionID string) (UndoableAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the action in the undo stack
	for i := len(s.undoStack) - 1; i >= 0; i-- {
		if s.undoStack[i].ID() == actionID {
			// If it's not at the top, we need to undo everything above it first
			if i < len(s.undoStack)-1 {
				return nil, ErrConflict
			}

			action := s.undoStack[i]
			if err := action.Undo(ctx); err != nil {
				return nil, err
			}

			s.undoStack = s.undoStack[:i]
			s.redoStack = append(s.redoStack, action)
			s.lastUpdated = time.Now().UTC()

			if s.onUndo != nil {
				s.onUndo(action)
			}

			return action, nil
		}
	}

	return nil, ErrStackEmpty
}

// SelectiveRedo redoes a specific action by ID, if possible.
func (s *Stack) SelectiveRedo(ctx context.Context, actionID string) (UndoableAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the action in the redo stack
	for i := len(s.redoStack) - 1; i >= 0; i-- {
		if s.redoStack[i].ID() == actionID {
			// If it's not at the top, we have a conflict
			if i < len(s.redoStack)-1 {
				return nil, ErrConflict
			}

			action := s.redoStack[i]
			if err := action.Redo(ctx); err != nil {
				return nil, err
			}

			s.redoStack = s.redoStack[:i]
			s.undoStack = append(s.undoStack, action)
			s.lastUpdated = time.Now().UTC()

			if s.onRedo != nil {
				s.onRedo(action)
			}

			return action, nil
		}
	}

	return nil, ErrRedoStackEmpty
}

// GroupedUndo undoes all actions until and including the specified action ID.
func (s *Stack) GroupedUndo(ctx context.Context, untilActionID string) ([]UndoableAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the action
	targetIndex := -1
	for i := len(s.undoStack) - 1; i >= 0; i-- {
		if s.undoStack[i].ID() == untilActionID {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return nil, ErrStackEmpty
	}

	undoneActions := make([]UndoableAction, 0)

	// Undo from top of stack down to target
	for i := len(s.undoStack) - 1; i >= targetIndex; i-- {
		action := s.undoStack[i]
		if err := action.Undo(ctx); err != nil {
			return undoneActions, err
		}
		undoneActions = append(undoneActions, action)
		s.redoStack = append(s.redoStack, action)
	}

	s.undoStack = s.undoStack[:targetIndex]
	s.lastUpdated = time.Now().UTC()

	if s.logger != nil {
		s.logger.Info("grouped undo performed",
			logging.F("actions_undone", len(undoneActions)),
			logging.F("target_action_id", untilActionID),
		)
	}

	return undoneActions, nil
}

// Stats returns statistics about the undo stack.
type Stats struct {
	UndoCount      int           `json:"undo_count"`
	RedoCount      int           `json:"redo_count"`
	MaxStackDepth  int           `json:"max_stack_depth"`
	OldestActionAt *time.Time    `json:"oldest_action_at,omitempty"`
	NewestActionAt *time.Time    `json:"newest_action_at,omitempty"`
	TTL            time.Duration `json:"ttl"`
	LastUpdated    time.Time     `json:"last_updated"`
}

// GetStats returns statistics about the stack.
func (s *Stack) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := Stats{
		UndoCount:     len(s.undoStack),
		RedoCount:     len(s.redoStack),
		MaxStackDepth: s.limits.MaxStackDepth,
		TTL:           s.limits.ActionTTL,
		LastUpdated:   s.lastUpdated,
	}

	if len(s.undoStack) > 0 {
		oldest := s.undoStack[0].CreatedAt()
		newest := s.undoStack[len(s.undoStack)-1].CreatedAt()
		stats.OldestActionAt = &oldest
		stats.NewestActionAt = &newest
	}

	return stats
}
