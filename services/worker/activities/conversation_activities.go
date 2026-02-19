package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ConversationActivities provides conversation auto-linking activities.
type ConversationActivities struct {
	logger     logging.Logger
	sourceRepo SourceRepository
	convRepo   ConversationRepository
	aiClient   AIClient // Optional: for summary+state generation
}

// NewConversationActivities creates a new ConversationActivities instance.
// aiClient is optional - if nil, summary generation is skipped (graceful degradation).
func NewConversationActivities(
	logger logging.Logger,
	sourceRepo SourceRepository,
	convRepo ConversationRepository,
	aiClient AIClient,
) *ConversationActivities {
	if logger == nil {
		panic("NewConversationActivities: logger is required")
	}
	if sourceRepo == nil {
		panic("NewConversationActivities: sourceRepo is required")
	}
	if convRepo == nil {
		panic("NewConversationActivities: convRepo is required")
	}
	return &ConversationActivities{
		logger:     logger.With(logging.F("component", "conversation_activities")),
		sourceRepo: sourceRepo,
		convRepo:   convRepo,
		aiClient:   aiClient,
	}
}

// Conversation represents a conversation for the activity layer.
// This is separate from the gateway type to avoid circular imports.
type Conversation struct {
	ID               string
	TenantID         string
	Topic            string
	ThreadKey        *string
	FirstSeen        *time.Time
	LastSeen         *time.Time
	ParticipantCount int32
	ItemCount        int32
	Metadata         map[string]interface{}
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// LinkConversationInput is the input for the LinkConversation activity.
type LinkConversationInput struct {
	TenantID        string `json:"tenant_id"`
	SourceID        int64  `json:"source_id"`
	ThreadID        string `json:"thread_id"`  // Root message ID from threading
	ContentID       string `json:"content_id"` // Content item ID to link
	PipelineTraceID string `json:"pipeline_trace_id,omitempty"` // Pipeline trace ID for Langfuse grouping
	PipelineSpanID  string `json:"pipeline_span_id,omitempty"` // Pipeline span ID for parent-child hierarchy
}

// LinkConversationOutput is the output from the LinkConversation activity.
type LinkConversationOutput struct {
	ConversationID string `json:"conversation_id,omitempty"` // Empty if skipped or failed
}

// LinkConversation links a content item to a conversation based on thread data.
// This activity is non-blocking: errors are logged but do not fail the activity.
func (a *ConversationActivities) LinkConversation(ctx context.Context, input LinkConversationInput) (*LinkConversationOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "LinkConversation"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("thread_id", input.ThreadID),
		logging.F("content_id", input.ContentID),
	)

	// Non-blocking wrapper: any error is logged and returns empty ConversationID
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("recovered from panic during conversation linking",
				logging.F("panic", r),
			)
		}
	}()

	// Validate inputs - skip if missing required fields
	if input.ThreadID == "" {
		logger.Debug("skipping conversation linking - no thread_id")
		return &LinkConversationOutput{ConversationID: ""}, nil
	}

	if input.ContentID == "" {
		logger.Debug("skipping conversation linking - no content_id")
		return &LinkConversationOutput{ConversationID: ""}, nil
	}

	recordHeartbeat(ctx, "fetching source metadata")

	// Fetch source metadata
	source, err := a.sourceRepo.GetSource(ctx, input.TenantID, input.SourceID)
	if err != nil {
		logger.Warn("failed to get source for conversation linking",
			logging.F("error", err.Error()),
		)
		return &LinkConversationOutput{ConversationID: ""}, nil
	}

	// Skip if not email
	if source.ContentType != "email" {
		logger.Debug("skipping conversation linking for non-email content",
			logging.F("content_type", source.ContentType),
		)
		return &LinkConversationOutput{ConversationID: ""}, nil
	}

	recordHeartbeat(ctx, "extracting email metadata")

	// Extract email metadata
	subject := source.Metadata["subject"]
	from := source.Metadata["from"]
	to := source.Metadata["to"]
	cc := source.Metadata["cc"]
	dateStr := source.Metadata["date"]

	// Parse message date
	var messageDate time.Time
	if dateStr != "" {
		if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
			messageDate = parsedDate
		}
	}
	if messageDate.IsZero() {
		messageDate = time.Now()
	}

	// Normalize subject for topic (strip Re:/FW:/Fwd: prefixes)
	topic := normalizeSubject(subject)

	recordHeartbeat(ctx, "upserting conversation")

	// Generate conversation ID
	conversationID := "conv-" + uuid.New().String()[:8]

	// Upsert conversation
	conversation := &Conversation{
		ID:               conversationID,
		TenantID:         input.TenantID,
		Topic:            topic,
		ThreadKey:        &input.ThreadID,
		FirstSeen:        &messageDate,
		LastSeen:         &messageDate,
		ParticipantCount: 0,
		ItemCount:        0,
		Metadata:         make(map[string]interface{}),
	}

	existingConvID, err := a.convRepo.UpsertConversation(ctx, conversation)
	if err != nil {
		logger.Warn("failed to upsert conversation",
			logging.F("thread_id", input.ThreadID),
			logging.F("error", err.Error()),
		)
		return &LinkConversationOutput{ConversationID: ""}, nil
	}

	recordHeartbeat(ctx, "adding conversation item")

	// Add conversation item
	err = a.convRepo.AddConversationItem(ctx, existingConvID, input.ContentID, &input.SourceID, input.TenantID)
	if err != nil {
		logger.Warn("failed to add conversation item",
			logging.F("conversation_id", existingConvID),
			logging.F("content_id", input.ContentID),
			logging.F("error", err.Error()),
		)
		// Don't fail - conversation was created
	}

	recordHeartbeat(ctx, "adding participants")

	// Extract and add participants
	participants := extractParticipants(from, to, cc)
	for _, participant := range participants {
		err := a.convRepo.AddConversationParticipant(ctx, existingConvID, nil, &participant, input.TenantID)
		if err != nil {
			logger.Warn("failed to add conversation participant",
				logging.F("conversation_id", existingConvID),
				logging.F("participant", participant),
				logging.F("error", err.Error()),
			)
			// Don't fail - continue with other participants
		}
	}

	recordHeartbeat(ctx, "updating conversation stats")

	// Update conversation stats
	err = a.convRepo.UpdateConversationStats(ctx, existingConvID)
	if err != nil {
		logger.Warn("failed to update conversation stats",
			logging.F("conversation_id", existingConvID),
			logging.F("error", err.Error()),
		)
		// Don't fail - conversation and items were created
	}

	// Generate rolling summary + state (non-blocking)
	a.generateSummaryAndState(ctx, input.TenantID, existingConvID, input.ContentID, input.PipelineTraceID, input.PipelineSpanID)

	logger.Info("conversation linking completed",
		logging.F("conversation_id", existingConvID),
		logging.F("thread_id", input.ThreadID),
		logging.F("topic", topic),
		logging.F("participants", len(participants)),
	)

	return &LinkConversationOutput{ConversationID: existingConvID}, nil
}

