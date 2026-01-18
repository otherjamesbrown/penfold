// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/ai/v1"
)

// AIModelSelector defines the interface for model selection.
type AIModelSelector interface {
	// SelectModel selects the best model for a given task.
	SelectModel(ctx context.Context, task string, constraints ModelConstraints) (*ModelSelection, error)

	// GetModelInfo returns information about a specific model.
	GetModelInfo(ctx context.Context, modelID string) (*ModelInfo, error)
}

// ModelConstraints defines constraints for model selection.
type ModelConstraints struct {
	MaxLatencyMs     int64    `json:"max_latency_ms,omitempty"`
	MaxCostPerToken  float64  `json:"max_cost_per_token,omitempty"`
	RequiredFeatures []string `json:"required_features,omitempty"`
	PreferLocal      bool     `json:"prefer_local,omitempty"`
	MinQuality       float64  `json:"min_quality,omitempty"`
}

// ModelSelection contains the selected model and reasoning.
type ModelSelection struct {
	ModelID       string  `json:"model_id"`
	ModelName     string  `json:"model_name"`
	Provider      string  `json:"provider"` // ollama, gemini, openai
	IsLocal       bool    `json:"is_local"`
	EstimatedCost float64 `json:"estimated_cost,omitempty"`
	Reasoning     string  `json:"reasoning"`
}

// ModelInfo contains information about an AI model.
type ModelInfo struct {
	ModelID       string            `json:"model_id"`
	ModelName     string            `json:"model_name"`
	Provider      string            `json:"provider"`
	Capabilities  []string          `json:"capabilities"`
	ContextWindow int32             `json:"context_window"`
	CostPerToken  float64           `json:"cost_per_token,omitempty"`
	IsLocal       bool              `json:"is_local"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// EscalationHandler defines the interface for handling escalations.
type EscalationHandler interface {
	// Escalate sends a request to a human or higher-tier system.
	Escalate(ctx context.Context, req *EscalationRequest) (*EscalationResponse, error)

	// GetEscalationStatus checks the status of an escalation.
	GetEscalationStatus(ctx context.Context, escalationID string) (*EscalationStatus, error)
}

// EscalationRequest represents a request for escalation.
type EscalationRequest struct {
	TenantID    string            `json:"tenant_id"`
	SourceID    int64             `json:"source_id"`
	Reason      string            `json:"reason"`
	Priority    string            `json:"priority"` // low, medium, high, urgent
	Category    string            `json:"category"` // quality, ambiguity, sensitivity, error
	Details     string            `json:"details"`
	Context     map[string]string `json:"context,omitempty"`
	RequestedBy string            `json:"requested_by,omitempty"`
}

// EscalationResponse contains the result of an escalation request.
type EscalationResponse struct {
	EscalationID string `json:"escalation_id"`
	Status       string `json:"status"` // pending, assigned, resolved
	AssignedTo   string `json:"assigned_to,omitempty"`
}

// EscalationStatus contains the current status of an escalation.
type EscalationStatus struct {
	EscalationID string    `json:"escalation_id"`
	Status       string    `json:"status"`
	Resolution   string    `json:"resolution,omitempty"`
	ResolvedBy   string    `json:"resolved_by,omitempty"`
	ResolvedAt   time.Time `json:"resolved_at,omitempty"`
}

// AIActivities holds dependencies for AI coordination activities.
type AIActivities struct {
	logger            zerolog.Logger
	aiClient          AIClient
	modelSelector     AIModelSelector
	escalationHandler EscalationHandler
}

// NewAIActivities creates a new AIActivities instance.
func NewAIActivities(
	logger zerolog.Logger,
	aiClient AIClient,
	modelSelector AIModelSelector,
	escalationHandler EscalationHandler,
) *AIActivities {
	return &AIActivities{
		logger:            logger.With().Str("component", "ai_activities").Logger(),
		aiClient:          aiClient,
		modelSelector:     modelSelector,
		escalationHandler: escalationHandler,
	}
}

// ProcessWithAIInput is the input for the ProcessWithAI activity.
type ProcessWithAIInput struct {
	TenantID     string            `json:"tenant_id"`
	SourceID     int64             `json:"source_id"`
	JobID        string            `json:"job_id"`
	Task         string            `json:"task"`         // summarize, extract, categorize, embed, chat
	Content      string            `json:"content"`
	Model        string            `json:"model,omitempty"`
	Parameters   map[string]string `json:"parameters,omitempty"`
	Constraints  *ModelConstraints `json:"constraints,omitempty"`
}

// ProcessWithAIOutput is the output from the ProcessWithAI activity.
type ProcessWithAIOutput struct {
	Result       string            `json:"result"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ModelUsed    string            `json:"model_used"`
	InputTokens  int32             `json:"input_tokens,omitempty"`
	OutputTokens int32             `json:"output_tokens,omitempty"`
	Duration     time.Duration     `json:"duration"`
	Confidence   float32           `json:"confidence,omitempty"`
}

