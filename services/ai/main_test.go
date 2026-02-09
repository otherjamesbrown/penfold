package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInitializationOrderPortBindingFirst tests that the service fails fast when the port
// is already in use, WITHOUT attempting expensive cloud API initialization (Langfuse, Gemini).
//
// Regression test for bug pf-f77151: ai-coordinator previously initialized cloud APIs
// before checking if the port was available, causing expensive crash loops.
//
// Expected behavior:
// - Port binding should happen BEFORE Langfuse tracing init
// - Port binding should happen BEFORE Gemini backend creation
// - Service should exit immediately when port is in use
// - NO backend initialization logs should appear
func TestInitializationOrderPortBindingFirst(t *testing.T) {
	// This test verifies the initialization order by attempting to start the service
	// when the port is already occupied, and checking that it fails WITHOUT logging
	// messages that indicate cloud API initialization.

	// Find two available ports for the test (gRPC and HTTP)
	// IMPORTANT: Bind to 0.0.0.0 (not 127.0.0.1) because the service binds to ":port"
	// which resolves to 0.0.0.0. On macOS, 127.0.0.1:X and 0.0.0.0:X don't conflict.
	grpcListener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available gRPC port: %v", err)
	}
	grpcPort := grpcListener.Addr().(*net.TCPAddr).Port
	// Keep the gRPC listener open - don't close it, this blocks the port
	defer grpcListener.Close()

	httpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available HTTP port: %v", err)
	}
	httpPort := httpListener.Addr().(*net.TCPAddr).Port
	httpListener.Close()

	t.Logf("Blocked gRPC port %d with active listener - attempting to start ai-coordinator", grpcPort)

	// Build the service binary (required since main package can't be imported)
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "ai-coordinator-test")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = "."
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build ai-coordinator: %v\nOutput: %s", err, buildOutput)
	}

	// Set up minimal environment for the service
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("AI_GRPC_PORT=%d", grpcPort),
		fmt.Sprintf("AI_HTTP_PORT=%d", httpPort),
		"AI_ENVIRONMENT=dev",
		"AI_LOG_LEVEL=debug",
		"AI_MLX_EMBEDDINGS_URL=http://localhost:8080", // Dummy URL
		"AI_MLX_LLM_URL=http://localhost:8081",        // Dummy URL
		// Explicitly disable Langfuse to make the test deterministic
		"LANGFUSE_HOST=",
		"LANGFUSE_PUBLIC_KEY=",
		"LANGFUSE_SECRET_KEY=",
		// Set a dummy Gemini API key to avoid early validation failure
		"AI_GEMINI_API_KEY=test-key-for-reproduction",
	)

	// Capture stdout and stderr to analyze initialization order
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start ai-coordinator: %v", err)
	}

	// Collect all output
	outputChan := make(chan string, 100)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			outputChan <- scanner.Text()
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			outputChan <- scanner.Text()
		}
	}()

	// Wait for process to exit or collect output for a reasonable time
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var allOutput []string
	timeout := time.After(3 * time.Second)
	processExited := false

collectLoop:
	for {
		select {
		case line := <-outputChan:
			allOutput = append(allOutput, line)
		case <-done:
			processExited = true
			// Process exited - collect remaining output
			time.Sleep(100 * time.Millisecond)
		drainLoop:
			for {
				select {
				case line := <-outputChan:
					allOutput = append(allOutput, line)
				default:
					break drainLoop
				}
			}
			break collectLoop
		case <-timeout:
			// Timeout - kill the process and collect what we have
			t.Logf("Service did not exit within timeout - killing process to collect output")
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			time.Sleep(100 * time.Millisecond)
		drainLoop2:
			for {
				select {
				case line := <-outputChan:
					allOutput = append(allOutput, line)
				default:
					break drainLoop2
				}
			}
			break collectLoop
		}
	}

	if !processExited {
		t.Logf("WARNING: Process did not exit cleanly - had to kill it")
	}

	// Analyze the output to determine initialization order
	foundMLXInit := false
	foundGeminiInit := false
	foundGRPCListening := false
	mlxInitLine := -1
	geminiInitLine := -1
	grpcListeningLine := -1

	for i, line := range allOutput {
		t.Logf("Line %d: %s", i, line)

		// Look for MLX backend initialization
		if strings.Contains(line, "MLX backend configured") {
			foundMLXInit = true
			mlxInitLine = i
		}

		// Look for Gemini backend creation
		if strings.Contains(line, "Gemini backend configured") ||
			strings.Contains(line, "Failed to create Gemini backend") {
			foundGeminiInit = true
			geminiInitLine = i
		}

		// Look for gRPC server listening message (happens AFTER net.Listen succeeds or in goroutine)
		if strings.Contains(line, "gRPC server listening") ||
			strings.Contains(line, "Failed to create gRPC listener") {
			foundGRPCListening = true
			grpcListeningLine = i
		}
	}

	// Verify: service should exit quickly when port is blocked, without initializing backends.
	// The gRPC listener error should appear in output.
	if !foundGRPCListening {
		t.Errorf("Expected to see gRPC listener failure in output")
	}

	// Regression check: backend initialization must NOT happen when port is unavailable.
	// If these appear, it means expensive cloud API calls are being made before port binding.
	if foundMLXInit {
		t.Errorf("REGRESSION: MLX backend initialization (line %d) happened despite port being unavailable (line %d)",
			mlxInitLine, grpcListeningLine)
	}
	if foundGeminiInit {
		t.Errorf("REGRESSION: Gemini backend initialization (line %d) happened despite port being unavailable (line %d)",
			geminiInitLine, grpcListeningLine)
	}

	// Service must exit on its own (not require timeout kill)
	if !processExited {
		t.Errorf("Service did not exit when port was unavailable - it should fail fast")
	}
}
