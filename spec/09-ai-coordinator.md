# AI Coordinator Specification

## Overview

The AI Coordinator manages model selection, orchestrates local and cloud AI processing, handles confidence-based escalation, tracks costs, and provides a unified interface for all AI operations across Penfold services.

## Status: Planned (Phase 4)

## Responsibilities

1. **Model Selection**: Choose optimal model for task based on complexity, cost, and requirements
2. **Escalation**: Route low-confidence results to cloud models for verification
3. **Cost Tracking**: Monitor API spending with budgets and alerts
4. **Performance Monitoring**: Track model latency, accuracy, and throughput
5. **Ensemble Combining**: Merge multi-model results for improved accuracy
6. **Request Routing**: Load balance across model instances
7. **Caching**: Cache repeated prompts for efficiency

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          AI Coordinator                                   │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                      gRPC Server (:8085)                        │    │
│  └──────────────────────────┬─────────────────────────────────────┘    │
│                             │                                           │
│  ┌──────────────────────────┼──────────────────────────────────────┐   │
│  │                          ▼                                       │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │    Model     │  │  Escalation  │  │    Cost      │          │   │
│  │  │   Selector   │  │   Manager    │  │   Tracker    │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                 │                 │                   │   │
│  │         └─────────────────┼─────────────────┘                   │   │
│  │                           ▼                                      │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │                   Model Router                           │   │   │
│  │  │         (load balancing, retry, circuit breaker)        │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  │              │                           │                      │   │
│  │              ▼                           ▼                       │   │
│  │    ┌─────────────────┐         ┌─────────────────┐             │   │
│  │    │   Local Models  │         │   Cloud Models  │             │   │
│  │    │   vLLM-MLX      │         │   Gemini API    │             │   │
│  │    │   Qwen2.5-14B   │         │   Claude API    │             │   │
│  │    │   (:8000)       │         │                 │             │   │
│  │    └─────────────────┘         └─────────────────┘             │   │
│  └──────────────────────────┬──────────────────────────────────────┘   │
│                             │                                           │
│         ┌───────────────────┼───────────────────┐                      │
│         ▼                   ▼                   ▼                       │
│  ┌───────────┐       ┌───────────┐       ┌───────────┐                │
│  │PostgreSQL │       │   Redis   │       │Prometheus │                │
│  │  (costs,  │       │  (cache,  │       │ (metrics) │                │
│  │  budgets) │       │   queue)  │       │           │                │
│  └───────────┘       └───────────┘       └───────────┘                │
└─────────────────────────────────────────────────────────────────────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/ai/v1/ai.proto

syntax = "proto3";
package ai.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";

