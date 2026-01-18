// Package automation provides action execution for automation rules.
package automation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ActionExecutor defines the interface for executing actions.
type ActionExecutor interface {
	// Execute performs the action and returns any error.
	Execute(ctx context.Context, action *Action, item *ReviewItem) error

	// CanExecute checks if this executor can handle the given action type.
	CanExecute(actionType ActionType) bool

	// Name returns the name of this executor.
	Name() string
}

// ActionRegistry manages action executors.
type ActionRegistry struct {
	executors map[ActionType]ActionExecutor
}

// NewActionRegistry creates a new ActionRegistry with built-in executors.
func NewActionRegistry() *ActionRegistry {
	registry := &ActionRegistry{
		executors: make(map[ActionType]ActionExecutor),
	}

	// Register built-in executors
	registry.Register(&AcceptExecutor{})
	registry.Register(&RejectExecutor{})
	registry.Register(&DeferExecutor{})
	registry.Register(&EscalateExecutor{})
	registry.Register(&TagExecutor{})
	registry.Register(&NotifyExecutor{})
	registry.Register(&RouteExecutor{})

	return registry
}

// Register adds an executor for a specific action type.
func (r *ActionRegistry) Register(executor ActionExecutor) {
	for _, actionType := range ValidActionTypes() {
		if executor.CanExecute(actionType) {
			r.executors[actionType] = executor
		}
	}
}

// GetExecutor returns the executor for a given action type.
func (r *ActionRegistry) GetExecutor(actionType ActionType) (ActionExecutor, error) {
	executor, ok := r.executors[actionType]
	if !ok {
		return nil, fmt.Errorf("no executor registered for action type: %s", actionType)
	}
	return executor, nil
}

// Execute finds and runs the appropriate executor for an action.
func (r *ActionRegistry) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	executor, err := r.GetExecutor(action.Type)
	if err != nil {
		return err
	}

	// Skip actual execution in dry-run mode
	if action.DryRun {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("[DRY RUN] Would execute %s action", action.Type)
		return nil
	}

	return executor.Execute(ctx, action, item)
}

// ActionResult represents the result of action execution.
type ActionResult struct {
	// Action is the action that was executed.
	Action *Action `json:"action"`

	// Success indicates if the action was successful.
	Success bool `json:"success"`

	// Error holds any error message.
	Error string `json:"error,omitempty"`

	// ExecutionTimeMs is how long execution took.
	ExecutionTimeMs int64 `json:"execution_time_ms"`

	// Output contains any output from the action.
	Output map[string]interface{} `json:"output,omitempty"`
}

// NewActionResult creates a new ActionResult.
func NewActionResult(action *Action) *ActionResult {
	return &ActionResult{
		Action:  action,
		Success: false,
		Output:  make(map[string]interface{}),
	}
}

// MarkSuccess marks the result as successful.
func (r *ActionResult) MarkSuccess() {
	r.Success = true
	r.Action.Executed = true
	now := time.Now().UTC()
	r.Action.ExecutedAt = &now
}

// MarkFailed marks the result as failed with the given error.
func (r *ActionResult) MarkFailed(err error) {
	r.Success = false
	r.Error = err.Error()
	r.Action.Error = err.Error()
}

// AcceptHandler is a callback for accept actions.
type AcceptHandler func(ctx context.Context, itemID string) error

// RejectHandler is a callback for reject actions.
type RejectHandler func(ctx context.Context, itemID string, reason string) error

// DeferHandler is a callback for defer actions.
type DeferHandler func(ctx context.Context, itemID string, until time.Time) error

// EscalateHandler is a callback for escalate actions.
type EscalateHandler func(ctx context.Context, itemID string, level int) error

// TagHandler is a callback for tag actions.
type TagHandler func(ctx context.Context, itemID string, tags []string) error

// NotifyHandler is a callback for notify actions.
type NotifyHandler func(ctx context.Context, itemID string, target, message string) error

// RouteHandler is a callback for route actions.
type RouteHandler func(ctx context.Context, itemID string, target string) error

// ActionHandlers holds callbacks for action execution.
type ActionHandlers struct {
	Accept   AcceptHandler
	Reject   RejectHandler
	Defer    DeferHandler
	Escalate EscalateHandler
	Tag      TagHandler
	Notify   NotifyHandler
	Route    RouteHandler
}

