# Test Fixtures Guide

How to create, modify, and use test fixtures in Penfold.

## Overview

Test fixtures provide realistic, consistent test data. The primary fixture set is **Acme Corp**, a mock organization with people, teams, projects, products, and glossary terms.

## Fixture Location

```
tests/fixtures/
└── acme-corp/
    ├── people.yaml      # 20 employees
    ├── teams.yaml       # 7 teams
    ├── projects.yaml    # 10 projects
    ├── products.yaml    # Products
    ├── glossary.yaml    # 50+ business terms
    ├── emails/          # Sample emails (RFC 5322)
    └── meetings/        # Sample meeting transcripts
```

## Using Fixtures in Tests

### Loading All Fixtures

```go
func TestWithFixtures(t *testing.T) {
    db := SetupTestDB(t)

    // Load all Acme Corp fixtures in dependency order
    loader := db.FixtureLoader()
    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)
    require.NoError(t, err)

    // Your test code here...
}
```

### Using the Integration Helper

```go
func TestCLI_GlossaryList(t *testing.T) {
    db := SetupTestDB(t)
    EnsureAcmeCorpFixtures(t, db)  // Loads fixtures once per test run

    stdout, stderr, err := runCLI(t, "glossary", "list")
    require.NoError(t, err)
    assert.Contains(t, stdout, "TER")  // Known term from fixtures
}
```

### Loading Individual Fixture Types

```go
func TestTeamsOnly(t *testing.T) {
    db := SetupTestDB(t)
    loader := db.FixtureLoader()
    ctx := context.Background()

    // Load in dependency order
    err := loader.LoadTeamsWithoutLeads(ctx)
    require.NoError(t, err)

    err = loader.LoadPeople(ctx)
    require.NoError(t, err)
}
```

### Custom Tenant ID

```go
func TestMultiTenant(t *testing.T) {
    db := SetupTestDB(t)

    // Load fixtures for a specific tenant
    customTenantID := "custom-tenant-uuid"
    loader := testfixtures.NewLoaderWithTenant(
        db.Pool,
        "tests/fixtures/acme-corp",
        customTenantID,
    )

    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)
    require.NoError(t, err)
}
```

## Fixture File Formats

### people.yaml

```yaml
# Acme Corp People Fixtures
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
    manager_id: null  # Top-level, no manager

  - id: 2
    canonical_name: "Sarah Chen"
    email: "sarah.chen@acme.com"
    aliases:
      - "Sarah"
      - "SC"
    title: "Product Manager"
    team_id: 2
    manager_id: 5  # Reports to David Park
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | int64 | Yes | Unique identifier |
| `canonical_name` | string | Yes | Full name |
| `email` | string | Yes | Primary email |
| `aliases` | []string | No | Alternative names/nicknames |
| `title` | string | No | Job title |
| `team_id` | int64 | No | Team membership |
| `manager_id` | *int64 | No | Manager's person ID (null if none) |

### teams.yaml

```yaml
teams:
  - id: 1
    name: "Engineering"
    description: "Core engineering team"
    lead_id: 1  # John Smith

  - id: 2
    name: "Product"
    description: "Product management"
    lead_id: 5  # David Park
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | int64 | Yes | Unique identifier |
| `name` | string | Yes | Team name |
| `description` | string | No | Team description |
| `lead_id` | *int64 | No | Team lead's person ID |

### projects.yaml

```yaml
projects:
  - id: 1
    name: "Project Alpha"
    slug: "project-alpha"
    description: "Main product initiative"
    status: "active"
    owner_id: 2  # Sarah Chen
    team_id: 1   # Engineering

  - id: 2
    name: "Widget Refresh"
    slug: "widget-refresh"
    description: "UI modernization project"
    status: "planning"
    owner_id: 5
    team_id: 2
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | int64 | Yes | Unique identifier |
| `name` | string | Yes | Project name |
| `slug` | string | No | URL-safe identifier |
| `description` | string | No | Project description |
| `status` | string | No | active, planning, completed |
| `owner_id` | int64 | No | Project owner's person ID |
| `team_id` | int64 | No | Owning team ID |

### products.yaml

```yaml
products:
  - id: 1
    name: "Enterprise Suite"
    slug: "enterprise-suite"
    description: "Full-featured enterprise product"
    team_id: 1

  - id: 2
    name: "Widget Pro"
    slug: "widget-pro"
    description: "Professional widget solution"
    team_id: null  # Cross-team product
```

### glossary.yaml

```yaml
terms:
  # Simple acronym
  - term: "TER"
    expansion: "Technical Execution Review"
    definition: "Weekly engineering sync to review technical progress"
    context: "meeting"
    aliases:
      - "Tech Review"
      - "T.E.R."

  # Linked to entity
  - term: "Project Alpha"
    expansion: null
    definition: "Primary Q1 initiative"
    context: "project"
    linked_entity_type: "project"
    linked_entity_id: 1

  # Multi-context term
  - term: "DBaaS"
    expansion: "Database as a Service"
    definition: "Managed database platform offering"
    context: "infrastructure, product"
    aliases:
      - "Database as a Service"