// normalizeSubject strips Re:/FW:/Fwd: prefixes from email subjects.
func normalizeSubject(subject string) string {
	normalized := strings.TrimSpace(subject)

	// Strip common reply/forward prefixes (case-insensitive, iterative)
	for {
		before := normalized
		normalized = strings.TrimSpace(normalized)

		// Case-insensitive prefix removal
		lower := strings.ToLower(normalized)
		if strings.HasPrefix(lower, "re:") {
			normalized = strings.TrimSpace(normalized[3:])
		} else if strings.HasPrefix(lower, "fw:") {
			normalized = strings.TrimSpace(normalized[3:])
		} else if strings.HasPrefix(lower, "fwd:") {
			normalized = strings.TrimSpace(normalized[4:])
		}

		// If no change was made, we're done
		if normalized == before {
			break
		}
	}

	return normalized
}

// extractParticipants extracts unique email addresses from from/to/cc headers.
func extractParticipants(from, to, cc string) []string {
	seen := make(map[string]bool)
	var participants []string

	// Helper to add participants from a comma-separated list
	addParticipants := func(addresses string) {
		if addresses == "" {
			return
		}
		parts := strings.Split(addresses, ",")
		for _, part := range parts {
			email := strings.TrimSpace(part)
			if email != "" && !seen[email] {
				seen[email] = true
				participants = append(participants, email)
			}
		}
	}

	// Add from address
	addParticipants(from)

	// Add to addresses
	addParticipants(to)

	// Add cc addresses
	addParticipants(cc)

	return participants
}

// generateSummaryAndState generates a rolling summary and state assessment for a conversation.
// This is non-blocking: all errors are logged but not returned.
func (a *ConversationActivities) generateSummaryAndState(ctx context.Context, tenantID, conversationID, newContentID, pipelineTraceID, pipelineSpanID string) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("conversation_id", conversationID),
		logging.F("content_id", newContentID),
	)

	// Skip if aiClient is nil (graceful degradation)
	if a.aiClient == nil {
		logger.Debug("skipping summary generation - AI client not configured")
		return
	}

	// Fetch current conversation
	conversation, err := a.convRepo.GetConversation(ctx, tenantID, conversationID)
	if err != nil {
		logger.Warn("failed to fetch conversation for summary generation",
			logging.F("error", err.Error()),
		)
		return
	}

	// Fetch last 3 conversation items (for context)
	items, err := a.convRepo.GetConversationItems(ctx, conversationID, 3)
	if err != nil {
		logger.Warn("failed to fetch conversation items for summary",
			logging.F("error", err.Error()),
		)
		return
	}

	// Build LLM prompt with: existing summary + last 3 items + new item
	prompt := a.buildSummaryPrompt(conversation, items, newContentID)

	// Call AI service for summary generation
	req := &aiv1.SummaryRequest{
		Content: prompt,
		// Use fast-tier model (per-stage model selection already available)
		// The AIClient should handle model selection internally based on task
	}
	// PipelineTraceId and PipelineSpanId are deprecated: OTel interceptors propagate context automatically.

	resp, err := a.aiClient.GenerateSummary(ctx, req)
	if err != nil {
		logger.Warn("failed to generate summary via LLM",
			logging.F("error", err.Error()),
		)
		return
	}

	// Parse response for summary text + state + reason
	summary := resp.Summary
	state, reason := a.parseStateFromSummary(resp)

	// Persist summary (version 1 for now - could be enhanced to track versions)
	err = a.convRepo.UpdateSummary(ctx, conversationID, summary, 1)
	if err != nil {
		logger.Warn("failed to persist summary",
			logging.F("error", err.Error()),
			logging.F("summary", summary),
		)
		// Continue to try persisting state even if summary fails
	}

	// Persist state + reason
	err = a.convRepo.UpdateState(ctx, conversationID, state, reason)
	if err != nil {
		logger.Warn("failed to persist conversation state",
			logging.F("error", err.Error()),
			logging.F("state", state),
			logging.F("reason", reason),
		)
		return
	}

	logger.Info("conversation summary and state updated",
		logging.F("state", state),
		logging.F("summary_length", len(summary)),
	)
}

