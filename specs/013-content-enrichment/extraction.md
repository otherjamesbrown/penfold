# AI Extraction & Processing

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

AI extraction applies to content with `full_ai` or `full_ai_chunked` processing profiles. It includes:
- Context injection (structured RAG)
- Template-based prompting
- Assertion extraction (RAID+)
- Embeddings and summaries
- Full audit trail for QA and A/B testing

---

## Context-Augmented Extraction (Structured RAG)

### Problem

Generic extraction prompts lack context:
```
"Extract risks, issues, decisions, and actions from this email: {body}"
```

Results in:
- "Rick" not linked to a person record
- "The project" not linked to specific project
- "As discussed" references nothing
- Actions assigned to raw text names

### Solution: Context Injection

After enrichment (entity resolution, classification, thread tracking), inject relevant context into extraction prompts:

```
"You are extracting structured information from a business email.

CONTEXT:
- Project: TikTok FY26 Discounts
  - Jira: OUT-697 (In Progress), OUT-698 (Open)
  - Goal: Finalize Q1 pricing for TikTok renewal

- Participants in this email:
  - Rick Eskelsen <reskelse@akamai.com> - Sales Lead, TikTok account owner
  - Sabina Sawyer <ssawyer@akamai.com> - Finance, pricing approval authority
  - Hrishikesh Varma <hvarma@akamai.com> - Pricing Analyst

- Thread context (message 4 of 5):
  - Msg 1: Rick proposed 15% discount
  - Msg 2: Sabina requested justification
  - Msg 3: Rick provided competitive analysis
  - This is Sabina's response

- Related prior decisions:
  - 2026-01-10: 'MTC pricing floor set at $X' (from thread RE: MTC Pricing)

INSTRUCTIONS:
Extract RAID (Risks, Actions, Issues, Decisions) from the following email.
For each item, reference specific people by their role when mentioned.
Link actions to the project and tickets where applicable.

EMAIL:
{email_body}
"
```

### Context Tiers

Not all content needs full context. Tiered approach based on processing profile:

| Profile | Context Injected | Token Budget | Rationale |
|---------|------------------|--------------|-----------|
| `full_ai` (threads) | Full | ~500 tokens | High value, worth the cost |
| `full_ai` (standalone) | Participants + Project | ~200 tokens | Less thread context needed |
| `full_ai_chunked` (docs) | Project + Doc metadata | ~150 tokens | Document-level context |
| `metadata_only` | None | 0 | No AI extraction |
| `state_tracking` | None | 0 | Structured parsing only |

### Context Sources

| Context Type | Source | When to Include |
|--------------|--------|-----------------|
| **Participant context** | `people` table | Always for `full_ai` |
| **Project context** | `projects` table (matched by participants/keywords) | If project identified |
| **Thread context** | Prior messages in `email_threads` | For thread replies |
| **Ticket context** | `jira_tickets` (linked or mentioned) | If Jira references found |
| **Prior decisions** | `assertions` table (type=decision, same project) | If project identified |
| **Meeting context** | `meetings` (recent, same participants) | If meeting referenced |

### Context Selection Algorithm

```
function buildExtractionContext(source, enrichment):
    context = {}
    token_budget = getTokenBudget(enrichment.processing_profile)

    if token_budget == 0:
        return null  // No AI extraction

    // Always include participants (high value, low tokens)
    context.participants = enrichment.participants
        .map(p => {
            person = people.get(p.person_id)
            return {
                name: person.canonical_name,
                email: person.primary_email,
                role: person.title,
                relationship: inferRelationship(person, enrichment.project)
            }
        })
    token_budget -= estimateTokens(context.participants)

    // Include project if identified
    if enrichment.project_id and token_budget > 100:
        project = projects.get(enrichment.project_id)
        context.project = {
            name: project.name,
            tickets: getLinkedTickets(project, limit=3),
            goal: project.description
        }
        token_budget -= estimateTokens(context.project)

    // Include thread context for replies
    if enrichment.content_subtype == 'thread' and token_budget > 150:
        thread = email_threads.get(enrichment.thread_id)
        context.thread = {
            position: enrichment.thread_position,
            total: thread.message_count,
            prior_messages: summarizePriorMessages(thread, limit=3)
        }
        token_budget -= estimateTokens(context.thread)

    // Include prior decisions if budget allows
    if enrichment.project_id and token_budget > 100:
        context.prior_decisions = assertions
            .where(project_id=enrichment.project_id, type='decision')
            .orderBy(created_at, desc)
            .limit(3)

    return context
```

