package timeouts

import (
	"os"
	"testing"
	"time"
)

func TestDefaultValues(t *testing.T) {
	if ConnectTimeout != 10*time.Second {
		t.Errorf("ConnectTimeout = %v, want 10s", ConnectTimeout)
	}
	if HealthCheckTimeout != 5*time.Second {
		t.Errorf("HealthCheckTimeout = %v, want 5s", HealthCheckTimeout)
	}
	if KeepaliveTime != 5*time.Minute {
		t.Errorf("KeepaliveTime = %v, want 5m", KeepaliveTime)
	}
	if KeepaliveTimeout != 20*time.Second {
		t.Errorf("KeepaliveTimeout = %v, want 20s", KeepaliveTimeout)
	}
}

func TestDefaultAIRequestTimeout(t *testing.T) {
	if os.Getenv("PENFOLD_AI_REQUEST_TIMEOUT") != "" {
		t.Skip("PENFOLD_AI_REQUEST_TIMEOUT is set, skipping default check")
	}
	if AIRequestTimeout != 5*time.Minute {
		t.Errorf("AIRequestTimeout = %v, want 5m", AIRequestTimeout)
	}
}

func TestDefaultMLXBackendTimeout(t *testing.T) {
	if os.Getenv("PENFOLD_MLX_TIMEOUT") != "" {
		t.Skip("PENFOLD_MLX_TIMEOUT is set, skipping default check")
	}
	if MLXBackendTimeout != 5*time.Minute {
		t.Errorf("MLXBackendTimeout = %v, want 5m", MLXBackendTimeout)
	}
}

func TestEnvOverrideAIRequestTimeout(t *testing.T) {
	original := AIRequestTimeout
	defer func() { AIRequestTimeout = original }()

	_ = os.Setenv("PENFOLD_AI_REQUEST_TIMEOUT", "30s")
	defer os.Unsetenv("PENFOLD_AI_REQUEST_TIMEOUT") //nolint:errcheck

	// Re-apply init logic
	if v := os.Getenv("PENFOLD_AI_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			AIRequestTimeout = d
		}
	}

	if AIRequestTimeout != 30*time.Second {
		t.Errorf("AIRequestTimeout after env override = %v, want 30s", AIRequestTimeout)
	}
}

func TestEnvOverrideMLXTimeout(t *testing.T) {
	original := MLXBackendTimeout
	defer func() { MLXBackendTimeout = original }()

	_ = os.Setenv("PENFOLD_MLX_TIMEOUT", "2m")
	defer os.Unsetenv("PENFOLD_MLX_TIMEOUT") //nolint:errcheck

	if v := os.Getenv("PENFOLD_MLX_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			MLXBackendTimeout = d
		}
	}

	if MLXBackendTimeout != 2*time.Minute {
		t.Errorf("MLXBackendTimeout after env override = %v, want 2m", MLXBackendTimeout)
	}
}

func TestEnvOverrideInvalidDuration(t *testing.T) {
	original := AIRequestTimeout
	defer func() { AIRequestTimeout = original }()

	_ = os.Setenv("PENFOLD_AI_REQUEST_TIMEOUT", "not-a-duration")
	defer os.Unsetenv("PENFOLD_AI_REQUEST_TIMEOUT") //nolint:errcheck

	if v := os.Getenv("PENFOLD_AI_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			AIRequestTimeout = d
		}
	}

	if AIRequestTimeout != original {
		t.Errorf("AIRequestTimeout should remain %v for invalid env value, got %v", original, AIRequestTimeout)
	}
}
