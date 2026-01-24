// Package ai provides a gRPC client for the AI Coordinator service.
package ai

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ClientOptions contains configuration options for the AI client.
type ClientOptions struct {
	// connectTimeout is the maximum time to wait for initial connection.
	connectTimeout time.Duration

	// requestTimeout is the default timeout for individual requests.
	// If zero, no timeout is applied.
	requestTimeout time.Duration

	// maxRetries is the maximum number of retry attempts for failed requests.
	maxRetries int

	// retryBackoff is the base duration for exponential backoff between retries.
	retryBackoff time.Duration

	// useTLS enables TLS for the connection.
	useTLS bool

	// dialOptions are additional gRPC dial options.
	dialOptions []grpc.DialOption
}

// ClientOption is a function that configures ClientOptions.
type ClientOption func(*ClientOptions)

// defaultOptions returns the default client options.
func defaultOptions() *ClientOptions {
	return &ClientOptions{
		connectTimeout: 10 * time.Second,
		requestTimeout: 30 * time.Second,
		maxRetries:     3,
		retryBackoff:   100 * time.Millisecond,
		useTLS:         false,
		dialOptions:    nil,
	}
}

// WithConnectTimeout sets the connection timeout.
func WithConnectTimeout(timeout time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.connectTimeout = timeout
	}
}

// WithRequestTimeout sets the default request timeout.
// If zero, no timeout is applied to requests.
func WithRequestTimeout(timeout time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.requestTimeout = timeout
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
// Set to 0 to disable retries.
func WithMaxRetries(maxRetries int) ClientOption {
	return func(o *ClientOptions) {
		o.maxRetries = maxRetries
	}
}

// WithRetryBackoff sets the base duration for exponential backoff.
func WithRetryBackoff(backoff time.Duration) ClientOption {
	return func(o *ClientOptions) {
		o.retryBackoff = backoff
	}
}

// WithTLS enables TLS for the connection.
func WithTLS() ClientOption {
	return func(o *ClientOptions) {
		o.useTLS = true
	}
}

// WithDialOptions adds additional gRPC dial options.
func WithDialOptions(opts ...grpc.DialOption) ClientOption {
	return func(o *ClientOptions) {
		o.dialOptions = append(o.dialOptions, opts...)
	}
}

// WithInsecure explicitly disables TLS (default behavior).
func WithInsecure() ClientOption {
	return func(o *ClientOptions) {
		o.useTLS = false
	}
}

// retryInterceptor creates a unary client interceptor that retries failed requests.
func retryInterceptor(maxRetries int, baseBackoff time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var lastErr error

		for attempt := 0; attempt <= maxRetries; attempt++ {
			// Check context before each attempt
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Execute the RPC
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}

			lastErr = err

			// Check if the error is retryable
			if !isRetryable(err) {
				return err
			}

			// Don't wait after the last attempt
			if attempt < maxRetries {
				// Calculate backoff with exponential increase
				backoff := baseBackoff * time.Duration(1<<uint(attempt))

				// Cap the backoff at a reasonable maximum
				maxBackoff := 10 * time.Second
				if backoff > maxBackoff {
					backoff = maxBackoff
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
					// Continue to next attempt
				}
			}
		}

		return lastErr
	}
}

// isRetryable determines if an error should trigger a retry.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		// Non-gRPC errors (network errors) are generally retryable
		return true
	}

	// Retry on transient errors
	switch st.Code() {
	case codes.Unavailable:
		// Service temporarily unavailable
		return true
	case codes.DeadlineExceeded:
		// Request timed out - may succeed on retry
		return true
	case codes.ResourceExhausted:
		// Rate limited - retry with backoff
		return true
	case codes.Aborted:
		// Transaction aborted - can retry
		return true
	case codes.Internal:
		// Internal server error - may be transient
		return true
	default:
		// Other errors are not retryable
		return false
	}
}
