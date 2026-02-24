// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	perrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/mentions/resolver"
	"github.com/otherjamesbrown/penfold/pkg/tracing"
)

// MentionsActivities holds dependencies for mention extraction and resolution activities.
type MentionsActivities struct {
	logger   logging.Logger
	db       *pgxpool.Pool
	resolver *resolver.Resolver
	repo     *mentions.PostgresRepository
}

// NewMentionsActivities creates a new MentionsActivities instance.
func NewMentionsActivities(
	logger logging.Logger,
	db *pgxpool.Pool,
	res *resolver.Resolver,
	repo *mentions.PostgresRepository,
) *MentionsActivities {
	if logger == nil {
		panic("NewMentionsActivities: logger is required")
	}
	if db == nil {
		panic("NewMentionsActivities: db is required")
	}
	if res == nil {
		panic("NewMentionsActivities: res is required")
	}
	if repo == nil {
		panic("NewMentionsActivities: repo is required")
	}
	return &MentionsActivities{
		logger:   logger.With(logging.F("component", "mentions_activities")),
		db:       db,
		resolver: res,
		repo:     repo,
	}
}

// ExtractMentionsInput is the input for the ExtractMentions activity.
type ExtractMentionsInput struct {
	TenantID    string  `json:"tenant_id"`
	SourceID    int64   `json:"source_id"`
	ContentID   int64   `json:"content_id"`
	// ContentTraceID is the unique content identifier for tracing (format: <type:2>-<base62:8>)
	ContentTraceID string  `json:"content_trace_id,omitempty"`
	ContentType    string  `json:"content_type"` // email, meeting, document
	Content        string  `json:"content"`
	ProjectID      *int64  `json:"project_id,omitempty"`
	Subject        string  `json:"subject,omitempty"`
	JobID          string  `json:"job_id,omitempty"`
}

// ExtractMentionsOutput is the output from the ExtractMentions activity.
type ExtractMentionsOutput struct {
	TraceID          string `json:"trace_id"`
	MentionsFound    int    `json:"mentions_found"`
	AutoResolved     int    `json:"auto_resolved"`
	QueuedForReview  int    `json:"queued_for_review"`
	NewEntities      int    `json:"new_entities_suggested"`
	ProcessingTimeMs int    `json:"processing_time_ms"`
}

// ExtractMentions extracts and resolves mentions from content using the LLM-driven resolver.
// This activity:
//  1. Calls the 4-stage resolver (understanding, cross-mention, matching, verification)
//  2. Stores resolved mentions in the content_mentions table
//  3. Creates patterns for high-confidence resolutions
//  4. Queues uncertain resolutions for human review
func (a *MentionsActivities) ExtractMentions(ctx context.Context, input ExtractMentionsInput) (*ExtractMentionsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ExtractMentions"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
		logging.F("content_id", input.ContentID),
		logging.F("content_type", input.ContentType),
		logging.F("content_length", len(input.Content)),
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting mention extraction")

	logger.Info("Extracting and resolving mentions from content")

	// Check for cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate input
	if input.Content == "" {
		return nil, temporal.NewApplicationError(
			"content is empty",
			"ValidationError",
		)
	}

	// Check if resolver is available
	if a.resolver == nil {
		logger.Warn("Resolver not configured, skipping mention extraction")
		return &ExtractMentionsOutput{}, nil
	}

	// Resolve tenant ID
	tenantID := resolveTenantID(input.TenantID)

	// Build resolution batch
	batch := resolver.ResolutionBatch{
		ContentID:   input.ContentID,
		ContentType: input.ContentType,
		ContentText: input.Content,
		ProjectID:   input.ProjectID,
	}

	// Add metadata if available
	if input.Subject != "" {
		batch.Metadata = &resolver.ContentMetadata{
			Subject: input.Subject,
			Date:    time.Now(), // TODO(pe-xxxx): Add ContentDate to ExtractMentionsInput struct
		}
	}

	// Record heartbeat before LLM processing
	activity.RecordHeartbeat(ctx, "calling LLM resolver")

	// Wire heartbeat into the resolver so it signals liveness between each
	// of the 4 LLM stages. Without this, the 90s heartbeat timeout can expire
	// during multi-stage resolution (4 stages * 30s+ each = >90s).
	a.resolver.SetHeartbeat(func(stage string) {
		activity.RecordHeartbeat(ctx, stage)
	})
	defer a.resolver.SetHeartbeat(nil) // clear after use

	// Get content ID for tracing with fallback to source ID
	contentID := input.ContentTraceID
	if contentID == "" {
		contentID = fmt.Sprintf("%d", input.SourceID)
	}

	// Start LLM call trace for mention resolution
	ctx, llmSpan := tracing.StartLLMCall(ctx, "mention_resolution", tracing.LLMCallOptions{
		TenantID:  input.TenantID,
		ContentID: contentID,
		TaskType:  "extract-mentions",
	})
	defer llmSpan.End()

	// Process the batch through the 4-stage resolver
	startTime := time.Now()
	result, err := a.resolver.ProcessBatch(ctx, tenantID, batch)
	if err != nil {
		pe := perrors.ClassifyError(err, "extract_mentions")
		tracing.SetLLMResult(llmSpan, tracing.LLMResult{
			LatencyMs: time.Since(startTime).Milliseconds(),
			Error:     pe,
		})
		logger.Error("Failed to process mentions through resolver", logging.Err(pe))
		return nil, WrapForTemporal(pe)
	}

	// Record success metrics
	tracing.SetLLMResult(llmSpan, tracing.LLMResult{
		LatencyMs: time.Since(startTime).Milliseconds(),
	})
	tracing.SetAttributes(llmSpan,
		tracing.AttrInt("mentions.found", len(result.Resolutions)),
		tracing.AttrInt("mentions.auto_resolved", result.AutoResolved),
		tracing.AttrInt("mentions.queued_for_review", result.QueuedForReview),
	)

	// Record heartbeat after LLM processing
	activity.RecordHeartbeat(ctx, "storing resolved mentions")

	// Store resolved mentions in database
	if a.repo != nil && len(result.Resolutions) > 0 {
		stored, err := a.storeMentions(ctx, tenantID, input.ContentID, result)
		if err != nil {
			logger.Error("Failed to store mentions", logging.Err(err))
			// Don't fail the activity - log and continue
		} else {
			logger.Info("Mentions stored in database", logging.F("stored", stored))
		}
	}

	// Build output
	output := &ExtractMentionsOutput{
		TraceID:          result.TraceID,
		MentionsFound:    len(result.Resolutions),
		AutoResolved:     result.AutoResolved,
		QueuedForReview:  result.QueuedForReview,
		NewEntities:      len(result.NewEntities),
		ProcessingTimeMs: int(time.Since(startTime).Milliseconds()),
	}

	logger.Info("Mention extraction completed",
		logging.F("trace_id", result.TraceID),
		logging.F("mentions_found", output.MentionsFound),
		logging.F("auto_resolved", output.AutoResolved),
		logging.F("queued_for_review", output.QueuedForReview),
		logging.F("new_entities", output.NewEntities),
		logging.F("processing_time_ms", output.ProcessingTimeMs),
	)

	return output, nil
}

