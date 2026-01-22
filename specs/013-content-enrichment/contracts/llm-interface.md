# LLM Interface Contract

**Version**: 1.0.0 | **Date**: 2026-01-21

## Overview

This document defines the contract between the mention resolution system and LLM providers (MLX local, Claude API). All providers must implement these interfaces to be used in the resolution pipeline.

## Provider Interface

```go
// LLMProvider defines the interface for LLM resolution providers.
type LLMProvider interface {
    // Name returns the provider identifier (e.g., "mlx-mistral-7b", "claude-sonnet")
    Name() string

    // Complete sends a completion request and returns the raw response.
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)

    // CompleteStructured sends a request expecting JSON output and parses it.
    CompleteStructured(ctx context.Context, req CompletionRequest, target any) error

    // IsAvailable checks if the provider is currently available.
    IsAvailable(ctx context.Context) bool

    // Close releases provider resources.
    Close() error
}
```

## Request/Response Types

### CompletionRequest

```go
type CompletionRequest struct {
    // Prompt is the full prompt text to send to the LLM.
    Prompt string `json:"prompt"`

    // SystemPrompt is an optional system-level instruction.
    SystemPrompt string `json:"system_prompt,omitempty"`

    // JSONMode enables structured JSON output.
    JSONMode bool `json:"json_mode"`

    // MaxTokens limits response length (0 = provider default).
    MaxTokens int `json:"max_tokens,omitempty"`

    // Temperature controls randomness (0.0-1.0, 0 = provider default).
    Temperature float32 `json:"temperature,omitempty"`

    // Metadata for tracing/logging.
    TraceID  string `json:"trace_id,omitempty"`
    StageNum int    `json:"stage_num,omitempty"`
}
```

### CompletionResponse

```go
type CompletionResponse struct {
    // Content is the raw text response from the LLM.
    Content string `json:"content"`

    // TokensUsed tracks token consumption.
    TokensUsed TokenUsage `json:"tokens_used"`

    // LatencyMs is the response time in milliseconds.
    LatencyMs int `json:"latency_ms"`

    // Model is the actual model used (may differ from requested).
    Model string `json:"model"`
}

type TokenUsage struct {
    Prompt     int `json:"prompt"`
    Completion int `json:"completion"`
    Total      int `json:"total"`
}
```

## Stage-Specific Schemas

### Stage 1: Understanding

**Input Context**:
```json
{
  "content": {
    "id": 4521,
    "type": "email",
    "subject": "Q4 Planning Meeting Notes",
    "body": "...",
    "date": "2026-01-21T10:30:00Z",
    "participants": ["alice@example.com", "bob@example.com"]
  },
  "project_context": {
    "id": 5,
    "name": "MTC",
    "members": ["Alice Smith", "Bob Jones"]
  }
}
```

**Expected Output**:
```json
{
  "mentions": [
    {
      "text": "Alan",
      "entity_type": "person",
      "position": 145,
      "context_snippet": "Alan will handle the LKE integration testing",
      "understanding": "A person who will handle LKE integration testing...",
      "transcription_flags": {
        "likely_error": false,
        "phonetic_variants": ["Allen", "Allan"]
      }
    }
  ]
}
```

**Schema Validation**:
- `mentions` is required array
- Each mention must have: `text`, `entity_type`, `understanding`
- `entity_type` must be: person, term, product, company, project
- `transcription_flags` is optional

### Stage 2: Cross-Mention Reasoning

**Input Context**:
```json
{
  "content_id": 4521,
  "mentions": [/* Stage 1 output */],
  "full_content": "..."
}
```

**Expected Output**:
```json
{
  "content_id": 4521,
  "unified_understanding": "This transcript discusses LKE integration for MTC project...",
  "mention_relationships": [
    {
      "from_mention": "Alan",
      "to_mention": "LKE",
      "relationship": "will_work_on",
      "inference": "If LKE resolves to product, person is likely LKE team member"
    }
  ],
  "resolution_hints": [
    "Look for person on LKE team with name similar to 'Alan'"
  ]
}
```

**Schema Validation**:
- `unified_understanding` is required string
- `mention_relationships` is optional array
- Each relationship must have: `from_mention`, `to_mention`, `relationship`

### Stage 3: Entity Matching

**Input Context**:
```json
{
  "understanding": {/* Stage 1 output */},
  "relationships": {/* Stage 2 output */},
  "candidates": {
    "Alan": [
      {
        "entity_id": 101,
        "entity_type": "person",
        "entity_name": "Allen Duet",
        "confidence_hints": {
          "fuzzy_match": 0.85,
          "project_member": true,
          "prior_links": 5
        }
      }
    ]
  }
}
```

