# Feature Specification: Soft Corrections (Context-Aware Acronym Linking)

**Feature Branch**: `015-soft-corrections`
**Created**: 2026-01-20
**Status**: Consolidated
**Bead**: pe-7i1s

> **NOTE**: This feature has been consolidated into the [Unified Mention Resolution System](../013-content-enrichment/mention-resolution.md).
> The unified spec covers soft corrections for all entity types (persons, terms, products, companies, projects) with a shared data model and resolution algorithm.
> This document is retained for historical context and original requirements.

---

## Problem Statement

The current glossary system only supports permanent, global acronym definitions. This creates problems:

1. **Transcription errors**: "LKD", "OKEE" are clearly "LKE" in context, but shouldn't be permanent aliases
2. **Context-dependent acronyms**: "VIP" means "Virtual IP" in networking discussions but "Very Important Person" in customer contexts
3. **One-time usage**: Some acronyms appear once and don't warrant permanent glossary entries

Users need a way to link an acronym to its meaning *for a specific piece of content* without polluting the global glossary.

## User Scenarios & Testing

### User Story 1 - Link Transcription Error to Correct Term (Priority: P1)

User is reviewing acronym questions and sees "OKEE" from a meeting transcript. They recognize this is a transcription error for "LKE" (Linode Kubernetes Engine). They want to link it so:
- This content is findable when searching for "LKE"
- "OKEE" is NOT permanently added as an alias to LKE
- The system remembers this correction for future suggestions

**Why this priority**: Core use case - transcription errors are the most common reason for needing soft corrections.

**Independent Test**: Can be tested by creating a content item with "OKEE", linking it to LKE, then searching for "LKE" and verifying the content appears.

**Acceptance Scenarios**:

1. **Given** a pending acronym question for "OKEE" from meeting M1, **When** user selects "Link to existing term: LKE", **Then** M1 is searchable via "LKE" AND "OKEE" is NOT added to glossary as alias
2. **Given** a soft correction linking "OKEE" → "LKE" exists, **When** user searches for "LKE", **Then** content containing "OKEE" (with correction) appears in results
3. **Given** a soft correction was created, **When** viewing the source content, **Then** the correction is visible (e.g., "OKEE [→LKE]")

---

### User Story 2 - Suggested Previous Links (Priority: P1)

User is reviewing acronym questions and sees "OKE" from a new email. The system recognizes this term was previously linked to "LKE" twice before. It suggests: "Previously linked to LKE (2x). Link again?"

**Why this priority**: This is the key intelligence feature - learning from past corrections to reduce user effort.

**Independent Test**: Can be tested by creating two soft corrections for "OKE" → "LKE", then creating a new content item with "OKE" and verifying the suggestion appears.

**Acceptance Scenarios**:

1. **Given** "OKE" was soft-linked to "LKE" 3 times before, **When** a new acronym question for "OKE" appears, **Then** system shows "Previously linked to: LKE (3x)" with quick-link option
2. **Given** user sees a previous link suggestion, **When** they select "Link again", **Then** the new content is linked AND the count increments to 4
3. **Given** user sees a previous link suggestion, **When** they select "Different this time", **Then** they can provide a new expansion or dismiss

---

### User Story 3 - Context-Dependent Acronyms (Priority: P2)

User encounters "VIP" in two different contexts:
- In a networking meeting: "VIP" = "Virtual IP"
- In a sales meeting: "VIP" = "Very Important Person"

Each content item should be correctly linked to its contextual meaning.

**Why this priority**: Important for accuracy but less common than transcription errors.

**Independent Test**: Can be tested by creating two content items with "VIP", linking each to different expansions, and verifying searches for either expansion find the correct content.

**Acceptance Scenarios**:

1. **Given** "VIP" in meeting M1 (networking), **When** user links to "Virtual IP", **Then** M1 is searchable via "Virtual IP"
2. **Given** "VIP" in meeting M2 (sales), **When** user links to "Very Important Person", **Then** M2 is searchable via "Very Important Person"
3. **Given** both corrections exist, **When** user searches "VIP", **Then** both M1 and M2 appear (original term matches both)
4. **Given** both corrections exist, **When** new "VIP" question appears, **Then** system shows both previous links with counts

---

### User Story 4 - Promote to Permanent Alias (Priority: P3)

After linking "LKD" → "LKE" 10 times, user decides this is common enough to warrant a permanent alias. They can promote the soft correction to a glossary alias.

**Why this priority**: Nice-to-have optimization for frequent corrections.

**Independent Test**: Can be tested by creating 5+ soft corrections, then using "Promote to alias" and verifying the glossary is updated.

**Acceptance Scenarios**:

