// Package sync provides Gmail synchronization capabilities including
// full sync, incremental sync via History API, and resumable sync state.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncState represents the persistent state of a sync operation for a tenant.
type SyncState struct {
	// TenantID is the unique identifier for the tenant.
	TenantID string `json:"tenant_id"`

	// SyncID is the unique identifier for this sync operation.
	SyncID string `json:"sync_id"`

	// HistoryID is the Gmail history ID for incremental sync.
	// This is used to fetch only changes since the last sync.
	HistoryID uint64 `json:"history_id"`

	// LastSyncAt is the timestamp of the last successful sync.
	LastSyncAt time.Time `json:"last_sync_at"`

	// LastFullSyncAt is the timestamp of the last full sync.
	LastFullSyncAt time.Time `json:"last_full_sync_at"`

	// NextPageToken is used for resumable pagination.
	// If non-empty, indicates a sync was interrupted.
	NextPageToken string `json:"next_page_token,omitempty"`

	// ProcessedCount is the number of messages processed in the current sync.
	ProcessedCount int64 `json:"processed_count"`

	// TotalCount is the estimated total messages to process.
	TotalCount int64 `json:"total_count"`

	// ErrorCount is the number of errors encountered during sync.
	ErrorCount int64 `json:"error_count"`

	// SyncType indicates the type of sync (full, incremental).
	SyncType SyncType `json:"sync_type"`

	// Status indicates the current status of the sync.
	Status SyncStatus `json:"status"`

	// Labels being synced (empty means all).
	Labels []string `json:"labels,omitempty"`

	// Query is the Gmail search query filter.
	Query string `json:"query,omitempty"`

	// Errors accumulated during sync.
	Errors []SyncErrorRecord `json:"errors,omitempty"`

	// CreatedAt is when this sync state was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this sync state was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncType represents the type of sync operation.
type SyncType string

const (
	// SyncTypeFull indicates a full mailbox sync.
	SyncTypeFull SyncType = "full"

	// SyncTypeIncremental indicates an incremental sync using History API.
	SyncTypeIncremental SyncType = "incremental"

	// SyncTypeResume indicates a resumed sync from a previous interruption.
	SyncTypeResume SyncType = "resume"
)

// SyncStatus represents the status of a sync operation.
type SyncStatus string

const (
	// SyncStatusPending indicates the sync is queued.
	SyncStatusPending SyncStatus = "pending"

	// SyncStatusInProgress indicates the sync is actively running.
	SyncStatusInProgress SyncStatus = "in_progress"

	// SyncStatusCompleted indicates the sync finished successfully.
	SyncStatusCompleted SyncStatus = "completed"

	// SyncStatusCompletedWithErrors indicates the sync finished with some errors.
	SyncStatusCompletedWithErrors SyncStatus = "completed_with_errors"

	// SyncStatusFailed indicates the sync failed.
	SyncStatusFailed SyncStatus = "failed"

	// SyncStatusCancelled indicates the sync was cancelled.
	SyncStatusCancelled SyncStatus = "cancelled"

	// SyncStatusInterrupted indicates the sync was interrupted and can be resumed.
	SyncStatusInterrupted SyncStatus = "interrupted"
)

// SyncErrorRecord represents an error that occurred during sync.
type SyncErrorRecord struct {
	// EmailID is the Gmail message ID that caused the error (if applicable).
	EmailID string `json:"email_id,omitempty"`

	// Code is the error code.
	Code string `json:"code"`

	// Message is the human-readable error message.
	Message string `json:"message"`

	// Timestamp is when the error occurred.
	Timestamp time.Time `json:"timestamp"`

	// Retryable indicates if the error is retryable.
	Retryable bool `json:"retryable"`
}

// IsComplete returns true if the sync has finished (successfully or not).
func (s *SyncState) IsComplete() bool {
	switch s.Status {
	case SyncStatusCompleted, SyncStatusCompletedWithErrors, SyncStatusFailed, SyncStatusCancelled:
		return true
	default:
		return false
	}
}

// CanResume returns true if the sync can be resumed.
func (s *SyncState) CanResume() bool {
	return s.Status == SyncStatusInterrupted && s.NextPageToken != ""
}

// Progress returns the sync progress as a percentage (0-100).
func (s *SyncState) Progress() int32 {
	if s.TotalCount == 0 {
		return 0
	}
	progress := float64(s.ProcessedCount) / float64(s.TotalCount) * 100
	if progress > 100 {
		progress = 100
	}
	return int32(progress)
}

// StateStorage defines the interface for sync state persistence.
type StateStorage interface {
	// SaveState persists the sync state.
	SaveState(ctx context.Context, state *SyncState) error

	// GetState retrieves the sync state for a tenant.
	GetState(ctx context.Context, tenantID string) (*SyncState, error)

	// GetStateByID retrieves a specific sync state by ID.
	GetStateByID(ctx context.Context, syncID string) (*SyncState, error)

	// GetLatestState retrieves the most recent sync state for a tenant.
	GetLatestState(ctx context.Context, tenantID string) (*SyncState, error)

	// ListStates retrieves all sync states for a tenant.
	ListStates(ctx context.Context, tenantID string, limit int) ([]*SyncState, error)

	// DeleteState removes a sync state.
	DeleteState(ctx context.Context, syncID string) error

	// CleanupOldStates removes sync states older than the given duration.
	CleanupOldStates(ctx context.Context, olderThan time.Duration) (int64, error)
}

// Errors for state operations.
var (
	ErrStateNotFound = fmt.Errorf("sync: state not found")
)

// PostgresStateStorage implements StateStorage using PostgreSQL.
type PostgresStateStorage struct {
	pool      *pgxpool.Pool
	tableName string
}

// PostgresStateStorageConfig holds configuration for PostgresStateStorage.
type PostgresStateStorageConfig struct {
	// Pool is the PostgreSQL connection pool.
	Pool *pgxpool.Pool

	// TableName is the table name for storing sync states.
	// Defaults to "gmail_sync_state".
	TableName string
}

// NewPostgresStateStorage creates a new PostgresStateStorage.
func NewPostgresStateStorage(cfg *PostgresStateStorageConfig) (*PostgresStateStorage, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "gmail_sync_state"
	}

	return &PostgresStateStorage{
		pool:      cfg.Pool,
		tableName: tableName,
	}, nil
}