---

## Thread Context Building

The `ThreadContextBuilder` uses **raw content**, not prior AI outputs:

```go
func (b *ThreadContextBuilder) Build(threadID string) (*ThreadContext, error) {
    messages := b.repo.GetThreadMessages(threadID)

    context := &ThreadContext{
        MessageCount: len(messages),
        Participants: extractParticipants(messages),
        DateRange:    getDateRange(messages),
        Messages:     make([]MessageSummary, 0),
    }

    for i, msg := range messages {
        // Use TRUNCATED RAW TEXT, not prior AI summaries
        summary := MessageSummary{
            Position:  i + 1,
            From:      msg.FromName,
            Date:      msg.Date,
            // First 500 chars of body, or full if shorter
            Preview:   truncate(msg.BodyText, 500),
        }
        context.Messages = append(context.Messages, summary)
    }

    return context, nil
}
```

**Why raw text, not prior summaries?**
1. Summaries may not exist yet (first-time batch processing)
2. Circular dependency avoided
3. LLM can work with raw text in context
4. Consistent behavior regardless of processing order

**For very long threads (>10 messages):**
- Include full preview for first message (thread root)
- Include full preview for last 3 messages
- Include truncated (100 char) preview for middle messages
- Total context stays within token budget

---

## Extraction Output Schema

With context, extraction outputs can be **grounded** to known entities. Expanded beyond RAID to capture full business intelligence:

```json
{
  "risks": [
    {
      "description": "Discount exceeds standard ceiling",
      "severity": "medium",
      "likelihood": "high",
      "owner_person_id": "uuid-sabina",
      "project_id": "uuid-tiktok-fy26",
      "source_quote": "This is above our usual 12% maximum"
    }
  ],
  "actions": [
    {
      "description": "Get VP approval for 15% discount",
      "assignee_person_id": "uuid-rick",
      "due_date": "2026-01-25",
      "due_date_source": "by end of week",
      "status": "open",
      "ticket_id": "OUT-697",
      "source_quote": "Rick, please escalate to VP by Friday"
    }
  ],
  "issues": [
    {
      "description": "Pricing tool doesn't support tiered discounts",
      "severity": "medium",
      "blocker_for": "OUT-697",
      "owner_person_id": "uuid-hrishikesh",
      "source_quote": "The system can't handle the tiered structure"
    }
  ],
  "decisions": [
    {
      "description": "Approved 15% discount ceiling for TikTok Q1",
      "decision_maker_person_id": "uuid-sabina",
      "project_id": "uuid-tiktok-fy26",
      "rationale": "Competitive pressure from Cloudflare",
      "reversible": true,
      "source_quote": "I'm approving the 15% based on the competitive analysis"
    }
  ],
  "commitments": [
    {
      "description": "Will deliver revised pricing by Wednesday",
      "committer_person_id": "uuid-rick",
      "committed_to_person_id": "uuid-sabina",
      "due_date": "2026-01-22",
      "source_quote": "I'll have the updated numbers to you by Wednesday"
    }
  ],
  "questions": [
    {
      "question": "Do we need legal review for this discount level?",
      "asker_person_id": "uuid-hrishikesh",
      "directed_to_person_id": "uuid-sabina",
      "answered": false,
      "source_quote": "Should legal sign off on anything over 12%?"
    }
  ],
  "sentiment": {
    "overall": "positive",
    "urgency": "high",
    "confidence_in_outcome": "medium",
    "tone": "collaborative"
  },
  "topics": [
    {"topic": "pricing", "relevance": 0.9},
    {"topic": "competitive_analysis", "relevance": 0.7},
    {"topic": "approval_process", "relevance": 0.6}
  ],
  "key_entities_mentioned": [
    {"type": "company", "name": "Cloudflare", "context": "competitor"},
    {"type": "product", "name": "TikTok CDN", "context": "deal_subject"}
  ],
  "thread_contribution": {
    "advances_discussion": true,
    "new_information": true,
    "is_decision_point": true,
    "summary": "Sabina approves discount, requests VP escalation for final sign-off"
  }
}
```

