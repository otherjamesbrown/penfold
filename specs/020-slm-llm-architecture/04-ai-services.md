# Penfold: AI Services

## Overview

Penfold's AI layer coordinates all machine learning operations through a dedicated AI Service that abstracts multiple backends behind a unified interface. The system supports local models (MLX on Apple Silicon) and remote models (Gemini, OpenAI), with a routing layer that selects models based on task type, cost, latency, and quality requirements.

## AI Service Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                       AI SERVICE                                  │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    gRPC Server                               │ │
│  │  GenerateEmbedding | GenerateSummary | ExtractAssertions    │ │
│  │  ClassifyContent | Query | AnalyzeByID | SummarizeByID     │ │
│  └──────────────────────────┬──────────────────────────────────┘ │
│                              │                                    │
│  ┌──────────────────────────▼──────────────────────────────────┐ │
│  │                    Model Router                              │ │
│  │  - Task-based routing rules                                  │ │
│  │  - Circuit breakers per backend                              │ │
│  │  - Health checks                                             │ │
│  │  - Fallback chains                                           │ │
│  │  - Optimization modes (latency/quality/cost/balanced)       │ │
│  └───┬──────────────┬───────────────┬──────────────────────────┘ │
│      │              │               │                             │
│  ┌───▼────┐   ┌────▼─────┐   ┌────▼──────┐                     │
│  │  MLX   │   │  Gemini  │   │  OpenAI   │                     │
│  │(local) │   │ (remote) │   │ (remote)  │                     │
│  └────────┘   └──────────┘   └───────────┘                     │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                 Model Registry (DB-backed)                   │ │
│  │  ai_models | ai_routing_rules | ai_model_health             │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                 Langfuse Integration                         │ │
│  │  LLM call tracing, token counting, latency tracking         │ │
│  └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

## AI Backends

### MLX Backend (Local — Apple Silicon)

The MLX backend runs models locally on Apple Silicon hardware using the MLX framework, providing:
- **Zero-cost inference** (no API charges)
- **Low latency** (no network round-trip)
- **Privacy** (content never leaves the machine)

**Current Models:**
| Purpose | Model | Details |
|---------|-------|---------|
| Embeddings | mxbai-embed-large-v1 | 1024 dimensions, runs at localhost:8081 |
| LLM | Qwen2.5-7B-Instruct-4bit | 7B parameter, 4-bit quantized, runs at localhost:8080 |

**API**: OpenAI-compatible REST API
**Timeout**: 120 seconds default for LLM operations
**Embedding format**: Float64→Float32 conversion

### Gemini Backend (Remote — Google)

**Current Models:**
| Purpose | Model | Details |
|---------|-------|---------|
| Embeddings | text-embedding-004 | Google's latest embedding model |
| LLM | gemini-2.0-flash | Fast, capable reasoning model |

**API**: Google Generative AI REST API with API key auth
**Features**: System instruction support, JSON mode, generation config (temperature, maxOutputTokens)

### OpenAI Backend (Remote)

**Current Models:**
| Purpose | Model | Details |
|---------|-------|---------|
| Embeddings | text-embedding-3-small | Cost-effective embedding model |
| LLM | gpt-4o-mini | Compact reasoning model |

**API**: OpenAI REST API (also supports Azure OpenAI deployments)
**Features**: Organization support, JSON mode, comprehensive error handling

## Backend Interface

All backends implement a common interface:

```
Backend:
  GenerateEmbedding(text, model) → EmbeddingResult ([]float32, dimensions, model, tokens)
  ChatCompletion(messages, options) → CompletionResult (content, model, prompt_tokens, completion_tokens, finish_reason)
  CheckEmbeddingsHealth() → error
  CheckLLMHealth() → error
  Close() → error
```

**ChatCompletionOptions**: model, temperature, max_tokens, json_mode, system_prompt

## Model Router

The router selects which backend to use for each request based on:

### Routing Rules (Database-Driven)

Each rule specifies:
- **task_type**: embedding, summarization, extraction, classification
- **preferred_models**: Ordered list of model IDs to try
- **fallback_models**: Models to try if all preferred models fail
- **require_local**: Force local-only (for privacy-sensitive content)
- **max_cost_per_request**: Cost ceiling
- **optimization_mode**: latency | quality | cost | balanced

### Circuit Breakers

Each backend has a circuit breaker that:
- Opens after repeated failures
- Redirects to fallback backends
- Half-opens periodically to test recovery
- Tracks health metrics (avg_latency_ms, error_count, success_count)

### Health Monitoring

The `ai_model_health` table tracks per-model:
- status: healthy, degraded, unhealthy, unknown
- last_check timestamp
- avg_latency_ms
- error_count / success_count
- last_error and last_error_at

