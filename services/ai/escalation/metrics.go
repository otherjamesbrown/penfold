package escalation

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// EscalationMetrics holds Prometheus metrics for the escalation manager.
type EscalationMetrics struct {
	// EscalationsTotal is the total number of escalations by tier and reason.
	EscalationsTotal *prometheus.CounterVec

	// EscalationDuration is the duration of escalation processing.
	EscalationDuration *prometheus.HistogramVec

	// EscalationRate is the current escalation rate per task type.
	EscalationRate *prometheus.GaugeVec

	// SuccessRateByTier tracks success rates by tier.
	SuccessRateByTier *prometheus.GaugeVec

	// CostPerEscalation tracks the cost of escalations.
	CostPerEscalation *prometheus.HistogramVec

	// LatencyImpact tracks the latency impact of escalations.
	LatencyImpact *prometheus.HistogramVec

	// TriggersFired tracks which triggers fired.
	TriggersFired *prometheus.CounterVec

	// PoliciesEvaluated tracks policy evaluations.
	PoliciesEvaluated *prometheus.CounterVec

	// BudgetUtilization tracks budget usage.
	BudgetUtilization *prometheus.GaugeVec

	// ConfidenceDistribution tracks confidence scores.
	ConfidenceDistribution *prometheus.HistogramVec

	// TierDistribution tracks the distribution of requests across tiers.
	TierDistribution *prometheus.GaugeVec

	// EscalationBlocked tracks blocked escalations.
	EscalationBlocked *prometheus.CounterVec
}

// NewEscalationMetrics creates new Prometheus metrics for the escalation manager.
func NewEscalationMetrics(namespace string) *EscalationMetrics {
	return &EscalationMetrics{
		EscalationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "total",
				Help:      "Total number of escalations by source tier, target tier, and trigger",
			},
			[]string{"source_tier", "target_tier", "trigger_type", "tenant_id"},
		),

		EscalationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "duration_seconds",
				Help:      "Duration of escalation processing in seconds",
				Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
			},
			[]string{"tier", "status"},
		),

		EscalationRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "rate",
				Help:      "Current escalation rate (escalations per minute) by task type",
			},
			[]string{"task_type", "tenant_id"},
		),

		SuccessRateByTier: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "success_rate",
				Help:      "Success rate of requests by tier (0.0 to 1.0)",
			},
			[]string{"tier"},
		),

		CostPerEscalation: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "cost_usd",
				Help:      "Cost per escalation in USD",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
			},
			[]string{"source_tier", "target_tier"},
		),

		LatencyImpact: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "latency_impact_seconds",
				Help:      "Additional latency caused by escalation in seconds",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
			},
			[]string{"source_tier", "target_tier"},
		),

		TriggersFired: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "triggers_fired_total",
				Help:      "Total count of trigger activations by type",
			},
			[]string{"trigger_type", "severity"},
		),

		PoliciesEvaluated: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "policies_evaluated_total",
				Help:      "Total count of policy evaluations by type and decision",
			},
			[]string{"policy_type", "decision"},
		),

		BudgetUtilization: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "budget_utilization_percent",
				Help:      "Current budget utilization percentage",
			},
			[]string{"tenant_id", "period"},
		),

		ConfidenceDistribution: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "confidence_score",
				Help:      "Distribution of confidence scores before escalation",
				Buckets:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			},
			[]string{"tier", "escalated"},
		),

		TierDistribution: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "tier_distribution",
				Help:      "Distribution of requests across tiers",
			},
			[]string{"tier"},
		),

		EscalationBlocked: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "escalation",
				Name:      "blocked_total",
				Help:      "Total count of blocked escalations by reason",
			},
			[]string{"reason", "tier"},
		),
	}
}

