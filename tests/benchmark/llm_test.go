//go:build benchmark

package benchmark

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Models to benchmark - these should be pre-downloaded.
var benchmarkModels = []string{
	"phi",      // Phi-3.5 Mini (4-bit) - smallest, fastest
	"qwen-7b",  // Qwen 2.5 7B (4-bit) - balanced
	"qwen-32b", // Qwen 2.5 32B (4-bit) - largest, highest quality
}

// TestLLMModelBenchmark benchmarks different LLM models using the CLI.
// Run with: go test -tags=benchmark -run TestLLMModelBenchmark -v ./tests/benchmark/
func TestLLMModelBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	cli := NewCLIRunner(t)
	ctx := context.Background()

	var results []BenchmarkResult

	for _, model := range benchmarkModels {
		t.Run(model, func(t *testing.T) {
			result := benchmarkModel(t, cli, ctx, model)
			results = append(results, result)
		})
	}

	// Print summary table
	t.Log("\n" + FormatResultsTable(results))
}

// TestLLMQuickBench runs a quick benchmark on the currently running model.
// Run with: go test -tags=benchmark -run TestLLMQuickBench -v ./tests/benchmark/
func TestLLMQuickBench(t *testing.T) {
	cli := NewCLIRunner(t)
	ctx := context.Background()

	// Check current model
	status, err := cli.GetModelStatus(ctx)
	if err != nil || len(status) == 0 {
		t.Skip("No model server running - start one with 'penf model serve <model>'")
	}

	currentModel := status[0].Model
	t.Logf("Benchmarking current model: %s", currentModel)

	// Run the built-in bench command
	result := cli.Run(ctx, "model", "bench")
	t.Logf("Bench output:\n%s", result.Stdout)

	if result.ExitCode != 0 {
		t.Errorf("bench command failed: %s", result.Stderr)
	}
}

// TestLLMModelComparison runs a detailed comparison of all models.
// This test takes several minutes per model.
// Run with: go test -tags=benchmark -run TestLLMModelComparison -v -timeout 30m ./tests/benchmark/
func TestLLMModelComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping detailed comparison in short mode")
	}

	cli := NewCLIRunner(t)
	ctx := context.Background()

	var results []BenchmarkResult

	for _, model := range benchmarkModels {
		t.Run(model, func(t *testing.T) {
			result := benchmarkModelDetailed(t, cli, ctx, model, 3)
			results = append(results, result)
			t.Log(result.String())
		})
	}

	// Print summary
	t.Log("\n=== BENCHMARK SUMMARY ===\n")
	t.Log(FormatResultsTable(results))
}

// benchmarkModel runs a quick benchmark on a single model.
func benchmarkModel(t *testing.T, cli *CLIRunner, ctx context.Context, model string) BenchmarkResult {
	t.Helper()

	result := BenchmarkResult{
		Model:  model,
		Passed: true,
	}

	// Stop any running server
	_ = cli.StopModel(ctx, 8080)
	time.Sleep(2 * time.Second)

	// Start the model and measure cold start time
	t.Logf("Starting model %s...", model)
	startTime := time.Now()
	if err := cli.ServeModel(ctx, model, 8080); err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("failed to start model: %v", err)
		return result
	}
	result.ColdStartTime = time.Since(startTime)
	t.Logf("Cold start: %v", result.ColdStartTime)

	// Run the quick bench
	benchResult := cli.Run(ctx, "model", "bench")
	if benchResult.ExitCode != 0 {
		result.Passed = false
		result.Error = fmt.Sprintf("bench failed: %s", benchResult.Stderr)
		return result
	}

	// Parse bench output for timing
	result.WarmRunTimes = parseBenchOutput(benchResult.Stdout)
	if len(result.WarmRunTimes) > 0 {
		var total time.Duration
		for _, d := range result.WarmRunTimes {
			total += d
		}
		result.AvgWarmTime = total / time.Duration(len(result.WarmRunTimes))
	}

	// Get memory usage
	status, err := cli.GetModelStatus(ctx)
	if err == nil && len(status) > 0 {
		result.MemoryPercent = status[0].MemPct
	}

	t.Logf("Bench output:\n%s", benchResult.Stdout)

	return result
}

