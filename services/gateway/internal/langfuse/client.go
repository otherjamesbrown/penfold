// Package langfuse provides a client for interacting with the Langfuse API.
package langfuse

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client provides access to Langfuse API
type Client struct {
	host      string
	publicKey string
	secretKey string
	http      *http.Client
}

// Trace represents a Langfuse trace with basic information.
type Trace struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// Observation represents a Langfuse observation (generation, span, etc).
type Observation struct {
	ID           string     `json:"id"`
	TraceID      string     `json:"traceId"`
	Type         string     `json:"type"`
	Name         string     `json:"name"`
	StartTime    time.Time  `json:"startTime"`
	EndTime      *time.Time `json:"endTime,omitempty"`
	Model        *string    `json:"model,omitempty"`
	PromptTokens *int       `json:"promptTokens,omitempty"`
	UsageDetails struct {
		Input  *int `json:"input,omitempty"`
		Output *int `json:"output,omitempty"`
		Total  *int `json:"total,omitempty"`
	} `json:"usage,omitempty"`
	CompletionTokens *int   `json:"completionTokens,omitempty"`
	TotalTokens      *int   `json:"totalTokens,omitempty"`
	Level            string `json:"level"`
	StatusMessage    *string `json:"statusMessage,omitempty"`
}

// NewClient creates a new Langfuse API client.
func NewClient(host, publicKey, secretKey string) *Client {
	// Create HTTP client with TLS skip verification (for self-signed certs in dev)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}

	return &Client{
		host:      host,
		publicKey: publicKey,
		secretKey: secretKey,
		http:      httpClient,
	}
}

// GetTracesByContentID queries traces by penfold.content_id metadata.
func (c *Client) GetTracesByContentID(ctx context.Context, contentID, environment string) ([]Trace, error) {
	if c.host == "" || c.publicKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("langfuse not configured")
	}

	// Build filter for metadata query
	// Langfuse API expects filter=[{"type":"stringObject","column":"metadata","key":"penfold.content_id","operator":"=","value":"<id>"}]
	filter := fmt.Sprintf(`[{"type":"stringObject","column":"metadata","key":"penfold.content_id","operator":"=","value":"%s"}]`, contentID)

	// Add environment filter if specified
	if environment != "" {
		filter = fmt.Sprintf(`[{"type":"stringObject","column":"metadata","key":"penfold.content_id","operator":"=","value":"%s"},{"type":"stringObject","column":"metadata","key":"environment","operator":"=","value":"%s"}]`, contentID, environment)
	}

	// Build URL
	apiURL := fmt.Sprintf("%s/api/public/traces?filter=%s", c.host, url.QueryEscape(filter))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Add basic auth
	auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("langfuse API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Data []Trace `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.Data, nil
}

// GetObservations gets observations for a trace.
func (c *Client) GetObservations(ctx context.Context, traceID string) ([]Observation, error) {
	if c.host == "" || c.publicKey == "" || c.secretKey == "" {
		return nil, fmt.Errorf("langfuse not configured")
	}

	// Build URL with fields parameter for detailed usage data
	apiURL := fmt.Sprintf("%s/api/public/v2/observations?traceId=%s&fields=core,basic,usage", c.host, url.QueryEscape(traceID))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	// Add basic auth
	auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("langfuse API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		Data []Observation `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.Data, nil
}

// BuildFilterURL builds the Langfuse UI URL with filters for a content ID.
func (c *Client) BuildFilterURL(contentID string) string {
	if c.host == "" {
		return ""
	}

	// Build filter JSON for UI (metadata filter)
	// Format: /project/<project>/traces?filter=[{"type":"stringObject","column":"metadata","key":"penfold.content_id","operator":"=","value":"<id>"}]
	filter := fmt.Sprintf(`[{"type":"stringObject","column":"metadata","key":"penfold.content_id","operator":"=","value":"%s"}]`, contentID)
	encodedFilter := url.QueryEscape(filter)

	// Note: This assumes default project. In production, you may need to extract project ID from config
	return fmt.Sprintf("%s/traces?filter=%s", c.host, encodedFilter)
}
