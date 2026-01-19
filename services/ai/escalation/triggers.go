package escalation

import (
	"context"
	"time"
)

// TriggerType identifies the type of escalation trigger.
type TriggerType string

const (
	// TriggerTypeConfidence triggers escalation based on low confidence.
	TriggerTypeConfidence TriggerType = "confidence"

	// TriggerTypeError triggers escalation based on model errors/failures.
	TriggerTypeError TriggerType = "error"

	// TriggerTypeContent triggers escalation based on content characteristics.
	TriggerTypeContent TriggerType = "content"

	// TriggerTypeUser triggers escalation based on user request.
	TriggerTypeUser TriggerType = "user"

	// TriggerTypeTimeout triggers escalation based on request timeout.
	TriggerTypeTimeout TriggerType = "timeout"

	// TriggerTypeQuality triggers escalation based on quality requirements.
	TriggerTypeQuality TriggerType = "quality"
)

// TriggerResult represents the result of evaluating an escalation trigger.
type TriggerResult struct {
	// ShouldEscalate indicates if escalation should occur.
	ShouldEscalate bool

	// TriggerType is the type of trigger that fired.
	TriggerType TriggerType

	// Reason describes why escalation was triggered.
	Reason string

	// Confidence is the confidence level that triggered escalation (if applicable).
	Confidence float64

	// Severity indicates the urgency of escalation (0.0 to 1.0).
	Severity float64

	// RecommendedTier is the suggested tier to escalate to.
	RecommendedTier TierLevel

	// Metadata contains additional trigger-specific information.
	Metadata map[string]interface{}
}

// Trigger defines the interface for escalation triggers.
type Trigger interface {
	// Type returns the trigger type.
	Type() TriggerType

	// Evaluate evaluates whether escalation should be triggered.
	Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult

	// Name returns a human-readable name for this trigger.
	Name() string
}

// ConfidenceTrigger triggers escalation when confidence is below a threshold.
type ConfidenceTrigger struct {
	// Threshold is the minimum confidence required to avoid escalation.
	Threshold float64

	// CriticalThreshold is the confidence below which immediate escalation to premium occurs.
	CriticalThreshold float64

	// MinSampleSize is the minimum number of samples before triggering.
	MinSampleSize int
}

// NewConfidenceTrigger creates a new confidence-based escalation trigger.
func NewConfidenceTrigger(threshold, criticalThreshold float64) *ConfidenceTrigger {
	return &ConfidenceTrigger{
		Threshold:         threshold,
		CriticalThreshold: criticalThreshold,
		MinSampleSize:     1,
	}
}

// Type returns the trigger type.
func (t *ConfidenceTrigger) Type() TriggerType {
	return TriggerTypeConfidence
}

// Name returns a human-readable name for this trigger.
func (t *ConfidenceTrigger) Name() string {
	return "Confidence Threshold Trigger"
}

// Evaluate checks if the result confidence is below the threshold.
func (t *ConfidenceTrigger) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult {
	tr := TriggerResult{
		TriggerType: TriggerTypeConfidence,
		Confidence:  result.Confidence,
		Metadata:    make(map[string]interface{}),
	}

	if result.Confidence >= t.Threshold {
		tr.ShouldEscalate = false
		tr.Reason = "confidence above threshold"
		return tr
	}

	tr.ShouldEscalate = true
	tr.Metadata["threshold"] = t.Threshold
	tr.Metadata["actual_confidence"] = result.Confidence

	// Determine severity and recommended tier based on how far below threshold
	if result.Confidence < t.CriticalThreshold {
		tr.Severity = 1.0
		tr.Reason = "confidence critically low, immediate escalation to premium recommended"
		tr.RecommendedTier = TierLevelCloudPremium
	} else {
		// Linear severity between critical and threshold
		range_ := t.Threshold - t.CriticalThreshold
		distance := t.Threshold - result.Confidence
		tr.Severity = distance / range_
		tr.Reason = "confidence below acceptable threshold"
		tr.RecommendedTier = TierLevelCloudStandard
	}

	return tr
}

// ErrorTrigger triggers escalation when model errors occur.
type ErrorTrigger struct {
	// MaxConsecutiveErrors is the max errors before escalation.
	MaxConsecutiveErrors int

	// IncludeTimeouts includes timeout errors as triggers.
	IncludeTimeouts bool

	// RetryableErrors are error types that should trigger escalation.
	RetryableErrors []string
}

