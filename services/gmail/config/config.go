// Package config provides Gmail Connector service-specific configuration.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	pkgconfig "github.com/otherjamesbrown/penfold/pkg/config"
)

// Config holds Gmail Connector service-specific configuration.
type Config struct {
	// Base embeds the shared Penfold configuration.
	Base *pkgconfig.Config

	// GRPCPort is the port to listen on for gRPC connections.
	GRPCPort int

	// HTTPPort is the port for health checks and metrics.
	HTTPPort int

	// OAuthCredentialsPath is the path to the OAuth 2.0 credentials JSON file.
	OAuthCredentialsPath string

	// TokenStorePath is the path where OAuth tokens are stored.
	TokenStorePath string

	// MaxSyncBatchSize is the maximum number of emails to fetch in a single batch.
	MaxSyncBatchSize int

	// SyncTimeoutSeconds is the timeout for sync operations in seconds.
	SyncTimeoutSeconds int

	// TokenEncryptionKey is the 32-byte AES-256-GCM key for encrypting OAuth tokens.
	// Loaded from GMAIL_TOKEN_ENCRYPTION_KEY as a 64-character hex string.
	TokenEncryptionKey []byte
}

// Default configuration values.
const (
	DefaultGRPCPort          = 50051
	DefaultHTTPPort          = 8081
	DefaultMaxSyncBatchSize  = 500
	DefaultSyncTimeoutSeconds = 300 // 5 minutes
)

// LoadConfig loads the Gmail Connector service configuration.
// It loads the base Penfold configuration and overlays service-specific settings.
func LoadConfig() (*Config, error) {
	baseConfig, err := pkgconfig.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading base config: %w", err)
	}

	// Set the service name if not already set.
	if baseConfig.ServiceName == "" {
		baseConfig.ServiceName = "gmail-connector"
	}

	cfg := &Config{
		Base:               baseConfig,
		GRPCPort:           DefaultGRPCPort,
		HTTPPort:           DefaultHTTPPort,
		MaxSyncBatchSize:   DefaultMaxSyncBatchSize,
		SyncTimeoutSeconds: DefaultSyncTimeoutSeconds,
	}

	// Load service-specific environment variables.
	loadServiceEnv(cfg)

	// Validate the configuration.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating gmail config: %w", err)
	}

	return cfg, nil
}

// loadServiceEnv loads Gmail Connector-specific environment variables.
func loadServiceEnv(cfg *Config) {
	if v := os.Getenv("GMAIL_GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GRPCPort = port
		}
	}

	if v := os.Getenv("GMAIL_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}

	if v := os.Getenv("GMAIL_OAUTH_CREDENTIALS_PATH"); v != "" {
		cfg.OAuthCredentialsPath = v
	}

	if v := os.Getenv("GMAIL_TOKEN_STORE_PATH"); v != "" {
		cfg.TokenStorePath = v
	}

	if v := os.Getenv("GMAIL_MAX_SYNC_BATCH_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil {
			cfg.MaxSyncBatchSize = size
		}
	}

	if v := os.Getenv("GMAIL_SYNC_TIMEOUT_SECONDS"); v != "" {
		if timeout, err := strconv.Atoi(v); err == nil {
			cfg.SyncTimeoutSeconds = timeout
		}
	}

	if v := os.Getenv("GMAIL_TOKEN_ENCRYPTION_KEY"); v != "" {
		if key, err := hex.DecodeString(v); err == nil {
			cfg.TokenEncryptionKey = key
		}
	}
}

// Validate checks that the Gmail Connector configuration is valid.
func (c *Config) Validate() error {
	var errs []string

	if c.GRPCPort <= 0 || c.GRPCPort > 65535 {
		errs = append(errs, fmt.Sprintf("invalid grpc_port: %d", c.GRPCPort))
	}

	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		errs = append(errs, fmt.Sprintf("invalid http_port: %d", c.HTTPPort))
	}

	if c.GRPCPort == c.HTTPPort {
		errs = append(errs, "grpc_port and http_port must be different")
	}

	if c.MaxSyncBatchSize <= 0 || c.MaxSyncBatchSize > 10000 {
		errs = append(errs, fmt.Sprintf("invalid max_sync_batch_size: %d (must be 1-10000)", c.MaxSyncBatchSize))
	}

	if c.SyncTimeoutSeconds <= 0 {
		errs = append(errs, fmt.Sprintf("invalid sync_timeout_seconds: %d", c.SyncTimeoutSeconds))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

// GRPCAddress returns the gRPC server listen address.
func (c *Config) GRPCAddress() string {
	return fmt.Sprintf(":%d", c.GRPCPort)
}

// HTTPAddress returns the HTTP server listen address.
func (c *Config) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}