service AICoordinatorService {
  // Processing
  rpc Process(ProcessRequest) returns (ProcessResponse);
  rpc ProcessStream(ProcessRequest) returns (stream ProcessStreamResponse);
  rpc ProcessBatch(ProcessBatchRequest) returns (ProcessBatchResponse);

  // Model management
  rpc GetModelStatus(GetModelStatusRequest) returns (GetModelStatusResponse);
  rpc ListModels(ListModelsRequest) returns (ListModelsResponse);
  rpc SetModelPreference(SetModelPreferenceRequest) returns (SetModelPreferenceResponse);
  rpc RegisterModel(RegisterModelRequest) returns (RegisterModelResponse);
  rpc UnregisterModel(UnregisterModelRequest) returns (UnregisterModelResponse);
  rpc UpdateModelConfig(UpdateModelConfigRequest) returns (UpdateModelConfigResponse);

  // Ensemble processing
  rpc ProcessEnsemble(ProcessEnsembleRequest) returns (ProcessEnsembleResponse);

  // Task routing
  rpc SetTaskRouting(SetTaskRoutingRequest) returns (SetTaskRoutingResponse);
  rpc GetTaskRouting(GetTaskRoutingRequest) returns (TaskRoutingConfig);

  // Cost tracking
  rpc GetCostSummary(GetCostSummaryRequest) returns (GetCostSummaryResponse);
  rpc SetBudget(SetBudgetRequest) returns (SetBudgetResponse);
  rpc GetBudget(GetBudgetRequest) returns (Budget);

  // Performance
  rpc GetPerformanceStats(GetPerformanceStatsRequest) returns (PerformanceStats);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

// Processing messages
message ProcessRequest {
  string tenant_id = 1;
  Task task = 2;
  string content = 3;
  string prompt = 4;  // Optional custom prompt
  ProcessOptions options = 5;
  map<string, string> metadata = 6;
}

enum Task {
  TASK_UNSPECIFIED = 0;
  TASK_ENTITY_EXTRACTION = 1;
  TASK_CATEGORIZATION = 2;
  TASK_SUMMARIZATION = 3;
  TASK_QUESTION_ANSWERING = 4;
  TASK_RELATIONSHIP_EXTRACTION = 5;
  TASK_SENTIMENT_ANALYSIS = 6;
  TASK_TRANSLATION = 7;
  TASK_EMBEDDING = 8;
  TASK_GENERAL = 9;
}

message ProcessOptions {
  ModelPreference model_preference = 1;
  float min_confidence = 2;
  bool allow_cloud_escalation = 3;
  int32 max_tokens = 4;
  float temperature = 5;
  string output_format = 6;  // json, text
  google.protobuf.Duration timeout = 7;
  bool use_cache = 8;
  string summary_length = 9;
}

enum ModelPreference {
  MODEL_PREFERENCE_UNSPECIFIED = 0;
  MODEL_PREFERENCE_LOCAL_ONLY = 1;
  MODEL_PREFERENCE_CLOUD_ONLY = 2;
  MODEL_PREFERENCE_LOCAL_PREFERRED = 3;
  MODEL_PREFERENCE_BEST_QUALITY = 4;
  MODEL_PREFERENCE_LOWEST_COST = 5;
}

message ProcessResponse {
  string request_id = 1;
  bool success = 2;
  string result = 3;
  float confidence = 4;
  ProcessingMetadata metadata = 5;
  string error = 6;
}

message ProcessingMetadata {
  string model_used = 1;
  ModelLocation model_location = 2;
  int64 processing_time_ms = 3;
  int32 input_tokens = 4;
  int32 output_tokens = 5;
  float estimated_cost = 6;
  bool from_cache = 7;
  bool escalated = 8;
  string escalation_reason = 9;
}

enum ModelLocation {
  MODEL_LOCATION_UNSPECIFIED = 0;
  MODEL_LOCATION_LOCAL = 1;
  MODEL_LOCATION_CLOUD = 2;
}

message ProcessStreamResponse {
  string chunk = 1;
  bool done = 2;
  ProcessingMetadata metadata = 3;
}

// Batch processing
message ProcessBatchRequest {
  string tenant_id = 1;
  repeated BatchItem items = 2;
  ProcessOptions options = 3;
}

message BatchItem {
  string id = 1;
  Task task = 2;
  string content = 3;
  string prompt = 4;
}

message ProcessBatchResponse {
  repeated BatchResult results = 1;
  BatchStats stats = 2;
}

message BatchResult {
  string id = 1;
  bool success = 2;
  string result = 3;
  float confidence = 4;
  string error = 5;
}

message BatchStats {
  int32 total = 1;
  int32 successful = 2;
  int32 failed = 3;
  int64 total_time_ms = 4;
  float total_cost = 5;
}

// Model management
message GetModelStatusRequest {
  string model_id = 1;
}

message GetModelStatusResponse {
  Model model = 1;
}

message Model {
  string id = 1;
  string name = 2;
  ModelLocation location = 3;
  ModelStatus status = 4;
  ModelCapabilities capabilities = 5;
  ModelPricing pricing = 6;
  ModelPerformance performance = 7;
}

enum ModelStatus {
  MODEL_STATUS_UNSPECIFIED = 0;
  MODEL_STATUS_AVAILABLE = 1;
  MODEL_STATUS_BUSY = 2;
  MODEL_STATUS_UNAVAILABLE = 3;
  MODEL_STATUS_RATE_LIMITED = 4;
}

message ModelCapabilities {
  repeated Task supported_tasks = 1;
  int32 max_context_length = 2;
  int32 max_output_tokens = 3;
  bool supports_streaming = 4;
  bool supports_json_mode = 5;
  repeated string languages = 6;
}

message ModelPricing {
  float input_cost_per_1k = 1;   // Cost per 1K input tokens
  float output_cost_per_1k = 2;  // Cost per 1K output tokens
  string currency = 3;
}

message ModelPerformance {
  float avg_latency_ms = 1;
  float p95_latency_ms = 2;
  float p99_latency_ms = 3;
  float tokens_per_second = 4;
  float error_rate = 5;
}

message ListModelsRequest {
  ModelLocation location = 1;
  Task task = 2;
}

message ListModelsResponse {
  repeated Model models = 1;
}

message SetModelPreferenceRequest {
  string tenant_id = 1;
  Task task = 2;
  ModelPreference preference = 3;
  string preferred_model_id = 4;
}

message SetModelPreferenceResponse {
  bool success = 1;
}

// Model registration messages
message RegisterModelRequest {
  string id = 1;
  string name = 2;
  ModelLocation location = 3;
  string endpoint = 4;           // For local models
  string api_key = 5;            // For cloud models
  ModelCapabilities capabilities = 6;
  ModelPricing pricing = 7;
  map<string, string> config = 8;  // Additional configuration
}

message RegisterModelResponse {
  bool success = 1;
  string model_id = 2;
  string error = 3;
}

message UnregisterModelRequest {
  string model_id = 1;
  bool force = 2;  // Force unregister even if in use
}

message UnregisterModelResponse {
  bool success = 1;
  int32 active_requests_cancelled = 2;
  string error = 3;
}

message UpdateModelConfigRequest {
  string model_id = 1;
  ModelCapabilities capabilities = 2;
  ModelPricing pricing = 3;
  map<string, string> config = 4;
}

message UpdateModelConfigResponse {
  bool success = 1;
  Model updated_model = 2;
}

// Ensemble processing messages
message ProcessEnsembleRequest {
  string tenant_id = 1;
  Task task = 2;
  string content = 3;
  EnsembleConfig config = 4;
  ProcessOptions options = 5;
}

message EnsembleConfig {
  repeated string model_ids = 1;
  CombinationStrategy strategy = 2;
  map<string, float> model_weights = 3;  // For weighted_average
  float min_agreement = 4;               // For voting strategies
  int32 min_models = 5;                  // Minimum models that must respond
}

enum CombinationStrategy {
  COMBINATION_STRATEGY_UNSPECIFIED = 0;
  COMBINATION_STRATEGY_WEIGHTED_AVERAGE = 1;   // Weight results by model confidence or explicit weights
  COMBINATION_STRATEGY_CONFIDENCE_VOTING = 2;   // Use highest confidence result
  COMBINATION_STRATEGY_MAJORITY_VOTE = 3;       // Use result that majority agrees on
  COMBINATION_STRATEGY_UNANIMOUS = 4;           // Require all models to agree
  COMBINATION_STRATEGY_CASCADE = 5;             // Use first successful result meeting threshold
}

message ProcessEnsembleResponse {
  string request_id = 1;
  bool success = 2;
  string result = 3;
  float confidence = 4;
  EnsembleMetadata ensemble_metadata = 5;
  repeated ModelResult individual_results = 6;
  string error = 7;
}

message EnsembleMetadata {
  CombinationStrategy strategy_used = 1;
  int32 models_queried = 2;
  int32 models_responded = 3;
  float agreement_rate = 4;
  string winning_model = 5;       // For voting strategies
  int64 total_processing_time_ms = 6;
  float total_cost = 7;
}

message ModelResult {
  string model_id = 1;
  string result = 2;
  float confidence = 3;
  int64 latency_ms = 4;
  float cost = 5;
  string error = 6;
  float weight = 7;  // Effective weight used in combination
}

// Task routing messages
message SetTaskRoutingRequest {
  string tenant_id = 1;
  TaskRoutingConfig config = 2;
}

message SetTaskRoutingResponse {
  bool success = 1;
}

message GetTaskRoutingRequest {
  string tenant_id = 1;
}

message TaskRoutingConfig {
  repeated TaskRoute routes = 1;
  string default_model = 2;
  FallbackConfig fallback = 3;
}

message TaskRoute {
  Task task = 1;
  string primary_model = 2;
  repeated string fallback_models = 3;
  float confidence_threshold = 4;       // Below this, use escalation
  bool enable_ensemble = 5;             // Use ensemble for this task
  EnsembleConfig ensemble_config = 6;
  map<string, string> task_specific_params = 7;
}

message FallbackConfig {
  bool enabled = 1;
  int32 max_retries = 2;
  repeated string fallback_chain = 3;  // Ordered list of fallback models
}

// Cost tracking
message GetCostSummaryRequest {
  string tenant_id = 1;
  google.protobuf.Timestamp start_time = 2;
  google.protobuf.Timestamp end_time = 3;
  CostGrouping grouping = 4;
}

enum CostGrouping {
  COST_GROUPING_UNSPECIFIED = 0;
  COST_GROUPING_HOURLY = 1;
  COST_GROUPING_DAILY = 2;
  COST_GROUPING_WEEKLY = 3;
  COST_GROUPING_MONTHLY = 4;
}

message GetCostSummaryResponse {
  float total_cost = 1;
  int64 total_requests = 2;
  int64 total_input_tokens = 3;
  int64 total_output_tokens = 4;
  repeated CostBreakdown by_model = 5;
  repeated CostBreakdown by_task = 6;
  repeated TimePeriodCost by_period = 7;
}

message CostBreakdown {
  string name = 1;
  float cost = 2;
  int64 requests = 3;
  int64 tokens = 4;
  float percentage = 5;
}

message TimePeriodCost {
  google.protobuf.Timestamp period_start = 1;
  float cost = 2;
  int64 requests = 3;
}

message SetBudgetRequest {
  string tenant_id = 1;
  Budget budget = 2;
}

message SetBudgetResponse {
  bool success = 1;
}

message GetBudgetRequest {
  string tenant_id = 1;
}

message Budget {
  string tenant_id = 1;
  float daily_limit = 2;
  float weekly_limit = 3;
  float monthly_limit = 4;
  float current_daily_spend = 5;
  float current_weekly_spend = 6;
  float current_monthly_spend = 7;
  bool alerts_enabled = 8;
  repeated float alert_thresholds = 9;  // e.g., [0.5, 0.8, 1.0]
  BudgetAction action_on_limit = 10;
}

enum BudgetAction {
  BUDGET_ACTION_UNSPECIFIED = 0;
  BUDGET_ACTION_WARN = 1;
  BUDGET_ACTION_LOCAL_ONLY = 2;
  BUDGET_ACTION_BLOCK = 3;
}

// Performance stats
message GetPerformanceStatsRequest {
  string tenant_id = 1;
  google.protobuf.Timestamp start_time = 2;
  google.protobuf.Timestamp end_time = 3;
}

message PerformanceStats {
  int64 total_requests = 1;
  float avg_latency_ms = 2;
  float p50_latency_ms = 3;
  float p95_latency_ms = 4;
  float p99_latency_ms = 5;
  float error_rate = 6;
  float cache_hit_rate = 7;
  float escalation_rate = 8;
  float local_model_usage = 9;
  float cloud_model_usage = 10;
  repeated TaskStats by_task = 11;
}

message TaskStats {
  Task task = 1;
  int64 request_count = 2;
  float avg_latency_ms = 3;
  float avg_confidence = 4;
  float error_rate = 5;
}

// Health
message HealthRequest {}
message HealthResponse {
  bool healthy = 1;
  map<string, ComponentHealth> components = 2;
}

message ComponentHealth {
  bool healthy = 1;
  string status = 2;
  int64 latency_ms = 3;
}
```

## Model Selection

```go
// internal/selector/selector.go

package selector

import (
    "context"
    "fmt"
    "log/slog"
)

type ModelSelector struct {
    models    map[string]*Model
    router    *ModelRouter
    costTracker *CostTracker
    metrics   *MetricsCollector
}

type SelectionContext struct {
    TenantID   string
    Task       Task
    Content    string
    Options    *ProcessOptions
    Budget     *Budget
}

type SelectionResult struct {
    Model           *Model
    Reason          string
    EstimatedCost   float64
    FallbackModels  []*Model
}

func (s *ModelSelector) SelectModel(ctx context.Context, sc *SelectionContext) (*SelectionResult, error) {
    // Get available models for task
    availableModels := s.getModelsForTask(sc.Task)
    if len(availableModels) == 0 {
        return nil, fmt.Errorf("no models available for task: %s", sc.Task)
    }

    // Filter by preference
    filteredModels := s.filterByPreference(availableModels, sc.Options.ModelPreference)

    // Check budget constraints
    if sc.Budget != nil {
        filteredModels = s.filterByBudget(filteredModels, sc)
    }

    // Score and rank models
    scoredModels := s.scoreModels(filteredModels, sc)

    if len(scoredModels) == 0 {
        return nil, fmt.Errorf("no suitable models after filtering")
    }

    // Select best model
    best := scoredModels[0]

    result := &SelectionResult{
        Model:          best.Model,
        Reason:         best.Reason,
        EstimatedCost:  best.EstimatedCost,
        FallbackModels: extractModels(scoredModels[1:]),
    }

    slog.Debug("model selected",
        "model", best.Model.ID,
        "reason", best.Reason,
        "estimated_cost", best.EstimatedCost,
    )

    return result, nil
}

type ScoredModel struct {
    Model         *Model
    Score         float64
    Reason        string
    EstimatedCost float64
}

func (s *ModelSelector) scoreModels(models []*Model, sc *SelectionContext) []ScoredModel {
    var scored []ScoredModel

    for _, model := range models {
        score, reason := s.calculateScore(model, sc)
        estimatedCost := s.estimateCost(model, sc.Content)

        scored = append(scored, ScoredModel{
            Model:         model,
            Score:         score,
            Reason:        reason,
            EstimatedCost: estimatedCost,
        })
    }

    // Sort by score descending
    sort.Slice(scored, func(i, j int) bool {
        return scored[i].Score > scored[j].Score
    })

    return scored
}

func (s *ModelSelector) calculateScore(model *Model, sc *SelectionContext) (float64, string) {
    var score float64
    var reasons []string

    // Base score
    score = 50.0

    // Task-specific scoring
    if containsTask(model.Capabilities.SupportedTasks, sc.Task) {
        score += 20.0
        reasons = append(reasons, "supports task")
    }

    // Preference scoring
    switch sc.Options.ModelPreference {
    case ModelPreferenceLocalOnly, ModelPreferenceLocalPreferred:
        if model.Location == ModelLocationLocal {
            score += 30.0
            reasons = append(reasons, "local preferred")
        }
    case ModelPreferenceCloudOnly:
        if model.Location == ModelLocationCloud {
            score += 30.0
            reasons = append(reasons, "cloud required")
        }
    case ModelPreferenceBestQuality:
        // Prefer cloud for quality
        if model.Location == ModelLocationCloud {
            score += 20.0
        }
        // Add performance bonus
        if model.Performance.ErrorRate < 0.01 {
            score += 10.0
            reasons = append(reasons, "low error rate")
        }
    case ModelPreferenceLowestCost:
        // Prefer local for cost
        if model.Location == ModelLocationLocal {
            score += 30.0
            reasons = append(reasons, "lowest cost")
        }
    }

    // Availability scoring
    switch model.Status {
    case ModelStatusAvailable:
        score += 10.0
    case ModelStatusBusy:
        score -= 10.0
        reasons = append(reasons, "busy")
    case ModelStatusRateLimited:
        score -= 30.0
        reasons = append(reasons, "rate limited")
    }

    // Latency scoring
    if model.Performance.AvgLatencyMs < 500 {
        score += 10.0
        reasons = append(reasons, "low latency")
    }

    // Context length scoring
    contentTokens := estimateTokens(sc.Content)
    if contentTokens < model.Capabilities.MaxContextLength/2 {
        score += 5.0
    } else if contentTokens > model.Capabilities.MaxContextLength*0.9 {
        score -= 20.0
        reasons = append(reasons, "near context limit")
    }

    return score, strings.Join(reasons, ", ")
}

func (s *ModelSelector) filterByPreference(models []*Model, pref ModelPreference) []*Model {
    var filtered []*Model

    for _, model := range models {
        switch pref {
        case ModelPreferenceLocalOnly:
            if model.Location == ModelLocationLocal {
                filtered = append(filtered, model)
            }
        case ModelPreferenceCloudOnly:
            if model.Location == ModelLocationCloud {
                filtered = append(filtered, model)
            }
        default:
            filtered = append(filtered, model)
        }
    }

    return filtered
}

func (s *ModelSelector) filterByBudget(models []*Model, sc *SelectionContext) []*Model {
    // Check if budget is exhausted
    if sc.Budget.CurrentDailySpend >= sc.Budget.DailyLimit {
        // Only allow local models
        var filtered []*Model
        for _, model := range models {
            if model.Location == ModelLocationLocal {
                filtered = append(filtered, model)
            }
        }
        return filtered
    }

    return models
}

func (s *ModelSelector) estimateCost(model *Model, content string) float64 {
    if model.Location == ModelLocationLocal {
        return 0.0  // Local models have no API cost
    }

    inputTokens := estimateTokens(content)
    // Estimate output as 20% of input for most tasks
    outputTokens := inputTokens / 5

    inputCost := float64(inputTokens) / 1000 * float64(model.Pricing.InputCostPer1k)
    outputCost := float64(outputTokens) / 1000 * float64(model.Pricing.OutputCostPer1k)

    return inputCost + outputCost
}

func estimateTokens(text string) int {
    // Rough estimate: ~4 characters per token
    return len(text) / 4
}
```

## Escalation Manager

```go
// internal/escalation/manager.go

package escalation

import (
    "context"
    "log/slog"
)

type EscalationManager struct {
    cloudClient  *CloudModelClient
    costTracker  *CostTracker
    metrics      *MetricsCollector
}

type EscalationConfig struct {
    ConfidenceThreshold float64
    MaxEntityCount      int
    AlwaysEscalate      []string  // Content types that always escalate
    NeverEscalate       []string  // Content types that never escalate
}

var DefaultConfig = &EscalationConfig{
    ConfidenceThreshold: 0.8,
    MaxEntityCount:      10,
    AlwaysEscalate:      []string{"executive", "legal", "financial"},
    NeverEscalate:       []string{"spam", "promotional"},
}

type EscalationDecision struct {
    ShouldEscalate bool
    Reason         string
    TargetModel    string
}

func (m *EscalationManager) ShouldEscalate(ctx context.Context, result *ProcessResult, opts *ProcessOptions) *EscalationDecision {
    // Check if escalation is allowed
    if !opts.AllowCloudEscalation {
        return &EscalationDecision{
            ShouldEscalate: false,
            Reason:         "cloud escalation disabled",
        }
    }

    // Check always/never escalate lists
    for _, t := range DefaultConfig.AlwaysEscalate {
        if result.ContentType == t {
            return &EscalationDecision{
                ShouldEscalate: true,
                Reason:         fmt.Sprintf("content type '%s' always requires verification", t),
                TargetModel:    "gemini-pro",
            }
        }
    }

    for _, t := range DefaultConfig.NeverEscalate {
        if result.ContentType == t {
            return &EscalationDecision{
                ShouldEscalate: false,
                Reason:         fmt.Sprintf("content type '%s' does not require escalation", t),
            }
        }
    }

    // Check confidence threshold
    if result.Confidence < DefaultConfig.ConfidenceThreshold {
        return &EscalationDecision{
            ShouldEscalate: true,
            Reason:         fmt.Sprintf("confidence %.2f below threshold %.2f", result.Confidence, DefaultConfig.ConfidenceThreshold),
            TargetModel:    "gemini-pro",
        }
    }

    // Check entity count (complex content may need verification)
    if result.EntityCount > DefaultConfig.MaxEntityCount {
        return &EscalationDecision{
            ShouldEscalate: true,
            Reason:         fmt.Sprintf("entity count %d exceeds threshold %d", result.EntityCount, DefaultConfig.MaxEntityCount),
            TargetModel:    "gemini-pro",
        }
    }

    return &EscalationDecision{
        ShouldEscalate: false,
        Reason:         "no escalation needed",
    }
}

func (m *EscalationManager) Escalate(ctx context.Context, req *ProcessRequest, localResult *ProcessResult) (*ProcessResult, error) {
    slog.Info("escalating to cloud model",
        "task", req.Task,
        "local_confidence", localResult.Confidence,
        "reason", localResult.EscalationReason,
    )

    // Build verification prompt
    prompt := m.buildVerificationPrompt(req, localResult)

    // Call cloud model
    cloudResult, err := m.cloudClient.Process(ctx, &CloudProcessRequest{
        Model:   "gemini-pro",
        Prompt:  prompt,
        Content: req.Content,
        Task:    req.Task,
    })
    if err != nil {
        return nil, fmt.Errorf("cloud escalation failed: %w", err)
    }

    // Merge results
    merged := m.mergeResults(localResult, cloudResult)

    m.metrics.RecordEscalation(req.TenantId, req.Task, cloudResult.Confidence)

    return merged, nil
}

func (m *EscalationManager) buildVerificationPrompt(req *ProcessRequest, localResult *ProcessResult) string {
    switch req.Task {
    case TaskEntityExtraction:
        return fmt.Sprintf(`Verify and correct the following entity extraction results.

Original content:
%s

Extracted entities:
%s

Please verify each entity and provide corrections if needed. Return the verified entities in JSON format.`, req.Content, localResult.Result)

    case TaskCategorization:
        return fmt.Sprintf(`Verify the following categorization.

Content:
%s

Assigned category: %s
Confidence: %.2f

Is this categorization correct? If not, provide the correct category.`, req.Content, localResult.Result, localResult.Confidence)

    default:
        return fmt.Sprintf(`Verify and improve the following AI output.

Task: %s
Input: %s
Output: %s

Please verify accuracy and provide improvements if needed.`, req.Task, req.Content, localResult.Result)
    }
}

func (m *EscalationManager) mergeResults(local, cloud *ProcessResult) *ProcessResult {
    // If cloud disagrees significantly, use cloud result
    if cloud.Confidence > local.Confidence+0.2 {
        cloud.MergeSource = "cloud_preferred"
        return cloud
    }

    // If both agree, boost confidence
    if resultsAgree(local, cloud) {
        merged := &ProcessResult{
            Result:      local.Result,
            Confidence:  min(1.0, (local.Confidence+cloud.Confidence)/2+0.1),
            MergeSource: "consensus",
        }
        return merged
    }

    // Default to cloud for quality
    cloud.MergeSource = "cloud_fallback"
    return cloud
}
```

## Model Registry

```go
// internal/registry/registry.go

package registry

import (
    "context"
    "fmt"
    "sync"
    "time"
)

type ModelRegistry struct {
    db        *pgxpool.Pool
    models    map[string]*RegisteredModel
    mu        sync.RWMutex
    router    *ModelRouter
}

type RegisteredModel struct {
    ID           string
    Name         string
    Location     ModelLocation
    Endpoint     string
    APIKey       string
    Capabilities *ModelCapabilities
    Pricing      *ModelPricing
    Config       map[string]string
    Status       ModelStatus
    RegisteredAt time.Time
    LastHealthCheck time.Time
    Client       ModelClient
}

func NewModelRegistry(db *pgxpool.Pool, router *ModelRouter) *ModelRegistry {
    r := &ModelRegistry{
        db:     db,
        models: make(map[string]*RegisteredModel),
        router: router,
    }

    // Start health check loop
    go r.healthCheckLoop()

    return r
}

func (r *ModelRegistry) Register(ctx context.Context, req *RegisterModelRequest) (*RegisterModelResponse, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Check if model already exists
    if _, exists := r.models[req.Id]; exists {
        return &RegisterModelResponse{
            Success: false,
            Error:   fmt.Sprintf("model %s already registered", req.Id),
        }, nil
    }

    // Create client based on location
    var client ModelClient
    var err error

    switch req.Location {
    case ModelLocationLocal:
        client, err = NewVLLMClient(req.Endpoint, req.Config)
    case ModelLocationCloud:
        client, err = NewCloudClient(req.Id, req.ApiKey, req.Config)
    default:
        return nil, fmt.Errorf("unknown model location: %v", req.Location)
    }

    if err != nil {
        return &RegisterModelResponse{
            Success: false,
            Error:   fmt.Sprintf("failed to create client: %v", err),
        }, nil
    }

    // Verify connectivity
    status := client.GetStatus()
    if status == ModelStatusUnavailable {
        return &RegisterModelResponse{
            Success: false,
            Error:   "model endpoint is not reachable",
        }, nil
    }

    model := &RegisteredModel{
        ID:           req.Id,
        Name:         req.Name,
        Location:     req.Location,
        Endpoint:     req.Endpoint,
        APIKey:       req.ApiKey,
        Capabilities: req.Capabilities,
        Pricing:      req.Pricing,
        Config:       req.Config,
        Status:       status,
        RegisteredAt: time.Now(),
        Client:       client,
    }

    r.models[req.Id] = model

    // Register with router
    r.router.RegisterModel(req.Id, client)

    // Persist to database
    if err := r.persistModel(ctx, model); err != nil {
        slog.Warn("failed to persist model registration", "model", req.Id, "error", err)
    }

    slog.Info("model registered", "id", req.Id, "name", req.Name, "location", req.Location)

    return &RegisterModelResponse{
        Success: true,
        ModelId: req.Id,
    }, nil
}

func (r *ModelRegistry) Unregister(ctx context.Context, req *UnregisterModelRequest) (*UnregisterModelResponse, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    model, exists := r.models[req.ModelId]
    if !exists {
        return &UnregisterModelResponse{
            Success: false,
            Error:   fmt.Sprintf("model %s not found", req.ModelId),
        }, nil
    }

    // Check for active requests
    load := r.router.GetModelLoad(req.ModelId)
    if load != nil && load.ActiveRequests > 0 && !req.Force {
        return &UnregisterModelResponse{
            Success: false,
            Error:   fmt.Sprintf("model has %d active requests; use force=true to override", load.ActiveRequests),
            ActiveRequestsCancelled: int32(load.ActiveRequests),
        }, nil
    }

    // Remove from router and registry
    r.router.UnregisterModel(req.ModelId)
    delete(r.models, req.ModelId)

    // Remove from database
    r.removeModelFromDB(ctx, req.ModelId)

    slog.Info("model unregistered", "id", req.ModelId)

    return &UnregisterModelResponse{
        Success:                 true,
        ActiveRequestsCancelled: int32(load.ActiveRequests),
    }, nil
}

func (r *ModelRegistry) UpdateConfig(ctx context.Context, req *UpdateModelConfigRequest) (*UpdateModelConfigResponse, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    model, exists := r.models[req.ModelId]
    if !exists {
        return nil, fmt.Errorf("model %s not found", req.ModelId)
    }

    if req.Capabilities != nil {
        model.Capabilities = req.Capabilities
    }
    if req.Pricing != nil {
        model.Pricing = req.Pricing
    }
    for k, v := range req.Config {
        model.Config[k] = v
    }

    // Persist changes
    r.persistModel(ctx, model)

    return &UpdateModelConfigResponse{
        Success:      true,
        UpdatedModel: r.toProto(model),
    }, nil
}

func (r *ModelRegistry) GetModel(id string) *RegisteredModel {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.models[id]
}

func (r *ModelRegistry) ListModels(location ModelLocation, task Task) []*RegisteredModel {
    r.mu.RLock()
    defer r.mu.RUnlock()

    var result []*RegisteredModel
    for _, model := range r.models {
        if location != ModelLocationUnspecified && model.Location != location {
            continue
        }
        if task != TaskUnspecified && !containsTask(model.Capabilities.SupportedTasks, task) {
            continue
        }
        result = append(result, model)
    }
    return result
}

func (r *ModelRegistry) healthCheckLoop() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        r.checkAllModels()
    }
}

