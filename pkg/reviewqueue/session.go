package reviewqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	pkgconfig "github.com/otherjamesbrown/penfold/pkg/config"
)

// SessionStatus defines the state of a review session.
type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusPaused    SessionStatus = "paused"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusAbandoned SessionStatus = "abandoned"
	// Alias for compatibility
	SessionStatusEnded SessionStatus = "completed"
)

// Session represents a user's review session.
type Session struct {
	ID               int64         `json:"id"`
	SessionUUID      string        `json:"session_uuid"`
	TenantID         string        `json:"tenant_id"`
	UserEmail        string        `json:"user_email"`
	Status           SessionStatus `json:"status"`
	StartedAt        time.Time     `json:"started_at"`
	LastActivityAt   time.Time     `json:"last_activity_at"`
	CompletedAt      *time.Time    `json:"completed_at,omitempty"`
	ExpiresAt        time.Time     `json:"expires_at"`
	CurrentPosition  int           `json:"current_position"`
	TotalItems       int           `json:"total_items"`
	TotalReviewed    int           `json:"total_reviewed"`    // = items_reviewed
	ApprovedCount    int           `json:"approved_count"`    // = items_accepted
	RejectedCount    int           `json:"rejected_count"`    // = items_rejected
	ModifiedCount    int           `json:"modified_count"`    // = items_modified
	DeferredCount    int           `json:"deferred_count"`    // = items_skipped
	ReviewMode       string        `json:"review_mode"`
	PriorityMode     string        `json:"priority_mode"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`

	// Computed field for backward compatibility
	PausedAt              *time.Time `json:"paused_at,omitempty"`
	EndedAt               *time.Time `json:"ended_at,omitempty"`
	ActiveDurationSeconds int64      `json:"active_duration_seconds"`
}

// StartSession starts a new review session.
// If there's an existing active or paused session, it ends that session first.
func (r *Repository) StartSession(ctx context.Context) (*Session, bool, error) {
	// Check for existing active/paused session
	existingSession, err := r.GetCurrentSession(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("checking for existing session: %w", err)
	}

	previousEnded := false
	if existingSession != nil {
		// End the existing session
		_, err := r.EndSession(ctx, existingSession.ID)
		if err != nil {
			return nil, false, fmt.Errorf("ending existing session: %w", err)
		}
		previousEnded = true
	}

	// Generate a session UUID
	sessionUUID := uuid.New().String()

	// Create new session
	var session Session
	err = r.db.QueryRow(ctx, `
		INSERT INTO review_sessions (
			session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, expires_at, total_items,
			review_mode, priority_mode
		)
		-- Safety/test tenant UUID: prevents CLI review sessions from touching production data.
		-- Do NOT replace with the production tenant UUID.
		VALUES ($1, $2, 'cli-user@penfold.local',
			'active', NOW(), NOW(), NOW() + INTERVAL '24 hours', 0,
			'standard', 'mixed')
		RETURNING id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
	`, sessionUUID, pkgconfig.SafetyTenantID().String()).Scan(
		&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
		&session.Status, &session.StartedAt, &session.LastActivityAt,
		&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
		&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
		&session.ModifiedCount, &session.DeferredCount,
		&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, previousEnded, fmt.Errorf("creating session: %w", err)
	}

	// Set computed fields
	session.EndedAt = session.CompletedAt
	session.ActiveDurationSeconds = int64(time.Since(session.StartedAt).Seconds())

	return &session, previousEnded, nil
}

// PauseSession pauses the specified session or the current active session.
func (r *Repository) PauseSession(ctx context.Context, sessionID int64) (*Session, error) {
	var id int64
	if sessionID > 0 {
		id = sessionID
	} else {
		// Find current active session
		current, err := r.GetCurrentSession(ctx)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, fmt.Errorf("no active session to pause")
		}
		if current.Status != SessionStatusActive {
			return nil, fmt.Errorf("session is not active (status: %s)", current.Status)
		}
		id = current.ID
	}

	var session Session
	err := r.db.QueryRow(ctx, `
		UPDATE review_sessions
		SET status = 'paused',
			last_activity_at = NOW()
		WHERE id = $1 AND status = 'active'
		RETURNING id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
	`, id).Scan(
		&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
		&session.Status, &session.StartedAt, &session.LastActivityAt,
		&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
		&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
		&session.ModifiedCount, &session.DeferredCount,
		&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("session not found or not active")
	}
	if err != nil {
		return nil, fmt.Errorf("pausing session: %w", err)
	}

	// Set computed fields
	pausedAt := session.LastActivityAt
	session.PausedAt = &pausedAt
	session.EndedAt = session.CompletedAt
	session.ActiveDurationSeconds = int64(session.LastActivityAt.Sub(session.StartedAt).Seconds())

	return &session, nil
}

// ResumeSession resumes a paused session.
func (r *Repository) ResumeSession(ctx context.Context, sessionID int64) (*Session, error) {
	var id int64
	if sessionID > 0 {
		id = sessionID
	} else {
		// Find current paused session
		var err error
		err = r.db.QueryRow(ctx, `
			SELECT id FROM review_sessions
			WHERE status = 'paused'
			ORDER BY started_at DESC
			LIMIT 1
		`).Scan(&id)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no paused session to resume")
		}
		if err != nil {
			return nil, fmt.Errorf("finding paused session: %w", err)
		}
	}

	var session Session
	err := r.db.QueryRow(ctx, `
		UPDATE review_sessions
		SET status = 'active',
			last_activity_at = NOW(),
			expires_at = NOW() + INTERVAL '24 hours'
		WHERE id = $1 AND status = 'paused'
		RETURNING id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
	`, id).Scan(
		&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
		&session.Status, &session.StartedAt, &session.LastActivityAt,
		&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
		&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
		&session.ModifiedCount, &session.DeferredCount,
		&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("session not found or not paused")
	}
	if err != nil {
		return nil, fmt.Errorf("resuming session: %w", err)
	}

	// Set computed fields
	session.PausedAt = nil
	session.EndedAt = session.CompletedAt
	session.ActiveDurationSeconds = int64(time.Since(session.StartedAt).Seconds())

	return &session, nil
}

