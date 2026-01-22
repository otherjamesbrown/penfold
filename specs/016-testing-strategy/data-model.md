# Data Model: Testing Strategy Fixtures

**Feature**: 016-testing-strategy
**Date**: 2026-01-22

## Overview

This document defines the YAML schemas for the "Acme Corp" mock organization fixtures used in E2E tests.

## Fixture Schemas

### People (`people.yaml`)

```yaml
# tests/fixtures/acme-corp/people.yaml
people:
  - id: 1
    canonical_name: "John Smith"
    email: "john.smith@acme.com"
    aliases:
      - "John"
      - "JS"
      - "Smith"
    title: "VP Engineering"
    team_id: 1
    manager_id: null  # Top-level

  - id: 2
    canonical_name: "Sarah Chen"
    email: "sarah.chen@acme.com"
    aliases:
      - "Sarah"
      - "SC"
    title: "Product Manager"
    team_id: 2
    manager_id: 5

  # ... 20+ people total
```

**Go Struct**:
```go
type PersonFixture struct {
    ID            int64    `yaml:"id"`
    CanonicalName string   `yaml:"canonical_name"`
    Email         string   `yaml:"email"`
    Aliases       []string `yaml:"aliases"`
    Title         string   `yaml:"title"`
    TeamID        int64    `yaml:"team_id"`
    ManagerID     *int64   `yaml:"manager_id"`
}
```

---

### Teams (`teams.yaml`)

```yaml
# tests/fixtures/acme-corp/teams.yaml
teams:
  - id: 1
    name: "Engineering"
    slug: "engineering"
    parent_id: null
    lead_id: 1  # John Smith

  - id: 2
    name: "Product"
    slug: "product"
    parent_id: null
    lead_id: 5

  - id: 3
    name: "Sales"
    slug: "sales"
    parent_id: null
    lead_id: 3

  - id: 4
    name: "Platform Team"
    slug: "platform"
    parent_id: 1  # Under Engineering
    lead_id: 6

  # ... 5+ teams total
```

**Go Struct**:
```go
type TeamFixture struct {
    ID       int64  `yaml:"id"`
    Name     string `yaml:"name"`
    Slug     string `yaml:"slug"`
    ParentID *int64 `yaml:"parent_id"`
    LeadID   int64  `yaml:"lead_id"`
}
```

---

### Projects (`projects.yaml`)

```yaml
# tests/fixtures/acme-corp/projects.yaml
projects:
  - id: 1
    name: "Project Alpha"
    slug: "project-alpha"
    description: "Q1 platform migration initiative"
    status: "active"
    owner_id: 1  # John Smith
    team_id: 4   # Platform Team
    start_date: "2026-01-01"
    target_date: "2026-03-31"

  - id: 2
    name: "Customer Portal Redesign"
    slug: "portal-redesign"
    description: "UX overhaul for customer dashboard"
    status: "active"
    owner_id: 2  # Sarah Chen
    team_id: 2
    start_date: "2026-01-15"
    target_date: "2026-04-30"

  # ... 10+ projects total
```

**Go Struct**:
```go
type ProjectFixture struct {
    ID          int64  `yaml:"id"`
    Name        string `yaml:"name"`
    Slug        string `yaml:"slug"`
    Description string `yaml:"description"`
    Status      string `yaml:"status"`
    OwnerID     int64  `yaml:"owner_id"`
    TeamID      int64  `yaml:"team_id"`
    StartDate   string `yaml:"start_date"`
    TargetDate  string `yaml:"target_date"`
}
```

---

### Products (`products.yaml`)

```yaml
# tests/fixtures/acme-corp/products.yaml
products:
  - id: 1
    name: "Widget Pro"
    slug: "widget-pro"
    description: "Enterprise widget management platform"
    team_id: 1

  - id: 2
    name: "Enterprise Suite"
    slug: "enterprise-suite"
    description: "Bundled enterprise solutions"
    team_id: null  # Cross-team

  # ... products as needed
```

**Go Struct**:
```go
type ProductFixture struct {
    ID          int64  `yaml:"id"`
    Name        string `yaml:"name"`
    Slug        string `yaml:"slug"`
    Description string `yaml:"description"`
    TeamID      *int64 `yaml:"team_id"`
}
```

