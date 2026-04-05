package config

import (
	"os"
	"path/filepath"
	"testing"
)

// clearEnv clears all PENFOLD_ environment variables.
func clearEnv(t *testing.T) {
	t.Helper()
	envVars := []string{
		"PENFOLD_CONFIG_FILE",
		"PENFOLD_SERVICE_NAME",
		"PENFOLD_ENVIRONMENT",
		"PENFOLD_LOG_LEVEL",
		"PENFOLD_DB_HOST",
		"PENFOLD_DB_PORT",
		"PENFOLD_DB_NAME",
		"PENFOLD_DB_USER",
		"PENFOLD_DB_PASSWORD",
		"PENFOLD_DB_SSL_MODE",
		"PENFOLD_DB_SSL_CERT",
		"PENFOLD_DB_SSL_KEY",
		"PENFOLD_DB_SSL_ROOT_CERT",
		"PENFOLD_REDIS_HOST",
		"PENFOLD_REDIS_PORT",
		"PENFOLD_TEMPORAL_HOST",
		"PENFOLD_TEMPORAL_NAMESPACE",
	}
	for _, v := range envVars {
		_ = os.Unsetenv(v)
	}
}

// setMinimalEnv sets the minimum required environment variables.
func setMinimalEnv(t *testing.T) {
	t.Helper()
	_ = os.Setenv("PENFOLD_SERVICE_NAME", "test-service")
}

func TestLoadConfig_FromEnv(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	// Set all environment variables.
	_ = os.Setenv("PENFOLD_SERVICE_NAME", "my-service")
	_ = os.Setenv("PENFOLD_ENVIRONMENT", "staging")
	_ = os.Setenv("PENFOLD_LOG_LEVEL", "debug")
	_ = os.Setenv("PENFOLD_DB_HOST", "db.example.com")
	_ = os.Setenv("PENFOLD_DB_PORT", "5433")
	_ = os.Setenv("PENFOLD_DB_NAME", "mydb")
	_ = os.Setenv("PENFOLD_DB_USER", "myuser")
	_ = os.Setenv("PENFOLD_DB_PASSWORD", "mypass")
	_ = os.Setenv("PENFOLD_REDIS_HOST", "redis.example.com")
	_ = os.Setenv("PENFOLD_REDIS_PORT", "6380")
	_ = os.Setenv("PENFOLD_TEMPORAL_HOST", "temporal.example.com:7234")
	_ = os.Setenv("PENFOLD_TEMPORAL_NAMESPACE", "my-namespace")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify all values.
	if cfg.ServiceName != "my-service" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "my-service")
	}
	if cfg.Environment != EnvStaging {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvStaging)
	}
	if cfg.LogLevel != LogDebug {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogDebug)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "db.example.com")
	}
	if cfg.Database.Port != 5433 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5433)
	}
	if cfg.Database.Name != "mydb" {
		t.Errorf("Database.Name = %q, want %q", cfg.Database.Name, "mydb")
	}
	if cfg.Database.User != "myuser" {
		t.Errorf("Database.User = %q, want %q", cfg.Database.User, "myuser")
	}
	if cfg.Database.Password != "mypass" {
		t.Errorf("Database.Password = %q, want %q", cfg.Database.Password, "mypass")
	}
	if cfg.Redis.Host != "redis.example.com" {
		t.Errorf("Redis.Host = %q, want %q", cfg.Redis.Host, "redis.example.com")
	}
	if cfg.Redis.Port != 6380 {
		t.Errorf("Redis.Port = %d, want %d", cfg.Redis.Port, 6380)
	}
	if cfg.Temporal.Host != "temporal.example.com:7234" {
		t.Errorf("Temporal.Host = %q, want %q", cfg.Temporal.Host, "temporal.example.com:7234")
	}
	if cfg.Temporal.Namespace != "my-namespace" {
		t.Errorf("Temporal.Namespace = %q, want %q", cfg.Temporal.Namespace, "my-namespace")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	// Only set required service name.
	setMinimalEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify defaults.
	if cfg.Environment != DefaultEnvironment {
		t.Errorf("Environment = %q, want %q", cfg.Environment, DefaultEnvironment)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
	if cfg.Database.Host != DefaultDBHost {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, DefaultDBHost)
	}
	if cfg.Database.Port != DefaultDBPort {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, DefaultDBPort)
	}
	if cfg.Database.Name != DefaultDBName {
		t.Errorf("Database.Name = %q, want %q", cfg.Database.Name, DefaultDBName)
	}
	if cfg.Database.User != DefaultDBUser {
		t.Errorf("Database.User = %q, want %q", cfg.Database.User, DefaultDBUser)
	}
	if cfg.Redis.Host != DefaultRedisHost {
		t.Errorf("Redis.Host = %q, want %q", cfg.Redis.Host, DefaultRedisHost)
	}
	if cfg.Redis.Port != DefaultRedisPort {
		t.Errorf("Redis.Port = %d, want %d", cfg.Redis.Port, DefaultRedisPort)
	}
	if cfg.Temporal.Host != DefaultTemporalHost {
		t.Errorf("Temporal.Host = %q, want %q", cfg.Temporal.Host, DefaultTemporalHost)
	}
	if cfg.Temporal.Namespace != DefaultTemporalNamespace {
		t.Errorf("Temporal.Namespace = %q, want %q", cfg.Temporal.Namespace, DefaultTemporalNamespace)
	}
}

