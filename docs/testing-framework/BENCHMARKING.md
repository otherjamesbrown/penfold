# Benchmarking Guide

Guide for running and creating LLM performance benchmarks in Penfold.

## Overview

Penfold includes benchmark tests for measuring LLM inference performance. These tests help:
- Compare different model sizes (phi, qwen-7b, qwen-32b)
- Measure cold start vs warm inference times
- Track memory usage across models
- Validate performance meets requirements

## Quick Start

```bash
# Run quick benchmark on currently running model
go test -tags=benchmark -v -run TestLLMQuickBench ./tests/benchmark/

# Run full model comparison
go test -tags=benchmark -v -timeout 30m -run TestLLMModelComparison ./tests/benchmark/
```

## Prerequisites

- LLM server running (on dev01 or locally)
- Models pre-downloaded via `penf model pull`

```bash
# Verify LLM server
curl -s http://localhost:8080/v1/models

# Pull models for benchmarking
penf model pull phi
penf model pull qwen-7b
penf model pull qwen-32b
```

## Available Benchmark Tests

### TestLLMQuickBench
Quick benchmark on the currently running model. No model switching.

```bash
go test -tags=benchmark -v -run TestLLMQuickBench ./tests/benchmark/
```

**Output:**
```
=== RUN   TestLLMQuickBench
Benchmarking current model: qwen-7b
Bench output:
  Simple: 0.35s
  Medium: 0.53s
  Complex: 1.70s
--- PASS: TestLLMQuickBench
```

### TestLLMModelBenchmark
Benchmarks all configured models sequentially.

```bash
go test -tags=benchmark -v -run TestLLMModelBenchmark ./tests/benchmark/
```

**Models tested:** phi, qwen-7b, qwen-32b

### TestLLMModelComparison
Detailed comparison with multiple iterations per model.

```bash
go test -tags=benchmark -v -timeout 30m -run TestLLMModelComparison ./tests/benchmark/
```

### TestLLMInferenceLatency
Measures raw inference latency for Simple, Medium, and Complex prompts.

```bash
go test -tags=benchmark -v -run TestLLMInferenceLatency ./tests/benchmark/
```

### TestSingleModelBenchmark
Benchmark a specific model via environment variable.

```bash
MODEL=phi go test -tags=benchmark -v -run TestSingleModelBenchmark ./tests/benchmark/
```

## Benchmark Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| Cold Start Time | Time to load model into memory | <30s (phi), <60s (qwen-32b) |
| Warm Run Time | Inference time after model loaded | <2s (simple), <5s (complex) |
| Memory Percent | Memory usage as % of system | <50% (phi), <80% (qwen-32b) |
| Simple Prompt | Short question/answer | <0.5s |
| Medium Prompt | Paragraph extraction | <1s |
| Complex Prompt | Multi-step reasoning | <3s |

## Benchmark Output Format

```
=== BENCHMARK SUMMARY ===
| Model    | Cold Start | Avg Warm | Memory |
|----------|------------|----------|--------|
| phi      | 12.3s      | 0.45s    | 23%    |
| qwen-7b  | 28.1s      | 0.82s    | 45%    |
| qwen-32b | 54.2s      | 1.90s    | 78%    |
```

## Writing New Benchmarks

### Basic Structure

```go
//go:build benchmark

package benchmark

import (
    "context"
    "testing"
    "time"
)

func TestMyBenchmark(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping benchmark in short mode")
    }

    cli := NewCLIRunner(t)
    ctx := context.Background()

    // Measure operation
    start := time.Now()
    result := cli.Run(ctx, "model", "bench")
    duration := time.Since(start)

    if result.ExitCode != 0 {
        t.Fatalf("benchmark failed: %s", result.Stderr)
    }

    t.Logf("Duration: %v", duration)
}
```

### Using BenchmarkResult

