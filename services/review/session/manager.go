// Package session provides the SessionManager for managing review sessions.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// DefaultSessionTimeout is the default session timeout duration.
const DefaultSessionTimeout = 4 * time.Hour

// DefaultExpiryCheckInterval is the default interval for checking expired sessions.
const DefaultExpiryCheckInterval = 1 * time.Minute

// ManagerConfig holds configuration for the SessionManager.
type ManagerConfig struct {
	// SessionTimeout is the duration after which a session expires.
	SessionTimeout time.Duration

	// ExpiryCheckInterval is how often to check for expired sessions.
	ExpiryCheckInterval time.Duration

	// ExpiryBatchSize is the maximum number of expired sessions to process at once.
	ExpiryBatchSize int
}

// DefaultManagerConfig returns a ManagerConfig with sensible defaults.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		SessionTimeout:      DefaultSessionTimeout,
		ExpiryCheckInterval: DefaultExpiryCheckInterval,
		ExpiryBatchSize:     100,
	}
}

// Manager manages review sessions.
type Manager struct {
	store     Store
	publisher EventPublisher
	logger    logging.Logger
	config    *ManagerConfig

	// stopCh signals the expiry goroutine to stop.
	stopCh chan struct{}
	// wg waits for goroutines to finish.
	wg sync.WaitGroup
	// running indicates if the expiry checker is running.
	running bool
	mu      sync.Mutex
}

// NewManager creates a new SessionManager.
func NewManager(store Store, publisher EventPublisher, logger logging.Logger, config *ManagerConfig) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}
	if publisher == nil {
		publisher = &NoOpPublisher{}
	}

	return &Manager{
		store:     store,
		publisher: publisher,
		logger:    logger.With(logging.F("component", "session_manager")),
		config:    config,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the session expiry background task.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("session manager already running")
	}

	m.running = true
	m.wg.Add(1)
	go m.expiryChecker(ctx)

	m.logger.Info("session manager started",
		logging.F("session_timeout", m.config.SessionTimeout),
		logging.F("expiry_check_interval", m.config.ExpiryCheckInterval),
	)

	return nil
}

// Stop stops the session manager and waits for cleanup.
func (m *Manager) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	m.mu.Unlock()

	close(m.stopCh)
	m.wg.Wait()

	m.logger.Info("session manager stopped")
	return nil
}

// StartSession creates a new review session for a user.
func (m *Manager) StartSession(ctx context.Context, tenantID, userID string) (*Session, error) {
	// Check for existing active sessions
	existing, err := m.store.GetByTenantAndUser(ctx, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing sessions: %w", err)
	}

	// If there's an active session for today, return it
	today := time.Now().UTC().Format("2006-01-02")
	for _, session := range existing {
		if session.Date == today && session.State == StateActive {
			m.logger.Debug("returning existing active session",
				logging.F("session_id", session.ID),
				logging.F("tenant_id", tenantID),
				logging.F("user_id", userID),
			)
			return session, nil
		}
	}

	// Create new session
	session := NewSession(tenantID, userID, m.config.SessionTimeout)

	// Save to store
	if err := m.store.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	// Publish event
	event := NewEvent(EventTypeSessionStarted, session.ID, tenantID, userID).
		WithData("date", session.Date).
		WithData("expires_at", session.ExpiresAt)

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish session started event",
			logging.Err(err),
			logging.F("session_id", session.ID),
		)
	}

	// Save event to store
	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save session started event",
			logging.Err(err),
			logging.F("session_id", session.ID),
		)
	}

	m.logger.Info("session started",
		logging.F("session_id", session.ID),
		logging.F("tenant_id", tenantID),
		logging.F("user_id", userID),
		logging.F("expires_at", session.ExpiresAt),
	)

	return session, nil
}

// GetSession retrieves a session by ID.
func (m *Manager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Check if expired
	if session.IsExpired() && (session.State == StateActive || session.State == StatePaused) {
		if err := m.expireSession(ctx, session); err != nil {
			m.logger.Warn("failed to expire session",
				logging.Err(err),
				logging.F("session_id", sessionID),
			)
		}
		return nil, ErrSessionExpired
	}

	return session, nil
}

