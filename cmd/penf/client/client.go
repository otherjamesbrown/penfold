// Package client provides the gRPC client for connecting to the Penfold API Gateway.
// It handles connection management, retry logic, and health checking.
package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// SystemStatus represents the overall system health and status.
type SystemStatus struct {
	Healthy   bool            `json:"healthy"`
	Message   string          `json:"message"`
	Services  []ServiceHealth `json:"services"`
	Database  *DatabaseStatus `json:"database,omitempty"`
	Queues    *QueueStatus    `json:"queues,omitempty"`
	Version   *VersionInfo    `json:"version,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// ServiceHealth represents the health status of a single service.
type ServiceHealth struct {
	Name          string  `json:"name"`
	Healthy       bool    `json:"healthy"`
	Status        string  `json:"status"`
	Message       string  `json:"message,omitempty"`
	LatencyMs     float64 `json:"latency_ms,omitempty"`
	Version       string  `json:"version,omitempty"`
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`
}

// DatabaseStatus represents database health and statistics.
type DatabaseStatus struct {
	Healthy                bool    `json:"healthy"`
	Type                   string  `json:"type"`
	ConnectionStatus       string  `json:"connection_status"`
	ActiveConnections      int32   `json:"active_connections"`
	MaxConnections         int32   `json:"max_connections"`
	VectorExtensionEnabled bool    `json:"vector_extension_enabled"`
	ContentCount           int64   `json:"content_count"`
	EntityCount            int64   `json:"entity_count"`
	LatencyMs              float64 `json:"latency_ms"`
}

// QueueStatus represents message queue health and depths.
type QueueStatus struct {
	Healthy         bool             `json:"healthy"`
	Type            string           `json:"type"`
	QueueDepths     map[string]int64 `json:"queue_depths,omitempty"`
	TotalPending    int64            `json:"total_pending"`
	ProcessingRate  float64          `json:"processing_rate"`
	DeadLetterCount int64            `json:"dead_letter_count"`
}

// VersionInfo contains system version information.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// Default connection settings.
const (
	DefaultConnectTimeout    = 10 * time.Second
	DefaultKeepaliveTime     = 30 * time.Second
	DefaultKeepaliveTimeout  = 10 * time.Second
	DefaultMaxRetries        = 3
	DefaultInitialBackoff    = 100 * time.Millisecond
	DefaultMaxBackoff        = 5 * time.Second
	DefaultBackoffMultiplier = 2.0
)

// GRPCClient manages the connection to the Penfold API Gateway.
type GRPCClient struct {
	// conn is the underlying gRPC connection.
	conn *grpc.ClientConn

	// serverAddr is the address of the API Gateway.
	serverAddr string

	// options holds the client configuration.
	options *ClientOptions

	// mu protects concurrent access to connection state.
	mu sync.RWMutex

	// connected indicates if the client is currently connected.
	connected bool
}

// ClientOptions configures the GRPCClient behavior.
type ClientOptions struct {
	// ConnectTimeout is the maximum time to wait for connection.
	ConnectTimeout time.Duration

	// KeepaliveTime is the interval for keepalive pings.
	KeepaliveTime time.Duration

	// KeepaliveTimeout is the timeout for keepalive ping response.
	KeepaliveTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// InitialBackoff is the initial backoff duration for retries.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration for retries.
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff.
	BackoffMultiplier float64

	// Insecure disables TLS (for development only).
	Insecure bool

	// Debug enables verbose logging.
	Debug bool

	// TenantID is the default tenant ID to include in all requests.
	TenantID string
}

// DefaultOptions returns ClientOptions with default values.
func DefaultOptions() *ClientOptions {
	return &ClientOptions{
		ConnectTimeout:    DefaultConnectTimeout,
		KeepaliveTime:     DefaultKeepaliveTime,
		KeepaliveTimeout:  DefaultKeepaliveTimeout,
		MaxRetries:        DefaultMaxRetries,
		InitialBackoff:    DefaultInitialBackoff,
		MaxBackoff:        DefaultMaxBackoff,
		BackoffMultiplier: DefaultBackoffMultiplier,
		Insecure:          true, // Default to insecure for local development.
	}
}

// NewGRPCClient creates a new GRPCClient with the given options.
// Call Connect() to establish the connection.
func NewGRPCClient(serverAddr string, opts *ClientOptions) *GRPCClient {
	if opts == nil {
		opts = DefaultOptions()
	}

	return &GRPCClient{
		serverAddr: serverAddr,
		options:    opts,
	}
}

