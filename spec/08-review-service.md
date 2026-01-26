# Review Service Specification

## Overview

The Review Service manages user review workflows for AI-processed content, including session management, queue prioritization, feedback collection, and progressive automation. It learns from user behavior to suggest and apply automation rules.

## Status: Planned (Phase 5)

## Responsibilities

1. **Session Management**: Create, track, complete review sessions with progress persistence
2. **Queue Prioritization**: Priority-based item ordering with configurable strategies
3. **Feedback Collection**: User corrections, approvals, and feedback tracking
4. **Automation Rules**: User-defined processing rules with pattern matching
5. **Pattern Detection**: Learn from user behavior to suggest automation
6. **Progressive Automation**: Gradually increase automation based on confidence

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Review Service                                    │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                      gRPC Server (:8086)                        │    │
│  └──────────────────────────┬─────────────────────────────────────┘    │
│                             │                                           │
│  ┌──────────────────────────┼──────────────────────────────────────┐   │
│  │                          ▼                                       │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │   │
│  │  │   Session    │  │    Queue     │  │  Feedback    │          │   │
│  │  │   Manager    │  │   Manager    │  │  Collector   │          │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │   │
│  │         │                 │                 │                   │   │
│  │         └─────────────────┼─────────────────┘                   │   │
│  │                           ▼                                      │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │                 Automation Engine                        │   │   │
│  │  │     (rules, patterns, progressive automation)           │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  │                           │                                      │   │
│  │  ┌─────────────────────────────────────────────────────────┐   │   │
│  │  │                 Pattern Detector                         │   │   │
│  │  │        (learns from user behavior)                      │   │   │
│  │  └─────────────────────────────────────────────────────────┘   │   │
│  └──────────────────────────┬──────────────────────────────────────┘   │
│                             │                                           │
│         ┌───────────────────┼───────────────────┐                      │
│         ▼                   ▼                   ▼                       │
│  ┌───────────┐       ┌───────────┐       ┌───────────┐                │
│  │PostgreSQL │       │   Redis   │       │  Content  │                │
│  │(sessions, │       │  (queue,  │       │ Processor │                │
│  │  rules)   │       │   cache)  │       │  (:8083)  │                │
│  └───────────┘       └───────────┘       └───────────┘                │
└─────────────────────────────────────────────────────────────────────────┘
```

## gRPC Service Definition

```protobuf
// api/proto/review/v1/review.proto

syntax = "proto3";
package review.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/empty.proto";

service ReviewService {
  // Sessions
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
  rpc GetSession(GetSessionRequest) returns (Session);
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc EndSession(EndSessionRequest) returns (EndSessionResponse);
  rpc PauseSession(PauseSessionRequest) returns (google.protobuf.Empty);
  rpc ResumeSession(ResumeSessionRequest) returns (Session);

  // Queue management
  rpc GetReviewQueue(GetReviewQueueRequest) returns (GetReviewQueueResponse);
  rpc GetNextItem(GetNextItemRequest) returns (ReviewItem);
  rpc SkipItem(SkipItemRequest) returns (ReviewItem);
  rpc DeferItem(DeferItemRequest) returns (google.protobuf.Empty);
  rpc GetQueueStats(GetQueueStatsRequest) returns (QueueStats);

  // Review actions
  rpc ApproveItem(ApproveItemRequest) returns (ApproveItemResponse);
  rpc RejectItem(RejectItemRequest) returns (RejectItemResponse);
  rpc EditItem(EditItemRequest) returns (EditItemResponse);
  rpc SubmitFeedback(SubmitFeedbackRequest) returns (SubmitFeedbackResponse);

  // Undo operations
  rpc UndoAction(UndoActionRequest) returns (UndoActionResponse);
  rpc GetUndoStack(GetUndoStackRequest) returns (GetUndoStackResponse);
  rpc ClearUndoStack(ClearUndoStackRequest) returns (google.protobuf.Empty);

  // Automation rules
  rpc CreateRule(CreateRuleRequest) returns (CreateRuleResponse);
  rpc ListRules(ListRulesRequest) returns (ListRulesResponse);
  rpc GetRule(GetRuleRequest) returns (AutomationRule);
  rpc UpdateRule(UpdateRuleRequest) returns (AutomationRule);
  rpc DeleteRule(DeleteRuleRequest) returns (google.protobuf.Empty);
  rpc TestRule(TestRuleRequest) returns (TestRuleResponse);
  rpc GetSuggestedRules(GetSuggestedRulesRequest) returns (GetSuggestedRulesResponse);

  // Bulk operations
  rpc BulkApprove(BulkApproveRequest) returns (BulkApproveResponse);
  rpc BulkReject(BulkRejectRequest) returns (BulkRejectResponse);
  rpc ApplyRuleToBacklog(ApplyRuleToBacklogRequest) returns (ApplyRuleToBacklogResponse);

  // Analytics
  rpc GetReviewStats(GetReviewStatsRequest) returns (ReviewStats);
  rpc GetPatternInsights(GetPatternInsightsRequest) returns (PatternInsights);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}

// Session messages
message CreateSessionRequest {
  string tenant_id = 1;
  string user_id = 2;
  SessionType type = 3;
  SessionConfig config = 4;
}

enum SessionType {
  SESSION_TYPE_UNSPECIFIED = 0;
  SESSION_TYPE_DAILY_REVIEW = 1;
  SESSION_TYPE_WEEKLY_SUMMARY = 2;
  SESSION_TYPE_FOCUSED = 3;      // Specific category/type
  SESSION_TYPE_CATCHUP = 4;       // Backlog clearing
}

message SessionConfig {
  int32 max_items = 1;
  google.protobuf.Duration time_limit = 2;
  repeated string categories = 3;         // Filter by category
  repeated string source_types = 4;       // Filter by source
  float min_confidence = 5;               // Only show items below this
  PriorityStrategy priority_strategy = 6;
}

enum PriorityStrategy {
  PRIORITY_STRATEGY_UNSPECIFIED = 0;
  PRIORITY_STRATEGY_URGENCY = 1;
  PRIORITY_STRATEGY_IMPORTANCE = 2;
  PRIORITY_STRATEGY_RECENCY = 3;
  PRIORITY_STRATEGY_CONFIDENCE = 4;   // Low confidence first
  PRIORITY_STRATEGY_MIXED = 5;
}

message CreateSessionResponse {
  string session_id = 1;
  Session session = 2;
}

message Session {
  string id = 1;
  string tenant_id = 2;
  string user_id = 3;
  SessionType type = 4;
  SessionState state = 5;
  SessionConfig config = 6;
  SessionProgress progress = 7;
  google.protobuf.Timestamp started_at = 8;
  google.protobuf.Timestamp paused_at = 9;
  google.protobuf.Timestamp completed_at = 10;
  google.protobuf.Duration elapsed_time = 11;
}

