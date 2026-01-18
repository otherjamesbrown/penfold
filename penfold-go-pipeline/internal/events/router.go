package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// EventHandler processes a specific event type.
type EventHandler interface {
	// HandleEvent processes an event and returns an error if processing fails.
	HandleEvent(ctx context.Context, event interface{}) error
}

// EventHandlerFunc is an adapter to allow use of ordinary functions as EventHandlers.
type EventHandlerFunc func(ctx context.Context, event interface{}) error

// HandleEvent calls f(ctx, event).
func (f EventHandlerFunc) HandleEvent(ctx context.Context, event interface{}) error {
	return f(ctx, event)
}

// TypedHandler provides type-safe event handling.
type TypedHandler[T any] struct {
	Handler func(ctx context.Context, event *T) error
}

// HandleEvent implements EventHandler with type assertion.
func (h *TypedHandler[T]) HandleEvent(ctx context.Context, event interface{}) error {
	typed, ok := event.(*T)
	if !ok {
		return fmt.Errorf("invalid event type: expected %T, got %T", new(T), event)
	}
	return h.Handler(ctx, typed)
}

// NewTypedHandler creates a typed handler for a specific event type.
func NewTypedHandler[T any](handler func(ctx context.Context, event *T) error) *TypedHandler[T] {
	return &TypedHandler[T]{Handler: handler}
}

// Router dispatches events to registered handlers based on event type.
type Router struct {
	handlers map[string][]EventHandler
	parsers  map[string]func([]byte) (interface{}, error)
	mu       sync.RWMutex
	logger   zerolog.Logger
}

// NewRouter creates a new event router.
func NewRouter(logger zerolog.Logger) *Router {
	r := &Router{
		handlers: make(map[string][]EventHandler),
		parsers:  make(map[string]func([]byte) (interface{}, error)),
		logger:   logger.With().Str("component", "router").Logger(),
	}

	// Register default parsers for known event types
	r.registerDefaultParsers()

	return r
}

// registerDefaultParsers sets up parsers for built-in event types.
func (r *Router) registerDefaultParsers() {
	r.RegisterParser("manual_email.ingested", func(data []byte) (interface{}, error) {
		return ParseManualEmailIngestedEvent(data)
	})

	r.RegisterParser("content.ingested", func(data []byte) (interface{}, error) {
		return ParseContentIngestedEvent(data)
	})

	r.RegisterParser("ingest_job.progress", func(data []byte) (interface{}, error) {
		var event IngestJobProgressEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		return &event, nil
	})

	r.RegisterParser("ingest_job.completed", func(data []byte) (interface{}, error) {
		var event IngestJobCompletedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		return &event, nil
	})

	r.RegisterParser("manual_attachment.extracted", func(data []byte) (interface{}, error) {
		var event ManualAttachmentExtractedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		return &event, nil
	})
}

// RegisterParser registers a parser function for a specific event type.
func (r *Router) RegisterParser(eventType string, parser func([]byte) (interface{}, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parsers[eventType] = parser
}

// Register adds a handler for a specific event type.
// Multiple handlers can be registered for the same event type.
func (r *Router) Register(eventType string, handler EventHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers[eventType] = append(r.handlers[eventType], handler)
	r.logger.Debug().
		Str("event_type", eventType).
		Int("handler_count", len(r.handlers[eventType])).
		Msg("registered handler")
}

// RegisterFunc registers a function as a handler for a specific event type.
func (r *Router) RegisterFunc(eventType string, handler func(ctx context.Context, event interface{}) error) {
	r.Register(eventType, EventHandlerFunc(handler))
}

// Unregister removes all handlers for a specific event type.
func (r *Router) Unregister(eventType string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.handlers, eventType)
	r.logger.Debug().
		Str("event_type", eventType).
		Msg("unregistered all handlers")
}