// ProcessWithAIActivity processes content using an AI model.
// This activity handles various AI tasks and manages model selection.
func (a *AIActivities) ProcessWithAIActivity(ctx context.Context, input ProcessWithAIInput) (*ProcessWithAIOutput, error) {
	logger := a.logger.With().
		Str("activity", "ProcessWithAIActivity").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("job_id", input.JobID).
		Str("task", input.Task).
		Int("content_length", len(input.Content)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting AI processing")

	logger.Info().Msg("Processing content with AI")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Task == "" {
		return nil, NewValidationError("task is required")
	}
	if input.Content == "" {
		return nil, NewValidationError("content is required")
	}

	// Check if AI client is available
	if a.aiClient == nil {
		return nil, NewConfigurationError("AI client not configured")
	}

	startTime := time.Now()

	// Select model if not specified
	modelID := input.Model
	if modelID == "" && a.modelSelector != nil {
		activity.RecordHeartbeat(ctx, "selecting model")
		constraints := input.Constraints
		if constraints == nil {
			constraints = &ModelConstraints{PreferLocal: true}
		}
		selection, err := a.modelSelector.SelectModel(ctx, input.Task, *constraints)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to select model, using default")
		} else {
			modelID = selection.ModelID
			logger.Debug().Str("selected_model", modelID).Str("reasoning", selection.Reasoning).Msg("Model selected")
		}
	}

	activity.RecordHeartbeat(ctx, fmt.Sprintf("processing with task: %s", input.Task))

	var output *ProcessWithAIOutput
	var err error

	// Route to appropriate handler based on task
	switch strings.ToLower(input.Task) {
	case "summarize", "summary":
		output, err = a.handleSummarizeTask(ctx, input, modelID)
	case "extract", "assertions":
		output, err = a.handleExtractTask(ctx, input, modelID)
	case "embed", "embedding":
		output, err = a.handleEmbedTask(ctx, input, modelID)
	case "categorize", "classify":
		output, err = a.handleCategorizeTask(ctx, input, modelID)
	default:
		// Default to summary for unknown tasks
		output, err = a.handleSummarizeTask(ctx, input, modelID)
	}

	if err != nil {
		logger.Error().Err(err).Str("task", input.Task).Msg("AI processing failed")
		return nil, err
	}

	output.Duration = time.Since(startTime)

	logger.Info().
		Dur("duration", output.Duration).
		Str("model_used", output.ModelUsed).
		Int32("input_tokens", output.InputTokens).
		Int32("output_tokens", output.OutputTokens).
		Msg("AI processing completed")

	return output, nil
}

// handleSummarizeTask handles summarization tasks.
func (a *AIActivities) handleSummarizeTask(ctx context.Context, input ProcessWithAIInput, modelID string) (*ProcessWithAIOutput, error) {
	maxLength := int32(150)
	if lenStr, ok := input.Parameters["max_length"]; ok {
		var parsed int32
		fmt.Sscanf(lenStr, "%d", &parsed)
		if parsed > 0 {
			maxLength = parsed
		}
	}

	req := &aiv1.SummaryRequest{
		Content:   input.Content,
		MaxLength: &maxLength,
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		TenantId:  &input.TenantID,
	}
	if modelID != "" {
		req.Model = &modelID
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("summarization failed: %w", err)
	}

	return &ProcessWithAIOutput{
		Result:       resp.Summary,
		ModelUsed:    resp.ModelUsed,
		InputTokens:  resp.GetInputTokens(),
		OutputTokens: resp.GetOutputTokens(),
		Metadata: map[string]string{
			"key_points_count": fmt.Sprintf("%d", len(resp.KeyPoints)),
		},
	}, nil
}

