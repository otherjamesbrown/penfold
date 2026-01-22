# Project-Aware Person Extraction & Resolution

Part of [Content Enrichment Pipeline](spec.md)
Extension to [Entity Resolution](entity-resolution.md)

**Bead**: pe-3m44
**Status**: Consolidated

> **NOTE**: This feature has been consolidated into the [Unified Mention Resolution System](mention-resolution.md).
> The unified spec covers person extraction along with all other entity types (terms, products, companies, projects) with a shared data model and resolution algorithm.
> This document is retained for historical context and original requirements.

---

## Problem Statement

Current entity resolution handles **email participants** (From/To/CC headers) well. But it doesn't handle:

1. **Names in body text**: "Alan said he would handle the deployment"
2. **Transcript speakers**: Meeting transcripts mention people by first name or nickname
3. **Ambiguous names**: "Alan" could be Allen Duet (LKE PM) or Alan Evans (Sales Director)
4. **Transcription errors**: "Allen" transcribed as "Alan" or "Allan"

The system needs to:
- Extract person references from content body, not just headers
- Use **project context** to rank candidates
- Provide **multiple suggestions** for ambiguous matches
- Let Claude use this data intelligently

---

## User Scenarios

### User Story 1 - Project-Context Disambiguation (Priority: P1)

In an MTC project meeting, speaker says "Alan will handle the LKE integration."
System recognizes "Alan" is ambiguous:
- Allen Duet (MTC team member, LKE PM) - 85% match
- Alan Evans (Sales Engineering Director) - 30% match

Because the content is tagged to MTC project, Allen Duet ranks first.

**Acceptance Scenarios**:

1. **Given** content tagged to MTC project and "Alan" mentioned, **When** resolving, **Then** Allen Duet is primary suggestion with higher confidence
2. **Given** content tagged to Sales project and "Alan" mentioned, **When** resolving, **Then** Alan Evans is primary suggestion
3. **Given** ambiguous match, **When** reviewing, **Then** user sees ranked list of candidates with project context

---

### User Story 2 - Transcription Error Handling (Priority: P1)

Transcript has "Allen" but it was transcribed as "Alan" or "Allan". System should:
- Recognize this as potential transcription variant
- Still match to Allen Duet if project context supports it
- Flag uncertainty for review

**Acceptance Scenarios**:

1. **Given** "Allan" in MTC meeting, **When** resolving, **Then** Allen Duet suggested with "possible transcription variant" note
2. **Given** resolved match, **When** user confirms, **Then** variant is added as soft alias (not permanent)

---

### User Story 3 - Claude-Native Workflow (Priority: P1)

When reviewing person questions, Claude receives:
- The extracted name ("Alan")
- Project context (MTC, LKE-related)
- Ranked candidates with confidence scores
- Soft alias history ("Alan" linked to Allen Duet 5x before in MTC context)

Claude can then intelligently suggest resolution without asking user for every match.

**Acceptance Scenarios**:

1. **Given** Claude reviewing person questions, **When** context includes prior links, **Then** Claude auto-links with high confidence matches
2. **Given** low-confidence match, **When** Claude presents to user, **Then** shows reasoning based on project context

---

## Data Model Extensions

### Person Project Affiliation

Track which people are associated with which projects:

```sql
-- Already exists in project_members, but add explicit affiliation tracking
CREATE TABLE person_project_affinity (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    person_id BIGINT NOT NULL REFERENCES people(id),
    project_id BIGINT NOT NULL REFERENCES projects(id),

    -- Affinity metrics
    mention_count INT DEFAULT 0,       -- Times mentioned in project content
    last_mentioned_at TIMESTAMPTZ,
    is_member BOOLEAN DEFAULT false,   -- Explicit project member
    role TEXT,                         -- Role in project (PM, lead, contributor)

    -- Computed affinity score (0.0 - 1.0)
    affinity_score DECIMAL(3,2) DEFAULT 0.5,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, person_id, project_id)
);

CREATE INDEX idx_person_project_affinity_project
    ON person_project_affinity(tenant_id, project_id);
CREATE INDEX idx_person_project_affinity_person
    ON person_project_affinity(tenant_id, person_id);
```

### Person Name Variants

Track name variants and their resolution history:

```sql
CREATE TABLE person_name_variants (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    variant_text TEXT NOT NULL,        -- "Alan", "Allan", "Al"
    person_id BIGINT REFERENCES people(id),  -- NULL if unresolved

    -- Resolution tracking
    times_seen INT DEFAULT 1,
    times_linked INT DEFAULT 0,        -- How often linked to this person_id
    last_seen_at TIMESTAMPTZ,

    -- Context tracking
    project_id BIGINT REFERENCES projects(id),  -- NULL = global

    -- Soft vs permanent
    is_permanent_alias BOOLEAN DEFAULT false,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(tenant_id, variant_text, project_id)
);

CREATE INDEX idx_person_name_variants_variant
    ON person_name_variants(tenant_id, lower(variant_text));
```

### Extracted Person Mentions

Track person mentions extracted from content:

