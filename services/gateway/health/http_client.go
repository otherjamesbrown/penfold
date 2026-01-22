// Package health provides health check clients for the API Gateway.
package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPHealthClient checks health of services via HTTP endpoints.
type HTTPHealthClient struct {
	baseURL    string
	endpoint   string
	httpClient *http.Client
}

// NewHTTPHealthClient creates a new HTTP health client.
func NewHTTPHealthClient(baseURL, endpoint string, timeout time.Duration) *HTTPHealthClient {
	return &HTTPHealthClient{
		baseURL:  baseURL,
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// HealthCheck performs a health check by making an HTTP GET request.
func (c *HTTPHealthClient) HealthCheck(ctx context.Context) error {
	url := c.baseURL + c.endpoint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body to ensure connection is properly closed
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unhealthy: HTTP %d", resp.StatusCode)
	}

	return nil
}

// DatabaseHealthClient checks PostgreSQL health via pgxpool.
type DatabaseHealthClient struct {
	pingFunc func(ctx context.Context) error
}

// NewDatabaseHealthClient creates a database health client.
func NewDatabaseHealthClient(pingFunc func(ctx context.Context) error) *DatabaseHealthClient {
	return &DatabaseHealthClient{pingFunc: pingFunc}
}

// HealthCheck pings the database.
func (c *DatabaseHealthClient) HealthCheck(ctx context.Context) error {
	return c.pingFunc(ctx)
}