// NewErrorTrigger creates a new error-based escalation trigger.
func NewErrorTrigger(maxErrors int) *ErrorTrigger {
	return &ErrorTrigger{
		MaxConsecutiveErrors: maxErrors,
		IncludeTimeouts:      true,
		RetryableErrors:      []string{"model_unavailable", "rate_limit", "context_length_exceeded"},
	}
}

// Type returns the trigger type.
func (t *ErrorTrigger) Type() TriggerType {
	return TriggerTypeError
}

// Name returns a human-readable name for this trigger.
func (t *ErrorTrigger) Name() string {
	return "Error Trigger"
}

// Evaluate checks if escalation should be triggered due to errors.
func (t *ErrorTrigger) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult {
	tr := TriggerResult{
		TriggerType: TriggerTypeError,
		Metadata:    make(map[string]interface{}),
	}

	if result.Error == nil {
		tr.ShouldEscalate = false
		tr.Reason = "no error occurred"
		return tr
	}

	tr.ShouldEscalate = true
	tr.Reason = "model error: " + result.Error.Error()
	tr.Severity = 0.8
	tr.RecommendedTier = TierLevelCloudStandard

	// Check if it's a critical error that needs premium
	if isContextLengthError(result.Error) {
		tr.Severity = 1.0
		tr.RecommendedTier = TierLevelCloudPremium
		tr.Reason = "context length exceeded, escalating to model with larger context"
	}

	tr.Metadata["error"] = result.Error.Error()
	tr.Metadata["error_count"] = result.ErrorCount

	return tr
}

// isContextLengthError checks if the error is related to context length.
func isContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "context length") ||
		contains(errStr, "token limit") ||
		contains(errStr, "too long")
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > 0 && len(substr) > 0 &&
			(s[0:len(substr)] == substr || contains(s[1:], substr))))
}

// ContentTrigger triggers escalation based on content characteristics.
type ContentTrigger struct {
	// ComplexityThreshold is the minimum content complexity score for escalation.
	ComplexityThreshold float64

	// LengthThreshold is the content length in characters that triggers escalation.
	LengthThreshold int

	// SensitiveContentTypes are content types that require premium models.
	SensitiveContentTypes []string

	// ComplexTaskTypes are task types that benefit from escalation.
	ComplexTaskTypes []string
}

// NewContentTrigger creates a new content-based escalation trigger.
func NewContentTrigger(complexityThreshold float64, lengthThreshold int) *ContentTrigger {
	return &ContentTrigger{
		ComplexityThreshold:   complexityThreshold,
		LengthThreshold:       lengthThreshold,
		SensitiveContentTypes: []string{"legal", "medical", "financial"},
		ComplexTaskTypes:      []string{"reasoning", "code_generation", "multi_step"},
	}
}

// Type returns the trigger type.
func (t *ContentTrigger) Type() TriggerType {
	return TriggerTypeContent
}

// Name returns a human-readable name for this trigger.
func (t *ContentTrigger) Name() string {
	return "Content Complexity Trigger"
}

// Evaluate checks if escalation should be triggered based on content.
func (t *ContentTrigger) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult {
	tr := TriggerResult{
		TriggerType: TriggerTypeContent,
		Metadata:    make(map[string]interface{}),
	}

	// Check content length
	contentLength := len(request.Content)
	if contentLength > t.LengthThreshold {
		tr.ShouldEscalate = true
		tr.Reason = "content length exceeds threshold"
		tr.Severity = 0.6
		tr.RecommendedTier = TierLevelCloudStandard
		tr.Metadata["content_length"] = contentLength
		tr.Metadata["threshold"] = t.LengthThreshold
	}

	// Check for sensitive content types
	for _, sensitiveType := range t.SensitiveContentTypes {
		if request.ContentType == sensitiveType {
			tr.ShouldEscalate = true
			tr.Reason = "sensitive content type requires premium processing"
			tr.Severity = 0.9
			tr.RecommendedTier = TierLevelCloudPremium
			tr.Metadata["content_type"] = sensitiveType
			return tr
		}
	}

	// Check for complex task types
	for _, complexType := range t.ComplexTaskTypes {
		if request.TaskType == complexType {
			tr.ShouldEscalate = true
			tr.Reason = "complex task type benefits from higher tier model"
			tr.Severity = 0.7
			tr.RecommendedTier = TierLevelCloudStandard
			tr.Metadata["task_type"] = complexType
			return tr
		}
	}

	if !tr.ShouldEscalate {
		tr.Reason = "content does not require escalation"
	}

	return tr
}

