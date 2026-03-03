// Package bridgeservice implements the BridgeService gRPC server.
package bridgeservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Session represents an active bridge session.
type Session struct {
	ID         int64
	TenantID   string
	SessionID  string
	Channel    string
	SenderID   string
	Messages   []Message
	CreatedAt  time.Time
	LastActive time.Time
}

// Message is a single chat message stored in the JSONB messages column.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	TS      string `json:"ts"`
}

// Repository provides data access for bridge_sessions.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new Repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "bridge_repository")),
	}
}

// GetOrCreateSession upserts a session and returns it. If a session with the
// given (tenant_id, session_id) already exists it is returned unchanged.
func (r *Repository) GetOrCreateSession(ctx context.Context, tenantID, sessionID, channel, senderID string) (*Session, error) {
	query := `
		INSERT INTO bridge_sessions (tenant_id, session_id, channel, sender_id, messages, created_at, last_active)
		VALUES ($1, $2, $3, $4, '[]'::jsonb, NOW(), NOW())
		ON CONFLICT (tenant_id, session_id) DO UPDATE
			SET last_active = EXCLUDED.last_active
		RETURNING id, tenant_id, session_id, channel, sender_id, messages, created_at, last_active
	`

	row := r.pool.QueryRow(ctx, query, tenantID, sessionID, channel, senderID)
	return scanSession(row)
}

// AppendMessage appends a message to the session's messages JSONB array and
// bumps last_active.
func (r *Repository) AppendMessage(ctx context.Context, tenantID, sessionID, role, content string) error {
	msg := Message{
		Role:    role,
		Content: content,
		TS:      time.Now().UTC().Format(time.RFC3339),
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	query := `
		UPDATE bridge_sessions
		SET messages    = messages || $1::jsonb,
		    last_active = NOW()
		WHERE tenant_id = $2 AND session_id = $3
	`

	tag, err := r.pool.Exec(ctx, query, string(msgJSON), tenantID, sessionID)
	if err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session not found: tenant=%s session=%s", tenantID, sessionID)
	}

	return nil
}

// GetSession returns the session or nil if it does not exist.
func (r *Repository) GetSession(ctx context.Context, tenantID, sessionID string) (*Session, error) {
	query := `
		SELECT id, tenant_id, session_id, channel, sender_id, messages, created_at, last_active
		FROM bridge_sessions
		WHERE tenant_id = $1 AND session_id = $2
	`

	row := r.pool.QueryRow(ctx, query, tenantID, sessionID)
	session, err := scanSession(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return session, nil
}

// DeleteSession removes a session. No error if it does not exist.
func (r *Repository) DeleteSession(ctx context.Context, tenantID, sessionID string) error {
	query := `DELETE FROM bridge_sessions WHERE tenant_id = $1 AND session_id = $2`
	_, err := r.pool.Exec(ctx, query, tenantID, sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// GetConfig reads a value from pipeline_operational_config. Returns an error
// (and empty string) when the key does not exist.
func (r *Repository) GetConfig(ctx context.Context, tenantID, key string) (string, error) {
	var value string
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM pipeline_operational_config
		WHERE tenant_id = $1 AND key = $2
	`, tenantID, key).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("config key not found: %s", key)
		}
		return "", fmt.Errorf("query config %s: %w", key, err)
	}
	return value, nil
}

// GetPrompt reads the active prompt for a pipeline stage from prompt_templates.
func (r *Repository) GetPrompt(ctx context.Context, stage string) (string, error) {
	var content string
	err := r.pool.QueryRow(ctx, `
		SELECT content
		FROM prompt_templates
		WHERE stage = $1 AND is_active = true
	`, stage).Scan(&content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("no active prompt for stage: %s", stage)
		}
		return "", fmt.Errorf("query prompt for stage %s: %w", stage, err)
	}
	return content, nil
}

// scanSession scans a single row into a Session.
func scanSession(row pgx.Row) (*Session, error) {
	var s Session
	var messagesJSON []byte

	err := row.Scan(
		&s.ID,
		&s.TenantID,
		&s.SessionID,
		&s.Channel,
		&s.SenderID,
		&messagesJSON,
		&s.CreatedAt,
		&s.LastActive,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(messagesJSON, &s.Messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	return &s, nil
}
