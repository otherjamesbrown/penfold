# Review Service Specification

## Overview

The Review Service manages user review workflows for AI-processed content, including session management, queue prioritization, and feedback collection.

## Status: Planned (Phase 5)

## Responsibilities

1. **Session Management**: Create, track, complete review sessions
2. **Queue Prioritization**: Priority-based item ordering
3. **Feedback Collection**: User corrections and approvals
4. **Automation Rules**: User-defined processing rules
5. **Pattern Detection**: Learn from user behavior

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Review Service                           │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Session    │    │    Queue     │    │  Feedback    │  │
│  │   Manager    │    │   Manager    │    │  Collector   │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│                             │                                │
│                             ▼                                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                 Automation Engine                    │   │
│  │     (rules, patterns, progressive automation)       │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## gRPC Service

```protobuf
service ReviewService {
  // Sessions
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
  rpc GetSession(GetSessionRequest) returns (GetSessionResponse);
  rpc EndSession(EndSessionRequest) returns (EndSessionResponse);

  // Queue
  rpc GetReviewQueue(GetReviewQueueRequest) returns (GetReviewQueueResponse);
  rpc GetNextItem(GetNextItemRequest) returns (ReviewItem);

  // Feedback
  rpc SubmitFeedback(SubmitFeedbackRequest) returns (SubmitFeedbackResponse);
  rpc ApproveItem(ApproveItemRequest) returns (ApproveItemResponse);
  rpc RejectItem(RejectItemRequest) returns (RejectItemResponse);

  // Rules
  rpc CreateRule(CreateRuleRequest) returns (CreateRuleResponse);
  rpc ListRules(ListRulesRequest) returns (ListRulesResponse);
  rpc UpdateRule(UpdateRuleRequest) returns (UpdateRuleResponse);

  // Health
  rpc Health(HealthRequest) returns (HealthResponse);
}
```

## Review Workflows

### Daily Review
- Show items from past 24 hours
- Prioritize by urgency/importance
- Track review progress

### Weekly Summary
- Aggregate week's content
- Highlight key decisions
- Show relationship changes

## Events Published

- `review.completed` - Item reviewed
- `feedback.submitted` - User feedback
- `rule.created` - New automation rule
- `pattern.detected` - Behavior pattern found
