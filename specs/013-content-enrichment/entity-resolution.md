# Entity Resolution

Part of [Content Enrichment Pipeline](spec.md)

---

## Overview

Entity resolution maps identifiers (email addresses, Slack IDs, names) to canonical person/team/project records. This enables:
- Unified search across identity variations
- Relationship tracking (who communicates with whom)
- Team and project attribution
- Internal vs external classification

---

## Data Model

### People

```
┌─────────────────┐
│     people      │
├─────────────────┤
│ id              │
│ tenant_id       │
│ canonical_name  │  ← Normalized display name
│ primary_email   │
│ title           │  ← Job title (added during review)
│ department      │  ← Department (added during review)
│ is_internal     │  ← Based on domain matching
│ account_type    │  ← person/role/distribution/bot/external_service
│ confidence      │  ← 0.0-1.0, higher = more trusted
│ needs_review    │  ← true if should be reviewed
│ auto_created    │  ← true if created by system
│ reviewed_at     │  ← when manually reviewed
│ reviewed_by     │  ← who reviewed
│ potential_duplicates[] │  ← IDs of possible duplicate records
│ created_at      │
│ updated_at      │
└────────┬────────┘
         │
┌────────┴────────┐
│ person_aliases  │
├─────────────────┤
│ id              │
│ person_id       │
│ alias_type      │  ← email/slack_id/name/display_name
│ alias_value     │
│ confidence      │  ← How confident we are in this alias
│ source          │  ← Where alias was discovered
│ discovered_at   │
└─────────────────┘
```

### Account Types

| Type | Description | Example | Detection |
|------|-------------|---------|-----------|
| `person` | Human individual | `sweisman@akamai.com` | Default |
| `role` | Shared role/function account | `Prb-Facilitator@akamai.com` | Pattern match |
| `distribution` | Mailing list | `team-mtc@akamai.com` | Pattern match |
| `bot` | Automated system | `gsd-jira@akamai.com` | Pattern match, noreply |
| `external_service` | External notification | `comments-noreply@docs.google.com` | External + noreply |

### Teams

```
┌─────────────────┐
│     teams       │
├─────────────────┤
│ id              │
│ tenant_id       │
│ name            │
│ description     │
│ created_at      │
└────────┬────────┘
         │
┌────────┴────────┐
│  team_members   │
├─────────────────┤
│ id              │
│ team_id         │
│ person_id       │
│ role            │  ← lead, member, etc.
│ joined_at       │
└─────────────────┘
```

### Projects

```
┌─────────────────┐
│    projects     │
├─────────────────┤
│ id              │
│ tenant_id       │
│ name            │
│ description     │
│ keywords[]      │  ← For content matching
│ jira_projects[] │  ← Linked Jira project keys
│ created_at      │
└────────┬────────┘
         │
┌────────┴────────┐
│ project_members │
├─────────────────┤
│ id              │
│ project_id      │
│ person_id       │
│ team_id         │  ← Can add team instead of individuals
│ role            │  ← owner, contributor, etc.
│ added_at        │
└─────────────────┘
```

---

## Resolution Algorithm

### Resolve or Create

```go
func (r *EntityResolver) ResolveOrCreate(ctx context.Context, email, displayName string) (*Person, error) {
    // 1. Try exact email match
    if person := r.repo.GetByEmail(email); person != nil {
        return person, nil
    }

    // 2. Try alias match
    if person := r.repo.GetByAlias(email); person != nil {
        return person, nil
    }

    // 3. Check for potential duplicates by name similarity
    candidates := r.repo.SearchByName(displayName)
    if len(candidates) > 0 {
        // Don't auto-merge, but flag for review
        // Create new record but link as potential duplicate
    }

    // 4. Auto-create with low confidence
    person := &Person{
        CanonicalName:  normalizeDisplayName(displayName),
        PrimaryEmail:   email,
        IsInternal:     r.config.IsInternalDomain(email),
        AccountType:    r.detectAccountType(email, displayName),
        Confidence:     0.6,
        NeedsReview:    true,
        AutoCreated:    true,
        CreatedAt:      time.Now(),
    }

    // 5. Add email as alias
    person.Aliases = []PersonAlias{{
        AliasType:  "email",
        AliasValue: email,
        Confidence: 1.0,
        Source:     "auto_created",
    }}

    return r.repo.Create(person)
}
```

### Account Type Detection

