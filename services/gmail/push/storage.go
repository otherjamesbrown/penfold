// Package push provides Gmail push notification handling via Cloud Pub/Sub.
package push

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SubscriptionStore defines the interface for subscription persistence.
type SubscriptionStore interface {
	// SaveSubscription persists a subscription.
	SaveSubscription(ctx context.Context, subscription *Subscription) error

	// GetSubscriptionByID retrieves a subscription by ID.
	GetSubscriptionByID(ctx context.Context, id string) (*Subscription, error)

	// GetSubscriptionByEmail retrieves a subscription by email address.
	GetSubscriptionByEmail(ctx context.Context, email string) (*Subscription, error)

	// GetSubscriptionByTenant retrieves a subscription by tenant ID.
	GetSubscriptionByTenant(ctx context.Context, tenantID string) (*Subscription, error)

	// ListSubscriptionsByTenant lists all subscriptions for a tenant.
	ListSubscriptionsByTenant(ctx context.Context, tenantID string) ([]*Subscription, error)

	// ListExpiringSubscriptions lists subscriptions expiring within the given duration.
	ListExpiringSubscriptions(ctx context.Context, within time.Duration) ([]*Subscription, error)

	// ListAllActiveSubscriptions lists all active subscriptions.
	ListAllActiveSubscriptions(ctx context.Context) ([]*Subscription, error)

	// DeleteSubscription deletes a subscription by ID.
	DeleteSubscription(ctx context.Context, id string) error

	// UpdateLastNotification updates the last notification timestamp.
	UpdateLastNotification(ctx context.Context, id string, timestamp time.Time) error
}

// Errors for storage operations.
var (
	ErrSubscriptionNotFound = fmt.Errorf("push: subscription not found")
)

// PostgresSubscriptionStore implements SubscriptionStore using PostgreSQL.
type PostgresSubscriptionStore struct {
	pool      *pgxpool.Pool
	tableName string
}

// PostgresSubscriptionStoreConfig holds configuration for PostgresSubscriptionStore.
type PostgresSubscriptionStoreConfig struct {
	// Pool is the PostgreSQL connection pool.
	Pool *pgxpool.Pool

	// TableName is the table name for storing subscriptions.
	// Defaults to "gmail_push_subscriptions".
	TableName string
}

// NewPostgresSubscriptionStore creates a new PostgresSubscriptionStore.
func NewPostgresSubscriptionStore(cfg *PostgresSubscriptionStoreConfig) (*PostgresSubscriptionStore, error) {
	if cfg.Pool == nil {
		return nil, fmt.Errorf("database pool is required")
	}

	tableName := cfg.TableName
	if tableName == "" {
		tableName = "gmail_push_subscriptions"
	}

	return &PostgresSubscriptionStore{
		pool:      cfg.Pool,
		tableName: tableName,
	}, nil
}

// EnsureTable creates the subscriptions table if it doesn't exist.
func (s *PostgresSubscriptionStore) EnsureTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			tenant_id VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			topic_name VARCHAR(500) NOT NULL,
			history_id BIGINT NOT NULL DEFAULT 0,
			expires_at TIMESTAMPTZ NOT NULL,
			labels TEXT[],
			status VARCHAR(50) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_notification_at TIMESTAMPTZ
		);

		CREATE INDEX IF NOT EXISTS idx_%s_tenant_id ON %s (tenant_id);
		CREATE INDEX IF NOT EXISTS idx_%s_email ON %s (email);
		CREATE INDEX IF NOT EXISTS idx_%s_status ON %s (status);
		CREATE INDEX IF NOT EXISTS idx_%s_expires_at ON %s (expires_at);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_%s_tenant_email ON %s (tenant_id, email);
	`, s.tableName,
		s.tableName, s.tableName,
		s.tableName, s.tableName,
		s.tableName, s.tableName,
		s.tableName, s.tableName,
		s.tableName, s.tableName)

	_, err := s.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("creating subscriptions table: %w", err)
	}

	return nil
}

// SaveSubscription persists a subscription.
func (s *PostgresSubscriptionStore) SaveSubscription(ctx context.Context, subscription *Subscription) error {
	subscription.UpdatedAt = time.Now()

	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, tenant_id, email, topic_name, history_id, expires_at,
			labels, status, created_at, updated_at, last_notification_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			tenant_id = EXCLUDED.tenant_id,
			email = EXCLUDED.email,
			topic_name = EXCLUDED.topic_name,
			history_id = EXCLUDED.history_id,
			expires_at = EXCLUDED.expires_at,
			labels = EXCLUDED.labels,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at,
			last_notification_at = EXCLUDED.last_notification_at
	`, s.tableName)

	_, err := s.pool.Exec(ctx, query,
		subscription.ID,
		subscription.TenantID,
		subscription.Email,
		subscription.TopicName,
		subscription.HistoryID,
		subscription.ExpiresAt,
		subscription.Labels,
		string(subscription.Status),
		subscription.CreatedAt,
		subscription.UpdatedAt,
		nullTime(subscription.LastNotificationAt),
	)
	if err != nil {
		return fmt.Errorf("saving subscription: %w", err)
	}

	return nil
}