// RegisterMetrics registers all escalation metrics with Prometheus.
func (m *EscalationMetrics) RegisterMetrics() error {
	collectors := []prometheus.Collector{
		m.EscalationsTotal,
		m.EscalationDuration,
		m.EscalationRate,
		m.SuccessRateByTier,
		m.CostPerEscalation,
		m.LatencyImpact,
		m.TriggersFired,
		m.PoliciesEvaluated,
		m.BudgetUtilization,
		m.ConfidenceDistribution,
		m.TierDistribution,
		m.EscalationBlocked,
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

// EscalationStats holds aggregated statistics about escalations.
type EscalationStats struct {
	mu sync.RWMutex

	// EscalationCounts tracks escalation counts by tier.
	EscalationCounts map[TierLevel]int64

	// SuccessCounts tracks successful requests by tier.
	SuccessCounts map[TierLevel]int64

	// FailureCounts tracks failed requests by tier.
	FailureCounts map[TierLevel]int64

	// TotalCost tracks total escalation cost.
	TotalCost float64

	// TotalLatencyAdded tracks total latency added by escalations.
	TotalLatencyAdded time.Duration

	// WindowSize is the time window for rate calculations.
	WindowSize time.Duration

	// RecentEscalations tracks recent escalations for rate calculation.
	RecentEscalations []escalationRecord

	// TriggerCounts tracks trigger activation counts.
	TriggerCounts map[TriggerType]int64

	// PolicyDecisionCounts tracks policy decisions.
	PolicyDecisionCounts map[PolicyType]map[string]int64

	// LastUpdated tracks when stats were last updated.
	LastUpdated time.Time
}

// escalationRecord tracks a single escalation for rate calculation.
type escalationRecord struct {
	Timestamp time.Time
	TaskType  string
	TenantID  string
	FromTier  TierLevel
	ToTier    TierLevel
}

// NewEscalationStats creates new escalation statistics tracking.
func NewEscalationStats() *EscalationStats {
	return &EscalationStats{
		EscalationCounts:     make(map[TierLevel]int64),
		SuccessCounts:        make(map[TierLevel]int64),
		FailureCounts:        make(map[TierLevel]int64),
		WindowSize:           5 * time.Minute,
		RecentEscalations:    make([]escalationRecord, 0),
		TriggerCounts:        make(map[TriggerType]int64),
		PolicyDecisionCounts: make(map[PolicyType]map[string]int64),
		LastUpdated:          time.Now(),
	}
}

// RecordEscalation records an escalation event.
func (s *EscalationStats) RecordEscalation(fromTier, toTier TierLevel, taskType, tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.EscalationCounts[toTier]++
	s.RecentEscalations = append(s.RecentEscalations, escalationRecord{
		Timestamp: time.Now(),
		TaskType:  taskType,
		TenantID:  tenantID,
		FromTier:  fromTier,
		ToTier:    toTier,
	})
	s.LastUpdated = time.Now()
	s.pruneOldRecords()
}

// RecordSuccess records a successful request at a tier.
func (s *EscalationStats) RecordSuccess(tier TierLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SuccessCounts[tier]++
	s.LastUpdated = time.Now()
}

// RecordFailure records a failed request at a tier.
func (s *EscalationStats) RecordFailure(tier TierLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FailureCounts[tier]++
	s.LastUpdated = time.Now()
}

// RecordCost records the cost of an escalation.
func (s *EscalationStats) RecordCost(cost float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalCost += cost
	s.LastUpdated = time.Now()
}

// RecordLatency records the latency added by an escalation.
func (s *EscalationStats) RecordLatency(latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalLatencyAdded += latency
	s.LastUpdated = time.Now()
}

// RecordTrigger records a trigger activation.
func (s *EscalationStats) RecordTrigger(triggerType TriggerType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TriggerCounts[triggerType]++
	s.LastUpdated = time.Now()
}

// RecordPolicyDecision records a policy decision.
func (s *EscalationStats) RecordPolicyDecision(policyType PolicyType, decision string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.PolicyDecisionCounts[policyType] == nil {
		s.PolicyDecisionCounts[policyType] = make(map[string]int64)
	}
	s.PolicyDecisionCounts[policyType][decision]++
	s.LastUpdated = time.Now()
}

// GetEscalationRate returns the escalation rate for a specific task type.
func (s *EscalationStats) GetEscalationRate(taskType string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.pruneOldRecordsLocked()

	count := 0
	for _, record := range s.RecentEscalations {
		if record.TaskType == taskType {
			count++
		}
	}

	// Calculate per-minute rate
	return float64(count) / s.WindowSize.Minutes()
}

// GetSuccessRate returns the success rate for a tier.
func (s *EscalationStats) GetSuccessRate(tier TierLevel) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	success := s.SuccessCounts[tier]
	failure := s.FailureCounts[tier]
	total := success + failure

	if total == 0 {
		return 1.0 // Assume success if no data
	}

	return float64(success) / float64(total)
}

// GetTierDistribution returns the distribution of escalations by tier.
func (s *EscalationStats) GetTierDistribution() map[TierLevel]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total int64
	for _, count := range s.EscalationCounts {
		total += count
	}

	distribution := make(map[TierLevel]float64)
	if total == 0 {
		return distribution
	}

	for tier, count := range s.EscalationCounts {
		distribution[tier] = float64(count) / float64(total)
	}

	return distribution
}

