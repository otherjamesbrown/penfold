# CLI API Contracts: Daily Review Workflow

**Feature**: 006-daily-review
**Date**: 2026-01-15
**Status**: Complete

## Command Reference

### penf review

Main entry point for the daily review workflow.

```bash
penf review [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --mode | Enum | standard | Review mode: quick, standard, detailed |
| --priority | Enum | mixed | Sort: confidence, importance, recency, mixed |
| --filter | String | None | Filter expression for queue |
| --limit | Integer | 100 | Maximum items to queue |
| --resume | Flag | True | Resume active session if exists |
| --new | Flag | False | Force new session (abandon existing) |

**Behavior**:
- If active session exists and --resume (default): Resume from last position
- If active session exists and --new: Abandon old session, start fresh
- If no active session: Create new session, populate queue

**Output**:
- Session summary (if resuming)
- Queue overview (total items, breakdown by confidence)
- First item for review

**Exit Codes**:
| Code | Meaning |
|------|---------|
| 0 | Session completed successfully |
| 1 | Error during review |
| 2 | Session abandoned by user |
| 130 | Interrupted (Ctrl+C) |

---

### penf review status

Show current review session status.

```bash
penf review status
```

**Output**:
```
Review Session Status
---------------------
Session: abc123-def456
Started: 2026-01-15 08:30
Mode: standard | Priority: mixed

Progress: 45/120 items (37.5%)
  Accepted: 30
  Modified: 10
  Rejected: 3
  Skipped: 2

