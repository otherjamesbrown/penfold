package embeddings

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("expected ServerURL %q, got %q", DefaultServerURL, cfg.ServerURL)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("expected Model %q, got %q", DefaultModel, cfg.Model)
	}
	if cfg.BatchSize != DefaultBatchSize {
		t.Errorf("expected BatchSize %d, got %d", DefaultBatchSize, cfg.BatchSize)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("expected Timeout %v, got %v", DefaultTimeout, cfg.Timeout)
	}
	if cfg.Dimensions != DefaultDimensions {
		t.Errorf("expected Dimensions %d, got %d", DefaultDimensions, cfg.Dimensions)
	}
	if cfg.MaxRetries != DefaultMaxRetries {
		t.Errorf("expected MaxRetries %d, got %d", DefaultMaxRetries, cfg.MaxRetries)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save and restore environment variables
	envVars := map[string]string{
		EnvServerURL:  os.Getenv(EnvServerURL),
		EnvModel:      os.Getenv(EnvModel),
		EnvBatchSize:  os.Getenv(EnvBatchSize),
		EnvTimeout:    os.Getenv(EnvTimeout),
		EnvDimensions: os.Getenv(EnvDimensions),
		EnvMaxRetries: os.Getenv(EnvMaxRetries),
	}
	defer func() {
		for k, v := range envVars {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()

	// Clear all env vars first
	for k := range envVars {
		_ = os.Unsetenv(k)
	}

	t.Run("defaults when no env vars", func(t *testing.T) {
		cfg := LoadFromEnv()
		if cfg.ServerURL != DefaultServerURL {
			t.Errorf("expected default ServerURL")
		}
	})

	t.Run("overrides from env vars", func(t *testing.T) {
		_ = os.Setenv(EnvServerURL, "http://custom:8080")
		_ = os.Setenv(EnvModel, "custom-model")
		_ = os.Setenv(EnvBatchSize, "64")
		_ = os.Setenv(EnvTimeout, "60")
		_ = os.Setenv(EnvDimensions, "512")
		_ = os.Setenv(EnvMaxRetries, "5")

		cfg := LoadFromEnv()

		if cfg.ServerURL != "http://custom:8080" {
			t.Errorf("expected ServerURL %q, got %q", "http://custom:8080", cfg.ServerURL)
		}
		if cfg.Model != "custom-model" {
			t.Errorf("expected Model %q, got %q", "custom-model", cfg.Model)
		}
		if cfg.BatchSize != 64 {
			t.Errorf("expected BatchSize 64, got %d", cfg.BatchSize)
		}
		if cfg.Timeout != 60*time.Second {
			t.Errorf("expected Timeout 60s, got %v", cfg.Timeout)
		}
		if cfg.Dimensions != 512 {
			t.Errorf("expected Dimensions 512, got %d", cfg.Dimensions)
		}
		if cfg.MaxRetries != 5 {
			t.Errorf("expected MaxRetries 5, got %d", cfg.MaxRetries)
		}
	})

	t.Run("ignores invalid values", func(t *testing.T) {
		_ = os.Setenv(EnvBatchSize, "invalid")
		_ = os.Setenv(EnvTimeout, "invalid")
		_ = os.Setenv(EnvDimensions, "-5")
		_ = os.Setenv(EnvMaxRetries, "-1")

		cfg := LoadFromEnv()

		if cfg.BatchSize != DefaultBatchSize {
			t.Errorf("expected default BatchSize, got %d", cfg.BatchSize)
		}
		if cfg.Timeout != DefaultTimeout {
			t.Errorf("expected default Timeout, got %v", cfg.Timeout)
		}
	})
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr error
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: nil,
		},
		{
			name: "valid config without server URL (gRPC client)",
			config: &Config{
				Model:      DefaultModel,
				BatchSize:  DefaultBatchSize,
				Timeout:    DefaultTimeout,
				Dimensions: DefaultDimensions,
			},
			wantErr: nil, // ServerURL is no longer required for AIClient-backed clients
		},
		{
			name: "empty model",
			config: &Config{
				ServerURL:  DefaultServerURL,
				BatchSize:  DefaultBatchSize,
				Timeout:    DefaultTimeout,
				Dimensions: DefaultDimensions,
			},
			wantErr: ErrInvalidModel,
		},
		{
			name: "zero batch size",
			config: &Config{
				ServerURL:  DefaultServerURL,
				Model:      DefaultModel,
				BatchSize:  0,
				Timeout:    DefaultTimeout,
				Dimensions: DefaultDimensions,
			},
			wantErr: ErrInvalidBatchSize,
		},
		{
			name: "negative dimensions",
			config: &Config{
				ServerURL:  DefaultServerURL,
				Model:      DefaultModel,
				BatchSize:  DefaultBatchSize,
				Timeout:    DefaultTimeout,
				Dimensions: -1,
			},
			wantErr: ErrInvalidDimensions,
		},
		{
			name: "zero timeout",
			config: &Config{
				ServerURL:  DefaultServerURL,
				Model:      DefaultModel,
				BatchSize:  DefaultBatchSize,
				Timeout:    0,
				Dimensions: DefaultDimensions,
			},
			wantErr: ErrInvalidTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetModelInfo(t *testing.T) {
	cfg := DefaultConfig()
	info := cfg.GetModelInfo()

	if info.Name != cfg.Model {
		t.Errorf("expected Name %q, got %q", cfg.Model, info.Name)
	}
	if info.Dimensions != cfg.Dimensions {
		t.Errorf("expected Dimensions %d, got %d", cfg.Dimensions, info.Dimensions)
	}
	if !info.IsLocal {
		t.Error("expected IsLocal to be true")
	}
	if info.Provider != "mlx" {
		t.Errorf("expected Provider %q, got %q", "mlx", info.Provider)
	}
}
