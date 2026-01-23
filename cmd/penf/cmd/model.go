// Package cmd provides CLI commands for the penf tool.
package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/otherjamesbrown/penfold/cmd/penf/config"
)

// ModelProfile represents a configured LLM model profile.
type ModelProfile struct {
	Name            string `json:"name" yaml:"name"`
	Type            string `json:"type" yaml:"type"`
	Model           string `json:"model" yaml:"model"`
	Endpoint        string `json:"endpoint" yaml:"endpoint"`
	APIKeyFile      string `json:"api_key_file,omitempty" yaml:"api_key_file,omitempty"`
	ExpectedLatency string `json:"expected_latency,omitempty" yaml:"expected_latency,omitempty"`
	DownloadSize    string `json:"download_size,omitempty" yaml:"download_size,omitempty"`
	Notes           string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// ModelServerStatus represents the status of a running model server.
type ModelServerStatus struct {
	PID       int       `json:"pid" yaml:"pid"`
	Model     string    `json:"model" yaml:"model"`
	Port      int       `json:"port" yaml:"port"`
	CPUPct    float64   `json:"cpu_pct" yaml:"cpu_pct"`
	MemPct    float64   `json:"mem_pct" yaml:"mem_pct"`
	StartedAt time.Time `json:"started_at" yaml:"started_at"`
	Healthy   bool      `json:"healthy" yaml:"healthy"`
}

// ModelCatalogEntry represents a model in the catalog.
type ModelCatalogEntry struct {
	ID              string `json:"id" yaml:"id"`
	Name            string `json:"name" yaml:"name"`
	Size            string `json:"size" yaml:"size"`
	Type            string `json:"type" yaml:"type"` // mlx, gguf, etc.
	Downloaded      bool   `json:"downloaded" yaml:"downloaded"`
	ExpectedLatency string `json:"expected_latency,omitempty" yaml:"expected_latency,omitempty"`
	MemoryRequired  string `json:"memory_required,omitempty" yaml:"memory_required,omitempty"`
}

// ModelConfig holds the model configuration from llm-models.yaml.
type ModelConfig struct {
	DefaultProfile string                  `yaml:"default_profile"`
	Profiles       map[string]ModelProfile `yaml:"profiles"`
}

// Model command flags.
var (
	modelOutput string
	modelPort   int
)

// Default model catalog (built-in known models).
var defaultModelCatalog = []ModelCatalogEntry{
	{
		ID:              "mlx-community/Qwen2.5-32B-Instruct-4bit",
		Name:            "Qwen 2.5 32B (4-bit)",
		Size:            "~18GB",
		Type:            "mlx",
		ExpectedLatency: "10-20s",
		MemoryRequired:  "20GB+",
	},
	{
		ID:              "mlx-community/Qwen2.5-7B-Instruct-4bit",
		Name:            "Qwen 2.5 7B (4-bit)",
		Size:            "~4GB",
		Type:            "mlx",
		ExpectedLatency: "3-8s",
		MemoryRequired:  "6GB+",
	},
	{
		ID:              "mlx-community/Phi-3.5-mini-instruct-4bit",
		Name:            "Phi-3.5 Mini (4-bit)",
		Size:            "~2.5GB",
		Type:            "mlx",
		ExpectedLatency: "1-4s",
		MemoryRequired:  "4GB+",
	},
	{
		ID:              "mlx-community/Llama-3.2-3B-Instruct-4bit",
		Name:            "Llama 3.2 3B (4-bit)",
		Size:            "~2GB",
		Type:            "mlx",
		ExpectedLatency: "1-3s",
		MemoryRequired:  "3GB+",
	},
	{
		ID:              "mlx-community/gemma-2-9b-it-4bit",
		Name:            "Gemma 2 9B (4-bit)",
		Size:            "~6GB",
		Type:            "mlx",
		ExpectedLatency: "3-8s",
		MemoryRequired:  "8GB+",
	},
}

// ModelCommandDeps holds the dependencies for model commands.
type ModelCommandDeps struct {
	Config       *config.CLIConfig
	LoadConfig   func() (*config.CLIConfig, error)
	ConfigDir    string
	SidecarDir   string
}

// DefaultModelDeps returns the default dependencies for production use.
func DefaultModelDeps() *ModelCommandDeps {
	homeDir, _ := os.UserHomeDir()
	return &ModelCommandDeps{
		LoadConfig: config.LoadConfig,
		ConfigDir:  filepath.Join(homeDir, ".config", "penfold"),
		SidecarDir: filepath.Join(homeDir, "github", "otherjamesbrown", "penfold", "penfold-go-pipeline", "sidecar"),
	}
}

// NewModelCommand creates the root model command with all subcommands.
func NewModelCommand(deps *ModelCommandDeps) *cobra.Command {
	if deps == nil {
		deps = DefaultModelDeps()
	}

	cmd := &cobra.Command{
		Use:   "model",
		Short: "Manage local LLM models",
		Long: `Manage local MLX LLM models for Penfold.

The model command provides tools to manage the local LLM models used for
entity extraction, mention resolution, and other AI-powered features.

Features:
  - List available and downloaded models
  - Start/stop model servers on different ports
  - Switch between models
  - Monitor running servers
  - Configure model profiles

Examples:
  # List available models
  penf model list

  # Show running servers
  penf model status

  # Start a model server
  penf model serve phi --port 8080

  # Stop a server
  penf model stop --port 8080

  # Switch models on a running server
  penf model switch qwen-7b`,
		Aliases: []string{"models", "llm"},
	}

	// Persistent flags.
	cmd.PersistentFlags().StringVarP(&modelOutput, "output", "o", "", "Output format: text, json, yaml")

	// Add subcommands.
	cmd.AddCommand(newModelListCommand(deps))
	cmd.AddCommand(newModelStatusCommand(deps))
	cmd.AddCommand(newModelServeCommand(deps))
	cmd.AddCommand(newModelStopCommand(deps))
	cmd.AddCommand(newModelSwitchCommand(deps))
	cmd.AddCommand(newModelInfoCommand(deps))
	cmd.AddCommand(newModelBenchCommand(deps))

	return cmd
}

// newModelListCommand creates the 'model list' subcommand.
func newModelListCommand(deps *ModelCommandDeps) *cobra.Command {
	var showAll bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available models",
		Long: `List available LLM models.

Shows both downloaded models and models available for download.
By default, only shows downloaded models. Use --all to show all available models.

Examples:
  penf model list
  penf model list --all
  penf model list -o json`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelList(cmd.Context(), deps, showAll)
		},
	}

	cmd.Flags().BoolVar(&showAll, "all", false, "Show all available models (including not downloaded)")

	return cmd
}

