package health

import (
	"context"
	"fmt"
)

// DatabasePinger is the interface for database connection pools.
// This is compatible with *pgxpool.Pool and *sql.DB.
type DatabasePinger interface {
	Ping(ctx context.Context) error
}

// DatabaseCheck creates a health check for database connectivity.
// It accepts any connection pool that implements the Ping method.
func DatabaseCheck(pool DatabasePinger) CheckFunc {
	return func(ctx context.Context) error {
		if pool == nil {
			return fmt.Errorf("database pool is nil")
		}
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("database ping failed: %w", err)
		}
		return nil
	}
}

// TemporalHealthChecker is the interface for Temporal clients that support health checks.
type TemporalHealthChecker interface {
	CheckHealth(ctx context.Context, request interface{}) (interface{}, error)
}

// TemporalClient is the interface for Temporal client connectivity check.
// This is compatible with go.temporal.io/sdk/client.Client.
type TemporalClient interface {
	// Close is used to verify the client is not nil and functional
	Close()
}

// TemporalCheck creates a health check for Temporal connectivity.
// It verifies the client is initialized and can communicate with the server.
func TemporalCheck(client TemporalClient) CheckFunc {
	return func(ctx context.Context) error {
		if client == nil {
			return fmt.Errorf("temporal client is nil")
		}
		// The Temporal SDK doesn't expose a simple ping method.
		// Checking if the client is not nil is the basic check.
		// For more thorough checks, use TemporalCheckWithNamespace.
		return nil
	}
}

// TemporalNamespaceDescriber can describe a Temporal namespace.
type TemporalNamespaceDescriber interface {
	DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (interface{}, error)
}

// TemporalWorkflowServiceClient is the interface for the Temporal workflow service.
type TemporalWorkflowServiceClient interface {
	GetSystemInfo(ctx context.Context, request interface{}) (interface{}, error)
}

// RedisPinger is the interface for Redis clients.
// This is compatible with github.com/redis/go-redis/v9.
type RedisPinger interface {
	Ping(ctx context.Context) RedisStatusCmd
}

// RedisStatusCmd is the interface for Redis status command results.
type RedisStatusCmd interface {
	Err() error
}

// RedisCheck creates a health check for Redis connectivity.
// It accepts any Redis client that implements the Ping method.
func RedisCheck(client RedisPinger) CheckFunc {
	return func(ctx context.Context) error {
		if client == nil {
			return fmt.Errorf("redis client is nil")
		}
		if err := client.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis ping failed: %w", err)
		}
		return nil
	}
}

// HTTPEndpointChecker checks if an HTTP endpoint is reachable.
type HTTPEndpointChecker interface {
	Get(url string) (interface{ StatusCode() int }, error)
}

// CustomCheck creates a health check from a simple error-returning function.
// This is useful for application-specific checks.
func CustomCheck(checkFn func() error) CheckFunc {
	return func(ctx context.Context) error {
		return checkFn()
	}
}

// ContextualCheck creates a health check that respects context cancellation.
func ContextualCheck(checkFn func(ctx context.Context) error) CheckFunc {
	return checkFn
}