// benchmarkModelDetailed runs a detailed benchmark with multiple iterations.
func benchmarkModelDetailed(t *testing.T, cli *CLIRunner, ctx context.Context, model string, iterations int) BenchmarkResult {
	t.Helper()

	result := BenchmarkResult{
		Model:  model,
		Passed: true,
	}

	// Stop any running server
	_ = cli.StopModel(ctx, 8080)
	time.Sleep(2 * time.Second)

	// Start the model and measure cold start time
	t.Logf("Starting model %s...", model)
	startTime := time.Now()
	if err := cli.ServeModel(ctx, model, 8080); err != nil {
		result.Passed = false
		result.Error = fmt.Sprintf("failed to start model: %v", err)
		return result
	}
	result.ColdStartTime = time.Since(startTime)
	t.Logf("Cold start: %v", result.ColdStartTime)

	// Run multiple iterations of the bench command
	for i := 0; i < iterations; i++ {
		t.Logf("Running iteration %d/%d...", i+1, iterations)
		benchResult := cli.Run(ctx, "model", "bench")
		if benchResult.ExitCode != 0 {
			t.Logf("Warning: bench iteration %d failed: %s", i+1, benchResult.Stderr)
			continue
		}

		// Parse and sum the timings from this run
		timings := parseBenchOutput(benchResult.Stdout)
		if len(timings) > 0 {
			var total time.Duration
			for _, d := range timings {
				total += d
			}
			result.WarmRunTimes = append(result.WarmRunTimes, total)
		}
	}

	// Calculate average
	if len(result.WarmRunTimes) > 0 {
		var total time.Duration
		for _, d := range result.WarmRunTimes {
			total += d
		}
		result.AvgWarmTime = total / time.Duration(len(result.WarmRunTimes))
	}

	// Get memory usage
	status, err := cli.GetModelStatus(ctx)
	if err == nil && len(status) > 0 {
		result.MemoryPercent = status[0].MemPct
	}

	return result
}

// parseBenchOutput extracts timing values from bench command output.
// Example output:
//
//	Simple: 0.35s
//	Medium: 0.53s
//	Complex: 1.70s
func parseBenchOutput(output string) []time.Duration {
	var durations []time.Duration

	// Match patterns like "0.35s" or "1.70s"
	re := regexp.MustCompile(`(\d+\.?\d*)s`)
	matches := re.FindAllStringSubmatch(output, -1)

	for _, match := range matches {
		if len(match) >= 2 {
			if seconds, err := strconv.ParseFloat(match[1], 64); err == nil {
				durations = append(durations, time.Duration(seconds*float64(time.Second)))
			}
		}
	}

	return durations
}

// TestLLMInferenceLatency measures raw inference latency for each model.
// This uses the CLI's model bench command which sends standardized prompts.
func TestLLMInferenceLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping inference latency test in short mode")
	}

	cli := NewCLIRunner(t)
	ctx := context.Background()

	type LatencyResult struct {
		Model   string
		Simple  time.Duration
		Medium  time.Duration
		Complex time.Duration
	}

	var results []LatencyResult

	for _, model := range benchmarkModels {
		t.Run(model, func(t *testing.T) {
			// Switch to this model
			t.Logf("Switching to %s...", model)
			if err := cli.SwitchModel(ctx, model, 8080); err != nil {
				t.Fatalf("failed to switch model: %v", err)
			}

			// Wait for warmup
			time.Sleep(3 * time.Second)

			// Run bench
			benchResult := cli.Run(ctx, "model", "bench")
			if benchResult.ExitCode != 0 {
				t.Fatalf("bench failed: %s", benchResult.Stderr)
			}

			// Parse output - expect Simple, Medium, Complex in order
			timings := parseBenchOutput(benchResult.Stdout)

			lr := LatencyResult{Model: model}
			if len(timings) >= 1 {
				lr.Simple = timings[0]
			}
			if len(timings) >= 2 {
				lr.Medium = timings[1]
			}
			if len(timings) >= 3 {
				lr.Complex = timings[2]
			}

			results = append(results, lr)
			t.Logf("%s: Simple=%v, Medium=%v, Complex=%v",
				model, lr.Simple, lr.Medium, lr.Complex)
		})
	}

	// Print comparison table
	t.Log("\n=== INFERENCE LATENCY COMPARISON ===")
	t.Log("| Model | Simple | Medium | Complex |")
	t.Log("|-------|--------|--------|---------|")
	for _, r := range results {
		t.Logf("| %s | %.2fs | %.2fs | %.2fs |",
			r.Model, r.Simple.Seconds(), r.Medium.Seconds(), r.Complex.Seconds())
	}
}

// TestSingleModelBenchmark benchmarks a single model specified by MODEL env var.
// Run with: MODEL=phi go test -tags=benchmark -run TestSingleModelBenchmark -v ./tests/benchmark/
func TestSingleModelBenchmark(t *testing.T) {
	model := getEnvOrDefault("MODEL", "")
	if model == "" {
		t.Skip("MODEL environment variable not set")
	}

	cli := NewCLIRunner(t)
	ctx := context.Background()

	result := benchmarkModelDetailed(t, cli, ctx, model, 3)
	t.Log("\n" + result.String())

	if !result.Passed {
		t.Errorf("benchmark failed: %s", result.Error)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}