```sql
CREATE TABLE content_person_mentions (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    content_id BIGINT NOT NULL,        -- Source content

    -- Extraction details
    mentioned_text TEXT NOT NULL,      -- "Alan" as it appeared
    position INT,                      -- Character offset
    context_snippet TEXT,              -- Surrounding text

    -- Resolution
    resolved_person_id BIGINT REFERENCES people(id),
    resolution_confidence DECIMAL(3,2),
    resolution_source TEXT,            -- exact_match, alias, project_context, user_confirmed

    -- Candidates at extraction time
    candidate_persons JSONB,           -- [{person_id, confidence, reason}]

    -- Status
    status TEXT DEFAULT 'pending',     -- pending, resolved, dismissed
    resolved_at TIMESTAMPTZ,
    resolved_by TEXT,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_content_person_mentions_content
    ON content_person_mentions(content_id);
CREATE INDEX idx_content_person_mentions_pending
    ON content_person_mentions(tenant_id, status) WHERE status = 'pending';
```

---

## Resolution Algorithm

### Multi-Stage Resolution

```go
func (r *PersonResolver) ResolveNameInContext(
    ctx context.Context,
    name string,
    contentID int64,
    projectID *int64,
) (*ResolutionResult, error) {

    candidates := []PersonCandidate{}

    // Stage 1: Exact canonical name match
    if person := r.repo.GetByCanonicalName(name); person != nil {
        candidates = append(candidates, PersonCandidate{
            Person:     person,
            Confidence: 1.0,
            Source:     "exact_canonical",
        })
    }

    // Stage 2: Alias match (including first names)
    aliases := r.repo.GetByAlias(name)
    for _, person := range aliases {
        candidates = append(candidates, PersonCandidate{
            Person:     person,
            Confidence: 0.9,
            Source:     "alias_match",
        })
    }

    // Stage 3: Fuzzy match (Levenshtein, phonetic)
    fuzzy := r.repo.FuzzySearch(name, threshold: 0.8)
    for _, match := range fuzzy {
        candidates = append(candidates, PersonCandidate{
            Person:     match.Person,
            Confidence: match.Similarity * 0.8,  // Discount fuzzy matches
            Source:     "fuzzy_match",
        })
    }

    // Stage 4: Apply project context boost
    if projectID != nil {
        for i, c := range candidates {
            affinity := r.repo.GetProjectAffinity(c.Person.ID, *projectID)
            if affinity != nil {
                // Boost confidence based on project affinity
                boost := affinity.AffinityScore * 0.3
                candidates[i].Confidence = min(1.0, c.Confidence + boost)
                candidates[i].ProjectContext = fmt.Sprintf(
                    "%s in %s (%dx mentions)",
                    affinity.Role,
                    affinity.ProjectName,
                    affinity.MentionCount,
                )
            }
        }
    }

    // Stage 5: Check soft alias history
    variants := r.repo.GetNameVariants(name, projectID)
    for _, v := range variants {
        if v.TimesLinked > 0 {
            // Boost candidate that was previously linked
            for i, c := range candidates {
                if c.Person.ID == v.PersonID {
                    historyBoost := min(0.2, float64(v.TimesLinked) * 0.05)
                    candidates[i].Confidence += historyBoost
                    candidates[i].PriorLinks = v.TimesLinked
                }
            }
        }
    }

    // Sort by confidence descending
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Confidence > candidates[j].Confidence
    })

    // Build result
    result := &ResolutionResult{
        MentionedText: name,
        ContentID:     contentID,
        ProjectID:     projectID,
        Candidates:    candidates,
    }

    // Auto-resolve if high confidence single match
    if len(candidates) == 1 && candidates[0].Confidence > 0.95 {
        result.AutoResolved = true
        result.ResolvedPersonID = &candidates[0].Person.ID
    } else if len(candidates) > 0 && candidates[0].Confidence > 0.9 &&
              (len(candidates) == 1 || candidates[0].Confidence - candidates[1].Confidence > 0.3) {
        // Clear winner
        result.AutoResolved = true
        result.ResolvedPersonID = &candidates[0].Person.ID
    }

    return result, nil
}
```

### Confidence Calculation

| Factor | Confidence Impact |
|--------|------------------|
| Exact canonical name match | 1.0 base |
| Exact alias match | 0.9 base |
| Fuzzy match (>90% similarity) | similarity * 0.8 |
| Project member | +0.2 boost |
| High project affinity (>10 mentions) | +0.15 boost |
| Prior soft links (per link) | +0.05 (max +0.2) |
| Transcription variant distance | -0.1 per edit |

---

## Extraction Integration

### During AI Extraction

When AI extracts names from content, the enrichment pipeline:

1. **Extracts raw names** from AI output (assignee: "Alan", owner: "Rick")
2. **Runs resolution** for each name with project context
3. **Stores candidates** for review if not auto-resolved
4. **Injects resolved IDs** into grounded output

