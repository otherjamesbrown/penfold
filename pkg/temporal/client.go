// Package temporal provides Temporal client factory and configuration for Penfold services.
package temporal

import (
	"fmt"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
)

// Config holds Temporal connection settings.
type Config struct {
	HostPort                string
	Namespace               string
	TaskQueue               string
	MaxConcurrentActivities int
	MaxConcurrentWorkflows  int
}

// ClientOption is a function that configures client options.
type ClientOption func(*client.Options)

// WithLogger sets a zerolog logger for the Temporal client.
func WithLogger(logger zerolog.Logger) ClientOption {
	return func(opts *client.Options) {
		opts.Logger = NewLogger(logger)
	}
}

// WithMetricsHandler sets a metrics handler for the Temporal client.
func WithMetricsHandler(handler client.MetricsHandler) ClientOption {
	return func(opts *client.Options) {
		opts.MetricsHandler = handler
	}
}

// WithConnectionOptions sets gRPC connection options.
func WithConnectionOptions(connOpts client.ConnectionOptions) ClientOption {
	return func(opts *client.Options) {
		opts.ConnectionOptions = connOpts
	}
}

// NewClient creates a new Temporal client with the given configuration.
func NewClient(cfg *Config, options ...ClientOption) (client.Client, error) {
	opts := client.Options{
		HostPort:  cfg.HostPort,
		Namespace: cfg.Namespace,
	}

	// Apply options
	for _, opt := range options {
		opt(&opts)
	}

	c, err := client.Dial(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}
	return c, nil
}

// zerologAdapter adapts zerolog to Temporal's log interface.
type zerologAdapter struct {
	logger zerolog.Logger
}

// Debug logs at debug level.
func (a *zerologAdapter) Debug(msg string, keyvals ...interface{}) {
	a.logWithKeyvals(a.logger.Debug(), msg, keyvals...)
}

// Info logs at info level.
func (a *zerologAdapter) Info(msg string, keyvals ...interface{}) {
	a.logWithKeyvals(a.logger.Info(), msg, keyvals...)
}

// Warn logs at warn level.
func (a *zerologAdapter) Warn(msg string, keyvals ...interface{}) {
	a.logWithKeyvals(a.logger.Warn(), msg, keyvals...)
}

// Error logs at error level.
func (a *zerologAdapter) Error(msg string, keyvals ...interface{}) {
	a.logWithKeyvals(a.logger.Error(), msg, keyvals...)
}

// logWithKeyvals logs a message with key-value pairs.
func (a *zerologAdapter) logWithKeyvals(event *zerolog.Event, msg string, keyvals ...interface{}) {
	for i := 0; i < len(keyvals)-1; i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyvals[i])
		}
		event = event.Interface(key, keyvals[i+1])
	}
	event.Msg(msg)
}

// NewLogger creates a Temporal-compatible logger from zerolog.
func NewLogger(logger zerolog.Logger) log.Logger {
	return &zerologAdapter{
		logger: logger.With().Str("component", "temporal").Logger(),
	}
}
