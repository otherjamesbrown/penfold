// Package temporal provides Temporal client factory and configuration for Penfold.
package temporal

import (
	"fmt"

	"github.com/rs/zerolog"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"

	"github.com/otherjamesbrown/penfold-go-pipeline/internal/config"
)

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

// NewClient creates a new Temporal client with the given configuration.
func NewClient(cfg config.TemporalConfig, logger zerolog.Logger) (client.Client, error) {
	temporalLogger := &zerologAdapter{
		logger: logger.With().Str("component", "temporal-client").Logger(),
	}

	options := client.Options{
		HostPort:  cfg.HostPort,
		Namespace: cfg.Namespace,
		Logger:    temporalLogger,
	}

	c, err := client.Dial(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create Temporal client: %w", err)
	}

	logger.Info().
		Str("host_port", cfg.HostPort).
		Str("namespace", cfg.Namespace).
		Msg("Temporal client connected")

	return c, nil
}

// NewLogger creates a Temporal-compatible logger from zerolog.
func NewLogger(logger zerolog.Logger) log.Logger {
	return &zerologAdapter{
		logger: logger.With().Str("component", "temporal").Logger(),
	}
}
