// Package session provides review session management for the Review service.
// It handles session lifecycle, state persistence, and analytics for daily review sessions.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrSessionNotFound is returned when a session cannot be found.
var ErrSessionNotFound = errors.New("session not found")

// ErrSessionExpired is returned when operating on an expired session.
var ErrSessionExpired = errors.New("session expired")

// ErrInvalidStateTransition is returned when an invalid state transition is attempted.
var ErrInvalidStateTransition = errors.New("invalid state transition")

// ErrSessionAlreadyCompleted is returned when trying to modify a completed session.
var ErrSessionAlreadyCompleted = errors.New("session already completed")

// State represents the current state of a review session.
type State string

const (
	// StateActive indicates the session is actively being used.
	StateActive State = "active"

	// StatePaused indicates the session has been paused (e.g., user interruption).
	StatePaused State = "paused"

	// StateCompleted indicates the session has been completed.
	StateCompleted State = "completed"

	// StateExpired indicates the session has expired due to timeout.
	StateExpired State = "expired"
)

// ValidStates returns all valid session states.
func ValidStates() []State {
	return []State{StateActive, StatePaused, StateCompleted, StateExpired}
}

// IsValid returns true if the state is a valid session state.
func (s State) IsValid() bool {
	for _, valid := range ValidStates() {
		if s == valid {
			return true
		}
	}
	return false
}

// CanTransitionTo returns true if a transition from current state to target state is valid.
func (s State) CanTransitionTo(target State) bool {
	switch s {
	case StateActive:
		return target == StatePaused || target == StateCompleted || target == StateExpired
	case StatePaused:
		return target == StateActive || target == StateCompleted || target == StateExpired
	case StateCompleted:
		return false // Cannot transition from completed
	case StateExpired:
		return false // Cannot transition from expired
	default:
		return false
	}
}

// ItemAction represents the action taken on a review item within a session.
type ItemAction string

const (
	// ItemActionApproved indicates the item was approved.
	ItemActionApproved ItemAction = "approved"

	// ItemActionRejected indicates the item was rejected.
	ItemActionRejected ItemAction = "rejected"

	// ItemActionDeferred indicates the item was deferred for later.
	ItemActionDeferred ItemAction = "deferred"

	// ItemActionSkipped indicates the item was skipped without action.
	ItemActionSkipped ItemAction = "skipped"
)

// ReviewItem represents an item reviewed within a session.
type ReviewItem struct {
	// ID is the unique identifier for the review item.
	ID string `json:"id"`

	// ContentID references the underlying content entity.
	ContentID string `json:"content_id"`

	// ContentType is the type of content (e.g., "email", "document").
	ContentType string `json:"content_type"`

	// Action is the action taken on this item.
	Action ItemAction `json:"action"`

	// Feedback is optional user feedback on the item.
	Feedback string `json:"feedback,omitempty"`

	// ReviewedAt is when the item was reviewed.
	ReviewedAt time.Time `json:"reviewed_at"`

	// TimeSpentMs is the time spent reviewing this item in milliseconds.
	TimeSpentMs int64 `json:"time_spent_ms"`
}

// Analytics contains session analytics and metrics.
type Analytics struct {
	// ItemsReviewed is the total number of items reviewed.
	ItemsReviewed int `json:"items_reviewed"`

	// ItemsApproved is the number of items approved.
	ItemsApproved int `json:"items_approved"`

	// ItemsRejected is the number of items rejected.
	ItemsRejected int `json:"items_rejected"`

	// ItemsDeferred is the number of items deferred.
	ItemsDeferred int `json:"items_deferred"`

	// ItemsSkipped is the number of items skipped.
	ItemsSkipped int `json:"items_skipped"`

	// TotalTimeMs is the total active time spent in milliseconds.
	TotalTimeMs int64 `json:"total_time_ms"`

	// AverageTimePerItemMs is the average time per item in milliseconds.
	AverageTimePerItemMs int64 `json:"average_time_per_item_ms"`

	// PausedTimeMs is the total time the session was paused in milliseconds.
	PausedTimeMs int64 `json:"paused_time_ms"`

	// PauseCount is the number of times the session was paused.
	PauseCount int `json:"pause_count"`
}

