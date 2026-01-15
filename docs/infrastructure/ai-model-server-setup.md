# AI Model Server Setup

This guide covers the complete setup of the AI model server for Penfold on Mac Mini M4 hardware, including Ollama installation, model management, resource allocation, and integration with Penfold's AI coordination framework.

## Table of Contents

- [Hardware Context](#hardware-context)
- [Ollama Installation and Configuration](#ollama-installation-and-configuration)
- [Model Management](#model-management)
- [Resource Allocation](#resource-allocation)
- [Model Selection Guide](#model-selection-guide)
- [Performance Benchmarking](#performance-benchmarking)
- [Integration with Penfold](#integration-with-penfold)
- [Troubleshooting](#troubleshooting)

---

## Hardware Context

### Mac Mini M4 Specifications

- **CPU**: Apple M4 chip with Neural Engine
- **RAM**: 32GB unified memory
- **Storage**: 2TB SSD
- **Role**: Primary development, real-time processing, model serving

### Memory Budget for AI Workloads

| Component | Memory Allocation |
|-----------|-------------------|
| Llama 3.1 8B | ~8GB |
| Phi-3 Mini 3.8B | ~4GB |
| Qwen2.5 7B | ~7GB |
| System overhead | ~5GB |
| **Maximum concurrent** | ~24GB |

With 32GB unified memory, you can run multiple models simultaneously while leaving headroom for system operations and development tools.

---

## Ollama Installation and Configuration

### Installation

Install Ollama on macOS using the official installer:

```bash
# Download and install via curl
curl -fsSL https://ollama.ai/install.sh | sh

# Verify installation
ollama --version
```

Alternatively, install via Homebrew:

```bash
brew install ollama
```

### Starting the Ollama Service

```bash
# Start Ollama as a background service
ollama serve

# Or run in the foreground for debugging
OLLAMA_DEBUG=1 ollama serve
```

For automatic startup on boot:

```bash
# Create a launchd plist for automatic startup
cat > ~/Library/LaunchAgents/com.ollama.server.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ollama.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/ollama</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/ollama.out.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/ollama.err.log</string>
</dict>
</plist>
EOF

# Load the service
launchctl load ~/Library/LaunchAgents/com.ollama.server.plist
```

### Configuration Options

Set environment variables for Ollama customization:

```bash
# Add to ~/.zshrc
export OLLAMA_HOST="127.0.0.1:11434"     # Bind address
export OLLAMA_ORIGINS="*"                 # CORS settings (for web access)
export OLLAMA_NUM_PARALLEL=4              # Max concurrent requests
export OLLAMA_MAX_LOADED_MODELS=3         # Max models in memory
export OLLAMA_KEEP_ALIVE="5m"             # Model unload timeout
export OLLAMA_MODELS="$HOME/.ollama/models"  # Model storage location
```

### Verifying Installation

```bash
# Check service status
curl http://localhost:11434/api/tags

# Test model inference
ollama run llama3.1 "Hello, how are you?"
```

---

## Model Management

### Pulling Models

Pull the recommended models for Penfold workloads:

```bash
# Primary models for local processing
ollama pull llama3.1:8b          # General-purpose processing
ollama pull phi3:mini            # Fast, lightweight tasks
ollama pull qwen2.5:7b           # Multi-project detection

# Embedding models
ollama pull nomic-embed-text     # Vector embeddings

# Specialized models
ollama pull codellama:7b         # Technical content analysis
ollama pull llama3.2:3b          # Fast classification
```

### Model Versioning

Ollama uses tags for version management:

```bash
# Pull specific versions
ollama pull llama3.1:8b-instruct-q4_0    # Quantized for speed
ollama pull llama3.1:8b-instruct-fp16    # Full precision for accuracy

# List local models with sizes
ollama list

# Example output:
# NAME                     ID            SIZE      MODIFIED
# llama3.1:8b              365c0bd3c000  4.7 GB    2 weeks ago
# phi3:mini                4f2222927938  2.2 GB    3 weeks ago
# qwen2.5:7b               845dbda0ea48  4.4 GB    1 week ago
```

### Model Storage and Cleanup

```bash
# Check model storage location
ls -la ~/.ollama/models/

# Remove unused models
ollama rm <model_name>

# Estimate storage needs (approximate sizes)
# llama3.1:8b     ~4.7GB
# phi3:mini       ~2.2GB
# qwen2.5:7b      ~4.4GB
# nomic-embed     ~0.3GB
# codellama:7b    ~3.8GB
```

### Custom Model Configuration

Create a Modelfile for custom configurations:

```bash
# Create custom Penfold model
cat > ~/penfold-llama.modelfile << 'EOF'
FROM llama3.1:8b

# System prompt for Penfold tasks
SYSTEM """You are an AI assistant specialized in analyzing business communications
for a COO. Focus on extracting: people mentioned, projects referenced, decisions made,
action items, and timeline information. Be concise and structured in your responses."""

# Optimal parameters for classification tasks
PARAMETER temperature 0.1
PARAMETER top_p 0.8
PARAMETER top_k 20
PARAMETER num_predict 500
PARAMETER repeat_penalty 1.1
EOF

# Create the custom model
ollama create penfold-llama -f ~/penfold-llama.modelfile

# Use the custom model
ollama run penfold-llama "Summarize this email about the Atlas project"
```

---

## Resource Allocation

### M4 Unified Memory Optimization

The M4's unified memory architecture is optimal for LLM workloads:

```bash
# Set memory allocation for Ollama
export OLLAMA_GPU_OVERHEAD="256MiB"  # GPU memory reserve

# Monitor memory usage
# Using Activity Monitor or:
memory_pressure  # Shows memory status
```

### Model Loading Strategy

Configure model lifecycle for optimal performance:

```bash
# Keep frequently used models loaded
export OLLAMA_KEEP_ALIVE="30m"  # Extended keep-alive for hot models

# Limit concurrent models to prevent memory pressure
export OLLAMA_MAX_LOADED_MODELS=2
```

### CPU/GPU Configuration

The M4 chip uses a unified memory architecture, meaning CPU and GPU share the same memory pool:

```python
# In Penfold configuration
OLLAMA_CONFIG = {
    "num_thread": 8,       # Use all performance cores
    "num_gpu": 1,          # Use GPU layers
    "main_gpu": 0,         # Primary GPU
    "low_vram": False,     # M4 has ample unified memory
}
```

### Concurrent Processing Limits

For parallel processing across multiple models:

```bash
# Recommended settings for Mac Mini M4 32GB
export OLLAMA_NUM_PARALLEL=2        # Max 2 parallel inferences
export OLLAMA_MAX_LOADED_MODELS=3   # Max 3 models in memory
```

---

## Model Selection Guide

### Penfold Task-to-Model Mapping

| Task | Recommended Model | Reasoning |
|------|-------------------|-----------|
| Email summarization | llama3.1:8b | Best balance of speed and quality |
| Entity extraction | qwen2.5:7b | Excellent at structured outputs |
| Quick categorization | phi3:mini | Fastest response time |
| Technical analysis | codellama:7b | Code and technical domain expertise |
| Embedding generation | nomic-embed-text | Consistent vector quality |
| Complex synthesis | Cloud (Gemini) | When local models underperform |

### Content-Type Model Assignment

Based on Penfold's local-first AI strategy:

**Meetings by Type:**
- Technical discussions: codellama:7b (primary), qwen2.5:7b (fallback)
- Customer calls: llama3.1:8b (fine-tuned for interactions)
- Internal 1:1s: phi3:mini (lightweight, sufficient)
- Executive meetings: Multi-model ensemble, cloud escalation as needed

**Email by Content:**
- Technical emails: codellama:7b (primary)
- Project updates: qwen2.5:7b (multi-project detection)
- External communications: llama3.1:8b (conservative, accurate)

### Model Selection Logic

```python
# Example model selection from Penfold's AI coordination
from penf_lib.ai_coordination.models import ModelProfile, ModelType

LOCAL_MODELS = {
    "summarization": "llama3.1:8b",
    "entity_extraction": "qwen2.5:7b",
    "categorization": "phi3:mini",
    "technical_analysis": "codellama:7b",
    "embedding": "nomic-embed-text",
}

def select_model_for_task(task_type: str, content_type: str, content_size: int) -> str:
    """Select optimal model based on task characteristics."""
    if task_type == "embedding":
        return LOCAL_MODELS["embedding"]

    if content_size < 500 and task_type == "categorization":
        return LOCAL_MODELS["categorization"]  # Fast model for small content

    if content_type == "technical" or "code" in content_type:
        return LOCAL_MODELS["technical_analysis"]

    return LOCAL_MODELS.get(task_type, "llama3.1:8b")
```

---

## Performance Benchmarking

### Baseline Benchmarks

Run baseline benchmarks to establish performance expectations:

```bash
# Create benchmark script
cat > ~/penfold-benchmark.sh << 'EOF'
#!/bin/zsh

echo "=== Penfold AI Model Benchmarks ==="
echo "Date: $(date)"
echo "Hardware: Mac Mini M4 32GB"
echo ""

# Test content
TEST_CONTENT="This is a test email about the Atlas project.
We discussed the timeline delays with Sarah Chen and John Smith.
The decision was made to push back the launch by 2 weeks."

# Benchmark each model
for model in llama3.1:8b phi3:mini qwen2.5:7b codellama:7b; do
    echo "--- Testing $model ---"

    # Ensure model is loaded
    ollama run $model "warmup" > /dev/null 2>&1

    # Time 5 inference runs
    total_time=0
    for i in {1..5}; do
        start_time=$(date +%s.%N)
        ollama run $model "Summarize: $TEST_CONTENT" > /dev/null 2>&1
        end_time=$(date +%s.%N)
        elapsed=$(echo "$end_time - $start_time" | bc)
        total_time=$(echo "$total_time + $elapsed" | bc)
    done

    avg_time=$(echo "scale=2; $total_time / 5" | bc)
    echo "Average inference time: ${avg_time}s"
    echo ""
done
EOF

chmod +x ~/penfold-benchmark.sh
~/penfold-benchmark.sh
```

### Expected Performance Targets

| Model | Cold Start | Warm Inference | Tokens/sec |
|-------|------------|----------------|------------|
| llama3.1:8b | 3-5s | 0.5-1.5s | 30-50 |
| phi3:mini | 2-3s | 0.3-0.8s | 50-80 |
| qwen2.5:7b | 3-4s | 0.5-1.2s | 35-55 |
| codellama:7b | 3-4s | 0.5-1.2s | 35-55 |

### Continuous Performance Monitoring

Integrate with Penfold's performance tracking:

```python
# From penf_lib/ai_coordination/performance.py
from penf_lib.ai_coordination.performance import PerformanceTracker

async def benchmark_model_suite():
    """Run comprehensive model benchmarks."""
    tracker = PerformanceTracker(session)

    test_content = "Test email content for benchmarking..."
    models = ["llama3.1:8b", "phi3:mini", "qwen2.5:7b"]

    for model in models:
        for content_type in ["email", "meeting", "document"]:
            start = time.time()
            result = await ollama_client.generate(model, test_content)
            elapsed_ms = (time.time() - start) * 1000

            await tracker.record_performance(
                model_id=model,
                content_type=content_type,
                processing_time=elapsed_ms,
                confidence_score=0.9,  # Placeholder
                success=True
            )

    # Get performance trends
    trends = await tracker.get_performance_trends(model_ids=models)
    return trends
```

### Weekly Performance Review

From Penfold's local-first AI strategy:

```bash
# Weekly model performance review
penf analyze models --last-week
# Expected output:
# > Processing summary:
# > - 15 meetings processed
# > - Local success rate: 78% (up from 65% last week)
# > - Cloud escalations: 3 meetings (complex customer calls)
# > - Best local model: Qwen2.5 7B (avg 88% accuracy)
# >
# > Recommendations:
# > - Fine-tune Qwen on meeting data
# > - Phi-3 struggling with technical discussions
# > - Consider trying Code Llama for technical meetings
```

---

## Integration with Penfold

### AI Coordination Framework

Penfold uses a multi-model coordination framework defined in `penf_lib/ai_coordination/`:

**Key Components:**
- `coordinator.py`: ModelCoordinator for parallel processing
- `models.py`: ModelProfile definitions and capabilities
- `performance.py`: PerformanceTracker for learning
- `escalation.py`: EscalationManager for cloud fallback
- `ensemble.py`: EnsembleCombiner for result aggregation

### Registering Models

```python
from penf_lib.ai_coordination.coordinator import ModelCoordinator
from penf_lib.ai_coordination.models import ModelProfile, ModelType, ModelCapability

# Define local model profile
llama_profile = ModelProfile(
    model_id="llama-3.1-8b",
    name="Llama 3.1 8B (Local)",
    model_type=ModelType.LOCAL,
    capabilities=[
        ModelCapability.TEXT_GENERATION,
        ModelCapability.SUMMARIZATION,
        ModelCapability.ENTITY_EXTRACTION,
        ModelCapability.CLASSIFICATION
    ],
    supported_content_types=["email", "document", "meeting", "text"],
    max_input_tokens=8192,
    avg_response_time_ms=2000,
    cost_per_1k_tokens=0.0,  # Local model
    confidence_reliability=0.7,
    priority=2
)

# Register with coordinator
coordinator = ModelCoordinator(session, event_publisher, job_manager)
await coordinator.register_model(
    model_id="llama-3.1-8b",
    model_profile=llama_profile,
    event_types=["email", "document", "meeting"]
)
```

### Event-Driven Processing

Penfold uses pub-sub for AI coordination:

```python
# Event processing subscribers
@subscribe_to('content.ingested')
async def summarize_content(event):
    content = event.payload['content']
    summary = await ollama_client.generate(
        model="llama3.1:8b",
        prompt=f"Summarize the following: {content}"
    )
    await publish_result('content.summarized', summary, event.id)

@subscribe_to('content.ingested')
async def extract_entities(event):
    content = event.payload['content']
    entities = await ollama_client.generate(
        model="qwen2.5:7b",
        prompt=f"Extract people, projects, and decisions from: {content}"
    )
    await publish_result('entities.extracted', entities, event.id)
```

### Environment Configuration

Set up environment for Penfold AI integration:

```bash
# .env or environment variables
OLLAMA_HOST=http://localhost:11434
OLLAMA_TIMEOUT=60

# Model preferences
PENFOLD_DEFAULT_MODEL=llama3.1:8b
PENFOLD_FAST_MODEL=phi3:mini
PENFOLD_ENTITY_MODEL=qwen2.5:7b
PENFOLD_TECHNICAL_MODEL=codellama:7b
PENFOLD_EMBEDDING_MODEL=nomic-embed-text

# Escalation thresholds
PENFOLD_CONFIDENCE_THRESHOLD=0.8
PENFOLD_CLOUD_ESCALATION_ENABLED=true
```

### CLI Integration

```bash
# Process meeting with multiple local models for comparison
penf meeting process --meeting-id <uuid> --models "llama,phi,qwen"

# Re-process with different approach
penf meeting reprocess --meeting-id <uuid> --strategy cloud

# Compare processing approaches
penf meeting compare --meeting-id <uuid>
```

---

## Troubleshooting

### Common Issues

**Model fails to load:**
```bash
# Check available memory
memory_pressure

# Reduce concurrent models
export OLLAMA_MAX_LOADED_MODELS=1

# Check model integrity
ollama show llama3.1:8b --modelfile
```

**Slow inference:**
```bash
# Check if model is already loaded
curl http://localhost:11434/api/tags

# Warm up the model
ollama run llama3.1:8b "warmup"

# Check system load
top -l 1 | head -10
```

**Connection refused:**
```bash
# Verify Ollama is running
pgrep ollama

# Start Ollama
ollama serve

# Check port availability
lsof -i :11434
```

**Out of memory:**
```bash
# Reduce model size with quantization
ollama pull llama3.1:8b-instruct-q4_0

# Limit concurrent models
export OLLAMA_MAX_LOADED_MODELS=1
export OLLAMA_NUM_PARALLEL=1
```

### Health Check Script

```bash
#!/bin/zsh
# penfold-ai-health.sh

echo "=== Penfold AI Health Check ==="

# Check Ollama service
if curl -s http://localhost:11434/api/tags > /dev/null; then
    echo "[OK] Ollama service running"
else
    echo "[FAIL] Ollama service not responding"
    exit 1
fi

# Check required models
REQUIRED_MODELS=("llama3.1:8b" "phi3:mini" "qwen2.5:7b" "nomic-embed-text")
for model in $REQUIRED_MODELS; do
    if ollama list | grep -q "$model"; then
        echo "[OK] Model $model available"
    else
        echo "[WARN] Model $model not found"
    fi
done

# Check memory
free_mem=$(memory_pressure | grep "Pages free" | awk '{print $3}')
echo "[INFO] Memory pressure status:"
memory_pressure | head -5

# Test inference
echo "[INFO] Testing inference..."
response=$(ollama run phi3:mini "Say hello" 2>&1)
if [[ -n "$response" ]]; then
    echo "[OK] Inference working"
else
    echo "[FAIL] Inference failed"
fi

echo "=== Health check complete ==="
```

### Logs and Debugging

```bash
# Ollama logs (if using launchd)
tail -f /tmp/ollama.err.log
tail -f /tmp/ollama.out.log

# Debug mode
OLLAMA_DEBUG=1 ollama serve

# Verbose API responses
curl -v http://localhost:11434/api/generate \
  -d '{"model": "llama3.1:8b", "prompt": "test"}'
```

---

## Quick Reference

### Essential Commands

```bash
# Service management
ollama serve                    # Start server
ollama list                     # List models
ollama pull <model>            # Download model
ollama rm <model>              # Remove model

# Model testing
ollama run <model> "prompt"    # Interactive
ollama show <model>            # Model info

# Health checks
curl http://localhost:11434/api/tags     # API check
memory_pressure                           # Memory status
```

### Recommended Model Set for Penfold

| Model | Size | Purpose |
|-------|------|---------|
| llama3.1:8b | 4.7GB | Primary processing |
| phi3:mini | 2.2GB | Fast classification |
| qwen2.5:7b | 4.4GB | Entity extraction |
| nomic-embed-text | 0.3GB | Embeddings |
| codellama:7b | 3.8GB | Technical content |

**Total storage needed:** ~15GB for recommended models

### Performance Targets

- Cold start: < 5 seconds
- Warm inference: < 2 seconds
- Embedding generation: < 500ms
- Concurrent capacity: 2 parallel inferences