// UserTrigger triggers escalation based on explicit user request.
type UserTrigger struct {
	// AllowedTiers are the tiers users can request.
	AllowedTiers []TierLevel

	// RequireJustification requires users to provide a reason.
	RequireJustification bool
}

// NewUserTrigger creates a new user-requested escalation trigger.
func NewUserTrigger() *UserTrigger {
	return &UserTrigger{
		AllowedTiers:         []TierLevel{TierLevelCloudStandard, TierLevelCloudPremium},
		RequireJustification: false,
	}
}

// Type returns the trigger type.
func (t *UserTrigger) Type() TriggerType {
	return TriggerTypeUser
}

// Name returns a human-readable name for this trigger.
func (t *UserTrigger) Name() string {
	return "User Request Trigger"
}

// Evaluate checks if escalation was explicitly requested by user.
func (t *UserTrigger) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult {
	tr := TriggerResult{
		TriggerType: TriggerTypeUser,
		Metadata:    make(map[string]interface{}),
	}

	if !request.UserRequestedUpgrade {
		tr.ShouldEscalate = false
		tr.Reason = "no user escalation request"
		return tr
	}

	// Check if requested tier is allowed
	requestedTier := request.RequestedTier
	allowed := false
	for _, tier := range t.AllowedTiers {
		if tier == requestedTier {
			allowed = true
			break
		}
	}

	if !allowed {
		tr.ShouldEscalate = false
		tr.Reason = "requested tier not allowed"
		tr.Metadata["requested_tier"] = requestedTier
		return tr
	}

	tr.ShouldEscalate = true
	tr.Reason = "user explicitly requested escalation"
	tr.Severity = 0.5 // User requests are normal priority
	tr.RecommendedTier = requestedTier
	tr.Metadata["user_justification"] = request.EscalationJustification

	return tr
}

// TimeoutTrigger triggers escalation when requests timeout.
type TimeoutTrigger struct {
	// TimeoutThreshold is the duration after which to consider escalation.
	TimeoutThreshold time.Duration

	// MaxRetries is the number of retries before escalating.
	MaxRetries int
}

// NewTimeoutTrigger creates a new timeout-based escalation trigger.
func NewTimeoutTrigger(threshold time.Duration) *TimeoutTrigger {
	return &TimeoutTrigger{
		TimeoutThreshold: threshold,
		MaxRetries:       2,
	}
}

// Type returns the trigger type.
func (t *TimeoutTrigger) Type() TriggerType {
	return TriggerTypeTimeout
}

// Name returns a human-readable name for this trigger.
func (t *TimeoutTrigger) Name() string {
	return "Timeout Trigger"
}

// Evaluate checks if escalation should be triggered due to timeouts.
func (t *TimeoutTrigger) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult {
	tr := TriggerResult{
		TriggerType: TriggerTypeTimeout,
		Metadata:    make(map[string]interface{}),
	}

	if !result.TimedOut {
		tr.ShouldEscalate = false
		tr.Reason = "request did not timeout"
		return tr
	}

	tr.ShouldEscalate = true
	tr.Reason = "request timed out"
	tr.Severity = 0.7
	tr.RecommendedTier = TierLevelCloudStandard
	tr.Metadata["processing_time"] = result.ProcessingTime
	tr.Metadata["timeout_threshold"] = t.TimeoutThreshold

	return tr
}

// QualityTrigger triggers escalation based on quality requirements.
type QualityTrigger struct {
	// MinQualityScore is the minimum acceptable quality score.
	MinQualityScore float64

	// TaskQualityRequirements maps task types to minimum quality scores.
	TaskQualityRequirements map[string]float64
}