enum SessionState {
  SESSION_STATE_UNSPECIFIED = 0;
  SESSION_STATE_ACTIVE = 1;
  SESSION_STATE_PAUSED = 2;
  SESSION_STATE_COMPLETED = 3;
  SESSION_STATE_ABANDONED = 4;
}

message SessionProgress {
  int32 total_items = 1;
  int32 reviewed_items = 2;
  int32 approved_items = 3;
  int32 rejected_items = 4;
  int32 edited_items = 5;
  int32 skipped_items = 6;
  int32 deferred_items = 7;
  float completion_percent = 8;
}

message GetSessionRequest {
  string session_id = 1;
}

message ListSessionsRequest {
  string tenant_id = 1;
  string user_id = 2;
  SessionState state = 3;
  int32 limit = 4;
}

message ListSessionsResponse {
  repeated Session sessions = 1;
}

message EndSessionRequest {
  string session_id = 1;
  string summary = 2;
}

message EndSessionResponse {
  Session session = 1;
  SessionSummary summary = 2;
}

message SessionSummary {
  int32 items_reviewed = 1;
  google.protobuf.Duration time_spent = 2;
  float avg_time_per_item = 3;
  int32 rules_suggested = 4;
  int32 entities_discovered = 5;
}

message PauseSessionRequest {
  string session_id = 1;
}

message ResumeSessionRequest {
  string session_id = 1;
}

// Queue messages
message GetReviewQueueRequest {
  string tenant_id = 1;
  string session_id = 2;
  int32 limit = 3;
  int32 offset = 4;
}

message GetReviewQueueResponse {
  repeated ReviewItem items = 1;
  int32 total_count = 2;
  QueueStats stats = 3;
}

message ReviewItem {
  string id = 1;
  string source_id = 2;
  string source_type = 3;
  string title = 4;
  string snippet = 5;
  string full_content = 6;
  ProcessingResult processing_result = 7;
  ItemMetadata metadata = 8;
  ReviewState review_state = 9;
  int32 priority_score = 10;
  google.protobuf.Timestamp created_at = 11;
}

message ProcessingResult {
  string summary = 1;
  repeated Entity entities = 2;
  string category = 3;
  float confidence = 4;
  string urgency = 5;
  string importance = 6;
}

message Entity {
  string type = 1;
  string value = 2;
  float confidence = 3;
}

message ItemMetadata {
  string from = 1;
  string to = 2;
  google.protobuf.Timestamp date = 3;
  map<string, string> extra = 4;
}

enum ReviewState {
  REVIEW_STATE_UNSPECIFIED = 0;
  REVIEW_STATE_PENDING = 1;
  REVIEW_STATE_IN_REVIEW = 2;
  REVIEW_STATE_APPROVED = 3;
  REVIEW_STATE_REJECTED = 4;
  REVIEW_STATE_EDITED = 5;
  REVIEW_STATE_SKIPPED = 6;
  REVIEW_STATE_DEFERRED = 7;
  REVIEW_STATE_AUTO_APPROVED = 8;
}

message GetNextItemRequest {
  string session_id = 1;
}

message SkipItemRequest {
  string session_id = 1;
  string item_id = 2;
  string reason = 3;
}

message DeferItemRequest {
  string session_id = 1;
  string item_id = 2;
  google.protobuf.Timestamp defer_until = 3;
}

message QueueStats {
  int32 total_pending = 1;
  int32 high_priority = 2;
  int32 low_confidence = 3;
  int32 deferred = 4;
  map<string, int32> by_category = 5;
  map<string, int32> by_source = 6;
}

message GetQueueStatsRequest {
  string tenant_id = 1;
}

// Review action messages
message ApproveItemRequest {
  string session_id = 1;
  string item_id = 2;
  bool create_rule = 3;  // Auto-approve similar in future
  string rule_scope = 4; // sender, category, pattern
}

message ApproveItemResponse {
  bool success = 1;
  ReviewItem next_item = 2;
  string suggested_rule_id = 3;
}

message RejectItemRequest {
  string session_id = 1;
  string item_id = 2;
  string reason = 3;
  RejectionType rejection_type = 4;
}

enum RejectionType {
  REJECTION_TYPE_UNSPECIFIED = 0;
  REJECTION_TYPE_WRONG_CATEGORY = 1;
  REJECTION_TYPE_BAD_EXTRACTION = 2;
  REJECTION_TYPE_NOT_RELEVANT = 3;
  REJECTION_TYPE_SPAM = 4;
  REJECTION_TYPE_DUPLICATE = 5;
  REJECTION_TYPE_OTHER = 6;
}

message RejectItemResponse {
  bool success = 1;
  ReviewItem next_item = 2;
}

message EditItemRequest {
  string session_id = 1;
  string item_id = 2;
  EditedContent edits = 3;
}

message EditedContent {
  string summary = 1;
  string category = 2;
  repeated EntityEdit entity_edits = 3;
  map<string, string> metadata_edits = 4;
}

message EntityEdit {
  string entity_id = 1;
  EditAction action = 2;
  string new_value = 3;
  string new_type = 4;
}

enum EditAction {
  EDIT_ACTION_UNSPECIFIED = 0;
  EDIT_ACTION_MODIFY = 1;
  EDIT_ACTION_DELETE = 2;
  EDIT_ACTION_ADD = 3;
}

message EditItemResponse {
  bool success = 1;
  ReviewItem next_item = 2;
}

message SubmitFeedbackRequest {
  string session_id = 1;
  string item_id = 2;
  FeedbackType type = 3;
  string details = 4;
  int32 rating = 5;  // 1-5
}

enum FeedbackType {
  FEEDBACK_TYPE_UNSPECIFIED = 0;
  FEEDBACK_TYPE_QUALITY = 1;
  FEEDBACK_TYPE_ACCURACY = 2;
  FEEDBACK_TYPE_SUGGESTION = 3;
  FEEDBACK_TYPE_BUG = 4;
}

message SubmitFeedbackResponse {
  bool success = 1;
}

// Undo operation messages
message UndoActionRequest {
  string session_id = 1;
  string action_id = 2;  // Optional, undo specific action; if empty, undo last
}

message UndoActionResponse {
  bool success = 1;
  UndoableAction undone_action = 2;
  ReviewItem restored_item = 3;
  int32 remaining_undo_count = 4;
}

message GetUndoStackRequest {
  string session_id = 1;
}

message GetUndoStackResponse {
  repeated UndoableAction actions = 1;
  int32 max_undo_depth = 2;
}

message ClearUndoStackRequest {
  string session_id = 1;
}

