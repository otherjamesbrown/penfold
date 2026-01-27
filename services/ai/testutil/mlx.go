// Package testutil provides test helpers for the AI service.
package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// DefaultMLXURL is the default URL for the MLX LLM server.
const DefaultMLXURL = "http://localhost:8080"

// MLXModels holds discovered MLX model information.
type MLXModels struct {
	Models []string
	url    string
}

// modelsResponse represents the OpenAI-compatible /v1/models response.
type modelsResponse struct {
	Object string       `json:"object"`
	Data   []modelEntry `json:"data"`
}

type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// DiscoverMLXModels queries the MLX server at the default URL and returns
// available models. Connection errors are handled gracefully by returning
// an empty model list rather than an error.
func DiscoverMLXModels() *MLXModels {
	return DiscoverMLXModelsAt(DefaultMLXURL)
}

// DiscoverMLXModelsAt queries the MLX server at the specified URL and returns
// available models. Connection errors are handled gracefully by returning
// an empty model list rather than an error.
func DiscoverMLXModelsAt(baseURL string) *MLXModels {
	m := &MLXModels{
		url:    baseURL,
		Models: []string{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := baseURL + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return m
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return m
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return m
	}

	var modelsResp modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return m
	}

	for _, entry := range modelsResp.Data {
		if entry.ID != "" {
			m.Models = append(m.Models, entry.ID)
		}
	}

	return m
}

// Available returns true if at least one MLX model was discovered.
func (m *MLXModels) Available() bool {
	return len(m.Models) > 0
}

// GetFirst returns the first available model ID, or an empty string if none.
func (m *MLXModels) GetFirst() string {
	if len(m.Models) == 0 {
		return ""
	}
	return m.Models[0]
}

// Has returns true if the specified model ID is available.
func (m *MLXModels) Has(modelID string) bool {
	for _, id := range m.Models {
		if id == modelID {
			return true
		}
	}
	return false
}

// URL returns the base URL that was queried for models.
func (m *MLXModels) URL() string {
	return m.url
}
