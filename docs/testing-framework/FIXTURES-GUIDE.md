# Test Fixtures

## Location

```
tests/fixtures/acme-corp/
├── people.yaml     # 20 employees
├── teams.yaml      # 7 teams
├── projects.yaml   # 10 projects
├── products.yaml   # Products
├── glossary.yaml   # 50+ terms
├── emails/         # Sample emails
└── meetings/       # Transcripts
```

## Loading Fixtures

```go
// Standard pattern - loads once per test run
func TestSomething(t *testing.T) {
    db := SetupTestDB(t)
    EnsureAcmeCorpFixtures(t, db)

    // Test code using fixture data...
}

// Manual loading
func TestManual(t *testing.T) {
    db := SetupTestDB(t)
    loader := db.FixtureLoader()
    ctx := context.Background()

    err := loader.LoadAcmeCorp(ctx)  // All fixtures
    // Or individually:
    // loader.LoadTeamsWithoutLeads(ctx)
    // loader.LoadPeople(ctx)
    // loader.LoadProjects(ctx)
    // loader.LoadProducts(ctx)
    // loader.LoadGlossary(ctx)
}

// Custom tenant
loader := testfixtures.NewLoaderWithTenant(db.Pool, "tests/fixtures/acme-corp", "custom-tenant-id")
```

## YAML Schemas

### people.yaml
```yaml
people:
  - id: 1                           # int64, required, unique
    canonical_name: "John Smith"    # string, required
    email: "john.smith@acme.com"    # string, required
    aliases: ["John", "JS"]         # []string, optional
    title: "VP Engineering"         # string, optional
    team_id: 1                      # int64, optional
    manager_id: null                # *int64, optional (null = no manager)
```

### teams.yaml
```yaml
teams:
  - id: 1                          # int64, required, unique
    name: "Engineering"            # string, required
    description: "Core team"       # string, optional
    lead_id: 1                     # *int64, optional
```

### projects.yaml
```yaml
projects:
  - id: 1                          # int64, required, unique
    name: "Project Alpha"          # string, required
    slug: "project-alpha"          # string, optional
    description: "Main initiative" # string, optional
    status: "active"               # string, optional
    owner_id: 2                    # int64, optional
    team_id: 1                     # int64, optional
```

### glossary.yaml
```yaml
terms:
  - term: "TER"                           # string, required
    expansion: "Technical Execution Review" # *string, optional
    definition: "Weekly sync"             # *string, optional
    context: "meeting"                    # *string, optional (comma-separated)
    aliases: ["Tech Review"]              # []string, optional
    linked_entity_type: "project"         # *string, optional
    linked_entity_id: 1                   # *int64, optional
```

## Adding Fixture Types

1. Add type in `pkg/testfixtures/types.go`:
```go
type NewEntityFixture struct {
    ID   int64  `yaml:"id"`
    Name string `yaml:"name"`
}
type NewEntitiesFile struct {
    Entities []NewEntityFixture `yaml:"new_entities"`
}
```

2. Add loader in `pkg/testfixtures/loader.go`:
```go
func (l *Loader) LoadNewEntities(ctx context.Context) error {
    path := filepath.Join(l.fixtureDir, "new-entity.yaml")
    data, _ := os.ReadFile(path)
    var file NewEntitiesFile
    yaml.Unmarshal(data, &file)
    for _, e := range file.Entities {
        l.db.Exec(ctx, `INSERT INTO new_entities (id, tenant_id, name)
            VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`,
            e.ID, l.tenantID, e.Name)
    }
    return nil
}
```

## Cleanup

```go
t.Cleanup(func() {
    loader.CleanupAll(ctx)  // Removes only this tenant's data
})
```

## Known Fixture Data

| Entity | Count | Example IDs |
|--------|-------|-------------|
| People | 20 | 1=John Smith, 2=Sarah Chen |
| Teams | 7 | 1=Engineering, 2=Product |
| Projects | 10 | 1=Project Alpha |
| Glossary | 50+ | TER, PBR, DBaaS |
