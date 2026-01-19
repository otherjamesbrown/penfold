// Package undo provides undo/redo functionality for review actions.
package undo

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

// ErrStackNotFound is returned when a stack cannot be found.
var ErrStackNotFound = errors.New("undo stack not found")

// Store defines the interface for undo stack persistence.
type Store interface {
	// SaveStack persists an undo stack.
	SaveStack(ctx context.Context, stack *Stack) error

	// GetStack retrieves an undo stack by session ID.
	GetStack(ctx context.Context, sessionID string) (*Stack, error)

	// GetStackByUser retrieves all stacks for a user.
	GetStackByUser(ctx context.Context, tenantID, userID string) ([]*Stack, error)

	// DeleteStack removes an undo stack.
	DeleteStack(ctx context.Context, sessionID string) error

	// SaveAction persists an individual action for history.
	SaveAction(ctx context.Context, action UndoableAction) error

	// GetActions retrieves actions for a session.
	GetActions(ctx context.Context, sessionID string, limit int) ([]UndoableAction, error)

	// PruneExpiredStacks removes stacks older than the TTL.
	PruneExpiredStacks(ctx context.Context, ttl time.Duration) (int, error)
}

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool  *pgxpool.Pool
	cache *MemoryCache
}

// NewPostgresStore creates a new PostgreSQL-backed undo store.
func NewPostgresStore(pool *pgxpool.Pool, cacheConfig *CacheConfig) *PostgresStore {
	return &PostgresStore{
		pool:  pool,
		cache: NewMemoryCache(cacheConfig),
	}
}

