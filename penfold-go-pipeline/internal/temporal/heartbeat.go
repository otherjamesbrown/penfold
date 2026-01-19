// Package temporal provides Temporal client factory and utilities for Penfold.
package temporal

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
)

// HeartbeatLoop runs a periodic heartbeat loop for long-running activities.
// It spawns a goroutine that sends heartbeats at the specified interval until
// the returned cancel function is called or the context is cancelled.
//
// The details function is called on each heartbeat to provide progress information.
// This allows activities to report their current progress state.
//
// If called outside of a Temporal activity context (e.g., in tests), the heartbeat
// is safely skipped without panicking.
//
// Usage:
//
//	cancel := temporal.HeartbeatLoop(ctx, 10*time.Second, func() interface{} {
//	    return map[string]interface{}{"progress": currentProgress}
//	})
//	defer cancel()
//	// ... do long-running work ...
func HeartbeatLoop(ctx context.Context, interval time.Duration, details func() interface{}) func() {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				safeRecordHeartbeat(ctx, details())
			}
		}
	}()

	return func() { close(done) }
}

// safeRecordHeartbeat calls activity.RecordHeartbeat but recovers from panic
// if called outside an activity context (e.g., in unit tests).
func safeRecordHeartbeat(ctx context.Context, details interface{}) {
	defer func() {
		// Recover from panic if RecordHeartbeat is called outside activity context
		_ = recover()
	}()
	activity.RecordHeartbeat(ctx, details)
}

// WithHeartbeat wraps a long-running operation with automatic heartbeats.
// It starts a heartbeat loop before executing the provided function and stops
// the loop when the function completes.
//
// This is useful for wrapping external API calls or other operations where
// you don't need fine-grained progress reporting.
//
// Usage:
//
//	result, err := temporal.WithHeartbeat(ctx, 10*time.Second, func() (MyResult, error) {
//	    return myClient.DoExpensiveOperation(ctx)
//	})
func WithHeartbeat[T any](ctx context.Context, interval time.Duration, fn func() (T, error)) (T, error) {
	cancel := HeartbeatLoop(ctx, interval, func() interface{} { return "processing" })
	defer cancel()

	return fn()
}

// Heartbeat intervals for different operation types.
const (
	// EmbeddingHeartbeatInterval is the recommended heartbeat interval for
	// embedding operations (local MLX inference, typically 1-5 seconds).
	EmbeddingHeartbeatInterval = 10 * time.Second

	// LLMHeartbeatInterval is the recommended heartbeat interval for
	// LLM operations (30-60 seconds typical duration).
	LLMHeartbeatInterval = 15 * time.Second
)

// WithEmbeddingHeartbeat wraps an embedding operation with the standard
// heartbeat interval for embedding ops (10 seconds).
func WithEmbeddingHeartbeat[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	return WithHeartbeat(ctx, EmbeddingHeartbeatInterval, fn)
}

// WithLLMHeartbeat wraps an LLM operation with the standard heartbeat
// interval for LLM ops (15 seconds).
func WithLLMHeartbeat[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	return WithHeartbeat(ctx, LLMHeartbeatInterval, fn)
}
