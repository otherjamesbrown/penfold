package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresConversationRepository implements ConversationRepository using pgxpool.
type PostgresConversationRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresConversationRepository creates a new PostgresConversationRepository.
func NewPostgresConversationRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresConversationRepository {
	return &PostgresConversationRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "conversation_repository")),
	}
}

// UpsertConversation creates or updates a conversation, returning the conversation ID.
func (r *PostgresConversationRepository) UpsertConversation(ctx context.Context, conversation *Conversation) (string, error) {
	query := `
		INSERT INTO conversations (
			id, tenant_id, thread_key, topic,
			item_count, first_seen, last_seen,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, 0, NOW(), NOW(), NOW(), NOW())
		ON CONFLICT (tenant_id, thread_key) WHERE thread_key IS NOT NULL DO UPDATE SET
			topic = COALESCE(EXCLUDED.topic, conversations.topic),
			last_seen = GREATEST(conversations.last_seen, NOW()),
			updated_at = NOW()
		RETURNING id
	`

	var id string
	err := r.pool.QueryRow(ctx, query,
		conversation.ID,
		conversation.TenantID,
		conversation.ThreadKey,
		conversation.Topic,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to upsert conversation: %w", err)
	}

	return id, nil
}

// AddConversationItem adds a content item to a conversation (idempotent).
func (r *PostgresConversationRepository) AddConversationItem(ctx context.Context, conversationID, contentID string, sourceID *int64, tenantID string) error {
	query := `
		INSERT INTO conversation_items (conversation_id, content_id, source_id, tenant_id, added_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (conversation_id, content_id) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, conversationID, contentID, sourceID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to add conversation item: %w", err)
	}
	return nil
}

// AddConversationParticipant adds a participant to a conversation with upsert semantics.
// On conflict (same address), the name is updated to the latest value. This allows
// reprocessing after a parsing fix to correct mangled names without creating duplicates.
func (r *PostgresConversationRepository) AddConversationParticipant(ctx context.Context, conversationID string, name, address *string, tenantID string) error {
	query := `
		INSERT INTO conversation_participants (conversation_id, name, address, tenant_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (conversation_id, COALESCE(address, name)) DO UPDATE SET
			name = EXCLUDED.name
	`
	_, err := r.pool.Exec(ctx, query, conversationID, name, address, tenantID)
	if err != nil {
		return fmt.Errorf("failed to add conversation participant: %w", err)
	}
	return nil
}

// UpdateConversationStats recalculates counts and timestamps for a conversation.
func (r *PostgresConversationRepository) UpdateConversationStats(ctx context.Context, conversationID string) error {
	query := `
		UPDATE conversations SET
			item_count = (SELECT COUNT(*) FROM conversation_items WHERE conversation_id = $1),
			participant_count = (SELECT COUNT(*) FROM conversation_participants WHERE conversation_id = $1),
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
func (r *PostgresConversationRepository) UpdateSummary(ctx context.Context, conversationID, summary string, version int32) error {
	query := `
		UPDATE conversations SET
			state_summary = $2,
			summary_version = $3,
			summary_updated_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, conversationID, summary, version)
	if err != nil {
		return fmt.Errorf("failed to update conversation summary: %w", err)
	}
	return nil
}

// UpdateState updates the state of a conversation and logs to history.
func (r *PostgresConversationRepository) UpdateState(ctx context.Context, conversationID, state, reason string) error {
	// Get current state for history
	var currentState string
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(state, 'unknown') FROM conversations WHERE id = $1`,
		conversationID,
	).Scan(&currentState)
	if err != nil {
		return fmt.Errorf("failed to get current conversation state: %w", err)
	}

	// Update the state
	query := `
		UPDATE conversations SET
			state = $2,
			state_reason = $3,
			state_changed_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`
	_, err = r.pool.Exec(ctx, query, conversationID, state, reason)
	if err != nil {
		return fmt.Errorf("failed to update conversation state: %w", err)
	}

	// Insert state history record
	historyQuery := `
		INSERT INTO conversation_state_history (conversation_id, old_state, new_state, reason, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	_, err = r.pool.Exec(ctx, historyQuery, conversationID, currentState, state, reason)
	if err != nil {
		r.logger.Warn("failed to insert state history (state update succeeded)",
			logging.F("conversation_id", conversationID),
			logging.Err(err),
		)
	}

	return nil
}

// GetConversationItems returns the most recent items for a conversation.
func (r *PostgresConversationRepository) GetConversationItems(ctx context.Context, conversationID string, limit int) ([]ConversationItem, error) {
	query := `
		SELECT conversation_id, content_id, source_id, added_at, tenant_id
		FROM conversation_items
		WHERE conversation_id = $1
		ORDER BY added_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation items: %w", err)
	}
	defer rows.Close()

	var items []ConversationItem
	for rows.Next() {
		var item ConversationItem
		if err := rows.Scan(&item.ConversationID, &item.ContentID, &item.SourceID, &item.AddedAt, &item.TenantID); err != nil {
			return nil, fmt.Errorf("failed to scan conversation item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetConversation returns a conversation by ID, including summary fields.
func (r *PostgresConversationRepository) GetConversation(ctx context.Context, tenantID, conversationID string) (*Conversation, error) {
	query := `
		SELECT id, tenant_id, thread_key, topic,
			state_summary, summary_version, item_count
		FROM conversations
		WHERE id = $1 AND tenant_id = $2
	`

	var conv Conversation
	err := r.pool.QueryRow(ctx, query, conversationID, tenantID).Scan(
		&conv.ID, &conv.TenantID, &conv.ThreadKey, &conv.Topic,
		&conv.StateSummary, &conv.SummaryVersion, &conv.ItemCount,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}

	return &conv, nil
}

// CleanInvalidParticipants removes conversation participants with obviously-invalid
// addresses (containing JSON artifacts like {, [, ", }, ] from the old comma-split
// parser that mangled JSON array fields). Valid email addresses never contain these.
func (r *PostgresConversationRepository) CleanInvalidParticipants(ctx context.Context, conversationID, tenantID string) (int64, error) {
	query := `
		DELETE FROM conversation_participants
		WHERE conversation_id = $1 AND tenant_id = $2
		AND address IS NOT NULL
		AND address ~ '[{}\[\]"]'
	`
	tag, err := r.pool.Exec(ctx, query, conversationID, tenantID)
	if err != nil {
		return 0, fmt.Errorf("failed to clean invalid conversation participants: %w", err)
	}
	return tag.RowsAffected(), nil
}

// GetConversationItemsWithContent returns items joined with source content text.
func (r *PostgresConversationRepository) GetConversationItemsWithContent(ctx context.Context, conversationID string, limit int) ([]ConversationItemWithContent, error) {
	query := `
		SELECT ci.content_id, ci.source_id, ci.added_at,
			COALESCE(s.raw_content, ''),
			COALESCE(s.ingestion_metadata->>'subject', ''),
			COALESCE(s.ingestion_metadata->>'from', '')
		FROM conversation_items ci
		LEFT JOIN sources s ON ci.source_id = s.id
		WHERE ci.conversation_id = $1
		ORDER BY ci.added_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation items with content: %w", err)
	}
	defer rows.Close()

	var items []ConversationItemWithContent
	for rows.Next() {
		var item ConversationItemWithContent
		if err := rows.Scan(
			&item.ContentID, &item.SourceID, &item.AddedAt,
			&item.ContentText, &item.Subject, &item.From,
		); err != nil {
			return nil, fmt.Errorf("failed to scan conversation item with content: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetConversationsWithoutSummary returns conversations that have items but no summary.
func (r *PostgresConversationRepository) GetConversationsWithoutSummary(ctx context.Context, tenantID string, limit int) ([]ConversationForSummary, error) {
	query := `
		SELECT id, tenant_id, topic, item_count,
			state_summary, summary_version, last_seen
		FROM conversations
		WHERE tenant_id = $1
			AND (state_summary IS NULL OR state_summary = '')
			AND item_count > 0
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversations without summary: %w", err)
	}
	defer rows.Close()

	var conversations []ConversationForSummary
	for rows.Next() {
		var conv ConversationForSummary
		if err := rows.Scan(
			&conv.ID, &conv.TenantID, &conv.Topic, &conv.ItemCount,
			&conv.StateSummary, &conv.SummaryVersion, &conv.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to scan conversation for summary: %w", err)
		}
		conversations = append(conversations, conv)
	}
	return conversations, rows.Err()
}

// GetStaleActiveConversations returns conversations in 'active' state with no
// activity for the given number of days.
func (r *PostgresConversationRepository) GetStaleActiveConversations(ctx context.Context, tenantID string, staleDays int, limit int) ([]ConversationForSummary, error) {
	query := `
		SELECT id, tenant_id, topic, item_count,
			state_summary, summary_version, last_seen
		FROM conversations
		WHERE tenant_id = $1
			AND state = 'active'
			AND last_seen < NOW() - ($2 || ' days')::interval
		ORDER BY last_seen ASC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, tenantID, staleDays, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale active conversations: %w", err)
	}
	defer rows.Close()

	var conversations []ConversationForSummary
	for rows.Next() {
		var conv ConversationForSummary
		if err := rows.Scan(
			&conv.ID, &conv.TenantID, &conv.Topic, &conv.ItemCount,
			&conv.StateSummary, &conv.SummaryVersion, &conv.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to scan stale conversation: %w", err)
		}
		conversations = append(conversations, conv)
	}
	return conversations, rows.Err()
}

// Ensure PostgresConversationRepository implements ConversationRepository at compile time.
var _ ConversationRepository = (*PostgresConversationRepository)(nil)