// NoOpHandlers returns handlers that do nothing (for testing).
func NoOpHandlers() *ActionHandlers {
	return &ActionHandlers{
		Accept:   func(ctx context.Context, itemID string) error { return nil },
		Reject:   func(ctx context.Context, itemID string, reason string) error { return nil },
		Defer:    func(ctx context.Context, itemID string, until time.Time) error { return nil },
		Escalate: func(ctx context.Context, itemID string, level int) error { return nil },
		Tag:      func(ctx context.Context, itemID string, tags []string) error { return nil },
		Notify:   func(ctx context.Context, itemID string, target, message string) error { return nil },
		Route:    func(ctx context.Context, itemID string, target string) error { return nil },
	}
}

// AcceptExecutor handles accept actions.
type AcceptExecutor struct {
	handler AcceptHandler
}

// NewAcceptExecutor creates an AcceptExecutor with the given handler.
func NewAcceptExecutor(handler AcceptHandler) *AcceptExecutor {
	return &AcceptExecutor{handler: handler}
}

// Execute performs the accept action.
func (e *AcceptExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	if e.handler == nil {
		// No-op if no handler configured
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = "Item accepted (no handler configured)"
		return nil
	}

	if err := e.handler(ctx, item.ID); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("accept action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = "Item accepted"
	return nil
}

// CanExecute returns true for accept actions.
func (e *AcceptExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionAccept
}

// Name returns the executor name.
func (e *AcceptExecutor) Name() string {
	return "accept_executor"
}

// RejectExecutor handles reject actions.
type RejectExecutor struct {
	handler RejectHandler
}

// NewRejectExecutor creates a RejectExecutor with the given handler.
func NewRejectExecutor(handler RejectHandler) *RejectExecutor {
	return &RejectExecutor{handler: handler}
}

// Execute performs the reject action.
func (e *RejectExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	reason := action.Message
	if reason == "" {
		if r, ok := action.Parameters["reason"].(string); ok {
			reason = r
		}
	}

	if e.handler == nil {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("Item rejected: %s (no handler configured)", reason)
		return nil
	}

	if err := e.handler(ctx, item.ID, reason); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("reject action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = fmt.Sprintf("Item rejected: %s", reason)
	return nil
}

// CanExecute returns true for reject actions.
func (e *RejectExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionReject
}

// Name returns the executor name.
func (e *RejectExecutor) Name() string {
	return "reject_executor"
}

// DeferExecutor handles defer actions.
type DeferExecutor struct {
	handler DeferHandler
}

// NewDeferExecutor creates a DeferExecutor with the given handler.
func NewDeferExecutor(handler DeferHandler) *DeferExecutor {
	return &DeferExecutor{handler: handler}
}

// Execute performs the defer action.
func (e *DeferExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	duration := action.DeferDuration
	if duration == 0 {
		// Check parameters
		if d, ok := action.Parameters["duration"].(time.Duration); ok {
			duration = d
		} else if h, ok := action.Parameters["hours"].(float64); ok {
			duration = time.Duration(h * float64(time.Hour))
		} else {
			duration = time.Hour // Default
		}
	}

	until := time.Now().UTC().Add(duration)

	if e.handler == nil {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("Item deferred until %s (no handler configured)", until.Format(time.RFC3339))
		return nil
	}

	if err := e.handler(ctx, item.ID, until); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("defer action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = fmt.Sprintf("Item deferred until %s", until.Format(time.RFC3339))
	return nil
}

// CanExecute returns true for defer actions.
func (e *DeferExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionDefer
}

// Name returns the executor name.
func (e *DeferExecutor) Name() string {
	return "defer_executor"
}

// EscalateExecutor handles escalate actions.
type EscalateExecutor struct {
	handler EscalateHandler
}

// NewEscalateExecutor creates an EscalateExecutor with the given handler.
func NewEscalateExecutor(handler EscalateHandler) *EscalateExecutor {
	return &EscalateExecutor{handler: handler}
}

// Execute performs the escalate action.
func (e *EscalateExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	level := action.EscalationLevel
	if level == 0 {
		if l, ok := action.Parameters["level"].(int); ok {
			level = l
		} else if l, ok := action.Parameters["level"].(float64); ok {
			level = int(l)
		} else {
			level = 1 // Default escalation level
		}
	}

	if e.handler == nil {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("Item escalated to level %d (no handler configured)", level)
		return nil
	}

	if err := e.handler(ctx, item.ID, level); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("escalate action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = fmt.Sprintf("Item escalated to level %d", level)
	return nil
}

// CanExecute returns true for escalate actions.
func (e *EscalateExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionEscalate
}

// Name returns the executor name.
func (e *EscalateExecutor) Name() string {
	return "escalate_executor"
}

// TagExecutor handles tag actions.
type TagExecutor struct {
	handler TagHandler
}

// NewTagExecutor creates a TagExecutor with the given handler.
func NewTagExecutor(handler TagHandler) *TagExecutor {
	return &TagExecutor{handler: handler}
}

// Execute performs the tag action.
func (e *TagExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	tags := action.Tags
	if len(tags) == 0 {
		if t, ok := action.Parameters["tags"].([]string); ok {
			tags = t
		} else if t, ok := action.Parameters["tags"].([]interface{}); ok {
			tags = make([]string, 0, len(t))
			for _, v := range t {
				if s, ok := v.(string); ok {
					tags = append(tags, s)
				}
			}
		}
	}

	if len(tags) == 0 {
		return errors.New("no tags specified for tag action")
	}

	if e.handler == nil {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("Tags added: %v (no handler configured)", tags)
		return nil
	}

	if err := e.handler(ctx, item.ID, tags); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("tag action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = fmt.Sprintf("Tags added: %v", tags)
	return nil
}

// CanExecute returns true for tag actions.
func (e *TagExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionTag
}

// Name returns the executor name.
func (e *TagExecutor) Name() string {
	return "tag_executor"
}

// NotifyExecutor handles notify actions.
type NotifyExecutor struct {
	handler NotifyHandler
}

// NewNotifyExecutor creates a NotifyExecutor with the given handler.
func NewNotifyExecutor(handler NotifyHandler) *NotifyExecutor {
	return &NotifyExecutor{handler: handler}
}

// Execute performs the notify action.
func (e *NotifyExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	target := action.NotificationTarget
	if target == "" {
		if t, ok := action.Parameters["target"].(string); ok {
			target = t
		}
	}

	message := action.Message
	if message == "" {
		if m, ok := action.Parameters["message"].(string); ok {
			message = m
		} else {
			message = fmt.Sprintf("Notification for item %s", item.ID)
		}
	}

	if target == "" {
		return errors.New("no notification target specified")
	}

	if e.handler == nil {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("Notification sent to %s (no handler configured)", target)
		return nil
	}

	if err := e.handler(ctx, item.ID, target, message); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("notify action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = fmt.Sprintf("Notification sent to %s", target)
	return nil
}

// CanExecute returns true for notify actions.
func (e *NotifyExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionNotify
}

// Name returns the executor name.
func (e *NotifyExecutor) Name() string {
	return "notify_executor"
}

// RouteExecutor handles route actions.
type RouteExecutor struct {
	handler RouteHandler
}

// NewRouteExecutor creates a RouteExecutor with the given handler.
func NewRouteExecutor(handler RouteHandler) *RouteExecutor {
	return &RouteExecutor{handler: handler}
}

// Execute performs the route action.
func (e *RouteExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	target := action.RouteTo
	if target == "" {
		if t, ok := action.Parameters["target"].(string); ok {
			target = t
		} else if t, ok := action.Parameters["route_to"].(string); ok {
			target = t
		}
	}

	if target == "" {
		return errors.New("no routing target specified")
	}

	if e.handler == nil {
		action.Executed = true
		now := time.Now().UTC()
		action.ExecutedAt = &now
		action.Message = fmt.Sprintf("Item routed to %s (no handler configured)", target)
		return nil
	}

	if err := e.handler(ctx, item.ID, target); err != nil {
		action.Error = err.Error()
		return fmt.Errorf("route action failed: %w", err)
	}

	action.Executed = true
	now := time.Now().UTC()
	action.ExecutedAt = &now
	action.Message = fmt.Sprintf("Item routed to %s", target)
	return nil
}

// CanExecute returns true for route actions.
func (e *RouteExecutor) CanExecute(actionType ActionType) bool {
	return actionType == ActionRoute
}

// Name returns the executor name.
func (e *RouteExecutor) Name() string {
	return "route_executor"
}

// CompositeExecutor allows chaining multiple executors.
type CompositeExecutor struct {
	executors []ActionExecutor
	name      string
}

// NewCompositeExecutor creates a CompositeExecutor with the given executors.
func NewCompositeExecutor(name string, executors ...ActionExecutor) *CompositeExecutor {
	return &CompositeExecutor{
		executors: executors,
		name:      name,
	}
}

// Execute runs all executors in sequence.
func (e *CompositeExecutor) Execute(ctx context.Context, action *Action, item *ReviewItem) error {
	for _, executor := range e.executors {
		if err := executor.Execute(ctx, action, item); err != nil {
			return err
		}
	}
	return nil
}

// CanExecute returns true if any executor can handle the action type.
func (e *CompositeExecutor) CanExecute(actionType ActionType) bool {
	for _, executor := range e.executors {
		if executor.CanExecute(actionType) {
			return true
		}
	}
	return false
}

// Name returns the composite executor name.
func (e *CompositeExecutor) Name() string {
	return e.name
}