// EnsureTable creates the sync state table if it doesn't exist.
func (s *PostgresStateStorage) EnsureTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			sync_id VARCHAR(255) PRIMARY KEY,
			tenant_id VARCHAR(255) NOT NULL,
			history_id BIGINT NOT NULL DEFAULT 0,
			last_sync_at TIMESTAMPTZ,
			last_full_sync_at TIMESTAMPTZ,
			next_page_token TEXT,
			processed_count BIGINT NOT NULL DEFAULT 0,
			total_count BIGINT NOT NULL DEFAULT 0,
			error_count BIGINT NOT NULL DEFAULT 0,
			sync_type VARCHAR(50) NOT NULL,
			status VARCHAR(50) NOT NULL,
			labels TEXT[],
			query TEXT,
			errors_json JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_%s_tenant_id ON %s (tenant_id);
		CREATE INDEX IF NOT EXISTS idx_%s_status ON %s (status);
		CREATE INDEX IF NOT EXISTS idx_%s_created_at ON %s (created_at DESC);
	`, s.tableName, s.tableName, s.tableName, s.tableName, s.tableName, s.tableName, s.tableName)

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("creating sync state table: %w", err)
	}

	return nil
}

// SaveState persists the sync state.
func (s *PostgresStateStorage) SaveState(ctx context.Context, state *SyncState) error {
	state.UpdatedAt = time.Now()

	errorsJSON, err := json.Marshal(state.Errors)
	if err != nil {
		return fmt.Errorf("marshaling errors: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (
			sync_id, tenant_id, history_id, last_sync_at, last_full_sync_at,
			next_page_token, processed_count, total_count, error_count,
			sync_type, status, labels, query, errors_json, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (sync_id) DO UPDATE SET
			history_id = EXCLUDED.history_id,
			last_sync_at = EXCLUDED.last_sync_at,
			last_full_sync_at = EXCLUDED.last_full_sync_at,
			next_page_token = EXCLUDED.next_page_token,
			processed_count = EXCLUDED.processed_count,
			total_count = EXCLUDED.total_count,
			error_count = EXCLUDED.error_count,
			sync_type = EXCLUDED.sync_type,
			status = EXCLUDED.status,
			labels = EXCLUDED.labels,
			query = EXCLUDED.query,
			errors_json = EXCLUDED.errors_json,
			updated_at = EXCLUDED.updated_at
	`, s.tableName)

	_, err = s.pool.Exec(ctx, query,
		state.SyncID,
		state.TenantID,
		state.HistoryID,
		nullTime(state.LastSyncAt),
		nullTime(state.LastFullSyncAt),
		nullString(state.NextPageToken),
		state.ProcessedCount,
		state.TotalCount,
		state.ErrorCount,
		string(state.SyncType),
		string(state.Status),
		state.Labels,
		nullString(state.Query),
		errorsJSON,
		state.CreatedAt,
		state.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving sync state: %w", err)
	}

	return nil
}

