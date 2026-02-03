// Package timeouts provides centralized timeout constants for the AI/LLM request path.
//
// The request chain is: CLI (penf) -> Gateway (dev02) -> AI Coordinator (dev01) -> MLX Backend -> MLX Server
//
// Instead of scattering timeout values across 14+ files, all AI-path timeouts
// are defined here as a single source of truth. Environment variable overrides
// are supported for operational tuning without recompilation.
package timeouts

import (
	"os"
	"time"
)

// AI request path timeouts.
var (
	// AIRequestTimeout is the end-to-end timeout for AI requests.
	// Applies to: CLI commands, gateway AI client, pkg/ai client.
	// Override: PENFOLD_AI_REQUEST_TIMEOUT (e.g. "10m", "30s")
	AIRequestTimeout = 5 * time.Minute

	// MLXBackendTimeout is the HTTP client timeout for MLX server requests.
	// Applies to: MLX backend HTTP client in the AI coordinator.
	// Override: PENFOLD_MLX_TIMEOUT (e.g. "10m", "30s")
	MLXBackendTimeout = 5 * time.Minute
)

// Connection and infrastructure timeouts (not overridable via env).
const (
	// ConnectTimeout is the timeout for establishing gRPC/HTTP connections.
	ConnectTimeout = 10 * time.Second

	// HealthCheckTimeout is the timeout for health check probes.
	HealthCheckTimeout = 5 * time.Second

	// KeepaliveTime is the interval between gRPC keepalive pings.
	// Must be >= gRPC server's enforcement policy MinTime (default 5 min).
	KeepaliveTime = 5 * time.Minute

	// KeepaliveTimeout is the timeout waiting for a keepalive ping response.
	KeepaliveTimeout = 20 * time.Second
)

func init() {
	if v := os.Getenv("PENFOLD_AI_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			AIRequestTimeout = d
		}
	}
	if v := os.Getenv("PENFOLD_MLX_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			MLXBackendTimeout = d
		}
	}
}
