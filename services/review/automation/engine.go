// Package automation provides the main AutomationEngine for rule-based review automation.
package automation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
)

// EngineConfig holds configuration for the AutomationEngine.
type EngineConfig struct {
	// MaxActionsPerItem limits the number of actions per item to prevent loops.
	MaxActionsPerItem int

	// DefaultDryRun runs all rules in dry-run mode when true.
	DefaultDryRun bool

	// EnableABTesting enables A/B testing for rules.
	EnableABTesting bool

	// ABTestSampleRate is the percentage of items to include in A/B tests (0.0-1.0).
	ABTestSampleRate float64

	// ConflictResolution defines how to handle conflicting rules.
	// Options: "first", "highest_priority", "merge", "fail"
	ConflictResolution string

	// EnableLearning enables rule learning from decisions.
	EnableLearning bool

	// LearningConfig is the configuration for rule learning.
	LearningConfig *LearningConfig
}

// DefaultEngineConfig returns sensible defaults.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		MaxActionsPerItem:  10,
		DefaultDryRun:      false,
		EnableABTesting:    false,
		ABTestSampleRate:   0.1,
		ConflictResolution: "highest_priority",
		EnableLearning:     true,
		LearningConfig:     DefaultLearningConfig(),
	}
}

// EngineMetrics holds Prometheus metrics for the automation engine.
type EngineMetrics struct {
	rulesEvaluated   *prometheus.CounterVec
	rulesMatched     *prometheus.CounterVec
	actionsExecuted  *prometheus.CounterVec
	actionErrors     *prometheus.CounterVec
	evaluationTime   *prometheus.HistogramVec
	executionTime    *prometheus.HistogramVec
	activeRules      *prometheus.GaugeVec
	dryRunActions    *prometheus.CounterVec
	conflictsTotal   prometheus.Counter
	learningDecisions prometheus.Counter
}

// NewEngineMetrics creates metrics for the automation engine.
func NewEngineMetrics(namespace string) *EngineMetrics {
	return &EngineMetrics{
		rulesEvaluated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "rules_evaluated_total",
				Help:      "Total number of rules evaluated",
			},
			[]string{"tenant_id"},
		),
		rulesMatched: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "rules_matched_total",
				Help:      "Total number of rules that matched",
			},
			[]string{"tenant_id", "rule_id", "action_type"},
		),
		actionsExecuted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "actions_executed_total",
				Help:      "Total number of actions executed",
			},
			[]string{"tenant_id", "action_type", "success"},
		),
		actionErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "action_errors_total",
				Help:      "Total number of action execution errors",
			},
			[]string{"tenant_id", "action_type", "error_type"},
		),
		evaluationTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "evaluation_duration_seconds",
				Help:      "Time spent evaluating rules",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
			},
			[]string{"tenant_id"},
		),
		executionTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "execution_duration_seconds",
				Help:      "Time spent executing actions",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"tenant_id", "action_type"},
		),
		activeRules: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "active_rules",
				Help:      "Number of active automation rules",
			},
			[]string{"tenant_id"},
		),
		dryRunActions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "dry_run_actions_total",
				Help:      "Total number of dry-run actions",
			},
			[]string{"tenant_id", "action_type"},
		),
		conflictsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "conflicts_total",
				Help:      "Total number of rule conflicts detected",
			},
		),
		learningDecisions: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "automation",
				Name:      "learning_decisions_total",
				Help:      "Total number of decisions recorded for learning",
			},
		),
	}
}