## Current AI Operations

### 1. Embeddings (GenerateEmbedding)
- **Input**: Text content
- **Output**: 1024-dimension float32 vector
- **Used for**: Semantic search, glossary matching
- **Primary model**: MLX local (mxbai-embed-large-v1)
- **Criticality**: REQUIRED — workflow fails without embeddings

### 2. Summarization (GenerateSummary)
- **Input**: Content text, summary style (brief/detailed/bullet_points/technical)
- **Output**: Summary text
- **Used for**: Content summaries, daily reviews
- **Primary model**: Local 7B SLM
- **Criticality**: Optional — workflow continues if summarization fails

### 3. Assertion Extraction (ExtractAssertions)
- **Input**: Content text, context (project, participants)
- **Output**: Structured assertions (risks, actions, issues, decisions, commitments, questions)
- **Used for**: RAID tracking, knowledge base enrichment
- **Primary model**: Local 7B SLM (extraction) with optional remote LLM (deep analysis)
- **Criticality**: Optional — workflow continues if extraction fails

### 4. Content Classification (ClassifyContent)
- **Input**: Content text, metadata
- **Output**: Content type, processing profile, classification confidence
- **Used for**: Determining processing pipeline
- **Primary model**: Local 7B SLM
- **Criticality**: Required for pipeline routing

### 5. RAG Query (Query)
- **Input**: Natural language question
- **Output**: Answer with source citations
- **Used for**: User Q&A over knowledge base
- **Flow**: Search → retrieve relevant content → generate answer with context

### 6. Analysis (AnalyzeByID)
- **Input**: Content ID
- **Output**: Summary, sentiment, entities, topics, action items
- **Used for**: Deep content analysis on demand
- **Current limitation**: Truncates content at 8000 characters

## Extraction Audit Trail

Every AI call is fully audited through the extraction_runs table:

```
extraction_runs:
  source_id          → Which content was processed
  template_id        → Which prompt template was used
  model_id           → Which model processed it
  context_injected   → What context was provided (JSONB)
  full_prompt        → Complete prompt text
  input_tokens       → Prompt token count
  output_tokens      → Response token count
  latency_ms         → Processing time
  raw_response       → Raw model output
  parsed_response    → Structured parsed result (JSONB)
  parse_errors       → Any parsing failures
  experiment_id      → A/B test variant (if applicable)
```

This enables:
- **Cost tracking** per content item, model, and task type
- **Quality analysis** by comparing extraction results across models
- **A/B testing** via the extraction_experiments system
- **Human feedback** via extraction_feedback (corrections, ratings)

## Prompt Templates

Prompt templates are managed in the database with a fallback chain:

1. **Project-specific template** — Custom prompts for specific projects
2. **Tenant default** — Organization-level customization
3. **System default** — Built-in prompts

Each template includes:
- template_text: The prompt with variable substitution
- extraction_schema: Expected output JSON schema
- context_config: What context to inject (people, glossary, project info)

## A/B Testing

The extraction_experiments system supports:
- Creating experiment variants (different models, prompts, or parameters)
- Routing a percentage of content to each variant
- Collecting per-extraction metrics
- Comparing results across variants

## Langfuse Integration

All LLM calls are traced through Langfuse for observability:
- Prompt/completion token counts
- Latency measurement
- Cost calculation
- Response quality tracking
- Conversation/chain tracing

## Current Limitations

1. **Single-call analysis**: The `AnalyzeByID` command truncates content at 8000 characters
2. **No chunking strategy**: Long content (transcripts, email threads) needs a map-reduce approach
3. **No context assembly**: The LLM doesn't receive entity context (known people, glossary, project info) before analysis
4. **No SLM/LLM split**: All AI operations use the same model rather than routing cheap tasks to SLMs and expensive tasks to remote LLMs
5. **No progressive availability**: Results aren't available until the entire pipeline completes
6. **No seniority/trust weighting**: All assertions are treated equally regardless of who said them. A VP flagging a risk surfaces the same as a junior mention. No mechanism for the human to mark trusted sources.
7. **No watch list or spotlight**: No way for the human to mark "I'm tracking these 3 risks" and have the system prioritise updates about them
8. **No bidirectional prompting**: The AI only responds to queries — it doesn't proactively surface items that need human attention (new risks from senior people, stale items, pattern changes)
9. **No human annotation**: No mechanism to capture offline context, gut feel, or verbal commitments that the AI can't observe from content

These limitations are what the SLM/LLM architecture guide (`guide.md`) proposes to address, grounded in the Human + AI Collaboration model described in `00-overview.md`.