func TestLoadConfig_FromYAML(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	// Create a temporary YAML config file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
service_name: yaml-service
environment: prod
log_level: warn
database:
  host: yaml-db.example.com
  port: 5434
  name: yamldb
  user: yamluser
  password: yamlpass
redis:
  host: yaml-redis.example.com
  port: 6381
temporal:
  host: yaml-temporal.example.com:7235
  namespace: yaml-namespace
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_ = os.Setenv("PENFOLD_CONFIG_FILE", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Verify YAML values.
	if cfg.ServiceName != "yaml-service" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "yaml-service")
	}
	if cfg.Environment != EnvProd {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvProd)
	}
	if cfg.LogLevel != LogWarn {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogWarn)
	}
	if cfg.Database.Host != "yaml-db.example.com" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "yaml-db.example.com")
	}
	if cfg.Database.Port != 5434 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5434)
	}
}

func TestLoadConfig_EnvOverridesYAML(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	// Create a YAML config file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
service_name: yaml-service
environment: staging
log_level: info
database:
  host: yaml-db.example.com
  port: 5432
  name: yamldb
  user: yamluser
  password: yamlpass
redis:
  host: yaml-redis.example.com
  port: 6379
temporal:
  host: yaml-temporal.example.com:7233
  namespace: yaml-namespace
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_ = os.Setenv("PENFOLD_CONFIG_FILE", configPath)
	// Override specific values with environment variables.
	_ = os.Setenv("PENFOLD_SERVICE_NAME", "env-service")
	_ = os.Setenv("PENFOLD_ENVIRONMENT", "prod")
	_ = os.Setenv("PENFOLD_DB_HOST", "env-db.example.com")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Environment should override YAML.
	if cfg.ServiceName != "env-service" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "env-service")
	}
	if cfg.Environment != EnvProd {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvProd)
	}
	if cfg.Database.Host != "env-db.example.com" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "env-db.example.com")
	}
	// YAML values not overridden should remain.
	if cfg.LogLevel != LogInfo {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogInfo)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5432)
	}
}

func TestLoadConfig_ValidationError_MissingServiceName(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	// Don't set service name.
	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for missing service_name, got nil")
	}
}

func TestLoadConfig_ValidationError_InvalidEnvironment(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	setMinimalEnv(t)
	_ = os.Setenv("PENFOLD_ENVIRONMENT", "invalid")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for invalid environment, got nil")
	}
}