// RegisterMetrics registers all engine metrics.
func (m *EngineMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.rulesEvaluated,
		m.rulesMatched,
		m.actionsExecuted,
		m.actionErrors,
		m.evaluationTime,
		m.executionTime,
		m.activeRules,
		m.dryRunActions,
		m.conflictsTotal,
		m.learningDecisions,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// AuditEntry represents an entry in the automation audit log.
type AuditEntry struct {
	// ID is the unique identifier for this audit entry.
	ID string `json:"id"`

	// TenantID is the tenant this entry belongs to.
	TenantID string `json:"tenant_id"`

	// Timestamp is when this action occurred.
	Timestamp time.Time `json:"timestamp"`

	// ItemID is the review item that was processed.
	ItemID string `json:"item_id"`

	// RuleID is the rule that was triggered.
	RuleID string `json:"rule_id"`

	// RuleName is the name of the rule.
	RuleName string `json:"rule_name"`

	// Action is the action that was taken.
	Action *Action `json:"action"`

	// Result indicates if the action succeeded.
	Result string `json:"result"` // success, failed, skipped, dry_run

	// Error holds any error message.
	Error string `json:"error,omitempty"`

	// DryRun indicates if this was a dry-run.
	DryRun bool `json:"dry_run"`

	// EvaluationTimeMs is how long rule evaluation took.
	EvaluationTimeMs int64 `json:"evaluation_time_ms"`

	// ExecutionTimeMs is how long action execution took.
	ExecutionTimeMs int64 `json:"execution_time_ms"`
}

// NewAuditEntry creates a new AuditEntry.
func NewAuditEntry(tenantID, itemID string, rule *Rule, action *Action) *AuditEntry {
	return &AuditEntry{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Timestamp: time.Now().UTC(),
		ItemID:    itemID,
		RuleID:    rule.ID,
		RuleName:  rule.Name,
		Action:    action,
		DryRun:    action.DryRun,
	}
}

// AuditStore interface for storing audit entries.
type AuditStore interface {
	// Save saves an audit entry.
	Save(ctx context.Context, entry *AuditEntry) error

	// GetByItem returns audit entries for an item.
	GetByItem(ctx context.Context, tenantID, itemID string) ([]*AuditEntry, error)

	// GetByRule returns audit entries for a rule.
	GetByRule(ctx context.Context, tenantID, ruleID string, limit int) ([]*AuditEntry, error)
}

// InMemoryAuditStore implements AuditStore for testing.
type InMemoryAuditStore struct {
	entries []*AuditEntry
	mu      sync.RWMutex
}

// NewInMemoryAuditStore creates an in-memory audit store.
func NewInMemoryAuditStore() *InMemoryAuditStore {
	return &InMemoryAuditStore{
		entries: make([]*AuditEntry, 0),
	}
}

// Save saves an audit entry.
func (s *InMemoryAuditStore) Save(ctx context.Context, entry *AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

// GetByItem returns audit entries for an item.
func (s *InMemoryAuditStore) GetByItem(ctx context.Context, tenantID, itemID string) ([]*AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AuditEntry, 0)
	for _, e := range s.entries {
		if e.TenantID == tenantID && e.ItemID == itemID {
			result = append(result, e)
		}
	}
	return result, nil
}

// GetByRule returns audit entries for a rule.
func (s *InMemoryAuditStore) GetByRule(ctx context.Context, tenantID, ruleID string, limit int) ([]*AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AuditEntry, 0)
	for i := len(s.entries) - 1; i >= 0 && len(result) < limit; i-- {
		e := s.entries[i]
		if e.TenantID == tenantID && e.RuleID == ruleID {
			result = append(result, e)
		}
	}
	return result, nil
}

// Engine is the main automation engine for processing review items.
type Engine struct {
	config          *EngineConfig
	ruleSets        map[string]*RuleSet // tenantID -> RuleSet
	ruleMetrics     map[string]*RuleMetrics // ruleID -> RuleMetrics
	matcher         *ConditionMatcher
	actionRegistry  *ActionRegistry
	auditStore      AuditStore
	learner         *RuleLearner
	feedbackTracker *FeedbackTracker
	logger          logging.Logger
	metrics         *EngineMetrics
	mu              sync.RWMutex
}

// NewEngine creates a new AutomationEngine.
func NewEngine(config *EngineConfig, logger logging.Logger) *Engine {
	if config == nil {
		config = DefaultEngineConfig()
	}

	var learner *RuleLearner
	if config.EnableLearning {
		learner = NewRuleLearner(config.LearningConfig)
	}

	return &Engine{
		config:          config,
		ruleSets:        make(map[string]*RuleSet),
		ruleMetrics:     make(map[string]*RuleMetrics),
		matcher:         NewConditionMatcher(),
		actionRegistry:  NewActionRegistry(),
		auditStore:      NewInMemoryAuditStore(),
		learner:         learner,
		feedbackTracker: NewFeedbackTracker(),
		logger:          logger.With(logging.F("component", "automation_engine")),
		metrics:         NewEngineMetrics("penfold"),
	}
}

