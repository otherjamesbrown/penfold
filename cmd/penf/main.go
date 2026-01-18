// Package main provides the penf CLI entry point.
// penf is the command-line interface for the Penfold personal information system.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// Version information (set via ldflags at build time).
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// Global flags and state.
var (
	cfgFile      string
	serverAddr   string
	timeout      time.Duration
	outputFormat string
	debug        bool
	insecure     bool

	// cfg holds the loaded configuration.
	cfg *config.CLIConfig

	// grpcClient is the shared gRPC client.
	grpcClient *client.GRPCClient
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "penf",
	Short: "Penfold CLI - Personal information system interface",
	Long: `penf is the command-line interface for the Penfold personal information system.

Penfold aggregates and correlates information from communication channels
(email, Slack, documents, meetings) into a queryable institutional memory.

Use penf to search, ingest content, review daily briefings, and manage
your personal knowledge base.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip initialization for commands that don't need it.
		if cmd.Name() == "version" || cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}

		// Load configuration.
		var err error
		cfg, err = config.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}

		// Override with command-line flags.
		if serverAddr != "" {
			cfg.ServerAddress = serverAddr
		}
		if timeout != 0 {
			cfg.Timeout = timeout
		}
		if outputFormat != "" {
			cfg.OutputFormat = config.OutputFormat(outputFormat)
		}
		if debug {
			cfg.Debug = true
		}
		if insecure {
			cfg.Insecure = true
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		// Clean up gRPC client if initialized.
		if grpcClient != nil {
			return grpcClient.Close()
		}
		return nil
	},
}

// versionCmd prints version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit hash, and build time of the penf CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("penf version %s\n", version)
		fmt.Printf("  commit:     %s\n", commit)
		fmt.Printf("  built:      %s\n", buildTime)
	},
}

// statusCmd checks the connection status to the API Gateway.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check connection status to the API Gateway",
	Long:  `Check the connection status to the Penfold API Gateway and display service health information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize client.
		if err := initClient(); err != nil {
			return err
		}

		// Create context with timeout.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
		defer cancel()

		// Perform health check.
		if err := grpcClient.HealthCheck(ctx); err != nil {
			fmt.Printf("Connection status: UNHEALTHY\n")
			fmt.Printf("  Server:  %s\n", cfg.ServerAddress)
			fmt.Printf("  State:   %s\n", grpcClient.ConnectionState())
			fmt.Printf("  Error:   %s\n", err)
			return nil // Don't return error, just report status.
		}

		fmt.Printf("Connection status: HEALTHY\n")
		fmt.Printf("  Server:  %s\n", cfg.ServerAddress)
		fmt.Printf("  State:   %s\n", grpcClient.ConnectionState())
		return nil
	},
}

// configCmd manages CLI configuration.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `View and modify the penf CLI configuration settings.`,
}

// configShowCmd displays current configuration.
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the current CLI configuration values.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config (uses PersistentPreRunE, so cfg is already loaded).
		if cfg == nil {
			var err error
			cfg, err = config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading configuration: %w", err)
			}
		}

		configPath, _ := config.ConfigPath()

		fmt.Println("Current configuration:")
		fmt.Printf("  Config file:    %s\n", configPath)
		fmt.Printf("  Server address: %s\n", cfg.ServerAddress)
		fmt.Printf("  Timeout:        %s\n", cfg.Timeout)
		fmt.Printf("  Output format:  %s\n", cfg.OutputFormat)
		fmt.Printf("  Tenant ID:      %s\n", valueOrDefault(cfg.TenantID, "(not set)"))
		fmt.Printf("  Debug:          %t\n", cfg.Debug)
		fmt.Printf("  Insecure:       %t\n", cfg.Insecure)

		return nil
	},
}

// configInitCmd initializes configuration.
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long:  `Create a new configuration file with default values if one doesn't exist.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := config.ConfigPath()
		if err != nil {
			return fmt.Errorf("getting config path: %w", err)
		}

		// Check if config already exists.
		if _, err := os.Stat(configPath); err == nil {
			fmt.Printf("Configuration file already exists: %s\n", configPath)
			fmt.Println("Use 'penf config show' to view current settings.")
			return nil
		}

		// Create default config.
		defaultCfg := config.DefaultConfig()
		if err := config.SaveConfig(defaultCfg); err != nil {
			return fmt.Errorf("saving configuration: %w", err)
		}

		fmt.Printf("Created configuration file: %s\n", configPath)
		fmt.Println("\nDefault settings:")
		fmt.Printf("  Server address: %s\n", defaultCfg.ServerAddress)
		fmt.Printf("  Timeout:        %s\n", defaultCfg.Timeout)
		fmt.Printf("  Output format:  %s\n", defaultCfg.OutputFormat)

		return nil
	},
}

// initClient initializes the gRPC client if not already initialized.
func initClient() error {
	if grpcClient != nil {
		return nil
	}

	opts := client.DefaultOptions()
	opts.Insecure = cfg.Insecure
	opts.Debug = cfg.Debug
	opts.ConnectTimeout = cfg.Timeout

	grpcClient = client.NewGRPCClient(cfg.ServerAddress, opts)

	// Create context with timeout for connection.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := grpcClient.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}

	return nil
}

// valueOrDefault returns the value if non-empty, otherwise the default.
func valueOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func init() {
	// Global flags.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.penf/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "", "API Gateway server address (host:port)")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 0, "request timeout (e.g., 30s, 1m)")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "", "output format: text, json, yaml")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "disable TLS verification")

	// Add commands.
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)

	// Config subcommands.
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
}

func main() {
	// Set up signal handling for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, shutting down...")
		cancel()
		if grpcClient != nil {
			_ = grpcClient.Close()
		}
		os.Exit(0)
	}()

	// Execute root command.
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
