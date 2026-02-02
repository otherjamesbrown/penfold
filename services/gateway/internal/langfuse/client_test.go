package langfuse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient("https://example.com", "pk_test", "sk_test")

	assert.NotNil(t, client)
	assert.Equal(t, "https://example.com", client.host)
	assert.Equal(t, "pk_test", client.publicKey)
	assert.Equal(t, "sk_test", client.secretKey)
	assert.NotNil(t, client.http)
}

func TestGetTracesByContentID(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		// Create mock server
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/public/traces", r.URL.Path)
			assert.Contains(t, r.URL.RawQuery, "filter=")
			assert.Contains(t, r.URL.RawQuery, "penfold.content_id")

			// Return mock traces
			response := map[string]interface{}{
				"data": []Trace{
					{
						ID:        "trace-1",
						Name:      "Content Processing",
						Timestamp: time.Now(),
						Metadata: map[string]interface{}{
							"penfold.content_id": "test-content-id",
						},
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer mockServer.Close()

		client := NewClient(mockServer.URL, "pk_test", "sk_test")
		traces, err := client.GetTracesByContentID(context.Background(), "test-content-id", "")

		require.NoError(t, err)
		assert.Len(t, traces, 1)
		assert.Equal(t, "trace-1", traces[0].ID)
		assert.Equal(t, "Content Processing", traces[0].Name)
	})

	t.Run("WithEnvironment", func(t *testing.T) {
		// Create mock server
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.RawQuery, "environment")
			assert.Contains(t, r.URL.RawQuery, "production")

			response := map[string]interface{}{
				"data": []Trace{},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer mockServer.Close()

		client := NewClient(mockServer.URL, "pk_test", "sk_test")
		traces, err := client.GetTracesByContentID(context.Background(), "test-content-id", "production")

		require.NoError(t, err)
		assert.Len(t, traces, 0)
	})

	t.Run("NotConfigured", func(t *testing.T) {
		client := NewClient("", "", "")
		_, err := client.GetTracesByContentID(context.Background(), "test-content-id", "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("APIError", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		}))
		defer mockServer.Close()

		client := NewClient(mockServer.URL, "pk_test", "sk_test")
		_, err := client.GetTracesByContentID(context.Background(), "test-content-id", "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})
}

func TestGetObservations(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/public/v2/observations", r.URL.Path)
			assert.Contains(t, r.URL.RawQuery, "traceId=trace-1")
			assert.Contains(t, r.URL.RawQuery, "fields=core,basic,usage")

			promptTokens := 100
			completionTokens := 200
			totalTokens := 300

			response := map[string]interface{}{
				"data": []Observation{
					{
						ID:               "obs-1",
						TraceID:          "trace-1",
						Type:             "GENERATION",
						Name:             "Generate Summary",
						StartTime:        time.Now(),
						PromptTokens:     &promptTokens,
						CompletionTokens: &completionTokens,
						TotalTokens:      &totalTokens,
						Level:            "DEFAULT",
					},
				},
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}))
		defer mockServer.Close()

		client := NewClient(mockServer.URL, "pk_test", "sk_test")
		observations, err := client.GetObservations(context.Background(), "trace-1")

		require.NoError(t, err)
		assert.Len(t, observations, 1)
		assert.Equal(t, "obs-1", observations[0].ID)
		assert.Equal(t, "GENERATION", observations[0].Type)
		assert.Equal(t, "Generate Summary", observations[0].Name)
		assert.NotNil(t, observations[0].PromptTokens)
		assert.Equal(t, 100, *observations[0].PromptTokens)
	})

	t.Run("NotConfigured", func(t *testing.T) {
		client := NewClient("", "", "")
		_, err := client.GetObservations(context.Background(), "trace-1")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})
}

func TestBuildFilterURL(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		client := NewClient("https://langfuse.example.com", "pk_test", "sk_test")
		url := client.BuildFilterURL("test-content-id")

		assert.Contains(t, url, "https://langfuse.example.com/traces")
		assert.Contains(t, url, "filter=")
		assert.Contains(t, url, "penfold.content_id")
		assert.Contains(t, url, "test-content-id")
	})

	t.Run("NotConfigured", func(t *testing.T) {
		client := NewClient("", "pk_test", "sk_test")
		url := client.BuildFilterURL("test-content-id")

		assert.Equal(t, "", url)
	})
}
