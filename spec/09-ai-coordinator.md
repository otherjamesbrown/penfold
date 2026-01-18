# AI Coordinator Specification

## Overview

The AI Coordinator manages model selection, orchestrates local and cloud AI processing, handles confidence-based escalation, and tracks costs.

## Status: Planned (Phase 4)

## Responsibilities

1. **Model Selection**: Choose optimal model for task
2. **Escalation**: Route low-confidence results to cloud
3. **Cost Tracking**: Monitor API spending
4. **Performance Monitoring**: Track model performance
5. **Ensemble Combining**: Merge multi-model results

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     AI Coordinator                           │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │    Model     │    │  Escalation  │    │    Cost      │  │
│  │   Selector   │    │   Manager    │    │   Tracker    │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│         │                   │                   │           │
│         └───────────────────┼───────────────────┘           │
│                             ▼                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │               Model Router                           │   │
│  └─────────────────────────────────────────────────────┘   │
│              │                           │                   │
│              ▼                           ▼                   │
│    ┌─────────────────┐         ┌─────────────────┐         │
│    │   vLLM-MLX      │         │   Gemini API    │         │
│    │   (local)       │         │   (cloud)       │         │
│    │   Qwen2.5-14B   │         │                 │         │
│    └─────────────────┘         └─────────────────┘         │
└─────────────────────────────────────────────────────────────┘
```

## gRPC Service

```protobuf
service AICoordinatorService {
  // Processing
  rpc Process(ProcessRequest) returns (ProcessResponse);
  rpc ProcessBatch(ProcessBatchRequest) returns (ProcessBatchResponse);

  // Model management
  rpc GetModelStatus(GetModelStatusRequest) returns (GetModelStatusResponse);
  rpc ListModels(ListModelsRequest) returns (ListModelsResponse);

  // Cost tracking
  rpc GetCostSummary(GetCostSummaryRequest) returns (GetCostSummaryResponse);
  rpc SetBudget(SetBudgetRequest) returns (SetBudgetResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

## Model Selection Strategy

```go
func (c *Coordinator) SelectModel(task Task, content string) Model {
    // Task complexity assessment
    complexity := c.assessComplexity(task, content)

    // Content size considerations
    tokenCount := c.estimateTokens(content)

    // Cost constraints
    budget := c.getRemainingBudget()

    // Decision logic
    if complexity < 0.3 && tokenCount < 4000 {
        return c.localModel  // Qwen2.5-14B
    }
    if budget > 0 && complexity > 0.7 {
        return c.cloudModel  // Gemini
    }
    return c.localModel  // Default to local
}
```

## Escalation Rules

- Confidence < 0.8: Escalate to cloud
- Entity count > 10: Use cloud for accuracy
- Content type "executive": Always validate with cloud
- Daily budget exhausted: Local only

## Cost Tracking

- Per-request cost calculation
- Daily/weekly/monthly budgets
- Cost attribution by tenant, task type
- Alerts on budget threshold
