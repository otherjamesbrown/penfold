// Package config provides CLI configuration management for the penf command-line tool.
// It supports loading configuration from YAML files, environment variables, and command-line flags.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OutputFormat defines the supported output formats for CLI results.
type OutputFormat string

const (
	// OutputFormatText is human-readable plain text output.
	OutputFormatText OutputFormat = "text"
	// OutputFormatJSON is JSON-formatted output for machine processing.
	OutputFormatJSON OutputFormat = "json"
	// OutputFormatYAML is YAML-formatted output for machine processing.
	OutputFormatYAML OutputFormat = "yaml"
)

// Default configuration values.
const (
	DefaultServerAddress        = "localhost:50051"
	DefaultSearchServiceAddress = "localhost:50053"
	DefaultTimeout              = 30 * time.Second
	DefaultOutputFormat         = OutputFormatText
	DefaultConfigDir            = ".penf"
	DefaultConfigFile           = "config.yaml"
	DefaultCertDir              = ".config/penf/certs"
)

// TLSConfig holds client TLS settings.
type TLSConfig struct {
	// Enabled indicates whether TLS should be used for connections.
	Enabled bool `yaml:"enabled"`

	// CACert is the path to the CA certificate for verifying the server.
	CACert string `yaml:"ca_cert"`

	// ClientCert is the path to the client certificate for mTLS authentication.
	ClientCert string `yaml:"client_cert"`

	// ClientKey is the path to the client private key for mTLS authentication.
	ClientKey string `yaml:"client_key"`

	// CertDir is a directory containing ca.crt, client.crt, and client.key files.
	// If set, it provides default paths for CACert, ClientCert, and ClientKey.
	CertDir string `yaml:"cert_dir"`

	// SkipVerify disables server certificate verification (insecure, for testing only).
	SkipVerify bool `yaml:"skip_verify"`
}