// EndSession ends the specified session or the current session.
func (r *Repository) EndSession(ctx context.Context, sessionID int64) (*Session, error) {
	var id int64
	if sessionID > 0 {
		id = sessionID
	} else {
		// Find current active/paused session
		current, err := r.GetCurrentSession(ctx)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, fmt.Errorf("no active session to end")
		}
		id = current.ID
	}

	var session Session
	err := r.db.QueryRow(ctx, `
		UPDATE review_sessions
		SET status = 'completed',
			completed_at = NOW(),
			last_activity_at = NOW()
		WHERE id = $1 AND status IN ('active', 'paused')
		RETURNING id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
	`, id).Scan(
		&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
		&session.Status, &session.StartedAt, &session.LastActivityAt,
		&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
		&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
		&session.ModifiedCount, &session.DeferredCount,
		&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("session not found or already ended")
	}
	if err != nil {
		return nil, fmt.Errorf("ending session: %w", err)
	}

	// Set computed fields
	session.EndedAt = session.CompletedAt
	if session.CompletedAt != nil {
		session.ActiveDurationSeconds = int64(session.CompletedAt.Sub(session.StartedAt).Seconds())
	}

	return &session, nil
}

// GetCurrentSession returns the current active or paused session, if any.
func (r *Repository) GetCurrentSession(ctx context.Context) (*Session, error) {
	var session Session
	err := r.db.QueryRow(ctx, `
		SELECT id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
		FROM review_sessions
		WHERE status IN ('active', 'paused')
		ORDER BY started_at DESC
		LIMIT 1
	`).Scan(
		&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
		&session.Status, &session.StartedAt, &session.LastActivityAt,
		&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
		&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
		&session.ModifiedCount, &session.DeferredCount,
		&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting current session: %w", err)
	}

	// Set computed fields
	if session.Status == SessionStatusPaused {
		pausedAt := session.LastActivityAt
		session.PausedAt = &pausedAt
	}
	session.EndedAt = session.CompletedAt
	session.ActiveDurationSeconds = int64(time.Since(session.StartedAt).Seconds())

	return &session, nil
}

// GetSession retrieves a session by ID.
func (r *Repository) GetSession(ctx context.Context, sessionID int64) (*Session, error) {
	var session Session
	err := r.db.QueryRow(ctx, `
		SELECT id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
		FROM review_sessions
		WHERE id = $1
	`, sessionID).Scan(
		&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
		&session.Status, &session.StartedAt, &session.LastActivityAt,
		&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
		&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
		&session.ModifiedCount, &session.DeferredCount,
		&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}

	// Set computed fields
	if session.Status == SessionStatusPaused {
		pausedAt := session.LastActivityAt
		session.PausedAt = &pausedAt
	}
	session.EndedAt = session.CompletedAt
	if session.CompletedAt != nil {
		session.ActiveDurationSeconds = int64(session.CompletedAt.Sub(session.StartedAt).Seconds())
	} else {
		session.ActiveDurationSeconds = int64(time.Since(session.StartedAt).Seconds())
	}

	return &session, nil
}

// ListSessions returns past review sessions.
func (r *Repository) ListSessions(ctx context.Context, limit, offset int) ([]*Session, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Get total count
	var totalCount int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM review_sessions`).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("counting sessions: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, session_uuid, tenant_id, user_email, status, started_at,
			last_activity_at, completed_at, expires_at, current_position, total_items,
			items_reviewed, items_accepted, items_rejected, items_modified, items_skipped,
			review_mode, priority_mode, created_at, updated_at
		FROM review_sessions
		ORDER BY started_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var session Session
		err := rows.Scan(
			&session.ID, &session.SessionUUID, &session.TenantID, &session.UserEmail,
			&session.Status, &session.StartedAt, &session.LastActivityAt,
			&session.CompletedAt, &session.ExpiresAt, &session.CurrentPosition, &session.TotalItems,
			&session.TotalReviewed, &session.ApprovedCount, &session.RejectedCount,
			&session.ModifiedCount, &session.DeferredCount,
			&session.ReviewMode, &session.PriorityMode, &session.CreatedAt, &session.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning session: %w", err)
		}

		// Set computed fields
		if session.Status == SessionStatusPaused {
			pausedAt := session.LastActivityAt
			session.PausedAt = &pausedAt
		}
		session.EndedAt = session.CompletedAt
		if session.CompletedAt != nil {
			session.ActiveDurationSeconds = int64(session.CompletedAt.Sub(session.StartedAt).Seconds())
		}

		sessions = append(sessions, &session)
	}

	return sessions, totalCount, nil
}

// IncrementSessionStats updates the session statistics after a review action.
func (r *Repository) IncrementSessionStats(ctx context.Context, sessionID int64, approved, rejected, deferred int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE review_sessions
		SET items_reviewed = items_reviewed + $2 + $3 + $4,
			items_accepted = items_accepted + $2,
			items_rejected = items_rejected + $3,
			items_skipped = items_skipped + $4,
			last_activity_at = NOW()
		WHERE id = $1
	`, sessionID, approved, rejected, deferred)
	if err != nil {
		return fmt.Errorf("updating session stats: %w", err)
	}
	return nil
}