// GetTotalCost returns the total cost of escalations.
func (s *EscalationStats) GetTotalCost() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalCost
}

// GetAvgLatencyImpact returns the average latency impact per escalation.
func (s *EscalationStats) GetAvgLatencyImpact() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total int64
	for _, count := range s.EscalationCounts {
		total += count
	}

	if total == 0 {
		return 0
	}

	return time.Duration(int64(s.TotalLatencyAdded) / total)
}

// pruneOldRecords removes old records outside the window.
func (s *EscalationStats) pruneOldRecords() {
	cutoff := time.Now().Add(-s.WindowSize)
	var newRecords []escalationRecord

	for _, record := range s.RecentEscalations {
		if record.Timestamp.After(cutoff) {
			newRecords = append(newRecords, record)
		}
	}

	s.RecentEscalations = newRecords
}

// pruneOldRecordsLocked is a read-lock compatible version of pruneOldRecords.
func (s *EscalationStats) pruneOldRecordsLocked() {
	// Note: This is called from GetEscalationRate which holds RLock
	// In a real implementation, you might want a separate goroutine for pruning
}

// Reset resets all statistics.
func (s *EscalationStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.EscalationCounts = make(map[TierLevel]int64)
	s.SuccessCounts = make(map[TierLevel]int64)
	s.FailureCounts = make(map[TierLevel]int64)
	s.TotalCost = 0
	s.TotalLatencyAdded = 0
	s.RecentEscalations = make([]escalationRecord, 0)
	s.TriggerCounts = make(map[TriggerType]int64)
	s.PolicyDecisionCounts = make(map[PolicyType]map[string]int64)
	s.LastUpdated = time.Now()
}

// Snapshot returns a copy of the current statistics.
func (s *EscalationStats) Snapshot() EscalationStatsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return EscalationStatsSnapshot{
		EscalationCounts:  copyTierMap(s.EscalationCounts),
		SuccessCounts:     copyTierMap(s.SuccessCounts),
		FailureCounts:     copyTierMap(s.FailureCounts),
		TotalCost:         s.TotalCost,
		TotalLatencyAdded: s.TotalLatencyAdded,
		LastUpdated:       s.LastUpdated,
	}
}

// EscalationStatsSnapshot is an immutable snapshot of escalation statistics.
type EscalationStatsSnapshot struct {
	EscalationCounts  map[TierLevel]int64
	SuccessCounts     map[TierLevel]int64
	FailureCounts     map[TierLevel]int64
	TotalCost         float64
	TotalLatencyAdded time.Duration
	LastUpdated       time.Time
}