// Connect establishes a connection to the API Gateway.
// It uses the configured timeout and returns an error if connection fails.
func (c *GRPCClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected && c.conn != nil {
		return nil // Already connected.
	}

	// Create connection context with timeout.
	connectCtx, cancel := context.WithTimeout(ctx, c.options.ConnectTimeout)
	defer cancel()

	// Build dial options.
	dialOpts := c.buildDialOptions()

	// Establish connection.
	conn, err := grpc.DialContext(connectCtx, c.serverAddr, dialOpts...)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", c.serverAddr, err)
	}

	c.conn = conn
	c.connected = true

	return nil
}

// buildDialOptions constructs the gRPC dial options from client configuration.
func (c *GRPCClient) buildDialOptions() []grpc.DialOption {
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                c.options.KeepaliveTime,
			Timeout:             c.options.KeepaliveTimeout,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.WaitForReady(true),
		),
	}

	// Configure credentials.
	if c.options.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return opts
}

// Close closes the connection to the API Gateway.
// It's safe to call Close multiple times.
func (c *GRPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	c.connected = false

	if err != nil {
		return fmt.Errorf("closing connection: %w", err)
	}

	return nil
}

// IsConnected returns true if the client has an active connection.
func (c *GRPCClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.connected && c.conn != nil
}

// GetConnection returns the underlying gRPC connection.
// Returns nil if not connected.
func (c *GRPCClient) GetConnection() *grpc.ClientConn {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.conn
}

// HealthCheck performs a connection health check.
// Returns nil if the connection is healthy, an error otherwise.
func (c *GRPCClient) HealthCheck(ctx context.Context) error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	// Check connection state.
	state := conn.GetState()
	switch state {
	case connectivity.Ready:
		return nil
	case connectivity.Connecting:
		// Wait for connection to be ready.
		if !conn.WaitForStateChange(ctx, connectivity.Connecting) {
			return fmt.Errorf("connection timeout while connecting")
		}
		newState := conn.GetState()
		if newState != connectivity.Ready {
			return fmt.Errorf("connection failed: state is %v", newState)
		}
		return nil
	case connectivity.Idle:
		// Trigger connection attempt.
		conn.Connect()
		if !conn.WaitForStateChange(ctx, connectivity.Idle) {
			return fmt.Errorf("connection timeout from idle state")
		}
		newState := conn.GetState()
		if newState != connectivity.Ready && newState != connectivity.Connecting {
			return fmt.Errorf("connection failed: state is %v", newState)
		}
		return nil
	case connectivity.TransientFailure:
		return fmt.Errorf("connection in transient failure state")
	case connectivity.Shutdown:
		return fmt.Errorf("connection has been shut down")
	default:
		return fmt.Errorf("unknown connection state: %v", state)
	}
}