func TestLoadConfig_ValidationError_InvalidLogLevel(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	setMinimalEnv(t)
	_ = os.Setenv("PENFOLD_LOG_LEVEL", "invalid")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for invalid log_level, got nil")
	}
}

func TestLoadConfig_InvalidYAMLFile(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	// Create an invalid YAML file.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:::"), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_ = os.Setenv("PENFOLD_CONFIG_FILE", configPath)

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for invalid YAML, got nil")
	}
}

func TestLoadConfig_MissingYAMLFile(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	_ = os.Setenv("PENFOLD_CONFIG_FILE", "/nonexistent/path/config.yaml")

	_, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() expected error for missing config file, got nil")
	}
}

func TestDatabaseConfig_ConnectionString(t *testing.T) {
	db := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "testdb",
		User:     "testuser",
		Password: "testpass",
	}

	expected := "host=localhost port=5432 dbname=testdb user=testuser password=testpass sslmode=disable"
	if got := db.ConnectionString(); got != expected {
		t.Errorf("ConnectionString() = %q, want %q", got, expected)
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	db := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "testdb",
		User:     "testuser",
		Password: "testpass",
	}

	expected := "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable"
	if got := db.DSN(); got != expected {
		t.Errorf("DSN() = %q, want %q", got, expected)
	}
}

func TestRedisConfig_Address(t *testing.T) {
	redis := RedisConfig{
		Host: "localhost",
		Port: 6379,
	}

	expected := "localhost:6379"
	if got := redis.Address(); got != expected {
		t.Errorf("Address() = %q, want %q", got, expected)
	}
}

func TestTemporalConfig_Address(t *testing.T) {
	temporal := TemporalConfig{
		Host:      "localhost:7233",
		Namespace: "default",
	}

	expected := "localhost:7233"
	if got := temporal.Address(); got != expected {
		t.Errorf("Address() = %q, want %q", got, expected)
	}
}

func TestEnvironment_IsValid(t *testing.T) {
	tests := []struct {
		env   Environment
		valid bool
	}{
		{EnvDev, true},
		{EnvStaging, true},
		{EnvProd, true},
		{Environment("invalid"), false},
		{Environment(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			if got := tt.env.IsValid(); got != tt.valid {
				t.Errorf("Environment(%q).IsValid() = %v, want %v", tt.env, got, tt.valid)
			}
		})
	}
}

func TestLogLevel_IsValid(t *testing.T) {
	tests := []struct {
		level LogLevel
		valid bool
	}{
		{LogDebug, true},
		{LogInfo, true},
		{LogWarn, true},
		{LogError, true},
		{LogLevel("invalid"), false},
		{LogLevel(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			if got := tt.level.IsValid(); got != tt.valid {
				t.Errorf("LogLevel(%q).IsValid() = %v, want %v", tt.level, got, tt.valid)
			}
		})
	}
}

func TestConfig_IsDevelopment(t *testing.T) {
	tests := []struct {
		env      Environment
		expected bool
	}{
		{EnvDev, true},
		{EnvStaging, false},
		{EnvProd, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			cfg := &Config{Environment: tt.env}
			if got := cfg.IsDevelopment(); got != tt.expected {
				t.Errorf("Config{Environment: %q}.IsDevelopment() = %v, want %v", tt.env, got, tt.expected)
			}
		})
	}
}

func TestConfig_IsProduction(t *testing.T) {
	tests := []struct {
		env      Environment
		expected bool
	}{
		{EnvDev, false},
		{EnvStaging, false},
		{EnvProd, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.env), func(t *testing.T) {
			cfg := &Config{Environment: tt.env}
			if got := cfg.IsProduction(); got != tt.expected {
				t.Errorf("Config{Environment: %q}.IsProduction() = %v, want %v", tt.env, got, tt.expected)
			}
		})
	}
}