// copyTierMap creates a copy of a tier-indexed map.
func copyTierMap(m map[TierLevel]int64) map[TierLevel]int64 {
	result := make(map[TierLevel]int64)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// MetricsCollector collects and updates Prometheus metrics from escalation stats.
type MetricsCollector struct {
	metrics *EscalationMetrics
	stats   *EscalationStats
	mu      sync.RWMutex

	// UpdateInterval is how often to update gauge metrics.
	UpdateInterval time.Duration

	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(metrics *EscalationMetrics, stats *EscalationStats) *MetricsCollector {
	return &MetricsCollector{
		metrics:        metrics,
		stats:          stats,
		UpdateInterval: 30 * time.Second,
		stopChan:       make(chan struct{}),
	}
}

// Start starts the background metrics collection.
func (c *MetricsCollector) Start() {
	c.wg.Add(1)
	go c.collectLoop()
}

// Stop stops the background metrics collection.
func (c *MetricsCollector) Stop() {
	close(c.stopChan)
	c.wg.Wait()
}

// collectLoop periodically updates gauge metrics.
func (c *MetricsCollector) collectLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.updateGauges()
		}
	}
}

// updateGauges updates gauge metrics from current stats.
func (c *MetricsCollector) updateGauges() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.metrics == nil || c.stats == nil {
		return
	}

	// Update success rates
	for tier := TierLevelLocal; tier <= TierLevelCloudPremium; tier++ {
		successRate := c.stats.GetSuccessRate(tier)
		c.metrics.SuccessRateByTier.WithLabelValues(tier.String()).Set(successRate)
	}

	// Update tier distribution
	distribution := c.stats.GetTierDistribution()
	for tier, ratio := range distribution {
		c.metrics.TierDistribution.WithLabelValues(tier.String()).Set(ratio)
	}
}

// RecordEscalation records an escalation event in both stats and metrics.
func (c *MetricsCollector) RecordEscalation(
	fromTier, toTier TierLevel,
	triggerType TriggerType,
	tenantID, taskType string,
	duration time.Duration,
	cost float64,
) {
	// Update stats
	c.stats.RecordEscalation(fromTier, toTier, taskType, tenantID)
	c.stats.RecordCost(cost)

	// Update counter metrics
	c.metrics.EscalationsTotal.WithLabelValues(
		fromTier.String(),
		toTier.String(),
		string(triggerType),
		tenantID,
	).Inc()

	// Update histogram metrics
	c.metrics.EscalationDuration.WithLabelValues(toTier.String(), "success").Observe(duration.Seconds())
	c.metrics.CostPerEscalation.WithLabelValues(fromTier.String(), toTier.String()).Observe(cost)
}

// RecordTrigger records a trigger activation in both stats and metrics.
func (c *MetricsCollector) RecordTrigger(triggerType TriggerType, severity float64) {
	c.stats.RecordTrigger(triggerType)

	severityLevel := "low"
	if severity > 0.7 {
		severityLevel = "high"
	} else if severity > 0.4 {
		severityLevel = "medium"
	}

	c.metrics.TriggersFired.WithLabelValues(string(triggerType), severityLevel).Inc()
}

// RecordPolicy records a policy evaluation in both stats and metrics.
func (c *MetricsCollector) RecordPolicy(policyType PolicyType, allowed bool) {
	decision := "deny"
	if allowed {
		decision = "allow"
	}

	c.stats.RecordPolicyDecision(policyType, decision)
	c.metrics.PoliciesEvaluated.WithLabelValues(string(policyType), decision).Inc()
}

// RecordBlocked records a blocked escalation.
func (c *MetricsCollector) RecordBlocked(reason string, tier TierLevel) {
	c.metrics.EscalationBlocked.WithLabelValues(reason, tier.String()).Inc()
}

// RecordConfidence records a confidence score observation.
func (c *MetricsCollector) RecordConfidence(tier TierLevel, confidence float64, escalated bool) {
	escalatedStr := "false"
	if escalated {
		escalatedStr = "true"
	}
	c.metrics.ConfidenceDistribution.WithLabelValues(tier.String(), escalatedStr).Observe(confidence)
}
