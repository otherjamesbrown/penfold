// Package session provides session storage interfaces and implementations.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store defines the interface for session persistence.
type Store interface {
	// Save persists a session.
	Save(ctx context.Context, session *Session) error

	// Get retrieves a session by ID.
	Get(ctx context.Context, sessionID string) (*Session, error)

	// GetByTenantAndUser retrieves active sessions for a tenant and user.
	GetByTenantAndUser(ctx context.Context, tenantID, userID string) ([]*Session, error)

	// GetExpired retrieves sessions that have expired but not been marked as such.
	GetExpired(ctx context.Context, limit int) ([]*Session, error)

	// Delete removes a session.
	Delete(ctx context.Context, sessionID string) error

	// SaveEvent persists a session event.
	SaveEvent(ctx context.Context, event *Event) error

	// GetEvents retrieves events for a session.
	GetEvents(ctx context.Context, sessionID string) ([]*Event, error)
}

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgreSQL-backed session store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema creates the necessary tables if they don't exist.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS review_sessions (
			id VARCHAR(255) PRIMARY KEY,
			tenant_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			state VARCHAR(50) NOT NULL,
			date VARCHAR(10) NOT NULL,
			items JSONB NOT NULL DEFAULT '[]',
			analytics JSONB NOT NULL DEFAULT '{}',
			metadata JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			started_at TIMESTAMPTZ,
			completed_at TIMESTAMPTZ,
			expires_at TIMESTAMPTZ NOT NULL,
			paused_at TIMESTAMPTZ
		);

		CREATE INDEX IF NOT EXISTS idx_review_sessions_tenant_user
			ON review_sessions(tenant_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_review_sessions_state
			ON review_sessions(state);
		CREATE INDEX IF NOT EXISTS idx_review_sessions_expires_at
			ON review_sessions(expires_at);
		CREATE INDEX IF NOT EXISTS idx_review_sessions_date
			ON review_sessions(date);

		CREATE TABLE IF NOT EXISTS review_session_events (
			id VARCHAR(255) PRIMARY KEY,
			type VARCHAR(100) NOT NULL,
			session_id VARCHAR(255) NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
			tenant_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			data JSONB NOT NULL DEFAULT '{}',
			correlation_id VARCHAR(255),
			version VARCHAR(50) NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_review_session_events_session
			ON review_session_events(session_id);
		CREATE INDEX IF NOT EXISTS idx_review_session_events_timestamp
			ON review_session_events(timestamp);
	`

	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Save persists a session.
func (s *PostgresStore) Save(ctx context.Context, session *Session) error {
	itemsJSON, err := json.Marshal(session.Items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}

	analyticsJSON, err := json.Marshal(session.Analytics)
	if err != nil {
		return fmt.Errorf("failed to marshal analytics: %w", err)
	}

	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO review_sessions (
			id, tenant_id, user_id, state, date, items, analytics, metadata,
			created_at, updated_at, started_at, completed_at, expires_at, paused_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			items = EXCLUDED.items,
			analytics = EXCLUDED.analytics,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			expires_at = EXCLUDED.expires_at,
			paused_at = EXCLUDED.paused_at
	`

	_, err = s.pool.Exec(ctx, query,
		session.ID,
		session.TenantID,
		session.UserID,
		string(session.State),
		session.Date,
		itemsJSON,
		analyticsJSON,
		metadataJSON,
		session.CreatedAt,
		session.UpdatedAt,
		session.StartedAt,
		session.CompletedAt,
		session.ExpiresAt,
		session.PausedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// Get retrieves a session by ID.
func (s *PostgresStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	query := `
		SELECT id, tenant_id, user_id, state, date, items, analytics, metadata,
			created_at, updated_at, started_at, completed_at, expires_at, paused_at
		FROM review_sessions
		WHERE id = $1
	`

	row := s.pool.QueryRow(ctx, query, sessionID)

	return s.scanSession(row)
}

// GetByTenantAndUser retrieves active sessions for a tenant and user.
func (s *PostgresStore) GetByTenantAndUser(ctx context.Context, tenantID, userID string) ([]*Session, error) {
	query := `
		SELECT id, tenant_id, user_id, state, date, items, analytics, metadata,
			created_at, updated_at, started_at, completed_at, expires_at, paused_at
		FROM review_sessions
		WHERE tenant_id = $1 AND user_id = $2
			AND state IN ('active', 'paused')
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, query, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	return s.scanSessions(rows)
}

// GetExpired retrieves sessions that have expired but not been marked as such.
func (s *PostgresStore) GetExpired(ctx context.Context, limit int) ([]*Session, error) {
	query := `
		SELECT id, tenant_id, user_id, state, date, items, analytics, metadata,
			created_at, updated_at, started_at, completed_at, expires_at, paused_at
		FROM review_sessions
		WHERE state IN ('active', 'paused') AND expires_at < NOW()
		ORDER BY expires_at ASC
		LIMIT $1
	`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query expired sessions: %w", err)
	}
	defer rows.Close()

	return s.scanSessions(rows)
}

// Delete removes a session.
func (s *PostgresStore) Delete(ctx context.Context, sessionID string) error {
	query := `DELETE FROM review_sessions WHERE id = $1`

	result, err := s.pool.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// SaveEvent persists a session event.
func (s *PostgresStore) SaveEvent(ctx context.Context, event *Event) error {
	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	query := `
		INSERT INTO review_session_events (
			id, type, session_id, tenant_id, user_id, timestamp, data, correlation_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = s.pool.Exec(ctx, query,
		event.ID,
		string(event.Type),
		event.SessionID,
		event.TenantID,
		event.UserID,
		event.Timestamp,
		dataJSON,
		event.CorrelationID,
		event.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to save event: %w", err)
	}

	return nil
}

// GetEvents retrieves events for a session.
func (s *PostgresStore) GetEvents(ctx context.Context, sessionID string) ([]*Event, error) {
	query := `
		SELECT id, type, session_id, tenant_id, user_id, timestamp, data, correlation_id, version
		FROM review_session_events
		WHERE session_id = $1
		ORDER BY timestamp ASC
	`

	rows, err := s.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event, err := s.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return events, nil
}

// scanSession scans a single session from a row.
func (s *PostgresStore) scanSession(row pgx.Row) (*Session, error) {
	var session Session
	var itemsJSON, analyticsJSON, metadataJSON []byte
	var state string

	err := row.Scan(
		&session.ID,
		&session.TenantID,
		&session.UserID,
		&state,
		&session.Date,
		&itemsJSON,
		&analyticsJSON,
		&metadataJSON,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.StartedAt,
		&session.CompletedAt,
		&session.ExpiresAt,
		&session.PausedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to scan session: %w", err)
	}

	session.State = State(state)

	if err := json.Unmarshal(itemsJSON, &session.Items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal items: %w", err)
	}

	if err := json.Unmarshal(analyticsJSON, &session.Analytics); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analytics: %w", err)
	}

	if err := json.Unmarshal(metadataJSON, &session.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &session, nil
}

// scanSessions scans multiple sessions from rows.
func (s *PostgresStore) scanSessions(rows pgx.Rows) ([]*Session, error) {
	var sessions []*Session

	for rows.Next() {
		var session Session
		var itemsJSON, analyticsJSON, metadataJSON []byte
		var state string

		err := rows.Scan(
			&session.ID,
			&session.TenantID,
			&session.UserID,
			&state,
			&session.Date,
			&itemsJSON,
			&analyticsJSON,
			&metadataJSON,
			&session.CreatedAt,
			&session.UpdatedAt,
			&session.StartedAt,
			&session.CompletedAt,
			&session.ExpiresAt,
			&session.PausedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.State = State(state)

		if err := json.Unmarshal(itemsJSON, &session.Items); err != nil {
			return nil, fmt.Errorf("failed to unmarshal items: %w", err)
		}

		if err := json.Unmarshal(analyticsJSON, &session.Analytics); err != nil {
			return nil, fmt.Errorf("failed to unmarshal analytics: %w", err)
		}

		if err := json.Unmarshal(metadataJSON, &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		sessions = append(sessions, &session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// scanEvent scans a single event from a row.
func (s *PostgresStore) scanEvent(rows pgx.Rows) (*Event, error) {
	var event Event
	var eventType string
	var dataJSON []byte

	err := rows.Scan(
		&event.ID,
		&eventType,
		&event.SessionID,
		&event.TenantID,
		&event.UserID,
		&event.Timestamp,
		&dataJSON,
		&event.CorrelationID,
		&event.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan event: %w", err)
	}

	event.Type = EventType(eventType)

	if err := json.Unmarshal(dataJSON, &event.Data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	return &event, nil
}

// InMemoryStore implements Store using in-memory storage for testing.
type InMemoryStore struct {
	sessions map[string]*Session
	events   map[string][]*Event
	mu       sync.RWMutex
}

// NewInMemoryStore creates a new in-memory session store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions: make(map[string]*Session),
		events:   make(map[string][]*Event),
	}
}

// Save persists a session.
func (s *InMemoryStore) Save(ctx context.Context, session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = session.Clone()
	return nil
}

// Get retrieves a session by ID.
func (s *InMemoryStore) Get(ctx context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}

	return session.Clone(), nil
}

// GetByTenantAndUser retrieves active sessions for a tenant and user.
func (s *InMemoryStore) GetByTenantAndUser(ctx context.Context, tenantID, userID string) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Session
	for _, session := range s.sessions {
		if session.TenantID == tenantID && session.UserID == userID &&
			(session.State == StateActive || session.State == StatePaused) {
			results = append(results, session.Clone())
		}
	}

	return results, nil
}

// GetExpired retrieves sessions that have expired but not been marked as such.
func (s *InMemoryStore) GetExpired(ctx context.Context, limit int) ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().UTC()
	var results []*Session

	for _, session := range s.sessions {
		if (session.State == StateActive || session.State == StatePaused) &&
			session.ExpiresAt.Before(now) {
			results = append(results, session.Clone())
			if len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// Delete removes a session.
func (s *InMemoryStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}

	delete(s.sessions, sessionID)
	delete(s.events, sessionID)
	return nil
}

// SaveEvent persists a session event.
func (s *InMemoryStore) SaveEvent(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[event.SessionID] = append(s.events[event.SessionID], event)
	return nil
}

// GetEvents retrieves events for a session.
func (s *InMemoryStore) GetEvents(ctx context.Context, sessionID string) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[sessionID]
	if events == nil {
		return []*Event{}, nil
	}

	// Return a copy
	result := make([]*Event, len(events))
	copy(result, events)
	return result, nil
}

// Clear clears all stored data (for testing).
func (s *InMemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions = make(map[string]*Session)
	s.events = make(map[string][]*Event)
}