// newModelStatusCommand creates the 'model status' subcommand.
func newModelStatusCommand(deps *ModelCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show running model servers",
		Long: `Display the status of running MLX model servers.

Shows all running mlx_lm.server processes with their model, port,
CPU/memory usage, and health status.

Examples:
  penf model status
  penf model status -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelStatus(cmd.Context(), deps)
		},
	}
}

// newModelServeCommand creates the 'model serve' subcommand.
func newModelServeCommand(deps *ModelCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve <model>",
		Short: "Start a model server",
		Long: `Start an MLX model server.

Starts a new mlx_lm.server process serving the specified model.
The model can be specified by:
  - Profile name (e.g., "phi", "qwen-7b")
  - Full model ID (e.g., "mlx-community/Phi-3.5-mini-instruct-4bit")

Examples:
  penf model serve phi
  penf model serve phi --port 8080
  penf model serve mlx-community/Qwen2.5-7B-Instruct-4bit --port 8081`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelServe(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().IntVar(&modelPort, "port", 8080, "Port to serve on")

	return cmd
}

// newModelStopCommand creates the 'model stop' subcommand.
func newModelStopCommand(deps *ModelCommandDeps) *cobra.Command {
	var stopAll bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop model server(s)",
		Long: `Stop running MLX model server(s).

Stops the server running on the specified port, or all servers with --all.

Examples:
  penf model stop --port 8080
  penf model stop --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelStop(cmd.Context(), deps, stopAll)
		},
	}

	cmd.Flags().IntVar(&modelPort, "port", 8080, "Port of server to stop")
	cmd.Flags().BoolVar(&stopAll, "all", false, "Stop all model servers")

	return cmd
}

// newModelSwitchCommand creates the 'model switch' subcommand.
func newModelSwitchCommand(deps *ModelCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "switch <model>",
		Short: "Switch model on a running server",
		Long: `Switch the model running on a server.

Stops the current server and starts a new one with the specified model.
The port defaults to 8080 unless specified.

Examples:
  penf model switch phi
  penf model switch qwen-7b --port 8081`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelSwitch(cmd.Context(), deps, args[0])
		},
	}

	cmd.Flags().IntVar(&modelPort, "port", 8080, "Port of server to switch")

	return cmd
}

