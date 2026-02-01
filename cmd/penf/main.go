// Package main provides the penf CLI entry point.
// penf is the command-line interface for the Penfold personal information system.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/client"
	"github.com/otherjamesbrown/penfold/cmd/penf/cmd"
	"github.com/otherjamesbrown/penfold/cmd/penf/config"
	"github.com/otherjamesbrown/penfold/cmd/penf/contextpalace"
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

	// Command logging state.
	cmdStartTime  time.Time
	cmdOutputBuf  *bytes.Buffer
	outputCapture *outputTee
)

// outputTee captures output while still writing to the original destination.
type outputTee struct {
	writer io.Writer
	buffer *bytes.Buffer
}

func (t *outputTee) Write(p []byte) (n int, err error) {
	t.buffer.Write(p)
	return t.writer.Write(p)
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "penf",
	Short: "Penfold CLI - Personal information system interface",
	Long: `penf is the command-line interface for the Penfold personal information system.

Penfold aggregates and correlates information from communication channels
(email, Slack, documents, meetings) into a queryable institutional memory.

DESIGNED FOR AI ASSISTANTS:
  This CLI is optimized for use by AI assistants (like Claude Code), not direct
  human interaction. Commands support JSON output (-o json) for structured data,
  and batch processing for intelligent bulk operations.

QUICK START:
  penf init              Initialize configuration and documentation
  penf search "query"    Search the knowledge base
  penf health            Check system status

DOCUMENTATION:
  After 'penf init', read docs/assistant-rules.md - it defines your identity,
  operating principles, and guides you to all other documentation.

  Run 'penf init' to install docs, or 'penf update' to refresh them.

FOR AI ASSISTANTS:
  - Start with docs/assistant-rules.md (your operating manual)
  - Use -o json for all commands when processing data
  - Use 'penf process <workflow> context' for batch operations`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Record start time for command logging.
		cmdStartTime = time.Now()

		// Set up output capture for command logging.
		cmdOutputBuf = &bytes.Buffer{}
		outputCapture = &outputTee{writer: os.Stdout, buffer: cmdOutputBuf}

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

		// Set output capture on the command if Context-Palace is configured.
		if cfg.ContextPalace != nil && cfg.ContextPalace.IsConfigured() {
			cmd.SetOut(outputCapture)
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

// configSetCmd sets a configuration value.
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in the config file.

Available keys:
  server_address  - API Gateway server address (host:port)
  timeout         - Request timeout (e.g., 30s, 1m)
  output_format   - Default output format (text, json, yaml)
  tenant_id       - Default tenant ID
  install_path    - Path for penf binary updates (supports ~)
  debug           - Enable debug mode (true/false)
  insecure        - Disable TLS verification (true/false)

Examples:
  penf config set server_address localhost:50051
  penf config set timeout 1m
  penf config set output_format json
  penf config set tenant_id my-tenant-123
  penf config set install_path ~/bin/penf`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		// Load current config.
		currentCfg, err := config.LoadConfig()
		if err != nil {
			// If config doesn't exist, start with defaults.
			currentCfg = config.DefaultConfig()
		}

		// Set the value.
		switch key {
		case "server_address":
			currentCfg.ServerAddress = value
		case "timeout":
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid timeout value: %w", err)
			}
			currentCfg.Timeout = duration
		case "output_format":
			format := config.OutputFormat(value)
			if !format.IsValid() {
				return fmt.Errorf("invalid output format: %s (must be text, json, or yaml)", value)
			}
			currentCfg.OutputFormat = format
		case "tenant_id":
			currentCfg.TenantID = value
		case "install_path":
			// Validate the path is expandable.
			expanded, err := config.ExpandPath(value)
			if err != nil {
				return fmt.Errorf("invalid install path: %w", err)
			}
			// Store the original value (with ~) for readability.
			currentCfg.InstallPath = value
			fmt.Printf("  (expands to: %s)\n", expanded)
		case "debug":
			if value == "true" || value == "1" {
				currentCfg.Debug = true
			} else if value == "false" || value == "0" {
				currentCfg.Debug = false
			} else {
				return fmt.Errorf("invalid debug value: %s (must be true or false)", value)
			}
		case "insecure":
			if value == "true" || value == "1" {
				currentCfg.Insecure = true
			} else if value == "false" || value == "0" {
				currentCfg.Insecure = false
			} else {
				return fmt.Errorf("invalid insecure value: %s (must be true or false)", value)
			}
		default:
			return fmt.Errorf("unknown configuration key: %s", key)
		}

		// Save the config.
		if err := config.SaveConfig(currentCfg); err != nil {
			return fmt.Errorf("saving configuration: %w", err)
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

// completionCmd generates shell completion scripts.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for penf.

To load completions:

Bash:
  $ source <(penf completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ penf completion bash > /etc/bash_completion.d/penf
  # macOS:
  $ penf completion bash > $(brew --prefix)/etc/bash_completion.d/penf

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. Execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ penf completion zsh > "${fpath[1]}/_penf"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ penf completion fish | source

  # To load completions for each session, execute once:
  $ penf completion fish > ~/.config/fish/completions/penf.fish

PowerShell:
  PS> penf completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> penf completion powershell > penf.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

// Health command flags.
var (
	healthWatch         bool
	healthWatchInterval time.Duration
)

// healthCmd checks system health status.
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check system health status",
	Long: `Check the health status of the Penfold system.

Displays the status of all services, database connections, and queue depths.
Use --json for machine-readable output or --watch for continuous monitoring.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize client.
		if err := initClient(); err != nil {
			return err
		}

		ctx := cmd.Context()

		if healthWatch {
			return runHealthWatch(ctx)
		}

		return runHealthOnce(ctx)
	},
}

