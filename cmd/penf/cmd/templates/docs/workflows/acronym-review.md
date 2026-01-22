# Workflow: Acronym Review

Resolve unknown acronyms found during content processing.

## Purpose

When Penfold processes content (emails, meeting transcripts), it detects
unknown acronyms and queues them for review. Resolved acronyms are added
to the glossary for search expansion.

## When to Use

- `penf review questions stats` shows pending acronym questions
- User asks to "review acronyms" or "process the question queue"
- After ingesting new content
- Part of the [onboarding workflow](onboarding.md)

## Batch Data Command

```bash
penf process acronyms context --output json
```

Returns everything needed for intelligent batch processing:

```json
{
  "questions": [
    {
      "id": 123,
      "term": "TER",
      "question": "What does 'TER' mean?",
      "context": "...discussed in the TER meeting yesterday...",
      "source_reference": "meeting-2024-01-15",
      "priority": "medium"
    }
  ],
  "glossary": [
    {
      "term": "MVP",
      "expansion": "Minimum Viable Product",
      "context": ["product", "development"]
    }
  ],
  "stats": {
    "total_pending": 15,
    "by_priority": {"high": 2, "medium": 8, "low": 5},
    "resolved_today": 3
  },
  "workflow": {
    "actions": [...],
    "auto_resolve_patterns": [...],
    "batch_resolve_command": "penf process acronyms batch-resolve '<json>'"
  }
}
```

## Decision Guidelines

### Auto-Resolve (Claude handles)

Standard tech/business acronyms Claude knows:

| Category | Examples |
|----------|----------|
| Web/API | REST, API, HTTP, HTTPS, JSON, XML, YAML, URL, DNS, CDN, SSL, TLS |
| Development | MVP, POC, SDK, IDE, CLI, CI/CD, TDD, OOP, DRY, CRUD, MVC |
| Cloud/Infra | AWS, GCP, Azure, K8s, VM, VPC, IAM, S3, EC2, RDS, Lambda |
| Database | SQL, NoSQL, RDBMS, ORM, ACID, ETL, CDC |
| Business | ROI, KPI, OKR, SLA, NDA, B2B, B2C, CRM, ERP |

### Needs Human Input

- **Domain-specific**: Acronyms specific to user's company/industry
- **Ambiguous**: Could mean multiple things (e.g., "PM" = Product Manager or Project Manager?)
- **Context-dependent**: Meaning varies by project or team
- **Uncertain**: Claude isn't confident about the expansion

### Potential Mis-transcriptions

Watch for acronyms that might be speech-to-text errors:
- Check if nearby words suggest a different spelling
- "PLD" might be "PLM", "PLC", "PID"
- Single letters might be words ("C" → "see")

### Already in Glossary

Before resolving, check if term exists in the glossary response.
If exact match exists → dismiss with "Already in glossary"

## Intelligent Processing Strategy

When Claude receives the context:

1. **Categorize all questions:**
   - Known tech acronyms → batch resolve
   - Duplicates of glossary → batch dismiss
   - Uncertain/domain-specific → present to user

2. **Group similar items:**
   - Multiple questions about same term → resolve once
   - Related terms (TER, TERs) → resolve consistently

3. **Present summary to user:**
   ```
   Found 15 acronym questions:
   - 8 standard tech terms (auto-resolving)
   - 3 already in glossary (dismissing)
   - 4 need your input:
     1. "PLD" in context "...the PLD review..." - Could be PLM, PLC, or domain-specific?
     2. "AW" in context "...AW mentioned..." - Likely person initials, dismiss?
   ```

4. **Execute batch after user confirms**

## Available Actions

| Action | Command | Effect |
|--------|---------|--------|
| Resolve | Single: `penf review questions resolve <id> "<expansion>"` | Adds to glossary |
| Dismiss | Single: `penf review questions dismiss <id> "[reason]"` | Marks as not needing expansion |
| Defer | Single: `penf review questions defer <id>` | Keeps in queue for later |
| View Source | `penf review questions source <id> --context 1500` | Shows surrounding content |
| Batch | `penf process acronyms batch-resolve '<json>'` | Multiple actions at once |

## Batch Resolve Format

```bash
penf process acronyms batch-resolve '{
  "resolutions": [
    {"id": 123, "expansion": "Technical Execution Review"},
    {"id": 456, "expansion": "Database as a Service"}
  ],
  "dismissals": [
    {"id": 789, "reason": "Already in glossary"},
    {"id": 101, "reason": "Speaker initials, not acronym"}
  ]
}'
```

## Multi-Context Terms

If an acronym means different things in different contexts:

```bash
# Add with specific context
penf glossary add VIP "Very Important Person" --context sales
penf glossary add VIP "Virtual IP Address" --context networking,MTC
```

When resolving, consider adding context tags if the term is domain-specific.

## Example Session

```
$ penf process acronyms context --output json > /tmp/ctx.json

# Claude analyzes and prepares:
Found 15 acronym questions:
- 8 standard tech terms (auto-resolving): MVP, API, SDK, CI/CD, K8s, VPC, ETL, SLA
- 3 already in glossary (dismissing): TER, DBaaS, MTC
- 4 need your input:

  1. "PLD" - "...the PLD review scheduled for..."
     Could be: Product Launch Date, Programmable Logic Device?
     > Product Launch Date

  2. "AW" - "...AW mentioned that..."
     Looks like person initials, not acronym.
     > dismiss (person initials)

  3. "OBJE" - "...update the OBJE status..."
     Might be typo for "OBJ" (Objective)?
     > alias to OBJ

  4. "TPS" - "...TPS reports are due..."
     Domain-specific, what is it?
     > Third Party Services

# Execute batch:
$ penf process acronyms batch-resolve '{
  "resolutions": [
    {"id": 24, "expansion": "Minimum Viable Product"},
    {"id": 25, "expansion": "Application Programming Interface"},
    ...
    {"id": 31, "expansion": "Product Launch Date"},
    {"id": 34, "expansion": "Third Party Services"}
  ],
  "dismissals": [
    {"id": 28, "reason": "Already in glossary (TER)"},
    {"id": 32, "reason": "Person initials (AW)"},
    {"id": 33, "reason": "Typo - added as alias to OBJ"}
  ]
}'

Batch complete: 10 resolved, 3 dismissed
```

## Related Documentation

- [Glossary concepts](../concepts/glossary.md) - How glossary works
- [Onboarding workflow](onboarding.md) - Full post-import review
- [Init entities workflow](init-entities.md) - Seed acronyms before import