message UndoableAction {
  string id = 1;
  string item_id = 2;
  string item_title = 3;
  ActionType action_type = 4;
  ReviewState previous_state = 5;
  ReviewState new_state = 6;
  google.protobuf.Timestamp performed_at = 7;
  map<string, string> metadata = 8;  // Stores original values for edits
}

enum ActionType {
  ACTION_TYPE_UNSPECIFIED = 0;
  ACTION_TYPE_APPROVE = 1;
  ACTION_TYPE_REJECT = 2;
  ACTION_TYPE_EDIT = 3;
  ACTION_TYPE_SKIP = 4;
  ACTION_TYPE_DEFER = 5;
  ACTION_TYPE_BULK_APPROVE = 6;
  ACTION_TYPE_BULK_REJECT = 7;
}

// Automation rule messages
message AutomationRule {
  string id = 1;
  string tenant_id = 2;
  string name = 3;
  string description = 4;
  bool enabled = 5;
  RuleTrigger trigger = 6;
  repeated RuleCondition conditions = 7;
  RuleAction action = 8;
  RuleStats stats = 9;
  google.protobuf.Timestamp created_at = 10;
  google.protobuf.Timestamp last_triggered_at = 11;
}

message RuleTrigger {
  TriggerType type = 1;
  repeated string source_types = 2;
  repeated string categories = 3;
}

enum TriggerType {
  TRIGGER_TYPE_UNSPECIFIED = 0;
  TRIGGER_TYPE_ON_INGEST = 1;
  TRIGGER_TYPE_ON_PROCESS = 2;
  TRIGGER_TYPE_SCHEDULED = 3;
}

message RuleCondition {
  ConditionField field = 1;
  ConditionOperator operator = 2;
  string value = 3;
}

enum ConditionField {
  CONDITION_FIELD_UNSPECIFIED = 0;
  CONDITION_FIELD_SENDER = 1;
  CONDITION_FIELD_SENDER_DOMAIN = 2;
  CONDITION_FIELD_CATEGORY = 3;
  CONDITION_FIELD_SUBJECT_CONTAINS = 4;
  CONDITION_FIELD_CONFIDENCE_ABOVE = 5;
  CONDITION_FIELD_ENTITY_TYPE = 6;
  CONDITION_FIELD_LABEL = 7;
}

enum ConditionOperator {
  CONDITION_OPERATOR_UNSPECIFIED = 0;
  CONDITION_OPERATOR_EQUALS = 1;
  CONDITION_OPERATOR_NOT_EQUALS = 2;
  CONDITION_OPERATOR_CONTAINS = 3;
  CONDITION_OPERATOR_STARTS_WITH = 4;
  CONDITION_OPERATOR_ENDS_WITH = 5;
  CONDITION_OPERATOR_MATCHES = 6;  // Regex
  CONDITION_OPERATOR_GREATER_THAN = 7;
  CONDITION_OPERATOR_LESS_THAN = 8;
}

message RuleAction {
  ActionType type = 1;
  map<string, string> params = 2;
}

enum ActionType {
  ACTION_TYPE_UNSPECIFIED = 0;
  ACTION_TYPE_AUTO_APPROVE = 1;
  ACTION_TYPE_AUTO_REJECT = 2;
  ACTION_TYPE_ASSIGN_CATEGORY = 3;
  ACTION_TYPE_ADD_TAG = 4;
  ACTION_TYPE_SET_PRIORITY = 5;
  ACTION_TYPE_NOTIFY = 6;
  ACTION_TYPE_SKIP_REVIEW = 7;
}

message RuleStats {
  int64 trigger_count = 1;
  int64 match_count = 2;
  int64 action_count = 3;
  float accuracy = 4;  // Based on user overrides
}

message CreateRuleRequest {
  string tenant_id = 1;
  string name = 2;
  string description = 3;
  RuleTrigger trigger = 4;
  repeated RuleCondition conditions = 5;
  RuleAction action = 6;
}

message CreateRuleResponse {
  AutomationRule rule = 1;
}

message ListRulesRequest {
  string tenant_id = 1;
  bool enabled_only = 2;
}

message ListRulesResponse {
  repeated AutomationRule rules = 1;
}

message GetRuleRequest {
  string rule_id = 1;
}

message UpdateRuleRequest {
  string rule_id = 1;
  AutomationRule rule = 2;
}

message DeleteRuleRequest {
  string rule_id = 1;
}

message TestRuleRequest {
  string tenant_id = 1;
  repeated RuleCondition conditions = 2;
  int32 sample_size = 3;
}

message TestRuleResponse {
  int32 matching_items = 1;
  repeated ReviewItem sample_matches = 2;
  float estimated_coverage = 3;
}

message GetSuggestedRulesRequest {
  string tenant_id = 1;
  int32 limit = 2;
}

message GetSuggestedRulesResponse {
  repeated SuggestedRule suggestions = 1;
}

message SuggestedRule {
  string description = 1;
  repeated RuleCondition conditions = 2;
  RuleAction action = 3;
  float confidence = 4;
  int32 potential_matches = 5;
  string reasoning = 6;
}

// Bulk operation messages
message BulkApproveRequest {
  string tenant_id = 1;
  repeated string item_ids = 2;
}

message BulkApproveResponse {
  int32 approved_count = 1;
  repeated string failed_ids = 2;
}

message BulkRejectRequest {
  string tenant_id = 1;
  repeated string item_ids = 2;
  string reason = 3;
}

message BulkRejectResponse {
  int32 rejected_count = 1;
  repeated string failed_ids = 2;
}

message ApplyRuleToBacklogRequest {
  string rule_id = 1;
  bool dry_run = 2;
}

message ApplyRuleToBacklogResponse {
  int32 matched_count = 1;
  int32 applied_count = 2;
  repeated ReviewItem sample = 3;
}

// Analytics messages
message GetReviewStatsRequest {
  string tenant_id = 1;
  google.protobuf.Timestamp start_date = 2;
  google.protobuf.Timestamp end_date = 3;
}

message ReviewStats {
  int32 total_reviewed = 1;
  int32 approved = 2;
  int32 rejected = 3;
  int32 edited = 4;
  int32 auto_processed = 5;
  float avg_review_time_seconds = 6;
  float accuracy_rate = 7;
  float automation_rate = 8;
  repeated CategoryStat by_category = 9;
  repeated DailyStat by_day = 10;
}

message CategoryStat {
  string category = 1;
  int32 count = 2;
  float approval_rate = 3;
}

message DailyStat {
  string date = 1;
  int32 reviewed = 2;
  int32 pending = 3;
}

message GetPatternInsightsRequest {
  string tenant_id = 1;
}

message PatternInsights {
  repeated BehaviorPattern patterns = 1;
  repeated Insight insights = 2;
}

