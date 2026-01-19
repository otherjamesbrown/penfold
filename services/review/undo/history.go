// Package undo provides undo/redo functionality for review actions.
package undo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Operation represents the type of operation performed on an action.
type Operation string

const (
	// OperationExecute indicates the action was executed.
	OperationExecute Operation = "execute"

	// OperationUndo indicates the action was undone.
	OperationUndo Operation = "undo"

	// OperationRedo indicates the action was redone.
	OperationRedo Operation = "redo"
)

// HistoryEntry represents a single entry in the action history.
type HistoryEntry struct {
	// ID is the unique identifier for this history entry.
	ID string `json:"id"`

	// ActionID is the ID of the action this entry is for.
	ActionID string `json:"action_id"`

	// ActionType is the type of the action.
	ActionType ActionType `json:"action_type"`

	// Description is a human-readable description of the action.
	Description string `json:"description"`

	// Operation is the operation performed (execute, undo, redo).
	Operation Operation `json:"operation"`

	// TenantID is the tenant this entry belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who performed the action.
	UserID string `json:"user_id"`

	// SessionID is the session the action was performed in.
	SessionID string `json:"session_id"`

	// Timestamp is when the operation was performed.
	Timestamp time.Time `json:"timestamp"`

	// ActionData contains serialized action-specific data.
	ActionData map[string]interface{} `json:"action_data,omitempty"`

	// Metadata contains additional metadata.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// CorrelationID for tracing across services.
	CorrelationID string `json:"correlation_id,omitempty"`
}

// HistoryFilter defines filters for querying action history.
type HistoryFilter struct {
	// SessionID filters by session.
	SessionID string

	// TenantID filters by tenant.
	TenantID string

	// UserID filters by user.
	UserID string

	// ActionType filters by action type.
	ActionType *ActionType

	// Operation filters by operation type.
	Operation *Operation

	// StartTime filters entries after this time.
	StartTime *time.Time

	// EndTime filters entries before this time.
	EndTime *time.Time

	// Limit is the maximum number of entries to return.
	Limit int

	// Offset is the number of entries to skip.
	Offset int

	// OrderDesc orders by timestamp descending when true.
	OrderDesc bool
}

// HistoryStore defines the interface for action history persistence.
type HistoryStore interface {
	// RecordOperation records an operation on an action.
	RecordOperation(ctx context.Context, action UndoableAction, operation Operation) error

	// GetHistory retrieves action history based on filters.
	GetHistory(ctx context.Context, filter HistoryFilter) ([]HistoryEntry, error)

	// GetActionHistory retrieves all history for a specific action.
	GetActionHistory(ctx context.Context, actionID string) ([]HistoryEntry, error)

	// DeleteHistory deletes history entries older than the specified duration.
	DeleteHistory(ctx context.Context, olderThan time.Duration) (int, error)

	// GetHistoryCount returns the count of history entries matching the filter.
	GetHistoryCount(ctx context.Context, filter HistoryFilter) (int, error)
}

// PostgresHistoryStore implements HistoryStore using PostgreSQL.
type PostgresHistoryStore struct {
	pool *pgxpool.Pool
}

// NewPostgresHistoryStore creates a new PostgreSQL-backed history store.
func NewPostgresHistoryStore(pool *pgxpool.Pool) *PostgresHistoryStore {
	return &PostgresHistoryStore{pool: pool}
}

