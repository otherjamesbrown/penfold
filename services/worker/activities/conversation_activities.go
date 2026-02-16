package activities

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ConversationActivities provides conversation auto-linking activities.
type ConversationActivities struct {
	logger     logging.Logger
	sourceRepo SourceRepository
	convRepo   ConversationRepository
}

// NewConversationActivities creates a new ConversationActivities instance.
func NewConversationActivities(
	logger logging.Logger,
	sourceRepo SourceRepository,
	convRepo ConversationRepository,
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