func (r *ModelRegistry) checkAllModels() {
    r.mu.Lock()
    defer r.mu.Unlock()

    for id, model := range r.models {
        status := model.Client.GetStatus()
        if status != model.Status {
            slog.Info("model status changed", "id", id, "old", model.Status, "new", status)
            model.Status = status
        }
        model.LastHealthCheck = time.Now()
    }
}
```

## Ensemble Processor

```go
// internal/ensemble/processor.go

package ensemble

import (
    "context"
    "encoding/json"
    "fmt"
    "sort"
    "sync"
    "time"
)

type EnsembleProcessor struct {
    router      *ModelRouter
    registry    *ModelRegistry
    costTracker *CostTracker
}

type EnsembleResult struct {
    Result           string
    Confidence       float64
    Strategy         CombinationStrategy
    IndividualResults []*ModelResult
    AgreementRate    float64
    WinningModel     string
    TotalCost        float64
    TotalLatencyMs   int64
}

func (p *EnsembleProcessor) Process(ctx context.Context, req *ProcessEnsembleRequest) (*EnsembleResult, error) {
    // Get models to query
    modelIDs := req.Config.ModelIds
    if len(modelIDs) == 0 {
        // Use default models for task
        modelIDs = p.getDefaultModelsForTask(req.Task)
    }

    if len(modelIDs) < 2 {
        return nil, fmt.Errorf("ensemble requires at least 2 models, got %d", len(modelIDs))
    }

    // Query all models concurrently
    results := p.queryModels(ctx, modelIDs, req)

    // Filter successful results
    var successfulResults []*ModelResult
    for _, r := range results {
        if r.Error == "" {
            successfulResults = append(successfulResults, r)
        }
    }

    // Check minimum models requirement
    if len(successfulResults) < int(req.Config.MinModels) {
        return nil, fmt.Errorf("only %d models responded, minimum %d required",
            len(successfulResults), req.Config.MinModels)
    }

    // Combine results based on strategy
    combined, err := p.combineResults(successfulResults, req.Config)
    if err != nil {
        return nil, fmt.Errorf("failed to combine results: %w", err)
    }

    // Calculate metrics
    combined.IndividualResults = results
    combined.TotalCost = p.calculateTotalCost(results)
    combined.TotalLatencyMs = p.calculateTotalLatency(results)
    combined.AgreementRate = p.calculateAgreement(successfulResults)

    return combined, nil
}

