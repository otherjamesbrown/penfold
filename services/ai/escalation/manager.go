package escalation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/ai/router"
)

// EscalationRequest represents a request that may need escalation.
type EscalationRequest struct {
	// ID is the unique identifier for this request.
	ID string

	// OriginalRequest is the original router request.
	OriginalRequest *router.Request

	// Content is the input content.
	Content string

	// TaskType is the type of AI task (extraction, summarization, etc.).
	TaskType string

	// ContentType is the type of content (legal, medical, etc.).
	ContentType string

	// TenantID is the tenant making the request.
	TenantID string

	// CurrentTier is the current model tier.
	CurrentTier TierLevel

	// CurrentModel is the current model being used.
	CurrentModel string

	// MinQuality is the minimum acceptable quality.
	MinQuality float64

	// MaxCost is the maximum acceptable cost.
	MaxCost float64

	// Timeout is the request timeout.
	Timeout time.Duration

	// UserRequestedUpgrade indicates if user explicitly requested escalation.
	UserRequestedUpgrade bool

	// RequestedTier is the tier requested by the user.
	RequestedTier TierLevel

	// EscalationJustification is the user's reason for requesting escalation.
	EscalationJustification string

	// Priority is the request priority.
	Priority int

	// Metadata contains additional request metadata.
	Metadata map[string]interface{}

	// CreatedAt is when the request was created.
	CreatedAt time.Time
}

// EscalationResult represents the result of processing at a specific tier.
type EscalationResult struct {
	// RequestID links back to the original request.
	RequestID string

	// ModelUsed is the model that processed the request.
	ModelUsed string

	// TierUsed is the tier that processed the request.
	TierUsed TierLevel

	// Confidence is the model's confidence in its response.
	Confidence float64

	// Response is the model's response data.
	Response interface{}

	// ProcessingTime is how long processing took.
	ProcessingTime time.Duration

	// TokensUsed is the number of tokens consumed.
	TokensUsed int

	// Cost is the estimated cost of this result.
	Cost float64

	// Error is set if processing failed.
	Error error

	// ErrorCount is the number of errors encountered.
	ErrorCount int

	// TimedOut indicates if the request timed out.
	TimedOut bool

	// ModelQualityScore is the quality score of the model used.
	ModelQualityScore float64

	// Metadata contains additional result metadata.
	Metadata map[string]interface{}
}

// EscalationDecision represents the decision about whether to escalate.
type EscalationDecision struct {
	// ShouldEscalate indicates if escalation should occur.
	ShouldEscalate bool

	// TargetTier is the tier to escalate to.
	TargetTier TierLevel

	// TargetModel is the specific model to use (if specified).
	TargetModel string

	// Reason explains the decision.
	Reason string

	// TriggerResults are the triggers that fired.
	TriggerResults []TriggerResult

	// PolicyDecision is the policy evaluation result.
	PolicyDecision PolicyDecision

	// EstimatedCost is the estimated cost of escalation.
	EstimatedCost float64

	// EstimatedLatency is the estimated additional latency.
	EstimatedLatency time.Duration

	// Metadata contains additional decision metadata.
	Metadata map[string]interface{}
}

// EscalationLog records an escalation event for auditing.
type EscalationLog struct {
	// ID is the unique log entry ID.
	ID string

	// RequestID is the original request ID.
	RequestID string

	// TenantID is the tenant ID.
	TenantID string

	// Timestamp is when the escalation occurred.
	Timestamp time.Time

	// FromTier is the source tier.
	FromTier TierLevel

	// ToTier is the target tier.
	ToTier TierLevel

	// FromModel is the source model.
	FromModel string

	// ToModel is the target model.
	ToModel string

	// TriggerType is the trigger that caused escalation.
	TriggerType TriggerType

	// Reason is the escalation reason.
	Reason string

	// OriginalConfidence is the confidence before escalation.
	OriginalConfidence float64

	// FinalConfidence is the confidence after escalation.
	FinalConfidence float64

	// Cost is the cost of the escalation.
	Cost float64

	// LatencyAdded is the additional latency from escalation.
	LatencyAdded time.Duration

	// Success indicates if the escalation was successful.
	Success bool

	// Metadata contains additional log metadata.
	Metadata map[string]interface{}
}

// ModelProcessor defines the interface for processing requests with a model.
type ModelProcessor interface {
	// Process processes a request and returns the result.
	Process(ctx context.Context, req *EscalationRequest, model string, tier TierLevel) (*EscalationResult, error)
}