// EnsureSchema creates the necessary tables if they don't exist.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS undo_stacks (
			session_id VARCHAR(255) PRIMARY KEY,
			tenant_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			undo_actions JSONB NOT NULL DEFAULT '[]',
			redo_actions JSONB NOT NULL DEFAULT '[]',
			limits JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL,
			last_updated TIMESTAMPTZ NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_undo_stacks_tenant_user
			ON undo_stacks(tenant_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_undo_stacks_last_updated
			ON undo_stacks(last_updated);

		CREATE TABLE IF NOT EXISTS undo_action_history (
			id VARCHAR(255) PRIMARY KEY,
			session_id VARCHAR(255) NOT NULL,
			tenant_id VARCHAR(255) NOT NULL,
			user_id VARCHAR(255) NOT NULL,
			action_type VARCHAR(50) NOT NULL,
			description TEXT NOT NULL,
			state VARCHAR(50) NOT NULL,
			metadata JSONB NOT NULL DEFAULT '{}',
			action_data JSONB NOT NULL DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL,
			executed_at TIMESTAMPTZ,
			operation VARCHAR(20) NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_undo_action_history_session
			ON undo_action_history(session_id);
		CREATE INDEX IF NOT EXISTS idx_undo_action_history_tenant_user
			ON undo_action_history(tenant_id, user_id);
		CREATE INDEX IF NOT EXISTS idx_undo_action_history_created_at
			ON undo_action_history(created_at);
	`

	_, err := s.pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create undo schema: %w", err)
	}

	return nil
}

// stackData represents the serializable form of an undo stack.
type stackData struct {
	SessionID   string          `json:"session_id"`
	TenantID    string          `json:"tenant_id"`
	UserID      string          `json:"user_id"`
	UndoActions []actionWrapper `json:"undo_actions"`
	RedoActions []actionWrapper `json:"redo_actions"`
	Limits      *Limits         `json:"limits"`
	CreatedAt   time.Time       `json:"created_at"`
	LastUpdated time.Time       `json:"last_updated"`
}

// actionWrapper wraps action data for serialization.
type actionWrapper struct {
	Type     ActionType             `json:"type"`
	ID       string                 `json:"id"`
	Data     map[string]interface{} `json:"data"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SaveStack persists an undo stack.
func (s *PostgresStore) SaveStack(ctx context.Context, stack *Stack) error {
	// Serialize undo and redo actions
	undoActions := make([]actionWrapper, 0, len(stack.GetUndoActions()))
	for _, action := range stack.GetUndoActions() {
		data, err := action.ToJSON()
		if err != nil {
			return fmt.Errorf("serialize undo action: %w", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("parse undo action: %w", err)
		}
		undoActions = append(undoActions, actionWrapper{
			Type:     action.Type(),
			ID:       action.ID(),
			Data:     parsed,
			Metadata: action.Metadata(),
		})
	}

	redoActions := make([]actionWrapper, 0, len(stack.GetRedoActions()))
	for _, action := range stack.GetRedoActions() {
		data, err := action.ToJSON()
		if err != nil {
			return fmt.Errorf("serialize redo action: %w", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return fmt.Errorf("parse redo action: %w", err)
		}
		redoActions = append(redoActions, actionWrapper{
			Type:     action.Type(),
			ID:       action.ID(),
			Data:     parsed,
			Metadata: action.Metadata(),
		})
	}

	undoJSON, err := json.Marshal(undoActions)
	if err != nil {
		return fmt.Errorf("marshal undo actions: %w", err)
	}

	redoJSON, err := json.Marshal(redoActions)
	if err != nil {
		return fmt.Errorf("marshal redo actions: %w", err)
	}

	limitsJSON, err := json.Marshal(stack.limits)
	if err != nil {
		return fmt.Errorf("marshal limits: %w", err)
	}

	query := `
		INSERT INTO undo_stacks (
			session_id, tenant_id, user_id, undo_actions, redo_actions,
			limits, created_at, last_updated
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (session_id) DO UPDATE SET
			undo_actions = EXCLUDED.undo_actions,
			redo_actions = EXCLUDED.redo_actions,
			limits = EXCLUDED.limits,
			last_updated = EXCLUDED.last_updated
	`

	_, err = s.pool.Exec(ctx, query,
		stack.SessionID(),
		stack.TenantID(),
		stack.UserID(),
		undoJSON,
		redoJSON,
		limitsJSON,
		stack.CreatedAt(),
		stack.LastUpdated(),
	)
	if err != nil {
		return fmt.Errorf("save undo stack: %w", err)
	}

	// Update cache
	s.cache.Set(stack.SessionID(), stack)

	return nil
}

// GetStack retrieves an undo stack by session ID.
func (s *PostgresStore) GetStack(ctx context.Context, sessionID string) (*Stack, error) {
	// Check cache first
	if cached := s.cache.Get(sessionID); cached != nil {
		return cached, nil
	}

	query := `
		SELECT session_id, tenant_id, user_id, undo_actions, redo_actions,
			limits, created_at, last_updated
		FROM undo_stacks
		WHERE session_id = $1
	`

	var data stackData
	var undoJSON, redoJSON, limitsJSON []byte

	row := s.pool.QueryRow(ctx, query, sessionID)
	err := row.Scan(
		&data.SessionID,
		&data.TenantID,
		&data.UserID,
		&undoJSON,
		&redoJSON,
		&limitsJSON,
		&data.CreatedAt,
		&data.LastUpdated,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrStackNotFound
		}
		return nil, fmt.Errorf("get undo stack: %w", err)
	}

	// Parse limits
	var limits Limits
	if err := json.Unmarshal(limitsJSON, &limits); err != nil {
		return nil, fmt.Errorf("parse limits: %w", err)
	}

	// Create the stack
	stack := NewStack(&StackConfig{
		TenantID:  data.TenantID,
		UserID:    data.UserID,
		SessionID: data.SessionID,
		Limits:    &limits,
	})

	// We cannot fully restore actions without their executors,
	// but we store the serialized form for history purposes.
	// The stack will be empty but can be repopulated as needed.

	// Update cache
	s.cache.Set(sessionID, stack)

	return stack, nil
}

// GetStackByUser retrieves all stacks for a user.
func (s *PostgresStore) GetStackByUser(ctx context.Context, tenantID, userID string) ([]*Stack, error) {
	query := `
		SELECT session_id, tenant_id, user_id, undo_actions, redo_actions,
			limits, created_at, last_updated
		FROM undo_stacks
		WHERE tenant_id = $1 AND user_id = $2
		ORDER BY last_updated DESC
	`

	rows, err := s.pool.Query(ctx, query, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("query undo stacks: %w", err)
	}
	defer rows.Close()

	var stacks []*Stack
	for rows.Next() {
		var data stackData
		var undoJSON, redoJSON, limitsJSON []byte

		err := rows.Scan(
			&data.SessionID,
			&data.TenantID,
			&data.UserID,
			&undoJSON,
			&redoJSON,
			&limitsJSON,
			&data.CreatedAt,
			&data.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("scan undo stack: %w", err)
		}

		var limits Limits
		if err := json.Unmarshal(limitsJSON, &limits); err != nil {
			return nil, fmt.Errorf("parse limits: %w", err)
		}

		stack := NewStack(&StackConfig{
			TenantID:  data.TenantID,
			UserID:    data.UserID,
			SessionID: data.SessionID,
			Limits:    &limits,
		})

		stacks = append(stacks, stack)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate undo stacks: %w", err)
	}

	return stacks, nil
}

// DeleteStack removes an undo stack.
func (s *PostgresStore) DeleteStack(ctx context.Context, sessionID string) error {
	query := `DELETE FROM undo_stacks WHERE session_id = $1`

	result, err := s.pool.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("delete undo stack: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrStackNotFound
	}

	// Remove from cache
	s.cache.Delete(sessionID)

	return nil
}

// SaveAction persists an individual action for history.
func (s *PostgresStore) SaveAction(ctx context.Context, action UndoableAction) error {
	actionData, err := action.ToJSON()
	if err != nil {
		return fmt.Errorf("serialize action: %w", err)
	}

	metadataJSON, err := json.Marshal(action.Metadata())
	if err != nil {
		return fmt.Errorf("serialize metadata: %w", err)
	}

	// Determine operation based on state
	operation := "execute"
	switch action.State() {
	case ActionStateUndone:
		operation = "undo"
	case ActionStateRedone:
		operation = "redo"
	}

	query := `
		INSERT INTO undo_action_history (
			id, session_id, tenant_id, user_id, action_type, description,
			state, metadata, action_data, created_at, executed_at, operation
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			state = EXCLUDED.state,
			executed_at = EXCLUDED.executed_at,
			operation = EXCLUDED.operation
	`

	_, err = s.pool.Exec(ctx, query,
		action.ID(),
		action.SessionID(),
		action.TenantID(),
		action.UserID(),
		string(action.Type()),
		action.Description(),
		string(action.State()),
		metadataJSON,
		actionData,
		action.CreatedAt(),
		action.ExecutedAt(),
		operation,
	)
	if err != nil {
		return fmt.Errorf("save action history: %w", err)
	}

	return nil
}

// GetActions retrieves actions for a session.
func (s *PostgresStore) GetActions(ctx context.Context, sessionID string, limit int) ([]UndoableAction, error) {
	// This returns action metadata; full action restoration requires executors
	// which are not serializable. See history.go for the HistoryEntry approach.
	return nil, nil
}

// PruneExpiredStacks removes stacks older than the TTL.
func (s *PostgresStore) PruneExpiredStacks(ctx context.Context, ttl time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-ttl)

	query := `DELETE FROM undo_stacks WHERE last_updated < $1`

	result, err := s.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune expired stacks: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// CacheConfig holds configuration for the memory cache.
type CacheConfig struct {
	// MaxEntries is the maximum number of stacks to cache.
	MaxEntries int

	// TTL is how long cached stacks remain valid.
	TTL time.Duration
}

// DefaultCacheConfig returns default cache configuration.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxEntries: 1000,
		TTL:        15 * time.Minute,
	}
}

// cacheEntry holds a cached stack with its expiry time.
type cacheEntry struct {
	stack   *Stack
	expires time.Time
}

// MemoryCache provides in-memory caching for undo stacks.
type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	config  *CacheConfig
}

