// Package undo provides undo/redo functionality for review actions.
// It implements an undo stack with support for grouping operations,
// persistence, and full audit trails for compliance.
package undo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrActionNotUndoable is returned when an action cannot be undone.
var ErrActionNotUndoable = errors.New("action cannot be undone")

// ErrActionNotRedoable is returned when an action cannot be redone.
var ErrActionNotRedoable = errors.New("action cannot be redone")

// ErrConflict is returned when there's a conflict during undo/redo.
var ErrConflict = errors.New("conflict detected during undo/redo")

// ErrActionExpired is returned when an action has expired and cannot be undone.
var ErrActionExpired = errors.New("action has expired")

// ActionType represents the type of undoable action.
type ActionType string

const (
	// ActionTypeAccept represents an accept action on a review item.
	ActionTypeAccept ActionType = "accept"

	// ActionTypeReject represents a reject action on a review item.
	ActionTypeReject ActionType = "reject"

	// ActionTypeDefer represents a defer action on a review item.
	ActionTypeDefer ActionType = "defer"

	// ActionTypeTag represents adding/removing tags on a review item.
	ActionTypeTag ActionType = "tag"

	// ActionTypeMerge represents merging review items.
	ActionTypeMerge ActionType = "merge"

	// ActionTypeSplit represents splitting a review item.
	ActionTypeSplit ActionType = "split"

	// ActionTypeBatch represents a batch of grouped operations.
	ActionTypeBatch ActionType = "batch"
)

// ActionState represents the current state of an action.
type ActionState string

const (
	// ActionStatePending indicates the action is pending execution.
	ActionStatePending ActionState = "pending"

	// ActionStateExecuted indicates the action has been executed.
	ActionStateExecuted ActionState = "executed"

	// ActionStateUndone indicates the action has been undone.
	ActionStateUndone ActionState = "undone"

	// ActionStateRedone indicates the action has been redone.
	ActionStateRedone ActionState = "redone"
)

// UndoableAction is the interface for actions that can be undone/redone.
type UndoableAction interface {
	// ID returns the unique identifier for this action.
	ID() string

	// Type returns the type of action.
	Type() ActionType

	// Description returns a human-readable description of the action.
	Description() string

	// Execute performs the action.
	Execute(ctx context.Context) error

	// Undo reverses the action.
	Undo(ctx context.Context) error

	// Redo re-performs the action after an undo.
	Redo(ctx context.Context) error

	// CanUndo returns true if the action can be undone.
	CanUndo() bool

	// CanRedo returns true if the action can be redone.
	CanRedo() bool

	// State returns the current state of the action.
	State() ActionState

	// SetState sets the current state of the action.
	SetState(state ActionState)

	// Metadata returns action-specific metadata.
	Metadata() map[string]interface{}

	// TenantID returns the tenant this action belongs to.
	TenantID() string

	// UserID returns the user who performed this action.
	UserID() string

	// SessionID returns the session this action belongs to.
	SessionID() string

	// CreatedAt returns when the action was created.
	CreatedAt() time.Time

	// ExecutedAt returns when the action was executed.
	ExecutedAt() *time.Time

	// ToJSON serializes the action to JSON.
	ToJSON() ([]byte, error)
}

// BaseAction provides common functionality for all undoable actions.
type BaseAction struct {
	id         string
	actionType ActionType
	desc       string
	state      ActionState
	metadata   map[string]interface{}
	tenantID   string
	userID     string
	sessionID  string
	createdAt  time.Time
	executedAt *time.Time
}

// NewBaseAction creates a new base action with common fields.
func NewBaseAction(actionType ActionType, desc, tenantID, userID, sessionID string) *BaseAction {
	return &BaseAction{
		id:         uuid.New().String(),
		actionType: actionType,
		desc:       desc,
		state:      ActionStatePending,
		metadata:   make(map[string]interface{}),
		tenantID:   tenantID,
		userID:     userID,
		sessionID:  sessionID,
		createdAt:  time.Now().UTC(),
	}
}

