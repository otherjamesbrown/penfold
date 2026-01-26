# Quickstart: Unified Mention Resolution System

**Date**: 2026-01-21 | **Branch**: `013-content-enrichment`

## Prerequisites

- Go 1.22+
- PostgreSQL 16+ (running on dev02)
- MLX sidecar running on dev01 (port 8081)
- Penfold database with existing schema

## Setup

### 1. Run Migrations

```bash
# From repo root
penf db migrate

# Or manually apply new migrations
psql -h dev02 -U penfold -d penfold -f migrations/016_glossary_linked_entity.sql
psql -h dev02 -U penfold -d penfold -f migrations/017_mention_resolution.sql
psql -h dev02 -U penfold -d penfold -f migrations/018_resolution_comparisons.sql
```

### 2. Verify MLX Sidecar

```bash
# Check embeddings endpoint (existing)
curl http://localhost:8081/api/embed -d '{"model":"mxbai-embed-large-v1","prompt":"test"}'

# Check completions endpoint (new usage)
curl http://localhost:8081/api/generate -d '{"model":"mistral-7b","prompt":"Hello","stream":false}'
```

### 3. Configuration

Add to `~/.penf/config.yaml`:

```yaml
mention_resolution:
  llm:
    provider: mlx
    model: mistral-7b-instruct-v0.2
    base_url: http://localhost:8081
    timeout: 30s
    max_retries: 2
  thresholds:
    auto_resolve: 0.8
    verification: 0.9
    suggest: 0.7
  trace_level: standard  # minimal|standard|full|debug
```

## Basic Usage

### Process Content

```bash
# Process single content item
penf resolve content 4521

# Process all pending content
penf resolve pending --limit 100

# Dry run (show what would be resolved)
penf resolve content 4521 --dry-run
```

### View Traces

```bash
# List recent traces
penf audit traces list --limit 10

# Show trace details
penf audit trace show trace_abc123

# Show decisions with reasoning
penf audit trace show trace_abc123 --decisions

# Show LLM calls (if trace_level >= full)
penf audit trace show trace_abc123 --llm-calls
```

### Review Queue

```bash
# List pending mentions for review
penf review questions list

# Resolve a mention
penf review questions resolve 72 --link-to person:101

# Dismiss a mention
penf review questions dismiss 74 --reason "Generic reference"

# Batch resolve via JSON
penf process mentions batch-resolve '{"resolutions":[...], "dismissals":[...]}'
```

### Model Comparison

```bash
# Compare models on content
penf audit compare --content-id=4521 --models=mlx-mistral-7b,claude-sonnet

# View comparison results
penf audit comparison show comp_xyz789

# View reasoning differences
penf audit comparison show comp_xyz789 --mention="Alan" --reasoning

# Model statistics
penf audit models stats --last-30-days
```

## Development Workflow

### Running Tests

```bash
# Unit tests
go test ./pkg/mentions/...

# Integration tests (requires DB)
go test ./pkg/mentions/... -tags=integration

# Contract tests for LLM interface
go test ./pkg/mentions/resolver/... -run TestLLMContract
```

### Adding a New LLM Provider

1. Implement `LLMProvider` interface in `pkg/mentions/resolver/`
2. Add configuration handling in `pkg/mentions/resolver/config.go`
3. Add contract tests in `pkg/mentions/resolver/provider_test.go`
4. Register provider in `pkg/mentions/resolver/registry.go`

### Debugging Resolution

```bash
# Enable debug trace level
export PENF_TRACE_LEVEL=debug

# Process with verbose output
penf resolve content 4521 --verbose

# Export trace for analysis
penf audit trace export trace_abc123 --format=json > trace.json
```

## Architecture Overview

```
Content → Extraction → [Stage 1: Understanding]
                              ↓
                       [Stage 2: Cross-Mention]
                              ↓
                       [Stage 3: Matching]
                              ↓
                       [Stage 4: Verification] (if needed)
                              ↓
                    ┌─────────┴─────────┐
                    ↓                   ↓
            Auto-Resolved        Human Review Queue
            (conf >= 0.8)         (conf < 0.8)
```

## Key Files

| File | Purpose |
|------|---------|
| `pkg/mentions/resolver/resolver.go` | Main orchestrator |
| `pkg/mentions/resolver/stages.go` | Stage 1-4 implementations |
| `pkg/mentions/resolver/mlx.go` | MLX provider |
| `pkg/mentions/audit/trace.go` | Trace management |
| `cmd/penf/cmd/audit.go` | CLI commands |

## Troubleshooting

### LLM Not Responding

```bash
# Check MLX sidecar status
curl http://localhost:8081/api/tags

# Check logs
journalctl -u mlx-sidecar -f
```

### Low Auto-Resolution Rate

1. Check trace decisions: `penf audit trace show <id> --decisions`
2. Review confidence factors in reasoning
3. Adjust thresholds if needed
4. Consider adding more glossary terms/aliases

### Parse Failures

```bash
# Check LLM output in full trace
penf audit trace show <id> --llm-calls

# Common issues:
# - Model not following JSON format
# - Missing required fields
# - Confidence outside 0-1 range
```

## Next Steps

1. Run migrations and verify setup
2. Process a test content item
3. Review trace output
4. Adjust configuration as needed
5. Run comparison against different models
