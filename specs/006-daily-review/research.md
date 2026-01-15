# Research: Daily Review Workflow Interface

**Feature**: 006-daily-review
**Date**: 2026-01-15
**Status**: Complete

## Research Tasks

### R1: CLI Review Interface UX Patterns

**Question**: What are the best practices for building efficient CLI review interfaces that support high-throughput item validation?

**Research Findings**:

1. **Progressive Disclosure Pattern**: Show minimal information first, allow drilling down on demand
   - Item summary (subject line, source, confidence) on queue view
   - Full context on item detail view
   - Keyboard shortcuts for rapid navigation

2. **Queue Navigation Patterns**:
   - Vim-style navigation (j/k for up/down, Enter for select)
   - Numbered shortcuts for quick selection (1-9)
   - Filter/search within queue without leaving context

3. **Confirmation Patterns**:
   - Single-key actions for high-confidence items (y/n/s for yes/no/skip)
   - Two-key confirmation for destructive or batch operations
   - Undo buffer for recent decisions

4. **Session Persistence**:
   - Auto-save progress every N items
   - Resume from last position on reconnect
   - Clear session state indicators

**Decision**: Implement vim-style navigation with single-key confirmation for individual items, two-key confirmation for batch operations. Use Rich Tables for queue display with live updates.

**Alternatives Considered**:
- Interactive TUI framework (textual) - rejected due to complexity and learning curve
- Web-based interface - rejected as violates CLI-first principle
- Simple prompt-based - rejected as too slow for 100+ item queues

---

### R2: Queue Prioritization Algorithm

**Question**: How should review items be prioritized to optimize user time and learning system effectiveness?

**Research Findings**:

1. **Confidence-Based Sorting**:
   - Low confidence items first (need most human input)
   - vs. High confidence first (quick wins, momentum)
   - Hybrid: Start with high-confidence warmup, then low-confidence focus

2. **Business Value Weighting**:
   - Content type importance (project emails > newsletters)
   - Sender importance (known contacts > unknown)
   - Recency (more recent = more relevant for learning)

3. **Learning System Optimization**:
   - Prioritize items that would fill knowledge gaps
   - Diverse content types for balanced training
   - Edge cases for boundary learning

**Decision**: Implement configurable prioritization with default "hybrid" strategy:
1. First 5 items: High-confidence quick wins (warmup)
2. Remaining: Sorted by `(1 - confidence) * business_importance * recency_weight`

User can switch modes: `--priority=confidence|importance|recency|mixed`

**Alternatives Considered**:
- Pure chronological - rejected as ignores learning optimization
- ML-predicted difficulty - rejected as adds complexity without proven benefit
- Random sampling - rejected as frustrating user experience

---

### R3: Session State Management

**Question**: How should review session state be persisted to support interruption and resumption?

**Research Findings**:

1. **PostgreSQL Session Table**:
   - Store session_id, user_id, current_position, started_at, last_activity
   - JSONB for session metadata (filters, mode, preferences)
   - Automatic expiration after 24 hours

2. **Decision Log**:
   - Record each decision immediately (not on session close)
   - Include decision type, item_id, timestamp, undo_eligible
   - Support rollback of recent decisions

3. **Resume Strategy**:
   - On resume: Load session, show summary of previous work
   - Offer: Continue from last position, restart queue, view changes since pause

**Decision**: Use PostgreSQL with ReviewSession table. Decisions logged immediately to UserFeedback table. Session auto-resumes on `penf review` if active session exists.

**Alternatives Considered**:
- Redis-only sessions - rejected as need durability for learning data
- File-based state - rejected as inconsistent with PostgreSQL-first architecture
- No persistence - rejected as violates FR-010 (session persistence requirement)

---

### R4: Batch Operation Safety

**Question**: How to implement batch operations that are safe, reversible, and efficient?

**Research Findings**:

1. **Transaction Patterns**:
   - Group batch operations in single database transaction
   - Validate all items before applying changes
   - Store batch_id for grouped undo capability

2. **Confirmation Flow**:
   - Preview: Show affected items with proposed changes
   - Confirm: Require explicit confirmation ("Apply to N items? [y/N]")
   - Summary: Show result counts after application

3. **Similarity Detection**:
   - Email thread matching (same thread_id)
   - Sender matching (same from address)
   - AI suggestion matching (same category + similar content)
   - Time window matching (items from same hour/day)

**Decision**: Implement batch operations with preview + confirmation. Group by: thread, sender, category, or time. All batch operations logged with batch_id for group undo.

**Alternatives Considered**:
- Implicit batching only - rejected as user needs explicit control
- No undo for batch - rejected as too risky for 50+ item batches
- Async batch processing - rejected as adds complexity without benefit for <200 items

---

### R5: Learning System Integration

**Question**: How should user feedback be structured for optimal AI learning integration?

**Research Findings**:

1. **Feedback Schema**:
   - decision: accept|reject|modify
   - original_suggestion: {category, confidence, model}
   - user_correction: {category, reason?}
   - context: {content_type, sender_info, time_of_day}

2. **Learning Signal Quality**:
   - High-confidence corrections most valuable
   - Reason codes improve learning ("wrong_project", "wrong_participant", "missed_context")
   - Implicit signals: Time spent on item, skip rate

3. **Integration Points**:
   - Immediate: Update ProcessingResult validation_status
   - Batch: Generate LearningRule suggestions weekly
   - Long-term: Model retraining dataset

**Decision**: Structured feedback with optional reason codes. Immediate integration via UserFeedback table linked to ProcessingResult. Weekly job analyzes patterns for LearningRule suggestions.

**Alternatives Considered**:
- Unstructured feedback - rejected as poor learning signal
- Real-time learning - rejected as too complex, diminishing returns
- Manual rule creation only - rejected as doesn't scale

---

### R6: Rich Terminal Display Patterns

**Question**: What Rich library patterns best support the review workflow display needs?

**Research Findings**:

1. **Component Selection**:
   - Table: Queue list with columns (status, type, subject, confidence, source)
   - Panel: Item detail view with styled content
   - Progress: Session progress bar
   - Prompt: Confirm actions with styled prompts

2. **Live Updates**:
   - Rich Live context for updating queue display
   - Spinner for async operations
   - Status bar for mode/filter indicators

3. **Color Coding**:
   - Confidence: green (>80%), yellow (50-80%), red (<50%)
   - Status: pending (dim), current (bold), completed (green), skipped (yellow)
   - Type: email (blue), meeting (purple), document (cyan)

**Decision**: Use Rich Tables with Panel details. Color-code by confidence and status. Progress bar in footer. No Live updates (refresh on action instead) to keep implementation simple.

**Alternatives Considered**:
- Plain text - rejected as poor readability for large queues
- Full TUI (textual) - rejected as over-engineered for use case
- Live updates - rejected as adds complexity for minimal benefit

---

## Summary of Decisions

| Area | Decision | Rationale |
|------|----------|-----------|
| Navigation | Vim-style with single-key actions | Fast throughput for power users |
| Priority | Hybrid confidence + importance | Balances UX and learning optimization |
| Sessions | PostgreSQL with immediate logging | Durability + learning integration |
| Batch | Preview + confirm pattern | Safety without friction |
| Feedback | Structured with optional reasons | Quality learning signal |
| Display | Rich Tables + Panels | Readability without over-engineering |

## Open Questions (None)

All research questions resolved. Ready for Phase 1 design.