Estimated time remaining: 8 minutes
```

---

### penf review queue

Display the review queue.

```bash
penf review queue [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --all | Flag | False | Show all items (not just pending) |
| --filter | String | None | Filter expression |
| --limit | Integer | 20 | Items per page |
| --page | Integer | 1 | Page number |

**Output**:
```
Review Queue (45 pending of 120 total)
======================================

 #  Type   Subject                          Confidence  Source
--- ------ -------------------------------- ----------- --------
 1  email  Re: Q1 Planning Meeting           0.45       gmail
 2  email  Project update: Alpha release     0.52       gmail
 3  meet   Weekly Standup 2026-01-14         0.61       calendar
...

[j/k] Navigate | [Enter] Review | [f] Filter | [q] Exit
```

---

### penf review next

Show next item in queue for review.

```bash
penf review next [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --skip | Integer | 0 | Skip N items |
| --id | Integer | None | Go to specific item |

**Output**:
```
Item 46 of 120 | Confidence: 0.45 | Type: email
================================================

From: john.doe@company.com
Subject: Re: Q1 Planning Meeting
Date: 2026-01-14 16:42

--- Preview ---
Thanks for the update. I think we should move forward with
Option B for the infrastructure changes. Let's discuss at
tomorrow's meeting...
--- End Preview ---

AI Suggestion:
  Category: project/infrastructure
  Participants: John Doe, Jane Smith
  Tags: decision, planning

[a] Accept | [r] Reject | [m] Modify | [s] Skip | [d] Details | [?] Help
```

---

### penf review accept

Accept AI suggestion for current or specified item.

```bash
penf review accept [OPTIONS] [ITEM_ID]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --batch | String | None | Batch mode: thread, sender, category |
| --confirm | Flag | False | Skip confirmation for batch |

**Behavior**:
- Single item: Accept immediately, move to next
- Batch mode: Show preview, require confirmation

---

### penf review reject

Reject AI suggestion for current or specified item.

```bash
penf review reject [OPTIONS] [ITEM_ID]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --reason | String | None | Rejection reason code |
| --batch | String | None | Batch mode: thread, sender, category |

**Reason Codes**:
- wrong_category
- wrong_participants
- not_relevant
- spam
- duplicate
- other

---

### penf review modify

Modify AI suggestion for current or specified item.

```bash
penf review modify [OPTIONS] [ITEM_ID]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --category | String | None | Override category |
| --add-tag | String | None | Add tag (repeatable) |
| --remove-tag | String | None | Remove tag (repeatable) |
| --add-participant | String | None | Add participant |
| --remove-participant | String | None | Remove participant |
| --interactive | Flag | True | Interactive modification mode |

**Interactive Mode Output**:
```
Modifying Item 46
=================

Current suggestion:
  Category: project/infrastructure
  Participants: John Doe, Jane Smith
  Tags: decision, planning

[c] Change category
[p] Edit participants
[t] Edit tags
[n] Add notes
[Enter] Apply changes
[Esc] Cancel
```

---

### penf review skip

Skip current item for later review.

```bash
penf review skip [OPTIONS] [ITEM_ID]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --reason | String | None | Skip reason |

---

### penf review undo

Undo recent review decision.

```bash
penf review undo [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --count | Integer | 1 | Number of decisions to undo |
| --batch-id | UUID | None | Undo specific batch |

**Output**:
```
Undo last 1 decision(s)?
  Item 45: email "Q1 Budget Review" - accepted

[y/N] Confirm undo
```

---

### penf review batch

Apply batch operations to multiple items.

```bash
penf review batch [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --group | Enum | None | Group by: thread, sender, category, time |
| --action | Enum | None | Action: accept, reject, skip |
| --filter | String | None | Filter items for batch |
| --preview | Flag | True | Show preview before applying |

**Output**:
```
Batch Operation Preview
=======================

Grouping: sender (john.doe@company.com)
Action: accept

Affected items (5):
 - Item 12: Re: Q1 Planning
 - Item 15: Re: Q1 Planning
 - Item 18: Budget Update
 - Item 23: Re: Q1 Planning
 - Item 31: Final Q1 Numbers

Apply to all 5 items? [y/N]
```

---

### penf review complete

Complete the current review session.

```bash
penf review complete [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --force | Flag | False | Complete even with pending items |
| --summary | Flag | True | Show session summary |

**Output**:
```
Review Session Complete
=======================

Session Duration: 23 minutes
Items Reviewed: 120

Summary:
  Accepted: 85 (70.8%)
  Modified: 20 (16.7%)
  Rejected: 10 (8.3%)
  Skipped: 5 (4.2%)

Average time per item: 11.5 seconds
AI accuracy (estimated): 78%

Learning rules suggested: 3
  - john.doe@company.com -> project/infrastructure
  - weekly standup -> meetings/standup
  - budget review -> finance/planning

View suggestions with: penf review rules --pending
```

---

### penf review rules

Manage learning rules.

```bash
penf review rules [SUBCOMMAND] [OPTIONS]
```

**Subcommands**:

```bash
# List rules
penf review rules list [--status=active|disabled|all]

# Show rule details
penf review rules show RULE_ID

# Enable/disable rule
penf review rules enable RULE_ID
penf review rules disable RULE_ID

# View pending suggestions
penf review rules --pending

# Accept suggested rule
penf review rules accept SUGGESTION_ID
```

---

### penf review analytics

Show review analytics (P3).

```bash
penf review analytics [OPTIONS]
```

**Options**:
| Option | Type | Default | Description |
|--------|------|---------|-------------|
| --period | Enum | week | day, week, month |
| --format | Enum | table | table, json, csv |

---

## Internal API Contracts

### ReviewService

Core service interface for review operations.

```python
class ReviewService:
    async def create_session(
        self,
        tenant_id: UUID,
        user_email: str,
        mode: ReviewMode = ReviewMode.STANDARD,
        priority: PriorityMode = PriorityMode.MIXED,
        filter_criteria: dict | None = None,
        limit: int = 100
    ) -> ReviewSession:
        """Create new review session and populate queue."""
        ...

    async def get_active_session(
        self,
        tenant_id: UUID,
        user_email: str
    ) -> ReviewSession | None:
        """Get active session for user, if any."""
        ...

    async def resume_session(
        self,
        session_id: int
    ) -> ReviewSession:
        """Resume paused or active session."""
        ...

    async def get_next_item(
        self,
        session_id: int,
        skip: int = 0
    ) -> ReviewItem | None:
        """Get next item in queue for review."""
        ...

    async def record_decision(
        self,
        item_id: int,
        decision: DecisionType,
        correction: dict | None = None,
        reason: str | None = None,
        time_spent_ms: int = 0
    ) -> UserFeedback:
        """Record user decision on review item."""
        ...

    async def undo_decision(
        self,
        item_id: int
    ) -> bool:
        """Undo recent decision if eligible."""
        ...

    async def apply_batch(
        self,
        session_id: int,
        batch_type: BatchType,
        group_value: str,
        action: DecisionType,
        action_details: dict | None = None
    ) -> BatchOperation:
        """Apply batch operation to grouped items."""
        ...

    async def complete_session(
        self,
        session_id: int,
        force: bool = False
    ) -> SessionSummary:
        """Complete review session and generate summary."""
        ...
```

### ReviewRepository

Database operations for review entities.

```python
class ReviewRepository:
    async def create_session(
        self,
        session: ReviewSession
    ) -> ReviewSession:
        """Persist new review session."""
        ...

    async def get_session_by_id(
        self,
        session_id: int
    ) -> ReviewSession | None:
        """Get session by ID."""
        ...

    async def get_active_session_for_user(
        self,
        tenant_id: UUID,
        user_email: str
    ) -> ReviewSession | None:
        """Get active session for user."""
        ...

    async def update_session(
        self,
        session: ReviewSession
    ) -> ReviewSession:
        """Update session state."""
        ...

    async def get_queue_items(
        self,
        session_id: int,
        status: list[str] | None = None,
        limit: int = 100,
        offset: int = 0
    ) -> list[ReviewItem]:
        """Get items in queue with optional filtering."""
        ...

    async def create_feedback(
        self,
        feedback: UserFeedback
    ) -> UserFeedback:
        """Record user feedback."""
        ...

    async def get_undo_eligible_items(
        self,
        session_id: int,
        count: int = 1
    ) -> list[ReviewItem]:
        """Get recent items eligible for undo."""
        ...
```

---

## Event Contracts

### Events Published

| Event Type | Payload | When |
|------------|---------|------|
| review.session.started | {session_id, user, mode, item_count} | Session created |
| review.session.completed | {session_id, summary} | Session completed |
| review.decision.made | {item_id, decision, correction?} | User makes decision |
| review.batch.applied | {batch_id, item_count, action} | Batch operation applied |
| review.rule.suggested | {rule_id, pattern, confidence} | Learning rule suggested |

### Events Consumed

| Event Type | Source | Handler |
|------------|--------|---------|
| ai.processing.completed | ai_coordination | Queue new items for review |
| content.ingested | connectors | Track for review eligibility |

---

## Error Contracts

### Error Codes

| Code | HTTP | Description |
|------|------|-------------|
| REVIEW_001 | 404 | Session not found |
| REVIEW_002 | 409 | Active session already exists |
| REVIEW_003 | 400 | Session expired |
| REVIEW_004 | 400 | Item not found in queue |
| REVIEW_005 | 400 | Undo not eligible |
| REVIEW_006 | 400 | Batch operation failed |
| REVIEW_007 | 400 | Invalid filter criteria |
| REVIEW_008 | 500 | Database operation failed |

### Error Response Format

```python
class ReviewError:
    code: str          # e.g., "REVIEW_001"
    message: str       # Human-readable message
    details: dict      # Additional context
    recoverable: bool  # Can user retry
```