```go
func TestCustomBenchmark(t *testing.T) {
    cli := NewCLIRunner(t)
    ctx := context.Background()

    result := BenchmarkResult{
        Model:  "custom-model",
        Passed: true,
    }

    // Measure cold start
    startTime := time.Now()
    if err := cli.ServeModel(ctx, "custom-model", 8080); err != nil {
        result.Passed = false
        result.Error = err.Error()
        return
    }
    result.ColdStartTime = time.Since(startTime)

    // Run benchmark iterations
    for i := 0; i < 3; i++ {
        benchResult := cli.Run(ctx, "model", "bench")
        timings := parseBenchOutput(benchResult.Stdout)
        result.WarmRunTimes = append(result.WarmRunTimes, timings...)
    }

    // Calculate average
    if len(result.WarmRunTimes) > 0 {
        var total time.Duration
        for _, d := range result.WarmRunTimes {
            total += d
        }
        result.AvgWarmTime = total / time.Duration(len(result.WarmRunTimes))
    }

    t.Log(result.String())
}
```

### Parsing Bench Output

The `parseBenchOutput` function extracts timing values:

```go
// Input: "Simple: 0.35s\nMedium: 0.53s\nComplex: 1.70s"
// Output: []time.Duration{350ms, 530ms, 1.7s}
timings := parseBenchOutput(output)
```

## CLI Runner API

```go
// Create runner
cli := NewCLIRunner(t)

// Run CLI command
result := cli.Run(ctx, "model", "bench")
// result.Stdout, result.Stderr, result.ExitCode

// Model management
cli.ServeModel(ctx, "qwen-7b", 8080)  // Start model server
cli.StopModel(ctx, 8080)              // Stop model server
cli.SwitchModel(ctx, "phi", 8080)     // Switch to different model

// Get status
status, err := cli.GetModelStatus(ctx)
// status[0].Model, status[0].MemPct
```

## Running in CI

Benchmarks are not run in regular CI due to time constraints. To run in CI:

```yaml
benchmark-tests:
  runs-on: [self-hosted, macos, ARM64]
  if: github.event_name == 'workflow_dispatch'
  steps:
    - run: go test -tags=benchmark -v -timeout 30m ./tests/benchmark/
```

## Comparing Results

### Manual Comparison

Run benchmarks on different configurations and compare:

```bash
# Benchmark with default settings
go test -tags=benchmark -v -run TestLLMQuickBench ./tests/benchmark/ > baseline.txt

# Make configuration change...

# Benchmark again
go test -tags=benchmark -v -run TestLLMQuickBench ./tests/benchmark/ > modified.txt

# Compare
diff baseline.txt modified.txt
```

### Tracking Over Time

Store benchmark results with timestamps:

```bash
# Run and save with date
go test -tags=benchmark -v -run TestLLMModelComparison ./tests/benchmark/ \
  | tee "benchmarks/$(date +%Y-%m-%d).txt"
```

## Performance Targets

| Use Case | Model | Max Latency | Notes |
|----------|-------|-------------|-------|
| Interactive queries | phi | 500ms | User-facing |
| Batch processing | qwen-7b | 2s | Background jobs |
| High-quality analysis | qwen-32b | 5s | Complex reasoning |

## Troubleshooting

### Model Server Not Running

```bash
# Check status
curl -s http://localhost:8080/v1/models

# Start server
penf model serve qwen-7b

# Or via launchctl (on dev01)
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist
```

### Benchmark Times Out

Increase timeout:
```bash
go test -tags=benchmark -v -timeout 60m ./tests/benchmark/
```

### Memory Issues

Ensure only one model is loaded:
```bash
penf model stop
penf model serve phi  # Smaller model
```

## Files

| File | Description |
|------|-------------|
| `tests/benchmark/llm_test.go` | Main benchmark tests |
| `tests/benchmark/cli_runner.go` | CLI runner helper (if exists) |
| `tests/benchmark/types.go` | BenchmarkResult type (if exists) |
