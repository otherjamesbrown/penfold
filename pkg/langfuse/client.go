// Package langfuse provides a client for the Langfuse Datasets API.
// This enables creating datasets, adding items, and recording benchmark results.
package langfuse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client provides access to the Langfuse Datasets API.
type Client struct {
	host       string
	publicKey  string
	secretKey  string
	httpClient *http.Client
}

// Config holds configuration for the Langfuse client.
type Config struct {
	Host      string
	PublicKey string
	SecretKey string
}

// ConfigFromEnv creates a Config from environment variables.
// Reads LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, and LANGFUSE_SECRET_KEY.
func ConfigFromEnv() *Config {
	host := os.Getenv("LANGFUSE_HOST")
	publicKey := os.Getenv("LANGFUSE_PUBLIC_KEY")
	secretKey := os.Getenv("LANGFUSE_SECRET_KEY")

	if host == "" || publicKey == "" || secretKey == "" {
		return nil
	}

	return &Config{
		Host:      host,
		PublicKey: publicKey,
		SecretKey: secretKey,
	}
}

// NewClient creates a new Langfuse client.
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if cfg.PublicKey == "" {
		return nil, fmt.Errorf("public key is required")
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("secret key is required")
	}

	return &Client{
		host:      cfg.Host,
		publicKey: cfg.PublicKey,
		secretKey: cfg.SecretKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// NewClientFromEnv creates a new Langfuse client from environment variables.
func NewClientFromEnv() (*Client, error) {
	cfg := ConfigFromEnv()
	if cfg == nil {
		return nil, fmt.Errorf("LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, and LANGFUSE_SECRET_KEY must be set")
	}
	return NewClient(cfg)
}

// authHeader returns the Basic auth header value.
func (c *Client) authHeader() string {
	credentials := c.publicKey + ":" + c.secretKey
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
	return "Basic " + encoded
}

// doRequest performs an HTTP request with authentication.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	url := c.host + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// parseResponse reads and unmarshals the response body.
func parseResponse[T any](resp *http.Response) (*T, error) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	return &result, nil
}

// Dataset represents a Langfuse dataset.
type Dataset struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ProjectID   string                 `json:"projectId,omitempty"`
	CreatedAt   time.Time              `json:"createdAt,omitempty"`
	UpdatedAt   time.Time              `json:"updatedAt,omitempty"`
}

// CreateDatasetRequest is the request body for creating a dataset.
type CreateDatasetRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CreateDataset creates a new dataset.
func (c *Client) CreateDataset(ctx context.Context, req *CreateDatasetRequest) (*Dataset, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/public/v2/datasets", req)
	if err != nil {
		return nil, err
	}
	return parseResponse[Dataset](resp)
}

// GetDataset retrieves a dataset by name.
func (c *Client) GetDataset(ctx context.Context, name string) (*Dataset, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/public/v2/datasets/"+name, nil)
	if err != nil {
		return nil, err
	}
	return parseResponse[Dataset](resp)
}

// DatasetItem represents an item in a dataset.
type DatasetItem struct {
	ID             string                 `json:"id"`
	DatasetID      string                 `json:"datasetId,omitempty"`
	DatasetName    string                 `json:"datasetName,omitempty"`
	Input          interface{}            `json:"input"`
	ExpectedOutput interface{}            `json:"expectedOutput,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	SourceTraceID  string                 `json:"sourceTraceId,omitempty"`
	Status         string                 `json:"status,omitempty"`
	CreatedAt      time.Time              `json:"createdAt,omitempty"`
	UpdatedAt      time.Time              `json:"updatedAt,omitempty"`
}

// CreateDatasetItemRequest is the request body for creating a dataset item.
type CreateDatasetItemRequest struct {
	DatasetName    string                 `json:"datasetName"`
	Input          interface{}            `json:"input"`
	ExpectedOutput interface{}            `json:"expectedOutput,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	ID             string                 `json:"id,omitempty"`
	SourceTraceID  string                 `json:"sourceTraceId,omitempty"`
	Status         string                 `json:"status,omitempty"`
}

// CreateDatasetItem creates a new item in a dataset.
func (c *Client) CreateDatasetItem(ctx context.Context, req *CreateDatasetItemRequest) (*DatasetItem, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/public/dataset-items", req)
	if err != nil {
		return nil, err
	}
	return parseResponse[DatasetItem](resp)
}

// DatasetRunItem represents a run result linked to a dataset item.
type DatasetRunItem struct {
	ID            string                 `json:"id"`
	DatasetItemID string                 `json:"datasetItemId"`
	DatasetRunID  string                 `json:"datasetRunId,omitempty"`
	TraceID       string                 `json:"traceId"`
	ObservationID string                 `json:"observationId,omitempty"`
	CreatedAt     time.Time              `json:"createdAt,omitempty"`
	UpdatedAt     time.Time              `json:"updatedAt,omitempty"`
}

// CreateDatasetRunItemRequest is the request body for recording a run result.
type CreateDatasetRunItemRequest struct {
	DatasetItemID   string                 `json:"datasetItemId"`
	TraceID         string                 `json:"traceId"`
	ObservationID   string                 `json:"observationId,omitempty"`
	RunName         string                 `json:"runName"`
	RunDescription  string                 `json:"runDescription,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// CreateDatasetRunItem records a run result linked to a dataset item and trace.
func (c *Client) CreateDatasetRunItem(ctx context.Context, req *CreateDatasetRunItemRequest) (*DatasetRunItem, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/public/dataset-run-items", req)
	if err != nil {
		return nil, err
	}
	return parseResponse[DatasetRunItem](resp)
}

// DatasetRun represents a run of a dataset (collection of run items).
type DatasetRun struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	DatasetID   string                 `json:"datasetId"`
	DatasetName string                 `json:"datasetName,omitempty"`
	CreatedAt   time.Time              `json:"createdAt,omitempty"`
	UpdatedAt   time.Time              `json:"updatedAt,omitempty"`
}

// DatasetRunsResponse is the response for listing dataset runs.
type DatasetRunsResponse struct {
	Data []DatasetRun `json:"data"`
}

// GetDatasetRuns retrieves all runs for a dataset.
func (c *Client) GetDatasetRuns(ctx context.Context, datasetName string) ([]DatasetRun, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/public/datasets/"+datasetName+"/runs", nil)
	if err != nil {
		return nil, err
	}
	result, err := parseResponse[DatasetRunsResponse](resp)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// Host returns the configured Langfuse host URL.
func (c *Client) Host() string {
	return c.host
}