```go
func (r *EntityResolver) detectAccountType(email, displayName string) string {
    // Check configured patterns
    if r.config.MatchesPattern(email, "bot") {
        return "bot"
    }
    if r.config.MatchesPattern(email, "distribution_list") {
        return "distribution"
    }
    if r.config.MatchesPattern(email, "role_account") {
        return "role"
    }
    // Check for common patterns
    if strings.Contains(email, "noreply") || strings.Contains(email, "no-reply") {
        return "bot"
    }
    return "person"
}
```

### Display Name Normalization

```go
func normalizeDisplayName(name string) string {
    // "Eskelsen, Rick" → "Rick Eskelsen"
    if strings.Contains(name, ",") {
        parts := strings.SplitN(name, ",", 2)
        if len(parts) == 2 {
            return strings.TrimSpace(parts[1]) + " " + strings.TrimSpace(parts[0])
        }
    }

    // Remove quotes, extra spaces
    name = strings.Trim(name, `"'`)
    name = strings.Join(strings.Fields(name), " ")

    return name
}
```

### Duplicate Detection

```go
func (r *EntityResolver) findPotentialDuplicates(name, email string) []string {
    var duplicates []string

    // 1. Same email domain + similar name
    domain := extractDomain(email)
    candidates := r.repo.GetPeopleByDomain(domain)

    for _, c := range candidates {
        similarity := nameSimilarity(name, c.CanonicalName)
        if similarity > 0.8 {
            duplicates = append(duplicates, c.ID)
        }
    }

    return duplicates
}

func nameSimilarity(a, b string) float64 {
    // Normalize both
    a = strings.ToLower(normalizeDisplayName(a))
    b = strings.ToLower(normalizeDisplayName(b))

    // Exact match
    if a == b {
        return 1.0
    }

    // Check if one contains the other
    if strings.Contains(a, b) || strings.Contains(b, a) {
        return 0.9
    }

    // Levenshtein distance based similarity
    return levenshteinSimilarity(a, b)
}
```

---

## People Review Queue

### Review CLI

```bash
# List people needing review
penf people review list
# Output:
# ID          Name                 Email                    Type    Confidence  Potential Duplicates
# p_abc123    Rick Eskelsen        reskelse@akamai.com     person  0.6         -
# p_def456    Eskelsen, Rick       rick.eskelsen@akamai.com person  0.6         p_abc123 (likely same)
# p_ghi789    TRACK JIRA           gsd-jira@akamai.com     bot     0.8         -

# Review a person - confirm or edit
penf people review p_abc123
# Interactive:
# Name: Rick Eskelsen [enter to keep, or type new]
# Title: [enter to skip, or type]  → "Sales Lead"
# Department: [enter to skip]      → "Sales"
# Account type: person [enter to keep]
# ✓ Marked as reviewed

# Merge duplicates
penf people merge p_def456 --into p_abc123
# Merges all aliases, updates all references, deletes p_def456

# Bulk operations
penf people review list --type bot --auto-approve
# Auto-approves all bot accounts (they don't need human review)
```

---

## Teams Definition

Teams are manually defined via CLI:

```bash
# Create team
penf teams create "MTC Core" --description "MTC product core team"

# Add members
penf teams add-member "MTC Core" --person p_abc123 --role lead
penf teams add-member "MTC Core" --person p_def456
penf teams add-member "MTC Core" --email ssawyer@akamai.com  # Resolves to person

# List teams
penf teams list

# Show team details
penf teams show "MTC Core"
# Output:
# Team: MTC Core
# Description: MTC product core team
# Members:
#   Rick Eskelsen (lead) - reskelse@akamai.com
#   Sabina Sawyer - ssawyer@akamai.com
#   Hrishikesh Varma - hvarma@akamai.com
# Recent activity: 45 emails, 12 threads in last 7 days
```

---

## Projects Definition

Projects are manually defined and linked to Jira:

```bash
# Create project
penf projects create "TikTok FY26" \
  --description "TikTok Q1 2026 renewal and pricing" \
  --jira OUT \
  --keywords "tiktok,fy26,discount"

# Add team to project
penf projects add-team "TikTok FY26" --team "MTC Core"

# Add individual members
penf projects add-member "TikTok FY26" --person p_abc123 --role owner

# List projects
penf projects list

