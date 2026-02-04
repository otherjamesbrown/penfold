# Penfold: Constraints and Current Limitations

## Hardware

### dev01 — Mac Mini (Apple Silicon)

| Resource | Specification |
|----------|--------------|
| Chip | Apple M-series (Apple Silicon) |
| Unified Memory | Shared between CPU, GPU, and Neural Engine |
| Role | Worker service, AI service, MLX model inference |

**Memory constraint**: Apple Silicon uses unified memory shared between the OS, applications, and GPU (used for ML inference). Running a 7B model (4-bit quantized) requires ~4-5GB of GPU memory. A 32B model requires ~18-20GB, potentially leaving insufficient memory for other services.

**Key tradeoff**: Model size directly competes with system memory for Worker, AI service, PostgreSQL connections, and Temporal client.

### dev02 — Linux Server (AMD64)

| Resource | Specification |
|----------|--------------|
| CPU | AMD64 |
| Role | Gateway service, PostgreSQL, Temporal, Langfuse |
| No GPU | Cannot run local ML models |

### Current AI Model Configuration

| Model | Size | Quantization | Memory | Speed |
|-------|------|-------------|--------|-------|
| Qwen2.5-7B-Instruct | 7B params | 4-bit | ~4-5GB | ~40-60 tok/s |
| mxbai-embed-large-v1 | ~335M params | FP16 | ~670MB | Fast |

**Alternative models considered**:
- 14B (4-bit): ~8-9GB, ~25-35 tok/s — good middle ground
- 32B (4-bit): ~18-20GB, ~8-15 tok/s — better quality but memory-constrained

## Cost Constraints

### Local Inference (MLX)
- **Cost per request**: $0.00 (electricity only)
- **Capacity**: Limited by hardware throughput
- **Privacy**: Content never leaves the machine

### Remote Inference (Gemini)

| Model | Input Cost | Output Cost |
|-------|-----------|-------------|
| Gemini 2.0 Flash | ~$0.10/M tokens | ~$0.40/M tokens |
| Gemini 2.0 Pro | ~$1.25/M tokens | ~$5.00/M tokens |

### Remote Inference (OpenAI)

| Model | Input Cost | Output Cost |
|-------|-----------|-------------|
| GPT-4o-mini | ~$0.15/M tokens | ~$0.60/M tokens |
| GPT-4o | ~$2.50/M tokens | ~$10.00/M tokens |

### Cost Implications

Processing 1,000 emails (average 2K chars each = ~500 tokens each):
- **Local SLM only**: $0.00
- **Remote LLM for all**: ~$0.30-1.50 depending on model
- **Hybrid (SLM triage + selective LLM)**: ~$0.03-0.15 (only 10-20% of content goes to LLM)

Processing 100 meeting transcripts (average 50K chars each = ~12,500 tokens each):
- **Local SLM only**: $0.00 but requires chunking and quality may suffer
- **Remote LLM for all**: ~$6-75 depending on model
- **Hybrid**: ~$1-15 (SLM extracts, LLM synthesizes summaries for key meetings)

## Current Limitations

### 1. Content Truncation
The `AnalyzeByID` command truncates content at 8,000 characters before sending to the LLM. This means:
- Meeting transcripts (44-64K chars) lose 85-90% of content
- Long email threads lose context
- Structured reports lose detailed sections

### 2. No Chunking Strategy for AI
While `splitIntoChunks()` exists in the worker (1000-char chunks with 100-char overlap for embeddings), there is no chunking strategy for LLM inference. Long content is simply truncated rather than processed in chunks.

### 3. No Context Assembly
AI calls don't receive relevant context before processing:
- No glossary terms injected (the LLM doesn't know "TER" = "Technical Execution Review")
- No known people context (can't match "JB" to a known person)
- No project context (doesn't know what project this content relates to)
- No prior assertions (doesn't know about existing risks/decisions)

### 4. Single Model Path
All AI tasks use the same model regardless of task complexity:
- Simple classification (could be SLM) uses the same model as deep reasoning (needs LLM)
- No cost optimization — every task pays the same compute cost
- No task-appropriate model selection

### 5. No Progressive Availability
Content isn't searchable until the entire pipeline completes. If summarization fails, the content isn't available for search even though embeddings may have succeeded.

### 6. One-Way Extraction
Assertions (risks, actions, decisions) are extracted from content but don't feed back into the knowledge base as context for processing future content. Each piece of content is processed in isolation.

### 7. No Significance Classification
Every mention of a topic is treated equally. If a risk is mentioned in passing in 15 meetings but deeply discussed in 3, there's no way to distinguish which meetings matter.

### 8. Byte-Level Truncation
The `truncateText()` function slices at byte boundaries rather than rune boundaries, potentially breaking multi-byte UTF-8 characters:
```go
func truncateText(text string, maxLen int) string {
    if len(text) <= maxLen { return text }
    return text[:maxLen]
}
```

## Operational Constraints

### Single-Machine AI
All AI processing runs on dev01. There is no horizontal scaling — if dev01 is down, all AI operations stop.

### Temporal Dependency
The worker service depends on Temporal (running on dev02). Network issues between dev01 and dev02 affect all async processing.

### No GPU on Gateway Machine
dev02 (Gateway) has no GPU and cannot run local ML models. All ML inference must happen on dev01 or via remote APIs.

### Batch Processing Limits
Large batch ingestions (thousands of emails) can saturate the worker's processing capacity, particularly the LLM inference queue which is single-threaded on the model.

## Volume Estimates

Based on real test data analysis:

| Content Type | Volume | Avg Size | Processing Time (7B) |
|-------------|--------|----------|---------------------|
| Emails | 267 test, ~500-2000 expected | 2K chars (median) | ~2-5 sec each |
| Meeting transcripts | 18 test, ~50-200 expected | 50K chars | ~30-120 sec each (chunked) |
| Calendar events | TBD | Minimal | <1 sec each |
| Slack messages | TBD (planned) | ~200-500 chars each | ~1-2 sec each |

### Throughput Estimates (7B model)

At ~40-60 tokens/second on Apple Silicon:
- Simple classification: ~2-3 seconds
- Email summarization (2K chars): ~5-10 seconds
- Transcript chunk extraction (4K chars): ~10-20 seconds
- Full transcript (map-reduce, 10 chunks): ~2-4 minutes

These are per-content-item estimates. Batch processing can parallelize embedding generation but LLM inference is sequential.