// runHealthOnce performs a single health check and outputs results.
func runHealthOnce(ctx context.Context) error {
	// Create context with timeout.
	checkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	status, err := grpcClient.GetStatus(checkCtx, false)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	return outputStatus(status)
}

// runHealthWatch performs continuous health monitoring.
func runHealthWatch(ctx context.Context) error {
	ticker := time.NewTicker(healthWatchInterval)
	defer ticker.Stop()

	// Initial check.
	if err := runHealthOnce(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nStopped watching.")
			return nil
		case <-ticker.C:
			if outputFormat != "json" && outputFormat != "yaml" {
				// Clear screen for human-readable output.
				fmt.Print("\033[H\033[2J")
			}
			if err := runHealthOnce(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	}
}

// outputStatus outputs the system status in the configured format.
func outputStatus(status *client.SystemStatus) error {
	format := cfg.OutputFormat
	if outputFormat != "" {
		format = config.OutputFormat(outputFormat)
	}

	switch format {
	case config.OutputFormatJSON:
		return outputJSON(status)
	case config.OutputFormatYAML:
		return outputYAML(status)
	default:
		return outputHealthHuman(status)
	}
}

// outputJSON outputs data as JSON.
func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outputYAML outputs data as YAML.
func outputYAML(v interface{}) error {
	enc := yaml.NewEncoder(os.Stdout)
	return enc.Encode(v)
}

// outputHealthHuman outputs health status in human-readable format.
func outputHealthHuman(status *client.SystemStatus) error {
	// Overall status with color.
	statusColor := "\033[32m" // Green
	if !status.Healthy {
		statusColor = "\033[31m" // Red
	}
	fmt.Printf("System Status: %s%s\033[0m\n", statusColor, boolToStatus(status.Healthy))
	fmt.Printf("Message: %s\n", status.Message)
	fmt.Printf("Timestamp: %s\n\n", status.Timestamp.Format(time.RFC3339))

	// Services.
	fmt.Println("Services:")
	fmt.Println("  NAME             STATUS     LATENCY    VERSION")
	fmt.Println("  ----             ------     -------    -------")
	for _, svc := range status.Services {
		statusStr := statusWithColor(svc.Healthy, svc.Status)
		latencyStr := "-"
		if svc.LatencyMs > 0 {
			latencyStr = fmt.Sprintf("%.1fms", svc.LatencyMs)
		}
		fmt.Printf("  %-16s %-10s %-10s %s\n", svc.Name, statusStr, latencyStr, svc.Version)
	}
	fmt.Println()

	// Database.
	if status.Database != nil {
		db := status.Database
		dbStatus := statusWithColor(db.Healthy, db.ConnectionStatus)
		fmt.Println("Database:")
		fmt.Printf("  Type: %s\n", db.Type)
		fmt.Printf("  Status: %s\n", dbStatus)
		fmt.Printf("  Connections: %d/%d\n", db.ActiveConnections, db.MaxConnections)
		fmt.Printf("  Vector Extension: %s\n", boolToEnabled(db.VectorExtensionEnabled))
		fmt.Printf("  Content Items: %d\n", db.ContentCount)
		fmt.Printf("  Entities: %d\n", db.EntityCount)
		fmt.Printf("  Latency: %.1fms\n", db.LatencyMs)
		fmt.Println()
	}

	// Queues.
	if status.Queues != nil {
		q := status.Queues
		queueStatus := statusWithColor(q.Healthy, "healthy")
		fmt.Println("Queues:")
		fmt.Printf("  Type: %s\n", q.Type)
		fmt.Printf("  Status: %s\n", queueStatus)
		fmt.Printf("  Total Pending: %d\n", q.TotalPending)
		fmt.Printf("  Processing Rate: %.1f/min\n", q.ProcessingRate)
		if q.DeadLetterCount > 0 {
			fmt.Printf("  Dead Letter: \033[33m%d\033[0m\n", q.DeadLetterCount)
		}
		if len(q.QueueDepths) > 0 {
			fmt.Println("  Queue Depths:")
			for name, depth := range q.QueueDepths {
				fmt.Printf("    %s: %d\n", name, depth)
			}
		}
		fmt.Println()
	}

	// Version info.
	if status.Version != nil {
		v := status.Version
		fmt.Println("Version:")
		fmt.Printf("  Version: %s\n", v.Version)
		fmt.Printf("  Commit: %s\n", v.Commit)
		fmt.Printf("  Build Time: %s\n", v.BuildTime)
		fmt.Printf("  Go Version: %s\n", v.GoVersion)
	}

	return nil
}

// boolToStatus converts a boolean to a status string.
func boolToStatus(healthy bool) string {
	if healthy {
		return "HEALTHY"
	}
	return "UNHEALTHY"
}

// boolToEnabled converts a boolean to an enabled/disabled string.
func boolToEnabled(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// statusWithColor returns a colored status string.
func statusWithColor(healthy bool, status string) string {
	if healthy {
		if status == "" {
			return "\033[32mhealthy\033[0m"
		}
		return fmt.Sprintf("\033[32m%s\033[0m", status)
	}
	if status == "" {
		return "\033[31munhealthy\033[0m"
	}
	return fmt.Sprintf("\033[31m%s\033[0m", status)
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

	// Load TLS configuration if not running in insecure mode.
	if !cfg.Insecure && cfg.TLS.Enabled {
		tlsConfig, err := client.LoadClientTLSConfig(&cfg.TLS)
		if err != nil {
			return fmt.Errorf("loading TLS config: %w", err)
		}
		opts.TLSConfig = tlsConfig
	}

	// Add tenant ID to default metadata if configured.
	tenantID := getTenantID()
	if tenantID != "" {
		opts.TenantID = tenantID
	}

	grpcClient = client.NewGRPCClient(cfg.ServerAddress, opts)

	// Create context with timeout for connection.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := grpcClient.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}

	return nil
}

// getTenantID returns the current tenant ID from environment or config.
func getTenantID() string {
	if envTenant := os.Getenv("PENF_TENANT_ID"); envTenant != "" {
		return envTenant
	}
	if cfg != nil {
		return cfg.TenantID
	}
	return ""
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

	// Health command flags.
	healthCmd.Flags().BoolVarP(&healthWatch, "watch", "w", false, "Continuously monitor health status")
	healthCmd.Flags().DurationVar(&healthWatchInterval, "interval", 5*time.Second, "Watch interval (default 5s)")

	// Health subcommands.
	healthCmd.AddCommand(cmd.NewHealthLocalCommand())
	healthCmd.AddCommand(cmd.NewHealthGatewayCommand())

	// Add commands.
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(cmd.AuthCmd)
	rootCmd.AddCommand(cmd.NewTenantCommand(nil))
	rootCmd.AddCommand(cmd.NewSearchCommand(nil))
	rootCmd.AddCommand(cmd.NewIngestCommand(nil))
	rootCmd.AddCommand(cmd.NewReviewCommand(nil))
	rootCmd.AddCommand(cmd.NewAICommand(nil))
	rootCmd.AddCommand(cmd.NewWorkflowCommand(nil))
	rootCmd.AddCommand(cmd.NewLogsCommand(nil))
	rootCmd.AddCommand(cmd.NewDebugCommand(nil))
	rootCmd.AddCommand(cmd.NewPipelineCommand(nil))
	rootCmd.AddCommand(cmd.NewGlossaryCommand(nil))
	rootCmd.AddCommand(cmd.NewContentCommand(nil))
	rootCmd.AddCommand(cmd.NewRelationshipCommand(nil))
	rootCmd.AddCommand(cmd.NewProductCommand(nil))
	rootCmd.AddCommand(cmd.NewProjectCommand(nil))
	rootCmd.AddCommand(cmd.NewTeamCommand(nil))
	rootCmd.AddCommand(cmd.NewProcessCommand(nil))
	rootCmd.AddCommand(cmd.NewInitCommand())
	rootCmd.AddCommand(cmd.NewUpdateCommand(version))
	rootCmd.AddCommand(cmd.NewFeedbackCommand(version))
	rootCmd.AddCommand(cmd.NewAuditCommand(nil))
	rootCmd.AddCommand(cmd.NewModelCommand(nil))
	rootCmd.AddCommand(cmd.NewTraceCommand(nil))
	rootCmd.AddCommand(cmd.NewCertCommand())
	rootCmd.AddCommand(cmd.NewContextCommand(nil))
	rootCmd.AddCommand(cmd.NewMemoryCommand(nil))
	rootCmd.AddCommand(cmd.NewBacklogCommand(nil))
	rootCmd.AddCommand(cmd.NewSessionCommand(nil))
	rootCmd.AddCommand(cmd.NewMessageCommand(nil))
	rootCmd.AddCommand(cmd.NewMeetingCommand(nil))

	// Config subcommands.
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configSetCmd)
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

	// Execute root command and capture the error for logging.
	cmdErr := rootCmd.ExecuteContext(ctx)

	// Log the command to Context-Palace (called here to capture both success and failure).
	logCommandExecution(os.Args, cmdErr)

	if cmdErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", cmdErr)
		os.Exit(1)
	}
}

