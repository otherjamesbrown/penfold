// Package conversationservice repository
package conversationservice

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresRepository implements conversation data access using pgxpool.
type PostgresRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresRepository {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &PostgresRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "conversation_repository")),
	}
}

// ListConversations retrieves a paginated list of conversations for a tenant.
func (r *PostgresRepository) ListConversations(ctx context.Context, tenantID string, limit, offset int32) ([]ConversationSummary, int64, error) {
	// Enforce default limits to prevent memory exhaustion
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	// Get total count
	var totalCount int64
	countQuery := `SELECT COUNT(*) FROM conversations WHERE tenant_id = $1`
	err := r.pool.QueryRow(ctx, countQuery, tenantID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count conversations: %w", err)
	}

	// Get conversations
	query := `
		SELECT
			id,
			tenant_id,
			topic,
			thread_key,
			first_seen,
			last_seen,
			participant_count,
			item_count,
			state,
			created_at,
			updated_at
		FROM conversations
		WHERE tenant_id = $1
		ORDER BY last_seen DESC NULLS LAST, created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var conversations []ConversationSummary
	for rows.Next() {
		var conv ConversationSummary
		err := rows.Scan(
			&conv.ID,
			&conv.TenantID,
			&conv.Topic,
			&conv.ThreadKey,
			&conv.FirstSeen,
			&conv.LastSeen,
			&conv.ParticipantCount,
			&conv.ItemCount,
			&conv.State,
			&conv.CreatedAt,
			&conv.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan conversation: %w", err)
		}
		conversations = append(conversations, conv)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating conversations: %w", err)
	}

	return conversations, totalCount, nil
}

// GetConversation retrieves a single conversation with items and participants.
func (r *PostgresRepository) GetConversation(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
	// Get conversation metadata.
	// When tenantID is empty (e.g. GetConversationProcessingStatus which only
	// receives conversation_id), look up by id alone — it's the primary key.
	var query string
	var args []interface{}
	if tenantID != "" {
		query = `
		SELECT id, tenant_id, topic, thread_key, first_seen, last_seen,
			participant_count, item_count, state_summary, summary_version,
			summary_updated_at, state, state_reason, state_changed_at,
			created_at, updated_at
		FROM conversations
		WHERE tenant_id = $1 AND id = $2`
		args = []interface{}{tenantID, conversationID}
	} else {
		query = `
		SELECT id, tenant_id, topic, thread_key, first_seen, last_seen,
			participant_count, item_count, state_summary, summary_version,
			summary_updated_at, state, state_reason, state_changed_at,
			created_at, updated_at
		FROM conversations
		WHERE id = $1`
		args = []interface{}{conversationID}
	}

	var detail ConversationDetail
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&detail.ID,
		&detail.TenantID,
		&detail.Topic,
		&detail.ThreadKey,
		&detail.FirstSeen,
		&detail.LastSeen,
		&detail.ParticipantCount,
		&detail.ItemCount,
		&detail.StateSummary,
		&detail.SummaryVersion,
		&detail.SummaryUpdatedAt,
		&detail.State,
		&detail.StateReason,
		&detail.StateChangedAt,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query conversation: %w", err)
	}

	// Get conversation items
	itemsQuery := `
		SELECT
			conversation_id,
			content_id,
			source_id,
			added_at,
			tenant_id
		FROM conversation_items
		WHERE conversation_id = $1
		ORDER BY added_at ASC
	`

	itemRows, err := r.pool.Query(ctx, itemsQuery, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversation items: %w", err)
	}
	defer itemRows.Close()

	var items []ConversationItem
	for itemRows.Next() {
		var item ConversationItem
		err := itemRows.Scan(
			&item.ConversationID,
			&item.ContentID,
			&item.SourceID,
			&item.AddedAt,
			&item.TenantID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation item: %w", err)
		}
		items = append(items, item)
	}

	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversation items: %w", err)
	}

	detail.Items = items

	// Get conversation participants
	participantsQuery := `
		SELECT
			conversation_id,
			name,
			address,
			tenant_id
		FROM conversation_participants
		WHERE conversation_id = $1
		ORDER BY address NULLS LAST, name
	`

	partRows, err := r.pool.Query(ctx, participantsQuery, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversation participants: %w", err)
	}
	defer partRows.Close()

	var participants []ConversationParticipant
	for partRows.Next() {
		var part ConversationParticipant
		err := partRows.Scan(
			&part.ConversationID,
			&part.Name,
			&part.Address,
			&part.TenantID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan conversation participant: %w", err)
		}
		participants = append(participants, part)
	}

	if err := partRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating conversation participants: %w", err)
	}

	detail.Participants = participants

	return &detail, nil
}

// UpsertConversation creates or updates a conversation.
// If thread_key is provided and matches an existing conversation for this tenant,
// the existing conversation is updated. Otherwise, a new conversation is created.
// Returns the conversation ID (existing or new).
func (r *PostgresRepository) UpsertConversation(ctx context.Context, conversation *Conversation) (string, error) {
	// If thread_key is provided, try to find existing conversation
	if conversation.ThreadKey != nil && *conversation.ThreadKey != "" {
		var existingID string
		findQuery := `
			SELECT id FROM conversations
			WHERE tenant_id = $1 AND thread_key = $2
		`
		err := r.pool.QueryRow(ctx, findQuery, conversation.TenantID, *conversation.ThreadKey).Scan(&existingID)
		if err == nil {
			// Found existing conversation, update it
			updateQuery := `
				UPDATE conversations
				SET
					topic = $1,
					first_seen = COALESCE($2, first_seen),
					last_seen = COALESCE($3, last_seen),
					participant_count = $4,
					item_count = $5,
					metadata = $6,
					updated_at = NOW()
				WHERE id = $7 AND tenant_id = $8
			`

			metadataJSON, err := json.Marshal(conversation.Metadata)
			if err != nil {
				return "", fmt.Errorf("failed to marshal metadata: %w", err)
			}

			_, err = r.pool.Exec(ctx, updateQuery,
				conversation.Topic,
				conversation.FirstSeen,
				conversation.LastSeen,
				conversation.ParticipantCount,
				conversation.ItemCount,
				metadataJSON,
				existingID,
				conversation.TenantID,
			)
			if err != nil {
				return "", fmt.Errorf("failed to update conversation: %w", err)
			}

			return existingID, nil
		} else if err != pgx.ErrNoRows {
			return "", fmt.Errorf("failed to check for existing conversation: %w", err)
		}
		// If ErrNoRows, fall through to insert
	}

	// Insert new conversation
	insertQuery := `
		INSERT INTO conversations (
			id,
			tenant_id,
			topic,
			thread_key,
			first_seen,
			last_seen,
			participant_count,
			item_count,
			metadata,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		RETURNING id
	`

	metadataJSON, err := json.Marshal(conversation.Metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var insertedID string
	err = r.pool.QueryRow(ctx, insertQuery,
		conversation.ID,
		conversation.TenantID,
		conversation.Topic,
		conversation.ThreadKey,
		conversation.FirstSeen,
		conversation.LastSeen,
		conversation.ParticipantCount,
		conversation.ItemCount,
		metadataJSON,
	).Scan(&insertedID)
	if err != nil {
		return "", fmt.Errorf("failed to insert conversation: %w", err)
	}

	return insertedID, nil
}

// AddConversationItem adds a content item to a conversation.
// Idempotent: duplicate (conversation_id, content_id) pairs are ignored.
func (r *PostgresRepository) AddConversationItem(ctx context.Context, conversationID, contentID string, sourceID *int64, tenantID string) error {
	query := `
		INSERT INTO conversation_items (
			conversation_id,
			content_id,
			source_id,
			added_at,
			tenant_id
		) VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (conversation_id, content_id) DO NOTHING
	`

	_, err := r.pool.Exec(ctx, query, conversationID, contentID, sourceID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to add conversation item: %w", err)
	}

	return nil
}

// AddConversationParticipant adds a participant to a conversation.
// Idempotent: duplicate participants are ignored.
// At least one of name or address must be non-null.
func (r *PostgresRepository) AddConversationParticipant(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
	if (name == nil || *name == "") && (address == nil || *address == "") {
		return fmt.Errorf("at least one of name or address must be provided")
	}

	query := `
		INSERT INTO conversation_participants (
			conversation_id,
			name,
			address,
			tenant_id
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (conversation_id, (COALESCE(address, name))) DO NOTHING
	`

	_, err := r.pool.Exec(ctx, query, conversationID, name, address, tenantID)
	if err != nil {
		return fmt.Errorf("failed to add conversation participant: %w", err)
	}

	return nil
}

// UpdateConversationStats recalculates counts and timestamps for a conversation.
// Updates item_count, participant_count, first_seen, and last_seen based on
// current conversation_items and conversation_participants data.
func (r *PostgresRepository) UpdateConversationStats(ctx context.Context, conversationID string) error {
	query := `
		UPDATE conversations
		SET
			item_count = (
				SELECT COUNT(*)
				FROM conversation_items
				WHERE conversation_id = $1
			),
			participant_count = (
				SELECT COUNT(*)
				FROM conversation_participants
				WHERE conversation_id = $1
			),
			first_seen = (
				SELECT MIN(added_at)
				FROM conversation_items
				WHERE conversation_id = $1
			),
			last_seen = (
				SELECT MAX(added_at)
				FROM conversation_items
				WHERE conversation_id = $1
			),
			updated_at = NOW()
		WHERE id = $1
	`

	_, err := r.pool.Exec(ctx, query, conversationID)
	if err != nil {
		return fmt.Errorf("failed to update conversation stats: %w", err)
	}

	return nil
}

// UpdateSummary updates the rolling summary for a conversation.
// Sets state_summary, summary_version, and summary_updated_at.
func (r *PostgresRepository) UpdateSummary(ctx context.Context, conversationID, summary string, version int32) error {
	query := `
		UPDATE conversations
		SET
			state_summary = $1,
			summary_version = $2,
			summary_updated_at = NOW(),
			updated_at = NOW()
		WHERE id = $3
	`

	_, err := r.pool.Exec(ctx, query, summary, version, conversationID)
	if err != nil {
		return fmt.Errorf("failed to update conversation summary: %w", err)
	}

	return nil
}

// UpdateState updates the conversation state and creates a history entry.
// Inserts a record into conversation_state_history with the old and new states.
func (r *PostgresRepository) UpdateState(ctx context.Context, conversationID, state, reason string) error {
	// Start a transaction to ensure atomic update + history insert
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current state
	var oldState string
	getStateQuery := `SELECT state FROM conversations WHERE id = $1`
	err = tx.QueryRow(ctx, getStateQuery, conversationID).Scan(&oldState)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	// Update conversation state
	updateQuery := `
		UPDATE conversations
		SET
			state = $1,
			state_reason = $2,
			state_changed_at = NOW(),
			updated_at = NOW()
		WHERE id = $3
	`
	_, err = tx.Exec(ctx, updateQuery, state, reason, conversationID)
	if err != nil {
		return fmt.Errorf("failed to update conversation state: %w", err)
	}

	// Insert state history
	historyQuery := `
		INSERT INTO conversation_state_history (
			conversation_id,
			old_state,
			new_state,
			reason,
			created_at
		) VALUES ($1, $2, $3, $4, NOW())
	`
	_, err = tx.Exec(ctx, historyQuery, conversationID, oldState, state, reason)
	if err != nil {
		return fmt.Errorf("failed to insert state history: %w", err)
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ListConversationsByState retrieves conversations filtered by state.
func (r *PostgresRepository) ListConversationsByState(ctx context.Context, tenantID, state string, limit, offset int32) ([]ConversationSummary, int64, error) {
	// Enforce default limits to prevent memory exhaustion
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	// Get total count for this state
	var totalCount int64
	countQuery := `SELECT COUNT(*) FROM conversations WHERE tenant_id = $1 AND state = $2`
	err := r.pool.QueryRow(ctx, countQuery, tenantID, state).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count conversations by state: %w", err)
	}

	// Get conversations
	query := `
		SELECT
			id,
			tenant_id,
			topic,
			thread_key,
			first_seen,
			last_seen,
			participant_count,
			item_count,
			state,
			created_at,
			updated_at
		FROM conversations
		WHERE tenant_id = $1 AND state = $2
		ORDER BY last_seen DESC NULLS LAST, created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, query, tenantID, state, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query conversations by state: %w", err)
	}
	defer rows.Close()

	var conversations []ConversationSummary
	for rows.Next() {
		var conv ConversationSummary
		err := rows.Scan(
			&conv.ID,
			&conv.TenantID,
			&conv.Topic,
			&conv.ThreadKey,
			&conv.FirstSeen,
			&conv.LastSeen,
			&conv.ParticipantCount,
			&conv.ItemCount,
			&conv.State,
			&conv.CreatedAt,
			&conv.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan conversation: %w", err)
		}
		conversations = append(conversations, conv)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating conversations: %w", err)
	}

	return conversations, totalCount, nil
}

// GetStateHistory retrieves the state transition history for a conversation.
// Returns entries ordered by created_at (chronological order).
func (r *PostgresRepository) GetStateHistory(ctx context.Context, conversationID string) ([]StateHistoryEntry, error) {
	query := `
		SELECT
			id,
			conversation_id,
			old_state,
			new_state,
			reason,
			created_at
		FROM conversation_state_history
		WHERE conversation_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query state history: %w", err)
	}
	defer rows.Close()

	var history []StateHistoryEntry
	for rows.Next() {
		var entry StateHistoryEntry
		err := rows.Scan(
			&entry.ID,
			&entry.ConversationID,
			&entry.OldState,
			&entry.NewState,
			&entry.Reason,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan state history entry: %w", err)
		}
		history = append(history, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating state history: %w", err)
	}

	return history, nil
}

// MoveItems moves all items from one conversation to another, skipping duplicates.
// Returns (moved, skipped) counts. Runs in the provided transaction.
func (r *PostgresRepository) MoveItems(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string) (int32, int32, error) {
	// Get source items
	rows, err := tx.Query(ctx, `
		SELECT content_id, source_id, added_at, tenant_id
		FROM conversation_items
		WHERE conversation_id = $1
		ORDER BY added_at ASC
	`, sourceConvID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query source items: %w", err)
	}
	defer rows.Close()

	type item struct {
		ContentID string
		SourceID  *int64
		AddedAt   interface{}
		TenantID  string
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ContentID, &it.SourceID, &it.AddedAt, &it.TenantID); err != nil {
			return 0, 0, fmt.Errorf("failed to scan source item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("error iterating source items: %w", err)
	}

	var moved, skipped int32
	for _, it := range items {
		tag, err := tx.Exec(ctx, `
			INSERT INTO conversation_items (conversation_id, content_id, source_id, added_at, tenant_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (conversation_id, content_id) DO NOTHING
		`, targetConvID, it.ContentID, it.SourceID, it.AddedAt, it.TenantID)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to move item %s: %w", it.ContentID, err)
		}
		if tag.RowsAffected() > 0 {
			moved++
		} else {
			skipped++
		}
	}

	// Delete source items
	_, err = tx.Exec(ctx, `DELETE FROM conversation_items WHERE conversation_id = $1`, sourceConvID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to delete source items: %w", err)
	}

	return moved, skipped, nil
}

// MoveSpecificItems moves specified items from one conversation to another.
// Returns the count of items moved. Runs in the provided transaction.
func (r *PostgresRepository) MoveSpecificItems(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string, contentIDs []string) (int32, error) {
	var moved int32
	for _, contentID := range contentIDs {
		// Get source item
		var sourceID *int64
		var addedAt interface{}
		var tenantID string
		err := tx.QueryRow(ctx, `
			SELECT source_id, added_at, tenant_id
			FROM conversation_items
			WHERE conversation_id = $1 AND content_id = $2
		`, sourceConvID, contentID).Scan(&sourceID, &addedAt, &tenantID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return 0, fmt.Errorf("item %s not found in conversation %s", contentID, sourceConvID)
			}
			return 0, fmt.Errorf("failed to get item %s: %w", contentID, err)
		}

		// Insert into target
		_, err = tx.Exec(ctx, `
			INSERT INTO conversation_items (conversation_id, content_id, source_id, added_at, tenant_id)
			VALUES ($1, $2, $3, $4, $5)
		`, targetConvID, contentID, sourceID, addedAt, tenantID)
		if err != nil {
			return 0, fmt.Errorf("failed to insert item %s into target: %w", contentID, err)
		}

		// Delete from source
		_, err = tx.Exec(ctx, `
			DELETE FROM conversation_items
			WHERE conversation_id = $1 AND content_id = $2
		`, sourceConvID, contentID)
		if err != nil {
			return 0, fmt.Errorf("failed to remove item %s from source: %w", contentID, err)
		}
		moved++
	}
	return moved, nil
}

// RemoveItem removes a single item from a conversation.
// Returns error if the item doesn't exist in the conversation.
func (r *PostgresRepository) RemoveItem(ctx context.Context, conversationID, contentID string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM conversation_items
		WHERE conversation_id = $1 AND content_id = $2
	`, conversationID, contentID)
	if err != nil {
		return fmt.Errorf("failed to remove item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("item %s not found in conversation %s", contentID, conversationID)
	}
	return nil
}

// DeleteConversation deletes a conversation and all its items/participants (via CASCADE).
func (r *PostgresRepository) DeleteConversation(ctx context.Context, conversationID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, conversationID)
	if err != nil {
		return fmt.Errorf("failed to delete conversation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("conversation %s not found", conversationID)
	}
	return nil
}

// DeleteConversationTx deletes a conversation within a transaction (CASCADE removes items, participants, state_history).
func (r *PostgresRepository) DeleteConversationTx(ctx context.Context, tx pgx.Tx, conversationID string) error {
	tag, err := tx.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, conversationID)
	if err != nil {
		return fmt.Errorf("failed to delete conversation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("conversation %s not found", conversationID)
	}
	return nil
}

// InvalidateSummary clears the summary for a conversation so the backfill will regenerate it.
func (r *PostgresRepository) InvalidateSummary(ctx context.Context, conversationID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE conversations
		SET state_summary = NULL, summary_updated_at = NULL, updated_at = NOW()
		WHERE id = $1
	`, conversationID)
	if err != nil {
		return fmt.Errorf("failed to invalidate summary: %w", err)
	}
	return nil
}

// CreateConversation inserts a new conversation record.
func (r *PostgresRepository) CreateConversation(ctx context.Context, tx pgx.Tx, conv *Conversation) (string, error) {
	metadataJSON, err := json.Marshal(conv.Metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	var insertedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations (
			id, tenant_id, topic, thread_key, first_seen, last_seen,
			participant_count, item_count, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		RETURNING id
	`, conv.ID, conv.TenantID, conv.Topic, conv.ThreadKey, conv.FirstSeen, conv.LastSeen,
		conv.ParticipantCount, conv.ItemCount, metadataJSON).Scan(&insertedID)
	if err != nil {
		return "", fmt.Errorf("failed to create conversation: %w", err)
	}
	return insertedID, nil
}

// UpdateConversationStatsTx recalculates counts and timestamps within a transaction.
func (r *PostgresRepository) UpdateConversationStatsTx(ctx context.Context, tx pgx.Tx, conversationID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE conversations
		SET
			item_count = (SELECT COUNT(*) FROM conversation_items WHERE conversation_id = $1),
			participant_count = (SELECT COUNT(*) FROM conversation_participants WHERE conversation_id = $1),
			first_seen = (SELECT MIN(added_at) FROM conversation_items WHERE conversation_id = $1),
			last_seen = (SELECT MAX(added_at) FROM conversation_items WHERE conversation_id = $1),
			updated_at = NOW()
		WHERE id = $1
	`, conversationID)
	if err != nil {
		return fmt.Errorf("failed to update conversation stats: %w", err)
	}
	return nil
}

// MergeParticipants copies participants from source to target (skipping duplicates).
func (r *PostgresRepository) MergeParticipants(ctx context.Context, tx pgx.Tx, sourceConvID, targetConvID string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO conversation_participants (conversation_id, name, address, tenant_id)
		SELECT $2, name, address, tenant_id
		FROM conversation_participants
		WHERE conversation_id = $1
		ON CONFLICT (conversation_id, (COALESCE(address, name))) DO NOTHING
	`, sourceConvID, targetConvID)
	if err != nil {
		return fmt.Errorf("failed to merge participants: %w", err)
	}
	return nil
}

