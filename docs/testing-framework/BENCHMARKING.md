# Benchmark Tests

## Commands

```bash
# Quick bench (current model)
go test -tags=benchmark -v -run TestLLMQuickBench ./tests/benchmark/

# Compare all models
go test -tags=benchmark -v -timeout 30m -run TestLLMModelComparison ./tests/benchmark/

# Single model
MODEL=phi go test -tags=benchmark -v -run TestSingleModelBenchmark ./tests/benchmark/
```

## Models Benchmarked

| Model | Cold Start | Inference | Memory |
|-------|------------|-----------|--------|
| phi | <15s | <0.5s | ~25% |
| qwen-7b | <30s | <1s | ~45% |
| qwen-32b | <60s | <2s | ~80% |

## Writing Benchmarks

```go
//go:build benchmark

package benchmark

func TestMyBenchmark(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping benchmark in short mode")
    }

    cli := NewCLIRunner(t)
    ctx := context.Background()

    start := time.Now()
    result := cli.Run(ctx, "model", "bench")
    t.Logf("Duration: %v", time.Since(start))

    if result.ExitCode != 0 {
        t.Fatalf("failed: %s", result.Stderr)
    }
}
```

## CLI Runner API

```go
cli := NewCLIRunner(t)
cli.Run(ctx, "model", "bench")           // Run command
cli.ServeModel(ctx, "phi", 8080)         // Start model
cli.StopModel(ctx, 8080)                 // Stop model
cli.SwitchModel(ctx, "qwen-7b", 8080)    // Switch model
status, _ := cli.GetModelStatus(ctx)     // Get status
```