func (p *EnsembleProcessor) queryModels(ctx context.Context, modelIDs []string, req *ProcessEnsembleRequest) []*ModelResult {
    var wg sync.WaitGroup
    results := make([]*ModelResult, len(modelIDs))

    for i, modelID := range modelIDs {
        wg.Add(1)
        go func(idx int, id string) {
            defer wg.Done()
            results[idx] = p.queryModel(ctx, id, req)
        }(i, modelID)
    }

    wg.Wait()
    return results
}

func (p *EnsembleProcessor) queryModel(ctx context.Context, modelID string, req *ProcessEnsembleRequest) *ModelResult {
    start := time.Now()

    result := &ModelResult{
        ModelId: modelID,
    }

    // Get model and execute
    modelReq := &ModelRequest{
        Prompt:       p.buildPrompt(req.Task, req.Content),
        Content:      req.Content,
        MaxTokens:    int(req.Options.MaxTokens),
        Temperature:  float64(req.Options.Temperature),
        OutputFormat: req.Options.OutputFormat,
    }

    resp, err := p.router.Route(ctx, modelID, modelReq)
    if err != nil {
        result.Error = err.Error()
        return result
    }

    result.Result = resp.Result
    result.LatencyMs = time.Since(start).Milliseconds()

    // Parse confidence from response
    result.Confidence = p.extractConfidence(resp.Result)

    // Calculate cost
    model := p.registry.GetModel(modelID)
    if model != nil && model.Pricing != nil {
        result.Cost = p.calculateCost(model.Pricing, resp.InputTokens, resp.OutputTokens)
    }

    return result
}