// Reconnect closes the existing connection and establishes a new one.
// Uses exponential backoff for retry attempts.
func (c *GRPCClient) Reconnect(ctx context.Context) error {
	// Close existing connection.
	if err := c.Close(); err != nil {
		// Log but don't fail - we want to try reconnecting.
		_ = err
	}

	// Attempt reconnection with exponential backoff.
	backoff := c.options.InitialBackoff
	var lastErr error

	for attempt := 0; attempt < c.options.MaxRetries; attempt++ {
		if err := c.Connect(ctx); err != nil {
			lastErr = err

			// Check if context is cancelled.
			select {
			case <-ctx.Done():
				return fmt.Errorf("reconnection cancelled: %w", ctx.Err())
			default:
			}

			// Wait with exponential backoff.
			select {
			case <-ctx.Done():
				return fmt.Errorf("reconnection cancelled during backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}

			// Increase backoff.
			backoff = time.Duration(float64(backoff) * c.options.BackoffMultiplier)
			if backoff > c.options.MaxBackoff {
				backoff = c.options.MaxBackoff
			}

			continue
		}

		return nil // Successfully reconnected.
	}

	return fmt.Errorf("reconnection failed after %d attempts: %w", c.options.MaxRetries, lastErr)
}

// WithRetry executes the given function with automatic retry on failure.
// Uses exponential backoff between retry attempts.
func (c *GRPCClient) WithRetry(ctx context.Context, fn func() error) error {
	backoff := c.options.InitialBackoff
	var lastErr error

	for attempt := 0; attempt <= c.options.MaxRetries; attempt++ {
		if err := fn(); err != nil {
			lastErr = err

			if attempt == c.options.MaxRetries {
				break
			}

			// Check if context is cancelled.
			select {
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled: %w", ctx.Err())
			default:
			}

			// Wait with exponential backoff.
			select {
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled during backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}

			// Increase backoff.
			backoff = time.Duration(float64(backoff) * c.options.BackoffMultiplier)
			if backoff > c.options.MaxBackoff {
				backoff = c.options.MaxBackoff
			}

			continue
		}

		return nil // Success.
	}

	return fmt.Errorf("operation failed after %d attempts: %w", c.options.MaxRetries+1, lastErr)
}

// ServerAddress returns the configured server address.
func (c *GRPCClient) ServerAddress() string {
	return c.serverAddr
}

// TenantID returns the configured tenant ID.
func (c *GRPCClient) TenantID() string {
	return c.options.TenantID
}

// SetTenantID updates the default tenant ID for requests.
func (c *GRPCClient) SetTenantID(tenantID string) {
	c.options.TenantID = tenantID
}

// ContextWithTenant returns a context with the tenant ID in metadata.
// If tenantID is empty, uses the default tenant ID from options.
func (c *GRPCClient) ContextWithTenant(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		tenantID = c.options.TenantID
	}
	if tenantID == "" {
		return ctx
	}
	md := metadata.Pairs("x-tenant-id", tenantID)
	return metadata.NewOutgoingContext(ctx, md)
}

// ConnectionState returns a human-readable connection state string.
func (c *GRPCClient) ConnectionState() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return "disconnected"
	}

	state := c.conn.GetState()
	switch state {
	case connectivity.Idle:
		return "idle"
	case connectivity.Connecting:
		return "connecting"
	case connectivity.Ready:
		return "ready"
	case connectivity.TransientFailure:
		return "transient_failure"
	case connectivity.Shutdown:
		return "shutdown"
	default:
		return "unknown"
	}
}

// GetStatus retrieves the system status from the gateway.
// If verbose is true, additional details are included.
// Currently returns mock data until the gateway service is implemented.
func (c *GRPCClient) GetStatus(ctx context.Context, verbose bool) (*SystemStatus, error) {
	// TODO: Replace with actual gRPC call once proto is generated.
	// The actual implementation would look like:
	//   client := cliv1.NewCLIServiceClient(c.conn)
	//   resp, err := client.GetStatus(ctx, &cliv1.GetStatusRequest{Verbose: verbose})
	//   if err != nil {
	//       return nil, fmt.Errorf("GetStatus RPC failed: %w", err)
	//   }
	//   return convertStatus(resp.Status), nil

	return getMockStatus(verbose), nil
}

// getMockStatus returns mock status data for development/testing.
// This will be removed once the gateway service is implemented.
func getMockStatus(verbose bool) *SystemStatus {
	return &SystemStatus{
		Healthy:   true,
		Message:   "All systems operational (mock data - gateway not connected)",
		Timestamp: time.Now(),
		Services: []ServiceHealth{
			{
				Name:          "gateway",
				Healthy:       true,
				Status:        "running",
				Message:       "Ready to accept connections",
				LatencyMs:     1.2,
				Version:       "0.1.0",
				UptimeSeconds: 3600,
			},
			{
				Name:          "orchestrator",
				Healthy:       true,
				Status:        "running",
				Message:       "",
				LatencyMs:     2.5,
				Version:       "0.1.0",
				UptimeSeconds: 3600,
			},
			{
				Name:          "ai_service",
				Healthy:       true,
				Status:        "running",
				Message:       "Ollama backend available",
				LatencyMs:     5.0,
				Version:       "0.1.0",
				UptimeSeconds: 3600,
			},
		},
		Database: &DatabaseStatus{
			Healthy:                true,
			Type:                   "postgresql",
			ConnectionStatus:       "connected",
			ActiveConnections:      5,
			MaxConnections:         100,
			VectorExtensionEnabled: true,
			ContentCount:           1250,
			EntityCount:            340,
			LatencyMs:              0.8,
		},
		Queues: &QueueStatus{
			Healthy:         true,
			Type:            "redis",
			TotalPending:    12,
			ProcessingRate:  45.2,
			DeadLetterCount: 0,
			QueueDepths: map[string]int64{
				"ingestion":  5,
				"embedding":  3,
				"extraction": 4,
			},
		},
		Version: &VersionInfo{
			Version:   "0.1.0",
			Commit:    "abc123",
			BuildTime: time.Now().Format(time.RFC3339),
			GoVersion: "go1.24.0",
		},
	}
}