// GetState retrieves the current (latest in-progress or most recent) sync state for a tenant.
func (s *PostgresStateStorage) GetState(ctx context.Context, tenantID string) (*SyncState, error) {
	// First try to find an in-progress sync
	query := fmt.Sprintf(`
		SELECT sync_id, tenant_id, history_id, last_sync_at, last_full_sync_at,
			   next_page_token, processed_count, total_count, error_count,
			   sync_type, status, labels, query, errors_json, created_at, updated_at
		FROM %s
		WHERE tenant_id = $1 AND status IN ('pending', 'in_progress', 'interrupted')
		ORDER BY created_at DESC
		LIMIT 1
	`, s.tableName)

	state, err := s.scanState(ctx, query, tenantID)
	if err == nil {
		return state, nil
	}
	if err != ErrStateNotFound {
		return nil, err
	}

	// Fall back to most recent completed sync
	return s.GetLatestState(ctx, tenantID)
}

// GetStateByID retrieves a specific sync state by ID.
func (s *PostgresStateStorage) GetStateByID(ctx context.Context, syncID string) (*SyncState, error) {
	query := fmt.Sprintf(`
		SELECT sync_id, tenant_id, history_id, last_sync_at, last_full_sync_at,
			   next_page_token, processed_count, total_count, error_count,
			   sync_type, status, labels, query, errors_json, created_at, updated_at
		FROM %s
		WHERE sync_id = $1
	`, s.tableName)

	return s.scanState(ctx, query, syncID)
}

// GetLatestState retrieves the most recent completed sync state for a tenant.
func (s *PostgresStateStorage) GetLatestState(ctx context.Context, tenantID string) (*SyncState, error) {
	query := fmt.Sprintf(`
		SELECT sync_id, tenant_id, history_id, last_sync_at, last_full_sync_at,
			   next_page_token, processed_count, total_count, error_count,
			   sync_type, status, labels, query, errors_json, created_at, updated_at
		FROM %s
		WHERE tenant_id = $1 AND status IN ('completed', 'completed_with_errors')
		ORDER BY last_sync_at DESC NULLS LAST
		LIMIT 1
	`, s.tableName)

	return s.scanState(ctx, query, tenantID)
}

// scanState scans a single state row from the database.
func (s *PostgresStateStorage) scanState(ctx context.Context, query string, arg interface{}) (*SyncState, error) {
	var state SyncState
	var lastSyncAt, lastFullSyncAt *time.Time
	var nextPageToken, queryStr *string
	var syncType, status string
	var errorsJSON []byte

	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&state.SyncID,
		&state.TenantID,
		&state.HistoryID,
		&lastSyncAt,
		&lastFullSyncAt,
		&nextPageToken,
		&state.ProcessedCount,
		&state.TotalCount,
		&state.ErrorCount,
		&syncType,
		&status,
		&state.Labels,
		&queryStr,
		&errorsJSON,
		&state.CreatedAt,
		&state.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrStateNotFound
		}
		return nil, fmt.Errorf("querying sync state: %w", err)
	}

	if lastSyncAt != nil {
		state.LastSyncAt = *lastSyncAt
	}
	if lastFullSyncAt != nil {
		state.LastFullSyncAt = *lastFullSyncAt
	}
	if nextPageToken != nil {
		state.NextPageToken = *nextPageToken
	}
	if queryStr != nil {
		state.Query = *queryStr
	}

	state.SyncType = SyncType(syncType)
	state.Status = SyncStatus(status)

	if len(errorsJSON) > 0 {
		if err := json.Unmarshal(errorsJSON, &state.Errors); err != nil {
			return nil, fmt.Errorf("unmarshaling errors: %w", err)
		}
	}

	return &state, nil
}

// ListStates retrieves all sync states for a tenant.
func (s *PostgresStateStorage) ListStates(ctx context.Context, tenantID string, limit int) ([]*SyncState, error) {
	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT sync_id, tenant_id, history_id, last_sync_at, last_full_sync_at,
			   next_page_token, processed_count, total_count, error_count,
			   sync_type, status, labels, query, errors_json, created_at, updated_at
		FROM %s
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, s.tableName)

	rows, err := s.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sync states: %w", err)
	}
	defer rows.Close()

	var states []*SyncState
	for rows.Next() {
		var state SyncState
		var lastSyncAt, lastFullSyncAt *time.Time
		var nextPageToken, queryStr *string
		var syncType, status string
		var errorsJSON []byte

		err := rows.Scan(
			&state.SyncID,
			&state.TenantID,
			&state.HistoryID,
			&lastSyncAt,
			&lastFullSyncAt,
			&nextPageToken,
			&state.ProcessedCount,
			&state.TotalCount,
			&state.ErrorCount,
			&syncType,
			&status,
			&state.Labels,
			&queryStr,
			&errorsJSON,
			&state.CreatedAt,
			&state.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning sync state row: %w", err)
		}

		if lastSyncAt != nil {
			state.LastSyncAt = *lastSyncAt
		}
		if lastFullSyncAt != nil {
			state.LastFullSyncAt = *lastFullSyncAt
		}
		if nextPageToken != nil {
			state.NextPageToken = *nextPageToken
		}
		if queryStr != nil {
			state.Query = *queryStr
		}

		state.SyncType = SyncType(syncType)
		state.Status = SyncStatus(status)

		if len(errorsJSON) > 0 {
			if err := json.Unmarshal(errorsJSON, &state.Errors); err != nil {
				return nil, fmt.Errorf("unmarshaling errors: %w", err)
			}
		}

		states = append(states, &state)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sync state rows: %w", err)
	}

	return states, nil
}