func (p *EnsembleProcessor) combineResults(results []*ModelResult, config *EnsembleConfig) (*EnsembleResult, error) {
    switch config.Strategy {
    case CombinationStrategyWeightedAverage:
        return p.weightedAverage(results, config.ModelWeights)

    case CombinationStrategyConfidenceVoting:
        return p.confidenceVoting(results)

    case CombinationStrategyMajorityVote:
        return p.majorityVote(results, config.MinAgreement)

    case CombinationStrategyUnanimous:
        return p.unanimousVote(results)

    case CombinationStrategyCascade:
        return p.cascadeSelection(results, config.MinAgreement)

    default:
        return p.confidenceVoting(results)  // Default to confidence voting
    }
}

func (p *EnsembleProcessor) weightedAverage(results []*ModelResult, weights map[string]float32) (*EnsembleResult, error) {
    // For numeric confidence, compute weighted average
    // For text results, use result from highest weighted model with agreement boost

    var totalWeight float64
    var weightedConfidence float64
    var bestResult *ModelResult
    var bestWeight float64

    for _, r := range results {
        weight := float64(weights[r.ModelId])
        if weight == 0 {
            weight = 1.0 / float64(len(results))  // Equal weight if not specified
        }
        r.Weight = float32(weight)

        weightedConfidence += float64(r.Confidence) * weight
        totalWeight += weight

        if weight > bestWeight {
            bestWeight = weight
            bestResult = r
        }
    }

    if totalWeight == 0 {
        return nil, fmt.Errorf("total weight is zero")
    }

    avgConfidence := weightedConfidence / totalWeight

    // Boost confidence if multiple models agree
    agreementBoost := p.calculateAgreementBoost(results, bestResult.Result)
    finalConfidence := min(1.0, avgConfidence+agreementBoost)

    return &EnsembleResult{
        Result:       bestResult.Result,
        Confidence:   finalConfidence,
        Strategy:     CombinationStrategyWeightedAverage,
        WinningModel: bestResult.ModelId,
    }, nil
}

