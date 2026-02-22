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

	// Insert the message (idempotent — skip if already exists for this thread+source)
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
		ON CONFLICT (thread_id, source_id) DO NOTHING
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

	// Update thread message_count from ground truth to handle concurrent workflows
	updateCountQuery := `
		UPDATE email_threads
		SET message_count = (SELECT COUNT(*) FROM thread_messages WHERE thread_id = $1),
		    updated_at = NOW()
		WHERE id = $1
	`
	_, err = r.pool.Exec(ctx, updateCountQuery, input.ThreadID)
	if err != nil {
		return fmt.Errorf("failed to update thread message count: %w", err)
	}

	return nil
}

// SetContentEnrichmentThreadID updates the thread_id column in content_enrichment
// and also stores it in sources.ingestion_metadata for CLI visibility.
func (r *PostgresThreadRepository) SetContentEnrichmentThreadID(ctx context.Context, sourceID int64, threadID string) error {
	query := `UPDATE content_enrichment SET thread_id = $1 WHERE source_id = $2`

	result, err := r.pool.Exec(ctx, query, threadID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to set content_enrichment thread_id: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no content_enrichment row found for source_id %d (thread_id update affected 0 rows)", sourceID)
	}

	// Also store thread_id in sources.ingestion_metadata so penf content show can display it
	metaQuery := `
		UPDATE sources
		SET ingestion_metadata = COALESCE(ingestion_metadata, '{}'::jsonb) || jsonb_build_object('thread_id', $1::text),
		    updated_at = NOW()
		WHERE id = $2
	`
	_, metaErr := r.pool.Exec(ctx, metaQuery, threadID, sourceID)
	if metaErr != nil {
		r.logger.Warn("failed to store thread_id in ingestion_metadata (content_enrichment updated successfully)",
			logging.F("source_id", sourceID),
			logging.F("error", metaErr.Error()),
		)
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

// GetThreadByMemberMessageID retrieves the thread that contains a given message ID.
// This checks thread_messages.message_id (any position in the thread), not just the root.
func (r *PostgresThreadRepository) GetThreadByMemberMessageID(ctx context.Context, tenantID, messageID string) (*EmailThread, error) {
	query := `
		SELECT et.id, et.root_message_id, et.subject, et.message_count
		FROM email_threads et
		JOIN thread_messages tm ON tm.thread_id = et.id
		WHERE et.tenant_id = $1 AND tm.message_id = $2
		LIMIT 1
	`

	var thread EmailThread
	err := r.pool.QueryRow(ctx, query, tenantID, messageID).Scan(
		&thread.ID,
		&thread.RootMessageID,
		&thread.Subject,
		&thread.MessageCount,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get thread by member message ID: %w", err)
	}

	return &thread, nil
}
