package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	// Guard against the nil-pointer-in-interface trap: a typed nil concrete value
	// (e.g. *ai.Client(nil)) passed as an AIClient interface is non-nil from Go's
	// perspective, bypassing `if a.aiClient == nil` checks and causing panics when
	// methods are dispatched on the nil concrete pointer. Normalize to a proper nil.
	if aiClient != nil && reflect.ValueOf(aiClient).IsNil() {
		aiClient = nil
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
	StateSummary     *string
	SummaryVersion   int32
	Metadata         map[string]interface{}
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// LinkConversationInput is the input for the LinkConversation activity.
type LinkConversationInput struct {
	TenantID  string `json:"tenant_id"`
	SourceID  int64  `json:"source_id"`
	ThreadID  string `json:"thread_id"`  // Root message ID from threading
	ContentID string `json:"content_id"` // Content item ID to link
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

	// Extract and upsert participants (ON CONFLICT updates name, preserving participants
	// from other emails in the conversation)
	participants := extractParticipants(from, to, cc)
	for _, p := range participants {
		var namePtr *string
		if p.name != "" {
			namePtr = &p.name
		}
		err := a.convRepo.AddConversationParticipant(ctx, existingConvID, namePtr, &p.address, input.TenantID)
		if err != nil {
			logger.Warn("failed to add conversation participant",
				logging.F("conversation_id", existingConvID),
				logging.F("participant", p.address),
				logging.F("error", err.Error()),
			)
			// Don't fail - continue with other participants
		}
	}

	// Clean up garbage participants from old JSON-split parser (addresses containing {, [, " etc)
	cleaned, cleanErr := a.convRepo.CleanInvalidParticipants(ctx, existingConvID, input.TenantID)
	if cleanErr != nil {
		logger.Warn("failed to clean invalid participants",
			logging.F("conversation_id", existingConvID),
			logging.F("error", cleanErr.Error()),
		)
	} else if cleaned > 0 {
		logger.Info("cleaned invalid participant entries",
			logging.F("conversation_id", existingConvID),
			logging.F("cleaned_count", cleaned),
		)
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
	a.generateSummaryAndState(ctx, input.TenantID, existingConvID, input.ContentID)

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

// emailParticipant holds a parsed name and address for a single participant.
type emailParticipant struct {
	name    string
	address string
}

// extractParticipants extracts unique participants from from/to/cc headers.
// The to/cc fields may contain JSON arrays like:
//
//	[{"address":"alice@example.com","name":"Alice Smith"},...]
//
// The from field is typically a plain email string.
// Falls back to comma-splitting for plain strings and on JSON parse failure.
func extractParticipants(from, to, cc string) []emailParticipant {
	// seen is keyed on lowercase address for case-insensitive dedup.
	seen := make(map[string]bool)
	var participants []emailParticipant

	// parseField attempts JSON array parsing; if that fails or the input
	// doesn't start with '[', it falls back to comma-splitting.
	parseField := func(field string) {
		if field == "" {
			return
		}

		// Try JSON array path first.
		if strings.HasPrefix(strings.TrimSpace(field), "[") {
			var entries []struct {
				Address string `json:"address"`
				Name    string `json:"name"`
			}
			if err := json.Unmarshal([]byte(field), &entries); err == nil {
				for _, e := range entries {
					addr := strings.TrimSpace(e.Address)
					if addr == "" {
						continue
					}
					key := strings.ToLower(addr)
					if !seen[key] {
						seen[key] = true
						participants = append(participants, emailParticipant{
							name:    strings.TrimSpace(e.Name),
							address: addr,
						})
					}
				}
				return
			}
			// JSON parse failed — fall through to comma-split.
		}

		// Plain comma-separated fallback (no names).
		parts := strings.Split(field, ",")
		for _, part := range parts {
			addr := strings.TrimSpace(part)
			if addr == "" {
				continue
			}
			key := strings.ToLower(addr)
			if !seen[key] {
				seen[key] = true
				participants = append(participants, emailParticipant{address: addr})
			}
		}
	}

	parseField(from)
	parseField(to)
	parseField(cc)

	return participants
}

// summaryResponse is the structured JSON response from the summary+state LLM call.
type summaryResponse struct {
	Summary     string `json:"summary"`
	State       string `json:"state"`
	StateReason string `json:"state_reason"`
}

// maxContentPerItem is the max characters of source content to include per item in the prompt.
const maxContentPerItem = 600

// generateSummaryAndState generates a rolling summary and state assessment for a conversation.
// This is non-blocking: all errors are logged but not returned.
func (a *ConversationActivities) generateSummaryAndState(ctx context.Context, tenantID, conversationID, newContentID string) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("conversation_id", conversationID),
		logging.F("content_id", newContentID),
	)

	// Skip if aiClient is nil (graceful degradation)
	if a.aiClient == nil {
		logger.Debug("skipping summary generation - AI client not configured")
		return
	}

	// Fetch current conversation (includes summary fields for version tracking)
	conversation, err := a.convRepo.GetConversation(ctx, tenantID, conversationID)
	if err != nil {
		logger.Warn("failed to fetch conversation for summary generation",
			logging.F("error", err.Error()),
		)
		return
	}

	// Fetch recent items with content text for meaningful summaries
	items, err := a.convRepo.GetConversationItemsWithContent(ctx, conversationID, 5)
	if err != nil {
		logger.Warn("failed to fetch conversation items for summary",
			logging.F("error", err.Error()),
		)
		return
	}

	if len(items) == 0 {
		logger.Debug("skipping summary generation - no items in conversation")
		return
	}

	// Build prompt with actual content and request structured JSON
	prompt := buildSummaryPrompt(conversation.Topic, conversation.StateSummary, items)

	jsonMode := true
	req := &aiv1.SummaryRequest{
		Content:  prompt,
		JsonMode: &jsonMode,
	}

	// Use a detached context for the AI call so that Temporal's activity timeout
	// (30s fastOpts) does not cancel it. Summary failure is non-blocking — the
	// conversation still links successfully if this times out.
	summaryCtx, summaryCancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer summaryCancel()

	resp, err := a.aiClient.GenerateSummary(summaryCtx, req)
	if err != nil {
		logger.Warn("failed to generate summary via LLM",
			logging.F("error", err.Error()),
		)
		return
	}

	// Parse structured JSON response (with keyword fallback)
	summary, state, reason := parseSummaryResponse(resp)

	// Increment version
	newVersion := conversation.SummaryVersion + 1

	// Persist summary
	err = a.convRepo.UpdateSummary(summaryCtx, conversationID, summary, newVersion)
	if err != nil {
		logger.Warn("failed to persist summary",
			logging.F("error", err.Error()),
		)
		// Continue to try persisting state even if summary fails
	}

	// Persist state + reason
	err = a.convRepo.UpdateState(summaryCtx, conversationID, state, reason)
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
		logging.F("summary_version", newVersion),
		logging.F("summary_length", len(summary)),
	)
}

