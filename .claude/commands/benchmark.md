# Benchmark

Run multi-model benchmarks on a document and record results to Langfuse.

## Arguments: $ARGUMENTS

Required: File path to the document to benchmark (e.g., `meeting-transcript.txt`, `./notes/standup.md`)

## Overview

This workflow benchmarks a document across multiple local LLM models, measuring their performance on standardized tasks (summarization, assertion extraction, classification). Results are recorded to Langfuse for comparison and analysis.

## Prerequisites

- At least one model downloaded (`penf model list` should show downloaded models)
- Model server capability (MLX sidecar environment configured)
- Langfuse credentials configured (LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY)

## Instructions

### Phase 1: Validate Input

1. **Check document exists**:
   ```bash
   FILE_PATH="$ARGUMENTS"
   if [ ! -f "$FILE_PATH" ]; then
       echo "Error: File not found: $FILE_PATH"
       exit 1
   fi
   ```

2. **Read document content**:
   ```bash
   cat "$FILE_PATH"
   ```

   Store the content for use in benchmark tasks. Note the approximate word count and content type.

3. **Check Langfuse configuration**:
   ```bash
   # Verify Langfuse env vars are set
   if [ -z "$LANGFUSE_HOST" ] || [ -z "$LANGFUSE_PUBLIC_KEY" ] || [ -z "$LANGFUSE_SECRET_KEY" ]; then
       echo "Warning: Langfuse credentials not configured."
       echo "Results will not be recorded to Langfuse."
       echo "Set LANGFUSE_HOST, LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY to enable recording."
   fi
   ```

### Phase 2: Discover Available Models

1. **Query downloaded models**:
   ```bash
   penf model list -o json
   ```

2. **Parse the JSON output** to extract model information:
   - Model ID (e.g., `mlx-community/Qwen2.5-7B-Instruct-4bit`)
   - Model name (e.g., `Qwen 2.5 7B (4-bit)`)
   - Expected latency
   - Download status (should all be `downloaded: true`)

3. **Present available models**:
   ```
   ## Available Models for Benchmarking

   | # | Model | Size | Expected Latency |
   |---|-------|------|------------------|
   | 1 | Qwen 2.5 32B (4-bit) | ~18GB | 10-20s |
   | 2 | Qwen 2.5 7B (4-bit) | ~4GB | 3-8s |
   | 3 | Phi-3.5 Mini (4-bit) | ~2.5GB | 1-4s |
   ```

4. **If no models available**:
   ```
   No downloaded models found.

   Download models first:
     penf model download phi      # Small, fast (~2.5GB)
     penf model download qwen-7b  # Medium, balanced (~4GB)
     penf model download qwen-32b # Large, capable (~18GB)
   ```
   Stop workflow if no models.

### Phase 3: Model Selection

**Ask the user which models to benchmark:**

```
Which models would you like to benchmark?

Options:
- Enter numbers separated by commas (e.g., "1,3")
- Enter "all" to benchmark all models
- Enter model short names (e.g., "phi, qwen-7b")
```

**Wait for user response.**

Store selected models in a list for Phase 5.

### Phase 4: Task Selection

**Ask the user which benchmark tasks to run:**

```
Which benchmark tasks should I run?

1. Summarization - Generate a concise summary of the document
2. Assertion Extraction - Extract key facts, decisions, and action items
3. Classification - Classify document type and extract metadata
4. All of the above

Enter your choice (1-4 or task names):
```

**Wait for user response.**

Store selected tasks. Default to "all" if unclear.

### Phase 5: Execute Benchmarks

**IMPORTANT**: This phase runs sequentially. Each model must be loaded before testing.

1. **Initialize tracking**:
   ```
   ## Benchmark Execution

   Document: $FILE_PATH
   Models: [list selected models]
   Tasks: [list selected tasks]

   Starting benchmark at [timestamp]...
   ```

2. **Create Langfuse dataset** (if configured):
   - Dataset name: `benchmark-{timestamp}` (e.g., `benchmark-20240124-143052`)
   - Description: Document filename and selected tasks
   - This will store the benchmark results for comparison

3. **For each selected model**:

   a. **Switch to model**:
      ```bash
      penf model switch <model-short-name> --port 8080
      ```

   b. **Wait for health check**:
      The switch command includes health checking. If it fails, record the error and continue to next model.

   c. **Report progress**:
      ```
      [1/3] Testing Phi-3.5 Mini...
      ```

   d. **For each selected task**, send the prompt and measure response:

      **Task: Summarization**
      ```
      Prompt: "Summarize the following document in 2-3 paragraphs, highlighting the main points and any key decisions made:

      ---
      {document_content}
      ---"
      ```

      **Task: Assertion Extraction**
      ```
      Prompt: "Extract all assertions, decisions, and action items from this document. Format as a bulleted list with categories:

      ## Facts/Assertions
      - ...

      ## Decisions Made
      - ...

      ## Action Items
      - ...

      ---
      {document_content}
      ---"
      ```

      **Task: Classification**
      ```
      Prompt: "Analyze this document and provide:
      1. Document type (meeting notes, email, report, etc.)
      2. Primary topics discussed (up to 5)
      3. Key people mentioned
      4. Sentiment/tone (professional, urgent, casual, etc.)
      5. Recommended follow-up actions

      ---
      {document_content}
      ---"
      ```

   e. **Record results**:
      - Response text
      - Response time (latency)
      - Token count (if available)
      - Any errors encountered

   f. **Display result summary**:
      ```
      [1/3] Phi-3.5 Mini
        - Summarization: 2.3s (245 tokens)
        - Extraction: 3.1s (312 tokens)
        - Classification: 1.8s (156 tokens)
      ```