// Route processes an incoming message by parsing it and dispatching to handlers.
// This method is designed to be used as the MessageHandler for Subscriber.
func (r *Router) Route(ctx context.Context, channel string, payload []byte) error {
	// Extract event type from channel name
	// Channel format: "events.<event_type>" e.g., "events.manual_email.ingested"
	eventType := extractEventType(channel)

	logger := r.logger.With().
		Str("channel", channel).
		Str("event_type", eventType).
		Logger()

	logger.Debug().Msg("routing event")

	// Parse the event
	event, err := r.parseEvent(eventType, payload)
	if err != nil {
		logger.Error().Err(err).Msg("failed to parse event")
		return fmt.Errorf("failed to parse event: %w", err)
	}

	// Dispatch to handlers
	return r.dispatch(ctx, eventType, event, logger)
}

// parseEvent parses the raw payload into a typed event.
func (r *Router) parseEvent(eventType string, payload []byte) (interface{}, error) {
	r.mu.RLock()
	parser, ok := r.parsers[eventType]
	r.mu.RUnlock()

	if !ok {
		// Fallback: try to extract event_type from payload and use that
		var envelope struct {
			EventType string `json:"event_type"`
		}
		if err := json.Unmarshal(payload, &envelope); err == nil && envelope.EventType != "" {
			r.mu.RLock()
			parser, ok = r.parsers[envelope.EventType]
			r.mu.RUnlock()
		}

		if !ok {
			// Return generic map if no specific parser
			var generic map[string]interface{}
			if err := json.Unmarshal(payload, &generic); err != nil {
				return nil, err
			}
			return generic, nil
		}
	}

	return parser(payload)
}

// dispatch sends the event to all registered handlers.
func (r *Router) dispatch(ctx context.Context, eventType string, event interface{}, logger zerolog.Logger) error {
	r.mu.RLock()
	handlers := r.handlers[eventType]
	r.mu.RUnlock()

	if len(handlers) == 0 {
		logger.Warn().Msg("no handlers registered for event type")
		return nil
	}

	var errs []error
	for i, handler := range handlers {
		if err := handler.HandleEvent(ctx, event); err != nil {
			logger.Error().
				Err(err).
				Int("handler_index", i).
				Msg("handler returned error")
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	logger.Debug().
		Int("handler_count", len(handlers)).
		Msg("event dispatched successfully")

	return nil
}

// HasHandlers returns whether any handlers are registered for an event type.
func (r *Router) HasHandlers(eventType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[eventType]) > 0
}

// HandlerCount returns the number of handlers registered for an event type.
func (r *Router) HandlerCount(eventType string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers[eventType])
}

// RegisteredEventTypes returns all event types that have handlers registered.
func (r *Router) RegisteredEventTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.handlers))
	for eventType := range r.handlers {
		types = append(types, eventType)
	}
	return types
}

// extractEventType extracts the event type from a channel name.
// Channel format: "events.<event_type>" e.g., "events.manual_email.ingested"
func extractEventType(channel string) string {
	const prefix = "events."
	if len(channel) > len(prefix) && channel[:len(prefix)] == prefix {
		return channel[len(prefix):]
	}
	return channel
}

// ManualEmailHandler is a convenience type for handling ManualEmailIngestedEvent.
type ManualEmailHandler func(ctx context.Context, event *ManualEmailIngestedEvent) error

// RegisterManualEmailHandler registers a handler specifically for ManualEmailIngestedEvent.
func (r *Router) RegisterManualEmailHandler(handler ManualEmailHandler) {
	r.Register("manual_email.ingested", NewTypedHandler(handler))
}

// ContentIngestedHandler is a convenience type for handling ContentIngestedEvent.
type ContentIngestedHandler func(ctx context.Context, event *ContentIngestedEvent) error

// RegisterContentIngestedHandler registers a handler specifically for ContentIngestedEvent.
func (r *Router) RegisterContentIngestedHandler(handler ContentIngestedHandler) {
	r.Register("content.ingested", NewTypedHandler(handler))
}