// handleExtractTask handles extraction tasks.
func (a *AIActivities) handleExtractTask(ctx context.Context, input ProcessWithAIInput, modelID string) (*ProcessWithAIOutput, error) {
	minConfidence := float32(0.5)
	maxAssertions := int32(20)

	req := &aiv1.AssertionRequest{
		Content:       input.Content,
		MinConfidence: &minConfidence,
		MaxAssertions: &maxAssertions,
		TenantId:      &input.TenantID,
	}

	resp, err := a.aiClient.ExtractAssertions(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// Format assertions as result
	var resultBuilder strings.Builder
	for i, assertion := range resp.Assertions {
		if i > 0 {
			resultBuilder.WriteString("\n")
		}
		resultBuilder.WriteString(fmt.Sprintf("%s %s %s (%.2f)",
			assertion.Subject, assertion.Predicate, assertion.Object, assertion.Confidence))
	}

	return &ProcessWithAIOutput{
		Result:    resultBuilder.String(),
		ModelUsed: resp.ModelUsed,
		Metadata: map[string]string{
			"assertions_count": fmt.Sprintf("%d", len(resp.Assertions)),
			"total_found":      fmt.Sprintf("%d", resp.TotalFound),
		},
	}, nil
}

// handleEmbedTask handles embedding tasks.
func (a *AIActivities) handleEmbedTask(ctx context.Context, input ProcessWithAIInput, modelID string) (*ProcessWithAIOutput, error) {
	req := &aiv1.EmbeddingRequest{
		Text:     input.Content,
		TenantId: &input.TenantID,
	}
	if modelID != "" {
		req.Model = &modelID
	}

	resp, err := a.aiClient.GenerateEmbedding(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	return &ProcessWithAIOutput{
		Result:    fmt.Sprintf("Generated %d-dimensional embedding", resp.Dimensions),
		ModelUsed: resp.ModelUsed,
		Metadata: map[string]string{
			"dimensions": fmt.Sprintf("%d", resp.Dimensions),
		},
	}, nil
}

// handleCategorizeTask handles categorization tasks.
func (a *AIActivities) handleCategorizeTask(ctx context.Context, input ProcessWithAIInput, modelID string) (*ProcessWithAIOutput, error) {
	// Use summary with categorization prompt
	categorizePrompt := "Categorize the following content into one of these categories: work, personal, finance, health, travel, technology, news, social. " +
		"Respond with only the category name.\n\nContent: " + input.Content

	maxLength := int32(20)
	req := &aiv1.SummaryRequest{
		Content:   categorizePrompt,
		MaxLength: &maxLength,
		Style:     aiv1.SummaryStyle_SUMMARY_STYLE_BRIEF,
		TenantId:  &input.TenantID,
	}
	if modelID != "" {
		req.Model = &modelID
	}

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("categorization failed: %w", err)
	}

	return &ProcessWithAIOutput{
		Result:       strings.TrimSpace(resp.Summary),
		ModelUsed:    resp.ModelUsed,
		InputTokens:  resp.GetInputTokens(),
		OutputTokens: resp.GetOutputTokens(),
	}, nil
}

// SelectModelInput is the input for the SelectModel activity.
type SelectModelInput struct {
	TenantID    string           `json:"tenant_id"`
	Task        string           `json:"task"`
	Constraints ModelConstraints `json:"constraints"`
}

// SelectModelOutput is the output from the SelectModel activity.
type SelectModelOutput struct {
	ModelID       string  `json:"model_id"`
	ModelName     string  `json:"model_name"`
	Provider      string  `json:"provider"`
	IsLocal       bool    `json:"is_local"`
	EstimatedCost float64 `json:"estimated_cost,omitempty"`
	Reasoning     string  `json:"reasoning"`
}

// SelectModelActivity selects the best AI model for a given task.
// This activity considers constraints like latency, cost, and quality.
func (a *AIActivities) SelectModelActivity(ctx context.Context, input SelectModelInput) (*SelectModelOutput, error) {
	logger := a.logger.With().
		Str("activity", "SelectModelActivity").
		Str("tenant_id", input.TenantID).
		Str("task", input.Task).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting model selection")

	logger.Info().Msg("Selecting AI model")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.Task == "" {
		return nil, NewValidationError("task is required")
	}

	// Check if model selector is available
	if a.modelSelector == nil {
		// Return default model selection
		logger.Debug().Msg("Model selector not configured, using defaults")
		return a.getDefaultModel(input.Task, input.Constraints)
	}

	startTime := time.Now()

	selection, err := a.modelSelector.SelectModel(ctx, input.Task, input.Constraints)
	if err != nil {
		logger.Warn().Err(err).Msg("Model selection failed, using defaults")
		return a.getDefaultModel(input.Task, input.Constraints)
	}

	output := &SelectModelOutput{
		ModelID:       selection.ModelID,
		ModelName:     selection.ModelName,
		Provider:      selection.Provider,
		IsLocal:       selection.IsLocal,
		EstimatedCost: selection.EstimatedCost,
		Reasoning:     selection.Reasoning,
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Str("model_id", output.ModelID).
		Str("provider", output.Provider).
		Bool("is_local", output.IsLocal).
		Msg("Model selected")

	return output, nil
}

// getDefaultModel returns default model selection for a task.
func (a *AIActivities) getDefaultModel(task string, constraints ModelConstraints) (*SelectModelOutput, error) {
	// Default models based on task type
	switch strings.ToLower(task) {
	case "embed", "embedding":
		return &SelectModelOutput{
			ModelID:   "nomic-embed-text",
			ModelName: "Nomic Embed Text",
			Provider:  "ollama",
			IsLocal:   true,
			Reasoning: "Default local embedding model",
		}, nil
	case "summarize", "summary", "extract", "assertions":
		if constraints.PreferLocal {
			return &SelectModelOutput{
				ModelID:   "llama3.2",
				ModelName: "Llama 3.2",
				Provider:  "ollama",
				IsLocal:   true,
				Reasoning: "Default local LLM for text tasks",
			}, nil
		}
		return &SelectModelOutput{
			ModelID:   "gemini-1.5-flash",
			ModelName: "Gemini 1.5 Flash",
			Provider:  "gemini",
			IsLocal:   false,
			Reasoning: "Default cloud LLM for text tasks",
		}, nil
	default:
		if constraints.PreferLocal {
			return &SelectModelOutput{
				ModelID:   "llama3.2",
				ModelName: "Llama 3.2",
				Provider:  "ollama",
				IsLocal:   true,
				Reasoning: "Default local model",
			}, nil
		}
		return &SelectModelOutput{
			ModelID:   "gemini-1.5-flash",
			ModelName: "Gemini 1.5 Flash",
			Provider:  "gemini",
			IsLocal:   false,
			Reasoning: "Default cloud model",
		}, nil
	}
}

// EscalateRequestInput is the input for the EscalateRequest activity.
type EscalateRequestInput struct {
	TenantID    string            `json:"tenant_id"`
	SourceID    int64             `json:"source_id"`
	JobID       string            `json:"job_id"`
	Reason      string            `json:"reason"`
	Priority    string            `json:"priority"` // low, medium, high, urgent
	Category    string            `json:"category"` // quality, ambiguity, sensitivity, error
	Details     string            `json:"details"`
	Context     map[string]string `json:"context,omitempty"`
}

// EscalateRequestOutput is the output from the EscalateRequest activity.
type EscalateRequestOutput struct {
	EscalationID string `json:"escalation_id"`
	Status       string `json:"status"`
	AssignedTo   string `json:"assigned_to,omitempty"`
}

// EscalateRequestActivity escalates a request to a human or higher-tier system.
// This activity is used when AI processing requires human intervention.
func (a *AIActivities) EscalateRequestActivity(ctx context.Context, input EscalateRequestInput) (*EscalateRequestOutput, error) {
	logger := a.logger.With().
		Str("activity", "EscalateRequestActivity").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Str("job_id", input.JobID).
		Str("reason", input.Reason).
		Str("priority", input.Priority).
		Str("category", input.Category).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting escalation")

	logger.Info().Msg("Escalating request")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.TenantID == "" {
		return nil, NewValidationError("tenant_id is required")
	}
	if input.Reason == "" {
		return nil, NewValidationError("reason is required")
	}

	// Validate priority
	validPriorities := map[string]bool{
		"low": true, "medium": true, "high": true, "urgent": true,
	}
	if input.Priority != "" && !validPriorities[strings.ToLower(input.Priority)] {
		return nil, NewValidationError(fmt.Sprintf("invalid priority: %s", input.Priority))
	}

	// Validate category
	validCategories := map[string]bool{
		"quality": true, "ambiguity": true, "sensitivity": true, "error": true, "other": true,
	}
	if input.Category != "" && !validCategories[strings.ToLower(input.Category)] {
		input.Category = "other"
	}

	startTime := time.Now()

	// Check if escalation handler is available
	if a.escalationHandler == nil {
		// Log the escalation for manual handling
		logger.Warn().
			Str("reason", input.Reason).
			Str("details", input.Details).
			Msg("Escalation handler not configured, logging escalation")

		return &EscalateRequestOutput{
			EscalationID: fmt.Sprintf("esc-%d-%d", input.SourceID, time.Now().UnixNano()),
			Status:       "logged",
		}, nil
	}

	req := &EscalationRequest{
		TenantID: input.TenantID,
		SourceID: input.SourceID,
		Reason:   input.Reason,
		Priority: input.Priority,
		Category: input.Category,
		Details:  input.Details,
		Context:  input.Context,
	}

	resp, err := a.escalationHandler.Escalate(ctx, req)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to escalate request")
		return nil, fmt.Errorf("failed to escalate request: %w", err)
	}

	output := &EscalateRequestOutput{
		EscalationID: resp.EscalationID,
		Status:       resp.Status,
		AssignedTo:   resp.AssignedTo,
	}

	logger.Info().
		Dur("duration", time.Since(startTime)).
		Str("escalation_id", output.EscalationID).
		Str("status", output.Status).
		Msg("Request escalated")

	return output, nil
}

// Ensure AIActivities implements required interfaces at compile time.
var _ interface {
	ProcessWithAIActivity(ctx context.Context, input ProcessWithAIInput) (*ProcessWithAIOutput, error)
	SelectModelActivity(ctx context.Context, input SelectModelInput) (*SelectModelOutput, error)
	EscalateRequestActivity(ctx context.Context, input EscalateRequestInput) (*EscalateRequestOutput, error)
} = (*AIActivities)(nil)