// buildSummaryPrompt constructs a prompt for the LLM to generate summary and assess state.
// Uses actual email content (not just IDs) for meaningful summaries.
func buildSummaryPrompt(topic string, existingSummary *string, items []ConversationItemWithContent) string {
	var prompt strings.Builder

	prompt.WriteString("You are analyzing an email conversation to produce a rolling summary and assess its current state.\n\n")
	prompt.WriteString(fmt.Sprintf("Conversation Topic: %s\n", topic))

	// Include existing summary for incremental updates
	if existingSummary != nil && *existingSummary != "" {
		prompt.WriteString(fmt.Sprintf("\nPrevious summary:\n%s\n", *existingSummary))
	}

	if len(items) > 0 {
		prompt.WriteString("\nMessages in this conversation (chronological):\n")
		for _, item := range items {
			prompt.WriteString("---\n")
			if item.From != "" {
				prompt.WriteString(fmt.Sprintf("From: %s\n", item.From))
			}
			if item.Subject != "" {
				prompt.WriteString(fmt.Sprintf("Subject: %s\n", item.Subject))
			}
			prompt.WriteString(fmt.Sprintf("Date: %s\n", item.AddedAt.Format("2006-01-02")))
			content := item.ContentText
			if len(content) > maxContentPerItem {
				content = content[:maxContentPerItem] + "..."
			}
			if content != "" {
				prompt.WriteString(content)
				prompt.WriteString("\n")
			}
		}
		prompt.WriteString("---\n")
	}

	prompt.WriteString("\nRespond with a JSON object (no other text):\n")
	prompt.WriteString(`{
  "summary": "A concise 200-400 token summary: what happened, who did what, decisions made, what's pending",
  "state": "active OR stalled OR resolved OR unknown",
  "state_reason": "Brief explanation for the state assessment (e.g. 'Waiting on Tim for Juniper timeline estimate')"
}`)
	prompt.WriteString("\n\nState definitions:\n")
	prompt.WriteString("- active: ongoing conversation with recent exchanges or pending follow-ups\n")
	prompt.WriteString("- stalled: waiting on someone, no progress, blocked\n")
	prompt.WriteString("- resolved: concluded, decision made, no further action expected\n")
	prompt.WriteString("- unknown: insufficient data to determine state\n")

	return prompt.String()
}