// PauseSession pauses a session.
func (m *Manager) PauseSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if err := session.Pause(); err != nil {
		return nil, err
	}

	if err := m.store.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save paused session: %w", err)
	}

	// Publish event
	event := NewEvent(EventTypeSessionPaused, session.ID, session.TenantID, session.UserID).
		WithData("pause_count", session.Analytics.PauseCount).
		WithData("items_reviewed", session.Analytics.ItemsReviewed)

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish session paused event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save session paused event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	m.logger.Info("session paused",
		logging.F("session_id", sessionID),
		logging.F("pause_count", session.Analytics.PauseCount),
	)

	return session, nil
}

// ResumeSession resumes a paused session.
func (m *Manager) ResumeSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	pausedTimeMs := int64(0)
	if session.PausedAt != nil {
		pausedTimeMs = time.Now().UTC().Sub(*session.PausedAt).Milliseconds()
	}

	if err := session.Resume(); err != nil {
		return nil, err
	}

	if err := m.store.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save resumed session: %w", err)
	}

	// Publish event
	event := NewEvent(EventTypeSessionResumed, session.ID, session.TenantID, session.UserID).
		WithData("paused_time_ms", pausedTimeMs)

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish session resumed event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save session resumed event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	m.logger.Info("session resumed",
		logging.F("session_id", sessionID),
		logging.F("paused_time_ms", pausedTimeMs),
	)

	return session, nil
}

// CompleteSession completes a session.
func (m *Manager) CompleteSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if err := session.Complete(); err != nil {
		return nil, err
	}

	if err := m.store.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save completed session: %w", err)
	}

	// Publish event
	event := NewEvent(EventTypeSessionCompleted, session.ID, session.TenantID, session.UserID).
		WithData("analytics", session.Analytics).
		WithData("duration_ms", session.Duration().Milliseconds()).
		WithData("completed_at", session.CompletedAt)

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish session completed event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save session completed event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	m.logger.Info("session completed",
		logging.F("session_id", sessionID),
		logging.F("items_reviewed", session.Analytics.ItemsReviewed),
		logging.F("duration_ms", session.Duration().Milliseconds()),
	)

	return session, nil
}

// AddReviewItem adds a reviewed item to a session.
func (m *Manager) AddReviewItem(ctx context.Context, sessionID string, item ReviewItem) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Check expiry
	if session.IsExpired() {
		if err := m.expireSession(ctx, session); err != nil {
			m.logger.Warn("failed to expire session",
				logging.Err(err),
				logging.F("session_id", sessionID),
			)
		}
		return nil, ErrSessionExpired
	}

	// Set review time if not provided
	if item.ReviewedAt.IsZero() {
		item.ReviewedAt = time.Now().UTC()
	}

	if err := session.AddItem(item); err != nil {
		return nil, err
	}

	if err := m.store.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save session with new item: %w", err)
	}

	// Publish item reviewed event
	event := NewEvent(EventTypeItemReviewed, session.ID, session.TenantID, session.UserID).
		WithData("item_id", item.ID).
		WithData("content_id", item.ContentID).
		WithData("content_type", item.ContentType).
		WithData("action", item.Action).
		WithData("time_spent_ms", item.TimeSpentMs)

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish item reviewed event",
			logging.Err(err),
			logging.F("session_id", sessionID),
			logging.F("item_id", item.ID),
		)
	}

	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save item reviewed event",
			logging.Err(err),
			logging.F("session_id", sessionID),
			logging.F("item_id", item.ID),
		)
	}

	m.logger.Debug("item reviewed",
		logging.F("session_id", sessionID),
		logging.F("item_id", item.ID),
		logging.F("action", item.Action),
	)

	return session, nil
}