// ManagerConfig holds configuration for the EscalationManager.
type ManagerConfig struct {
	// TierConfig configures the model tiers.
	TierConfig *TierConfig

	// DefaultTimeout is the default request timeout.
	DefaultTimeout time.Duration

	// MaxEscalations is the maximum escalation attempts per request.
	MaxEscalations int

	// EnableMetrics enables Prometheus metrics.
	EnableMetrics bool

	// MetricsNamespace is the Prometheus namespace.
	MetricsNamespace string

	// DailyBudget is the daily escalation budget.
	DailyBudget float64

	// RetryOnFailure enables retry on escalation failure.
	RetryOnFailure bool

	// MaxRetries is the maximum retry attempts.
	MaxRetries int

	// EnableAuditLog enables escalation audit logging.
	EnableAuditLog bool
}

// DefaultManagerConfig returns a configuration with sensible defaults.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		TierConfig:       DefaultTierConfig(),
		DefaultTimeout:   30 * time.Second,
		MaxEscalations:   3,
		EnableMetrics:    true,
		MetricsNamespace: "penfold_ai",
		DailyBudget:      100.0,
		RetryOnFailure:   true,
		MaxRetries:       2,
		EnableAuditLog:   true,
	}
}

// EscalationManager manages AI request escalation across model tiers.
type EscalationManager struct {
	mu sync.RWMutex

	config      *ManagerConfig
	logger      logging.Logger
	tierRegistry *TierRegistry
	triggers    *TriggerChain
	policies    *PolicyChain
	processor   ModelProcessor

	// Metrics and stats
	metrics   *EscalationMetrics
	stats     *EscalationStats
	collector *MetricsCollector

	// Audit log storage
	auditMu   sync.Mutex
	auditLogs []EscalationLog
	maxLogs   int
}

// NewEscalationManager creates a new escalation manager.
func NewEscalationManager(config *ManagerConfig, logger logging.Logger) *EscalationManager {
	if config == nil {
		config = DefaultManagerConfig()
	}

	if logger == nil {
		logger = logging.MustGlobal()
	}

	em := &EscalationManager{
		config:       config,
		logger:       logger.With(logging.F("component", "escalation_manager")),
		tierRegistry: NewTierRegistry(config.TierConfig),
		triggers:     DefaultTriggerChain(),
		policies:     DefaultPolicyChain(config.DailyBudget),
		stats:        NewEscalationStats(),
		auditLogs:    make([]EscalationLog, 0),
		maxLogs:      1000,
	}

	if config.EnableMetrics {
		em.metrics = NewEscalationMetrics(config.MetricsNamespace)
		if err := em.metrics.RegisterMetrics(); err != nil {
			em.logger.Warn("Failed to register some metrics", logging.Err(err))
		}
		em.collector = NewMetricsCollector(em.metrics, em.stats)
		em.collector.Start()
	}

	return em
}

// SetProcessor sets the model processor for executing requests.
func (m *EscalationManager) SetProcessor(processor ModelProcessor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processor = processor
}

// SetTriggerChain sets the trigger chain for escalation evaluation.
func (m *EscalationManager) SetTriggerChain(triggers *TriggerChain) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triggers = triggers
}

// SetPolicyChain sets the policy chain for escalation evaluation.
func (m *EscalationManager) SetPolicyChain(policies *PolicyChain) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies = policies
}

// Evaluate evaluates whether a request should be escalated based on its result.
func (m *EscalationManager) Evaluate(request *EscalationRequest, result *EscalationResult) EscalationDecision {
	m.mu.RLock()
	defer m.mu.RUnlock()

	decision := EscalationDecision{
		Metadata: make(map[string]interface{}),
	}

	ctx := context.Background()

	// Evaluate triggers
	triggerResults := m.triggers.Evaluate(ctx, request, result)

	if len(triggerResults) == 0 {
		decision.ShouldEscalate = false
		decision.Reason = "no triggers fired"
		return decision
	}

	// Use the highest severity trigger result
	var primaryTrigger TriggerResult
	for _, tr := range triggerResults {
		if tr.Severity > primaryTrigger.Severity {
			primaryTrigger = tr
		}
	}

	decision.TriggerResults = triggerResults

	// Record trigger in metrics
	if m.collector != nil {
		m.collector.RecordTrigger(primaryTrigger.TriggerType, primaryTrigger.Severity)
	}

	// Evaluate policies
	policyDecision := m.policies.Evaluate(ctx, request, &primaryTrigger)
	decision.PolicyDecision = policyDecision

	// Record policy decision
	if m.collector != nil {
		for _, policy := range m.policies.policies {
			m.collector.RecordPolicy(policy.Type(), policyDecision.Allow)
		}
	}

	if policyDecision.Deny {
		decision.ShouldEscalate = false
		decision.Reason = "policy denied: " + policyDecision.Reason

		if m.collector != nil {
			m.collector.RecordBlocked(policyDecision.Reason, request.CurrentTier)
		}
		return decision
	}

	if !policyDecision.Allow {
		decision.ShouldEscalate = false
		decision.Reason = "no policy allowed escalation"
		return decision
	}

	// Check if already at max tier
	if m.tierRegistry.IsMaxTier(request.CurrentTier) {
		decision.ShouldEscalate = false
		decision.Reason = "already at maximum tier"
		return decision
	}

	// Determine target tier
	targetTier := policyDecision.RecommendedTier
	if targetTier <= request.CurrentTier {
		targetTier = request.CurrentTier + 1
	}

	// Respect max tier from policy
	if targetTier > policyDecision.MaxTier {
		targetTier = policyDecision.MaxTier
	}

	// Get target tier configuration
	tier, err := m.tierRegistry.GetTier(targetTier)
	if err != nil {
		decision.ShouldEscalate = false
		decision.Reason = "target tier not available: " + err.Error()
		return decision
	}

	decision.ShouldEscalate = true
	decision.TargetTier = targetTier
	decision.TargetModel = tier.PreferredModel
	decision.Reason = primaryTrigger.Reason
	decision.EstimatedCost = policyDecision.CostEstimate
	decision.EstimatedLatency = tier.MaxLatency

	// Record confidence observation
	if m.collector != nil {
		m.collector.RecordConfidence(request.CurrentTier, result.Confidence, true)
	}

	return decision
}

