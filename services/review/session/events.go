// Package session provides event types for session audit trails.
package session

import (
	"encoding/json"
	"time"
)

// EventType represents the type of session event.
type EventType string

const (
	// EventTypeSessionStarted is emitted when a session is created.
	EventTypeSessionStarted EventType = "session.started"

	// EventTypeSessionPaused is emitted when a session is paused.
	EventTypeSessionPaused EventType = "session.paused"

	// EventTypeSessionResumed is emitted when a session is resumed.
	EventTypeSessionResumed EventType = "session.resumed"

	// EventTypeSessionCompleted is emitted when a session is completed.
	EventTypeSessionCompleted EventType = "session.completed"

	// EventTypeSessionExpired is emitted when a session expires.
	EventTypeSessionExpired EventType = "session.expired"

	// EventTypeItemReviewed is emitted when an item is reviewed.
	EventTypeItemReviewed EventType = "session.item_reviewed"

	// EventTypeSessionExtended is emitted when session expiry is extended.
	EventTypeSessionExtended EventType = "session.extended"
)

// Event represents a session audit event.
type Event struct {
	// ID is the unique identifier for the event.
	ID string `json:"id"`

	// Type is the type of event.
	Type EventType `json:"type"`

	// SessionID is the session this event belongs to.
	SessionID string `json:"session_id"`

	// TenantID is the tenant this event belongs to.
	TenantID string `json:"tenant_id"`

	// UserID is the user who triggered the event.
	UserID string `json:"user_id"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Data contains event-specific data.
	Data map[string]interface{} `json:"data,omitempty"`

	// CorrelationID for tracing across services.
	CorrelationID string `json:"correlation_id,omitempty"`

	// Version is the event schema version.
	Version string `json:"version"`
}

// NewEvent creates a new session event.
func NewEvent(eventType EventType, sessionID, tenantID, userID string) *Event {
	return &Event{
		ID:        generateEventID(),
		Type:      eventType,
		SessionID: sessionID,
		TenantID:  tenantID,
		UserID:    userID,
		Timestamp: time.Now().UTC(),
		Data:      make(map[string]interface{}),
		Version:   "1.0",
	}
}

// WithData adds data to the event.
func (e *Event) WithData(key string, value interface{}) *Event {
	e.Data[key] = value
	return e
}

// WithCorrelationID sets the correlation ID for tracing.
func (e *Event) WithCorrelationID(correlationID string) *Event {
	e.CorrelationID = correlationID
	return e
}

// ToJSON serializes the event to JSON.
func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// EventFromJSON deserializes an event from JSON.
func EventFromJSON(data []byte) (*Event, error) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// generateEventID generates a unique event ID.
// Uses a combination of timestamp and random bytes for uniqueness.
func generateEventID() string {
	return time.Now().UTC().Format("20060102150405.000000") + "-" + randomHex(8)
}

// randomHex generates a random hex string of the specified length.
func randomHex(length int) string {
	const charset = "0123456789abcdef"
	b := make([]byte, length)
	for i := range b {
		// Use time-based seeding for simplicity
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// SessionStartedEvent contains data for a session started event.
type SessionStartedEvent struct {
	SessionID string    `json:"session_id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	Date      string    `json:"date"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionPausedEvent contains data for a session paused event.
type SessionPausedEvent struct {
	SessionID     string `json:"session_id"`
	TenantID      string `json:"tenant_id"`
	UserID        string `json:"user_id"`
	PauseCount    int    `json:"pause_count"`
	ItemsReviewed int    `json:"items_reviewed"`
}

// SessionResumedEvent contains data for a session resumed event.
type SessionResumedEvent struct {
	SessionID    string `json:"session_id"`
	TenantID     string `json:"tenant_id"`
	UserID       string `json:"user_id"`
	PausedTimeMs int64  `json:"paused_time_ms"`
}

// SessionCompletedEvent contains data for a session completed event.
type SessionCompletedEvent struct {
	SessionID     string    `json:"session_id"`
	TenantID      string    `json:"tenant_id"`
	UserID        string    `json:"user_id"`
	Analytics     Analytics `json:"analytics"`
	DurationMs    int64     `json:"duration_ms"`
	CompletedAt   time.Time `json:"completed_at"`
}

// SessionExpiredEvent contains data for a session expired event.
type SessionExpiredEvent struct {
	SessionID     string    `json:"session_id"`
	TenantID      string    `json:"tenant_id"`
	UserID        string    `json:"user_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	ItemsReviewed int       `json:"items_reviewed"`
}

// ItemReviewedEvent contains data for an item reviewed event.
type ItemReviewedEvent struct {
	SessionID   string     `json:"session_id"`
	TenantID    string     `json:"tenant_id"`
	UserID      string     `json:"user_id"`
	ItemID      string     `json:"item_id"`
	ContentID   string     `json:"content_id"`
	ContentType string     `json:"content_type"`
	Action      ItemAction `json:"action"`
	TimeSpentMs int64      `json:"time_spent_ms"`
}

// EventPublisher defines the interface for publishing session events.
type EventPublisher interface {
	// Publish publishes an event.
	Publish(event *Event) error
}

// NoOpPublisher is a no-op event publisher for testing.
type NoOpPublisher struct{}

// Publish does nothing.
func (p *NoOpPublisher) Publish(event *Event) error {
	return nil
}

// InMemoryPublisher stores events in memory for testing.
type InMemoryPublisher struct {
	events []*Event
}

// NewInMemoryPublisher creates a new in-memory publisher.
func NewInMemoryPublisher() *InMemoryPublisher {
	return &InMemoryPublisher{
		events: make([]*Event, 0),
	}
}

// Publish stores the event in memory.
func (p *InMemoryPublisher) Publish(event *Event) error {
	p.events = append(p.events, event)
	return nil
}

// Events returns all published events.
func (p *InMemoryPublisher) Events() []*Event {
	return p.events
}

// Clear clears all stored events.
func (p *InMemoryPublisher) Clear() {
	p.events = make([]*Event, 0)
}