### Phase 6: Record to Langfuse

**If Langfuse is configured**, record all results:

1. **Create dataset item** for the benchmark input:
   - Input: The document content
   - Metadata: filename, word count, benchmark timestamp

2. **For each model/task combination**, create a dataset run item:
   - Link to the dataset item
   - Include model name, task type, latency, output
   - Add metadata: token count, errors if any

3. **Get the Langfuse URL**:
   - Construct URL: `{LANGFUSE_HOST}/project/{projectId}/datasets/{datasetName}`

### Phase 7: Present Results

**Display comprehensive results:**

```
## Benchmark Results

Document: meeting-notes-2024-01-24.txt (523 words)
Timestamp: 2024-01-24 14:30:52

### Performance Comparison

| Model | Summarization | Extraction | Classification | Total |
|-------|--------------|------------|----------------|-------|
| Phi-3.5 Mini | 2.3s | 3.1s | 1.8s | 7.2s |
| Qwen 2.5 7B | 4.2s | 5.8s | 3.1s | 13.1s |
| Qwen 2.5 32B | 12.4s | 15.2s | 9.8s | 37.4s |

### Fastest by Task
- Summarization: Phi-3.5 Mini (2.3s)
- Extraction: Phi-3.5 Mini (3.1s)
- Classification: Phi-3.5 Mini (1.8s)

### Quality Notes
[Add any observations about response quality differences]

### Langfuse Dashboard
View detailed results: https://langfuse.example.com/datasets/benchmark-20240124-143052

### Raw Outputs
[Optionally show the actual responses, or offer to display them]
```

### Phase 8: Cleanup and Recommendations

1. **Ask about cleanup**:
   ```
   Would you like me to:
   1. Stop the model server (currently running: [model])
   2. Keep it running for further testing
   3. Switch to a specific model
   ```

2. **Provide recommendations**:
   ```
   ### Recommendations

   Based on the benchmark results:
   - For speed: Use Phi-3.5 Mini (fastest overall)
   - For quality: [Assess based on response quality]
   - For balanced: Qwen 2.5 7B offers good speed/quality tradeoff

   To re-run this benchmark:
     /benchmark $FILE_PATH

   To view all benchmarks:
     penf langfuse datasets list
   ```

## Error Handling

### Model Switch Failures
If a model fails to start:
```
Warning: Failed to start [model-name]
Error: [error message]

Skipping this model and continuing with remaining models.
```

### API Call Failures
If a task call fails:
```
Warning: [task] failed for [model]
Error: [error message]

Recording failure in results.
```

### Langfuse Recording Failures
If Langfuse API fails:
```
Warning: Failed to record to Langfuse
Error: [error message]

Results are still displayed above but not persisted to Langfuse.
```

## Notes

### Model Short Names
- `phi` -> `mlx-community/Phi-3.5-mini-instruct-4bit`
- `qwen-7b` -> `mlx-community/Qwen2.5-7B-Instruct-4bit`
- `qwen-32b` -> `mlx-community/Qwen2.5-32B-Instruct-4bit`
- `llama` -> `mlx-community/Llama-3.2-3B-Instruct-4bit`
- `gemma` -> `mlx-community/gemma-2-9b-it-4bit`

### Calling the MLX Server

The MLX server exposes an OpenAI-compatible API at `http://localhost:8080/v1/chat/completions`.

Example request:
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Your prompt here"}],
    "max_tokens": 1024,
    "temperature": 0.7
  }'
```

### Timing Considerations
- Small models (phi, llama-3b): 1-5s per task
- Medium models (qwen-7b, gemma-9b): 3-10s per task
- Large models (qwen-32b): 10-30s per task

Plan for total benchmark time based on models and tasks selected.

### Langfuse Integration

The `pkg/langfuse/client.go` provides the Go client for Langfuse Datasets API:

```go
// Create a client from environment
client, err := langfuse.NewClientFromEnv()

// Create a dataset for the benchmark
dataset, err := client.CreateDataset(ctx, &langfuse.CreateDatasetRequest{
    Name:        "benchmark-20240124-143052",
    Description: "Benchmark: meeting-notes.txt - all tasks",
    Metadata: map[string]interface{}{
        "file":      filename,
        "wordCount": wordCount,
        "models":    selectedModels,
        "tasks":     selectedTasks,
    },
})

// Create a dataset item for the input document
item, err := client.CreateDatasetItem(ctx, &langfuse.CreateDatasetItemRequest{
    DatasetName: dataset.Name,
    Input:       documentContent,
    Metadata: map[string]interface{}{
        "filename": filename,
    },
})

// Record each benchmark result as a run item
runItem, err := client.CreateDatasetRunItem(ctx, &langfuse.CreateDatasetRunItemRequest{
    DatasetItemID: item.ID,
    TraceID:       traceID, // From the model call
    RunName:       "phi-summarization",
    RunDescription: "Phi-3.5 Mini - Summarization task",
    Metadata: map[string]interface{}{
        "model":     "phi",
        "task":      "summarization",
        "latency":   2.3,
        "tokens":    245,
    },
})
```

The Langfuse dashboard URL will be:
`{LANGFUSE_HOST}/project/{projectId}/datasets/{datasetName}`

### Response Parsing

The MLX server returns OpenAI-compatible responses:

```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1706123456,
  "model": "mlx-community/Phi-3.5-mini-instruct-4bit",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "The response text..."
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  }
}
```

Use the `usage` field to record token counts in the benchmark results.