func (p *EnsembleProcessor) confidenceVoting(results []*ModelResult) (*EnsembleResult, error) {
    // Select result with highest confidence

    var best *ModelResult
    for _, r := range results {
        if best == nil || r.Confidence > best.Confidence {
            best = r
        }
    }

    if best == nil {
        return nil, fmt.Errorf("no results to vote on")
    }

    // Apply agreement boost
    agreementBoost := p.calculateAgreementBoost(results, best.Result)
    finalConfidence := min(1.0, float64(best.Confidence)+agreementBoost)

    return &EnsembleResult{
        Result:       best.Result,
        Confidence:   finalConfidence,
        Strategy:     CombinationStrategyConfidenceVoting,
        WinningModel: best.ModelId,
    }, nil
}

func (p *EnsembleProcessor) majorityVote(results []*ModelResult, minAgreement float32) (*EnsembleResult, error) {
    // Count votes for each unique result
    votes := make(map[string][]*ModelResult)
    for _, r := range results {
        key := p.normalizeResult(r.Result)
        votes[key] = append(votes[key], r)
    }

    // Find majority
    var majority []*ModelResult
    var majorityKey string
    for key, voters := range votes {
        if len(voters) > len(majority) {
            majority = voters
            majorityKey = key
        }
    }

    agreementRate := float64(len(majority)) / float64(len(results))
    if agreementRate < float64(minAgreement) {
        return nil, fmt.Errorf("no majority agreement: %.2f < %.2f required",
            agreementRate, minAgreement)
    }

    // Average confidence of majority voters
    var totalConf float64
    for _, r := range majority {
        totalConf += float64(r.Confidence)
    }
    avgConfidence := totalConf / float64(len(majority))

    // Boost for majority agreement
    confidenceBoost := agreementRate * 0.1
    finalConfidence := min(1.0, avgConfidence+confidenceBoost)

    return &EnsembleResult{
        Result:        majority[0].Result,
        Confidence:    finalConfidence,
        Strategy:      CombinationStrategyMajorityVote,
        AgreementRate: agreementRate,
        WinningModel:  majority[0].ModelId,
    }, nil
}

func (p *EnsembleProcessor) unanimousVote(results []*ModelResult) (*EnsembleResult, error) {
    if len(results) == 0 {
        return nil, fmt.Errorf("no results")
    }

    // Check if all results agree
    firstNormalized := p.normalizeResult(results[0].Result)
    for _, r := range results[1:] {
        if p.normalizeResult(r.Result) != firstNormalized {
            return nil, fmt.Errorf("results are not unanimous")
        }
    }

    // All agree - high confidence
    var totalConf float64
    for _, r := range results {
        totalConf += float64(r.Confidence)
    }
    avgConfidence := totalConf / float64(len(results))

    // Strong boost for unanimous agreement
    finalConfidence := min(1.0, avgConfidence+0.15)

    return &EnsembleResult{
        Result:        results[0].Result,
        Confidence:    finalConfidence,
        Strategy:      CombinationStrategyUnanimous,
        AgreementRate: 1.0,
        WinningModel:  results[0].ModelId,
    }, nil
}

func (p *EnsembleProcessor) cascadeSelection(results []*ModelResult, threshold float32) (*EnsembleResult, error) {
    // Use first result that meets confidence threshold
    for _, r := range results {
        if r.Confidence >= threshold {
            return &EnsembleResult{
                Result:       r.Result,
                Confidence:   float64(r.Confidence),
                Strategy:     CombinationStrategyCascade,
                WinningModel: r.ModelId,
            }, nil
        }
    }

    // No result met threshold - use highest confidence
    return p.confidenceVoting(results)
}

func (p *EnsembleProcessor) normalizeResult(result string) string {
    // Normalize JSON for comparison
    var parsed interface{}
    if err := json.Unmarshal([]byte(result), &parsed); err == nil {
        normalized, _ := json.Marshal(parsed)
        return string(normalized)
    }
    return strings.TrimSpace(strings.ToLower(result))
}

func (p *EnsembleProcessor) calculateAgreementBoost(results []*ModelResult, targetResult string) float64 {
    normalizedTarget := p.normalizeResult(targetResult)
    agreeing := 0
    for _, r := range results {
        if p.normalizeResult(r.Result) == normalizedTarget {
            agreeing++
        }
    }

    agreementRate := float64(agreeing) / float64(len(results))
    // 10% boost for full agreement, scaled down
    return agreementRate * 0.1
}

func (p *EnsembleProcessor) calculateAgreement(results []*ModelResult) float64 {
    if len(results) < 2 {
        return 1.0
    }

    votes := make(map[string]int)
    for _, r := range results {
        key := p.normalizeResult(r.Result)
        votes[key]++
    }

    // Find largest group
    maxVotes := 0
    for _, count := range votes {
        if count > maxVotes {
            maxVotes = count
        }
    }

    return float64(maxVotes) / float64(len(results))
}
```

## Cost Tracker

```go
// internal/cost/tracker.go

package cost