// SetAuditStore sets a custom audit store.
func (e *Engine) SetAuditStore(store AuditStore) {
	e.auditStore = store
}

// SetActionHandlers configures action handlers for the engine.
func (e *Engine) SetActionHandlers(handlers *ActionHandlers) {
	e.actionRegistry = NewActionRegistry()
	e.actionRegistry.Register(NewAcceptExecutor(handlers.Accept))
	e.actionRegistry.Register(NewRejectExecutor(handlers.Reject))
	e.actionRegistry.Register(NewDeferExecutor(handlers.Defer))
	e.actionRegistry.Register(NewEscalateExecutor(handlers.Escalate))
	e.actionRegistry.Register(NewTagExecutor(handlers.Tag))
	e.actionRegistry.Register(NewNotifyExecutor(handlers.Notify))
	e.actionRegistry.Register(NewRouteExecutor(handlers.Route))
}

// GetRuleSet returns the rule set for a tenant.
func (e *Engine) GetRuleSet(tenantID string) *RuleSet {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ruleSets[tenantID]
}

// SetRuleSet sets the rule set for a tenant.
func (e *Engine) SetRuleSet(ruleSet *RuleSet) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ruleSets[ruleSet.TenantID] = ruleSet

	// Initialize metrics for all rules
	for _, rule := range ruleSet.Rules {
		if _, exists := e.ruleMetrics[rule.ID]; !exists {
			e.ruleMetrics[rule.ID] = NewRuleMetrics(rule.ID, rule.TenantID)
		}
	}

	// Update active rules metric
	if e.metrics != nil {
		e.metrics.activeRules.WithLabelValues(ruleSet.TenantID).Set(float64(len(ruleSet.GetEnabledRules())))
	}
}

// AddRule adds a rule to a tenant's rule set.
func (e *Engine) AddRule(rule *Rule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := rule.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRule, err)
	}

	ruleSet, exists := e.ruleSets[rule.TenantID]
	if !exists {
		ruleSet = NewRuleSet(rule.TenantID)
		e.ruleSets[rule.TenantID] = ruleSet
	}

	ruleSet.AddRule(rule)
	e.ruleMetrics[rule.ID] = NewRuleMetrics(rule.ID, rule.TenantID)

	if e.metrics != nil {
		e.metrics.activeRules.WithLabelValues(rule.TenantID).Set(float64(len(ruleSet.GetEnabledRules())))
	}

	e.logger.Info("rule added",
		logging.F("rule_id", rule.ID),
		logging.F("rule_name", rule.Name),
		logging.F("tenant_id", rule.TenantID),
	)

	return nil
}

// RemoveRule removes a rule from a tenant's rule set.
func (e *Engine) RemoveRule(tenantID, ruleID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	ruleSet, exists := e.ruleSets[tenantID]
	if !exists {
		return false
	}

	removed := ruleSet.RemoveRule(ruleID)
	if removed {
		delete(e.ruleMetrics, ruleID)

		if e.metrics != nil {
			e.metrics.activeRules.WithLabelValues(tenantID).Set(float64(len(ruleSet.GetEnabledRules())))
		}

		e.logger.Info("rule removed",
			logging.F("rule_id", ruleID),
			logging.F("tenant_id", tenantID),
		)
	}

	return removed
}