// Session represents a daily review session.
type Session struct {
	// ID is the unique identifier for the session.
	ID string `json:"id"`

	// TenantID is the tenant this session belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who owns this session.
	UserID string `json:"user_id"`

	// State is the current state of the session.
	State State `json:"state"`

	// Items contains the review items processed in this session.
	Items []ReviewItem `json:"items"`

	// Analytics contains session metrics.
	Analytics Analytics `json:"analytics"`

	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the session was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// StartedAt is when the session was started (first item reviewed).
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the session was completed.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// ExpiresAt is when the session will expire if not completed.
	ExpiresAt time.Time `json:"expires_at"`

	// PausedAt is when the session was paused (if currently paused).
	PausedAt *time.Time `json:"paused_at,omitempty"`

	// Date is the date this session is for in YYYY-MM-DD format.
	Date string `json:"date"`

	// Metadata contains additional session metadata.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewSession creates a new review session with the given parameters.
func NewSession(tenantID, userID string, sessionTimeout time.Duration) *Session {
	now := time.Now().UTC()
	id := uuid.New().String()

	return &Session{
		ID:        id,
		TenantID:  tenantID,
		UserID:    userID,
		State:     StateActive,
		Items:     make([]ReviewItem, 0),
		Analytics: Analytics{},
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(sessionTimeout),
		Date:      now.Format("2006-01-02"),
		Metadata:  make(map[string]string),
	}
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().UTC().After(s.ExpiresAt)
}

// IsActive returns true if the session is in an active state and not expired.
func (s *Session) IsActive() bool {
	return s.State == StateActive && !s.IsExpired()
}

// CanModify returns true if the session can still be modified.
func (s *Session) CanModify() bool {
	return s.State == StateActive || s.State == StatePaused
}

// AddItem adds a reviewed item to the session.
func (s *Session) AddItem(item ReviewItem) error {
	if !s.CanModify() {
		if s.State == StateCompleted {
			return ErrSessionAlreadyCompleted
		}
		if s.State == StateExpired || s.IsExpired() {
			return ErrSessionExpired
		}
	}

	// Set start time on first item
	if s.StartedAt == nil {
		now := time.Now().UTC()
		s.StartedAt = &now
	}

	s.Items = append(s.Items, item)
	s.UpdatedAt = time.Now().UTC()

	// Update analytics
	s.Analytics.ItemsReviewed++
	s.Analytics.TotalTimeMs += item.TimeSpentMs

	switch item.Action {
	case ItemActionApproved:
		s.Analytics.ItemsApproved++
	case ItemActionRejected:
		s.Analytics.ItemsRejected++
	case ItemActionDeferred:
		s.Analytics.ItemsDeferred++
	case ItemActionSkipped:
		s.Analytics.ItemsSkipped++
	}

	if s.Analytics.ItemsReviewed > 0 {
		s.Analytics.AverageTimePerItemMs = s.Analytics.TotalTimeMs / int64(s.Analytics.ItemsReviewed)
	}

	return nil
}

// Pause pauses the session.
func (s *Session) Pause() error {
	if !s.State.CanTransitionTo(StatePaused) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, s.State, StatePaused)
	}

	if s.IsExpired() {
		s.State = StateExpired
		return ErrSessionExpired
	}

	now := time.Now().UTC()
	s.State = StatePaused
	s.PausedAt = &now
	s.UpdatedAt = now
	s.Analytics.PauseCount++

	return nil
}

// Resume resumes a paused session.
func (s *Session) Resume() error {
	if !s.State.CanTransitionTo(StateActive) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, s.State, StateActive)
	}

	if s.IsExpired() {
		s.State = StateExpired
		return ErrSessionExpired
	}

	now := time.Now().UTC()

	// Calculate paused time
	if s.PausedAt != nil {
		pausedDuration := now.Sub(*s.PausedAt)
		s.Analytics.PausedTimeMs += pausedDuration.Milliseconds()
	}

	s.State = StateActive
	s.PausedAt = nil
	s.UpdatedAt = now

	return nil
}

// Complete marks the session as completed.
func (s *Session) Complete() error {
	if !s.State.CanTransitionTo(StateCompleted) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, s.State, StateCompleted)
	}

	// Handle paused time if completing from paused state
	now := time.Now().UTC()
	if s.State == StatePaused && s.PausedAt != nil {
		pausedDuration := now.Sub(*s.PausedAt)
		s.Analytics.PausedTimeMs += pausedDuration.Milliseconds()
	}

	s.State = StateCompleted
	s.CompletedAt = &now
	s.PausedAt = nil
	s.UpdatedAt = now

	return nil
}

// Expire marks the session as expired.
func (s *Session) Expire() error {
	if !s.State.CanTransitionTo(StateExpired) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidStateTransition, s.State, StateExpired)
	}

	now := time.Now().UTC()
	s.State = StateExpired
	s.UpdatedAt = now

	return nil
}

// ExtendExpiry extends the session expiry time.
func (s *Session) ExtendExpiry(duration time.Duration) error {
	if !s.CanModify() {
		if s.State == StateCompleted {
			return ErrSessionAlreadyCompleted
		}
		return ErrSessionExpired
	}

	s.ExpiresAt = s.ExpiresAt.Add(duration)
	s.UpdatedAt = time.Now().UTC()

	return nil
}

// Duration returns the total duration of the session.
func (s *Session) Duration() time.Duration {
	if s.StartedAt == nil {
		return 0
	}

	end := s.UpdatedAt
	if s.CompletedAt != nil {
		end = *s.CompletedAt
	}

	return end.Sub(*s.StartedAt)
}

// ActiveDuration returns the active (non-paused) duration.
func (s *Session) ActiveDuration() time.Duration {
	total := s.Duration()
	paused := time.Duration(s.Analytics.PausedTimeMs) * time.Millisecond
	if paused > total {
		return 0
	}
	return total - paused
}

// ToJSON serializes the session to JSON.
func (s *Session) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// SessionFromJSON deserializes a session from JSON.
func SessionFromJSON(data []byte) (*Session, error) {
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &session, nil
}

// Clone creates a deep copy of the session.
func (s *Session) Clone() *Session {
	clone := &Session{
		ID:        s.ID,
		TenantID:  s.TenantID,
		UserID:    s.UserID,
		State:     s.State,
		Items:     make([]ReviewItem, len(s.Items)),
		Analytics: s.Analytics,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		ExpiresAt: s.ExpiresAt,
		Date:      s.Date,
		Metadata:  make(map[string]string, len(s.Metadata)),
	}

	copy(clone.Items, s.Items)

	for k, v := range s.Metadata {
		clone.Metadata[k] = v
	}

	if s.StartedAt != nil {
		t := *s.StartedAt
		clone.StartedAt = &t
	}
	if s.CompletedAt != nil {
		t := *s.CompletedAt
		clone.CompletedAt = &t
	}
	if s.PausedAt != nil {
		t := *s.PausedAt
		clone.PausedAt = &t
	}

	return clone
}
