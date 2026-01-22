//go:build live

package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeminiAPIConnection verifies connectivity to the Gemini API.
func TestGeminiAPIConnection(t *testing.T) {
	apiKey := RequireGeminiAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List available models
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "Gemini API should be accessible")

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	t.Logf("Found %d Gemini models", len(result.Models))
	for i, m := range result.Models {
		if i < 5 {
			t.Logf("  - %s", m.Name)
		}
	}
}

// TestGeminiBasicCompletion tests a simple Gemini API completion.
func TestGeminiBasicCompletion(t *testing.T) {
	apiKey := RequireGeminiAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use gemini-pro model
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", apiKey)

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": "What is 2 + 2? Answer with just the number."},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature": 0.0,
			"maxOutputTokens": 100,
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Gemini API call should succeed: %s", string(respBody))

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	require.NotEmpty(t, result.Candidates, "should have at least one candidate")
	require.NotEmpty(t, result.Candidates[0].Content.Parts, "should have at least one part")

	response := result.Candidates[0].Content.Parts[0].Text
	t.Logf("Gemini response: %s", response)
	assert.Contains(t, response, "4", "response should contain the answer '4'")
}

// TestGeminiEmbeddings tests the Gemini embeddings API.
func TestGeminiEmbeddings(t *testing.T) {
	apiKey := RequireGeminiAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/embedding-001:embedContent?key=%s", apiKey)

	reqBody := map[string]any{
		"model": "models/embedding-001",
		"content": map[string]any{
			"parts": []map[string]string{
				{"text": "This is a test document for embedding generation."},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	require.Equal(t, http.StatusOK, resp.StatusCode, "Embeddings API should succeed: %s", string(respBody))

	var result struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err)

	t.Logf("Embedding dimension: %d", len(result.Embedding.Values))
	assert.NotEmpty(t, result.Embedding.Values, "should have embedding values")
	assert.Equal(t, 768, len(result.Embedding.Values), "embedding-001 should return 768 dimensions")
}