// ResolvePaths expands ~ in paths and sets defaults from CertDir if configured.
func (c *TLSConfig) ResolvePaths() {
	if c.CertDir != "" {
		c.CertDir = expandPath(c.CertDir)
		if c.CACert == "" {
			c.CACert = filepath.Join(c.CertDir, "ca.crt")
		}
		if c.ClientCert == "" {
			c.ClientCert = filepath.Join(c.CertDir, "client.crt")
		}
		if c.ClientKey == "" {
			c.ClientKey = filepath.Join(c.CertDir, "client.key")
		}
	} else {
		c.CACert = expandPath(c.CACert)
		c.ClientCert = expandPath(c.ClientCert)
		c.ClientKey = expandPath(c.ClientKey)
	}
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path // Return original if home dir lookup fails.
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// CLIConfig holds the CLI configuration settings.
type CLIConfig struct {
	// ServerAddress is the address of the API Gateway (host:port).
	ServerAddress string `yaml:"server_address"`

	// SearchServiceAddress is the address of the Search service (host:port).
	// If empty, search commands will use the gateway address.
	SearchServiceAddress string `yaml:"search_service_address,omitempty"`

	// Timeout is the default timeout for API requests.
	Timeout time.Duration `yaml:"timeout"`

	// OutputFormat specifies the default output format for commands.
	OutputFormat OutputFormat `yaml:"output_format"`

	// TenantID is the default tenant identifier for multi-tenant operations.
	TenantID string `yaml:"tenant_id,omitempty"`

	// TenantAliases provides user-friendly names for tenant IDs.
	// Example: {"work": "tenant-acme-123", "personal": "tenant-default-001"}
	TenantAliases map[string]string `yaml:"tenant_aliases,omitempty"`

	// InstallPath is the path where penf should be installed during updates.
	// If empty, uses the current executable's location.
	// Supports ~ for home directory expansion.
	InstallPath string `yaml:"install_path,omitempty"`

	// Debug enables verbose debug logging.
	Debug bool `yaml:"debug,omitempty"`

	// Insecure disables TLS verification (for development only).
	Insecure bool `yaml:"insecure,omitempty"`

	// TLS contains the TLS/mTLS configuration settings.
	TLS TLSConfig `yaml:"tls"`
}

// DefaultConfig returns a CLIConfig with default values.
func DefaultConfig() *CLIConfig {
	return &CLIConfig{
		ServerAddress: DefaultServerAddress,
		Timeout:       DefaultTimeout,
		OutputFormat:  DefaultOutputFormat,
		// Insecure defaults to false (secure). Use --insecure flag or PENF_INSECURE=true for development.
	}
}

// ConfigDir returns the configuration directory path.
// Uses $PENF_CONFIG_DIR if set, otherwise ~/.penf
func ConfigDir() (string, error) {
	if dir := os.Getenv("PENF_CONFIG_DIR"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	return filepath.Join(home, DefaultConfigDir), nil
}

// ConfigPath returns the full path to the configuration file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultConfigFile), nil
}

// LoadConfig loads the CLI configuration from file and environment variables.
// Configuration is loaded in this order (later sources override earlier):
// 1. Default values
// 2. Config file (~/.penf/config.yaml or $PENF_CONFIG_DIR/config.yaml)
// 3. Environment variables (PENF_SERVER_ADDRESS, PENF_TIMEOUT, PENF_OUTPUT_FORMAT)
func LoadConfig() (*CLIConfig, error) {
	cfg := DefaultConfig()

	// Try to load from config file.
	configPath, err := ConfigPath()
	if err != nil {
		return nil, fmt.Errorf("getting config path: %w", err)
	}

	if _, err := os.Stat(configPath); err == nil {
		if err := loadFromFile(cfg, configPath); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Overlay environment variables.
	loadFromEnv(cfg)

	// Validate the configuration.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// loadFromFile loads configuration from a YAML file.
func loadFromFile(cfg *CLIConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	// We need a temp struct for unmarshaling duration as string.
	type configFile struct {
		ServerAddress        string            `yaml:"server_address"`
		SearchServiceAddress string            `yaml:"search_service_address"`
		Timeout              string            `yaml:"timeout"`
		OutputFormat         OutputFormat      `yaml:"output_format"`
		TenantID             string            `yaml:"tenant_id"`
		TenantAliases        map[string]string `yaml:"tenant_aliases"`
		InstallPath          string            `yaml:"install_path"`
		Debug                bool              `yaml:"debug"`
		Insecure             bool              `yaml:"insecure"`
		TLS                  TLSConfig         `yaml:"tls"`
	}

	var fileCfg configFile
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	if fileCfg.ServerAddress != "" {
		cfg.ServerAddress = fileCfg.ServerAddress
	}
	if fileCfg.SearchServiceAddress != "" {
		cfg.SearchServiceAddress = fileCfg.SearchServiceAddress
	}
	if fileCfg.Timeout != "" {
		timeout, err := time.ParseDuration(fileCfg.Timeout)
		if err != nil {
			return fmt.Errorf("parsing timeout: %w", err)
		}
		cfg.Timeout = timeout
	}
	if fileCfg.OutputFormat != "" {
		cfg.OutputFormat = fileCfg.OutputFormat
	}
	if fileCfg.TenantID != "" {
		cfg.TenantID = fileCfg.TenantID
	}
	if fileCfg.TenantAliases != nil {
		cfg.TenantAliases = fileCfg.TenantAliases
	}
	if fileCfg.InstallPath != "" {
		cfg.InstallPath = fileCfg.InstallPath
	}
	cfg.Debug = fileCfg.Debug
	cfg.Insecure = fileCfg.Insecure
	cfg.TLS = fileCfg.TLS

	return nil
}

// loadFromEnv overlays environment variables onto the configuration.
func loadFromEnv(cfg *CLIConfig) {
	if v := os.Getenv("PENF_SERVER_ADDRESS"); v != "" {
		cfg.ServerAddress = v
	}

	if v := os.Getenv("PENF_SEARCH_SERVICE_ADDRESS"); v != "" {
		cfg.SearchServiceAddress = v
	}

	if v := os.Getenv("PENF_TIMEOUT"); v != "" {
		if timeout, err := time.ParseDuration(v); err == nil {
			cfg.Timeout = timeout
		}
	}

	if v := os.Getenv("PENF_OUTPUT_FORMAT"); v != "" {
		cfg.OutputFormat = OutputFormat(v)
	}

	if v := os.Getenv("PENF_TENANT_ID"); v != "" {
		cfg.TenantID = v
	}

	if v := os.Getenv("PENF_INSTALL_PATH"); v != "" {
		cfg.InstallPath = v
	}

	if v := os.Getenv("PENF_DEBUG"); v == "true" || v == "1" {
		cfg.Debug = true
	}

	if v := os.Getenv("PENF_INSECURE"); v == "true" || v == "1" {
		cfg.Insecure = true
	}

	// TLS environment variables.
	if v := os.Getenv("PENF_TLS_ENABLED"); v == "true" || v == "1" {
		cfg.TLS.Enabled = true
	}

	if v := os.Getenv("PENF_TLS_CA_CERT"); v != "" {
		cfg.TLS.CACert = v
	}

	if v := os.Getenv("PENF_TLS_CLIENT_CERT"); v != "" {
		cfg.TLS.ClientCert = v
	}

	if v := os.Getenv("PENF_TLS_CLIENT_KEY"); v != "" {
		cfg.TLS.ClientKey = v
	}

	if v := os.Getenv("PENF_TLS_CERT_DIR"); v != "" {
		cfg.TLS.CertDir = v
	}

	if v := os.Getenv("PENF_TLS_SKIP_VERIFY"); v == "true" || v == "1" {
		cfg.TLS.SkipVerify = true
	}
}

// Validate checks that the configuration is valid.
func (c *CLIConfig) Validate() error {
	if c.ServerAddress == "" {
		return fmt.Errorf("server_address is required")
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if !c.OutputFormat.IsValid() {
		return fmt.Errorf("invalid output_format: %q (must be text, json, or yaml)", c.OutputFormat)
	}

	return nil
}

// IsValid checks if the output format is valid.
func (f OutputFormat) IsValid() bool {
	switch f {
	case OutputFormatText, OutputFormatJSON, OutputFormatYAML:
		return true
	default:
		return false
	}
}

// String returns the string representation of the output format.
func (f OutputFormat) String() string {
	return string(f)
}

// SaveConfig saves the configuration to the config file.
func SaveConfig(cfg *CLIConfig) error {
	configDir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("getting config directory: %w", err)
	}

	// Ensure config directory exists.
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, DefaultConfigFile)

	// Convert to YAML-friendly format with duration as string.
	type configFile struct {
		ServerAddress        string            `yaml:"server_address"`
		SearchServiceAddress string            `yaml:"search_service_address,omitempty"`
		Timeout              string            `yaml:"timeout"`
		OutputFormat         OutputFormat      `yaml:"output_format"`
		TenantID             string            `yaml:"tenant_id,omitempty"`
		TenantAliases        map[string]string `yaml:"tenant_aliases,omitempty"`
		InstallPath          string            `yaml:"install_path,omitempty"`
		Debug                bool              `yaml:"debug,omitempty"`
		Insecure             bool              `yaml:"insecure,omitempty"`
		TLS                  TLSConfig         `yaml:"tls,omitempty"`
	}

	fileCfg := configFile{
		ServerAddress:        cfg.ServerAddress,
		SearchServiceAddress: cfg.SearchServiceAddress,
		Timeout:              cfg.Timeout.String(),
		OutputFormat:         cfg.OutputFormat,
		TenantID:             cfg.TenantID,
		TenantAliases:        cfg.TenantAliases,
		InstallPath:          cfg.InstallPath,
		Debug:                cfg.Debug,
		Insecure:             cfg.Insecure,
		TLS:                  cfg.TLS,
	}

	data, err := yaml.Marshal(&fileCfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// EnsureConfigDir creates the configuration directory if it doesn't exist.
func EnsureConfigDir() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("getting home directory: %w", err)
		}
		return filepath.Join(home, path[1:]), nil
	}
	return path, nil
}

// GetInstallPath returns the expanded install path from config,
// or falls back to the current executable's location.
func (c *CLIConfig) GetInstallPath() (string, error) {
	if c.InstallPath != "" {
		return ExpandPath(c.InstallPath)
	}

	// Fall back to current executable location.
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting executable path: %w", err)
	}
	return filepath.EvalSymlinks(execPath)
}

// GetSearchServiceAddress returns the search service address.
// If not configured, returns the default search service address.
func (c *CLIConfig) GetSearchServiceAddress() string {
	if c.SearchServiceAddress != "" {
		return c.SearchServiceAddress
	}
	return DefaultSearchServiceAddress
}