// EnsureSchema creates the history table if it doesn't exist.
func (s *PostgresHistoryStore) EnsureSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS undo_history (
			id VARCHAR(255) PRIMARY KEY,
			action_id VARCHAR(255) NOT NULL,
			action_type VARCHAR(50) NOT NULL,
			description TEXT NOT NULL,
			operation VARCHAR(20) NOT NULL,
			tenant_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			session_id VARCHAR(255) NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			action_data JSONB NOT NULL DEFAULT '{}',
			metadata JSONB NOT NULL DEFAULT '{}',
			correlation_id VARCHAR(255)
		);

		CREATE INDEX IF NOT EXISTS idx_undo_history_session
			ON undo_history(session_id);
		CREATE INDEX IF NOT EXISTS idx_undo_history_action
			ON undo_history(action_id);
		CREATE INDEX IF NOT EXISTS idx_undo_history_tenant_user
			ON undo_history(tenant_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_undo_history_timestamp
			ON undo_history(timestamp);
		CREATE INDEX IF NOT EXISTS idx_undo_history_operation
			ON undo_history(operation);
	`

	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("create history schema: %w", err)
	}

	return nil
}

// RecordOperation records an operation on an action.
func (s *PostgresHistoryStore) RecordOperation(ctx context.Context, action UndoableAction, operation Operation) error {
	entryID := fmt.Sprintf("%s-%s-%d", action.ID(), operation, time.Now().UnixNano())

	actionData, err := action.ToJSON()
	if err != nil {
		return fmt.Errorf("serialize action: %w", err)
	}

	metadataJSON, err := json.Marshal(action.Metadata())
	if err != nil {
		return fmt.Errorf("serialize metadata: %w", err)
	}

	query := `
		INSERT INTO undo_history (
			id, action_id, action_type, description, operation,
			tenant_id, user_id, session_id, timestamp, action_data, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = s.pool.Exec(ctx, query,
		entryID,
		action.ID(),
		string(action.Type()),
		action.Description(),
		string(operation),
		action.TenantID(),
		action.UserID(),
		action.SessionID(),
		time.Now().UTC(),
		actionData,
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("record operation: %w", err)
	}

	return nil
}

// GetHistory retrieves action history based on filters.
func (s *PostgresHistoryStore) GetHistory(ctx context.Context, filter HistoryFilter) ([]HistoryEntry, error) {
	query := `
		SELECT id, action_id, action_type, description, operation,
			tenant_id, user_id, session_id, timestamp, action_data, metadata, correlation_id
		FROM undo_history
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argNum := 1

	if filter.SessionID != "" {
		query += fmt.Sprintf(" AND session_id = $%d", argNum)
		args = append(args, filter.SessionID)
		argNum++
	}

	if filter.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argNum)
		args = append(args, filter.TenantID)
		argNum++
	}

	if filter.UserID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argNum)
		args = append(args, filter.UserID)
		argNum++
	}

	if filter.ActionType != nil {
		query += fmt.Sprintf(" AND action_type = $%d", argNum)
		args = append(args, string(*filter.ActionType))
		argNum++
	}

	if filter.Operation != nil {
		query += fmt.Sprintf(" AND operation = $%d", argNum)
		args = append(args, string(*filter.Operation))
		argNum++
	}

	if filter.StartTime != nil {
		query += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, *filter.StartTime)
		argNum++
	}

	if filter.EndTime != nil {
		query += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, *filter.EndTime)
		argNum++
	}

	if filter.OrderDesc {
		query += " ORDER BY timestamp DESC"
	} else {
		query += " ORDER BY timestamp ASC"
	}

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, filter.Limit)
		argNum++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argNum)
		args = append(args, filter.Offset)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		var actionType, operation string
		var actionDataJSON, metadataJSON []byte
		var correlationID *string

		err := rows.Scan(
			&entry.ID,
			&entry.ActionID,
			&actionType,
			&entry.Description,
			&operation,
			&entry.TenantID,
			&entry.UserID,
			&entry.SessionID,
			&entry.Timestamp,
			&actionDataJSON,
			&metadataJSON,
			&correlationID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan history entry: %w", err)
		}

		entry.ActionType = ActionType(actionType)
		entry.Operation = Operation(operation)

		if err := json.Unmarshal(actionDataJSON, &entry.ActionData); err != nil {
			return nil, fmt.Errorf("parse action data: %w", err)
		}

		if err := json.Unmarshal(metadataJSON, &entry.Metadata); err != nil {
			return nil, fmt.Errorf("parse metadata: %w", err)
		}

		if correlationID != nil {
			entry.CorrelationID = *correlationID
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}

	return entries, nil
}

// GetActionHistory retrieves all history for a specific action.
func (s *PostgresHistoryStore) GetActionHistory(ctx context.Context, actionID string) ([]HistoryEntry, error) {
	return s.GetHistory(ctx, HistoryFilter{
		Limit: 1000, // Reasonable limit for a single action
	})
}

// DeleteHistory deletes history entries older than the specified duration.
func (s *PostgresHistoryStore) DeleteHistory(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan)

	query := `DELETE FROM undo_history WHERE timestamp < $1`

	result, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete history: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// GetHistoryCount returns the count of history entries matching the filter.
func (s *PostgresHistoryStore) GetHistoryCount(ctx context.Context, filter HistoryFilter) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM undo_history
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argNum := 1

	if filter.SessionID != "" {
		query += fmt.Sprintf(" AND session_id = $%d", argNum)
		args = append(args, filter.SessionID)
		argNum++
	}

	if filter.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argNum)
		args = append(args, filter.TenantID)
		argNum++
	}

	if filter.UserID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argNum)
		args = append(args, filter.UserID)
		argNum++
	}

	if filter.ActionType != nil {
		query += fmt.Sprintf(" AND action_type = $%d", argNum)
		args = append(args, string(*filter.ActionType))
		argNum++
	}

	if filter.Operation != nil {
		query += fmt.Sprintf(" AND operation = $%d", argNum)
		args = append(args, string(*filter.Operation))
		argNum++
	}

	if filter.StartTime != nil {
		query += fmt.Sprintf(" AND timestamp >= $%d", argNum)
		args = append(args, *filter.StartTime)
		argNum++
	}

	if filter.EndTime != nil {
		query += fmt.Sprintf(" AND timestamp <= $%d", argNum)
		args = append(args, *filter.EndTime)
	}

	var count int
	err := s.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count history: %w", err)
	}

	return count, nil
}

// InMemoryHistoryStore implements HistoryStore using in-memory storage for testing.
type InMemoryHistoryStore struct {
	entries []HistoryEntry
}

// NewInMemoryHistoryStore creates a new in-memory history store.
func NewInMemoryHistoryStore() *InMemoryHistoryStore {
	return &InMemoryHistoryStore{
		entries: make([]HistoryEntry, 0),
	}
}

// RecordOperation records an operation on an action.
func (s *InMemoryHistoryStore) RecordOperation(ctx context.Context, action UndoableAction, operation Operation) error {
	entryID := fmt.Sprintf("%s-%s-%d", action.ID(), operation, time.Now().UnixNano())

	actionData, err := action.ToJSON()
	if err != nil {
		return fmt.Errorf("serialize action: %w", err)
	}

	var parsedData map[string]interface{}
	if err := json.Unmarshal(actionData, &parsedData); err != nil {
		return fmt.Errorf("parse action data: %w", err)
	}

	entry := HistoryEntry{
		ID:          entryID,
		ActionID:    action.ID(),
		ActionType:  action.Type(),
		Description: action.Description(),
		Operation:   operation,
		TenantID:    action.TenantID(),
		UserID:      action.UserID(),
		SessionID:   action.SessionID(),
		Timestamp:   time.Now().UTC(),
		ActionData:  parsedData,
		Metadata:    action.Metadata(),
	}

	s.entries = append(s.entries, entry)
	return nil
}

// GetHistory retrieves action history based on filters.
func (s *InMemoryHistoryStore) GetHistory(ctx context.Context, filter HistoryFilter) ([]HistoryEntry, error) {
	var results []HistoryEntry

	for _, entry := range s.entries {
		if filter.SessionID != "" && entry.SessionID != filter.SessionID {
			continue
		}
		if filter.TenantID != "" && entry.TenantID != filter.TenantID {
			continue
		}
		if filter.UserID != "" && entry.UserID != filter.UserID {
			continue
		}
		if filter.ActionType != nil && entry.ActionType != *filter.ActionType {
			continue
		}
		if filter.Operation != nil && entry.Operation != *filter.Operation {
			continue
		}
		if filter.StartTime != nil && entry.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && entry.Timestamp.After(*filter.EndTime) {
			continue
		}

		results = append(results, entry)
	}

	// Apply offset
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	} else if filter.Offset >= len(results) {
		return []HistoryEntry{}, nil
	}

	// Apply limit
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}

	return results, nil
}

// GetActionHistory retrieves all history for a specific action.
func (s *InMemoryHistoryStore) GetActionHistory(ctx context.Context, actionID string) ([]HistoryEntry, error) {
	var results []HistoryEntry

	for _, entry := range s.entries {
		if entry.ActionID == actionID {
			results = append(results, entry)
		}
	}

	return results, nil
}

// DeleteHistory deletes history entries older than the specified duration.
func (s *InMemoryHistoryStore) DeleteHistory(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	deleted := 0

	var remaining []HistoryEntry
	for _, entry := range s.entries {
		if entry.Timestamp.Before(cutoff) {
			deleted++
		} else {
			remaining = append(remaining, entry)
		}
	}

	s.entries = remaining
	return deleted, nil
}

// GetHistoryCount returns the count of history entries matching the filter.
func (s *InMemoryHistoryStore) GetHistoryCount(ctx context.Context, filter HistoryFilter) (int, error) {
	entries, err := s.GetHistory(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Clear clears all history entries (for testing).
func (s *InMemoryHistoryStore) Clear() {
	s.entries = make([]HistoryEntry, 0)
}

// AuditLog represents a formatted audit log entry for compliance.
type AuditLog struct {
	// Timestamp is when the operation was performed.
	Timestamp time.Time `json:"timestamp"`

	// UserID is who performed the operation.
	UserID string `json:"user_id"`

	// TenantID is the tenant context.
	TenantID string `json:"tenant_id"`

	// SessionID is the session context.
	SessionID string `json:"session_id"`

	// Action is a human-readable action description.
	Action string `json:"action"`

	// Operation is the operation type.
	Operation Operation `json:"operation"`

	// TargetType is the type of object affected.
	TargetType string `json:"target_type"`

	// TargetID is the ID of the object affected.
	TargetID string `json:"target_id"`

	// Details contains additional operation details.
	Details map[string]interface{} `json:"details,omitempty"`

	// Result indicates success or failure.
	Result string `json:"result"`
}

// ToAuditLog converts a history entry to an audit log format.
func (e *HistoryEntry) ToAuditLog() AuditLog {
	var targetType, targetID string

	// Extract target from action data based on action type
	switch e.ActionType {
	case ActionTypeAccept, ActionTypeReject, ActionTypeDefer:
		if contentID, ok := e.ActionData["content_id"].(string); ok {
			targetID = contentID
		}
		if contentType, ok := e.ActionData["content_type"].(string); ok {
			targetType = contentType
		}
	case ActionTypeTag:
		if contentID, ok := e.ActionData["content_id"].(string); ok {
			targetID = contentID
		}
		targetType = "tags"
	case ActionTypeMerge:
		if targetIDVal, ok := e.ActionData["target_id"].(string); ok {
			targetID = targetIDVal
		}
		targetType = "merged_item"
	case ActionTypeSplit:
		if sourceID, ok := e.ActionData["source_id"].(string); ok {
			targetID = sourceID
		}
		targetType = "split_item"
	case ActionTypeBatch:
		targetType = "batch"
		if count, ok := e.ActionData["action_count"].(float64); ok {
			targetID = fmt.Sprintf("%d actions", int(count))
		}
	}

	return AuditLog{
		Timestamp:  e.Timestamp,
		UserID:     e.UserID,
		TenantID:   e.TenantID,
		SessionID:  e.SessionID,
		Action:     e.Description,
		Operation:  e.Operation,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    e.ActionData,
		Result:     "success",
	}
}

// GenerateAuditReport generates an audit report for a time period.
func GenerateAuditReport(ctx context.Context, store HistoryStore, filter HistoryFilter) ([]AuditLog, error) {
	entries, err := store.GetHistory(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("get history for audit: %w", err)
	}

	logs := make([]AuditLog, len(entries))
	for i, entry := range entries {
		logs[i] = entry.ToAuditLog()
	}

	return logs, nil
}