// ID returns the unique identifier for this action.
func (a *BaseAction) ID() string { return a.id }

// Type returns the type of action.
func (a *BaseAction) Type() ActionType { return a.actionType }

// Description returns a human-readable description of the action.
func (a *BaseAction) Description() string { return a.desc }

// State returns the current state of the action.
func (a *BaseAction) State() ActionState { return a.state }

// SetState sets the current state of the action.
func (a *BaseAction) SetState(state ActionState) { a.state = state }

// Metadata returns action-specific metadata.
func (a *BaseAction) Metadata() map[string]interface{} { return a.metadata }

// TenantID returns the tenant this action belongs to.
func (a *BaseAction) TenantID() string { return a.tenantID }

// UserID returns the user who performed this action.
func (a *BaseAction) UserID() string { return a.userID }

// SessionID returns the session this action belongs to.
func (a *BaseAction) SessionID() string { return a.sessionID }

// CreatedAt returns when the action was created.
func (a *BaseAction) CreatedAt() time.Time { return a.createdAt }

// ExecutedAt returns when the action was executed.
func (a *BaseAction) ExecutedAt() *time.Time { return a.executedAt }

// CanUndo returns true if the action can be undone.
func (a *BaseAction) CanUndo() bool {
	return a.state == ActionStateExecuted || a.state == ActionStateRedone
}

// CanRedo returns true if the action can be redone.
func (a *BaseAction) CanRedo() bool {
	return a.state == ActionStateUndone
}

// SetMetadata sets a metadata value.
func (a *BaseAction) SetMetadata(key string, value interface{}) {
	a.metadata[key] = value
}

// MarkExecuted marks the action as executed.
func (a *BaseAction) MarkExecuted() {
	now := time.Now().UTC()
	a.executedAt = &now
	a.state = ActionStateExecuted
}

// actionJSON is used for JSON serialization.
type actionJSON struct {
	ID         string                 `json:"id"`
	Type       ActionType             `json:"type"`
	Desc       string                 `json:"description"`
	State      ActionState            `json:"state"`
	Metadata   map[string]interface{} `json:"metadata"`
	TenantID   string                 `json:"tenant_id"`
	UserID     string                 `json:"user_id"`
	SessionID  string                 `json:"session_id"`
	CreatedAt  time.Time              `json:"created_at"`
	ExecutedAt *time.Time             `json:"executed_at,omitempty"`
}

// toActionJSON converts base action to JSON struct.
func (a *BaseAction) toActionJSON() actionJSON {
	return actionJSON{
		ID:         a.id,
		Type:       a.actionType,
		Desc:       a.desc,
		State:      a.state,
		Metadata:   a.metadata,
		TenantID:   a.tenantID,
		UserID:     a.userID,
		SessionID:  a.sessionID,
		CreatedAt:  a.createdAt,
		ExecutedAt: a.executedAt,
	}
}

// fromActionJSON populates base action from JSON struct.
func (a *BaseAction) fromActionJSON(j actionJSON) {
	a.id = j.ID
	a.actionType = j.Type
	a.desc = j.Desc
	a.state = j.State
	a.metadata = j.Metadata
	a.tenantID = j.TenantID
	a.userID = j.UserID
	a.sessionID = j.SessionID
	a.createdAt = j.CreatedAt
	a.executedAt = j.ExecutedAt
}

// ActionExecutor is a function type for executing review actions.
type ActionExecutor func(ctx context.Context, contentID string, action string) error

// AcceptAction represents accepting a review item.
type AcceptAction struct {
	*BaseAction
	ContentID     string         `json:"content_id"`
	ContentType   string         `json:"content_type"`
	PreviousState string         `json:"previous_state"`
	Executor      ActionExecutor `json:"-"`
}

// NewAcceptAction creates a new accept action.
func NewAcceptAction(tenantID, userID, sessionID, contentID, contentType, previousState string) *AcceptAction {
	return &AcceptAction{
		BaseAction:    NewBaseAction(ActionTypeAccept, fmt.Sprintf("Accept %s: %s", contentType, contentID), tenantID, userID, sessionID),
		ContentID:     contentID,
		ContentType:   contentType,
		PreviousState: previousState,
	}
}