// newModelInfoCommand creates the 'model info' subcommand.
func newModelInfoCommand(deps *ModelCommandDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "info <model>",
		Short: "Show model details",
		Long: `Display detailed information about a model.

Shows the model's size, type, expected latency, memory requirements,
and download status.

Examples:
  penf model info phi
  penf model info mlx-community/Phi-3.5-mini-instruct-4bit`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelInfo(cmd.Context(), deps, args[0])
		},
	}
}

// newModelBenchCommand creates the 'model bench' subcommand.
func newModelBenchCommand(deps *ModelCommandDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark model server",
		Long: `Run a quick benchmark against a model server.

Sends test requests to measure response latency.

Examples:
  penf model bench
  penf model bench --port 8081`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelBench(cmd.Context(), deps)
		},
	}

	cmd.Flags().IntVar(&modelPort, "port", 8080, "Port of server to benchmark")

	return cmd
}

// ==================== Command Execution Functions ====================

// runModelList executes the model list command.
func runModelList(ctx context.Context, deps *ModelCommandDeps, showAll bool) error {
	// Get list of downloaded models from HuggingFace cache.
	downloadedModels := getDownloadedModels(deps)

	// Build catalog with download status.
	catalog := make([]ModelCatalogEntry, len(defaultModelCatalog))
	copy(catalog, defaultModelCatalog)

	for i := range catalog {
		catalog[i].Downloaded = downloadedModels[catalog[i].ID]
	}

	// Filter to downloaded only unless --all.
	if !showAll {
		filtered := make([]ModelCatalogEntry, 0)
		for _, m := range catalog {
			if m.Downloaded {
				filtered = append(filtered, m)
			}
		}
		catalog = filtered
	}

	return outputModelCatalog(deps, catalog, showAll)
}

// runModelStatus executes the model status command.
func runModelStatus(ctx context.Context, deps *ModelCommandDeps) error {
	servers, err := getRunningServers()
	if err != nil {
		return fmt.Errorf("getting running servers: %w", err)
	}

	return outputModelStatus(deps, servers)
}

// runModelServe executes the model serve command.
func runModelServe(ctx context.Context, deps *ModelCommandDeps, model string) error {
	// Resolve model name to full ID.
	modelID := resolveModelID(deps, model)

	// Check if port is already in use.
	servers, _ := getRunningServers()
	for _, s := range servers {
		if s.Port == modelPort {
			return fmt.Errorf("port %d is already in use by model %s (PID %d)", modelPort, s.Model, s.PID)
		}
	}

	fmt.Printf("Starting model server...\n")
	fmt.Printf("  Model: %s\n", modelID)
	fmt.Printf("  Port:  %d\n\n", modelPort)

	// Build the command.
	venvPython := filepath.Join(deps.SidecarDir, ".venv", "bin", "python")
	cmdArgs := []string{
		"-m", "mlx_lm.server",
		"--model", modelID,
		"--port", strconv.Itoa(modelPort),
		"--host", "0.0.0.0",
	}

	// Start the server in the background.
	execCmd := exec.Command(venvPython, cmdArgs...)
	execCmd.Dir = deps.SidecarDir

	// Redirect output to log file.
	logFile := fmt.Sprintf("/tmp/mlx-server-%d.log", modelPort)
	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}
	execCmd.Stdout = f
	execCmd.Stderr = f

	// Start the process.
	if err := execCmd.Start(); err != nil {
		f.Close()
		return fmt.Errorf("starting server: %w", err)
	}

	fmt.Printf("Server starting in background (PID: %d)\n", execCmd.Process.Pid)
	fmt.Printf("Log file: %s\n\n", logFile)

	// Wait for server to be ready.
	fmt.Print("Waiting for server to be ready")
	endpoint := fmt.Sprintf("http://localhost:%d/v1/models", modelPort)

	for i := 0; i < 60; i++ {
		resp, err := http.Get(endpoint)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			fmt.Printf("\n\n\033[32mServer is ready!\033[0m\n")
			fmt.Printf("Endpoint: http://localhost:%d/v1/chat/completions\n", modelPort)
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		fmt.Print(".")
		time.Sleep(2 * time.Second)
	}

	fmt.Println()
	return fmt.Errorf("server did not start within 2 minutes - check %s", logFile)
}