// Escalate executes an escalation to a higher tier.
func (m *EscalationManager) Escalate(ctx context.Context, request *EscalationRequest, reason string) (*EscalationResult, error) {
	m.mu.RLock()
	processor := m.processor
	m.mu.RUnlock()

	if processor == nil {
		return nil, errors.New("no model processor configured")
	}

	startTime := time.Now()

	// Get next tier
	nextTier, err := m.tierRegistry.GetNextTier(request.CurrentTier)
	if err != nil {
		return nil, err
	}

	// Set timeout
	timeout := request.Timeout
	if timeout == 0 {
		timeout = m.config.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Process request at new tier
	result, err := processor.Process(ctx, request, nextTier.PreferredModel, nextTier.Level)

	processingTime := time.Since(startTime)

	// Handle result
	if err != nil {
		// Try retry if enabled
		if m.config.RetryOnFailure {
			for i := 0; i < m.config.MaxRetries; i++ {
				result, err = processor.Process(ctx, request, nextTier.PreferredModel, nextTier.Level)
				if err == nil {
					break
				}
			}
		}

		if err != nil {
			m.logEscalation(request, &EscalationResult{
				RequestID:      request.ID,
				TierUsed:       nextTier.Level,
				ModelUsed:      nextTier.PreferredModel,
				Error:          err,
				ProcessingTime: processingTime,
			}, reason, false)

			return nil, err
		}
	}

	// Record successful escalation
	if m.collector != nil {
		m.collector.RecordEscalation(
			request.CurrentTier,
			nextTier.Level,
			TriggerTypeConfidence, // Default trigger type
			request.TenantID,
			request.TaskType,
			processingTime,
			result.Cost,
		)
	}

	// Log escalation
	m.logEscalation(request, result, reason, true)

	return result, nil
}

// GetEscalationPath returns the escalation path for a task type.
func (m *EscalationManager) GetEscalationPath(taskType string) []*ModelTier {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get capabilities required for this task type
	var capabilities []string
	switch taskType {
	case "extraction", "summarization", "classification":
		capabilities = []string{taskType}
	case "reasoning", "code_generation":
		capabilities = []string{taskType}
	default:
		capabilities = []string{}
	}

	path, err := m.tierRegistry.GetEscalationPathForTask(TierLevelLocal, capabilities)
	if err != nil {
		m.logger.Warn("Failed to get escalation path",
			logging.F("task_type", taskType),
			logging.Err(err),
		)
		return nil
	}

	return path
}

// LogEscalation logs an escalation event.
func (m *EscalationManager) LogEscalation(request *EscalationRequest, result *EscalationResult, reason string) {
	m.logEscalation(request, result, reason, result.Error == nil)
}

// logEscalation internal logging implementation.
func (m *EscalationManager) logEscalation(request *EscalationRequest, result *EscalationResult, reason string, success bool) {
	if !m.config.EnableAuditLog {
		return
	}

	log := EscalationLog{
		ID:                 uuid.New().String(),
		RequestID:          request.ID,
		TenantID:           request.TenantID,
		Timestamp:          time.Now(),
		FromTier:           request.CurrentTier,
		ToTier:             result.TierUsed,
		FromModel:          request.CurrentModel,
		ToModel:            result.ModelUsed,
		TriggerType:        TriggerTypeConfidence, // Default
		Reason:             reason,
		OriginalConfidence: 0,
		FinalConfidence:    result.Confidence,
		Cost:               result.Cost,
		LatencyAdded:       result.ProcessingTime,
		Success:            success,
		Metadata:           make(map[string]interface{}),
	}

	m.auditMu.Lock()
	defer m.auditMu.Unlock()

	m.auditLogs = append(m.auditLogs, log)

	// Prune old logs if needed
	if len(m.auditLogs) > m.maxLogs {
		m.auditLogs = m.auditLogs[len(m.auditLogs)-m.maxLogs:]
	}

	m.logger.Info("Escalation logged",
		logging.F("request_id", request.ID),
		logging.F("from_tier", log.FromTier.String()),
		logging.F("to_tier", log.ToTier.String()),
		logging.F("reason", reason),
		logging.F("success", success),
	)
}

// GetAuditLogs returns recent escalation audit logs.
func (m *EscalationManager) GetAuditLogs(limit int) []EscalationLog {
	m.auditMu.Lock()
	defer m.auditMu.Unlock()

	if limit <= 0 || limit > len(m.auditLogs) {
		limit = len(m.auditLogs)
	}

	// Return most recent logs
	start := len(m.auditLogs) - limit
	result := make([]EscalationLog, limit)
	copy(result, m.auditLogs[start:])

	return result
}

// GetAuditLogsForTenant returns audit logs for a specific tenant.
func (m *EscalationManager) GetAuditLogsForTenant(tenantID string, limit int) []EscalationLog {
	m.auditMu.Lock()
	defer m.auditMu.Unlock()

	var result []EscalationLog
	for i := len(m.auditLogs) - 1; i >= 0 && len(result) < limit; i-- {
		if m.auditLogs[i].TenantID == tenantID {
			result = append(result, m.auditLogs[i])
		}
	}

	return result
}

// GetStats returns current escalation statistics.
func (m *EscalationManager) GetStats() EscalationStatsSnapshot {
	return m.stats.Snapshot()
}

// GetTierRegistry returns the tier registry.
func (m *EscalationManager) GetTierRegistry() *TierRegistry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tierRegistry
}