### Extraction Categories

| Category | Description | Grounded To |
|----------|-------------|-------------|
| `risks` | Potential problems, uncertainties | person_id, project_id |
| `actions` | Tasks to be done | person_id, ticket_id, due_date |
| `issues` | Current blockers/problems | person_id, ticket_id |
| `decisions` | Choices made | person_id, project_id |
| `commitments` | Promises/pledges made | committer_id, committed_to_id |
| `questions` | Unanswered questions | asker_id, directed_to_id |
| `sentiment` | Tone, urgency, confidence | - |
| `topics` | Subject matter tags | - |
| `key_entities` | Companies, products, etc. mentioned | - |
| `thread_contribution` | How this message advances the thread | - |

---

## Assertions Data Model

```
┌─────────────────────────┐
│      assertions         │
├─────────────────────────┤
│ id                      │
│ tenant_id               │
│ source_id               │  ← Source this was extracted from
│ thread_id               │  ← If part of thread
│ extraction_run_id       │  ← Link to audit trail
│ assertion_type          │  ← risk, action, issue, decision, commitment, question
│ description             │
│ source_quote            │  ← Original text that triggered extraction
│ confidence              │  ← LLM confidence score
│                         │
│ -- Grounded references  │
│ owner_person_id         │  ← Who owns/is responsible
│ assignee_person_id      │  ← Who it's assigned to (for actions)
│ target_person_id        │  ← Who it's directed at (for questions)
│ decision_maker_person_id│  ← Who made decision (for decisions)
│ project_id              │
│ ticket_id               │  ← Jira ticket if linked
│                         │
│ -- Type-specific fields │
│ severity                │  ← For risks/issues: low/medium/high/critical
│ status                  │  ← For actions: open/in_progress/completed/cancelled
│ due_date                │  ← For actions/commitments
│ due_date_source         │  ← Original text ("by Friday")
│ rationale               │  ← For decisions
│                         │
│ -- Metadata             │
│ created_at              │
│ updated_at              │
│ superseded_by           │  ← If later extraction updated this
│ is_current              │  ← Latest version of this assertion
└─────────────────────────┘
```

### Assertion Types

| Type | Key Fields | Example |
|------|------------|---------|
| `risk` | severity, owner_person_id | "Discount exceeds policy ceiling" |
| `action` | assignee_person_id, due_date, status | "Get VP approval by Friday" |
| `issue` | severity, owner_person_id, ticket_id | "Pricing tool can't handle tiered discounts" |
| `decision` | decision_maker_person_id, rationale | "Approved 15% discount" |
| `commitment` | owner_person_id, target_person_id, due_date | "Will deliver numbers by Wednesday" |
| `question` | owner_person_id, target_person_id, answered | "Do we need legal review?" |

### Assertion Versioning

When re-extracting (new message in thread, rule change, etc.):
1. Don't delete old assertions
2. Mark old as `is_current = false`
3. Set `superseded_by` to new assertion ID
4. Create new assertion with `is_current = true`

This preserves history and allows tracking how understanding evolved.

### Content Sentiment Table

```
┌─────────────────────────┐
│   content_sentiment     │
├─────────────────────────┤
│ id                      │
│ source_id               │
│ extraction_run_id       │
│ overall_sentiment       │  ← positive/neutral/negative
│ urgency                 │  ← low/medium/high/critical
│ confidence_in_outcome   │  ← low/medium/high
│ tone                    │  ← collaborative/confrontational/informational
│ topics[]                │  ← Array of {topic, relevance}
│ key_entities[]          │  ← Companies, products mentioned
│ created_at              │
└─────────────────────────┘
```

---

## Project-Based Prompt Templates

Instead of domain-based templates (sales vs engineering), use **project-based** templates with explicit fallback chain:

```
┌─────────────────────┐
│  prompt_templates   │
├─────────────────────┤
│ id                  │
│ tenant_id           │  ← NULL for system templates
│ name                │  ← "system-default", "tenant-default", "tiktok-fy26"
│ template_type       │  ← system_default | tenant_default | project
│ version             │  ← Semantic versioning
│ project_ids[]       │  ← Projects using this template (if project type)
│ template_text       │  ← The actual prompt template
│ extraction_schema   │  ← JSON schema for expected output
│ created_at          │
│ created_by          │
│ active              │  ← Can be disabled
└─────────────────────┘
```

**Template Resolution (explicit fallback chain):**

```
resolveTemplate(source, enrichment):

    // 1. Check for project-specific template
    if enrichment.project_id:
        template = templates.findByProjectId(enrichment.project_id)
        if template and template.active:
            return template

    // 2. Fall back to tenant default
    template = templates.findByTenantAndType(
        tenant_id=source.tenant_id,
        type='tenant_default'
    )
    if template and template.active:
        return template

    // 3. Fall back to system default (always exists)
    return templates.findByType('system_default')
```

**Template Types:**

| Type | Scope | When Used |
|------|-------|-----------|
| `system_default` | All tenants | No project, no tenant default |
| `tenant_default` | Single tenant | No project identified, or project has no template |
| `project` | Specific projects | Content matched to project |

---

## Extraction Audit Trail

**Critical for quality control, A/B testing, and debugging.**

```
┌─────────────────────────┐
│   extraction_runs       │
├─────────────────────────┤
│ id                      │
│ tenant_id               │
│ source_id               │  ← The content being extracted from
│ thread_id               │  ← If part of thread processing
│ template_id             │  ← Which prompt template was used
│ template_version        │  ← Snapshot of version at run time
│ model_id                │  ← "gpt-4", "claude-3", etc.
│ model_version           │  ← Specific model version
│ context_injected        │  ← JSON: what context was provided
│ full_prompt             │  ← Complete prompt sent to LLM
│ input_tokens            │
│ output_tokens           │
│ latency_ms              │
│ raw_response            │  ← Exact LLM response
│ parsed_response         │  ← Structured extraction result
│ parse_errors            │  ← Any parsing issues
│ created_at              │
└───────────┬─────────────┘
            │
┌───────────┴─────────────┐
│   extraction_feedback   │
├─────────────────────────┤
│ id                      │
│ extraction_run_id       │
│ feedback_type           │  ← correction, validation, rating
│ field_path              │  ← "actions[0].assignee_person_id"
│ original_value          │
│ corrected_value         │
│ feedback_by             │  ← User who provided feedback
│ feedback_at             │
│ notes                   │
└─────────────────────────┘
```

---

## A/B Testing Support

```
┌─────────────────────────┐
│   extraction_experiments│
├─────────────────────────┤
│ id                      │
│ tenant_id               │
│ name                    │  ← "gpt4-vs-claude-raid"
│ description             │
│ status                  │  ← draft, running, completed, analyzed
│ variants[]              │  ← [{name: "control", template_id, model_id, weight: 50}, ...]
│ sample_size             │  ← Target number of extractions
│ started_at              │
│ completed_at            │
└───────────┬─────────────┘
            │
┌───────────┴─────────────┐
│ extraction_experiment_  │
│         results         │
├─────────────────────────┤
│ experiment_id           │
│ variant_name            │
│ extraction_run_id       │
│ metrics                 │  ← {latency, tokens, parse_success, etc.}
└─────────────────────────┘
```

**What you can test:**
- Different prompt templates
- Different models (GPT-4 vs Claude vs Llama)
- Different context injection strategies
- Different extraction schemas

**Quality metrics to track:**
- Parse success rate (did output match schema?)
- User correction rate (how often do users fix results?)
- Entity grounding rate (% of names resolved to person_ids)
- Token efficiency (extraction quality per token spent)
- Latency (end-to-end extraction time)

---

## Thread-Aware Batch Processing

**Key insight:** When ingesting a batch of emails, detect complete threads and only run full extraction on the **latest message** with full thread context.