// runModelStop executes the model stop command.
func runModelStop(ctx context.Context, deps *ModelCommandDeps, stopAll bool) error {
	servers, err := getRunningServers()
	if err != nil {
		return fmt.Errorf("getting running servers: %w", err)
	}

	if len(servers) == 0 {
		fmt.Println("No model servers are running.")
		return nil
	}

	stopped := 0
	for _, s := range servers {
		if stopAll || s.Port == modelPort {
			process, err := os.FindProcess(s.PID)
			if err != nil {
				fmt.Printf("Warning: could not find process %d: %v\n", s.PID, err)
				continue
			}

			if err := process.Signal(syscall.SIGTERM); err != nil {
				fmt.Printf("Warning: could not stop process %d: %v\n", s.PID, err)
				continue
			}

			fmt.Printf("Stopped server on port %d (PID %d, model: %s)\n", s.Port, s.PID, s.Model)
			stopped++

			if !stopAll {
				break
			}
		}
	}

	if stopped == 0 && !stopAll {
		return fmt.Errorf("no server found on port %d", modelPort)
	}

	return nil
}

// runModelSwitch executes the model switch command.
func runModelSwitch(ctx context.Context, deps *ModelCommandDeps, model string) error {
	// Stop the current server on the port.
	servers, _ := getRunningServers()
	for _, s := range servers {
		if s.Port == modelPort {
			fmt.Printf("Stopping current server (model: %s)...\n", s.Model)
			process, _ := os.FindProcess(s.PID)
			if process != nil {
				_ = process.Signal(syscall.SIGTERM)
				time.Sleep(2 * time.Second)
			}
			break
		}
	}

	// Start the new model.
	return runModelServe(ctx, deps, model)
}

// runModelInfo executes the model info command.
func runModelInfo(ctx context.Context, deps *ModelCommandDeps, model string) error {
	modelID := resolveModelID(deps, model)

	// Find in catalog.
	var entry *ModelCatalogEntry
	for _, m := range defaultModelCatalog {
		if m.ID == modelID || strings.Contains(strings.ToLower(m.ID), strings.ToLower(model)) {
			entryCopy := m
			entry = &entryCopy
			break
		}
	}

	if entry == nil {
		// Create a basic entry for unknown models.
		entry = &ModelCatalogEntry{
			ID:   modelID,
			Name: modelID,
			Type: "mlx",
		}
	}

	// Check if downloaded.
	downloadedModels := getDownloadedModels(deps)
	entry.Downloaded = downloadedModels[entry.ID]

	return outputModelInfo(deps, entry)
}

// runModelBench executes the model bench command.
func runModelBench(ctx context.Context, deps *ModelCommandDeps) error {
	endpoint := fmt.Sprintf("http://localhost:%d/v1/chat/completions", modelPort)

	// First check if server is running.
	modelsEndpoint := fmt.Sprintf("http://localhost:%d/v1/models", modelPort)
	resp, err := http.Get(modelsEndpoint)
	if err != nil {
		return fmt.Errorf("no server running on port %d", modelPort)
	}
	resp.Body.Close()

	fmt.Printf("Benchmarking model server on port %d...\n\n", modelPort)

	// Run benchmark tests.
	tests := []struct {
		name      string
		prompt    string
		maxTokens int
	}{
		{"Simple", "Reply with just: hello", 10},
		{"Medium", "What is 2+2? Answer briefly.", 50},
		{"Complex", "Explain in one sentence why the sky is blue.", 100},
	}

	for _, test := range tests {
		start := time.Now()

		reqBody := fmt.Sprintf(`{"messages":[{"role":"user","content":"%s"}],"max_tokens":%d}`,
			test.prompt, test.maxTokens)

		req, _ := http.NewRequest("POST", endpoint, strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(req)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("  %s: \033[31mFAILED\033[0m (%v)\n", test.name, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Printf("  %s: \033[31mFAILED\033[0m (status %d)\n", test.name, resp.StatusCode)
			continue
		}

		color := "\033[32m" // Green
		if elapsed > 5*time.Second {
			color = "\033[33m" // Yellow
		}
		if elapsed > 15*time.Second {
			color = "\033[31m" // Red
		}

		fmt.Printf("  %s: %s%.2fs\033[0m\n", test.name, color, elapsed.Seconds())
	}

	return nil
}

// ==================== Helper Functions ====================

// getDownloadedModels returns a map of model IDs that are downloaded.
func getDownloadedModels(deps *ModelCommandDeps) map[string]bool {
	downloaded := make(map[string]bool)

	// Check HuggingFace cache directory.
	homeDir, _ := os.UserHomeDir()
	cacheDir := filepath.Join(homeDir, ".cache", "huggingface", "hub")

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return downloaded
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "models--") {
			// Convert directory name to model ID.
			// e.g., "models--mlx-community--Phi-3.5-mini-instruct-4bit"
			// -> "mlx-community/Phi-3.5-mini-instruct-4bit"
			modelID := strings.TrimPrefix(name, "models--")
			modelID = strings.Replace(modelID, "--", "/", 1)
			downloaded[modelID] = true
		}
	}

	return downloaded
}

