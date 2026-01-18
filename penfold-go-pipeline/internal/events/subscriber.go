package events

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// Default channels for event subscription.
const (
	ChannelManualEmailIngested = "events.manual_email.ingested"
	ChannelContentIngested     = "events.content.ingested"
)

// MessageHandler is a callback function that processes incoming messages.
type MessageHandler func(ctx context.Context, channel string, payload []byte) error

// SubscriberConfig holds configuration for the Redis subscriber.
type SubscriberConfig struct {
	// Channels to subscribe to
	Channels []string

	// Reconnection settings
	ReconnectDelay    time.Duration
	MaxReconnectDelay time.Duration
	ReconnectBackoff  float64

	// Processing settings
	WorkerCount int
}

// DefaultSubscriberConfig returns a SubscriberConfig with sensible defaults.
func DefaultSubscriberConfig() SubscriberConfig {
	return SubscriberConfig{
		Channels: []string{
			ChannelManualEmailIngested,
			ChannelContentIngested,
		},
		ReconnectDelay:    time.Second,
		MaxReconnectDelay: 30 * time.Second,
		ReconnectBackoff:  2.0,
		WorkerCount:       1,
	}
}

// Subscriber manages Redis pub/sub subscriptions with reconnection logic.
type Subscriber struct {
	client  *redis.Client
	config  SubscriberConfig
	handler MessageHandler
	logger  zerolog.Logger

	pubsub    *redis.PubSub
	cancelFn  context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.RWMutex
	running   bool
	reconnecting bool
}

// NewSubscriber creates a new Redis subscriber.
func NewSubscriber(client *redis.Client, config SubscriberConfig, handler MessageHandler, logger zerolog.Logger) *Subscriber {
	return &Subscriber{
		client:  client,
		config:  config,
		handler: handler,
		logger:  logger.With().Str("component", "subscriber").Logger(),
	}
}

// Start begins subscribing to configured channels.
// This method is non-blocking and returns immediately.
func (s *Subscriber) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("subscriber already running")
	}

	subCtx, cancel := context.WithCancel(ctx)
	s.cancelFn = cancel
	s.running = true
	s.mu.Unlock()

	if err := s.subscribe(subCtx); err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	// Start message processing goroutine
	s.wg.Add(1)
	go s.processMessages(subCtx)

	s.logger.Info().
		Strs("channels", s.config.Channels).
		Msg("subscriber started")

	return nil
}

// Stop gracefully shuts down the subscriber.
func (s *Subscriber) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	// Cancel the context to signal shutdown
	if s.cancelFn != nil {
		s.cancelFn()
	}

	// Close the pubsub connection
	if s.pubsub != nil {
		if err := s.pubsub.Close(); err != nil {
			s.logger.Warn().Err(err).Msg("error closing pubsub connection")
		}
	}

	// Wait for processing goroutine to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info().Msg("subscriber stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn().Msg("subscriber stop timed out")
		return ctx.Err()
	}
}

// IsRunning returns whether the subscriber is currently running.
func (s *Subscriber) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// subscribe creates the Redis pub/sub subscription.
func (s *Subscriber) subscribe(ctx context.Context) error {
	s.pubsub = s.client.Subscribe(ctx, s.config.Channels...)

	// Wait for subscription confirmation
	_, err := s.pubsub.Receive(ctx)
	if err != nil {
		return err
	}

	return nil
}

// processMessages reads messages from the pub/sub channel and dispatches them.
func (s *Subscriber) processMessages(ctx context.Context) {
	defer s.wg.Done()

	ch := s.pubsub.Channel()
	reconnectDelay := s.config.ReconnectDelay

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug().Msg("context cancelled, stopping message processing")
			return

		case msg, ok := <-ch:
			if !ok {
				// Channel closed, attempt reconnection
				if !s.shouldReconnect(ctx) {
					return
				}

				if err := s.reconnect(ctx, &reconnectDelay); err != nil {
					s.logger.Error().Err(err).Msg("failed to reconnect")
					return
				}

				// Reset delay on successful reconnect
				reconnectDelay = s.config.ReconnectDelay
				ch = s.pubsub.Channel()
				continue
			}

			// Process the message
			s.handleMessage(ctx, msg)
		}
	}
}

// shouldReconnect checks if reconnection should be attempted.
func (s *Subscriber) shouldReconnect(ctx context.Context) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running && ctx.Err() == nil
}

// reconnect attempts to re-establish the Redis subscription.
func (s *Subscriber) reconnect(ctx context.Context, delay *time.Duration) error {
	s.mu.Lock()
	if s.reconnecting {
		s.mu.Unlock()
		return nil
	}
	s.reconnecting = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.reconnecting = false
		s.mu.Unlock()
	}()

	s.logger.Info().
		Dur("delay", *delay).
		Msg("attempting to reconnect")

	// Wait before reconnecting
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(*delay):
	}

	// Close existing connection if any
	if s.pubsub != nil {
		_ = s.pubsub.Close()
	}

	// Attempt to subscribe again
	if err := s.subscribe(ctx); err != nil {
		// Increase delay for next attempt with backoff
		newDelay := time.Duration(float64(*delay) * s.config.ReconnectBackoff)
		if newDelay > s.config.MaxReconnectDelay {
			newDelay = s.config.MaxReconnectDelay
		}
		*delay = newDelay

		s.logger.Warn().
			Err(err).
			Dur("next_delay", *delay).
			Msg("reconnection failed, will retry")
		return err
	}

	s.logger.Info().Msg("successfully reconnected")
	return nil
}

// handleMessage processes a single message.
func (s *Subscriber) handleMessage(ctx context.Context, msg *redis.Message) {
	logger := s.logger.With().
		Str("channel", msg.Channel).
		Logger()

	logger.Debug().Msg("received message")

	if s.handler == nil {
		logger.Warn().Msg("no handler configured, dropping message")
		return
	}

	if err := s.handler(ctx, msg.Channel, []byte(msg.Payload)); err != nil {
		logger.Error().
			Err(err).
			Msg("handler returned error")
	}
}

// Ping checks the Redis connection health.
func (s *Subscriber) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}