// storeMentions stores the resolution results in the database.
func (a *MentionsActivities) storeMentions(ctx context.Context, tenantID string, contentID int64, result *resolver.BatchResult) (int, error) {
	if a.repo == nil {
		return 0, nil
	}

	stored := 0
	for _, res := range result.Resolutions {
		// Use entity type from resolution (populated from Stage 1 understanding).
		// This preserves the LLM's entity classification even when unresolved.
		entityType := res.EntityType
		if entityType == "" {
			entityType = mentions.EntityTypePerson // fallback
		}

		// Create mention input for repository
		input := mentions.MentionInput{
			ContentID:      contentID,
			EntityType:     entityType,
			MentionedText:  res.MentionText,
			Position:       &res.MentionPosition,
		}

		// Store the mention
		mention, err := a.repo.CreateMention(ctx, input)
		if err != nil {
			a.logger.Warn("Failed to store mention",
				logging.Err(err),
				logging.F("mention_text", res.MentionText),
			)
			continue
		}

		// If resolved with high confidence, update the resolution
		if res.ResolvedTo != nil && res.Decision == resolver.DecisionTypeResolve {
			resolution := mentions.ResolutionInput{
				MentionID:       mention.ID,
				EntityID:        res.ResolvedTo.EntityID.Int64(),
				Source:          mapResolutionSource(res),
				TranscriptError: res.IsTranscription,
				ResolvedBy:      "system",
			}

			if err := a.repo.UpdateMentionResolution(ctx, mention.ID, resolution); err != nil {
				a.logger.Warn("Failed to update mention resolution",
					logging.Err(err),
					logging.F("mention_id", mention.ID),
				)
			}
		}

		stored++
	}

	return stored, nil
}

// mapResolutionSource maps resolver factors to a resolution source.
func mapResolutionSource(res resolver.Resolution) mentions.ResolutionSource {
	if res.Factors == nil {
		return mentions.ResolutionSourceFuzzy
	}

	// Check factors to determine source
	if _, ok := res.Factors["exact_match"]; ok {
		return mentions.ResolutionSourceExactMatch
	}
	if _, ok := res.Factors["alias_match"]; ok {
		return mentions.ResolutionSourceAlias
	}
	if _, ok := res.Factors["project_membership"]; ok {
		return mentions.ResolutionSourceProjectContext
	}
	if priorLinks, ok := res.Factors["prior_links"].(float64); ok && priorLinks > 0 {
		return mentions.ResolutionSourcePriorLink
	}

	return mentions.ResolutionSourceFuzzy
}

// Ensure MentionsActivities implements required interfaces at compile time.
var _ interface {
	ExtractMentions(ctx context.Context, input ExtractMentionsInput) (*ExtractMentionsOutput, error)
} = (*MentionsActivities)(nil)