---

### Glossary (`glossary.yaml`)

```yaml
# tests/fixtures/acme-corp/glossary.yaml
terms:
  - term: "TER"
    expansion: "Technical Execution Review"
    definition: "Weekly engineering review meeting"
    context: "meeting"
    aliases:
      - "T.E.R."
      - "Tech Exec Review"

  - term: "MVP"
    expansion: "Minimum Viable Product"
    definition: null
    context: null
    aliases: []

  - term: "OKR"
    expansion: "Objectives and Key Results"
    definition: "Goal-setting framework"
    context: null
    aliases:
      - "OKRs"

  - term: "Project Alpha"
    expansion: null
    definition: "Q1 platform migration initiative"
    context: "project"
    linked_entity_type: "project"
    linked_entity_id: 1

  # ... 50+ terms total
```

**Go Struct**:
```go
type GlossaryTermFixture struct {
    Term             string   `yaml:"term"`
    Expansion        *string  `yaml:"expansion"`
    Definition       *string  `yaml:"definition"`
    Context          *string  `yaml:"context"`
    Aliases          []string `yaml:"aliases"`
    LinkedEntityType *string  `yaml:"linked_entity_type"`
    LinkedEntityID   *int64   `yaml:"linked_entity_id"`
}
```

---

### Emails (`emails/*.eml`)

Sample email files in standard RFC 5322 format:

```
# tests/fixtures/acme-corp/emails/project-update.eml
From: john.smith@acme.com
To: sarah.chen@acme.com
Cc: marcus.r@acme.com
Subject: RE: Project Alpha MVP Status
Date: Mon, 20 Jan 2026 09:15:00 -0800
Message-ID: <test-001@acme.com>

Hi Sarah,

Quick update on Project Alpha - we discussed the MVP timeline at TER yesterday.
Marcus mentioned some concerns about the Q1 deadline.

Can we sync with the team on Thursday?

Thanks,
John
```

**Test Expectations for this email**:
| Assertion | Expected Value |
|-----------|----------------|
| Sender resolved | person_id: 1 (John Smith) |
| Recipients resolved | person_id: 2 (Sarah Chen), 3 (Marcus Rodriguez) |
| Mentions in body | "Sarah", "Marcus" |
| Glossary matches | "MVP" → Minimum Viable Product, "TER" → Technical Execution Review |
| Project reference | "Project Alpha" → project_id: 1 |

---

## Entity Relationships

```
┌─────────┐     manages      ┌─────────┐
│ Person  │─────────────────▶│ Person  │
└────┬────┘                  └─────────┘
     │
     │ member_of
     ▼
┌─────────┐     parent_of    ┌─────────┐
│  Team   │◀────────────────▶│  Team   │
└────┬────┘                  └─────────┘
     │
     │ owns
     ▼
┌─────────┐
│ Project │
└─────────┘

┌─────────┐     linked_to    ┌─────────────────┐
│Glossary │─────────────────▶│ Person/Team/    │
│  Term   │                  │ Project/Product │
└─────────┘                  └─────────────────┘
```

---

## Database Tables Populated

When `LoadAcmeCorp()` is called, the following tables are populated:

| Table | Source Fixture | Row Count |
|-------|----------------|-----------|
| `people` | people.yaml | 20+ |
| `teams` | teams.yaml | 5+ |
| `projects` | projects.yaml | 10+ |
| `products` | products.yaml | 3+ |
| `glossary_terms` | glossary.yaml | 50+ |
| `emails` | emails/*.eml | 10+ |

---

## Fixture Loader Interface

```go
type FixtureLoader interface {
    // Load all Acme Corp fixtures
    LoadAcmeCorp(ctx context.Context) error

    // Load specific fixture types
    LoadPeople(ctx context.Context, path string) error
    LoadTeams(ctx context.Context, path string) error
    LoadProjects(ctx context.Context, path string) error
    LoadGlossary(ctx context.Context, path string) error
    LoadEmails(ctx context.Context, dir string) error

    // Cleanup
    TruncateAll(ctx context.Context) error
}
```
