// Package lifecycle provides relationship lifecycle management capabilities.
package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/relationship/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// EventType represents the type of lifecycle event.
type EventType string

const (
	// EventRelationshipCreated is emitted when a new relationship is created.
	EventRelationshipCreated EventType = "relationship.created"

	// EventRelationshipUpdated is emitted when a relationship is updated.
	EventRelationshipUpdated EventType = "relationship.updated"

	// EventRelationshipArchived is emitted when a relationship is archived.
	EventRelationshipArchived EventType = "relationship.archived"

	// EventRelationshipRestored is emitted when an archived relationship is restored.
	EventRelationshipRestored EventType = "relationship.restored"

	// EventRelationshipMerged is emitted when two relationships are merged.
	EventRelationshipMerged EventType = "relationship.merged"

	// EventRelationshipStateChanged is emitted when a relationship's state changes.
	EventRelationshipStateChanged EventType = "relationship.state_changed"

	// EventRelationshipDecayed is emitted when a relationship is detected as decayed.
	EventRelationshipDecayed EventType = "relationship.decayed"

	// EventRelationshipConfirmed is emitted when a relationship is confirmed by a user.
	EventRelationshipConfirmed EventType = "relationship.confirmed"

	// EventRelationshipRejected is emitted when a relationship is rejected by a user.
	EventRelationshipRejected EventType = "relationship.rejected"

	// EventEvidenceAdded is emitted when evidence is added to a relationship.
	EventEvidenceAdded EventType = "relationship.evidence_added"
)