// getRunningServers returns a list of running MLX model servers.
func getRunningServers() ([]ModelServerStatus, error) {
	var servers []ModelServerStatus

	// Use ps to find mlx_lm.server processes.
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "mlx_lm.server") || strings.Contains(line, "grep") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		pid, _ := strconv.Atoi(fields[1])
		cpuPct, _ := strconv.ParseFloat(fields[2], 64)
		memPct, _ := strconv.ParseFloat(fields[3], 64)

		// Extract model and port from command line.
		model := ""
		port := 8080

		for i, f := range fields {
			if f == "--model" && i+1 < len(fields) {
				model = fields[i+1]
			}
			if f == "--port" && i+1 < len(fields) {
				port, _ = strconv.Atoi(fields[i+1])
			}
		}

		// Check health.
		healthy := false
		endpoint := fmt.Sprintf("http://localhost:%d/v1/models", port)
		resp, err := http.Get(endpoint)
		if err == nil && resp.StatusCode == 200 {
			healthy = true
			resp.Body.Close()
		}

		servers = append(servers, ModelServerStatus{
			PID:     pid,
			Model:   model,
			Port:    port,
			CPUPct:  cpuPct,
			MemPct:  memPct,
			Healthy: healthy,
		})
	}

	return servers, nil
}

// resolveModelID resolves a short model name to a full model ID.
func resolveModelID(deps *ModelCommandDeps, model string) string {
	// If it looks like a full model ID, return as-is.
	if strings.Contains(model, "/") {
		return model
	}

	// Map short names to full IDs.
	shortNames := map[string]string{
		"phi":       "mlx-community/Phi-3.5-mini-instruct-4bit",
		"phi-3.5":   "mlx-community/Phi-3.5-mini-instruct-4bit",
		"qwen-32b":  "mlx-community/Qwen2.5-32B-Instruct-4bit",
		"qwen-7b":   "mlx-community/Qwen2.5-7B-Instruct-4bit",
		"qwen":      "mlx-community/Qwen2.5-7B-Instruct-4bit",
		"llama-3b":  "mlx-community/Llama-3.2-3B-Instruct-4bit",
		"llama":     "mlx-community/Llama-3.2-3B-Instruct-4bit",
		"gemma":     "mlx-community/gemma-2-9b-it-4bit",
		"gemma-9b":  "mlx-community/gemma-2-9b-it-4bit",
	}

	if fullID, ok := shortNames[strings.ToLower(model)]; ok {
		return fullID
	}

	// Search catalog for partial match.
	modelLower := strings.ToLower(model)
	for _, entry := range defaultModelCatalog {
		if strings.Contains(strings.ToLower(entry.ID), modelLower) {
			return entry.ID
		}
	}

	// Return as-is if not found.
	return model
}

// ==================== Output Functions ====================

// getModelOutputFormat determines the output format from flags and config.
func getModelOutputFormat(deps *ModelCommandDeps) config.OutputFormat {
	if modelOutput != "" {
		return config.OutputFormat(modelOutput)
	}
	if deps.Config != nil {
		return deps.Config.OutputFormat
	}
	return config.OutputFormatText
}

// outputModelCatalog outputs the model catalog.
func outputModelCatalog(deps *ModelCommandDeps, catalog []ModelCatalogEntry, showAll bool) error {
	format := getModelOutputFormat(deps)

	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(catalog)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(catalog)
	default:
		return outputModelCatalogText(catalog, showAll)
	}
}