import (
    "context"
    "fmt"
    "sync"
    "time"
)

type CostTracker struct {
    db        *pgxpool.Pool
    redis     *redis.Client
    publisher *events.Publisher
    mu        sync.RWMutex
    budgets   map[string]*Budget
}

type UsageRecord struct {
    TenantID     string
    ModelID      string
    Task         string
    InputTokens  int
    OutputTokens int
    Cost         float64
    Timestamp    time.Time
}

func (t *CostTracker) RecordUsage(ctx context.Context, record *UsageRecord) error {
    // Store in database
    query := `
        INSERT INTO ai_usage (
            id, tenant_id, model_id, task, input_tokens, output_tokens,
            cost, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `
    _, err := t.db.Exec(ctx, query,
        generateID(),
        record.TenantID,
        record.ModelID,
        record.Task,
        record.InputTokens,
        record.OutputTokens,
        record.Cost,
        record.Timestamp,
    )
    if err != nil {
        return err
    }

    // Update real-time counters in Redis
    t.updateCounters(ctx, record)

    // Check budget alerts
    t.checkBudgetAlerts(ctx, record.TenantID)

    return nil
}

func (t *CostTracker) updateCounters(ctx context.Context, record *UsageRecord) {
    now := time.Now()

    pipe := t.redis.Pipeline()

    // Daily counter
    dailyKey := fmt.Sprintf("cost:%s:daily:%s", record.TenantID, now.Format("2006-01-02"))
    pipe.IncrByFloat(ctx, dailyKey, record.Cost)
    pipe.Expire(ctx, dailyKey, 48*time.Hour)

    // Weekly counter
    weekStart := now.Truncate(24*time.Hour).AddDate(0, 0, -int(now.Weekday()))
    weeklyKey := fmt.Sprintf("cost:%s:weekly:%s", record.TenantID, weekStart.Format("2006-01-02"))
    pipe.IncrByFloat(ctx, weeklyKey, record.Cost)
    pipe.Expire(ctx, weeklyKey, 14*24*time.Hour)

    // Monthly counter
    monthlyKey := fmt.Sprintf("cost:%s:monthly:%s", record.TenantID, now.Format("2006-01"))
    pipe.IncrByFloat(ctx, monthlyKey, record.Cost)
    pipe.Expire(ctx, monthlyKey, 45*24*time.Hour)

    pipe.Exec(ctx)
}

func (t *CostTracker) GetCurrentSpend(ctx context.Context, tenantID string) (*CurrentSpend, error) {
    now := time.Now()

    dailyKey := fmt.Sprintf("cost:%s:daily:%s", tenantID, now.Format("2006-01-02"))
    weekStart := now.Truncate(24*time.Hour).AddDate(0, 0, -int(now.Weekday()))
    weeklyKey := fmt.Sprintf("cost:%s:weekly:%s", tenantID, weekStart.Format("2006-01-02"))
    monthlyKey := fmt.Sprintf("cost:%s:monthly:%s", tenantID, now.Format("2006-01"))

    pipe := t.redis.Pipeline()
    dailyCmd := pipe.Get(ctx, dailyKey)
    weeklyCmd := pipe.Get(ctx, weeklyKey)
    monthlyCmd := pipe.Get(ctx, monthlyKey)
    pipe.Exec(ctx)

    daily, _ := dailyCmd.Float64()
    weekly, _ := weeklyCmd.Float64()
    monthly, _ := monthlyCmd.Float64()

    return &CurrentSpend{
        Daily:   daily,
        Weekly:  weekly,
        Monthly: monthly,
    }, nil
}

func (t *CostTracker) checkBudgetAlerts(ctx context.Context, tenantID string) {
    budget := t.getBudget(ctx, tenantID)
    if budget == nil || !budget.AlertsEnabled {
        return
    }

    spend, _ := t.GetCurrentSpend(ctx, tenantID)

    // Check daily threshold
    for _, threshold := range budget.AlertThresholds {
        dailyPercent := spend.Daily / budget.DailyLimit
        if dailyPercent >= threshold && dailyPercent < threshold+0.1 {
            t.publisher.Publish(ctx, "ai.budget_alert", &BudgetAlertEvent{
                TenantID:   tenantID,
                Period:     "daily",
                Threshold:  threshold,
                Current:    spend.Daily,
                Limit:      budget.DailyLimit,
                Percentage: dailyPercent,
            })
        }
    }
}

func (t *CostTracker) IsBudgetExhausted(ctx context.Context, tenantID string) (bool, BudgetAction) {
    budget := t.getBudget(ctx, tenantID)
    if budget == nil {
        return false, BudgetActionWarn
    }

    spend, _ := t.GetCurrentSpend(ctx, tenantID)

    if spend.Daily >= budget.DailyLimit {
        return true, budget.ActionOnLimit
    }

    return false, BudgetActionWarn
}

type CurrentSpend struct {
    Daily   float64
    Weekly  float64
    Monthly float64
}

type BudgetAlertEvent struct {
    TenantID   string
    Period     string
    Threshold  float64
    Current    float64
    Limit      float64
    Percentage float64
}
```

## Model Router

```go
// internal/router/router.go

package router

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/sony/gobreaker"
)

type ModelRouter struct {
    clients    map[string]ModelClient
    breakers   map[string]*gobreaker.CircuitBreaker
    loadTracker *LoadTracker
    mu         sync.RWMutex
}

type ModelClient interface {
    Process(ctx context.Context, req *ModelRequest) (*ModelResponse, error)
    ProcessStream(ctx context.Context, req *ModelRequest) (<-chan *StreamChunk, error)
    GetStatus() ModelStatus
}

type ModelRequest struct {
    Prompt      string
    Content     string
    MaxTokens   int
    Temperature float64
    OutputFormat string
}

type ModelResponse struct {
    Result       string
    InputTokens  int
    OutputTokens int
    LatencyMs    int64
}

func NewModelRouter() *ModelRouter {
    r := &ModelRouter{
        clients:     make(map[string]ModelClient),
        breakers:    make(map[string]*gobreaker.CircuitBreaker),
        loadTracker: NewLoadTracker(),
    }

    return r
}

func (r *ModelRouter) RegisterModel(id string, client ModelClient) {
    r.mu.Lock()
    defer r.mu.Unlock()

    r.clients[id] = client
    r.breakers[id] = gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        id,
        MaxRequests: 5,
        Interval:    10 * time.Second,
        Timeout:     30 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
            return counts.Requests >= 3 && failureRatio >= 0.6
        },
    })
}

func (r *ModelRouter) Route(ctx context.Context, modelID string, req *ModelRequest) (*ModelResponse, error) {
    r.mu.RLock()
    client, ok := r.clients[modelID]
    breaker, hasBreaker := r.breakers[modelID]
    r.mu.RUnlock()

    if !ok {
        return nil, fmt.Errorf("model not found: %s", modelID)
    }

    // Check circuit breaker
    if hasBreaker && breaker.State() == gobreaker.StateOpen {
        return nil, fmt.Errorf("model %s circuit breaker is open", modelID)
    }

    // Track load
    r.loadTracker.IncrementActive(modelID)
    defer r.loadTracker.DecrementActive(modelID)

    // Execute with circuit breaker
    start := time.Now()
    var resp *ModelResponse
    var err error

    if hasBreaker {
        result, cbErr := breaker.Execute(func() (interface{}, error) {
            return client.Process(ctx, req)
        })
        if cbErr != nil {
            return nil, cbErr
        }
        resp = result.(*ModelResponse)
    } else {
        resp, err = client.Process(ctx, req)
        if err != nil {
            return nil, err
        }
    }

    // Record latency
    resp.LatencyMs = time.Since(start).Milliseconds()
    r.loadTracker.RecordLatency(modelID, resp.LatencyMs)

    return resp, nil
}