1. **Given** "LKD" has been soft-linked to "LKE" 5+ times, **When** reviewing "LKD", **Then** system shows option "Promote to permanent alias"
2. **Given** user selects "Promote to permanent alias", **When** confirmed, **Then** "LKD" is added as alias to "LKE" in glossary
3. **Given** alias was promoted, **When** new content with "LKD" is ingested, **Then** it auto-expands to "LKE" (no question generated)

---

### Edge Cases

- What happens when a term has been linked to multiple different terms? Show all with counts, most frequent first.
- What if the target term doesn't exist in glossary? Allow linking to free-text expansion (creates soft expansion, not glossary entry).
- What if content is re-processed? Preserve existing soft corrections.
- What if a soft correction is wrong? User can remove/edit corrections from content view.

## Requirements

### Functional Requirements

- **FR-001**: System MUST allow linking a term in specific content to an existing glossary term without creating permanent alias
- **FR-002**: System MUST allow linking a term to a free-text expansion (for terms not in glossary)
- **FR-003**: System MUST track soft correction patterns: term → linked_term, count, last_used
- **FR-004**: System MUST suggest previous links when the same term appears in new content
- **FR-005**: System MUST include soft-corrected content in search results for the linked term
- **FR-006**: System MUST display soft corrections in the acronym review workflow
- **FR-007**: Users MUST be able to view soft corrections on a piece of content
- **FR-008**: Users MUST be able to remove/edit a soft correction
- **FR-009**: System SHOULD allow promoting frequent soft corrections to permanent aliases
- **FR-010**: System MUST NOT automatically create permanent aliases from soft corrections

### Key Entities

- **SoftCorrection**: Links a term occurrence to its meaning for specific content
  - term: The acronym/term as it appears (e.g., "OKEE")
  - linked_to: The expansion or glossary term it maps to (e.g., "LKE" or "Linode Kubernetes Engine")
  - content_id: The specific content item this applies to
  - position: Optional character offset in content for precise location
  - created_at, created_by

- **CorrectionPattern**: Aggregated statistics for term corrections
  - term: The acronym/term (e.g., "OKE")
  - linked_to: What it's commonly linked to (e.g., "LKE")
  - count: Number of times this pairing has been used
  - last_used_at: When this pairing was last applied
  - (Could be a view/materialized view over SoftCorrection)

## Technical Considerations

### Search Integration

When searching for term X:
1. Standard glossary expansion (permanent aliases)
2. ALSO include content where X was soft-linked as the target
3. Example: Search "LKE" → finds content containing "LKE" OR content where any term was soft-corrected to "LKE"

### Database Schema (Sketch)

```sql
CREATE TABLE soft_corrections (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    content_id BIGINT NOT NULL REFERENCES content(id),
    term TEXT NOT NULL,           -- Original term: "OKEE"
    linked_to TEXT NOT NULL,      -- Target: "LKE" or full expansion
    linked_to_glossary_id BIGINT, -- Optional FK to glossary if linking to existing term
    position INT,                 -- Character offset in content
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by TEXT,

    UNIQUE(content_id, term, position)
);

CREATE INDEX idx_soft_corrections_term ON soft_corrections(tenant_id, term);
CREATE INDEX idx_soft_corrections_linked ON soft_corrections(tenant_id, linked_to);

-- View for correction patterns
CREATE VIEW correction_patterns AS
SELECT
    tenant_id,
    term,
    linked_to,
    COUNT(*) as times_used,
    MAX(created_at) as last_used
FROM soft_corrections
GROUP BY tenant_id, term, linked_to;
```

### CLI Changes

```bash
# New resolve option: link without adding to glossary
penf review questions resolve 44 --link-to LKE
# Resolves question, creates soft correction, does NOT add to glossary

# Show suggestions in question context
penf review questions show 44
# Output includes: "Previously linked to: LKE (3x), OKE (1x)"

# Batch resolve with soft corrections
penf process acronyms batch-resolve '{
  "resolutions": [...],
  "dismissals": [...],
  "soft_links": [
    {"id": 44, "link_to": "LKE"},
    {"id": 60, "link_to": "LKE"}
  ]
}'

# View corrections on content
penf content show <content-id> --corrections

# Promote to permanent alias
penf glossary alias LKE OKE --from-corrections
```

## Success Criteria

### Measurable Outcomes

- **SC-001**: Users can resolve transcription errors without polluting glossary (no permanent aliases created for one-off errors)
- **SC-002**: Previously-seen corrections are suggested with >90% accuracy (same term shows previous link)
- **SC-003**: Search finds soft-corrected content (search for "LKE" finds content where "OKEE" was corrected to LKE)
- **SC-004**: Acronym review workflow is faster - users can "link again" with one action instead of typing expansion

## Open Questions

1. Should soft corrections affect content embeddings? (Re-embed with corrected text?)
2. How to display soft corrections in content view? Inline annotation vs. sidebar?
3. Should there be a confidence score on pattern suggestions?
4. Archive/cleanup policy for soft corrections when content is deleted?