// DeleteState removes a sync state.
func (s *PostgresStateStorage) DeleteState(ctx context.Context, syncID string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE sync_id = $1`, s.tableName)

	result, err := s.pool.Exec(ctx, query, syncID)
	if err != nil {
		return fmt.Errorf("deleting sync state: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrStateNotFound
	}

	return nil
}

// CleanupOldStates removes sync states older than the given duration.
func (s *PostgresStateStorage) CleanupOldStates(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE created_at < $1
		AND status IN ('completed', 'completed_with_errors', 'failed', 'cancelled')
	`, s.tableName)

	result, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("cleaning up old sync states: %w", err)
	}

	return result.RowsAffected(), nil
}

// MemoryStateStorage implements StateStorage using in-memory storage.
// Useful for testing.
type MemoryStateStorage struct {
	states map[string]*SyncState
}

// NewMemoryStateStorage creates a new MemoryStateStorage.
func NewMemoryStateStorage() *MemoryStateStorage {
	return &MemoryStateStorage{
		states: make(map[string]*SyncState),
	}
}

// SaveState persists the sync state.
func (s *MemoryStateStorage) SaveState(ctx context.Context, state *SyncState) error {
	state.UpdatedAt = time.Now()
	// Make a copy to avoid mutation
	stateCopy := *state
	stateCopy.Labels = append([]string{}, state.Labels...)
	stateCopy.Errors = append([]SyncErrorRecord{}, state.Errors...)
	s.states[state.SyncID] = &stateCopy
	return nil
}

// GetState retrieves the current sync state for a tenant.
func (s *MemoryStateStorage) GetState(ctx context.Context, tenantID string) (*SyncState, error) {
	// First try to find an in-progress sync
	for _, state := range s.states {
		if state.TenantID == tenantID &&
			(state.Status == SyncStatusPending ||
				state.Status == SyncStatusInProgress ||
				state.Status == SyncStatusInterrupted) {
			stateCopy := *state
			return &stateCopy, nil
		}
	}
	// Fall back to latest completed
	return s.GetLatestState(ctx, tenantID)
}

// GetStateByID retrieves a specific sync state by ID.
func (s *MemoryStateStorage) GetStateByID(ctx context.Context, syncID string) (*SyncState, error) {
	state, ok := s.states[syncID]
	if !ok {
		return nil, ErrStateNotFound
	}
	stateCopy := *state
	return &stateCopy, nil
}

// GetLatestState retrieves the most recent completed sync state for a tenant.
func (s *MemoryStateStorage) GetLatestState(ctx context.Context, tenantID string) (*SyncState, error) {
	var latest *SyncState
	for _, state := range s.states {
		if state.TenantID == tenantID &&
			(state.Status == SyncStatusCompleted || state.Status == SyncStatusCompletedWithErrors) {
			if latest == nil || state.LastSyncAt.After(latest.LastSyncAt) {
				latest = state
			}
		}
	}
	if latest == nil {
		return nil, ErrStateNotFound
	}
	stateCopy := *latest
	return &stateCopy, nil
}

// ListStates retrieves all sync states for a tenant.
func (s *MemoryStateStorage) ListStates(ctx context.Context, tenantID string, limit int) ([]*SyncState, error) {
	var states []*SyncState
	for _, state := range s.states {
		if state.TenantID == tenantID {
			stateCopy := *state
			states = append(states, &stateCopy)
		}
	}
	if limit > 0 && len(states) > limit {
		states = states[:limit]
	}
	return states, nil
}

// DeleteState removes a sync state.
func (s *MemoryStateStorage) DeleteState(ctx context.Context, syncID string) error {
	if _, ok := s.states[syncID]; !ok {
		return ErrStateNotFound
	}
	delete(s.states, syncID)
	return nil
}

// CleanupOldStates removes sync states older than the given duration.
func (s *MemoryStateStorage) CleanupOldStates(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	var count int64
	for id, state := range s.states {
		if state.CreatedAt.Before(cutoff) && state.IsComplete() {
			delete(s.states, id)
			count++
		}
	}
	return count, nil
}

// Ensure interfaces are implemented.
var _ StateStorage = (*PostgresStateStorage)(nil)
var _ StateStorage = (*MemoryStateStorage)(nil)

// Helper functions for nullable values.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
