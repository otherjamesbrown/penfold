//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// Success returns true if the command succeeded.
func (r *RunResult) Success() bool {
	return r.ExitCode == 0 && r.Err == nil
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

// SetEnv adds or updates an environment variable for CLI commands.
func (r *CLIRunner) SetEnv(key, value string) {
	// Check if the key already exists
	prefix := key + "="
	for i, env := range r.Env {
		if len(env) >= len(prefix) && env[:len(prefix)] == prefix {
			r.Env[i] = prefix + value
			return
		}
	}
	// Key doesn't exist, append it
	r.Env = append(r.Env, prefix+value)
}

// IngestEmail runs the email ingestion command.
func (r *CLIRunner) IngestEmail(ctx context.Context, filePath string) *RunResult {
	return r.Run(ctx, "ingest", "email", filePath)
}

// IngestMeeting runs the meeting ingestion command.
func (r *CLIRunner) IngestMeeting(ctx context.Context, filePath string) *RunResult {
	return r.Run(ctx, "ingest", "meeting", filePath)
}

// AIQuery runs an AI query command.
func (r *CLIRunner) AIQuery(ctx context.Context, query string) *RunResult {
	return r.Run(ctx, "ai", "query", query)
}

// AIAnalyze runs an AI analyze command.
func (r *CLIRunner) AIAnalyze(ctx context.Context, contentID, analysisType string) *RunResult {
	args := []string{"ai", "analyze", contentID}
	if analysisType != "" {
		args = append(args, "--type", analysisType)
	}
	return r.Run(ctx, args...)
}

// Search runs a search command.
func (r *CLIRunner) Search(ctx context.Context, query string) *RunResult {
	return r.Run(ctx, "search", query)
}

// SearchJSON runs a search command and returns parsed JSON results.
func (r *CLIRunner) SearchJSON(ctx context.Context, query string) ([]CLISearchResult, error) {
	var results []CLISearchResult
	err := r.RunJSON(ctx, &results, "search", query)
	return results, err
}

// CLISearchResult represents a search result from the CLI.
type CLISearchResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ContentType string   `json:"content_type"`
	Score       float64  `json:"score"`
	Snippet     string   `json:"snippet"`
	Highlights  []string `json:"highlights,omitempty"`
}

// ProcessMentions runs the mention processing command.
func (r *CLIRunner) ProcessMentions(ctx context.Context, contentID string) *RunResult {
	return r.Run(ctx, "process", "mentions", contentID)
}

// GetHealth checks the health of Penfold services.
func (r *CLIRunner) GetHealth(ctx context.Context) *RunResult {
	return r.Run(ctx, "health")
}

// GetHealthJSON checks health and returns parsed JSON.
func (r *CLIRunner) GetHealthJSON(ctx context.Context) (*HealthStatus, error) {
	var status HealthStatus
	err := r.RunJSON(ctx, &status, "health")
	return &status, err
}

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

// WaitForServices waits for required services to be healthy.
func (r *CLIRunner) WaitForServices(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := r.GetHealthJSON(ctx)
		if err == nil && status.Status == "healthy" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			// Retry
		}
	}
	return fmt.Errorf("services not healthy after %v", timeout)
}

// WorkflowResult represents the result of a workflow execution.
type WorkflowResult struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}

// TriggerWorkflow triggers a named workflow.
func (r *CLIRunner) TriggerWorkflow(ctx context.Context, workflowName string, args ...string) *RunResult {
	cmdArgs := []string{"workflow", "trigger", workflowName}
	cmdArgs = append(cmdArgs, args...)
	return r.Run(ctx, cmdArgs...)
}

// GetWorkflowStatus gets the status of a workflow.
func (r *CLIRunner) GetWorkflowStatus(ctx context.Context, workflowID string) (*WorkflowResult, error) {
	var result WorkflowResult
	err := r.RunJSON(ctx, &result, "workflow", "status", workflowID)
	return &result, err
}