func TestEnvironment_String(t *testing.T) {
	if got := EnvDev.String(); got != "dev" {
		t.Errorf("EnvDev.String() = %q, want %q", got, "dev")
	}
	if got := EnvStaging.String(); got != "staging" {
		t.Errorf("EnvStaging.String() = %q, want %q", got, "staging")
	}
	if got := EnvProd.String(); got != "prod" {
		t.Errorf("EnvProd.String() = %q, want %q", got, "prod")
	}
}

func TestLogLevel_String(t *testing.T) {
	if got := LogDebug.String(); got != "debug" {
		t.Errorf("LogDebug.String() = %q, want %q", got, "debug")
	}
	if got := LogInfo.String(); got != "info" {
		t.Errorf("LogInfo.String() = %q, want %q", got, "info")
	}
	if got := LogWarn.String(); got != "warn" {
		t.Errorf("LogWarn.String() = %q, want %q", got, "warn")
	}
	if got := LogError.String(); got != "error" {
		t.Errorf("LogError.String() = %q, want %q", got, "error")
	}
}

func TestConfig_Validate_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too_high", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ServiceName: "test",
				Environment: EnvDev,
				LogLevel:    LogInfo,
				Database: DatabaseConfig{
					Host: "localhost",
					Port: tt.port,
					Name: "test",
					User: "test",
				},
				Redis: RedisConfig{
					Host: "localhost",
					Port: 6379,
				},
				Temporal: TemporalConfig{
					Host:      "localhost:7233",
					Namespace: "default",
				},
			}

			if err := cfg.Validate(); err == nil {
				t.Error("Validate() expected error for invalid database port, got nil")
			}
		})
	}
}

func TestLoadConfig_CaseInsensitiveEnv(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	setMinimalEnv(t)
	_ = os.Setenv("PENFOLD_ENVIRONMENT", "PROD")
	_ = os.Setenv("PENFOLD_LOG_LEVEL", "DEBUG")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Environment != EnvProd {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvProd)
	}
	if cfg.LogLevel != LogDebug {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, LogDebug)
	}
}