// GetItemCount returns the number of items in a conversation.
func (r *PostgresRepository) GetItemCount(ctx context.Context, conversationID string) (int32, error) {
	var count int32
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversation_items WHERE conversation_id = $1
	`, conversationID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count items: %w", err)
	}
	return count, nil
}

// BeginTx starts a new transaction.
func (r *PostgresRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// ListConversationsFiltered retrieves conversations with combined filters.
func (r *PostgresRepository) ListConversationsFiltered(ctx context.Context, tenantID string, state *string, minItems *int32, limit, offset int32) ([]ConversationSummary, int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	// Build dynamic WHERE clause
	where := "WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if state != nil && *state != "" {
		where += fmt.Sprintf(" AND state = $%d", argIdx)
		args = append(args, *state)
		argIdx++
	}
	if minItems != nil && *minItems > 0 {
		where += fmt.Sprintf(" AND item_count >= $%d", argIdx)
		args = append(args, *minItems)
		argIdx++
	}

	// Count
	var totalCount int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM conversations %s", where)
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count conversations: %w", err)
	}

	// Query
	query := fmt.Sprintf(`
		SELECT id, tenant_id, topic, thread_key, first_seen, last_seen,
			participant_count, item_count, state, created_at, updated_at
		FROM conversations
		%s
		ORDER BY last_seen DESC NULLS LAST, created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	var conversations []ConversationSummary
	for rows.Next() {
		var conv ConversationSummary
		err := rows.Scan(
			&conv.ID, &conv.TenantID, &conv.Topic, &conv.ThreadKey,
			&conv.FirstSeen, &conv.LastSeen, &conv.ParticipantCount,
			&conv.ItemCount, &conv.State, &conv.CreatedAt, &conv.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan conversation: %w", err)
		}
		conversations = append(conversations, conv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating conversations: %w", err)
	}

	return conversations, totalCount, nil
}
