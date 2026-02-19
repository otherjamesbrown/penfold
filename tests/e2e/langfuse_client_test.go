//go:build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// langfuseClient is a thin test-only HTTP client for the Langfuse REST API.
// Config from env: LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY
type langfuseClient struct {
	host      string
	publicKey string
	secretKey string
	http      *http.Client
}

// LangfuseTrace represents a Langfuse trace object from the API.
type LangfuseTrace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
	Tags      []string  `json:"tags"`
	// Input and output are present but may be arbitrary JSON.
	Input  interface{} `json:"input"`
	Output interface{} `json:"output"`
}

// LangfuseObservation represents a Langfuse observation (SPAN or GENERATION).
type LangfuseObservation struct {
	ID                  string     `json:"id"`
	TraceID             string     `json:"traceId"`
	ParentObservationID *string    `json:"parentObservationId"`
	Type                string     `json:"type"`
	Name                string     `json:"name"`
	StartTime           time.Time  `json:"startTime"`
	EndTime             *time.Time `json:"endTime"`
	Level               string     `json:"level"`
	StatusMessage       *string    `json:"statusMessage"`
	// Input/Output may be null for SPANs or present for GENERATIONs.
	Input  interface{} `json:"input"`
	Output interface{} `json:"output"`
}

// newLangfuseClient creates a langfuseClient from environment variables.
// Skips the test if required env vars are not set.
func newLangfuseClient(t *testing.T) *langfuseClient {
	t.Helper()

	host := getEnvOrDefault("LANGFUSE_HOST", "http://home-01.brown.chat:3000")
	publicKey := getEnvOrDefault("LANGFUSE_PUBLIC_KEY", "pk-lf-penfold")
	secretKey := getEnvOrDefault("LANGFUSE_SECRET_KEY", "sk-lf-penfold-secret")

	if host == "" {
		t.Skip("LANGFUSE_HOST not set - skipping Langfuse trace test")
	}

	// Use InsecureSkipVerify for dev/self-signed certs (same as gateway client).
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	return &langfuseClient{
		host:      host,
		publicKey: publicKey,
		secretKey: secretKey,
		http: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
	}
}

// authHeader returns the Basic auth header value.
func (c *langfuseClient) authHeader() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.publicKey+":"+c.secretKey))
}