// NewQualityTrigger creates a new quality-based escalation trigger.
func NewQualityTrigger(minQuality float64) *QualityTrigger {
	return &QualityTrigger{
		MinQualityScore: minQuality,
		TaskQualityRequirements: map[string]float64{
			"extraction":     0.8,
			"classification": 0.75,
			"summarization":  0.7,
			"embedding":      0.65,
		},
	}
}

// Type returns the trigger type.
func (t *QualityTrigger) Type() TriggerType {
	return TriggerTypeQuality
}

// Name returns a human-readable name for this trigger.
func (t *QualityTrigger) Name() string {
	return "Quality Requirement Trigger"
}

// Evaluate checks if escalation should be triggered based on quality requirements.
func (t *QualityTrigger) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) TriggerResult {
	tr := TriggerResult{
		TriggerType: TriggerTypeQuality,
		Metadata:    make(map[string]interface{}),
	}

	// Get required quality for this task type
	requiredQuality := t.MinQualityScore
	if taskQuality, ok := t.TaskQualityRequirements[request.TaskType]; ok {
		requiredQuality = taskQuality
	}

	// Check if request specifies minimum quality
	if request.MinQuality > requiredQuality {
		requiredQuality = request.MinQuality
	}

	// Check if current tier meets quality requirements
	if result.ModelQualityScore >= requiredQuality {
		tr.ShouldEscalate = false
		tr.Reason = "quality requirements met"
		return tr
	}

	tr.ShouldEscalate = true
	tr.Reason = "quality requirements not met by current tier"
	tr.Severity = (requiredQuality - result.ModelQualityScore) / requiredQuality
	tr.Metadata["required_quality"] = requiredQuality
	tr.Metadata["actual_quality"] = result.ModelQualityScore

	// Recommend tier based on required quality
	if requiredQuality > 0.9 {
		tr.RecommendedTier = TierLevelCloudPremium
	} else {
		tr.RecommendedTier = TierLevelCloudStandard
	}

	return tr
}

// TriggerChain evaluates multiple triggers and returns the first to fire or highest severity.
type TriggerChain struct {
	triggers []Trigger
	mode     ChainMode
}

// ChainMode determines how the trigger chain evaluates triggers.
type ChainMode string

const (
	// ChainModeFirstMatch returns the first trigger that fires.
	ChainModeFirstMatch ChainMode = "first_match"

	// ChainModeHighestSeverity returns the trigger with highest severity.
	ChainModeHighestSeverity ChainMode = "highest_severity"

	// ChainModeAll returns all triggers that fire.
	ChainModeAll ChainMode = "all"
)

// NewTriggerChain creates a new trigger chain with the specified mode.
func NewTriggerChain(mode ChainMode) *TriggerChain {
	return &TriggerChain{
		triggers: make([]Trigger, 0),
		mode:     mode,
	}
}

// Add adds a trigger to the chain.
func (c *TriggerChain) Add(trigger Trigger) {
	c.triggers = append(c.triggers, trigger)
}

// Evaluate evaluates all triggers in the chain according to the chain mode.
func (c *TriggerChain) Evaluate(ctx context.Context, request *EscalationRequest, result *EscalationResult) []TriggerResult {
	var results []TriggerResult
	var highestSeverity TriggerResult

	for _, trigger := range c.triggers {
		tr := trigger.Evaluate(ctx, request, result)

		if c.mode == ChainModeFirstMatch && tr.ShouldEscalate {
			return []TriggerResult{tr}
		}

		if tr.ShouldEscalate {
			results = append(results, tr)

			if tr.Severity > highestSeverity.Severity {
				highestSeverity = tr
			}
		}
	}

	if c.mode == ChainModeHighestSeverity && highestSeverity.ShouldEscalate {
		return []TriggerResult{highestSeverity}
	}

	return results
}

// DefaultTriggerChain creates a trigger chain with all default triggers.
func DefaultTriggerChain() *TriggerChain {
	chain := NewTriggerChain(ChainModeHighestSeverity)

	// Add triggers in priority order
	chain.Add(NewUserTrigger())
	chain.Add(NewErrorTrigger(2))
	chain.Add(NewConfidenceTrigger(0.7, 0.4))
	chain.Add(NewTimeoutTrigger(30 * time.Second))
	chain.Add(NewContentTrigger(0.8, 50000))
	chain.Add(NewQualityTrigger(0.7))

	return chain
}