func (r *ModelRouter) GetModelLoad(modelID string) *ModelLoad {
    return r.loadTracker.GetLoad(modelID)
}

type LoadTracker struct {
    active   map[string]int64
    latencies map[string]*LatencyWindow
    mu       sync.RWMutex
}

type ModelLoad struct {
    ActiveRequests int64
    AvgLatencyMs   float64
    P95LatencyMs   float64
}

type LatencyWindow struct {
    samples []int64
    index   int
    size    int
}

func (l *LoadTracker) RecordLatency(modelID string, latencyMs int64) {
    l.mu.Lock()
    defer l.mu.Unlock()

    if _, ok := l.latencies[modelID]; !ok {
        l.latencies[modelID] = &LatencyWindow{
            samples: make([]int64, 100),
            size:    100,
        }
    }

    window := l.latencies[modelID]
    window.samples[window.index] = latencyMs
    window.index = (window.index + 1) % window.size
}
```

## Configuration

```yaml
# config/ai-coordinator.yaml

server:
  grpc_port: 8085
  metrics_port: 9085

models:
  local:
    - id: "qwen2.5-14b"
      name: "Qwen 2.5 14B"
      endpoint: "http://localhost:8000/v1"
      max_context: 32768
      max_output: 4096
      supported_tasks:
        - entity_extraction
        - categorization
        - summarization
        - question_answering
        - relationship_extraction
        - sentiment_analysis

  cloud:
    - id: "gemini-pro"
      name: "Gemini Pro"
      api_key: "${GEMINI_API_KEY}"
      max_context: 128000
      max_output: 8192
      pricing:
        input_per_1k: 0.00025
        output_per_1k: 0.0005
      supported_tasks:
        - entity_extraction
        - categorization
        - summarization
        - question_answering
        - relationship_extraction
        - sentiment_analysis
        - translation

escalation:
  confidence_threshold: 0.8
  max_entity_count: 10
  always_escalate:
    - executive
    - legal
    - financial
  never_escalate:
    - spam
    - promotional

cost:
  default_daily_limit: 10.0
  default_monthly_limit: 100.0
  alert_thresholds: [0.5, 0.8, 0.95]

cache:
  enabled: true
  ttl: "1h"
  max_size: 10000

circuit_breaker:
  max_requests: 5
  interval: "10s"
  timeout: "30s"
  failure_ratio: 0.6

ensemble:
  enabled: true
  default_strategy: "confidence_voting"
  min_models: 2
  default_min_agreement: 0.6
  timeout: "30s"

task_routing:
  entity_extraction:
    primary_model: "qwen2.5-14b"
    fallback_models: ["gemini-pro"]
    confidence_threshold: 0.8
  categorization:
    primary_model: "qwen2.5-14b"
    fallback_models: ["gemini-pro"]
    confidence_threshold: 0.85
  summarization:
    primary_model: "qwen2.5-14b"
    fallback_models: ["gemini-pro"]
    confidence_threshold: 0.75
  relationship_extraction:
    primary_model: "qwen2.5-14b"
    fallback_models: ["gemini-pro"]
    enable_ensemble: true
    confidence_threshold: 0.7

registry:
  health_check_interval: "30s"
  auto_disable_on_failure: true
  failure_threshold: 3

database:
  host: "dev02"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

redis:
  address: "dev02:6379"

logging:
  level: "info"
  format: "json"
```

## Database Schema

```sql
-- AI usage tracking
CREATE TABLE ai_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    model_id VARCHAR(100) NOT NULL,
    task VARCHAR(50) NOT NULL,
    input_tokens INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    cost DECIMAL(10, 6) NOT NULL,
    latency_ms INTEGER,
    escalated BOOLEAN DEFAULT false,
    cache_hit BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_usage_tenant_date ON ai_usage(tenant_id, created_at);
CREATE INDEX idx_ai_usage_model ON ai_usage(model_id, created_at);

-- Budgets
CREATE TABLE ai_budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL UNIQUE,
    daily_limit DECIMAL(10, 2) NOT NULL DEFAULT 10.0,
    weekly_limit DECIMAL(10, 2) NOT NULL DEFAULT 50.0,
    monthly_limit DECIMAL(10, 2) NOT NULL DEFAULT 100.0,
    alerts_enabled BOOLEAN DEFAULT true,
    alert_thresholds JSONB DEFAULT '[0.5, 0.8, 0.95]',
    action_on_limit VARCHAR(50) DEFAULT 'local_only',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Model preferences
CREATE TABLE ai_model_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    task VARCHAR(50) NOT NULL,
    preference VARCHAR(50) NOT NULL,
    preferred_model_id VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, task)
);

-- Prompt cache
CREATE TABLE ai_prompt_cache (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cache_key VARCHAR(64) NOT NULL UNIQUE,
    prompt_hash VARCHAR(64) NOT NULL,
    model_id VARCHAR(100) NOT NULL,
    task VARCHAR(50) NOT NULL,
    result TEXT NOT NULL,
    confidence FLOAT,
    hit_count INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_prompt_cache_key ON ai_prompt_cache(cache_key);
CREATE INDEX idx_prompt_cache_expires ON ai_prompt_cache(expires_at);
```

## Implementation Structure

```
services/ai-coordinator/
├── cmd/
│   └── ai-coordinator/
│       └── main.go
├── internal/
│   ├── selector/
│   │   ├── selector.go
│   │   └── scoring.go
│   ├── escalation/
│   │   ├── manager.go
│   │   └── rules.go
│   ├── router/
│   │   ├── router.go
│   │   ├── breaker.go
│   │   └── loadbalancer.go
│   ├── registry/
│   │   ├── registry.go
│   │   └── health.go
│   ├── ensemble/
│   │   ├── processor.go
│   │   ├── strategies.go
│   │   └── voting.go
│   ├── routing/
│   │   ├── task_router.go
│   │   └── config.go
│   ├── cost/
│   │   ├── tracker.go
│   │   ├── budget.go
│   │   └── alerts.go
│   ├── cache/
│   │   └── prompt_cache.go
│   ├── clients/
│   │   ├── vllm.go
│   │   └── gemini.go
│   ├── service/
│   │   └── grpc.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── ai/
│           └── v1/
│               └── ai.proto
└── go.mod
```

## Events Published

| Event | Trigger | Payload |
|-------|---------|---------|
| `ai.request_completed` | Request finished | ModelID, Task, Latency, Cost |
| `ai.escalation` | Cloud escalation | Task, Reason, LocalConfidence |
| `ai.budget_alert` | Budget threshold | Period, Threshold, Current |
| `ai.model_unavailable` | Circuit breaker open | ModelID, Duration |
| `ai.cache_hit` | Prompt cache hit | CacheKey, Savings |