// doGet performs a GET request with basic auth and decodes the JSON response into v.
func (c *langfuseClient) doGet(ctx context.Context, rawURL string, v interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("langfuse API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// fetchTracesByTag queries traces tagged with the given tag string.
// Uses GET /api/public/traces?tags=<tag>&limit=5&orderBy=timestamp.desc
func (c *langfuseClient) fetchTracesByTag(ctx context.Context, tag string) ([]LangfuseTrace, error) {
	params := url.Values{}
	params.Set("tags", tag)
	params.Set("limit", "5")
	params.Set("orderBy", "timestamp.desc")

	apiURL := fmt.Sprintf("%s/api/public/traces?%s", c.host, params.Encode())

	var result struct {
		Data []LangfuseTrace `json:"data"`
	}
	if err := c.doGet(ctx, apiURL, &result); err != nil {
		return nil, fmt.Errorf("fetchTracesByTag %q: %w", tag, err)
	}
	return result.Data, nil
}

// fetchObservations queries all observations for a trace.
// Uses GET /api/public/observations?traceId=<traceID>&limit=100
func (c *langfuseClient) fetchObservations(ctx context.Context, traceID string) ([]LangfuseObservation, error) {
	params := url.Values{}
	params.Set("traceId", traceID)
	params.Set("limit", "100")

	apiURL := fmt.Sprintf("%s/api/public/observations?%s", c.host, params.Encode())

	var result struct {
		Data []LangfuseObservation `json:"data"`
	}
	if err := c.doGet(ctx, apiURL, &result); err != nil {
		return nil, fmt.Errorf("fetchObservations traceID=%q: %w", traceID, err)
	}
	return result.Data, nil
}

// reprocessAndWait triggers pipeline reprocessing for a content item and waits
// for the pipeline to fully complete. It invokes `penf reprocess <contentID>` using
// the real penf binary found in PATH, then polls the Langfuse API until the
// "email-processing.finish" span appears in a NEW trace (created after the reprocess
// trigger), indicating all pipeline stages have run.
//
// timeout is the maximum time to wait for reprocessing to complete.
func reprocessAndWait(t *testing.T, contentID string, timeout time.Duration) {
	t.Helper()

	// Record the time before triggering reprocess so we can filter out old traces.
	triggerTime := time.Now()

	cli := NewCLIRunner(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Trigger reprocessing via CLI. The penf binary uses the default config
	// which points to the production tenant on dev02.
	result := cli.Run(ctx, "reprocess", contentID)
	if !result.Success() {
		t.Logf("reprocess stdout: %s", result.Stdout)
		t.Logf("reprocess stderr: %s", result.Stderr)
		// Not fatal — we still poll for a trace. The content may already be queued.
		t.Logf("warning: reprocess command returned exit code %d", result.ExitCode)
	} else {
		t.Logf("reprocess triggered for %s: %s", contentID, result.Stdout)
	}

	// Wait for a NEW Langfuse trace (created after triggerTime) tagged with the
	// contentID, then continue polling until the "email-processing.finish" span
	// appears (created by FinishPipelineTracing at the very end of the pipeline).
	lf := newLangfuseClient(t)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Give the pipeline a moment to start before first poll.
	time.Sleep(10 * time.Second)

	var traceID string
	for {
		select {
		case <-ctx.Done():
			if traceID != "" {
				t.Fatalf("reprocessAndWait: timeout waiting for pipeline completion (trace %s found but email-processing.finish span never appeared)", traceID)
			}
			t.Fatalf("reprocessAndWait: timeout waiting for NEW Langfuse trace for content %s (after %s)", contentID, triggerTime.Format(time.RFC3339))
		case <-ticker.C:
			// Phase 1: Find a NEW trace (created after triggerTime).
			if traceID == "" {
				traces, err := lf.fetchTracesByTag(ctx, contentID)
				if err != nil {
					t.Logf("reprocessAndWait: poll error (will retry): %v", err)
					continue
				}
				// Filter to traces created after we triggered the reprocess.
				// This prevents picking up old traces from previous pipeline runs.
				for _, tr := range traces {
					if tr.Timestamp.After(triggerTime) && tr.Name == "email-processing" {
						traceID = tr.ID
						t.Logf("reprocessAndWait: found NEW trace %s (ts=%s) for content %s, waiting for pipeline completion...",
							traceID, tr.Timestamp.Format(time.RFC3339), contentID)
						break
					}
				}
				if traceID == "" {
					t.Logf("reprocessAndWait: no NEW traces yet for content %s (checked %d, all before %s)...",
						contentID, len(traces), triggerTime.Format(time.RFC3339))
					continue
				}
			}

			// Phase 2: Wait for the finish span.
			observations, err := lf.fetchObservations(ctx, traceID)
			if err != nil {
				t.Logf("reprocessAndWait: observation poll error (will retry): %v", err)
				continue
			}
			for _, obs := range observations {
				if obs.Name == "email-processing.finish" {
					t.Logf("reprocessAndWait: pipeline complete — found email-processing.finish span (%d total observations)", len(observations))
					// Brief delay for any remaining observations to flush to Langfuse.
					time.Sleep(5 * time.Second)
					return
				}
			}
			t.Logf("reprocessAndWait: trace %s has %d observations but no finish span yet...", traceID, len(observations))
		}
	}
}

// langfuseHostFromEnv returns the LANGFUSE_HOST env var for display/debug.
func langfuseHostFromEnv() string {
	if h := os.Getenv("LANGFUSE_HOST"); h != "" {
		return h
	}
	return "http://home-01.brown.chat:3000"
}