```

**Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `term` | string | Yes | The acronym or term |
| `expansion` | *string | No | Full form (null for proper nouns) |
| `definition` | *string | No | Explanation |
| `context` | *string | No | Comma-separated contexts |
| `aliases` | []string | No | Alternative forms |
| `linked_entity_type` | *string | No | product, project, company |
| `linked_entity_id` | *int64 | No | ID of linked entity |

## Creating New Fixtures

### Step 1: Plan the Data

Consider:
- What entities are needed for your test scenario?
- What relationships exist between entities?
- What edge cases should be covered?

### Step 2: Create YAML File

```yaml
# tests/fixtures/acme-corp/new-entity.yaml
new_entities:
  - id: 1
    name: "Example"
    description: "Test entity"
```

### Step 3: Add Type Definition

In `pkg/testfixtures/types.go`:

```go
type NewEntityFixture struct {
    ID          int64   `yaml:"id"`
    Name        string  `yaml:"name"`
    Description *string `yaml:"description"`
}

type NewEntitiesFile struct {
    Entities []NewEntityFixture `yaml:"new_entities"`
}
```

### Step 4: Add Loader Method

In `pkg/testfixtures/loader.go`:

```go
func (l *Loader) LoadNewEntities(ctx context.Context) error {
    path := filepath.Join(l.fixtureDir, "new-entity.yaml")
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("read new-entity.yaml: %w", err)
    }

    var file NewEntitiesFile
    if err := yaml.Unmarshal(data, &file); err != nil {
        return fmt.Errorf("unmarshal new-entity.yaml: %w", err)
    }

    for _, entity := range file.Entities {
        _, err := l.db.Exec(ctx, `
            INSERT INTO new_entities (id, tenant_id, name, description)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (id) DO UPDATE SET
                name = EXCLUDED.name,
                description = EXCLUDED.description
        `, entity.ID, l.tenantID, entity.Name, entity.Description)
        if err != nil {
            return fmt.Errorf("insert entity %s: %w", entity.Name, err)
        }
    }

    return nil
}
```

### Step 5: Update LoadAcmeCorp (if appropriate)

```go
func (l *Loader) LoadAcmeCorp(ctx context.Context) error {
    // ... existing loads ...

    if err := l.LoadNewEntities(ctx); err != nil {
        return fmt.Errorf("load new entities: %w", err)
    }

    return nil
}
```

## Modifying Existing Fixtures

### Adding New People

1. Edit `tests/fixtures/acme-corp/people.yaml`
2. Add new person with unique ID
3. Ensure team_id and manager_id reference existing entities

```yaml
  - id: 21  # Next available ID
    canonical_name: "New Person"
    email: "new.person@acme.com"
    aliases:
      - "New"
      - "NP"
    title: "Software Engineer"
    team_id: 1
    manager_id: 6
```

### Adding New Glossary Terms

1. Edit `tests/fixtures/acme-corp/glossary.yaml`
2. Add term with appropriate context

```yaml
  - term: "NEWTERM"
    expansion: "New Term Expansion"
    definition: "What this term means"
    context: "meeting"
    aliases:
      - "NT"
```

## Fixture Validation

CI validates fixtures automatically via `TestYAML`:

```bash
# Run fixture validation locally
go test -v -run TestYAML ./pkg/testfixtures/...
```

### Manual Validation

```bash
# Check YAML syntax
python3 -c "import yaml; yaml.safe_load(open('tests/fixtures/acme-corp/people.yaml'))"

# Verify IDs are unique
grep "id:" tests/fixtures/acme-corp/people.yaml | sort | uniq -d
```

## Best Practices

### 1. Use Realistic Data

```yaml
# Good: Realistic names and emails
- canonical_name: "Sarah Chen"
  email: "sarah.chen@acme.com"

# Bad: Test placeholders
- canonical_name: "Test User 1"
  email: "test1@test.com"
```

### 2. Maintain Referential Integrity

```yaml
# Person references existing team
- id: 10
  canonical_name: "New Employee"
  team_id: 1  # Must exist in teams.yaml
  manager_id: 6  # Must exist in people.yaml
```

### 3. Use Sequential IDs

```yaml
# Good: Sequential, easy to track
- id: 1
- id: 2
- id: 3

# Bad: Random gaps
- id: 100
- id: 5
- id: 999
```

### 4. Document Special Cases

```yaml
# Executive with no manager (top of hierarchy)
- id: 1
  canonical_name: "John Smith"
  manager_id: null  # CEO, no manager

# Cross-functional role (multiple team contexts)
- id: 15
  canonical_name: "Alex Kim"
  title: "DevOps Engineer"
  team_id: 4  # Primary team
  # Note: Also works with teams 1, 5 via team_members table
```

### 5. Cover Edge Cases

Include fixtures that test:
- Null/empty optional fields
- Maximum length strings
- Special characters in names
- Circular references (where valid)

## Cleanup

Tests should clean up after themselves:

```go
func TestWithCleanup(t *testing.T) {
    db := SetupTestDB(t)
    loader := db.FixtureLoader()

    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)
    require.NoError(t, err)

    t.Cleanup(func() {
        loader.CleanupAll(ctx)
    })

    // Test code...
}
```

The loader's `CleanupAll` method removes all data for the loader's tenant only, preserving other tenants' data.