// Execute performs the accept action.
func (a *AcceptAction) Execute(ctx context.Context) error {
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, "accept"); err != nil {
			return fmt.Errorf("execute accept: %w", err)
		}
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses the accept action.
func (a *AcceptAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, a.PreviousState); err != nil {
			return fmt.Errorf("undo accept: %w", err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs the accept action.
func (a *AcceptAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, "accept"); err != nil {
			return fmt.Errorf("redo accept: %w", err)
		}
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the action to JSON.
func (a *AcceptAction) ToJSON() ([]byte, error) {
	type acceptJSON struct {
		actionJSON
		ContentID     string `json:"content_id"`
		ContentType   string `json:"content_type"`
		PreviousState string `json:"previous_state"`
	}
	j := acceptJSON{
		actionJSON:    a.BaseAction.toActionJSON(),
		ContentID:     a.ContentID,
		ContentType:   a.ContentType,
		PreviousState: a.PreviousState,
	}
	return json.Marshal(j)
}

// RejectAction represents rejecting a review item.
type RejectAction struct {
	*BaseAction
	ContentID     string         `json:"content_id"`
	ContentType   string         `json:"content_type"`
	PreviousState string         `json:"previous_state"`
	Reason        string         `json:"reason,omitempty"`
	Executor      ActionExecutor `json:"-"`
}

// NewRejectAction creates a new reject action.
func NewRejectAction(tenantID, userID, sessionID, contentID, contentType, previousState, reason string) *RejectAction {
	return &RejectAction{
		BaseAction:    NewBaseAction(ActionTypeReject, fmt.Sprintf("Reject %s: %s", contentType, contentID), tenantID, userID, sessionID),
		ContentID:     contentID,
		ContentType:   contentType,
		PreviousState: previousState,
		Reason:        reason,
	}
}

// Execute performs the reject action.
func (a *RejectAction) Execute(ctx context.Context) error {
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, "reject"); err != nil {
			return fmt.Errorf("execute reject: %w", err)
		}
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses the reject action.
func (a *RejectAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, a.PreviousState); err != nil {
			return fmt.Errorf("undo reject: %w", err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs the reject action.
func (a *RejectAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, "reject"); err != nil {
			return fmt.Errorf("redo reject: %w", err)
		}
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the action to JSON.
func (a *RejectAction) ToJSON() ([]byte, error) {
	type rejectJSON struct {
		actionJSON
		ContentID     string `json:"content_id"`
		ContentType   string `json:"content_type"`
		PreviousState string `json:"previous_state"`
		Reason        string `json:"reason,omitempty"`
	}
	j := rejectJSON{
		actionJSON:    a.BaseAction.toActionJSON(),
		ContentID:     a.ContentID,
		ContentType:   a.ContentType,
		PreviousState: a.PreviousState,
		Reason:        a.Reason,
	}
	return json.Marshal(j)
}

// DeferAction represents deferring a review item.
type DeferAction struct {
	*BaseAction
	ContentID     string         `json:"content_id"`
	ContentType   string         `json:"content_type"`
	PreviousState string         `json:"previous_state"`
	DeferUntil    *time.Time     `json:"defer_until,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	Executor      ActionExecutor `json:"-"`
}

// NewDeferAction creates a new defer action.
func NewDeferAction(tenantID, userID, sessionID, contentID, contentType, previousState, reason string, deferUntil *time.Time) *DeferAction {
	return &DeferAction{
		BaseAction:    NewBaseAction(ActionTypeDefer, fmt.Sprintf("Defer %s: %s", contentType, contentID), tenantID, userID, sessionID),
		ContentID:     contentID,
		ContentType:   contentType,
		PreviousState: previousState,
		DeferUntil:    deferUntil,
		Reason:        reason,
	}
}

// Execute performs the defer action.
func (a *DeferAction) Execute(ctx context.Context) error {
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, "defer"); err != nil {
			return fmt.Errorf("execute defer: %w", err)
		}
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses the defer action.
func (a *DeferAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, a.PreviousState); err != nil {
			return fmt.Errorf("undo defer: %w", err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs the defer action.
func (a *DeferAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	if a.Executor != nil {
		if err := a.Executor(ctx, a.ContentID, "defer"); err != nil {
			return fmt.Errorf("redo defer: %w", err)
		}
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the action to JSON.
func (a *DeferAction) ToJSON() ([]byte, error) {
	type deferJSON struct {
		actionJSON
		ContentID     string     `json:"content_id"`
		ContentType   string     `json:"content_type"`
		PreviousState string     `json:"previous_state"`
		DeferUntil    *time.Time `json:"defer_until,omitempty"`
		Reason        string     `json:"reason,omitempty"`
	}
	j := deferJSON{
		actionJSON:    a.BaseAction.toActionJSON(),
		ContentID:     a.ContentID,
		ContentType:   a.ContentType,
		PreviousState: a.PreviousState,
		DeferUntil:    a.DeferUntil,
		Reason:        a.Reason,
	}
	return json.Marshal(j)
}

// TagAction represents adding or removing tags on a review item.
type TagAction struct {
	*BaseAction
	ContentID    string   `json:"content_id"`
	ContentType  string   `json:"content_type"`
	AddedTags    []string `json:"added_tags"`
	RemovedTags  []string `json:"removed_tags"`
	PreviousTags []string `json:"previous_tags"`
	// TagExecutor is called with (contentID, tagsToAdd, tagsToRemove).
	TagExecutor func(ctx context.Context, contentID string, add, remove []string) error `json:"-"`
}

// NewTagAction creates a new tag action.
func NewTagAction(tenantID, userID, sessionID, contentID, contentType string, addedTags, removedTags, previousTags []string) *TagAction {
	desc := fmt.Sprintf("Tag %s: %s", contentType, contentID)
	if len(addedTags) > 0 {
		desc = fmt.Sprintf("Add tags to %s: %s", contentType, contentID)
	}
	if len(removedTags) > 0 {
		if len(addedTags) > 0 {
			desc = fmt.Sprintf("Modify tags on %s: %s", contentType, contentID)
		} else {
			desc = fmt.Sprintf("Remove tags from %s: %s", contentType, contentID)
		}
	}
	return &TagAction{
		BaseAction:   NewBaseAction(ActionTypeTag, desc, tenantID, userID, sessionID),
		ContentID:    contentID,
		ContentType:  contentType,
		AddedTags:    addedTags,
		RemovedTags:  removedTags,
		PreviousTags: previousTags,
	}
}

// Execute performs the tag action.
func (a *TagAction) Execute(ctx context.Context) error {
	if a.TagExecutor != nil {
		if err := a.TagExecutor(ctx, a.ContentID, a.AddedTags, a.RemovedTags); err != nil {
			return fmt.Errorf("execute tag: %w", err)
		}
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses the tag action.
func (a *TagAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	if a.TagExecutor != nil {
		// Reverse: remove what was added, add what was removed
		if err := a.TagExecutor(ctx, a.ContentID, a.RemovedTags, a.AddedTags); err != nil {
			return fmt.Errorf("undo tag: %w", err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs the tag action.
func (a *TagAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	if a.TagExecutor != nil {
		if err := a.TagExecutor(ctx, a.ContentID, a.AddedTags, a.RemovedTags); err != nil {
			return fmt.Errorf("redo tag: %w", err)
		}
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the action to JSON.
func (a *TagAction) ToJSON() ([]byte, error) {
	type tagJSON struct {
		actionJSON
		ContentID    string   `json:"content_id"`
		ContentType  string   `json:"content_type"`
		AddedTags    []string `json:"added_tags"`
		RemovedTags  []string `json:"removed_tags"`
		PreviousTags []string `json:"previous_tags"`
	}
	j := tagJSON{
		actionJSON:   a.BaseAction.toActionJSON(),
		ContentID:    a.ContentID,
		ContentType:  a.ContentType,
		AddedTags:    a.AddedTags,
		RemovedTags:  a.RemovedTags,
		PreviousTags: a.PreviousTags,
	}
	return json.Marshal(j)
}

// MergeAction represents merging multiple review items into one.
type MergeAction struct {
	*BaseAction
	SourceIDs      []string `json:"source_ids"`
	TargetID       string   `json:"target_id"`
	ContentType    string   `json:"content_type"`
	MergedMetadata map[string]interface{} `json:"merged_metadata,omitempty"`
	// MergeExecutor is called with (sourceIDs, targetID).
	MergeExecutor func(ctx context.Context, sourceIDs []string, targetID string) error `json:"-"`
	// SplitExecutor is called to undo a merge.
	SplitExecutor func(ctx context.Context, targetID string, sourceIDs []string) error `json:"-"`
}

// NewMergeAction creates a new merge action.
func NewMergeAction(tenantID, userID, sessionID string, sourceIDs []string, targetID, contentType string) *MergeAction {
	return &MergeAction{
		BaseAction:  NewBaseAction(ActionTypeMerge, fmt.Sprintf("Merge %d %s items into %s", len(sourceIDs), contentType, targetID), tenantID, userID, sessionID),
		SourceIDs:   sourceIDs,
		TargetID:    targetID,
		ContentType: contentType,
	}
}

// Execute performs the merge action.
func (a *MergeAction) Execute(ctx context.Context) error {
	if a.MergeExecutor != nil {
		if err := a.MergeExecutor(ctx, a.SourceIDs, a.TargetID); err != nil {
			return fmt.Errorf("execute merge: %w", err)
		}
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses the merge action.
func (a *MergeAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	if a.SplitExecutor != nil {
		if err := a.SplitExecutor(ctx, a.TargetID, a.SourceIDs); err != nil {
			return fmt.Errorf("undo merge: %w", err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs the merge action.
func (a *MergeAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	if a.MergeExecutor != nil {
		if err := a.MergeExecutor(ctx, a.SourceIDs, a.TargetID); err != nil {
			return fmt.Errorf("redo merge: %w", err)
		}
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the action to JSON.
func (a *MergeAction) ToJSON() ([]byte, error) {
	type mergeJSON struct {
		actionJSON
		SourceIDs      []string               `json:"source_ids"`
		TargetID       string                 `json:"target_id"`
		ContentType    string                 `json:"content_type"`
		MergedMetadata map[string]interface{} `json:"merged_metadata,omitempty"`
	}
	j := mergeJSON{
		actionJSON:     a.BaseAction.toActionJSON(),
		SourceIDs:      a.SourceIDs,
		TargetID:       a.TargetID,
		ContentType:    a.ContentType,
		MergedMetadata: a.MergedMetadata,
	}
	return json.Marshal(j)
}

// SplitAction represents splitting a review item into multiple items.
type SplitAction struct {
	*BaseAction
	SourceID     string   `json:"source_id"`
	TargetIDs    []string `json:"target_ids"`
	ContentType  string   `json:"content_type"`
	SplitDetails map[string]interface{} `json:"split_details,omitempty"`
	// SplitExecutor is called with (sourceID) and returns targetIDs.
	SplitExecutor func(ctx context.Context, sourceID string) ([]string, error) `json:"-"`
	// MergeExecutor is called to undo a split.
	MergeExecutor func(ctx context.Context, targetIDs []string, sourceID string) error `json:"-"`
}

// NewSplitAction creates a new split action.
func NewSplitAction(tenantID, userID, sessionID, sourceID, contentType string) *SplitAction {
	return &SplitAction{
		BaseAction:  NewBaseAction(ActionTypeSplit, fmt.Sprintf("Split %s: %s", contentType, sourceID), tenantID, userID, sessionID),
		SourceID:    sourceID,
		ContentType: contentType,
		TargetIDs:   make([]string, 0),
	}
}

// Execute performs the split action.
func (a *SplitAction) Execute(ctx context.Context) error {
	if a.SplitExecutor != nil {
		targetIDs, err := a.SplitExecutor(ctx, a.SourceID)
		if err != nil {
			return fmt.Errorf("execute split: %w", err)
		}
		a.TargetIDs = targetIDs
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses the split action.
func (a *SplitAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	if a.MergeExecutor != nil {
		if err := a.MergeExecutor(ctx, a.TargetIDs, a.SourceID); err != nil {
			return fmt.Errorf("undo split: %w", err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs the split action.
func (a *SplitAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	if a.SplitExecutor != nil {
		targetIDs, err := a.SplitExecutor(ctx, a.SourceID)
		if err != nil {
			return fmt.Errorf("redo split: %w", err)
		}
		a.TargetIDs = targetIDs
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the action to JSON.
func (a *SplitAction) ToJSON() ([]byte, error) {
	type splitJSON struct {
		actionJSON
		SourceID     string                 `json:"source_id"`
		TargetIDs    []string               `json:"target_ids"`
		ContentType  string                 `json:"content_type"`
		SplitDetails map[string]interface{} `json:"split_details,omitempty"`
	}
	j := splitJSON{
		actionJSON:   a.BaseAction.toActionJSON(),
		SourceID:     a.SourceID,
		TargetIDs:    a.TargetIDs,
		ContentType:  a.ContentType,
		SplitDetails: a.SplitDetails,
	}
	return json.Marshal(j)
}

// BatchAction represents a group of related actions that should be undone/redone together.
type BatchAction struct {
	*BaseAction
	Actions     []UndoableAction `json:"-"`
	ActionCount int              `json:"action_count"`
}

// NewBatchAction creates a new batch action.
func NewBatchAction(tenantID, userID, sessionID, description string, actions []UndoableAction) *BatchAction {
	return &BatchAction{
		BaseAction:  NewBaseAction(ActionTypeBatch, description, tenantID, userID, sessionID),
		Actions:     actions,
		ActionCount: len(actions),
	}
}

// Execute performs all actions in the batch.
func (a *BatchAction) Execute(ctx context.Context) error {
	for i, action := range a.Actions {
		if err := action.Execute(ctx); err != nil {
			// Rollback already executed actions
			for j := i - 1; j >= 0; j-- {
				_ = a.Actions[j].Undo(ctx)
			}
			return fmt.Errorf("execute batch action %d: %w", i, err)
		}
	}
	a.MarkExecuted()
	return nil
}

// Undo reverses all actions in the batch in reverse order.
func (a *BatchAction) Undo(ctx context.Context) error {
	if !a.CanUndo() {
		return ErrActionNotUndoable
	}
	// Undo in reverse order
	for i := len(a.Actions) - 1; i >= 0; i-- {
		if err := a.Actions[i].Undo(ctx); err != nil {
			return fmt.Errorf("undo batch action %d: %w", i, err)
		}
	}
	a.SetState(ActionStateUndone)
	return nil
}

// Redo re-performs all actions in the batch.
func (a *BatchAction) Redo(ctx context.Context) error {
	if !a.CanRedo() {
		return ErrActionNotRedoable
	}
	for i, action := range a.Actions {
		if err := action.Redo(ctx); err != nil {
			// Rollback already redone actions
			for j := i - 1; j >= 0; j-- {
				_ = a.Actions[j].Undo(ctx)
			}
			return fmt.Errorf("redo batch action %d: %w", i, err)
		}
	}
	a.SetState(ActionStateRedone)
	return nil
}

// ToJSON serializes the batch action to JSON.
func (a *BatchAction) ToJSON() ([]byte, error) {
	type batchJSON struct {
		actionJSON
		ActionCount int `json:"action_count"`
	}
	j := batchJSON{
		actionJSON:  a.BaseAction.toActionJSON(),
		ActionCount: a.ActionCount,
	}
	return json.Marshal(j)
}

// GetActions returns the actions in the batch.
func (a *BatchAction) GetActions() []UndoableAction {
	return a.Actions
}
