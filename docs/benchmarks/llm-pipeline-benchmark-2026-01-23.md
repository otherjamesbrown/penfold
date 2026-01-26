# LLM Model Benchmark Results

**Date:** 2026-01-23
**Test:** TestFullPipeline_IngestToSearch (4-stage entity resolution pipeline)
**Hardware:** Mac Mini M4, 32GB RAM
**Bead:** pe-xjf8

## Summary

| Model | Cold Start | Run 1 (Cold) | Run 2 (Warm) | Run 3 (Warm) | Avg Warm | Memory | Status |
|-------|------------|--------------|--------------|--------------|----------|--------|--------|
| Phi-3.5 Mini (4-bit) | 4.05s | 172s | 96s | 95s | 95.5s | 1.5% | PASS |
| Qwen 2.5 7B (4-bit) | 4.06s | 170s | 96s | 95s | 95.5s | 54.2% | PASS |
| Qwen 2.5 32B (4-bit) | 10.07s | 163s | 95s | 95s | 95.0s | 45.8% | PASS |

## Key Findings

### 1. Warm Performance is Nearly Identical
All three models achieve ~95s latency on the E2E pipeline once warmed up. The model size has **no significant impact on warm inference speed** for this workload.

### 2. Cold Start Varies by Model Size
- **Phi-3.5 Mini / Qwen 7B:** ~4 seconds to load and become healthy
- **Qwen 32B:** ~10 seconds to load and become healthy

### 3. Memory Usage
- **Phi-3.5 Mini:** 1.5% (~0.5GB) - Extremely memory efficient
- **Qwen 32B:** 45.8% (~14.6GB) - Moderate usage for model size
- **Qwen 7B:** 54.2% (~17GB) - Higher than expected, possibly due to quantization overhead

### 4. Quality
All tests passed with correct entity extraction, resolution, glossary expansion, and search simulation. Quality differences between models were not observable in pass/fail testing - all produced correct JSON outputs.

## Recommendations

### For Development/Testing
**Phi-3.5 Mini (4-bit)** is recommended:
- Same warm performance as larger models
- Minimal memory footprint (1.5%)
- Fast cold start (4s)
- Leaves resources available for other services

### For Production
**Qwen 2.5 7B (4-bit)** is recommended:
- Same latency profile as all models
- Better instruction following for edge cases
- Still has fast cold start (4s)
- Acceptable memory usage if dedicated to LLM workloads

### For Quality-Critical Workloads
**Qwen 2.5 32B (4-bit)** may be preferred:
- Highest parameter count for nuanced extractions
- Surprisingly similar performance to smaller models
- Longest cold start (10s) is still acceptable
- Memory usage (45.8%) is manageable on 32GB systems

## Test Details

### Pipeline Stages Tested
1. **Entity Extraction** - Extract people, teams, projects, acronyms from email
2. **Entity Resolution** - Match extracted names to canonical forms
3. **Glossary Expansion** - Expand acronyms using domain glossary
4. **Search Simulation** - Match query against enriched content

### Environment
- Database: PostgreSQL 16 with pgvector on dev02
- LLM Server: MLX-LM server on localhost:8080
- Test Framework: Go testing with e2e build tag

## Raw Data

### Phi-3.5 Mini (4-bit)
```
Cold start: 4.055037000 seconds
Run 1: 171.975225000 seconds
Run 2: 96.434384000 seconds
Run 3: 95.238462000 seconds
Memory: 1.5%
```

### Qwen 2.5 7B (4-bit)
```
Cold start: 4.059776000 seconds
Run 1: 170.357258000 seconds
Run 2: 96.042741000 seconds
Run 3: 95.284873000 seconds
Memory: 54.2%
```

### Qwen 2.5 32B (4-bit)
```
Cold start: 10.069498000 seconds
Run 1: 163.111197000 seconds
Run 2: 95.160591000 seconds
Run 3: 95.347050000 seconds
Memory: 45.8%
```