// buildSummaryPrompt constructs a prompt for the LLM to generate summary and assess state.
func (a *ConversationActivities) buildSummaryPrompt(conversation *Conversation, items []ConversationItem, newContentID string) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a concise rolling summary (200-400 tokens) and assess the conversation state.\n\n")
	prompt.WriteString(fmt.Sprintf("Conversation Topic: %s\n", conversation.Topic))

	// Include existing summary if available
	// Note: Conversation struct doesn't have StateSummary field yet
	// This will be added by the data-dev layer later
	// For now, we'll just work with what we have

	if len(items) > 0 {
		prompt.WriteString("\nRecent conversation items:\n")
		for i, item := range items {
			prompt.WriteString(fmt.Sprintf("%d. Content ID: %s\n", i+1, item.ContentID))
		}
	}

	prompt.WriteString(fmt.Sprintf("\nNew content item: %s\n", newContentID))

	prompt.WriteString("\nProvide:\n")
	prompt.WriteString("1. Updated summary paragraph\n")
	prompt.WriteString("2. State assessment: active, stalled, resolved, or unknown\n")
	prompt.WriteString("3. Brief reason for state assessment\n")

	return prompt.String()
}

// parseStateFromSummary extracts state and reason from the LLM response.
// This is a simple implementation - could be enhanced with structured output.
func (a *ConversationActivities) parseStateFromSummary(resp *aiv1.SummaryResponse) (state, reason string) {
	// Default to "active" if we can't determine state
	state = "active"
	reason = "New message received"

	// Try to infer state from key points
	if len(resp.KeyPoints) > 0 {
		for _, point := range resp.KeyPoints {
			lowerPoint := strings.ToLower(point)
			if strings.Contains(lowerPoint, "blocked") || strings.Contains(lowerPoint, "waiting") {
				state = "stalled"
				reason = point
				return
			}
			if strings.Contains(lowerPoint, "resolved") || strings.Contains(lowerPoint, "closed") || strings.Contains(lowerPoint, "completed") {
				state = "resolved"
				reason = point
				return
			}
		}
	}

	// Check summary text for state indicators
	summaryLower := strings.ToLower(resp.Summary)
	if strings.Contains(summaryLower, "blocked") || strings.Contains(summaryLower, "stalled") || strings.Contains(summaryLower, "waiting") {
		state = "stalled"
		reason = "Conversation appears blocked or waiting"
	} else if strings.Contains(summaryLower, "resolved") || strings.Contains(summaryLower, "closed") {
		state = "resolved"
		reason = "Conversation appears resolved"
	}

	return state, reason
}

// BackfillConversationSummaries generates summaries for existing conversations.
type BackfillConversationSummariesInput struct {
	TenantID string `json:"tenant_id"`
	Limit    int    `json:"limit"`
}

// BackfillConversationSummariesOutput contains the results of backfill operation.
type BackfillConversationSummariesOutput struct {
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
}

// BackfillConversationSummaries generates initial summaries for existing conversations.
func (a *ConversationActivities) BackfillConversationSummaries(ctx context.Context, input BackfillConversationSummariesInput) (*BackfillConversationSummariesOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "BackfillConversationSummaries"),
		logging.F("tenant_id", input.TenantID),
		logging.F("limit", input.Limit),
	)

	logger.Info("starting conversation summary backfill")

	// TODO: Implementation requires a method to list conversations without summaries
	// This would need to be added to ConversationRepository interface:
	// - GetConversationsWithoutSummary(ctx, tenantID, limit) ([]string, error)
	//
	// For now, return a stub that indicates the method exists but needs data layer support

	output := &BackfillConversationSummariesOutput{
		Processed: 0,
		Failed:    0,
	}

	logger.Warn("backfill not yet implemented - requires data layer support for listing conversations without summaries")

	return output, nil
}