// ProcessRules evaluates all rules against an item and returns actions to take.
func (e *Engine) ProcessRules(ctx context.Context, item *ReviewItem) ([]*Action, error) {
	if item == nil {
		return nil, ErrInvalidRule
	}

	e.mu.RLock()
	ruleSet, exists := e.ruleSets[item.TenantID]
	e.mu.RUnlock()

	if !exists || !ruleSet.Enabled {
		return []*Action{}, nil
	}

	startTime := time.Now()
	actions := make([]*Action, 0)
	enabledRules := ruleSet.GetEnabledRules()

	e.logger.Debug("processing rules",
		logging.F("item_id", item.ID),
		logging.F("tenant_id", item.TenantID),
		logging.F("rule_count", len(enabledRules)),
	)

	for _, rule := range enabledRules {
		// Skip A/B test variants if not in test
		if rule.ABTestGroup != "" && !e.shouldIncludeInABTest(item, rule) {
			continue
		}

		evalStart := time.Now()
		matched := e.matcher.Match(item, rule.Condition)
		evalDuration := time.Since(evalStart)

		// Update metrics
		e.mu.Lock()
		if metrics, ok := e.ruleMetrics[rule.ID]; ok {
			metrics.RecordEvaluation(matched, evalDuration.Milliseconds())
		}
		e.mu.Unlock()

		if e.metrics != nil {
			e.metrics.rulesEvaluated.WithLabelValues(item.TenantID).Inc()
		}

		if matched {
			action := NewAction(rule.ActionType, rule.ID, item.ID)
			action.Parameters = rule.ActionParameters
			action.DryRun = e.config.DefaultDryRun || ruleSet.DefaultDryRun

			// Copy action-specific parameters
			if tags, ok := rule.ActionParameters["tags"].([]string); ok {
				action.Tags = tags
			}
			if duration, ok := rule.ActionParameters["defer_duration"].(time.Duration); ok {
				action.DeferDuration = duration
			}
			if level, ok := rule.ActionParameters["escalation_level"].(int); ok {
				action.EscalationLevel = level
			}
			if target, ok := rule.ActionParameters["route_to"].(string); ok {
				action.RouteTo = target
			}
			if target, ok := rule.ActionParameters["notification_target"].(string); ok {
				action.NotificationTarget = target
			}

			actions = append(actions, action)

			if e.metrics != nil {
				e.metrics.rulesMatched.WithLabelValues(item.TenantID, rule.ID, string(rule.ActionType)).Inc()
			}

			e.logger.Debug("rule matched",
				logging.F("rule_id", rule.ID),
				logging.F("rule_name", rule.Name),
				logging.F("item_id", item.ID),
				logging.F("action_type", rule.ActionType),
			)

			// Check if we should stop processing
			if rule.StopOnMatch {
				break
			}

			// Check max actions limit
			if len(actions) >= ruleSet.MaxActionsPerItem {
				e.logger.Warn("max actions per item reached",
					logging.F("item_id", item.ID),
					logging.F("max_actions", ruleSet.MaxActionsPerItem),
				)
				break
			}
		}
	}

	// Handle conflicts if multiple actions
	if len(actions) > 1 {
		actions = e.resolveConflicts(ruleSet, actions)
	}

	if e.metrics != nil {
		e.metrics.evaluationTime.WithLabelValues(item.TenantID).Observe(time.Since(startTime).Seconds())
	}

	return actions, nil
}

// ExecuteAction executes a single action.
func (e *Engine) ExecuteAction(ctx context.Context, action *Action, item *ReviewItem) error {
	if action == nil {
		return ErrActionFailed
	}

	startTime := time.Now()

	// Execute through the registry
	err := e.actionRegistry.Execute(ctx, action, item)

	execDuration := time.Since(startTime)

	// Get the rule for metrics
	e.mu.Lock()
	if metrics, ok := e.ruleMetrics[action.RuleID]; ok {
		metrics.RecordActionExecution(err == nil)
	}
	e.mu.Unlock()

	// Record audit entry
	if e.auditStore != nil {
		e.mu.RLock()
		var rule *Rule
		if ruleSet, exists := e.ruleSets[item.TenantID]; exists {
			rule = ruleSet.GetRule(action.RuleID)
		}
		e.mu.RUnlock()

		if rule != nil {
			entry := NewAuditEntry(item.TenantID, item.ID, rule, action)
			entry.ExecutionTimeMs = execDuration.Milliseconds()
			if err != nil {
				entry.Result = "failed"
				entry.Error = err.Error()
			} else if action.DryRun {
				entry.Result = "dry_run"
			} else {
				entry.Result = "success"
			}
			if saveErr := e.auditStore.Save(ctx, entry); saveErr != nil {
				e.logger.Warn("failed to save audit entry",
					logging.Err(saveErr),
					logging.F("item_id", item.ID),
				)
			}
		}
	}

	// Update metrics
	if e.metrics != nil {
		success := "true"
		if err != nil {
			success = "false"
			e.metrics.actionErrors.WithLabelValues(item.TenantID, string(action.Type), "execution").Inc()
		}
		e.metrics.actionsExecuted.WithLabelValues(item.TenantID, string(action.Type), success).Inc()
		e.metrics.executionTime.WithLabelValues(item.TenantID, string(action.Type)).Observe(execDuration.Seconds())

		if action.DryRun {
			e.metrics.dryRunActions.WithLabelValues(item.TenantID, string(action.Type)).Inc()
		}
	}

	if err != nil {
		e.logger.Error("action execution failed",
			logging.Err(err),
			logging.F("action_type", action.Type),
			logging.F("item_id", item.ID),
			logging.F("rule_id", action.RuleID),
		)
		return fmt.Errorf("%w: %v", ErrActionFailed, err)
	}

	e.logger.Debug("action executed",
		logging.F("action_type", action.Type),
		logging.F("item_id", item.ID),
		logging.F("dry_run", action.DryRun),
		logging.F("duration_ms", execDuration.Milliseconds()),
	)

	return nil
}

