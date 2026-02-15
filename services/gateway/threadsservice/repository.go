// Package threadsservice repository
package threadsservice

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// PostgresRepository implements the Repository interface using pgxpool.
type PostgresRepository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool, logger logging.Logger) *PostgresRepository {
	return &PostgresRepository{
		pool:   pool,
		logger: logger.With(logging.F("component", "threads_repository")),
	}
}

// ListThreads retrieves a paginated list of threads for a tenant.
func (r *PostgresRepository) ListThreads(ctx context.Context, tenantID string, limit, offset int32) ([]ThreadSummary, int64, error) {
	// Get total count
	var totalCount int64
	countQuery := `SELECT COUNT(*) FROM email_threads WHERE tenant_id = $1`
	err := r.pool.QueryRow(ctx, countQuery, tenantID).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count threads: %w", err)
	}

	// Get threads
	query := `
		SELECT
			id,
			subject,
			message_count,
			first_message_at,
			last_message_at,
			participant_ids,
			thread_summary,
			latest_source_id
		FROM email_threads
		WHERE tenant_id = $1
		ORDER BY last_message_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []ThreadSummary
	for rows.Next() {
		var thread ThreadSummary
		err := rows.Scan(
			&thread.ID,
			&thread.Subject,
			&thread.MessageCount,
			&thread.FirstMessageAt,
			&thread.LastMessageAt,
			&thread.ParticipantIDs,
			&thread.ThreadSummary,
			&thread.LatestSourceID,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan thread: %w", err)
		}
		threads = append(threads, thread)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating threads: %w", err)
	}

	return threads, totalCount, nil
}

// GetThread retrieves a single thread with all messages.
func (r *PostgresRepository) GetThread(ctx context.Context, tenantID string, threadID int64) (*ThreadDetail, error) {
	// Get thread metadata
	threadQuery := `
		SELECT
			id,
			subject,
			message_count,
			first_message_at,
			last_message_at,
			participant_ids,
			thread_summary
		FROM email_threads
		WHERE tenant_id = $1 AND id = $2
	`

	var thread ThreadDetail
	err := r.pool.QueryRow(ctx, threadQuery, tenantID, threadID).Scan(
		&thread.ID,
		&thread.Subject,
		&thread.MessageCount,
		&thread.FirstMessageAt,
		&thread.LastMessageAt,
		&thread.ParticipantIDs,
		&thread.ThreadSummary,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query thread: %w", err)
	}

	// Get thread messages
	messagesQuery := `
		SELECT
			tm.id,
			tm.source_id,
			tm.message_id,
			tm.position_in_thread,
			tm.message_date,
			COALESCE(s.metadata->>'from_email', '') AS from_email,
			COALESCE(s.metadata->>'from_name', '') AS from_name,
			COALESCE(s.subject, '') AS subject,
			COALESCE(LEFT(s.raw_content, 200), '') AS body_preview,
			tm.is_reply
		FROM thread_messages tm
		LEFT JOIN sources s ON tm.source_id = s.id
		WHERE tm.thread_id = $1
		ORDER BY tm.position_in_thread ASC
	`

	rows, err := r.pool.Query(ctx, messagesQuery, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread messages: %w", err)
	}
	defer rows.Close()

	var messages []ThreadMessage
	for rows.Next() {
		var msg ThreadMessage
		err := rows.Scan(
			&msg.ID,
			&msg.SourceID,
			&msg.MessageID,
			&msg.PositionInThread,
			&msg.MessageDate,
			&msg.FromEmail,
			&msg.FromName,
			&msg.Subject,
			&msg.BodyPreview,
			&msg.IsReply,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan thread message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating thread messages: %w", err)
	}

	thread.Messages = messages
	return &thread, nil
}