// parseSummaryResponse extracts summary, state, and reason from the LLM response.
// Tries JSON parsing first, falls back to keyword-based heuristics.
func parseSummaryResponse(resp *aiv1.SummaryResponse) (summary, state, reason string) {
	// Default values
	summary = resp.Summary
	state = "active"
	reason = "New message received"

	// Try JSON parsing of the summary field (LLM may return JSON as the summary)
	var parsed summaryResponse
	if err := json.Unmarshal([]byte(resp.Summary), &parsed); err == nil {
		if parsed.Summary != "" {
			summary = parsed.Summary
		}
		if isValidState(parsed.State) {
			state = parsed.State
		}
		if parsed.StateReason != "" {
			reason = parsed.StateReason
		}
		return summary, state, reason
	}

	// Fallback: keyword-based state detection from summary text and key points
	if len(resp.KeyPoints) > 0 {
		for _, point := range resp.KeyPoints {
			lowerPoint := strings.ToLower(point)
			if strings.Contains(lowerPoint, "blocked") || strings.Contains(lowerPoint, "waiting") || strings.Contains(lowerPoint, "stalled") {
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

	summaryLower := strings.ToLower(resp.Summary)
	if strings.Contains(summaryLower, "blocked") || strings.Contains(summaryLower, "stalled") || strings.Contains(summaryLower, "waiting") {
		state = "stalled"
		reason = "Conversation appears blocked or waiting"
	} else if strings.Contains(summaryLower, "resolved") || strings.Contains(summaryLower, "closed") {
		state = "resolved"
		reason = "Conversation appears resolved"
	}

	return summary, state, reason
}

// isValidState checks if a state string is one of the valid conversation states.
func isValidState(s string) bool {
	switch strings.ToLower(s) {
	case "active", "stalled", "resolved", "unknown":
		return true
	}
	return false
}

// BackfillConversationSummariesInput is the input for the backfill activity.
type BackfillConversationSummariesInput struct {
	TenantID string `json:"tenant_id"`
	Limit    int    `json:"limit"` // max conversations to process (0 = all)
}

// BackfillConversationSummariesOutput contains the results of backfill operation.
type BackfillConversationSummariesOutput struct {
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// BackfillConversationSummaries generates summaries and state for conversations without them.
// Processes conversations chronologically, fetching content for each and generating
// summary + state via the LLM.
func (a *ConversationActivities) BackfillConversationSummaries(ctx context.Context, input BackfillConversationSummariesInput) (*BackfillConversationSummariesOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "BackfillConversationSummaries"),
		logging.F("tenant_id", input.TenantID),
		logging.F("limit", input.Limit),
	)

	logger.Info("starting conversation summary backfill")

	if a.aiClient == nil {
		return nil, fmt.Errorf("AI client is required for summary generation")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 200 // default cap
	}

	conversations, err := a.convRepo.GetConversationsWithoutSummary(ctx, input.TenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations without summary: %w", err)
	}

	logger.Info("found conversations to backfill",
		logging.F("count", len(conversations)),
	)

	output := &BackfillConversationSummariesOutput{}

	for _, conv := range conversations {
		recordHeartbeat(ctx, fmt.Sprintf("backfill %s", conv.ID))

		items, err := a.convRepo.GetConversationItemsWithContent(ctx, conv.ID, 20)
		if err != nil {
			logger.Warn("failed to get items for conversation",
				logging.F("conversation_id", conv.ID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		if len(items) == 0 {
			output.Skipped++
			continue
		}

		prompt := buildSummaryPrompt(conv.Topic, nil, items)

		jsonMode := true
		req := &aiv1.SummaryRequest{
			Content:  prompt,
			JsonMode: &jsonMode,
		}

		resp, err := a.aiClient.GenerateSummary(ctx, req)
		if err != nil {
			logger.Warn("LLM summary generation failed",
				logging.F("conversation_id", conv.ID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		summary, state, reason := parseSummaryResponse(resp)

		if err := a.convRepo.UpdateSummary(ctx, conv.ID, summary, int32(len(items))); err != nil {
			logger.Warn("failed to persist backfill summary",
				logging.F("conversation_id", conv.ID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		if err := a.convRepo.UpdateState(ctx, conv.ID, state, reason); err != nil {
			logger.Warn("failed to persist backfill state",
				logging.F("conversation_id", conv.ID),
				logging.F("error", err.Error()),
			)
			// Summary was saved, so count as processed
		}

		output.Processed++
		logger.Info("backfilled conversation",
			logging.F("conversation_id", conv.ID),
			logging.F("state", state),
			logging.F("items", len(items)),
		)
	}

	logger.Info("backfill complete",
		logging.F("processed", output.Processed),
		logging.F("failed", output.Failed),
		logging.F("skipped", output.Skipped),
	)

	return output, nil
}

// RegenerateConversationSummaryInput is the input for regenerating a single conversation's summary.
type RegenerateConversationSummaryInput struct {
	TenantID       string `json:"tenant_id"`
	ConversationID string `json:"conversation_id"`
}

// RegenerateConversationSummaryOutput contains the regenerated summary and state.
type RegenerateConversationSummaryOutput struct {
	Summary        string `json:"summary"`
	State          string `json:"state"`
	StateReason    string `json:"state_reason"`
	SummaryVersion int32  `json:"summary_version"`
	ItemCount      int    `json:"item_count"`
}

// RegenerateConversationSummary rebuilds a conversation's summary from scratch.
// Used after merge/split operations to ensure the summary reflects the current state.
func (a *ConversationActivities) RegenerateConversationSummary(ctx context.Context, input RegenerateConversationSummaryInput) (*RegenerateConversationSummaryOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "RegenerateConversationSummary"),
		logging.F("conversation_id", input.ConversationID),
	)

	if a.aiClient == nil {
		return nil, fmt.Errorf("AI client is required for summary generation")
	}

	conv, err := a.convRepo.GetConversation(ctx, input.TenantID, input.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	if conv == nil {
		return nil, fmt.Errorf("conversation not found: %s", input.ConversationID)
	}

	items, err := a.convRepo.GetConversationItemsWithContent(ctx, input.ConversationID, 50)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation items: %w", err)
	}

	if len(items) == 0 {
		return &RegenerateConversationSummaryOutput{
			Summary:        "",
			State:          "unknown",
			StateReason:    "No items in conversation",
			SummaryVersion: 0,
			ItemCount:      0,
		}, nil
	}

	// Build prompt WITHOUT existing summary (full regeneration)
	prompt := buildSummaryPrompt(conv.Topic, nil, items)

	jsonMode := true
	resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
		Content:  prompt,
		JsonMode: &jsonMode,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM summary generation failed: %w", err)
	}

	summary, state, reason := parseSummaryResponse(resp)
	newVersion := int32(len(items))

	if err := a.convRepo.UpdateSummary(ctx, input.ConversationID, summary, newVersion); err != nil {
		return nil, fmt.Errorf("failed to persist regenerated summary: %w", err)
	}

	if err := a.convRepo.UpdateState(ctx, input.ConversationID, state, reason); err != nil {
		return nil, fmt.Errorf("failed to persist regenerated state: %w", err)
	}

	logger.Info("conversation summary regenerated",
		logging.F("state", state),
		logging.F("summary_version", newVersion),
		logging.F("items", len(items)),
	)

	return &RegenerateConversationSummaryOutput{
		Summary:        summary,
		State:          state,
		StateReason:    reason,
		SummaryVersion: newVersion,
		ItemCount:      len(items),
	}, nil
}

// CheckStaleConversationsInput is the input for the stale detection activity.
type CheckStaleConversationsInput struct {
	TenantID  string `json:"tenant_id"`
	StaleDays int    `json:"stale_days"` // days of inactivity before checking (default 14)
	Limit     int    `json:"limit"`
}

// CheckStaleConversationsOutput contains the results of stale detection.
type CheckStaleConversationsOutput struct {
	Checked      int `json:"checked"`
	Transitioned int `json:"transitioned"`
	Failed       int `json:"failed"`
}

// CheckStaleConversations evaluates active conversations with no recent activity
// and transitions them to stalled or resolved if appropriate.
func (a *ConversationActivities) CheckStaleConversations(ctx context.Context, input CheckStaleConversationsInput) (*CheckStaleConversationsOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "CheckStaleConversations"),
		logging.F("tenant_id", input.TenantID),
		logging.F("stale_days", input.StaleDays),
	)

	if a.aiClient == nil {
		return nil, fmt.Errorf("AI client is required for stale detection")
	}

	staleDays := input.StaleDays
	if staleDays <= 0 {
		staleDays = 14
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}

	conversations, err := a.convRepo.GetStaleActiveConversations(ctx, input.TenantID, staleDays, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale conversations: %w", err)
	}

	logger.Info("found stale conversations to check",
		logging.F("count", len(conversations)),
	)

	output := &CheckStaleConversationsOutput{}

	for _, conv := range conversations {
		recordHeartbeat(ctx, fmt.Sprintf("stale-check %s", conv.ID))

		items, err := a.convRepo.GetConversationItemsWithContent(ctx, conv.ID, 10)
		if err != nil {
			logger.Warn("failed to get items for stale check",
				logging.F("conversation_id", conv.ID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		prompt := buildStaleCheckPrompt(conv, items)

		jsonMode := true
		resp, err := a.aiClient.GenerateSummary(ctx, &aiv1.SummaryRequest{
			Content:  prompt,
			JsonMode: &jsonMode,
		})
		if err != nil {
			logger.Warn("LLM stale check failed",
				logging.F("conversation_id", conv.ID),
				logging.F("error", err.Error()),
			)
			output.Failed++
			continue
		}

		_, newState, reason := parseSummaryResponse(resp)
		output.Checked++

		if newState != "active" {
			if err := a.convRepo.UpdateState(ctx, conv.ID, newState, reason); err != nil {
				logger.Warn("failed to update stale conversation state",
					logging.F("conversation_id", conv.ID),
					logging.F("error", err.Error()),
				)
				output.Failed++
				continue
			}
			output.Transitioned++
			logger.Info("stale conversation transitioned",
				logging.F("conversation_id", conv.ID),
				logging.F("new_state", newState),
				logging.F("reason", reason),
			)
		}
	}

	logger.Info("stale check complete",
		logging.F("checked", output.Checked),
		logging.F("transitioned", output.Transitioned),
		logging.F("failed", output.Failed),
	)

	return output, nil
}

// buildStaleCheckPrompt constructs a prompt for evaluating whether a stale conversation
// should transition from active to stalled or resolved.
func buildStaleCheckPrompt(conv ConversationForSummary, items []ConversationItemWithContent) string {
	var prompt strings.Builder

	prompt.WriteString("You are evaluating whether an email conversation that has gone quiet should be considered stalled or resolved.\n\n")
	prompt.WriteString(fmt.Sprintf("Conversation Topic: %s\n", conv.Topic))

	if conv.StateSummary != nil && *conv.StateSummary != "" {
		prompt.WriteString(fmt.Sprintf("Current summary: %s\n", *conv.StateSummary))
	}

	if conv.LastSeen != nil {
		prompt.WriteString(fmt.Sprintf("Last activity: %s\n", conv.LastSeen.Format("2006-01-02")))
	}

	if len(items) > 0 {
		prompt.WriteString("\nRecent messages:\n")
		for _, item := range items {
			prompt.WriteString("---\n")
			if item.From != "" {
				prompt.WriteString(fmt.Sprintf("From: %s\n", item.From))
			}
			if item.Subject != "" {
				prompt.WriteString(fmt.Sprintf("Subject: %s\n", item.Subject))
			}
			prompt.WriteString(fmt.Sprintf("Date: %s\n", item.AddedAt.Format("2006-01-02")))
			content := item.ContentText
			if len(content) > maxContentPerItem {
				content = content[:maxContentPerItem] + "..."
			}
			if content != "" {
				prompt.WriteString(content)
				prompt.WriteString("\n")
			}
		}
		prompt.WriteString("---\n")
	}

	prompt.WriteString("\nRespond with a JSON object (no other text):\n")
	prompt.WriteString(`{
  "summary": "Keep the existing summary unchanged",
  "state": "stalled OR resolved OR active",
  "state_reason": "Brief explanation (e.g. 'No response in 3 weeks, waiting on vendor pricing')"
}`)
	prompt.WriteString("\n\nOnly change state from active if there is clear evidence the conversation is stalled or resolved.\n")

	return prompt.String()
}