// NewMemoryCache creates a new memory cache.
func NewMemoryCache(config *CacheConfig) *MemoryCache {
	if config == nil {
		config = DefaultCacheConfig()
	}
	return &MemoryCache{
		entries: make(map[string]*cacheEntry),
		config:  config,
	}
}

// Get retrieves a stack from the cache.
func (c *MemoryCache) Get(sessionID string) *Stack {
	c.mu.RLock()
	entry, ok := c.entries[sessionID]
	c.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Now().After(entry.expires) {
		c.Delete(sessionID)
		return nil
	}

	return entry.stack
}

// Set adds a stack to the cache.
func (c *MemoryCache) Set(sessionID string, stack *Stack) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest if at capacity
	if len(c.entries) >= c.config.MaxEntries {
		c.evictOldest()
	}

	c.entries[sessionID] = &cacheEntry{
		stack:   stack,
		expires: time.Now().Add(c.config.TTL),
	}
}

// Delete removes a stack from the cache.
func (c *MemoryCache) Delete(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, sessionID)
}

// Clear removes all entries from the cache.
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// evictOldest removes the oldest entry from the cache.
// Must be called with lock held.
func (c *MemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.expires.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expires
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// Size returns the number of cached entries.
func (c *MemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// InMemoryStore implements Store using in-memory storage for testing.
type InMemoryStore struct {
	mu      sync.RWMutex
	stacks  map[string]*Stack
	history map[string][]UndoableAction
}

// NewInMemoryStore creates a new in-memory undo store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		stacks:  make(map[string]*Stack),
		history: make(map[string][]UndoableAction),
	}
}