// logCommandExecution logs the CLI command to Context-Palace.
// This is best-effort - errors are logged to stderr but don't affect the command result.
func logCommandExecution(args []string, cmdErr error) {
	// Skip if config not loaded or Context-Palace not configured.
	if cfg == nil || cfg.ContextPalace == nil || !cfg.ContextPalace.IsConfigured() {
		return
	}

	// Skip logging for certain commands.
	if len(args) > 1 {
		cmd := args[1]
		if cmd == "version" || cmd == "help" || cmd == "completion" || cmd == "context" {
			return
		}
	}

	// Calculate duration.
	duration := time.Since(cmdStartTime)

	// Build the command entry.
	entry := &contextpalace.CommandEntry{
		Command:     getCommandName(args),
		Args:        getCommandArgs(args),
		FullCommand: strings.Join(args, " "),
		DurationMs:  int(duration.Milliseconds()),
		Success:     cmdErr == nil,
		TenantID:    cfg.TenantID,
	}

	// Capture error message if command failed.
	if cmdErr != nil {
		entry.ErrorMessage = cmdErr.Error()
	}

	// Capture response output.
	if cmdOutputBuf != nil {
		entry.Response = cmdOutputBuf.String()
	}

	// Connect to Context-Palace and log.
	cpClient, err := contextpalace.NewClient(cfg.ContextPalace)
	if err != nil {
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "Warning: failed to connect to Context-Palace: %v\n", err)
		}
		return
	}
	defer cpClient.Close()

	logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cpClient.LogCommand(logCtx, entry); err != nil {
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "Warning: failed to log command to Context-Palace: %v\n", err)
		}
	}
}

// getCommandName extracts the command name from args (e.g., "search" from ["penf", "search", "query"]).
func getCommandName(args []string) string {
	if len(args) < 2 {
		return "penf"
	}
	// Find the first non-flag argument after "penf".
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			return args[i]
		}
	}
	return "penf"
}

// getCommandArgs extracts the arguments after the command name.
func getCommandArgs(args []string) []string {
	if len(args) < 3 {
		return nil
	}
	// Find the command name index and return everything after it.
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			return args[i+1:]
		}
	}
	return nil
}
