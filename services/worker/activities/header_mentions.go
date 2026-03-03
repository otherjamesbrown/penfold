// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/activity"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// HeaderMentionsActivities holds dependencies for header-based mention extraction.
type HeaderMentionsActivities struct {
	logger logging.Logger
	db     *pgxpool.Pool
}

// NewHeaderMentionsActivities creates a new HeaderMentionsActivities instance.
func NewHeaderMentionsActivities(logger logging.Logger, db *pgxpool.Pool) *HeaderMentionsActivities {
	if logger == nil {
		panic("NewHeaderMentionsActivities: logger is required")
	}
	if db == nil {
		panic("NewHeaderMentionsActivities: db is required")
	}
	return &HeaderMentionsActivities{
		logger: logger.With(logging.F("component", "header_mentions_activities")),
		db:     db,
	}
}

// ExtractHeaderMentions creates content_mentions from email From/To/CC headers.
// For each header participant:
//  1. Resolves email address to entity ID via people.primary_email
//  2. Creates a mention with the appropriate participation role (FROM=1, TO=2, CC=3)
//  3. For distribution lists (account_type='distribution'), expands group members
//
// This activity is deterministic (no LLM) and runs as a fast activity.
func (a *HeaderMentionsActivities) ExtractHeaderMentions(ctx context.Context, input workflows.ExtractHeaderMentionsInput) (*workflows.ExtractHeaderMentionsOutput, error) {
	logger := a.logger.With(
		logging.F("activity", "ExtractHeaderMentions"),
		logging.F("tenant_id", input.TenantID),
		logging.F("source_id", input.SourceID),
	)

	activity.RecordHeartbeat(ctx, "starting header mention extraction")
	logger.Info("Extracting mentions from email headers",
		logging.F("sender_email", input.SenderEmail),
		logging.F("participant_count", len(input.Participants)),
	)

	output := &workflows.ExtractHeaderMentionsOutput{}

	// Build the list of participants with roles
	type headerParticipant struct {
		email       string
		displayName string
		entityID    *int64
		role        int32
		accountType string
	}
	var participants []headerParticipant

	// Add sender as FROM (role=1)
	if input.SenderEmail != "" {
		participants = append(participants, headerParticipant{
			email:       strings.ToLower(input.SenderEmail),
			displayName: input.SenderName,
			role:        1,
		})
	}

	// Add To/CC participants
	for _, p := range input.Participants {
		if p.Email == "" {
			continue
		}
		var role int32
		switch strings.ToLower(p.HeaderRole) {
		case "to":
			role = 2
		case "cc":
			role = 3
		default:
			continue
		}
		participants = append(participants, headerParticipant{
			email:       strings.ToLower(p.Email),
			displayName: p.DisplayName,
			role:        role,
		})
	}

	if len(participants) == 0 {
		logger.Info("No header participants to process")
		return output, nil
	}

	// Resolve email addresses to entity IDs
	activity.RecordHeartbeat(ctx, "resolving email addresses")
	for i := range participants {
		p := &participants[i]
		var entityID int64
		var displayName string
		var accountType string
		err := a.db.QueryRow(ctx, `
			SELECT id, canonical_name, account_type::text
			FROM people
			WHERE tenant_id = $1 AND LOWER(primary_email) = $2
			LIMIT 1
		`, input.TenantID, p.email).Scan(&entityID, &displayName, &accountType)
		if err == nil {
			p.entityID = &entityID
			p.accountType = accountType
			if p.displayName == "" {
				p.displayName = displayName
			}
		}
		// If not found, use email as mentioned_text
		if p.displayName == "" {
			p.displayName = p.email
		}
	}

	// Create mentions for each participant
	activity.RecordHeartbeat(ctx, "creating header mentions")
	for _, p := range participants {
		// Check for existing mention with same entity to avoid duplicates on reprocessing
		if p.entityID != nil {
			var exists bool
			err := a.db.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM content_mentions
					WHERE tenant_id = $1 AND content_id = $2 AND resolved_entity_id = $3
				)
			`, input.TenantID, input.SourceID, *p.entityID).Scan(&exists)
			if err != nil {
				logger.Warn("Failed to check existing mention",
					logging.Err(err), logging.F("email", p.email))
				continue
			}
			if exists {
				// Update role if existing mention has weaker role (header role wins)
				_, _ = a.db.Exec(ctx, `
					UPDATE content_mentions
					SET participation_role = $4
					WHERE tenant_id = $1 AND content_id = $2 AND resolved_entity_id = $3
					AND (participation_role = 0 OR participation_role > $4)
				`, input.TenantID, input.SourceID, *p.entityID, p.role)
				continue
			}
		}

		// Build INSERT with optional resolved fields
		var resolvedEntityID *int64
		var resolutionConfidence float64
		var resolutionSource *string
		var status string

		if p.entityID != nil {
			resolvedEntityID = p.entityID
			resolutionConfidence = 1.0
			src := string(mentions.ResolutionSourceExactMatch)
			resolutionSource = &src
			status = string(mentions.MentionStatusAutoResolved)
		} else {
			status = string(mentions.MentionStatusPending)
		}

		_, err := a.db.Exec(ctx, `
			INSERT INTO content_mentions (
				tenant_id, content_id, entity_type, mentioned_text,
				resolved_entity_id, resolution_confidence, resolution_source,
				status, resolved_at, candidates,
				participation_role, via_group_entity_id
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, NOW(), '[]'::jsonb, $9, NULL
			)
		`,
			input.TenantID,
			input.SourceID,
			string(mentions.EntityTypePerson),
			p.displayName,
			resolvedEntityID,
			resolutionConfidence,
			resolutionSource,
			status,
			p.role,
		)
		if err != nil {
			logger.Warn("Failed to create header mention",
				logging.Err(err), logging.F("email", p.email), logging.F("role", p.role))
			continue
		}

		output.MentionsCreated++
		switch p.role {
		case 1:
			output.FromMentions++
		case 2:
			output.ToMentions++
		case 3:
			output.CcMentions++
		}

		// Group expansion for distribution lists
		if p.accountType == "distribution" && p.entityID != nil {
			expanded, err := a.expandGroupMembers(ctx, input.TenantID, input.SourceID, *p.entityID)
			if err != nil {
				logger.Warn("Failed to expand group members",
					logging.Err(err),
					logging.F("dl_email", p.email),
					logging.F("dl_entity_id", *p.entityID),
				)
			} else {
				output.MentionsCreated += expanded
				output.GroupExpanded += expanded
			}
		}
	}

	logger.Info("Header mention extraction completed",
		logging.F("mentions_created", output.MentionsCreated),
		logging.F("from_mentions", output.FromMentions),
		logging.F("to_mentions", output.ToMentions),
		logging.F("cc_mentions", output.CcMentions),
		logging.F("group_expanded", output.GroupExpanded),
	)

	return output, nil
}

// expandGroupMembers creates GROUP_MEMBER (role=9) mentions for members of a distribution list.
func (a *HeaderMentionsActivities) expandGroupMembers(ctx context.Context, tenantID string, sourceID int64, groupEntityID int64) (int, error) {
	rows, err := a.db.Query(ctx, `
		SELECT m.member_entity_id, p.canonical_name
		FROM entity_group_members m
		JOIN people p ON p.id = m.member_entity_id AND p.tenant_id = $1
		WHERE m.group_entity_id = $2 AND m.tenant_id = $1 AND m.removed_at IS NULL
	`, tenantID, groupEntityID)
	if err != nil {
		return 0, fmt.Errorf("query group members: %w", err)
	}
	defer rows.Close()

	created := 0
	for rows.Next() {
		var memberID int64
		var displayName string
		if err := rows.Scan(&memberID, &displayName); err != nil {
			continue
		}

		// Skip if member already has a direct mention (direct role wins)
		var exists bool
		if err := a.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM content_mentions
				WHERE tenant_id = $1 AND content_id = $2 AND resolved_entity_id = $3
			)
		`, tenantID, sourceID, memberID).Scan(&exists); err != nil || exists {
			continue
		}

		src := string(mentions.ResolutionSourceExactMatch)
		_, err = a.db.Exec(ctx, `
			INSERT INTO content_mentions (
				tenant_id, content_id, entity_type, mentioned_text,
				resolved_entity_id, resolution_confidence, resolution_source,
				status, resolved_at, candidates,
				participation_role, via_group_entity_id
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, NOW(), '[]'::jsonb, $9, $10
			)
		`,
			tenantID,
			sourceID,
			string(mentions.EntityTypePerson),
			displayName,
			memberID,
			1.0,
			&src,
			string(mentions.MentionStatusAutoResolved),
			int32(9), // GROUP_MEMBER
			groupEntityID,
		)
		if err != nil {
			continue
		}
		created++
	}

	return created, nil
}

// Ensure HeaderMentionsActivities implements required interfaces at compile time.
var _ interface {
	ExtractHeaderMentions(ctx context.Context, input workflows.ExtractHeaderMentionsInput) (*workflows.ExtractHeaderMentionsOutput, error)
} = (*HeaderMentionsActivities)(nil)
