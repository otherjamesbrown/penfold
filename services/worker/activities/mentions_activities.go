// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"

	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/pkg/mentions/resolver"
)

// MentionsActivities holds dependencies for mention extraction and resolution activities.
type MentionsActivities struct {
	logger   zerolog.Logger
	db       *pgxpool.Pool
	resolver *resolver.Resolver
	repo     *mentions.PostgresRepository
}

// NewMentionsActivities creates a new MentionsActivities instance.
func NewMentionsActivities(
	logger zerolog.Logger,
	db *pgxpool.Pool,
	res *resolver.Resolver,
	repo *mentions.PostgresRepository,
) *MentionsActivities {
	return &MentionsActivities{
		logger:   logger.With().Str("component", "mentions_activities").Logger(),
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
	ContentType string  `json:"content_type"` // email, meeting, document
	Content     string  `json:"content"`
	ProjectID   *int64  `json:"project_id,omitempty"`
	Subject     string  `json:"subject,omitempty"`
	JobID       string  `json:"job_id,omitempty"`
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
	logger := a.logger.With().
		Str("activity", "ExtractMentions").
		Str("tenant_id", input.TenantID).
		Int64("source_id", input.SourceID).
		Int64("content_id", input.ContentID).
		Str("content_type", input.ContentType).
		Int("content_length", len(input.Content)).
		Logger()

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, "starting mention extraction")

	logger.Info().Msg("Extracting and resolving mentions from content")

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
		logger.Warn().Msg("Resolver not configured, skipping mention extraction")
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
			Date:    time.Now(), // TODO: Pass actual content date
		}
	}

	// Record heartbeat before LLM processing
	activity.RecordHeartbeat(ctx, "calling LLM resolver")

	// Process the batch through the 4-stage resolver
	startTime := time.Now()
	result, err := a.resolver.ProcessBatch(ctx, tenantID, batch)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to process mentions through resolver")
		return nil, fmt.Errorf("failed to process mentions: %w", err)
	}

	// Record heartbeat after LLM processing
	activity.RecordHeartbeat(ctx, "storing resolved mentions")

	// Store resolved mentions in database
	if a.repo != nil && len(result.Resolutions) > 0 {
		stored, err := a.storeMentions(ctx, tenantID, input.ContentID, result)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to store mentions")
			// Don't fail the activity - log and continue
		} else {
			logger.Info().Int("stored", stored).Msg("Mentions stored in database")
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

	logger.Info().
		Str("trace_id", result.TraceID).
		Int("mentions_found", output.MentionsFound).
		Int("auto_resolved", output.AutoResolved).
		Int("queued_for_review", output.QueuedForReview).
		Int("new_entities", output.NewEntities).
		Int("processing_time_ms", output.ProcessingTimeMs).
		Msg("Mention extraction completed")

	return output, nil
}

// storeMentions stores the resolution results in the database.
func (a *MentionsActivities) storeMentions(ctx context.Context, tenantID string, contentID int64, result *resolver.BatchResult) (int, error) {
	if a.repo == nil {
		return 0, nil
	}

	stored := 0
	for _, res := range result.Resolutions {
		// Determine entity type
		entityType := mentions.EntityTypePerson // default
		if res.ResolvedTo != nil {
			entityType = res.ResolvedTo.EntityType
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
			a.logger.Warn().
				Err(err).
				Str("mention_text", res.MentionText).
				Msg("Failed to store mention")
			continue
		}

		// If resolved with high confidence, update the resolution
		if res.ResolvedTo != nil && res.Decision == resolver.DecisionTypeResolve {
			resolution := mentions.ResolutionInput{
				MentionID:       mention.ID,
				EntityID:        res.ResolvedTo.EntityID,
				Source:          mapResolutionSource(res),
				TranscriptError: res.IsTranscription,
				ResolvedBy:      "system",
			}

			if err := a.repo.UpdateMentionResolution(ctx, mention.ID, resolution); err != nil {
				a.logger.Warn().
					Err(err).
					Int64("mention_id", mention.ID).
					Msg("Failed to update mention resolution")
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
