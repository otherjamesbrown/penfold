// Package ai provides a gRPC client for the AI Coordinator service.
package ai

import (
	"context"
	"fmt"
	"sync"
	"time"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client provides a gRPC client for the AI Coordinator service.
// It implements the AIClient interface from services/worker/activities.
type Client struct {
	conn      *grpc.ClientConn
	client    aiv1.AICoordinatorServiceClient
	options   *ClientOptions
	closeOnce sync.Once
	closeErr  error
}

// NewClient creates a new AI service gRPC client.
// The addr parameter should be in the format "host:port".
// Returns an error if the connection cannot be established.
func NewClient(addr string, opts ...ClientOption) (*Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("address is required")
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	// Build dial options
	dialOpts := []grpc.DialOption{}

	// Add retry interceptor if configured
	if options.maxRetries > 0 {
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(
			retryInterceptor(options.maxRetries, options.retryBackoff),
		))
	}

	// Add connection timeout
	ctx, cancel := context.WithTimeout(context.Background(), options.connectTimeout)
	defer cancel()

	// Add insecure credentials if TLS is not configured
	if !options.useTLS {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Append any additional dial options
	dialOpts = append(dialOpts, options.dialOptions...)

	// Create the connection
	conn, err := grpc.DialContext(ctx, addr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to AI service at %s: %w", addr, err)
	}

	return &Client{
		conn:    conn,
		client:  aiv1.NewAICoordinatorServiceClient(conn),
		options: options,
	}, nil
}

// GenerateEmbedding generates a vector embedding for the given text.
// Implements the AIClient interface.
func (c *Client) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.GenerateEmbedding(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate embedding failed: %w", err)
	}

	return resp, nil
}

// GenerateSummary generates a summary for the given content.
// Implements the AIClient interface.
func (c *Client) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.GenerateSummary(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("generate summary failed: %w", err)
	}

	return resp, nil
}

// ExtractAssertions extracts assertions from the given content.
// Implements the AIClient interface.
func (c *Client) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.ExtractAssertions(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("extract assertions failed: %w", err)
	}

	return resp, nil
}

// Close closes the gRPC connection and releases resources.
// Close is safe to call multiple times; subsequent calls return the same error.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if c.conn != nil {
			c.closeErr = c.conn.Close()
		}
	})
	return c.closeErr
}

// HealthCheck verifies the connection to the AI service is healthy.
// It performs a GetModelStatus call to check connectivity and service availability.
func (c *Client) HealthCheck(ctx context.Context) error {
	// Apply a shorter timeout for health checks
	timeout := 5 * time.Second
	if c.options.requestTimeout > 0 && c.options.requestTimeout < timeout {
		timeout = c.options.requestTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use GetModelStatus as a health check endpoint
	_, err := c.client.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{})
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// GetModelStatus retrieves the status of AI models.
// This can be used for monitoring and health checks.
func (c *Client) GetModelStatus(ctx context.Context, req *aiv1.GetModelStatusRequest) (*aiv1.GetModelStatusResponse, error) {
	if req == nil {
		req = &aiv1.GetModelStatusRequest{}
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.GetModelStatus(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get model status failed: %w", err)
	}

	return resp, nil
}

// ClassifyContent categorizes content into predefined or dynamic categories.
func (c *Client) ClassifyContent(ctx context.Context, req *aiv1.ClassifyContentRequest) (*aiv1.ClassifyContentResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.ClassifyContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("classify content failed: %w", err)
	}

	return resp, nil
}
