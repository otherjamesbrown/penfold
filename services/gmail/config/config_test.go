package config

import (
	"os"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           8081,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: false,
		},
		{
			name: "invalid grpc_port - zero",
			config: &Config{
				GRPCPort:           0,
				HTTPPort:           8081,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid grpc_port",
		},
		{
			name: "invalid grpc_port - negative",
			config: &Config{
				GRPCPort:           -1,
				HTTPPort:           8081,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid grpc_port",
		},
		{
			name: "invalid grpc_port - too high",
			config: &Config{
				GRPCPort:           65536,
				HTTPPort:           8081,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid grpc_port",
		},
		{
			name: "invalid http_port - zero",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           0,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid http_port",
		},
		{
			name: "invalid http_port - negative",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           -1,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid http_port",
		},
		{
			name: "invalid http_port - too high",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           65536,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid http_port",
		},
		{
			name: "same ports",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           50051,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "must be different",
		},
		{
			name: "invalid max_sync_batch_size - zero",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           8081,
				MaxSyncBatchSize:   0,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid max_sync_batch_size",
		},
		{
			name: "invalid max_sync_batch_size - negative",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           8081,
				MaxSyncBatchSize:   -1,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid max_sync_batch_size",
		},
		{
			name: "invalid max_sync_batch_size - too high",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           8081,
				MaxSyncBatchSize:   10001,
				SyncTimeoutSeconds: 300,
			},
			wantErr: true,
			errMsg:  "invalid max_sync_batch_size",
		},
		{
			name: "invalid sync_timeout_seconds - zero",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           8081,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: 0,
			},
			wantErr: true,
			errMsg:  "invalid sync_timeout_seconds",
		},
		{
			name: "invalid sync_timeout_seconds - negative",
			config: &Config{
				GRPCPort:           50051,
				HTTPPort:           8081,
				MaxSyncBatchSize:   500,
				SyncTimeoutSeconds: -1,
			},
			wantErr: true,
			errMsg:  "invalid sync_timeout_seconds",
		},
		{
			name: "boundary values - minimum valid",
			config: &Config{
				GRPCPort:           1,
				HTTPPort:           2,
				MaxSyncBatchSize:   1,
				SyncTimeoutSeconds: 1,
			},
			wantErr: false,
		},
		{
			name: "boundary values - maximum valid",
			config: &Config{
				GRPCPort:           65535,
				HTTPPort:           65534,
				MaxSyncBatchSize:   10000,
				SyncTimeoutSeconds: 86400,
			},
			wantErr: false,
		},
		{
			name: "multiple errors",
			config: &Config{
				GRPCPort:           0,
				HTTPPort:           0,
				MaxSyncBatchSize:   0,
				SyncTimeoutSeconds: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Config.Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestConfig_GRPCAddress(t *testing.T) {
	tests := []struct {
		name     string
		grpcPort int
		want     string
	}{
		{"default port", 50051, ":50051"},
		{"custom port", 9000, ":9000"},
		{"min port", 1, ":1"},
		{"max port", 65535, ":65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{GRPCPort: tt.grpcPort}
			if got := cfg.GRPCAddress(); got != tt.want {
				t.Errorf("Config.GRPCAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_HTTPAddress(t *testing.T) {
	tests := []struct {
		name     string
		httpPort int
		want     string
	}{
		{"default port", 8081, ":8081"},
		{"custom port", 9000, ":9000"},
		{"min port", 1, ":1"},
		{"max port", 65535, ":65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{HTTPPort: tt.httpPort}
			if got := cfg.HTTPAddress(); got != tt.want {
				t.Errorf("Config.HTTPAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadServiceEnv(t *testing.T) {
	// Save and clear existing env vars
	envVars := []string{
		"GMAIL_GRPC_PORT",
		"GMAIL_HTTP_PORT",
		"GMAIL_OAUTH_CREDENTIALS_PATH",
		"GMAIL_TOKEN_STORE_PATH",
		"GMAIL_MAX_SYNC_BATCH_SIZE",
		"GMAIL_SYNC_TIMEOUT_SECONDS",
	}
	savedEnv := make(map[string]string)
	for _, v := range envVars {
		savedEnv[v] = os.Getenv(v)
		_ = os.Unsetenv(v)
	}
	defer func() {
		for k, v := range savedEnv {
			if v != "" {
				_ = os.Setenv(k, v)
			}
		}
	}()

	t.Run("no env vars set uses defaults", func(t *testing.T) {
		cfg := &Config{
			GRPCPort:           DefaultGRPCPort,
			HTTPPort:           DefaultHTTPPort,
			MaxSyncBatchSize:   DefaultMaxSyncBatchSize,
			SyncTimeoutSeconds: DefaultSyncTimeoutSeconds,
		}
		loadServiceEnv(cfg)

		if cfg.GRPCPort != DefaultGRPCPort {
			t.Errorf("GRPCPort = %d, want %d", cfg.GRPCPort, DefaultGRPCPort)
		}
		if cfg.HTTPPort != DefaultHTTPPort {
			t.Errorf("HTTPPort = %d, want %d", cfg.HTTPPort, DefaultHTTPPort)
		}
	})

	t.Run("GMAIL_GRPC_PORT", func(t *testing.T) {
		_ = os.Setenv("GMAIL_GRPC_PORT", "9000")
		defer os.Unsetenv("GMAIL_GRPC_PORT") //nolint:errcheck

		cfg := &Config{GRPCPort: DefaultGRPCPort}
		loadServiceEnv(cfg)

		if cfg.GRPCPort != 9000 {
			t.Errorf("GRPCPort = %d, want 9000", cfg.GRPCPort)
		}
	})

	t.Run("GMAIL_GRPC_PORT invalid value", func(t *testing.T) {
		_ = os.Setenv("GMAIL_GRPC_PORT", "not-a-number")
		defer os.Unsetenv("GMAIL_GRPC_PORT") //nolint:errcheck

		cfg := &Config{GRPCPort: DefaultGRPCPort}
		loadServiceEnv(cfg)

		// Should keep default on parse error
		if cfg.GRPCPort != DefaultGRPCPort {
			t.Errorf("GRPCPort = %d, want %d (default)", cfg.GRPCPort, DefaultGRPCPort)
		}
	})

	t.Run("GMAIL_HTTP_PORT", func(t *testing.T) {
		_ = os.Setenv("GMAIL_HTTP_PORT", "9001")
		defer os.Unsetenv("GMAIL_HTTP_PORT") //nolint:errcheck

		cfg := &Config{HTTPPort: DefaultHTTPPort}
		loadServiceEnv(cfg)

		if cfg.HTTPPort != 9001 {
			t.Errorf("HTTPPort = %d, want 9001", cfg.HTTPPort)
		}
	})

	t.Run("GMAIL_HTTP_PORT invalid value", func(t *testing.T) {
		_ = os.Setenv("GMAIL_HTTP_PORT", "invalid")
		defer os.Unsetenv("GMAIL_HTTP_PORT") //nolint:errcheck

		cfg := &Config{HTTPPort: DefaultHTTPPort}
		loadServiceEnv(cfg)

		// Should keep default on parse error
		if cfg.HTTPPort != DefaultHTTPPort {
			t.Errorf("HTTPPort = %d, want %d (default)", cfg.HTTPPort, DefaultHTTPPort)
		}
	})

	t.Run("GMAIL_OAUTH_CREDENTIALS_PATH", func(t *testing.T) {
		_ = os.Setenv("GMAIL_OAUTH_CREDENTIALS_PATH", "/path/to/credentials.json")
		defer os.Unsetenv("GMAIL_OAUTH_CREDENTIALS_PATH") //nolint:errcheck

		cfg := &Config{}
		loadServiceEnv(cfg)

		if cfg.OAuthCredentialsPath != "/path/to/credentials.json" {
			t.Errorf("OAuthCredentialsPath = %s, want /path/to/credentials.json", cfg.OAuthCredentialsPath)
		}
	})

	t.Run("GMAIL_TOKEN_STORE_PATH", func(t *testing.T) {
		_ = os.Setenv("GMAIL_TOKEN_STORE_PATH", "/path/to/tokens")
		defer os.Unsetenv("GMAIL_TOKEN_STORE_PATH") //nolint:errcheck

		cfg := &Config{}
		loadServiceEnv(cfg)

		if cfg.TokenStorePath != "/path/to/tokens" {
			t.Errorf("TokenStorePath = %s, want /path/to/tokens", cfg.TokenStorePath)
		}
	})

	t.Run("GMAIL_MAX_SYNC_BATCH_SIZE", func(t *testing.T) {
		_ = os.Setenv("GMAIL_MAX_SYNC_BATCH_SIZE", "1000")
		defer os.Unsetenv("GMAIL_MAX_SYNC_BATCH_SIZE") //nolint:errcheck

		cfg := &Config{MaxSyncBatchSize: DefaultMaxSyncBatchSize}
		loadServiceEnv(cfg)

		if cfg.MaxSyncBatchSize != 1000 {
			t.Errorf("MaxSyncBatchSize = %d, want 1000", cfg.MaxSyncBatchSize)
		}
	})

	t.Run("GMAIL_MAX_SYNC_BATCH_SIZE invalid value", func(t *testing.T) {
		_ = os.Setenv("GMAIL_MAX_SYNC_BATCH_SIZE", "abc")
		defer os.Unsetenv("GMAIL_MAX_SYNC_BATCH_SIZE") //nolint:errcheck

		cfg := &Config{MaxSyncBatchSize: DefaultMaxSyncBatchSize}
		loadServiceEnv(cfg)

		// Should keep default on parse error
		if cfg.MaxSyncBatchSize != DefaultMaxSyncBatchSize {
			t.Errorf("MaxSyncBatchSize = %d, want %d (default)", cfg.MaxSyncBatchSize, DefaultMaxSyncBatchSize)
		}
	})

	t.Run("GMAIL_SYNC_TIMEOUT_SECONDS", func(t *testing.T) {
		_ = os.Setenv("GMAIL_SYNC_TIMEOUT_SECONDS", "600")
		defer os.Unsetenv("GMAIL_SYNC_TIMEOUT_SECONDS") //nolint:errcheck

		cfg := &Config{SyncTimeoutSeconds: DefaultSyncTimeoutSeconds}
		loadServiceEnv(cfg)

		if cfg.SyncTimeoutSeconds != 600 {
			t.Errorf("SyncTimeoutSeconds = %d, want 600", cfg.SyncTimeoutSeconds)
		}
	})

	t.Run("GMAIL_SYNC_TIMEOUT_SECONDS invalid value", func(t *testing.T) {
		_ = os.Setenv("GMAIL_SYNC_TIMEOUT_SECONDS", "xyz")
		defer os.Unsetenv("GMAIL_SYNC_TIMEOUT_SECONDS") //nolint:errcheck

		cfg := &Config{SyncTimeoutSeconds: DefaultSyncTimeoutSeconds}
		loadServiceEnv(cfg)

		// Should keep default on parse error
		if cfg.SyncTimeoutSeconds != DefaultSyncTimeoutSeconds {
			t.Errorf("SyncTimeoutSeconds = %d, want %d (default)", cfg.SyncTimeoutSeconds, DefaultSyncTimeoutSeconds)
		}
	})

	t.Run("all env vars set", func(t *testing.T) {
		_ = os.Setenv("GMAIL_GRPC_PORT", "9000")
		_ = os.Setenv("GMAIL_HTTP_PORT", "9001")
		_ = os.Setenv("GMAIL_OAUTH_CREDENTIALS_PATH", "/credentials.json")
		_ = os.Setenv("GMAIL_TOKEN_STORE_PATH", "/tokens")
		_ = os.Setenv("GMAIL_MAX_SYNC_BATCH_SIZE", "1000")
		_ = os.Setenv("GMAIL_SYNC_TIMEOUT_SECONDS", "600")
		defer func() {
			_ = os.Unsetenv("GMAIL_GRPC_PORT")
			_ = os.Unsetenv("GMAIL_HTTP_PORT")
			_ = os.Unsetenv("GMAIL_OAUTH_CREDENTIALS_PATH")
			_ = os.Unsetenv("GMAIL_TOKEN_STORE_PATH")
			_ = os.Unsetenv("GMAIL_MAX_SYNC_BATCH_SIZE")
			_ = os.Unsetenv("GMAIL_SYNC_TIMEOUT_SECONDS")
		}()

		cfg := &Config{
			GRPCPort:           DefaultGRPCPort,
			HTTPPort:           DefaultHTTPPort,
			MaxSyncBatchSize:   DefaultMaxSyncBatchSize,
			SyncTimeoutSeconds: DefaultSyncTimeoutSeconds,
		}
		loadServiceEnv(cfg)

		if cfg.GRPCPort != 9000 {
			t.Errorf("GRPCPort = %d, want 9000", cfg.GRPCPort)
		}
		if cfg.HTTPPort != 9001 {
			t.Errorf("HTTPPort = %d, want 9001", cfg.HTTPPort)
		}
		if cfg.OAuthCredentialsPath != "/credentials.json" {
			t.Errorf("OAuthCredentialsPath = %s, want /credentials.json", cfg.OAuthCredentialsPath)
		}
		if cfg.TokenStorePath != "/tokens" {
			t.Errorf("TokenStorePath = %s, want /tokens", cfg.TokenStorePath)
		}
		if cfg.MaxSyncBatchSize != 1000 {
			t.Errorf("MaxSyncBatchSize = %d, want 1000", cfg.MaxSyncBatchSize)
		}
		if cfg.SyncTimeoutSeconds != 600 {
			t.Errorf("SyncTimeoutSeconds = %d, want 600", cfg.SyncTimeoutSeconds)
		}
	})
}

func TestDefaultConstants(t *testing.T) {
	if DefaultGRPCPort != 50051 {
		t.Errorf("DefaultGRPCPort = %d, want 50051", DefaultGRPCPort)
	}
	if DefaultHTTPPort != 8081 {
		t.Errorf("DefaultHTTPPort = %d, want 8081", DefaultHTTPPort)
	}
	if DefaultMaxSyncBatchSize != 500 {
		t.Errorf("DefaultMaxSyncBatchSize = %d, want 500", DefaultMaxSyncBatchSize)
	}
	if DefaultSyncTimeoutSeconds != 300 {
		t.Errorf("DefaultSyncTimeoutSeconds = %d, want 300", DefaultSyncTimeoutSeconds)
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := &Config{
		GRPCPort:             50052,
		HTTPPort:             8082,
		OAuthCredentialsPath: "/path/to/creds.json",
		TokenStorePath:       "/path/to/tokens",
		MaxSyncBatchSize:     1000,
		SyncTimeoutSeconds:   600,
	}

	if cfg.GRPCPort != 50052 {
		t.Errorf("GRPCPort = %d, want 50052", cfg.GRPCPort)
	}
	if cfg.HTTPPort != 8082 {
		t.Errorf("HTTPPort = %d, want 8082", cfg.HTTPPort)
	}
	if cfg.OAuthCredentialsPath != "/path/to/creds.json" {
		t.Errorf("OAuthCredentialsPath = %s, want /path/to/creds.json", cfg.OAuthCredentialsPath)
	}
	if cfg.TokenStorePath != "/path/to/tokens" {
		t.Errorf("TokenStorePath = %s, want /path/to/tokens", cfg.TokenStorePath)
	}
	if cfg.MaxSyncBatchSize != 1000 {
		t.Errorf("MaxSyncBatchSize = %d, want 1000", cfg.MaxSyncBatchSize)
	}
	if cfg.SyncTimeoutSeconds != 600 {
		t.Errorf("SyncTimeoutSeconds = %d, want 600", cfg.SyncTimeoutSeconds)
	}
}

// Helper function for substring checking
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