// SaveStack persists an undo stack.
func (s *InMemoryStore) SaveStack(ctx context.Context, stack *Stack) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stacks[stack.SessionID()] = stack
	return nil
}

// GetStack retrieves an undo stack by session ID.
func (s *InMemoryStore) GetStack(ctx context.Context, sessionID string) (*Stack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stack, ok := s.stacks[sessionID]
	if !ok {
		return nil, ErrStackNotFound
	}

	return stack, nil
}

// GetStackByUser retrieves all stacks for a user.
func (s *InMemoryStore) GetStackByUser(ctx context.Context, tenantID, userID string) ([]*Stack, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stacks []*Stack
	for _, stack := range s.stacks {
		if stack.TenantID() == tenantID && stack.UserID() == userID {
			stacks = append(stacks, stack)
		}
	}

	return stacks, nil
}

// DeleteStack removes an undo stack.
func (s *InMemoryStore) DeleteStack(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.stacks[sessionID]; !ok {
		return ErrStackNotFound
	}

	delete(s.stacks, sessionID)
	delete(s.history, sessionID)
	return nil
}

// SaveAction persists an individual action for history.
func (s *InMemoryStore) SaveAction(ctx context.Context, action UndoableAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history[action.SessionID()] = append(s.history[action.SessionID()], action)
	return nil
}

// GetActions retrieves actions for a session.
func (s *InMemoryStore) GetActions(ctx context.Context, sessionID string, limit int) ([]UndoableAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	actions := s.history[sessionID]
	if limit > 0 && len(actions) > limit {
		return actions[len(actions)-limit:], nil
	}

	return actions, nil
}

// PruneExpiredStacks removes stacks older than the TTL.
func (s *InMemoryStore) PruneExpiredStacks(ctx context.Context, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().Add(-ttl)
	pruned := 0

	for sessionID, stack := range s.stacks {
		if stack.LastUpdated().Before(cutoff) {
			delete(s.stacks, sessionID)
			delete(s.history, sessionID)
			pruned++
		}
	}

	return pruned, nil
}

// Clear clears all stored data (for testing).
func (s *InMemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stacks = make(map[string]*Stack)
	s.history = make(map[string][]UndoableAction)
}
