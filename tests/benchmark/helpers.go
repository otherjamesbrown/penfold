//go:build benchmark

// Package benchmark provides benchmarking tests for Penfold components.
// These tests use the actual CLI and infrastructure to measure real-world performance.
package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// CLIRunner executes penf CLI commands and captures output.
type CLIRunner struct {
	BinaryPath string
	WorkDir    string
	Env        []string
}

// NewCLIRunner creates a CLI runner, building the binary if needed.
func NewCLIRunner(t *testing.T) *CLIRunner {
	t.Helper()

	// Find project root by looking for cmd/penf directory
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk up to find cmd/penf (the actual project root, not tests/go.mod)
	for workDir != "/" {
		cmdPath := filepath.Join(workDir, "cmd", "penf")
		if _, err := os.Stat(cmdPath); err == nil {
			break
		}
		workDir = filepath.Dir(workDir)
	}

	if workDir == "/" {
		t.Fatalf("could not find project root (looking for cmd/penf)")
	}

	binaryPath := filepath.Join(workDir, "penf")

	// Build the CLI if it doesn't exist or is stale
	cmdPath := filepath.Join(workDir, "cmd", "penf")
	if _, err := os.Stat(cmdPath); err == nil {
		t.Log("Building penf CLI...")
		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/penf")
		cmd.Dir = workDir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to build penf: %v\n%s", err, output)
		}
	}

	return &CLIRunner{
		BinaryPath: binaryPath,
		WorkDir:    workDir,
		Env:        os.Environ(),
	}
}

// RunResult holds the result of a CLI command execution.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	Err      error
}

// Run executes a penf command and returns the result.
func (r *CLIRunner) Run(ctx context.Context, args ...string) *RunResult {
	start := time.Now()

	cmd := exec.CommandContext(ctx, r.BinaryPath, args...)
	cmd.Dir = r.WorkDir
	cmd.Env = r.Env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	return &RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
		Err:      err,
	}
}

// RunJSON executes a command with JSON output and unmarshals the result.
func (r *CLIRunner) RunJSON(ctx context.Context, v interface{}, args ...string) error {
	// Add --output json flag
	args = append(args, "--output", "json")
	result := r.Run(ctx, args...)
	if result.Err != nil {
		return fmt.Errorf("command failed: %w\nstderr: %s", result.Err, result.Stderr)
	}
	if err := json.Unmarshal([]byte(result.Stdout), v); err != nil {
		return fmt.Errorf("failed to parse JSON: %w\nstdout: %s", err, result.Stdout)
	}
	return nil
}

// ModelStatus represents the status of a running model server.
type ModelStatus struct {
	PID     int     `json:"pid"`
	Model   string  `json:"model"`
	Port    int     `json:"port"`
	CPUPct  float64 `json:"cpu_pct"`
	MemPct  float64 `json:"mem_pct"`
	Healthy bool    `json:"healthy"`
}

// ModelInfo represents information about a model.
type ModelInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Size            string `json:"size"`
	Type            string `json:"type"`
	Downloaded      bool   `json:"downloaded"`
	ExpectedLatency string `json:"expected_latency,omitempty"`
	MemoryRequired  string `json:"memory_required,omitempty"`
}

// GetModelStatus returns the current model server status.
func (r *CLIRunner) GetModelStatus(ctx context.Context) ([]ModelStatus, error) {
	var status []ModelStatus
	err := r.RunJSON(ctx, &status, "model", "status")
	return status, err
}

// SwitchModel switches to a different model and waits for it to be ready.
func (r *CLIRunner) SwitchModel(ctx context.Context, model string, port int) error {
	args := []string{"model", "switch", model}
	if port != 0 {
		args = append(args, "--port", fmt.Sprintf("%d", port))
	}
	result := r.Run(ctx, args...)
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to switch model: %s", result.Stderr)
	}
	return nil
}

// StopModel stops a model server.
func (r *CLIRunner) StopModel(ctx context.Context, port int) error {
	args := []string{"model", "stop"}
	if port != 0 {
		args = append(args, "--port", fmt.Sprintf("%d", port))
	}
	result := r.Run(ctx, args...)
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to stop model: %s", result.Stderr)
	}
	return nil
}

// ServeModel starts a model server.
func (r *CLIRunner) ServeModel(ctx context.Context, model string, port int) error {
	args := []string{"model", "serve", model}
	if port != 0 {
		args = append(args, "--port", fmt.Sprintf("%d", port))
	}
	result := r.Run(ctx, args...)
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to serve model: %s", result.Stderr)
	}
	return nil
}

// BenchmarkResult holds timing results for a benchmark run.
type BenchmarkResult struct {
	Model           string
	ColdStartTime   time.Duration
	WarmRunTimes    []time.Duration
	AvgWarmTime     time.Duration
	MemoryPercent   float64
	PromptTokens    int
	CompletionTokens int
	Passed          bool
	Error           string
}

// String returns a human-readable summary of the benchmark result.
func (r *BenchmarkResult) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Model: %s\n", r.Model))
	sb.WriteString(fmt.Sprintf("  Cold Start: %v\n", r.ColdStartTime))
	sb.WriteString(fmt.Sprintf("  Warm Runs: %v\n", r.WarmRunTimes))
	sb.WriteString(fmt.Sprintf("  Avg Warm: %v\n", r.AvgWarmTime))
	sb.WriteString(fmt.Sprintf("  Memory: %.1f%%\n", r.MemoryPercent))
	sb.WriteString(fmt.Sprintf("  Passed: %v\n", r.Passed))
	if r.Error != "" {
		sb.WriteString(fmt.Sprintf("  Error: %s\n", r.Error))
	}
	return sb.String()
}

// FormatResultsTable formats multiple benchmark results as a markdown table.
func FormatResultsTable(results []BenchmarkResult) string {
	var sb strings.Builder
	sb.WriteString("| Model | Cold Start | Run 1 | Run 2 | Run 3 | Avg Warm | Memory | Status |\n")
	sb.WriteString("|-------|------------|-------|-------|-------|----------|--------|--------|\n")

	for _, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}

		run1, run2, run3 := "-", "-", "-"
		if len(r.WarmRunTimes) > 0 {
			run1 = fmt.Sprintf("%.1fs", r.WarmRunTimes[0].Seconds())
		}
		if len(r.WarmRunTimes) > 1 {
			run2 = fmt.Sprintf("%.1fs", r.WarmRunTimes[1].Seconds())
		}
		if len(r.WarmRunTimes) > 2 {
			run3 = fmt.Sprintf("%.1fs", r.WarmRunTimes[2].Seconds())
		}

		sb.WriteString(fmt.Sprintf("| %s | %.1fs | %s | %s | %s | %.1fs | %.1f%% | %s |\n",
			r.Model,
			r.ColdStartTime.Seconds(),
			run1, run2, run3,
			r.AvgWarmTime.Seconds(),
			r.MemoryPercent,
			status,
		))
	}

	return sb.String()
}