# Show project details
penf projects show "TikTok FY26"
# Output:
# Project: TikTok FY26
# Description: TikTok Q1 2026 renewal and pricing
# Jira: OUT (12 tickets, 3 open)
# Keywords: tiktok, fy26, discount
# Teams: MTC Core
# Members: Rick Eskelsen (owner), +5 from teams
# Activity: 89 emails, 23 threads, 8 decisions in last 30 days
```

---

## Auto-Tagging Content to Projects

The `ProjectMatcher` processor tags content based on:

```go
func (m *ProjectMatcher) Match(source *Source, enrichment *Enrichment) *string {
    // 1. Check Jira ticket references
    for _, link := range enrichment.Links {
        if link.Category == "jira_ticket" {
            if project := m.repo.GetProjectByJiraKey(link.ExternalID); project != nil {
                return &project.ID
            }
        }
    }

    // 2. Check keywords in subject/body
    text := source.Subject + " " + source.BodyText
    for _, project := range m.repo.GetProjectsWithKeywords() {
        for _, keyword := range project.Keywords {
            if strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
                return &project.ID
            }
        }
    }

    // 3. Check participant overlap with project teams
    projectMembers := m.repo.GetProjectMemberIDs(projectID)
    participantIDs := extractPersonIDs(enrichment.Participants)
    overlap := intersect(projectMembers, participantIDs)

    // If >50% of participants are project members, tag it
    if float64(len(overlap))/float64(len(participantIDs)) > 0.5 {
        return &project.ID
    }

    return nil  // No project match
}
```

---

## Edge Cases

| Edge Case | Handling |
|-----------|----------|
| Email address doesn't match any known person | Auto-create person with `confidence=low`, queue for review |
| Same person has conflicting display names | Use most recent, store all as aliases with `source` and `discovered_at` |
| Distribution lists vs individuals | Detect via naming patterns (`team-*`, `all-*`), flag as `account_type=distribution` |
| External Google notification mentioning internal user | Extract @mention from body, resolve to internal person, flag sender as `external_service` |
| System account sends on behalf of person | Use `X-MS-Exchange-Organization-AuthSource` or `Sender` header to identify real sender |
| Forwarded email attribution | Parse "From:" lines in quoted content, link to `forwarded_from_person_id` |

---

## Functional Requirements

### Entity Resolution

- **FR-100**: System MUST maintain canonical `people` table with unique person records
- **FR-101**: System MUST resolve email addresses to person records
- **FR-102**: System MUST resolve display names to person records with fuzzy matching
- **FR-103**: System MUST support multiple identifiers per person (email, Slack ID, aliases)
- **FR-104**: System MUST distinguish internal vs external participants based on domain patterns
- **FR-105**: System MUST track confidence score for entity resolution matches
- **FR-106**: System SHOULD suggest person merges when high-confidence duplicates detected
- **FR-107**: System MUST support manual entity resolution corrections
- **FR-108**: System MUST propagate entity resolution to historical content when person record updated
- **FR-109**: System MUST distinguish between person accounts vs system/role accounts
- **FR-110**: System MUST detect and flag distribution lists vs individual recipients
- **FR-111**: System MUST use Exchange header `X-MS-Exchange-Organization-AuthAs` for internal/external detection when available
- **FR-112**: System MUST flag external notification senders as system accounts

### Team/Project Resolution

- **FR-200**: System MUST maintain `teams` table with membership relationships
- **FR-201**: System MUST maintain `projects` table with associated keywords and people
- **FR-202**: System MUST auto-tag content with teams when majority of participants are team members
- **FR-203**: System MUST link Jira tickets to projects based on configuration
- **FR-204**: System SHOULD infer team membership from communication patterns

### Initial Data Load

- **FR-800**: System MUST auto-create person records when unknown email encountered
- **FR-801**: System MUST assign confidence score to auto-created records (default 0.6)
- **FR-802**: System MUST flag auto-created records for review
- **FR-803**: System MUST detect potential duplicates by name similarity
- **FR-804**: System MUST normalize display names (handle "Last, First" format)
- **FR-805**: System MUST support manual person review via CLI
- **FR-806**: System MUST support merging duplicate person records
- **FR-807**: System MUST support manual team definition via CLI
- **FR-808**: System MUST support manual project definition with Jira linking
- **FR-809**: System MUST auto-tag content to projects by Jira reference, keywords, or participant overlap

### Non-Functional Requirements

- **NFR-001**: Entity resolution MUST complete within 100ms for single content item
- **NFR-005**: Entity resolution database MUST support 100,000+ person records efficiently