// GetSubscriptionByID retrieves a subscription by ID.
func (s *PostgresSubscriptionStore) GetSubscriptionByID(ctx context.Context, id string) (*Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, tenant_id, email, topic_name, history_id, expires_at,
			   labels, status, created_at, updated_at, last_notification_at
		FROM %s
		WHERE id = $1
	`, s.tableName)

	return s.scanSubscription(ctx, query, id)
}

// GetSubscriptionByEmail retrieves a subscription by email address.
func (s *PostgresSubscriptionStore) GetSubscriptionByEmail(ctx context.Context, email string) (*Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, tenant_id, email, topic_name, history_id, expires_at,
			   labels, status, created_at, updated_at, last_notification_at
		FROM %s
		WHERE email = $1 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`, s.tableName)

	return s.scanSubscription(ctx, query, email)
}

// GetSubscriptionByTenant retrieves a subscription by tenant ID.
func (s *PostgresSubscriptionStore) GetSubscriptionByTenant(ctx context.Context, tenantID string) (*Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, tenant_id, email, topic_name, history_id, expires_at,
			   labels, status, created_at, updated_at, last_notification_at
		FROM %s
		WHERE tenant_id = $1 AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`, s.tableName)

	return s.scanSubscription(ctx, query, tenantID)
}

// scanSubscription scans a single subscription row.
func (s *PostgresSubscriptionStore) scanSubscription(ctx context.Context, query string, arg interface{}) (*Subscription, error) {
	var sub Subscription
	var status string
	var lastNotificationAt *time.Time

	err := s.pool.QueryRow(ctx, query, arg).Scan(
		&sub.ID,
		&sub.TenantID,
		&sub.Email,
		&sub.TopicName,
		&sub.HistoryID,
		&sub.ExpiresAt,
		&sub.Labels,
		&status,
		&sub.CreatedAt,
		&sub.UpdatedAt,
		&lastNotificationAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("querying subscription: %w", err)
	}

	sub.Status = SubscriptionStatus(status)
	if lastNotificationAt != nil {
		sub.LastNotificationAt = *lastNotificationAt
	}

	return &sub, nil
}

// ListSubscriptionsByTenant lists all subscriptions for a tenant.
func (s *PostgresSubscriptionStore) ListSubscriptionsByTenant(ctx context.Context, tenantID string) ([]*Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, tenant_id, email, topic_name, history_id, expires_at,
			   labels, status, created_at, updated_at, last_notification_at
		FROM %s
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, s.tableName)

	return s.scanSubscriptions(ctx, query, tenantID)
}

// ListExpiringSubscriptions lists subscriptions expiring within the given duration.
func (s *PostgresSubscriptionStore) ListExpiringSubscriptions(ctx context.Context, within time.Duration) ([]*Subscription, error) {
	expiresBy := time.Now().Add(within)

	query := fmt.Sprintf(`
		SELECT id, tenant_id, email, topic_name, history_id, expires_at,
			   labels, status, created_at, updated_at, last_notification_at
		FROM %s
		WHERE status = 'active' AND expires_at <= $1
		ORDER BY expires_at ASC
	`, s.tableName)

	return s.scanSubscriptions(ctx, query, expiresBy)
}

// ListAllActiveSubscriptions lists all active subscriptions.
func (s *PostgresSubscriptionStore) ListAllActiveSubscriptions(ctx context.Context) ([]*Subscription, error) {
	query := fmt.Sprintf(`
		SELECT id, tenant_id, email, topic_name, history_id, expires_at,
			   labels, status, created_at, updated_at, last_notification_at
		FROM %s
		WHERE status = 'active'
		ORDER BY created_at DESC
	`, s.tableName)

	return s.scanSubscriptions(ctx, query)
}

// scanSubscriptions scans multiple subscription rows.
func (s *PostgresSubscriptionStore) scanSubscriptions(ctx context.Context, query string, args ...interface{}) ([]*Subscription, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}
	defer rows.Close()

	var subscriptions []*Subscription
	for rows.Next() {
		var sub Subscription
		var status string
		var lastNotificationAt *time.Time

		err := rows.Scan(
			&sub.ID,
			&sub.TenantID,
			&sub.Email,
			&sub.TopicName,
			&sub.HistoryID,
			&sub.ExpiresAt,
			&sub.Labels,
			&status,
			&sub.CreatedAt,
			&sub.UpdatedAt,
			&lastNotificationAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning subscription row: %w", err)
		}

		sub.Status = SubscriptionStatus(status)
		if lastNotificationAt != nil {
			sub.LastNotificationAt = *lastNotificationAt
		}

		subscriptions = append(subscriptions, &sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subscription rows: %w", err)
	}

	return subscriptions, nil
}

// DeleteSubscription deletes a subscription by ID.
func (s *PostgresSubscriptionStore) DeleteSubscription(ctx context.Context, id string) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, s.tableName)

	result, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting subscription: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// UpdateLastNotification updates the last notification timestamp.
func (s *PostgresSubscriptionStore) UpdateLastNotification(ctx context.Context, id string, timestamp time.Time) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET last_notification_at = $2, updated_at = NOW()
		WHERE id = $1
	`, s.tableName)

	result, err := s.pool.Exec(ctx, query, id, timestamp)
	if err != nil {
		return fmt.Errorf("updating last notification: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// MemorySubscriptionStore implements SubscriptionStore using in-memory storage.
// Useful for testing.
type MemorySubscriptionStore struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	byEmail       map[string]string // email -> id
	byTenant      map[string]string // tenantID -> id
}

// NewMemorySubscriptionStore creates a new MemorySubscriptionStore.
func NewMemorySubscriptionStore() *MemorySubscriptionStore {
	return &MemorySubscriptionStore{
		subscriptions: make(map[string]*Subscription),
		byEmail:       make(map[string]string),
		byTenant:      make(map[string]string),
	}
}

// SaveSubscription persists a subscription.
func (s *MemorySubscriptionStore) SaveSubscription(ctx context.Context, subscription *Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscription.UpdatedAt = time.Now()

	// Make a copy to avoid mutation.
	subCopy := *subscription
	subCopy.Labels = append([]string{}, subscription.Labels...)

	s.subscriptions[subscription.ID] = &subCopy
	s.byEmail[subscription.Email] = subscription.ID
	s.byTenant[subscription.TenantID] = subscription.ID

	return nil
}

// GetSubscriptionByID retrieves a subscription by ID.
func (s *MemorySubscriptionStore) GetSubscriptionByID(ctx context.Context, id string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sub, ok := s.subscriptions[id]
	if !ok {
		return nil, ErrSubscriptionNotFound
	}

	// Return a copy.
	subCopy := *sub
	return &subCopy, nil
}

// GetSubscriptionByEmail retrieves a subscription by email address.
func (s *MemorySubscriptionStore) GetSubscriptionByEmail(ctx context.Context, email string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byEmail[email]
	if !ok {
		return nil, ErrSubscriptionNotFound
	}

	sub, ok := s.subscriptions[id]
	if !ok || sub.Status != SubscriptionStatusActive {
		return nil, ErrSubscriptionNotFound
	}

	subCopy := *sub
	return &subCopy, nil
}

// GetSubscriptionByTenant retrieves a subscription by tenant ID.
func (s *MemorySubscriptionStore) GetSubscriptionByTenant(ctx context.Context, tenantID string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.byTenant[tenantID]
	if !ok {
		return nil, ErrSubscriptionNotFound
	}

	sub, ok := s.subscriptions[id]
	if !ok || sub.Status != SubscriptionStatusActive {
		return nil, ErrSubscriptionNotFound
	}

	subCopy := *sub
	return &subCopy, nil
}

// ListSubscriptionsByTenant lists all subscriptions for a tenant.
func (s *MemorySubscriptionStore) ListSubscriptionsByTenant(ctx context.Context, tenantID string) ([]*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var subs []*Subscription
	for _, sub := range s.subscriptions {
		if sub.TenantID == tenantID {
			subCopy := *sub
			subs = append(subs, &subCopy)
		}
	}

	return subs, nil
}

// ListExpiringSubscriptions lists subscriptions expiring within the given duration.
func (s *MemorySubscriptionStore) ListExpiringSubscriptions(ctx context.Context, within time.Duration) ([]*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	expiresBy := time.Now().Add(within)
	var subs []*Subscription

	for _, sub := range s.subscriptions {
		if sub.Status == SubscriptionStatusActive && sub.ExpiresAt.Before(expiresBy) {
			subCopy := *sub
			subs = append(subs, &subCopy)
		}
	}

	return subs, nil
}

// ListAllActiveSubscriptions lists all active subscriptions.
func (s *MemorySubscriptionStore) ListAllActiveSubscriptions(ctx context.Context) ([]*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var subs []*Subscription
	for _, sub := range s.subscriptions {
		if sub.Status == SubscriptionStatusActive {
			subCopy := *sub
			subs = append(subs, &subCopy)
		}
	}

	return subs, nil
}

// DeleteSubscription deletes a subscription by ID.
func (s *MemorySubscriptionStore) DeleteSubscription(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.subscriptions[id]
	if !ok {
		return ErrSubscriptionNotFound
	}

	delete(s.subscriptions, id)
	delete(s.byEmail, sub.Email)
	delete(s.byTenant, sub.TenantID)

	return nil
}

// UpdateLastNotification updates the last notification timestamp.
func (s *MemorySubscriptionStore) UpdateLastNotification(ctx context.Context, id string, timestamp time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.subscriptions[id]
	if !ok {
		return ErrSubscriptionNotFound
	}

	sub.LastNotificationAt = timestamp
	sub.UpdatedAt = time.Now()

	return nil
}

// Ensure interfaces are implemented.
var _ SubscriptionStore = (*PostgresSubscriptionStore)(nil)
var _ SubscriptionStore = (*MemorySubscriptionStore)(nil)

// nullTime returns nil for zero times.
func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// SubscriptionJSON returns a subscription as JSON for logging/debugging.
func SubscriptionJSON(sub *Subscription) string {
	b, _ := json.Marshal(sub)
	return string(b)
}