// ExecuteActions executes multiple actions for an item.
func (e *Engine) ExecuteActions(ctx context.Context, actions []*Action, item *ReviewItem) []error {
	errors := make([]error, 0)

	for _, action := range actions {
		if err := e.ExecuteAction(ctx, action, item); err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// ProcessAndExecute is a convenience method that processes rules and executes all resulting actions.
func (e *Engine) ProcessAndExecute(ctx context.Context, item *ReviewItem) ([]*Action, []error) {
	actions, err := e.ProcessRules(ctx, item)
	if err != nil {
		return nil, []error{err}
	}

	if len(actions) == 0 {
		return actions, nil
	}

	errors := e.ExecuteActions(ctx, actions, item)
	return actions, errors
}

// TrainRule creates a new rule based on training examples.
func (e *Engine) TrainRule(ctx context.Context, tenantID, name string, examples []Example) (*Rule, error) {
	if e.learner == nil {
		return nil, fmt.Errorf("learning is not enabled")
	}

	rule, err := e.learner.TrainRule(ctx, tenantID, name, examples)
	if err != nil {
		return nil, err
	}

	// Don't automatically add - let caller decide
	return rule, nil
}

// LearnFromDecision records a user decision for learning.
func (e *Engine) LearnFromDecision(decision *Decision) {
	if e.learner == nil {
		return
	}

	e.learner.LearnFromDecision(decision)

	if e.metrics != nil {
		e.metrics.learningDecisions.Inc()
	}

	e.logger.Debug("decision recorded for learning",
		logging.F("item_id", decision.ItemID),
		logging.F("action", decision.Action),
		logging.F("overrode_automation", decision.OverrodeAutomation),
	)
}

// SuggestRules returns suggested rules based on learned patterns.
func (e *Engine) SuggestRules(ctx context.Context, tenantID string) ([]*RuleSuggestion, error) {
	if e.learner == nil {
		return nil, fmt.Errorf("learning is not enabled")
	}

	return e.learner.SuggestRules(ctx, tenantID)
}

// RecordFeedback records user feedback on an automated action.
func (e *Engine) RecordFeedback(feedback *Feedback) {
	e.feedbackTracker.RecordFeedback(feedback)

	// Update rule metrics
	e.mu.Lock()
	if metrics, ok := e.ruleMetrics[feedback.RuleID]; ok {
		metrics.RecordFeedback(feedback.Positive)
	}
	e.mu.Unlock()

	e.logger.Debug("feedback recorded",
		logging.F("rule_id", feedback.RuleID),
		logging.F("positive", feedback.Positive),
	)
}

// EvaluateRule returns performance metrics for a rule.
func (e *Engine) EvaluateRule(ruleID string) (*RuleMetrics, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	metrics, exists := e.ruleMetrics[ruleID]
	if !exists {
		return nil, ErrRuleNotFound
	}

	// Add feedback score
	feedbackScore := e.feedbackTracker.GetFeedbackScore(ruleID)

	// Clone metrics and add feedback data
	result := *metrics
	if feedbackScore > 0 {
		// Feedback score is available
	}

	return &result, nil
}

// GetRuleMetrics returns metrics for all rules in a tenant.
func (e *Engine) GetRuleMetrics(tenantID string) map[string]*RuleMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]*RuleMetrics)
	for ruleID, metrics := range e.ruleMetrics {
		if metrics.TenantID == tenantID {
			result[ruleID] = metrics
		}
	}
	return result
}

