package langfuse_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/langfuse"
)

func TestConfigFromEnv(t *testing.T) {
	// Save and restore env
	origHost := os.Getenv("LANGFUSE_HOST")
	origPK := os.Getenv("LANGFUSE_PUBLIC_KEY")
	origSK := os.Getenv("LANGFUSE_SECRET_KEY")
	defer func() {
		os.Setenv("LANGFUSE_HOST", origHost)
		os.Setenv("LANGFUSE_PUBLIC_KEY", origPK)
		os.Setenv("LANGFUSE_SECRET_KEY", origSK)
	}()

	// Test with no env vars
	os.Unsetenv("LANGFUSE_HOST")
	os.Unsetenv("LANGFUSE_PUBLIC_KEY")
	os.Unsetenv("LANGFUSE_SECRET_KEY")

	cfg := langfuse.ConfigFromEnv()
	if cfg != nil {
		t.Error("expected nil config when no env vars set")
	}

	// Test with partial env vars
	os.Setenv("LANGFUSE_HOST", "http://localhost:3000")
	cfg = langfuse.ConfigFromEnv()
	if cfg != nil {
		t.Error("expected nil config when only host set")
	}

	// Test with all env vars
	os.Setenv("LANGFUSE_HOST", "http://localhost:3000")
	os.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test")
	os.Setenv("LANGFUSE_SECRET_KEY", "sk-test")

	cfg = langfuse.ConfigFromEnv()
	if cfg == nil {
		t.Fatal("expected config when all env vars set")
	}
	if cfg.Host != "http://localhost:3000" {
		t.Errorf("expected host 'http://localhost:3000', got '%s'", cfg.Host)
	}
	if cfg.PublicKey != "pk-test" {
		t.Errorf("expected public key 'pk-test', got '%s'", cfg.PublicKey)
	}
	if cfg.SecretKey != "sk-test" {
		t.Errorf("expected secret key 'sk-test', got '%s'", cfg.SecretKey)
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *langfuse.Config
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name:    "missing host",
			cfg:     &langfuse.Config{PublicKey: "pk", SecretKey: "sk"},
			wantErr: true,
		},
		{
			name:    "missing public key",
			cfg:     &langfuse.Config{Host: "http://localhost", SecretKey: "sk"},
			wantErr: true,
		},
		{
			name:    "missing secret key",
			cfg:     &langfuse.Config{Host: "http://localhost", PublicKey: "pk"},
			wantErr: true,
		},
		{
			name:    "valid config",
			cfg:     &langfuse.Config{Host: "http://localhost", PublicKey: "pk", SecretKey: "sk"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := langfuse.NewClient(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if client == nil {
				t.Error("expected client, got nil")
			}
		})
	}
}

func TestCreateDataset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/public/v2/datasets" {
			t.Errorf("expected path /api/public/v2/datasets, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Parse request body
		var req langfuse.CreateDatasetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Name != "test-dataset" {
			t.Errorf("expected name 'test-dataset', got '%s'", req.Name)
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(langfuse.Dataset{
			ID:          "ds-123",
			Name:        req.Name,
			Description: req.Description,
		})
	}))
	defer server.Close()

	client, err := langfuse.NewClient(&langfuse.Config{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	dataset, err := client.CreateDataset(context.Background(), &langfuse.CreateDatasetRequest{
		Name:        "test-dataset",
		Description: "Test description",
	})
	if err != nil {
		t.Fatalf("failed to create dataset: %v", err)
	}

	if dataset.ID != "ds-123" {
		t.Errorf("expected ID 'ds-123', got '%s'", dataset.ID)
	}
	if dataset.Name != "test-dataset" {
		t.Errorf("expected name 'test-dataset', got '%s'", dataset.Name)
	}
}

func TestCreateDatasetItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/public/dataset-items" {
			t.Errorf("expected path /api/public/dataset-items, got %s", r.URL.Path)
		}

		var req langfuse.CreateDatasetItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(langfuse.DatasetItem{
			ID:          "item-456",
			DatasetName: req.DatasetName,
			Input:       req.Input,
		})
	}))
	defer server.Close()

	client, _ := langfuse.NewClient(&langfuse.Config{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})

	item, err := client.CreateDatasetItem(context.Background(), &langfuse.CreateDatasetItemRequest{
		DatasetName: "test-dataset",
		Input:       map[string]string{"text": "hello world"},
		Metadata:    map[string]interface{}{"source": "test"},
	})
	if err != nil {
		t.Fatalf("failed to create dataset item: %v", err)
	}

	if item.ID != "item-456" {
		t.Errorf("expected ID 'item-456', got '%s'", item.ID)
	}
}

func TestCreateDatasetRunItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/public/dataset-run-items" {
			t.Errorf("expected path /api/public/dataset-run-items, got %s", r.URL.Path)
		}

		var req langfuse.CreateDatasetRunItemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(langfuse.DatasetRunItem{
			ID:            "run-item-789",
			DatasetItemID: req.DatasetItemID,
			TraceID:       req.TraceID,
		})
	}))
	defer server.Close()

	client, _ := langfuse.NewClient(&langfuse.Config{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})

	runItem, err := client.CreateDatasetRunItem(context.Background(), &langfuse.CreateDatasetRunItemRequest{
		DatasetItemID: "item-456",
		TraceID:       "trace-abc",
		RunName:       "phi-summarization",
		RunDescription: "Benchmark run with phi model",
	})
	if err != nil {
		t.Fatalf("failed to create dataset run item: %v", err)
	}

	if runItem.ID != "run-item-789" {
		t.Errorf("expected ID 'run-item-789', got '%s'", runItem.ID)
	}
	if runItem.TraceID != "trace-abc" {
		t.Errorf("expected traceID 'trace-abc', got '%s'", runItem.TraceID)
	}
}

func TestGetDataset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/public/v2/datasets/my-dataset" {
			t.Errorf("expected path /api/public/v2/datasets/my-dataset, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(langfuse.Dataset{
			ID:   "ds-123",
			Name: "my-dataset",
		})
	}))
	defer server.Close()

	client, _ := langfuse.NewClient(&langfuse.Config{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})

	dataset, err := client.GetDataset(context.Background(), "my-dataset")
	if err != nil {
		t.Fatalf("failed to get dataset: %v", err)
	}

	if dataset.Name != "my-dataset" {
		t.Errorf("expected name 'my-dataset', got '%s'", dataset.Name)
	}
}

func TestGetDatasetRuns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/public/datasets/my-dataset/runs" {
			t.Errorf("expected path /api/public/datasets/my-dataset/runs, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(langfuse.DatasetRunsResponse{
			Data: []langfuse.DatasetRun{
				{ID: "run-1", Name: "phi-test"},
				{ID: "run-2", Name: "qwen-test"},
			},
		})
	}))
	defer server.Close()

	client, _ := langfuse.NewClient(&langfuse.Config{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})

	runs, err := client.GetDatasetRuns(context.Background(), "my-dataset")
	if err != nil {
		t.Fatalf("failed to get dataset runs: %v", err)
	}

	if len(runs) != 2 {
		t.Errorf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].Name != "phi-test" {
		t.Errorf("expected first run name 'phi-test', got '%s'", runs[0].Name)
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "Dataset name is required"}`))
	}))
	defer server.Close()

	client, _ := langfuse.NewClient(&langfuse.Config{
		Host:      server.URL,
		PublicKey: "pk-test",
		SecretKey: "sk-test",
	})

	_, err := client.CreateDataset(context.Background(), &langfuse.CreateDatasetRequest{})
	if err == nil {
		t.Error("expected error for bad request")
	}
}
