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

// TriageContent classifies content into categories and importance levels.
func (c *Client) TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.TriageContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("triage content failed: %w", err)
	}

	return resp, nil
}

// ExtractEntities performs two-pass entity extraction from content.
func (c *Client) ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.ExtractEntities(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("extract entities failed: %w", err)
	}

	return resp, nil
}

// DeepAnalyze performs Stage 4 deep analysis using a remote LLM.
func (c *Client) DeepAnalyze(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured (or use extended timeout for deep analysis)
	timeout := c.options.requestTimeout
	if timeout == 0 || timeout < 60*time.Second {
		timeout = 60 * time.Second // Deep analysis may take longer
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	resp, err := c.client.DeepAnalyze(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("deep analyze failed: %w", err)
	}

	return resp, nil
}

// ListModels returns all registered models with optional filtering.
func (c *Client) ListModels(ctx context.Context, req *aiv1.ListModelsRequest) (*aiv1.ListModelsResponse, error) {
	if req == nil {
		req = &aiv1.ListModelsRequest{}
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.ListModels(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list models failed: %w", err)
	}

	return resp, nil
}

// RegisterModel adds a new model to the registry.
func (c *Client) RegisterModel(ctx context.Context, req *aiv1.RegisterModelRequest) (*aiv1.RegisterModelResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.RegisterModel(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("register model failed: %w", err)
	}

	return resp, nil
}

// UpdateModel modifies an existing model's configuration.
func (c *Client) UpdateModel(ctx context.Context, req *aiv1.UpdateModelRequest) (*aiv1.UpdateModelResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.UpdateModel(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("update model failed: %w", err)
	}

	return resp, nil
}

// DeleteModel removes a model from the registry.
func (c *Client) DeleteModel(ctx context.Context, req *aiv1.DeleteModelRequest) (*aiv1.DeleteModelResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.DeleteModel(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("delete model failed: %w", err)
	}

	return resp, nil
}

// GetRoutingRules returns routing rules with optional filtering.
func (c *Client) GetRoutingRules(ctx context.Context, req *aiv1.GetRoutingRulesRequest) (*aiv1.GetRoutingRulesResponse, error) {
	if req == nil {
		req = &aiv1.GetRoutingRulesRequest{}
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.GetRoutingRules(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get routing rules failed: %w", err)
	}

	return resp, nil
}

// UpdateRoutingRule creates or updates a routing rule.
func (c *Client) UpdateRoutingRule(ctx context.Context, req *aiv1.UpdateRoutingRuleRequest) (*aiv1.UpdateRoutingRuleResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}

	// Apply request timeout if configured
	if c.options.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.options.requestTimeout)
		defer cancel()
	}

	resp, err := c.client.UpdateRoutingRule(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("update routing rule failed: %w", err)
	}

	return resp, nil
}
