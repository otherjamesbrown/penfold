package activities

import (
	"context"
	"encoding/json"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/handlers"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/processors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ThreadActivities provides email threading activities.
type ThreadActivities struct {
	logger     logging.Logger
	sourceRepo SourceRepository
	threadRepo ThreadRepository
}

// NewThreadActivities creates a new ThreadActivities instance.
func NewThreadActivities(
	logger logging.Logger,
	sourceRepo SourceRepository,
	threadRepo ThreadRepository,
) *ThreadActivities {
	if logger == nil {
		panic("NewThreadActivities: logger is required")
	}
	if sourceRepo == nil {
		panic("NewThreadActivities: sourceRepo is required")
	}
	if threadRepo == nil {
		panic("NewThreadActivities: threadRepo is required")
	}
	return &ThreadActivities{
		logger:     logger.With(logging.F("component", "thread_activities")),
		sourceRepo: sourceRepo,
		threadRepo: threadRepo,
	}
}

// GroupEmailThread groups an email into a thread based on headers.
// This activity is non-blocking: errors are logged but do not fail the activity.
func (a *ThreadActivities) GroupEmailThread(ctx context.Context, input GroupEmailThreadInput) (*GroupEmailThreadOutput, error) {
	logger := a.logger.WithContext(ctx).With(
		logging.F("activity", "GroupEmailThread"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	// Record heartbeat (safe to call even outside activity context)
	recordHeartbeat(ctx, "starting email threading")

	// Non-blocking wrapper: any error is logged and returns nil ThreadID
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("recovered from panic during email threading",
				logging.F("panic", r),
			)
		}
	}()

	// Fetch source metadata
	source, err := a.sourceRepo.GetSource(ctx, input.TenantID, input.SourceID)
	if err != nil {
		logger.Warn("failed to get source for threading",
			logging.F("error", err.Error()),
		)
		return &GroupEmailThreadOutput{ThreadID: nil}, nil
	}

	// Skip if not email
	if source.ContentType != "email" {
		logger.Debug("skipping threading for non-email content",
			logging.F("content_type", source.ContentType),
		)
		return &GroupEmailThreadOutput{ThreadID: nil}, nil
	}

	recordHeartbeat(ctx, "processing thread data")

	// Convert metadata map[string]string to map[string]interface{} for ThreadGrouper
	// Parse date fields and JSON-encoded arrays if present
	metadata := make(map[string]interface{})
	for k, v := range source.Metadata {
		// Parse date fields as time.Time
		if k == "date" || k == "received_at" {
			if parsedTime, err := time.Parse(time.RFC3339, v); err == nil {
				metadata[k] = parsedTime
				continue
			}
		}

		// Parse JSON arrays (e.g., references header stored as JSON string)
		if len(v) > 0 && v[0] == '[' {
			var arr []interface{}
			if err := json.Unmarshal([]byte(v), &arr); err == nil {
				metadata[k] = arr
				continue
			}
		}

		metadata[k] = v
	}

	// Create a processors.Source for ThreadGrouper
	procSource := &processors.Source{
		Metadata: metadata,
	}

	// Use ThreadGrouper to compute thread data
	grouper := handlers.NewThreadGrouper()
	threadData := extractThreadData(grouper, procSource)

	// Determine the root message ID
	rootMessageID := threadData.ThreadRoot
	if rootMessageID == "" {
		// If no thread root was determined, this message is its own root
		rootMessageID = threadData.MessageID
	}

	if rootMessageID == "" {
		logger.Warn("no message ID found for threading")
		return &GroupEmailThreadOutput{ThreadID: nil}, nil
	}

	recordHeartbeat(ctx, "checking existing thread")

	// Cross-thread dedup: when References[0] is a mid-chain message-id (e.g. Outlook
	// truncated the References header), check if it's already a member of an existing
	// thread. If so, use that thread's root instead of creating a new thread.
	if len(threadData.References) > 0 {
		for _, ref := range threadData.References {
			memberThread, memberErr := a.threadRepo.GetThreadByMemberMessageID(ctx, input.TenantID, ref)
			if memberErr != nil {
				logger.Warn("failed to check thread membership for reference",
					logging.F("reference", ref),
					logging.F("error", memberErr.Error()),
				)
				continue
			}
			if memberThread != nil && memberThread.RootMessageID != rootMessageID {
				logger.Info("cross-thread dedup: reference found in existing thread, using its root",
					logging.F("original_root", rootMessageID),
					logging.F("matched_reference", ref),
					logging.F("existing_thread_root", memberThread.RootMessageID),
					logging.F("existing_thread_id", memberThread.ID),
				)
				rootMessageID = memberThread.RootMessageID
				break
			}
		}
	}

	// Check if thread exists
	existingThread, err := a.threadRepo.GetThreadByRootMessageID(ctx, input.TenantID, rootMessageID)
	if err != nil {
		logger.Warn("failed to check existing thread",
			logging.F("root_message_id", rootMessageID),
			logging.F("error", err.Error()),
		)
		return &GroupEmailThreadOutput{ThreadID: nil}, nil
	}

	recordHeartbeat(ctx, "upserting thread")

	// Determine message date
	messageDate := threadData.MessageDate
	if messageDate.IsZero() {
		messageDate = time.Now()
	}

	// Upsert thread
	threadID, err := a.threadRepo.UpsertThread(ctx, &UpsertThreadInput{
		TenantID:          input.TenantID,
		RootMessageID:     rootMessageID,
		NormalizedSubject: threadData.NormalizedSubject,
		LatestSourceID:    input.SourceID,
		FirstMessageAt:    messageDate,
		LastMessageAt:     messageDate,
	})
	if err != nil {
		logger.Warn("failed to upsert thread",
			logging.F("root_message_id", rootMessageID),
			logging.F("error", err.Error()),
		)
		return &GroupEmailThreadOutput{ThreadID: nil}, nil
	}

	recordHeartbeat(ctx, "adding thread message")

	// Determine position in thread
	position := 1
	if existingThread != nil {
		position = existingThread.MessageCount + 1
	}

	// Determine reply-to message ID
	replyToMessageID := ""
	if threadData.IsReply {
		if threadData.InReplyTo != "" {
			replyToMessageID = threadData.InReplyTo
		} else if rootMessageID != threadData.MessageID {
			replyToMessageID = rootMessageID
		}
	}

	// Add message to thread
	err = a.threadRepo.AddThreadMessage(ctx, &AddThreadMessageInput{
		ThreadID:         threadID,
		SourceID:         input.SourceID,
		MessageID:        threadData.MessageID,
		PositionInThread: position,
		IsReply:          threadData.IsReply,
		ReplyToMessageID: replyToMessageID,
		MessageDate:      messageDate,
	})
	if err != nil {
		logger.Warn("failed to add thread message",
			logging.F("thread_id", threadID),
			logging.F("message_id", threadData.MessageID),
			logging.F("error", err.Error()),
		)
		return &GroupEmailThreadOutput{ThreadID: nil}, nil
	}

	recordHeartbeat(ctx, "updating content enrichment")

	// Set thread_id in content_enrichment
	err = a.threadRepo.SetContentEnrichmentThreadID(ctx, input.SourceID, rootMessageID)
	if err != nil {
		logger.Warn("failed to set content_enrichment thread_id",
			logging.F("source_id", input.SourceID),
			logging.F("thread_id", rootMessageID),
			logging.F("error", err.Error()),
		)
		// Don't fail the activity - threading data is already persisted
	}

	logger.Info("email threading completed",
		logging.F("root_message_id", rootMessageID),
		logging.F("thread_id", threadID),
		logging.F("position", position),
		logging.F("is_reply", threadData.IsReply),
	)

	return &GroupEmailThreadOutput{ThreadID: &rootMessageID, EmailThreadID: &threadID}, nil
}

// extractThreadData is a helper to extract thread data from a source.
// This is a simplified version that directly calls the ThreadGrouper's internal logic.
func extractThreadData(grouper *handlers.ThreadGrouper, source *processors.Source) *handlers.ThreadData {
	// Create a minimal ProcessorContext with Enrichment initialized
	pctx := &processors.ProcessorContext{
		Source: source,
		Enrichment: &enrichment.Enrichment{
			ExtractedData: make(map[string]interface{}),
		},
	}

	// Process the source (this populates ExtractedData)
	err := grouper.Process(context.Background(), pctx)
	if err != nil {
		return &handlers.ThreadData{}
	}

	// Extract the thread data from the context
	if pctx.Enrichment != nil && pctx.Enrichment.ExtractedData != nil {
		if threadDataInterface, ok := pctx.Enrichment.ExtractedData["thread"]; ok {
			if threadData, ok := threadDataInterface.(*handlers.ThreadData); ok {
				return threadData
			}
		}
	}

	return &handlers.ThreadData{}
}