// ExtendSession extends the session expiry time.
func (m *Manager) ExtendSession(ctx context.Context, sessionID string, duration time.Duration) (*Session, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	oldExpiresAt := session.ExpiresAt

	if err := session.ExtendExpiry(duration); err != nil {
		return nil, err
	}

	if err := m.store.Save(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to save extended session: %w", err)
	}

	// Publish event
	event := NewEvent(EventTypeSessionExtended, session.ID, session.TenantID, session.UserID).
		WithData("old_expires_at", oldExpiresAt).
		WithData("new_expires_at", session.ExpiresAt).
		WithData("extension_ms", duration.Milliseconds())

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish session extended event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save session extended event",
			logging.Err(err),
			logging.F("session_id", sessionID),
		)
	}

	m.logger.Info("session extended",
		logging.F("session_id", sessionID),
		logging.F("old_expires_at", oldExpiresAt),
		logging.F("new_expires_at", session.ExpiresAt),
	)

	return session, nil
}

// GetSessionAnalytics returns analytics for a session.
func (m *Manager) GetSessionAnalytics(ctx context.Context, sessionID string) (*Analytics, error) {
	session, err := m.store.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	analytics := session.Analytics
	return &analytics, nil
}

// GetSessionEvents returns the event audit trail for a session.
func (m *Manager) GetSessionEvents(ctx context.Context, sessionID string) ([]*Event, error) {
	return m.store.GetEvents(ctx, sessionID)
}

// expireSession marks a session as expired and publishes an event.
func (m *Manager) expireSession(ctx context.Context, session *Session) error {
	if err := session.Expire(); err != nil {
		return err
	}

	if err := m.store.Save(ctx, session); err != nil {
		return fmt.Errorf("failed to save expired session: %w", err)
	}

	// Publish event
	event := NewEvent(EventTypeSessionExpired, session.ID, session.TenantID, session.UserID).
		WithData("expires_at", session.ExpiresAt).
		WithData("items_reviewed", session.Analytics.ItemsReviewed)

	if err := m.publisher.Publish(event); err != nil {
		m.logger.Warn("failed to publish session expired event",
			logging.Err(err),
			logging.F("session_id", session.ID),
		)
	}

	if err := m.store.SaveEvent(ctx, event); err != nil {
		m.logger.Warn("failed to save session expired event",
			logging.Err(err),
			logging.F("session_id", session.ID),
		)
	}

	m.logger.Info("session expired",
		logging.F("session_id", session.ID),
		logging.F("items_reviewed", session.Analytics.ItemsReviewed),
	)

	return nil
}

// expiryChecker periodically checks for and expires stale sessions.
func (m *Manager) expiryChecker(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.ExpiryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("expiry checker stopping due to context cancellation")
			return
		case <-m.stopCh:
			m.logger.Info("expiry checker stopping due to stop signal")
			return
		case <-ticker.C:
			m.processExpiredSessions(ctx)
		}
	}
}

// processExpiredSessions finds and expires stale sessions.
func (m *Manager) processExpiredSessions(ctx context.Context) {
	expired, err := m.store.GetExpired(ctx, m.config.ExpiryBatchSize)
	if err != nil {
		m.logger.Error("failed to get expired sessions",
			logging.Err(err),
		)
		return
	}

	if len(expired) == 0 {
		return
	}

	m.logger.Debug("processing expired sessions",
		logging.F("count", len(expired)),
	)

	for _, session := range expired {
		if err := m.expireSession(ctx, session); err != nil {
			m.logger.Error("failed to expire session",
				logging.Err(err),
				logging.F("session_id", session.ID),
			)
		}
	}
}

// DeleteSession deletes a session (admin function).
func (m *Manager) DeleteSession(ctx context.Context, sessionID string) error {
	return m.store.Delete(ctx, sessionID)
}

// GetActiveSessions returns all active sessions for a tenant and user.
func (m *Manager) GetActiveSessions(ctx context.Context, tenantID, userID string) ([]*Session, error) {
	return m.store.GetByTenantAndUser(ctx, tenantID, userID)
}