// GetAuditLog returns recent audit entries for an item.
func (e *Engine) GetAuditLog(ctx context.Context, tenantID, itemID string) ([]*AuditEntry, error) {
	if e.auditStore == nil {
		return []*AuditEntry{}, nil
	}
	return e.auditStore.GetByItem(ctx, tenantID, itemID)
}

// shouldIncludeInABTest determines if an item should be included in an A/B test.
func (e *Engine) shouldIncludeInABTest(item *ReviewItem, rule *Rule) bool {
	if !e.config.EnableABTesting || rule.ABTestGroup == "" {
		return true
	}

	// Use a deterministic hash based on item ID and test group
	// to ensure consistent assignment
	hash := 0
	for _, c := range item.ID + rule.ABTestGroup {
		hash = hash*31 + int(c)
	}

	// Check if within sample rate
	threshold := int(e.config.ABTestSampleRate * 100)
	return (hash % 100) < threshold
}

// resolveConflicts handles conflicting actions based on configuration.
func (e *Engine) resolveConflicts(ruleSet *RuleSet, actions []*Action) []*Action {
	if len(actions) <= 1 {
		return actions
	}

	if e.metrics != nil {
		e.metrics.conflictsTotal.Inc()
	}

	switch ruleSet.ConflictResolution {
	case "first":
		// Return only the first action (from highest priority rule)
		return actions[:1]

	case "highest_priority":
		// Already sorted by priority, return first
		return actions[:1]

	case "merge":
		// Return all non-conflicting actions
		return e.mergeActions(actions)

	case "fail":
		// Return no actions when there's a conflict
		e.logger.Warn("conflicting rules detected, skipping all actions",
			logging.F("action_count", len(actions)),
		)
		return []*Action{}

	default:
		// Default to highest priority
		return actions[:1]
	}
}

// mergeActions returns actions that don't conflict with each other.
func (e *Engine) mergeActions(actions []*Action) []*Action {
	// Actions that can coexist: tag, notify
	// Actions that conflict: accept, reject, defer, escalate, route

	result := make([]*Action, 0)
	hasDecision := false // accept, reject, defer

	for _, action := range actions {
		switch action.Type {
		case ActionAccept, ActionReject, ActionDefer:
			if !hasDecision {
				result = append(result, action)
				hasDecision = true
			}
		case ActionTag, ActionNotify:
			// These can always be added
			result = append(result, action)
		case ActionEscalate, ActionRoute:
			// Only one escalate/route action
			hasType := false
			for _, r := range result {
				if r.Type == action.Type {
					hasType = true
					break
				}
			}
			if !hasType {
				result = append(result, action)
			}
		}
	}

	return result
}

// EnableRule enables a rule.
func (e *Engine) EnableRule(tenantID, ruleID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ruleSet, exists := e.ruleSets[tenantID]
	if !exists {
		return ErrRuleNotFound
	}

	rule := ruleSet.GetRule(ruleID)
	if rule == nil {
		return ErrRuleNotFound
	}

	rule.Enabled = true
	rule.UpdatedAt = time.Now().UTC()

	if e.metrics != nil {
		e.metrics.activeRules.WithLabelValues(tenantID).Set(float64(len(ruleSet.GetEnabledRules())))
	}

	return nil
}

// DisableRule disables a rule.
func (e *Engine) DisableRule(tenantID, ruleID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ruleSet, exists := e.ruleSets[tenantID]
	if !exists {
		return ErrRuleNotFound
	}

	rule := ruleSet.GetRule(ruleID)
	if rule == nil {
		return ErrRuleNotFound
	}

	rule.Enabled = false
	rule.UpdatedAt = time.Now().UTC()

	if e.metrics != nil {
		e.metrics.activeRules.WithLabelValues(tenantID).Set(float64(len(ruleSet.GetEnabledRules())))
	}

	return nil
}

// UpdateRulePriority updates a rule's priority.
func (e *Engine) UpdateRulePriority(tenantID, ruleID string, priority int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ruleSet, exists := e.ruleSets[tenantID]
	if !exists {
		return ErrRuleNotFound
	}

	rule := ruleSet.GetRule(ruleID)
	if rule == nil {
		return ErrRuleNotFound
	}

	rule.Priority = priority
	rule.UpdatedAt = time.Now().UTC()
	ruleSet.SortByPriority()

	return nil
}