```
Batch Ingest: 5 emails from "RE: TikTok FY26 discounts" thread
│
├─ Phase 1: Thread Detection (fast, no AI)
│   └─ Group by In-Reply-To/References → single thread
│
├─ Phase 2: Per-Message Embedding (parallel)
│   ├─ Message 1: Embed
│   ├─ Message 2: Embed
│   ├─ Message 3: Embed
│   ├─ Message 4: Embed
│   └─ Message 5: Embed
│
├─ Phase 3: Extraction (on latest only, with full context)
│   └─ Message 5 (latest): Full RAID extraction
│      Context includes:
│      - Full text of messages 1-4
│      - Participants with roles
│      - Project context
│      - Prior decisions from same project
│
└─ Phase 4: Thread Summary (single call)
    └─ Generate thread-level summary from all messages
       - Key decisions made
       - Action items identified
       - Participants and their positions
       - How discussion evolved
```

**Why extract only on latest?**
- Earlier messages' actions may already be completed
- Decisions may have been revised
- Reduces redundant extraction
- Latest message has full context
- If thread grows, we re-extract on new latest

**When to re-process a thread:**
- New message arrives → Re-extract on new latest
- User requests "refresh thread analysis"
- Extraction feedback indicates quality issues

---

## Functional Requirements

### Context-Augmented Extraction

- **FR-480**: System MUST build extraction context from enrichment results before AI extraction
- **FR-481**: System MUST inject participant context (name, email, role) for `full_ai` profiles
- **FR-482**: System MUST inject project context when project is identified with confidence > 0.7
- **FR-483**: System MUST inject thread summary for thread replies (prior message summaries)
- **FR-484**: System MUST inject linked Jira ticket context when tickets are referenced
- **FR-485**: System MUST respect token budget per processing profile
- **FR-486**: System MUST output grounded extractions with person_ids and project_ids where resolvable
- **FR-487**: System MUST fall back to generic extraction if context retrieval fails
- **FR-488**: System MUST log injected context for extraction quality debugging
- **FR-489**: System SHOULD cache person and project context to reduce latency
- **FR-490**: System SHOULD inject prior decisions from same project when available

### Project-Based Templates

- **FR-500**: System MUST support multiple prompt templates per tenant
- **FR-501**: System MUST support template inheritance (project → tenant default → system default)
- **FR-502**: System MUST version prompt templates with semantic versioning
- **FR-503**: System MUST allow templates to be associated with specific projects
- **FR-504**: System MUST support template activation/deactivation without deletion

### Extraction Audit Trail

- **FR-510**: System MUST store complete extraction run records (prompt, context, response)
- **FR-511**: System MUST track model ID and version for each extraction
- **FR-512**: System MUST track token usage (input/output) and latency per extraction
- **FR-513**: System MUST store raw LLM response before parsing
- **FR-514**: System MUST support user feedback/corrections on extraction results
- **FR-515**: System MUST link feedback to specific extraction fields
- **FR-516**: System MUST support replay of extractions with different templates/models

### A/B Testing

- **FR-520**: System MUST support defining extraction experiments with multiple variants
- **FR-521**: System MUST randomly assign content to experiment variants based on weights
- **FR-522**: System MUST track per-variant metrics (latency, tokens, success rate)
- **FR-523**: System MUST support comparing variants on same content (shadow mode)
- **FR-524**: System SHOULD support automatic experiment completion based on sample size

### Thread Processing

- **FR-530**: System MUST detect threads during batch ingest before AI processing
- **FR-531**: System MUST run full extraction only on latest message in thread
- **FR-532**: System MUST include full thread context (all prior messages) in extraction
- **FR-533**: System MUST generate thread-level summary spanning all messages
- **FR-534**: System MUST re-extract on new latest when thread grows
- **FR-535**: System MUST embed each message individually for retrieval
- **FR-536**: System MUST re-generate thread summary when new messages arrive

### Content-Aware AI Processing

- **FR-460**: System MUST apply processing profile based on content classification
- **FR-461**: System MUST support per-message embedding for email threads
- **FR-462**: System MUST generate thread-level summaries for email threads
- **FR-463**: System MUST skip embedding for notification content types
- **FR-464**: System MUST use appropriate chunking strategy per content type
- **FR-465**: System MUST support delta summaries for thread replies
- **FR-466**: System MUST link all thread message embeddings via thread_id