// GetConfig returns the current configuration.
func (m *EscalationManager) GetConfig() *ManagerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig updates the manager configuration.
func (m *EscalationManager) UpdateConfig(config *ManagerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.config = config
	m.tierRegistry = NewTierRegistry(config.TierConfig)

	m.logger.Info("Configuration updated")
}

// ProcessWithEscalation processes a request with automatic escalation.
func (m *EscalationManager) ProcessWithEscalation(ctx context.Context, request *EscalationRequest) (*EscalationResult, error) {
	m.mu.RLock()
	processor := m.processor
	config := m.config
	m.mu.RUnlock()

	if processor == nil {
		return nil, errors.New("no model processor configured")
	}

	// Get starting tier
	startTier, err := m.tierRegistry.GetTier(request.CurrentTier)
	if err != nil {
		startTier, _ = m.tierRegistry.GetDefaultTier()
		request.CurrentTier = startTier.Level
	}

	// Process at current tier
	result, err := processor.Process(ctx, request, startTier.PreferredModel, startTier.Level)
	if err != nil {
		return nil, err
	}

	// Evaluate for escalation
	escalationCount := 0
	for escalationCount < config.MaxEscalations {
		decision := m.Evaluate(request, result)

		if !decision.ShouldEscalate {
			break
		}

		// Update request for next tier
		request.CurrentTier = decision.TargetTier
		request.CurrentModel = decision.TargetModel

		// Escalate
		newResult, err := m.Escalate(ctx, request, decision.Reason)
		if err != nil {
			m.logger.Warn("Escalation failed",
				logging.F("request_id", request.ID),
				logging.F("target_tier", decision.TargetTier.String()),
				logging.Err(err),
			)
			// Return previous result if escalation fails
			break
		}

		result = newResult
		escalationCount++
	}

	return result, nil
}

// Close shuts down the escalation manager.
func (m *EscalationManager) Close() error {
	if m.collector != nil {
		m.collector.Stop()
	}
	return nil
}

// AddTrigger adds a custom trigger to the trigger chain.
func (m *EscalationManager) AddTrigger(trigger Trigger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.triggers.Add(trigger)
}

// AddPolicy adds a custom policy to the policy chain.
func (m *EscalationManager) AddPolicy(policy Policy) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies.Add(policy)
}

// GetMetrics returns the metrics instance (for testing).
func (m *EscalationManager) GetMetrics() *EscalationMetrics {
	return m.metrics
}