```go
func (e *Extractor) EnrichWithPeople(
    extraction *ExtractionOutput,
    contentID int64,
    projectID *int64,
) error {

    // Collect all person references
    names := collectPersonReferences(extraction)  // ["Alan", "Rick", "Sabina"]

    for _, name := range names {
        result := e.resolver.ResolveNameInContext(ctx, name, contentID, projectID)

        if result.AutoResolved {
            // Update extraction with resolved person_id
            updateExtractionPersonIDs(extraction, name, *result.ResolvedPersonID)
        } else {
            // Create person question for review
            e.questions.CreatePersonQuestion(PersonQuestion{
                ContentID:   contentID,
                ProjectID:   projectID,
                MentionText: name,
                Candidates:  result.Candidates,
                Context:     extractContext(extraction, name),
            })
        }

        // Store mention for tracking
        e.mentions.Create(ContentPersonMention{
            ContentID:       contentID,
            MentionedText:   name,
            Candidates:      result.Candidates,
            ResolvedPersonID: result.ResolvedPersonID,
            Status:          statusFromResult(result),
        })
    }

    return nil
}
```

---

## CLI / Review Interface

### Person Question Format

```bash
penf review questions list --type person

# Output:
#  ID    PRI    NAME     PROJECT   CANDIDATES
#  --    ---    ----     -------   ----------
#  72    high   Alan     MTC       Allen Duet (85%), Alan Evans (30%)
#  73    med    Kate     Sales     Kate Williams (92%)
#  74    low    Bob      -         Robert Chen (70%), Bob Smith (68%)
```

### Resolution Options

```bash
penf review questions resolve 72 --link-to <person-id>
# Links "Alan" in this content to the specified person
# Creates soft variant link for future matching

penf review questions resolve 72 --link-to <person-id> --make-alias
# Same as above, PLUS adds as permanent alias

penf review questions dismiss 72 --reason "Not a person name"
```

### Batch Resolve for Claude

```bash
penf process people context --output json

# Returns:
{
  "questions": [
    {
      "id": 72,
      "mentioned_text": "Alan",
      "project": {"id": 5, "name": "MTC"},
      "context": "Alan will handle the LKE integration testing",
      "candidates": [
        {
          "person_id": 101,
          "name": "Allen Duet",
          "confidence": 0.85,
          "reasons": ["Project member", "LKE PM", "Linked 5x before in MTC"],
          "prior_links": 5
        },
        {
          "person_id": 203,
          "name": "Alan Evans",
          "confidence": 0.30,
          "reasons": ["Name match only"],
          "prior_links": 0
        }
      ]
    }
  ],
  "workflow": {
    "auto_link_threshold": 0.9,
    "suggest_threshold": 0.7
  }
}
```

### Claude Decision Flow

Claude receives this context and can:

1. **Auto-link high confidence** (>0.9): Just do it
2. **Suggest with rationale** (0.7-0.9): "I'm linking 'Alan' to Allen Duet because he's the LKE PM and this is an MTC meeting about LKE. Is that correct?"
3. **Ask for help** (<0.7): "I found 'Alan' but I'm not sure who this is. Candidates are..."

---

## Integration with Soft Corrections (pe-7i1s)

This feature shares infrastructure with soft corrections:

| Soft Corrections | Person Resolution |
|------------------|-------------------|
| `soft_corrections` table | `person_name_variants` table |
| Links term → expansion for content | Links variant → person for content |
| Tracks times_used per pairing | Tracks times_linked per pairing |
| Project-scoped corrections | Project-scoped affinity |

Both use the **same pattern**: context-dependent linking with suggestions based on history.

---

## Functional Requirements

### Person Extraction

- **FR-600**: System MUST extract person names from content body text, not just email headers
- **FR-601**: System MUST extract names from AI extraction output (assignee, owner, etc.)
- **FR-602**: System MUST store extracted mentions with position and context

### Project-Aware Resolution

- **FR-610**: System MUST rank candidates based on project affinity
- **FR-611**: System MUST track person-project affinity (mentions, membership)
- **FR-612**: System MUST boost confidence for project members
- **FR-613**: System MUST consider prior soft links in same project context

### Multi-Candidate Workflow

- **FR-620**: System MUST provide ranked candidates for ambiguous matches
- **FR-621**: System MUST include confidence score and reasoning for each candidate
- **FR-622**: System MUST support auto-resolution when single high-confidence match
- **FR-623**: System MUST queue ambiguous matches for review

### Soft Variant Linking

- **FR-630**: System MUST support soft linking (variant → person for specific content)
- **FR-631**: System MUST track soft link history for future suggestions
- **FR-632**: System MUST support promoting soft links to permanent aliases
- **FR-633**: System MUST NOT auto-create permanent aliases from soft links

### Claude Integration

- **FR-640**: System MUST provide batch context for Claude-native processing
- **FR-641**: System MUST include prior link history in question context
- **FR-642**: System MUST support batch resolution via JSON API

---

## Success Criteria

- **SC-001**: 80% of person mentions auto-resolved (no user intervention)
- **SC-002**: Project-context ranking places correct person first 90% of time
- **SC-003**: Prior soft links improve subsequent resolution (5+ links = high confidence)
- **SC-004**: Claude can process person questions in batch with <10% requiring user input