// outputModelCatalogText outputs the catalog in human-readable format.
func outputModelCatalogText(catalog []ModelCatalogEntry, showAll bool) error {
	if len(catalog) == 0 {
		if showAll {
			fmt.Println("No models in catalog.")
		} else {
			fmt.Println("No downloaded models found.")
			fmt.Println("\nUse 'penf model list --all' to see available models.")
		}
		return nil
	}

	title := "Downloaded Models"
	if showAll {
		title = "Available Models"
	}
	fmt.Printf("%s (%d)\n", title, len(catalog))
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  %-42s %-12s %-8s %s\n", "MODEL", "SIZE", "LATENCY", "STATUS")
	fmt.Printf("  %-42s %-12s %-8s %s\n", "-----", "----", "-------", "------")

	for _, m := range catalog {
		status := "\033[32mdownloaded\033[0m"
		if !m.Downloaded {
			status = "\033[90mavailable\033[0m"
		}

		name := m.Name
		if len(name) > 40 {
			name = name[:37] + "..."
		}

		fmt.Printf("  %-42s %-12s %-8s %s\n",
			name,
			m.Size,
			m.ExpectedLatency,
			status)
	}

	fmt.Println()
	return nil
}

// outputModelStatus outputs running server status.
func outputModelStatus(deps *ModelCommandDeps, servers []ModelServerStatus) error {
	format := getModelOutputFormat(deps)

	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(servers)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(servers)
	default:
		return outputModelStatusText(servers)
	}
}

// outputModelStatusText outputs server status in human-readable format.
func outputModelStatusText(servers []ModelServerStatus) error {
	if len(servers) == 0 {
		fmt.Println("No model servers are running.")
		fmt.Println("\nUse 'penf model serve <model>' to start a server.")
		return nil
	}

	fmt.Printf("Running Model Servers (%d)\n", len(servers))
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("  %-6s %-40s %-6s %-6s %-6s %s\n", "PID", "MODEL", "PORT", "CPU", "MEM", "HEALTH")
	fmt.Printf("  %-6s %-40s %-6s %-6s %-6s %s\n", "---", "-----", "----", "---", "---", "------")

	for _, s := range servers {
		healthColor := "\033[32m"
		healthStr := "healthy"
		if !s.Healthy {
			healthColor = "\033[31m"
			healthStr = "unhealthy"
		}

		modelName := s.Model
		if len(modelName) > 38 {
			modelName = modelName[:35] + "..."
		}

		fmt.Printf("  %-6d %-40s %-6d %-6.1f %-6.1f %s%s\033[0m\n",
			s.PID,
			modelName,
			s.Port,
			s.CPUPct,
			s.MemPct,
			healthColor,
			healthStr)
	}

	fmt.Println()
	return nil
}

// outputModelInfo outputs model information.
func outputModelInfo(deps *ModelCommandDeps, entry *ModelCatalogEntry) error {
	format := getModelOutputFormat(deps)

	switch format {
	case config.OutputFormatJSON:
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entry)
	case config.OutputFormatYAML:
		enc := yaml.NewEncoder(os.Stdout)
		return enc.Encode(entry)
	default:
		return outputModelInfoText(entry)
	}
}

// outputModelInfoText outputs model info in human-readable format.
func outputModelInfoText(entry *ModelCatalogEntry) error {
	fmt.Println("Model Information")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  \033[1mID:\033[0m              %s\n", entry.ID)
	fmt.Printf("  \033[1mName:\033[0m            %s\n", entry.Name)
	fmt.Printf("  \033[1mType:\033[0m            %s\n", entry.Type)

	if entry.Size != "" {
		fmt.Printf("  \033[1mSize:\033[0m            %s\n", entry.Size)
	}
	if entry.ExpectedLatency != "" {
		fmt.Printf("  \033[1mLatency:\033[0m         %s\n", entry.ExpectedLatency)
	}
	if entry.MemoryRequired != "" {
		fmt.Printf("  \033[1mMemory Required:\033[0m %s\n", entry.MemoryRequired)
	}

	status := "\033[32mdownloaded\033[0m"
	if !entry.Downloaded {
		status = "\033[33mnot downloaded\033[0m"
	}
	fmt.Printf("  \033[1mStatus:\033[0m          %s\n", status)

	fmt.Println()
	return nil
}