// LifecycleEvent represents an event in the relationship lifecycle.
type LifecycleEvent struct {
	// ID is a unique identifier for the event.
	ID string `json:"id"`

	// Type is the type of event.
	Type EventType `json:"type"`

	// TenantID is the tenant this event belongs to.
	TenantID string `json:"tenant_id"`

	// RelationshipID is the relationship this event relates to.
	RelationshipID string `json:"relationship_id"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Actor is who or what triggered the event.
	Actor string `json:"actor"`

	// Data contains event-specific data.
	Data map[string]interface{} `json:"data,omitempty"`

	// PreviousState is the state before the event (for state change events).
	PreviousState *State `json:"previous_state,omitempty"`

	// NewState is the state after the event (for state change events).
	NewState *State `json:"new_state,omitempty"`

	// Metadata contains additional event context.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MarshalJSON implements json.Marshaler for LifecycleEvent.
func (e *LifecycleEvent) MarshalJSON() ([]byte, error) {
	type Alias LifecycleEvent
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     (*Alias)(e),
		Timestamp: e.Timestamp.Format(time.RFC3339),
	})
}

// EventHandler is a function that handles lifecycle events.
type EventHandler func(ctx context.Context, event *LifecycleEvent) error

// EventPublisher publishes lifecycle events to downstream consumers.
type EventPublisher interface {
	// Publish publishes an event to all subscribers.
	Publish(ctx context.Context, event *LifecycleEvent) error

	// Subscribe registers a handler for a specific event type.
	Subscribe(eventType EventType, handler EventHandler)

	// SubscribeAll registers a handler for all event types.
	SubscribeAll(handler EventHandler)

	// Close shuts down the publisher.
	Close() error
}

// InMemoryEventPublisher is an in-memory implementation of EventPublisher.
type InMemoryEventPublisher struct {
	mu             sync.RWMutex
	logger         logging.Logger
	handlers       map[EventType][]EventHandler
	globalHandlers []EventHandler
	eventLog       []LifecycleEvent
	maxLogSize     int
}

// NewInMemoryEventPublisher creates a new in-memory event publisher.
func NewInMemoryEventPublisher(logger logging.Logger, maxLogSize int) *InMemoryEventPublisher {
	if maxLogSize <= 0 {
		maxLogSize = 1000
	}
	return &InMemoryEventPublisher{
		logger:     logger,
		handlers:   make(map[EventType][]EventHandler),
		maxLogSize: maxLogSize,
	}
}

// Publish publishes an event to all subscribers.
func (p *InMemoryEventPublisher) Publish(ctx context.Context, event *LifecycleEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Assign ID if not set
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	p.logger.Info("Publishing lifecycle event",
		logging.F("event_id", event.ID),
		logging.F("event_type", string(event.Type)),
		logging.F("relationship_id", event.RelationshipID),
		logging.F("tenant_id", event.TenantID),
	)

	// Log the event
	p.mu.Lock()
	p.eventLog = append(p.eventLog, *event)
	if len(p.eventLog) > p.maxLogSize {
		p.eventLog = p.eventLog[1:]
	}

	// Get handlers while holding lock
	handlers := make([]EventHandler, 0)
	handlers = append(handlers, p.globalHandlers...)
	if typeHandlers, ok := p.handlers[event.Type]; ok {
		handlers = append(handlers, typeHandlers...)
	}
	p.mu.Unlock()

	// Execute handlers without lock
	var lastErr error
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			p.logger.Error("Event handler failed",
				logging.F("event_id", event.ID),
				logging.F("event_type", string(event.Type)),
				logging.Err(err),
			)
			lastErr = err
		}
	}

	return lastErr
}

// Subscribe registers a handler for a specific event type.
func (p *InMemoryEventPublisher) Subscribe(eventType EventType, handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[eventType] = append(p.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all event types.
func (p *InMemoryEventPublisher) SubscribeAll(handler EventHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.globalHandlers = append(p.globalHandlers, handler)
}

// Close shuts down the publisher.
func (p *InMemoryEventPublisher) Close() error {
	return nil
}

// GetEventLog returns the logged events (for testing/debugging).
func (p *InMemoryEventPublisher) GetEventLog() []LifecycleEvent {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]LifecycleEvent, len(p.eventLog))
	copy(result, p.eventLog)
	return result
}

// EventBuilder helps construct lifecycle events.
type EventBuilder struct {
	event *LifecycleEvent
}

// NewEventBuilder creates a new EventBuilder.
func NewEventBuilder(eventType EventType) *EventBuilder {
	return &EventBuilder{
		event: &LifecycleEvent{
			ID:        uuid.New().String(),
			Type:      eventType,
			Timestamp: time.Now(),
			Data:      make(map[string]interface{}),
			Metadata:  make(map[string]string),
		},
	}
}

// WithRelationship sets the relationship information.
func (b *EventBuilder) WithRelationship(rel *relationshipv1.Relationship) *EventBuilder {
	if rel != nil {
		b.event.RelationshipID = rel.Id
		b.event.TenantID = rel.TenantId
	}
	return b
}

// WithRelationshipID sets the relationship ID.
func (b *EventBuilder) WithRelationshipID(id string) *EventBuilder {
	b.event.RelationshipID = id
	return b
}

// WithTenantID sets the tenant ID.
func (b *EventBuilder) WithTenantID(tenantID string) *EventBuilder {
	b.event.TenantID = tenantID
	return b
}

// WithActor sets the actor.
func (b *EventBuilder) WithActor(actor string) *EventBuilder {
	b.event.Actor = actor
	return b
}

// WithStateChange sets the state change information.
func (b *EventBuilder) WithStateChange(from, to State) *EventBuilder {
	b.event.PreviousState = &from
	b.event.NewState = &to
	return b
}

// WithData adds a key-value pair to the event data.
func (b *EventBuilder) WithData(key string, value interface{}) *EventBuilder {
	b.event.Data[key] = value
	return b
}

// WithMetadata adds a key-value pair to the event metadata.
func (b *EventBuilder) WithMetadata(key, value string) *EventBuilder {
	b.event.Metadata[key] = value
	return b
}

// WithTimestamp sets the event timestamp.
func (b *EventBuilder) WithTimestamp(t time.Time) *EventBuilder {
	b.event.Timestamp = t
	return b
}

// Build returns the constructed event.
func (b *EventBuilder) Build() *LifecycleEvent {
	return b.event
}

// AuditLogger provides audit log integration for lifecycle events.
type AuditLogger struct {
	logger logging.Logger
}

// NewAuditLogger creates a new AuditLogger.
func NewAuditLogger(logger logging.Logger) *AuditLogger {
	return &AuditLogger{logger: logger}
}

// LogEvent logs a lifecycle event for audit purposes.
func (a *AuditLogger) LogEvent(ctx context.Context, event *LifecycleEvent) error {
	a.logger.Info("AUDIT: Lifecycle event",
		logging.F("event_id", event.ID),
		logging.F("event_type", string(event.Type)),
		logging.F("tenant_id", event.TenantID),
		logging.F("relationship_id", event.RelationshipID),
		logging.F("actor", event.Actor),
		logging.F("timestamp", event.Timestamp),
	)

	// Log state changes with more detail
	if event.PreviousState != nil && event.NewState != nil {
		a.logger.Info("AUDIT: State transition",
			logging.F("event_id", event.ID),
			logging.F("previous_state", event.PreviousState.String()),
			logging.F("new_state", event.NewState.String()),
		)
	}

	return nil
}

// CreateAuditHandler creates an EventHandler that logs to the audit logger.
func (a *AuditLogger) CreateAuditHandler() EventHandler {
	return func(ctx context.Context, event *LifecycleEvent) error {
		return a.LogEvent(ctx, event)
	}
}

// EventFactory provides helper methods for creating common lifecycle events.
type EventFactory struct {
	defaultActor string
}

// NewEventFactory creates a new EventFactory.
func NewEventFactory(defaultActor string) *EventFactory {
	return &EventFactory{defaultActor: defaultActor}
}

// CreatedEvent creates a relationship created event.
func (f *EventFactory) CreatedEvent(rel *relationshipv1.Relationship, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipCreated).
		WithRelationship(rel).
		WithActor(actor).
		WithData("confidence", rel.Confidence).
		WithData("relationship_type", rel.RelationshipType.String()).
		Build()
}

// UpdatedEvent creates a relationship updated event.
func (f *EventFactory) UpdatedEvent(rel *relationshipv1.Relationship, actor string, changes map[string]interface{}) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	builder := NewEventBuilder(EventRelationshipUpdated).
		WithRelationship(rel).
		WithActor(actor)

	for k, v := range changes {
		builder.WithData(k, v)
	}

	return builder.Build()
}

// StateChangedEvent creates a state changed event.
func (f *EventFactory) StateChangedEvent(rel *relationshipv1.Relationship, from, to State, reason, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipStateChanged).
		WithRelationship(rel).
		WithActor(actor).
		WithStateChange(from, to).
		WithData("reason", reason).
		Build()
}

// ArchivedEvent creates a relationship archived event.
func (f *EventFactory) ArchivedEvent(rel *relationshipv1.Relationship, reason, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipArchived).
		WithRelationship(rel).
		WithActor(actor).
		WithData("reason", reason).
		Build()
}

// RestoredEvent creates a relationship restored event.
func (f *EventFactory) RestoredEvent(rel *relationshipv1.Relationship, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipRestored).
		WithRelationship(rel).
		WithActor(actor).
		Build()
}

// MergedEvent creates a relationship merged event.
func (f *EventFactory) MergedEvent(sourceID, targetID, tenantID, reason, actor string, evidenceTransferred int) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipMerged).
		WithRelationshipID(sourceID).
		WithTenantID(tenantID).
		WithActor(actor).
		WithData("target_id", targetID).
		WithData("reason", reason).
		WithData("evidence_transferred", evidenceTransferred).
		Build()
}

// DecayedEvent creates a relationship decayed event.
func (f *EventFactory) DecayedEvent(rel *relationshipv1.Relationship, decayInfo *DecayInfo) *LifecycleEvent {
	return NewEventBuilder(EventRelationshipDecayed).
		WithRelationship(rel).
		WithActor("system").
		WithData("days_since_activity", decayInfo.DaysSinceActivity).
		WithData("confidence_decay", decayInfo.ConfidenceDecay).
		WithData("last_activity_at", decayInfo.LastActivityAt).
		Build()
}

// ConfirmedEvent creates a relationship confirmed event.
func (f *EventFactory) ConfirmedEvent(rel *relationshipv1.Relationship, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipConfirmed).
		WithRelationship(rel).
		WithActor(actor).
		WithStateChange(StateProposed, StateActive).
		Build()
}

// RejectedEvent creates a relationship rejected event.
func (f *EventFactory) RejectedEvent(rel *relationshipv1.Relationship, reason, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventRelationshipRejected).
		WithRelationship(rel).
		WithActor(actor).
		WithStateChange(StateProposed, StateArchived).
		WithData("reason", reason).
		Build()
}

// EvidenceAddedEvent creates an evidence added event.
func (f *EventFactory) EvidenceAddedEvent(rel *relationshipv1.Relationship, evidenceCount int, actor string) *LifecycleEvent {
	if actor == "" {
		actor = f.defaultActor
	}
	return NewEventBuilder(EventEvidenceAdded).
		WithRelationship(rel).
		WithActor(actor).
		WithData("evidence_count", evidenceCount).
		Build()
}