**Expected Output**:
```json
{
  "resolutions": [
    {
      "mention_text": "Alan",
      "mention_position": 145,
      "decision": "resolve",
      "resolved_to": {
        "entity_type": "person",
        "entity_id": 101,
        "entity_name": "Allen Duet"
      },
      "confidence": 0.88,
      "reasoning": "Phonetic match + project membership + prior links...",
      "factors": {
        "phonetic_match": 0.85,
        "project_membership": true,
        "prior_links": 5
      },
      "alternatives_considered": [
        {
          "entity_id": 203,
          "entity_name": "Alan Evans",
          "confidence": 0.25,
          "rejection_reason": "Not on project"
        }
      ]
    }
  ],
  "new_entities_suggested": [
    {
      "mention_text": "DataDog",
      "suggested_type": "company",
      "suggested_name": "Datadog Inc",
      "reasoning": "Mentioned as monitoring tool, not in database",
      "confidence": 0.85
    }
  ]
}
```

**Schema Validation**:
- `resolutions` is required array
- Each resolution must have: `mention_text`, `decision`, `confidence`, `reasoning`
- `decision` must be: resolve, queue_review, suggest_new_entity
- `confidence` must be 0.00-1.00
- `resolved_to` required when decision = "resolve"
- `reasoning` is required (non-empty string)

### Stage 4: Verification

**Input Context**:
```json
{
  "resolution": {/* Single resolution from Stage 3 */},
  "full_content": "...",
  "challenge": "You resolved 'Alan' to Allen Duet. Is 'Al' the same person?"
}
```

**Expected Output**:
```json
{
  "mention_text": "Alan",
  "original_confidence": 0.88,
  "verification_result": "confirmed",
  "adjusted_confidence": 0.88,
  "verification_notes": "No contradictory evidence found..."
}
```

**Schema Validation**:
- `verification_result` must be: confirmed, adjusted, rejected
- `adjusted_confidence` must be 0.00-1.00
- `verification_notes` is required

## Error Handling

### Error Types

```go
type LLMError struct {
    Code    LLMErrorCode `json:"code"`
    Message string       `json:"message"`
    Details any          `json:"details,omitempty"`
}

type LLMErrorCode string

const (
    ErrTimeout        LLMErrorCode = "timeout"
    ErrUnavailable    LLMErrorCode = "unavailable"
    ErrRateLimit      LLMErrorCode = "rate_limit"
    ErrParseFailure   LLMErrorCode = "parse_failure"
    ErrInvalidSchema  LLMErrorCode = "invalid_schema"
    ErrContentTooLong LLMErrorCode = "content_too_long"
)
```

### Retry Policy

| Error Code | Retry | Max Attempts | Backoff |
|------------|-------|--------------|---------|
| timeout | Yes | 2 | Exponential (1s, 2s) |
| unavailable | Yes | 2 | Exponential (2s, 4s) |
| rate_limit | Yes | 3 | Fixed (5s) |
| parse_failure | Yes (with reprompt) | 2 | None |
| invalid_schema | Yes (with reprompt) | 2 | None |
| content_too_long | No | - | Split content |

### Fallback Strategy

1. Primary: Local MLX provider
2. On persistent failure after retries: Queue for human review
3. Future: Optional Claude API escalation for low-confidence cases

## Configuration

```go
type LLMConfig struct {
    // Provider selection
    Provider string `json:"provider"` // "mlx", "claude", "openai"
    Model    string `json:"model"`    // "mistral-7b", "claude-3-sonnet"

    // Connection
    BaseURL string        `json:"base_url"` // "http://localhost:8081"
    Timeout time.Duration `json:"timeout"`  // 30s default

    // Performance
    MaxRetries        int `json:"max_retries"`         // 2 default
    TimeoutPerContent time.Duration `json:"timeout_per_content"` // 30s default

    // Escalation (future)
    EscalateOnLowConfidence bool    `json:"escalate_on_low_confidence"`
    EscalationThreshold     float64 `json:"escalation_threshold"` // 0.7
    EscalationProvider      string  `json:"escalation_provider"`  // "claude"
}
```

## Prompt Templates

Templates are stored in `pkg/mentions/resolver/prompts/` and versioned with the code.

| Template | Stage | Purpose |
|----------|-------|---------|
| `understanding.tmpl` | 1 | Extract and understand mentions |
| `cross_mention.tmpl` | 2 | Reason across mentions |
| `matching.tmpl` | 3 | Match to candidates |
| `verification.tmpl` | 4 | Verify uncertain resolutions |

Template format: Go text/template with JSON schema hints embedded.

## Testing Contract

Providers must pass these contract tests:

1. **Availability Check**: `IsAvailable()` returns true when service is up
2. **Basic Completion**: `Complete()` returns valid response for simple prompt
3. **JSON Mode**: `CompleteStructured()` returns parseable JSON
4. **Timeout Handling**: Request times out correctly, returns ErrTimeout
5. **Retry Logic**: Transient errors retry correctly
6. **Schema Validation**: Invalid schemas rejected with ErrInvalidSchema
