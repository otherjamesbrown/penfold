package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresThreadRepository implements ThreadRepository using pgxpool.
type PostgresThreadRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresThreadRepository creates a new PostgresThreadRepository.
func NewPostgresThreadRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresThreadRepository {
	return &PostgresThreadRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "thread_repository")),
	}
}

// UpsertThread creates or updates an email thread, returning the thread ID.
func (r *PostgresThreadRepository) UpsertThread(ctx context.Context, input *UpsertThreadInput) (int64, error) {
	query := `
		INSERT INTO email_threads (
			tenant_id,
			root_message_id,
			subject,
			message_count,
			first_message_at,
			last_message_at,
			latest_source_id,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, 1, $4, $5, $6, NOW(), NOW())
		ON CONFLICT (tenant_id, root_message_id) DO UPDATE SET
			message_count = email_threads.message_count + 1,
			last_message_at = GREATEST(email_threads.last_message_at, EXCLUDED.last_message_at),
			latest_source_id = EXCLUDED.latest_source_id,
			updated_at = NOW()
		RETURNING id
	`

	var threadID int64
	err := r.pool.QueryRow(ctx, query,
		input.TenantID,
		input.RootMessageID,
		input.NormalizedSubject,
		input.FirstMessageAt,
		input.LastMessageAt,
		input.LatestSourceID,
	).Scan(&threadID)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert thread: %w", err)
	}

	return threadID, nil
}

// AddThreadMessage adds a message to a thread with the correct position.
func (r *PostgresThreadRepository) AddThreadMessage(ctx context.Context, input *AddThreadMessageInput) error {
	// First, count existing messages to determine position
	var existingCount int
	countQuery := `SELECT COUNT(*) FROM thread_messages WHERE thread_id = $1`
	err := r.pool.QueryRow(ctx, countQuery, input.ThreadID).Scan(&existingCount)
	if err != nil {
		return fmt.Errorf("failed to count thread messages: %w", err)
	}

	// Position is existing count + 1 (1-based)
	position := existingCount + 1

	// Insert the message
	insertQuery := `
		INSERT INTO thread_messages (
			thread_id,
			source_id,
			message_id,
			position_in_thread,
			is_reply,
			reply_to_message_id,
			message_date,
			added_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`

	_, err = r.pool.Exec(ctx, insertQuery,
		input.ThreadID,
		input.SourceID,
		input.MessageID,
		position,
		input.IsReply,
		input.ReplyToMessageID,
		input.MessageDate,
	)
	if err != nil {
		return fmt.Errorf("failed to insert thread message: %w", err)
	}

	return nil
}

// SetContentEnrichmentThreadID updates the thread_id column in content_enrichment.
func (r *PostgresThreadRepository) SetContentEnrichmentThreadID(ctx context.Context, sourceID int64, threadID string) error {
	query := `UPDATE content_enrichment SET thread_id = $1 WHERE source_id = $2`

	_, err := r.pool.Exec(ctx, query, threadID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to set content_enrichment thread_id: %w", err)
	}

	return nil
}

// GetThreadByRootMessageID retrieves a thread by its root message ID.
func (r *PostgresThreadRepository) GetThreadByRootMessageID(ctx context.Context, tenantID, rootMessageID string) (*EmailThread, error) {
	query := `
		SELECT id, root_message_id, subject, message_count
		FROM email_threads
		WHERE tenant_id = $1 AND root_message_id = $2
	`

	var thread EmailThread
	err := r.pool.QueryRow(ctx, query, tenantID, rootMessageID).Scan(
		&thread.ID,
		&thread.RootMessageID,
		&thread.Subject,
		&thread.MessageCount,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get thread by root message ID: %w", err)
	}

	return &thread, nil
}