func TestDatabaseConfig_DSN_WithSSL(t *testing.T) {
	tests := []struct {
		name     string
		db       DatabaseConfig
		expected string
	}{
		{
			name: "sslmode only",
			db: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				User:     "testuser",
				Password: "testpass",
				SSLMode:  "require",
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=require",
		},
		{
			name: "full mTLS config",
			db: DatabaseConfig{
				Host:        "db.example.com",
				Port:        5432,
				Name:        "testdb",
				User:        "testuser",
				Password:    "testpass",
				SSLMode:     "verify-full",
				SSLCert:     "/path/to/client.crt",
				SSLKey:      "/path/to/client.key",
				SSLRootCert: "/path/to/root.crt",
			},
			expected: "host=db.example.com port=5432 user=testuser password=testpass dbname=testdb sslmode=verify-full sslcert=/path/to/client.crt sslkey=/path/to/client.key sslrootcert=/path/to/root.crt",
		},
		{
			name: "partial SSL config",
			db: DatabaseConfig{
				Host:        "db.example.com",
				Port:        5432,
				Name:        "testdb",
				User:        "testuser",
				Password:    "testpass",
				SSLMode:     "verify-ca",
				SSLRootCert: "/path/to/root.crt",
			},
			expected: "host=db.example.com port=5432 user=testuser password=testpass dbname=testdb sslmode=verify-ca sslrootcert=/path/to/root.crt",
		},
		{
			name: "empty sslmode defaults to disable",
			db: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				User:     "testuser",
				Password: "testpass",
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.db.DSN(); got != tt.expected {
				t.Errorf("DSN() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDatabaseConfig_ConnectionString_WithSSL(t *testing.T) {
	tests := []struct {
		name     string
		db       DatabaseConfig
		expected string
	}{
		{
			name: "sslmode only",
			db: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				User:     "testuser",
				Password: "testpass",
				SSLMode:  "require",
			},
			expected: "host=localhost port=5432 dbname=testdb user=testuser password=testpass sslmode=require",
		},
		{
			name: "full mTLS config",
			db: DatabaseConfig{
				Host:        "db.example.com",
				Port:        5432,
				Name:        "testdb",
				User:        "testuser",
				Password:    "testpass",
				SSLMode:     "verify-full",
				SSLCert:     "/path/to/client.crt",
				SSLKey:      "/path/to/client.key",
				SSLRootCert: "/path/to/root.crt",
			},
			expected: "host=db.example.com port=5432 dbname=testdb user=testuser password=testpass sslmode=verify-full sslcert=/path/to/client.crt sslkey=/path/to/client.key sslrootcert=/path/to/root.crt",
		},
		{
			name: "empty sslmode defaults to disable",
			db: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "testdb",
				User:     "testuser",
				Password: "testpass",
			},
			expected: "host=localhost port=5432 dbname=testdb user=testuser password=testpass sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.db.ConnectionString(); got != tt.expected {
				t.Errorf("ConnectionString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLoadConfig_SSLFromEnv(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	setMinimalEnv(t)
	_ = os.Setenv("PENFOLD_DB_SSL_MODE", "verify-full")
	_ = os.Setenv("PENFOLD_DB_SSL_CERT", "/path/to/client.crt")
	_ = os.Setenv("PENFOLD_DB_SSL_KEY", "/path/to/client.key")
	_ = os.Setenv("PENFOLD_DB_SSL_ROOT_CERT", "/path/to/root.crt")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Database.SSLMode != "verify-full" {
		t.Errorf("Database.SSLMode = %q, want %q", cfg.Database.SSLMode, "verify-full")
	}
	if cfg.Database.SSLCert != "/path/to/client.crt" {
		t.Errorf("Database.SSLCert = %q, want %q", cfg.Database.SSLCert, "/path/to/client.crt")
	}
	if cfg.Database.SSLKey != "/path/to/client.key" {
		t.Errorf("Database.SSLKey = %q, want %q", cfg.Database.SSLKey, "/path/to/client.key")
	}
	if cfg.Database.SSLRootCert != "/path/to/root.crt" {
		t.Errorf("Database.SSLRootCert = %q, want %q", cfg.Database.SSLRootCert, "/path/to/root.crt")
	}
}

func TestLoadConfig_SSLFromYAML(t *testing.T) {
	clearEnv(t)
	defer clearEnv(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
service_name: ssl-test-service
environment: prod
log_level: info
database:
  host: db.example.com
  port: 5432
  name: testdb
  user: testuser
  password: testpass
  ssl_mode: verify-full
  ssl_cert: /yaml/path/to/client.crt
  ssl_key: /yaml/path/to/client.key
  ssl_root_cert: /yaml/path/to/root.crt
redis:
  host: localhost
  port: 6379
temporal:
  host: localhost:7233
  namespace: default
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_ = os.Setenv("PENFOLD_CONFIG_FILE", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Database.SSLMode != "verify-full" {
		t.Errorf("Database.SSLMode = %q, want %q", cfg.Database.SSLMode, "verify-full")
	}
	if cfg.Database.SSLCert != "/yaml/path/to/client.crt" {
		t.Errorf("Database.SSLCert = %q, want %q", cfg.Database.SSLCert, "/yaml/path/to/client.crt")
	}
	if cfg.Database.SSLKey != "/yaml/path/to/client.key" {
		t.Errorf("Database.SSLKey = %q, want %q", cfg.Database.SSLKey, "/yaml/path/to/client.key")
	}
	if cfg.Database.SSLRootCert != "/yaml/path/to/root.crt" {
		t.Errorf("Database.SSLRootCert = %q, want %q", cfg.Database.SSLRootCert, "/yaml/path/to/root.crt")
	}
}