message BehaviorPattern {
  string description = 1;
  int32 occurrence_count = 2;
  float consistency = 3;
  bool suggested_for_automation = 4;
}

message Insight {
  string title = 1;
  string description = 2;
  InsightType type = 3;
  string recommendation = 4;
}

enum InsightType {
  INSIGHT_TYPE_UNSPECIFIED = 0;
  INSIGHT_TYPE_EFFICIENCY = 1;
  INSIGHT_TYPE_ACCURACY = 2;
  INSIGHT_TYPE_AUTOMATION_OPPORTUNITY = 3;
  INSIGHT_TYPE_QUALITY = 4;
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

## Session Manager

```go
// internal/session/manager.go

package session

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/google/uuid"
)

type Manager struct {
    db         *pgxpool.Pool
    redis      *redis.Client
    queue      *QueueManager
    automation *AutomationEngine
}

type Session struct {
    ID          string
    TenantID    string
    UserID      string
    Type        SessionType
    State       SessionState
    Config      *SessionConfig
    Progress    *SessionProgress
    StartedAt   time.Time
    PausedAt    *time.Time
    CompletedAt *time.Time
    ElapsedTime time.Duration
}

func (m *Manager) CreateSession(ctx context.Context, req *CreateSessionRequest) (*Session, error) {
    // Build queue for session
    queueItems, err := m.queue.BuildQueue(ctx, req.TenantId, req.Config)
    if err != nil {
        return nil, fmt.Errorf("failed to build queue: %w", err)
    }

    session := &Session{
        ID:        uuid.New().String(),
        TenantID:  req.TenantId,
        UserID:    req.UserId,
        Type:      req.Type,
        State:     SessionStateActive,
        Config:    req.Config,
        Progress: &SessionProgress{
            TotalItems: int32(len(queueItems)),
        },
        StartedAt: time.Now(),
    }

    // Store session
    if err := m.storeSession(ctx, session); err != nil {
        return nil, fmt.Errorf("failed to store session: %w", err)
    }

    // Initialize queue in Redis for fast access
    if err := m.queue.InitializeSessionQueue(ctx, session.ID, queueItems); err != nil {
        return nil, fmt.Errorf("failed to initialize queue: %w", err)
    }

    slog.Info("session created",
        "session_id", session.ID,
        "type", session.Type,
        "items", len(queueItems),
    )

    return session, nil
}

func (m *Manager) GetNextItem(ctx context.Context, sessionID string) (*ReviewItem, error) {
    // Get session to verify it's active
    session, err := m.GetSession(ctx, sessionID)
    if err != nil {
        return nil, err
    }

    if session.State != SessionStateActive {
        return nil, fmt.Errorf("session is not active")
    }

    // Get next item from queue
    item, err := m.queue.PopNext(ctx, sessionID)
    if err != nil {
        return nil, err
    }

    if item == nil {
        return nil, nil  // Queue is empty
    }

    // Mark as in-review
    item.ReviewState = ReviewStateInReview
    if err := m.queue.UpdateItemState(ctx, item); err != nil {
        slog.Warn("failed to update item state", "error", err)
    }

    return item, nil
}

func (m *Manager) ApproveItem(ctx context.Context, req *ApproveItemRequest) (*ApproveItemResponse, error) {
    // Record approval
    if err := m.recordAction(ctx, req.SessionId, req.ItemId, ReviewStateApproved, nil); err != nil {
        return nil, err
    }

    // Update progress
    if err := m.updateProgress(ctx, req.SessionId, func(p *SessionProgress) {
        p.ReviewedItems++
        p.ApprovedItems++
    }); err != nil {
        slog.Warn("failed to update progress", "error", err)
    }

    // Learn pattern for automation
    if req.CreateRule {
        rule, err := m.automation.SuggestRuleFromItem(ctx, req.ItemId, req.RuleScope)
        if err == nil {
            return &ApproveItemResponse{
                Success:         true,
                SuggestedRuleId: rule.ID,
            }, nil
        }
    }

    // Get next item
    nextItem, _ := m.GetNextItem(ctx, req.SessionId)

    return &ApproveItemResponse{
        Success:  true,
        NextItem: nextItem,
    }, nil
}

func (m *Manager) RejectItem(ctx context.Context, req *RejectItemRequest) (*RejectItemResponse, error) {
    feedback := map[string]string{
        "reason":          req.Reason,
        "rejection_type":  req.RejectionType.String(),
    }

    if err := m.recordAction(ctx, req.SessionId, req.ItemId, ReviewStateRejected, feedback); err != nil {
        return nil, err
    }

    if err := m.updateProgress(ctx, req.SessionId, func(p *SessionProgress) {
        p.ReviewedItems++
        p.RejectedItems++
    }); err != nil {
        slog.Warn("failed to update progress", "error", err)
    }

    nextItem, _ := m.GetNextItem(ctx, req.SessionId)

    return &RejectItemResponse{
        Success:  true,
        NextItem: nextItem,
    }, nil
}

func (m *Manager) EditItem(ctx context.Context, req *EditItemRequest) (*EditItemResponse, error) {
    // Apply edits
    if err := m.applyEdits(ctx, req.ItemId, req.Edits); err != nil {
        return nil, err
    }

    feedback := map[string]string{"edited": "true"}
    if err := m.recordAction(ctx, req.SessionId, req.ItemId, ReviewStateEdited, feedback); err != nil {
        return nil, err
    }

    if err := m.updateProgress(ctx, req.SessionId, func(p *SessionProgress) {
        p.ReviewedItems++
        p.EditedItems++
    }); err != nil {
        slog.Warn("failed to update progress", "error", err)
    }

    nextItem, _ := m.GetNextItem(ctx, req.SessionId)

    return &EditItemResponse{
        Success:  true,
        NextItem: nextItem,
    }, nil
}

func (m *Manager) EndSession(ctx context.Context, sessionID string) (*SessionSummary, error) {
    session, err := m.GetSession(ctx, sessionID)
    if err != nil {
        return nil, err
    }

    now := time.Now()
    session.State = SessionStateCompleted
    session.CompletedAt = &now
    session.ElapsedTime = now.Sub(session.StartedAt)

    if session.PausedAt != nil {
        // Subtract paused time
        // ... calculate actual elapsed
    }

    if err := m.storeSession(ctx, session); err != nil {
        return nil, err
    }

    // Clear queue from Redis
    m.queue.ClearSessionQueue(ctx, sessionID)

    // Generate summary
    summary := &SessionSummary{
        ItemsReviewed:  session.Progress.ReviewedItems,
        TimeSpent:      session.ElapsedTime,
        AvgTimePerItem: float32(session.ElapsedTime.Seconds()) / float32(max(1, session.Progress.ReviewedItems)),
    }

    // Detect patterns for automation suggestions
    patterns, _ := m.automation.DetectPatterns(ctx, sessionID)
    summary.RulesSuggested = int32(len(patterns))

    slog.Info("session ended",
        "session_id", sessionID,
        "reviewed", session.Progress.ReviewedItems,
        "duration", session.ElapsedTime,
    )

    return summary, nil
}

func (m *Manager) recordAction(ctx context.Context, sessionID, itemID string, state ReviewState, feedback map[string]string) error {
    query := `
        INSERT INTO review_actions (
            id, session_id, item_id, action, feedback, created_at
        ) VALUES ($1, $2, $3, $4, $5, NOW())
    `
    _, err := m.db.Exec(ctx, query,
        uuid.New().String(),
        sessionID,
        itemID,
        state.String(),
        feedback,
    )
    return err
}
```

## Undo Stack

```go
// internal/undo/stack.go

package undo

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
)

const (
    MaxUndoDepth    = 50  // Maximum actions to keep in undo stack
    UndoStackExpiry = 24 * time.Hour
)

type UndoStack struct {
    redis *redis.Client
    db    *pgxpool.Pool
}

type UndoableAction struct {
    ID            string            `json:"id"`
    SessionID     string            `json:"session_id"`
    ItemID        string            `json:"item_id"`
    ItemTitle     string            `json:"item_title"`
    ActionType    ActionType        `json:"action_type"`
    PreviousState ReviewState       `json:"previous_state"`
    NewState      ReviewState       `json:"new_state"`
    PerformedAt   time.Time         `json:"performed_at"`
    Metadata      map[string]string `json:"metadata"`  // Original values for restoration
}

func NewUndoStack(redis *redis.Client, db *pgxpool.Pool) *UndoStack {
    return &UndoStack{
        redis: redis,
        db:    db,
    }
}

func (u *UndoStack) stackKey(sessionID string) string {
    return fmt.Sprintf("review:undo:%s", sessionID)
}

// Push adds an action to the undo stack
func (u *UndoStack) Push(ctx context.Context, action *UndoableAction) error {
    action.ID = uuid.New().String()
    action.PerformedAt = time.Now()

    data, err := json.Marshal(action)
    if err != nil {
        return fmt.Errorf("failed to marshal action: %w", err)
    }

    key := u.stackKey(action.SessionID)

    pipe := u.redis.Pipeline()

    // Push to left (newest first)
    pipe.LPush(ctx, key, data)

    // Trim to max depth
    pipe.LTrim(ctx, key, 0, MaxUndoDepth-1)

    // Set expiry
    pipe.Expire(ctx, key, UndoStackExpiry)

    _, err = pipe.Exec(ctx)
    return err
}

// Pop removes and returns the most recent action (LIFO)
func (u *UndoStack) Pop(ctx context.Context, sessionID string) (*UndoableAction, error) {
    key := u.stackKey(sessionID)

    data, err := u.redis.LPop(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, nil  // Empty stack
    }
    if err != nil {
        return nil, err
    }

    var action UndoableAction
    if err := json.Unmarshal(data, &action); err != nil {
        return nil, err
    }

    return &action, nil
}

// PopByID removes a specific action by ID
func (u *UndoStack) PopByID(ctx context.Context, sessionID, actionID string) (*UndoableAction, error) {
    key := u.stackKey(sessionID)

    // Get all actions
    actions, err := u.redis.LRange(ctx, key, 0, -1).Result()
    if err != nil {
        return nil, err
    }

    for i, data := range actions {
        var action UndoableAction
        if err := json.Unmarshal([]byte(data), &action); err != nil {
            continue
        }

        if action.ID == actionID {
            // Remove this item
            u.redis.LSet(ctx, key, int64(i), "__DELETED__")
            u.redis.LRem(ctx, key, 1, "__DELETED__")
            return &action, nil
        }
    }

    return nil, fmt.Errorf("action not found: %s", actionID)
}

// GetStack returns all actions in the undo stack
func (u *UndoStack) GetStack(ctx context.Context, sessionID string) ([]*UndoableAction, error) {
    key := u.stackKey(sessionID)

    data, err := u.redis.LRange(ctx, key, 0, -1).Result()
    if err != nil {
        return nil, err
    }

    var actions []*UndoableAction
    for _, item := range data {
        var action UndoableAction
        if err := json.Unmarshal([]byte(item), &action); err != nil {
            continue
        }
        actions = append(actions, &action)
    }

    return actions, nil
}

// Clear empties the undo stack for a session
func (u *UndoStack) Clear(ctx context.Context, sessionID string) error {
    return u.redis.Del(ctx, u.stackKey(sessionID)).Err()
}

// Count returns the number of actions in the stack
func (u *UndoStack) Count(ctx context.Context, sessionID string) (int64, error) {
    return u.redis.LLen(ctx, u.stackKey(sessionID)).Result()
}
```

```go
// internal/session/undo.go

package session

import (
    "context"
    "fmt"
)

// UndoLastAction undoes the most recent action in the session
func (m *Manager) UndoLastAction(ctx context.Context, sessionID string) (*UndoActionResponse, error) {
    // Pop the most recent action
    action, err := m.undoStack.Pop(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("failed to get undo action: %w", err)
    }
    if action == nil {
        return &UndoActionResponse{
            Success: false,
        }, nil
    }

    return m.performUndo(ctx, sessionID, action)
}

// UndoSpecificAction undoes a specific action by ID
func (m *Manager) UndoSpecificAction(ctx context.Context, sessionID, actionID string) (*UndoActionResponse, error) {
    action, err := m.undoStack.PopByID(ctx, sessionID, actionID)
    if err != nil {
        return nil, fmt.Errorf("failed to get undo action: %w", err)
    }

    return m.performUndo(ctx, sessionID, action)
}

func (m *Manager) performUndo(ctx context.Context, sessionID string, action *UndoableAction) (*UndoActionResponse, error) {
    // Restore item to previous state
    var err error
    switch action.ActionType {
    case ActionTypeApprove, ActionTypeReject, ActionTypeSkip:
        err = m.restoreItemState(ctx, action.ItemID, action.PreviousState)

    case ActionTypeEdit:
        // Restore original values from metadata
        err = m.restoreEditedValues(ctx, action.ItemID, action.Metadata)

    case ActionTypeDefer:
        err = m.undefer(ctx, action.ItemID)

    case ActionTypeBulkApprove, ActionTypeBulkReject:
        // For bulk operations, metadata contains affected item IDs
        itemIDs := parseBulkItemIDs(action.Metadata["item_ids"])
        for _, itemID := range itemIDs {
            if e := m.restoreItemState(ctx, itemID, action.PreviousState); e != nil {
                err = e  // Continue but track error
            }
        }
    }

    if err != nil {
        return nil, fmt.Errorf("failed to undo action: %w", err)
    }

    // Update progress (reverse the counts)
    m.reverseProgress(ctx, sessionID, action)

    // Re-add item to queue
    item, _ := m.getItem(ctx, action.ItemID)
    if item != nil {
        m.queue.RequeueItem(ctx, sessionID, item)
    }

    // Record undo in audit log
    m.recordUndoAction(ctx, sessionID, action)

    // Get remaining undo count
    remaining, _ := m.undoStack.Count(ctx, sessionID)

    return &UndoActionResponse{
        Success:            true,
        UndoneAction:       convertToProto(action),
        RestoredItem:       item,
        RemainingUndoCount: int32(remaining),
    }, nil
}

func (m *Manager) restoreItemState(ctx context.Context, itemID string, state ReviewState) error {
    query := `
        UPDATE content_processing_results
        SET review_state = $2,
            reviewed_at = CASE WHEN $2 = 'pending' THEN NULL ELSE reviewed_at END,
            updated_at = NOW()
        WHERE id = $1
    `
    _, err := m.db.Exec(ctx, query, itemID, state.String())
    return err
}

func (m *Manager) restoreEditedValues(ctx context.Context, itemID string, original map[string]string) error {
    // Restore each edited field
    if summary, ok := original["original_summary"]; ok {
        if err := m.updateField(ctx, itemID, "summary", summary); err != nil {
            return err
        }
    }

    if category, ok := original["original_category"]; ok {
        if err := m.updateField(ctx, itemID, "primary_category", category); err != nil {
            return err
        }
    }

    // Restore entities if they were edited
    if entitiesJSON, ok := original["original_entities"]; ok {
        if err := m.updateField(ctx, itemID, "entities", entitiesJSON); err != nil {
            return err
        }
    }

    return nil
}

func (m *Manager) undefer(ctx context.Context, itemID string) error {
    query := `
        UPDATE content_processing_results
        SET review_state = 'pending',
            deferred_until = NULL,
            updated_at = NOW()
        WHERE id = $1
    `
    _, err := m.db.Exec(ctx, query, itemID)
    return err
}

func (m *Manager) reverseProgress(ctx context.Context, sessionID string, action *UndoableAction) {
    m.updateProgress(ctx, sessionID, func(p *SessionProgress) {
        p.ReviewedItems--

        switch action.ActionType {
        case ActionTypeApprove:
            p.ApprovedItems--
        case ActionTypeReject:
            p.RejectedItems--
        case ActionTypeEdit:
            p.EditedItems--
        case ActionTypeSkip:
            p.SkippedItems--
        case ActionTypeDefer:
            p.DeferredItems--
        }
    })
}

// PushUndoableAction records an action that can be undone
func (m *Manager) PushUndoableAction(ctx context.Context, sessionID string, item *ReviewItem, actionType ActionType, newState ReviewState, metadata map[string]string) error {
    action := &UndoableAction{
        SessionID:     sessionID,
        ItemID:        item.Id,
        ItemTitle:     item.Title,
        ActionType:    actionType,
        PreviousState: item.ReviewState,
        NewState:      newState,
        Metadata:      metadata,
    }

    return m.undoStack.Push(ctx, action)
}
```

## Queue Manager

```go
// internal/queue/manager.go

package queue

import (
    "context"
    "encoding/json"
    "fmt"
    "sort"

    "github.com/redis/go-redis/v9"
)

type QueueManager struct {
    db    *pgxpool.Pool
    redis *redis.Client
}

func (q *QueueManager) BuildQueue(ctx context.Context, tenantID string, config *SessionConfig) ([]*ReviewItem, error) {
    query := `
        SELECT
            cpr.id,
            cpr.source_id,
            cpr.source_type,
            s.title,
            s.snippet,
            cpr.summary,
            cpr.primary_category,
            cpr.overall_confidence,
            cpr.urgency,
            cpr.importance,
            cpr.created_at
        FROM content_processing_results cpr
        JOIN sources s ON s.id = cpr.source_id
        WHERE cpr.tenant_id = $1
          AND cpr.requires_review = true
          AND cpr.reviewed_at IS NULL
    `

    args := []interface{}{tenantID}
    argIdx := 2

    // Apply filters
    if config.MinConfidence > 0 {
        query += fmt.Sprintf(" AND cpr.overall_confidence < $%d", argIdx)
        args = append(args, config.MinConfidence)
        argIdx++
    }

    if len(config.Categories) > 0 {
        query += fmt.Sprintf(" AND cpr.primary_category = ANY($%d)", argIdx)
        args = append(args, config.Categories)
        argIdx++
    }

    if len(config.SourceTypes) > 0 {
        query += fmt.Sprintf(" AND cpr.source_type = ANY($%d)", argIdx)
        args = append(args, config.SourceTypes)
        argIdx++
    }

    // Apply limit
    if config.MaxItems > 0 {
        query += fmt.Sprintf(" LIMIT $%d", argIdx)
        args = append(args, config.MaxItems)
    }

    rows, err := q.db.Query(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var items []*ReviewItem
    for rows.Next() {
        var item ReviewItem
        var result ProcessingResult
        err := rows.Scan(
            &item.Id,
            &item.SourceId,
            &item.SourceType,
            &item.Title,
            &item.Snippet,
            &result.Summary,
            &result.Category,
            &result.Confidence,
            &result.Urgency,
            &result.Importance,
            &item.CreatedAt,
        )
        if err != nil {
            continue
        }
        item.ProcessingResult = &result
        item.ReviewState = ReviewStatePending

        // Calculate priority score
        item.PriorityScore = q.calculatePriority(&item, config.PriorityStrategy)

        items = append(items, &item)
    }

    // Sort by priority
    sort.Slice(items, func(i, j int) bool {
        return items[i].PriorityScore > items[j].PriorityScore
    })

    return items, nil
}

func (q *QueueManager) calculatePriority(item *ReviewItem, strategy PriorityStrategy) int32 {
    var score int32 = 50  // Base score

    switch strategy {
    case PriorityStrategyUrgency:
        score += urgencyScore(item.ProcessingResult.Urgency)
    case PriorityStrategyImportance:
        score += importanceScore(item.ProcessingResult.Importance)
    case PriorityStrategyRecency:
        hoursSinceCreated := time.Since(item.CreatedAt.AsTime()).Hours()
        score += int32(max(0, 100-hoursSinceCreated))
    case PriorityStrategyConfidence:
        // Lower confidence = higher priority
        score += int32((1 - item.ProcessingResult.Confidence) * 100)
    case PriorityStrategyMixed:
        score += urgencyScore(item.ProcessingResult.Urgency) / 3
        score += importanceScore(item.ProcessingResult.Importance) / 3
        score += int32((1 - item.ProcessingResult.Confidence) * 33)
    }

    return score
}

func (q *QueueManager) InitializeSessionQueue(ctx context.Context, sessionID string, items []*ReviewItem) error {
    key := fmt.Sprintf("review:queue:%s", sessionID)

    pipe := q.redis.Pipeline()
    for _, item := range items {
        data, _ := json.Marshal(item)
        pipe.RPush(ctx, key, data)
    }

    // Set expiry (24 hours)
    pipe.Expire(ctx, key, 24*time.Hour)

    _, err := pipe.Exec(ctx)
    return err
}

func (q *QueueManager) PopNext(ctx context.Context, sessionID string) (*ReviewItem, error) {
    key := fmt.Sprintf("review:queue:%s", sessionID)

    data, err := q.redis.LPop(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }

    var item ReviewItem
    if err := json.Unmarshal(data, &item); err != nil {
        return nil, err
    }

    return &item, nil
}

func urgencyScore(urgency string) int32 {
    switch urgency {
    case "critical":
        return 100
    case "high":
        return 75
    case "medium":
        return 50
    case "low":
        return 25
    default:
        return 0
    }
}

func importanceScore(importance string) int32 {
    switch importance {
    case "critical":
        return 100
    case "high":
        return 75
    case "medium":
        return 50
    case "low":
        return 25
    default:
        return 0
    }
}
```

## Automation Engine

```go
// internal/automation/engine.go

package automation

import (
    "context"
    "encoding/json"
    "fmt"
    "regexp"
)

type AutomationEngine struct {
    db        *pgxpool.Pool
    publisher *events.Publisher
}

type Rule struct {
    ID          string
    TenantID    string
    Name        string
    Description string
    Enabled     bool
    Trigger     *RuleTrigger
    Conditions  []*RuleCondition
    Action      *RuleAction
    Stats       *RuleStats
    CreatedAt   time.Time
}

func (e *AutomationEngine) EvaluateRules(ctx context.Context, tenantID string, item *ReviewItem) ([]*Rule, error) {
    rules, err := e.getEnabledRules(ctx, tenantID)
    if err != nil {
        return nil, err
    }

    var matchingRules []*Rule
    for _, rule := range rules {
        if e.matchesTrigger(rule.Trigger, item) && e.matchesConditions(rule.Conditions, item) {
            matchingRules = append(matchingRules, rule)
        }
    }

    return matchingRules, nil
}

func (e *AutomationEngine) matchesTrigger(trigger *RuleTrigger, item *ReviewItem) bool {
    if len(trigger.SourceTypes) > 0 && !contains(trigger.SourceTypes, item.SourceType) {
        return false
    }

    if len(trigger.Categories) > 0 && !contains(trigger.Categories, item.ProcessingResult.Category) {
        return false
    }

    return true
}

func (e *AutomationEngine) matchesConditions(conditions []*RuleCondition, item *ReviewItem) bool {
    for _, cond := range conditions {
        if !e.evaluateCondition(cond, item) {
            return false  // All conditions must match
        }
    }
    return true
}

func (e *AutomationEngine) evaluateCondition(cond *RuleCondition, item *ReviewItem) bool {
    var fieldValue string

    switch cond.Field {
    case ConditionFieldSender:
        fieldValue = item.Metadata.From
    case ConditionFieldSenderDomain:
        fieldValue = extractDomain(item.Metadata.From)
    case ConditionFieldCategory:
        fieldValue = item.ProcessingResult.Category
    case ConditionFieldSubjectContains:
        fieldValue = item.Title
    case ConditionFieldConfidenceAbove:
        return item.ProcessingResult.Confidence > parseFloat(cond.Value)
    case ConditionFieldLabel:
        // Check if item has label
        if labels, ok := item.Metadata.Extra["labels"]; ok {
            fieldValue = labels
        }
    }

    return e.matchValue(fieldValue, cond.Operator, cond.Value)
}

func (e *AutomationEngine) matchValue(fieldValue string, op ConditionOperator, expected string) bool {
    switch op {
    case ConditionOperatorEquals:
        return strings.EqualFold(fieldValue, expected)
    case ConditionOperatorNotEquals:
        return !strings.EqualFold(fieldValue, expected)
    case ConditionOperatorContains:
        return strings.Contains(strings.ToLower(fieldValue), strings.ToLower(expected))
    case ConditionOperatorStartsWith:
        return strings.HasPrefix(strings.ToLower(fieldValue), strings.ToLower(expected))
    case ConditionOperatorEndsWith:
        return strings.HasSuffix(strings.ToLower(fieldValue), strings.ToLower(expected))
    case ConditionOperatorMatches:
        re, err := regexp.Compile(expected)
        if err != nil {
            return false
        }
        return re.MatchString(fieldValue)
    default:
        return false
    }
}

func (e *AutomationEngine) ApplyRule(ctx context.Context, rule *Rule, item *ReviewItem) error {
    switch rule.Action.Type {
    case ActionTypeAutoApprove:
        item.ReviewState = ReviewStateAutoApproved
        // Update database
        return e.markAutoProcessed(ctx, item.Id, "approved", rule.ID)

    case ActionTypeAutoReject:
        item.ReviewState = ReviewStateRejected
        return e.markAutoProcessed(ctx, item.Id, "rejected", rule.ID)

    case ActionTypeAssignCategory:
        newCategory := rule.Action.Params["category"]
        return e.updateCategory(ctx, item.Id, newCategory)

    case ActionTypeAddTag:
        tag := rule.Action.Params["tag"]
        return e.addTag(ctx, item.Id, tag)

    case ActionTypeSetPriority:
        priority := rule.Action.Params["priority"]
        return e.setPriority(ctx, item.Id, priority)

    case ActionTypeSkipReview:
        return e.markReviewSkipped(ctx, item.Id, rule.ID)
    }

    return nil
}

func (e *AutomationEngine) DetectPatterns(ctx context.Context, sessionID string) ([]*SuggestedRule, error) {
    // Analyze review actions for patterns
    query := `
        SELECT
            ra.action,
            cpr.primary_category,
            s.source_type,
            COUNT(*) as count
        FROM review_actions ra
        JOIN content_processing_results cpr ON ra.item_id = cpr.id
        JOIN sources s ON cpr.source_id = s.id
        WHERE ra.session_id = $1
        GROUP BY ra.action, cpr.primary_category, s.source_type
        HAVING COUNT(*) >= 3
        ORDER BY count DESC
    `

    rows, err := e.db.Query(ctx, query, sessionID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var suggestions []*SuggestedRule
    for rows.Next() {
        var action, category, sourceType string
        var count int
        if err := rows.Scan(&action, &category, &sourceType, &count); err != nil {
            continue
        }

        // Create suggestion based on pattern
        if action == "approved" && count >= 5 {
            suggestion := &SuggestedRule{
                Description: fmt.Sprintf("Auto-approve %s items in %s category", sourceType, category),
                Conditions: []*RuleCondition{
                    {Field: ConditionFieldCategory, Operator: ConditionOperatorEquals, Value: category},
                },
                Action: &RuleAction{
                    Type:   ActionTypeAutoApprove,
                    Params: map[string]string{},
                },
                Confidence:       float32(count) / 10.0,
                PotentialMatches: int32(count * 2),
                Reasoning:        fmt.Sprintf("You approved %d similar items in this session", count),
            }

            if suggestion.Confidence > 1.0 {
                suggestion.Confidence = 1.0
            }

            suggestions = append(suggestions, suggestion)
        }
    }

    return suggestions, nil
}

func (e *AutomationEngine) SuggestRuleFromItem(ctx context.Context, itemID, scope string) (*Rule, error) {
    // Get item details
    item, err := e.getItem(ctx, itemID)
    if err != nil {
        return nil, err
    }

    var conditions []*RuleCondition

    switch scope {
    case "sender":
        conditions = append(conditions, &RuleCondition{
            Field:    ConditionFieldSender,
            Operator: ConditionOperatorEquals,
            Value:    item.Metadata.From,
        })
    case "sender_domain":
        conditions = append(conditions, &RuleCondition{
            Field:    ConditionFieldSenderDomain,
            Operator: ConditionOperatorEquals,
            Value:    extractDomain(item.Metadata.From),
        })
    case "category":
        conditions = append(conditions, &RuleCondition{
            Field:    ConditionFieldCategory,
            Operator: ConditionOperatorEquals,
            Value:    item.ProcessingResult.Category,
        })
    }

    rule := &Rule{
        ID:          generateID(),
        Name:        fmt.Sprintf("Auto-approve from %s", scope),
        Description: fmt.Sprintf("Automatically approve items matching %s", scope),
        Enabled:     false,  // Require explicit enable
        Conditions:  conditions,
        Action: &RuleAction{
            Type: ActionTypeAutoApprove,
        },
    }

    // Store as draft
    if err := e.storeRule(ctx, rule); err != nil {
        return nil, err
    }

    return rule, nil
}
```

## Configuration

```yaml
# config/review-service.yaml

server:
  grpc_port: 8086
  metrics_port: 9086

session:
  default_max_items: 50
  default_time_limit: "1h"
  session_timeout: "24h"
  auto_pause_after: "10m"  # Pause if no activity

queue:
  default_priority_strategy: "mixed"
  cache_ttl: "24h"
  prefetch_count: 10

automation:
  enabled: true
  min_pattern_count: 3
  suggestion_confidence_threshold: 0.7
  max_rules_per_tenant: 100

undo:
  max_depth: 50              # Maximum actions in undo stack per session
  expiry_hours: 24           # How long undo history is kept

database:
  host: "dev02"
  port: 5432
  database: "penfold"
  user: "penfold"
  password: "${DB_PASSWORD}"
  pool_size: 20

redis:
  address: "dev02:6379"

content_processor:
  address: "localhost:8083"
  timeout: "10s"

logging:
  level: "info"
  format: "json"
```

## Database Schema

```sql
-- Review sessions
CREATE TABLE review_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    session_type VARCHAR(50) NOT NULL,
    state VARCHAR(50) NOT NULL DEFAULT 'active',
    config JSONB DEFAULT '{}',
    progress JSONB DEFAULT '{}',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paused_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    elapsed_seconds INTEGER DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_review_sessions_user ON review_sessions(tenant_id, user_id);
CREATE INDEX idx_review_sessions_state ON review_sessions(state, created_at);

-- Review actions
CREATE TABLE review_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES review_sessions(id),
    item_id UUID NOT NULL,
    action VARCHAR(50) NOT NULL,
    feedback JSONB DEFAULT '{}',
    time_spent_seconds INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_review_actions_session ON review_actions(session_id);
CREATE INDEX idx_review_actions_item ON review_actions(item_id);

-- Automation rules
CREATE TABLE automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT true,
    trigger JSONB NOT NULL,
    conditions JSONB NOT NULL,
    action JSONB NOT NULL,
    stats JSONB DEFAULT '{"trigger_count": 0, "match_count": 0, "action_count": 0}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_triggered_at TIMESTAMPTZ
);

CREATE INDEX idx_automation_rules_tenant ON automation_rules(tenant_id, enabled);

-- Rule execution log
CREATE TABLE rule_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES automation_rules(id),
    item_id UUID NOT NULL,
    matched BOOLEAN NOT NULL,
    action_applied BOOLEAN NOT NULL,
    execution_time_ms INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rule_executions_rule ON rule_executions(rule_id, created_at);

-- User feedback
CREATE TABLE review_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID REFERENCES review_sessions(id),
    item_id UUID NOT NULL,
    feedback_type VARCHAR(50) NOT NULL,
    details TEXT,
    rating INTEGER CHECK (rating >= 1 AND rating <= 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_review_feedback_item ON review_feedback(item_id);
```

## Implementation Structure

```
services/review-service/
├── cmd/
│   └── review-service/
│       └── main.go
├── internal/
│   ├── session/
│   │   ├── manager.go
│   │   ├── progress.go
│   │   └── undo.go
│   ├── undo/
│   │   └── stack.go
│   ├── queue/
│   │   ├── manager.go
│   │   ├── priority.go
│   │   └── cache.go
│   ├── automation/
│   │   ├── engine.go
│   │   ├── rules.go
│   │   ├── patterns.go
│   │   └── suggestions.go
│   ├── feedback/
│   │   └── collector.go
│   ├── analytics/
│   │   ├── stats.go
│   │   └── insights.go
│   ├── service/
│   │   └── grpc.go
│   └── config/
│       └── config.go
├── api/
│   └── proto/
│       └── review/
│           └── v1/
│               └── review.proto
└── go.mod
```

## Events Published

| Event | Trigger | Payload |
|-------|---------|---------|
| `review.session_started` | Session created | SessionID, Type, ItemCount |
| `review.session_completed` | Session ended | SessionID, Stats |
| `review.item_approved` | Item approved | ItemID, SessionID |
| `review.item_rejected` | Item rejected | ItemID, SessionID, Reason |
| `review.item_edited` | Item edited | ItemID, SessionID, Edits |
| `review.rule_created` | Rule created | RuleID, Conditions |
| `review.rule_triggered` | Rule matched | RuleID, ItemID, Action |
| `review.pattern_detected` | Pattern found | Description, Confidence |
