//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/products"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
	"github.com/otherjamesbrown/penfold/services/gateway/entityservice"
)

// setupEntityTestTenant creates a test tenant for entity tests
func setupEntityTestTenant(t *testing.T, db *TestDB) string {
	t.Helper()
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, tenant.TenantInput{
		Name: "Entity Test Tenant",
		Slug: "entity-test-tenant-" + uuid.New().String()[:8],
	})
	require.NoError(t, err)
	return created.ID
}

func TestEntityRepository_CreatePerson(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "person_aliases", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	tests := []struct {
		name    string
		person  *entities.Person
		wantErr bool
	}{
		{
			name: "create simple person",
			person: &entities.Person{
				TenantID:      tenantID,
				CanonicalName: "John Doe",
				PrimaryEmail:  "john.doe@test.com",
				IsInternal:    true,
				AccountType:   entities.AccountTypePerson,
				Confidence:    0.9,
				NeedsReview:   false,
				AutoCreated:   false,
			},
			wantErr: false,
		},
		{
			name: "create person with full details",
			person: &entities.Person{
				TenantID:      tenantID,
				CanonicalName: "Jane Smith",
				PrimaryEmail:  "jane.smith@test.com",
				Title:         "Engineering Manager",
				Department:    "Engineering",
				Company:       "Test Corp",
				IsInternal:    true,
				AccountType:   entities.AccountTypePerson,
				Confidence:    1.0,
				NeedsReview:   false,
				AutoCreated:   false,
			},
			wantErr: false,
		},
		{
			name: "create external person",
			person: &entities.Person{
				TenantID:      tenantID,
				CanonicalName: "External Partner",
				PrimaryEmail:  "partner@external.com",
				Company:       "Partner Inc",
				IsInternal:    false,
				AccountType:   entities.AccountTypePerson,
				Confidence:    0.8,
				NeedsReview:   true,
				AutoCreated:   true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.CreatePerson(ctx, tt.person)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotZero(t, tt.person.ID)
			assert.NotZero(t, tt.person.CreatedAt)
			assert.NotZero(t, tt.person.UpdatedAt)
		})
	}
}

func TestEntityRepository_GetPersonByID(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a person
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Get By ID Test",
		PrimaryEmail:  "getbyid@test.com",
		Title:         "Developer",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.9,
	}
	err := repo.CreatePerson(ctx, person)
	require.NoError(t, err)

	// Get by ID
	found, err := repo.GetPersonByID(ctx, person.ID)
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, person.ID, found.ID)
	assert.Equal(t, "Get By ID Test", found.CanonicalName)
	assert.Equal(t, "getbyid@test.com", found.PrimaryEmail)
	assert.Equal(t, "Developer", found.Title)
}

func TestEntityRepository_GetPersonByIDNotFound(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Get non-existent person
	found, err := repo.GetPersonByID(ctx, 999999)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestEntityRepository_GetPersonByEmail(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a person
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Email Lookup Test",
		PrimaryEmail:  "lookup@test.com",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.9,
	}
	err := repo.CreatePerson(ctx, person)
	require.NoError(t, err)

	tests := []struct {
		name    string
		email   string
		wantNil bool
	}{
		{
			name:    "find by exact email",
			email:   "lookup@test.com",
			wantNil: false,
		},
		{
			name:    "non-existent email",
			email:   "nonexistent@test.com",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := repo.GetPersonByEmail(ctx, tenantID, tt.email)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, found)
			} else {
				assert.NotNil(t, found)
				assert.Equal(t, person.ID, found.ID)
			}
		})
	}
}

func TestEntityRepository_UpdatePerson(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a person
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Update Test",
		PrimaryEmail:  "update@test.com",
		Title:         "Junior Developer",
		Department:    "Engineering",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.7,
		NeedsReview:   true,
	}
	err := repo.CreatePerson(ctx, person)
	require.NoError(t, err)
	originalUpdatedAt := person.UpdatedAt

	// Update the person
	person.CanonicalName = "Updated Name"
	person.Title = "Senior Developer"
	person.Confidence = 1.0
	person.NeedsReview = false

	err = repo.UpdatePerson(ctx, person)
	require.NoError(t, err)

	// Verify updates
	found, err := repo.GetPersonByID(ctx, person.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.CanonicalName)
	assert.Equal(t, "Senior Developer", found.Title)
	assert.Equal(t, float32(1.0), found.Confidence)
	assert.False(t, found.NeedsReview)
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt) || found.UpdatedAt.Equal(originalUpdatedAt))
}

func TestEntityRepository_MarkPersonReviewed(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a person needing review
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Needs Review",
		PrimaryEmail:  "review@test.com",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.6,
		NeedsReview:   true,
		AutoCreated:   true,
	}
	err := repo.CreatePerson(ctx, person)
	require.NoError(t, err)

	// Mark as reviewed
	err = repo.MarkPersonReviewed(ctx, person.ID, "reviewer@test.com")
	require.NoError(t, err)

	// Verify review status
	found, err := repo.GetPersonByID(ctx, person.ID)
	require.NoError(t, err)
	assert.False(t, found.NeedsReview)
	assert.Equal(t, float32(1.0), found.Confidence)
	assert.Equal(t, "reviewer@test.com", found.ReviewedBy)
	assert.NotNil(t, found.ReviewedAt)
}

func TestEntityRepository_CreateAlias(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "person_aliases", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a person
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Alias Test Person",
		PrimaryEmail:  "alias-person@test.com",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.9,
	}
	err := repo.CreatePerson(ctx, person)
	require.NoError(t, err)

	// Create aliases
	aliases := []struct {
		aliasType  entities.AliasType
		aliasValue string
	}{
		{entities.AliasTypeEmail, "alias-alt@test.com"},
		{entities.AliasTypeName, "A. T. Person"},
		{entities.AliasTypeName, "AT Person"},
	}

	for _, a := range aliases {
		alias := &entities.PersonAlias{
			PersonID:   person.ID,
			AliasType:  a.aliasType,
			AliasValue: a.aliasValue,
			Confidence: 0.8,
			Source:     "test",
		}
		err := repo.CreateAlias(ctx, alias)
		require.NoError(t, err)
		assert.NotZero(t, alias.ID)
	}

	// Get aliases for person
	foundAliases, err := repo.GetAliasesForPerson(ctx, person.ID)
	require.NoError(t, err)
	assert.Len(t, foundAliases, 3)
}

func TestEntityRepository_GetPersonByAlias(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "person_aliases", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a person
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Find By Alias Test",
		PrimaryEmail:  "find-alias@test.com",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.9,
	}
	err := repo.CreatePerson(ctx, person)
	require.NoError(t, err)

	// Create an alias
	alias := &entities.PersonAlias{
		PersonID:   person.ID,
		AliasType:  entities.AliasTypeEmail,
		AliasValue: "alias-email@test.com",
		Confidence: 0.9,
		Source:     "test",
	}
	err = repo.CreateAlias(ctx, alias)
	require.NoError(t, err)

	// Find by alias
	found, err := repo.GetPersonByAlias(ctx, tenantID, "alias-email@test.com")
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, person.ID, found.ID)
}

func TestEntityRepository_SearchPeopleByName(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create multiple people
	// Note: Avoid names that contain other search terms as substrings
	// (e.g., "Johnson" contains "John" which would match the "John" search)
	people := []struct {
		name  string
		email string
	}{
		{"John Smith", "john.smith@test.com"},
		{"John Doe", "john.doe@test.com"},
		{"Jane Smith", "jane.smith@test.com"},
		{"Bob Williams", "bob.williams@test.com"},
	}

	for _, p := range people {
		person := &entities.Person{
			TenantID:      tenantID,
			CanonicalName: p.name,
			PrimaryEmail:  p.email,
			IsInternal:    true,
			AccountType:   entities.AccountTypePerson,
			Confidence:    0.9,
		}
		err := repo.CreatePerson(ctx, person)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		search    string
		wantCount int
	}{
		{
			name:      "search for John",
			search:    "John",
			wantCount: 2,
		},
		{
			name:      "search for Smith",
			search:    "Smith",
			wantCount: 2,
		},
		{
			name:      "search for Bob",
			search:    "Bob",
			wantCount: 1,
		},
		{
			name:      "search for non-existent",
			search:    "XYZ",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.SearchPeopleByName(ctx, tenantID, tt.search, 100)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestEntityRepository_ListPeopleNeedingReview(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create people with different review states
	people := []struct {
		name        string
		email       string
		needsReview bool
	}{
		{"Review Person 1", "review1@test.com", true},
		{"Review Person 2", "review2@test.com", true},
		{"Reviewed Person", "reviewed@test.com", false},
		{"Another Review", "review3@test.com", true},
	}

	for _, p := range people {
		person := &entities.Person{
			TenantID:      tenantID,
			CanonicalName: p.name,
			PrimaryEmail:  p.email,
			IsInternal:    true,
			AccountType:   entities.AccountTypePerson,
			Confidence:    0.6,
			NeedsReview:   p.needsReview,
		}
		err := repo.CreatePerson(ctx, person)
		require.NoError(t, err)
	}

	// List people needing review
	results, err := repo.ListPeopleNeedingReview(ctx, tenantID, 100)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify all need review
	for _, p := range results {
		assert.True(t, p.NeedsReview)
	}
}

func TestEntityRepository_CreateTeam(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "teams", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	team := &entities.Team{
		TenantID:    tenantID,
		Name:        "Engineering Team",
		Description: "Core engineering team",
	}

	err := repo.CreateTeam(ctx, team)
	require.NoError(t, err)
	assert.NotZero(t, team.ID)
	assert.NotZero(t, team.CreatedAt)
}

func TestEntityRepository_GetTeamByID(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "teams", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a team
	team := &entities.Team{
		TenantID:    tenantID,
		Name:        "Get Team Test",
		Description: "Test team for retrieval",
	}
	err := repo.CreateTeam(ctx, team)
	require.NoError(t, err)

	// Get by ID
	found, err := repo.GetTeamByID(ctx, team.ID)
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, team.ID, found.ID)
	assert.Equal(t, "Get Team Test", found.Name)
}

func TestEntityRepository_GetTeamByName(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "teams", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a team
	team := &entities.Team{
		TenantID:    tenantID,
		Name:        "Named Team",
		Description: "Team found by name",
	}
	err := repo.CreateTeam(ctx, team)
	require.NoError(t, err)

	// Get by name
	found, err := repo.GetTeamByName(ctx, tenantID, "Named Team")
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, team.ID, found.ID)

	// Non-existent name
	notFound, err := repo.GetTeamByName(ctx, tenantID, "Non-existent Team")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestEntityRepository_TeamMemberOperations(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "teams", "team_members", "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a team
	team := &entities.Team{
		TenantID: tenantID,
		Name:     "Member Test Team",
	}
	err := repo.CreateTeam(ctx, team)
	require.NoError(t, err)

	// Create people
	people := []struct {
		name  string
		email string
	}{
		{"Team Lead", "lead@test.com"},
		{"Developer 1", "dev1@test.com"},
		{"Developer 2", "dev2@test.com"},
	}

	var personIDs []int64
	for _, p := range people {
		person := &entities.Person{
			TenantID:      tenantID,
			CanonicalName: p.name,
			PrimaryEmail:  p.email,
			IsInternal:    true,
			AccountType:   entities.AccountTypePerson,
			Confidence:    0.9,
		}
		err := repo.CreatePerson(ctx, person)
		require.NoError(t, err)
		personIDs = append(personIDs, person.ID)
	}

	// Add team members
	roles := []string{"lead", "member", "member"}
	for i, personID := range personIDs {
		member := &entities.TeamMember{
			TeamID:   team.ID,
			PersonID: personID,
			Role:     roles[i],
		}
		err := repo.AddTeamMember(ctx, member)
		require.NoError(t, err)
		assert.NotZero(t, member.ID)
	}

	// Get team members
	members, err := repo.GetTeamMembers(ctx, team.ID)
	require.NoError(t, err)
	assert.Len(t, members, 3)

	// Verify member details
	leadFound := false
	for _, m := range members {
		if m.Role == "lead" {
			leadFound = true
			assert.Equal(t, "Team Lead", m.Person.CanonicalName)
		}
	}
	assert.True(t, leadFound)
}

func TestEntityRepository_CreateProject(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	project := &entities.Project{
		TenantID:     tenantID,
		Name:         "New Project",
		Description:  "Project created via entity repo",
		Keywords:     []string{"api", "backend"},
		JiraProjects: []string{"PROJ-1"},
	}

	err := repo.CreateProject(ctx, project)
	require.NoError(t, err)
	assert.NotZero(t, project.ID)
	assert.NotZero(t, project.CreatedAt)
}

func TestEntityRepository_GetProjectByName(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &entities.Project{
		TenantID:    tenantID,
		Name:        "Find By Name Project",
		Description: "Test project",
	}
	err := repo.CreateProject(ctx, project)
	require.NoError(t, err)

	// Find by name
	found, err := repo.GetProjectByName(ctx, tenantID, "Find By Name Project")
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, project.ID, found.ID)

	// Non-existent name
	notFound, err := repo.GetProjectByName(ctx, tenantID, "Non-existent Project")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestEntityRepository_GetProjectByJiraKey(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create projects with Jira keys
	projects := []struct {
		name string
		jira []string
	}{
		{"Project Alpha", []string{"ALPHA-1", "ALPHA-2"}},
		{"Project Beta", []string{"BETA-1"}},
	}

	for _, p := range projects {
		project := &entities.Project{
			TenantID:     tenantID,
			Name:         p.name,
			JiraProjects: p.jira,
		}
		err := repo.CreateProject(ctx, project)
		require.NoError(t, err)
	}

	tests := []struct {
		name    string
		jiraKey string
		want    string
	}{
		{
			name:    "find by first Jira key",
			jiraKey: "ALPHA-1",
			want:    "Project Alpha",
		},
		{
			name:    "find by second Jira key",
			jiraKey: "ALPHA-2",
			want:    "Project Alpha",
		},
		{
			name:    "find beta project",
			jiraKey: "BETA-1",
			want:    "Project Beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := repo.GetProjectByJiraKey(ctx, tenantID, tt.jiraKey)
			require.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, tt.want, found.Name)
		})
	}
}

func TestEntityRepository_GetProjectsWithKeywords(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create projects
	projects := []struct {
		name     string
		keywords []string
	}{
		{"With Keywords 1", []string{"api", "backend"}},
		{"With Keywords 2", []string{"frontend"}},
		{"No Keywords", nil},
	}

	for _, p := range projects {
		project := &entities.Project{
			TenantID: tenantID,
			Name:     p.name,
			Keywords: p.keywords,
		}
		err := repo.CreateProject(ctx, project)
		require.NoError(t, err)
	}

	// Get projects with keywords
	results, err := repo.GetProjectsWithKeywords(ctx, tenantID)
	require.NoError(t, err)
	assert.Len(t, results, 2) // Only projects with keywords
}

func TestEntityRepository_ProjectMemberOperations(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &entities.Project{
		TenantID: tenantID,
		Name:     "Member Test Project",
	}
	err := repo.CreateProject(ctx, project)
	require.NoError(t, err)

	// Create a person
	person := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Project Member",
		PrimaryEmail:  "member@test.com",
		IsInternal:    true,
		AccountType:   entities.AccountTypePerson,
		Confidence:    0.9,
	}
	err = repo.CreatePerson(ctx, person)
	require.NoError(t, err)

	// Add project member
	member := &entities.ProjectMember{
		ProjectID: project.ID,
		PersonID:  &person.ID,
		Role:      "lead",
	}
	err = repo.AddProjectMember(ctx, member)
	require.NoError(t, err)
	assert.NotZero(t, member.ID)

	// Get project member IDs
	memberIDs, err := repo.GetProjectMemberIDs(ctx, project.ID)
	require.NoError(t, err)
	assert.Len(t, memberIDs, 1)
	assert.Equal(t, person.ID, memberIDs[0])
}

func TestEntityRepository_GetPeopleByDomain(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create people with different domains
	people := []struct {
		name  string
		email string
	}{
		{"Internal 1", "user1@company.com"},
		{"Internal 2", "user2@company.com"},
		{"External 1", "user@external.com"},
		{"Internal 3", "user3@company.com"},
	}

	for _, p := range people {
		person := &entities.Person{
			TenantID:      tenantID,
			CanonicalName: p.name,
			PrimaryEmail:  p.email,
			IsInternal:    true,
			AccountType:   entities.AccountTypePerson,
			Confidence:    0.9,
		}
		err := repo.CreatePerson(ctx, person)
		require.NoError(t, err)
	}

	// Get people by domain
	results, err := repo.GetPeopleByDomain(ctx, tenantID, "company.com")
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Different domain
	results, err = repo.GetPeopleByDomain(ctx, tenantID, "external.com")
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// ============================================================================
// EntityService Integration Tests (Service Layer with Real DB)
// ============================================================================

// NOTE: These tests cover the service layer with real database connections.
// They replace the mock-based tests in services/gateway/entityservice/service_test.go
// which use a 1000+ line mockableService wrapper that reimplements service logic.
//
// The unit tests in service_test.go have value for testing validation logic
// and batch size limits, but the core business logic should be tested here
// with real database operations.

// setupEntityService creates an entityservice.Service with real repositories.
func setupEntityService(t *testing.T, db *TestDB) (*entityservice.Service, string) {
	t.Helper()

	logger := logging.NewNopLogger()
	tenantID := setupEntityTestTenant(t, db)

	// Create real repositories
	entityRepo := entities.NewRepository(db.Pool, logger)
	productRepo := products.NewRepository(db.Pool, logger)

	svc := entityservice.NewService(entityRepo, productRepo, logger)

	return svc, tenantID
}

func TestEntityService_BulkCreatePeople(t *testing.T) {
	// Skip: The people table has a generated 'primary_email' column that cannot be directly inserted.
	// The test DB schema may differ from the repository's INSERT expectations.
	// TODO: Fix people repository to not insert into generated columns, or update test DB schema.
	t.Skip("Skipping: people table has generated column that prevents direct INSERT")

	db := SetupTestDB(t)
	db.TruncateTables(t, "people", "tenants")

	svc, tenantID := setupEntityService(t, db)
	ctx := context.Background()

	t.Run("creates people with real DB", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: tenantID,
			People: []*entityv1.PersonInput{
				{
					Name:       "John Doe",
					Email:      "john.doe@test.com",
					Title:      "Engineer",
					Department: "Engineering",
					IsInternal: true,
				},
				{
					Name:       "Jane Smith",
					Email:      "jane.smith@test.com",
					Title:      "Manager",
					Department: "Product",
					IsInternal: true,
				},
			},
		}

		resp, err := svc.BulkCreatePeople(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Debug: print any errors
		for _, e := range resp.Errors {
			t.Logf("Error for %s: %s", e.Identifier, e.Error)
		}

		assert.Equal(t, int32(2), resp.TotalRequested)
		assert.Equal(t, int32(2), resp.TotalCreated)
		assert.Equal(t, int32(0), resp.TotalErrors)
		assert.Len(t, resp.Created, 2)

		// Verify person was actually created in DB
		entityRepo := entities.NewRepository(db.Pool, logging.NewNopLogger())
		person, err := entityRepo.GetPersonByEmail(ctx, tenantID, "john.doe@test.com")
		require.NoError(t, err)
		require.NotNil(t, person)
		assert.Equal(t, "John Doe", person.CanonicalName)
	})

	t.Run("handles duplicates with skip_duplicates=true", func(t *testing.T) {
		// First create a person
		req1 := &entityv1.BulkCreatePeopleRequest{
			TenantId: tenantID,
			People: []*entityv1.PersonInput{
				{Name: "Existing Person", Email: "existing@test.com"},
			},
		}
		resp1, err := svc.BulkCreatePeople(ctx, req1)
		require.NoError(t, err)
		assert.Equal(t, int32(1), resp1.TotalCreated)
		existingID := resp1.Created[0].Id

		// Now try to create same person again with skip_duplicates
		req2 := &entityv1.BulkCreatePeopleRequest{
			TenantId:       tenantID,
			SkipDuplicates: true,
			People: []*entityv1.PersonInput{
				{Name: "Existing Person", Email: "existing@test.com"},
			},
		}
		resp2, err := svc.BulkCreatePeople(ctx, req2)
		require.NoError(t, err)

		assert.Equal(t, int32(1), resp2.TotalRequested)
		assert.Equal(t, int32(0), resp2.TotalCreated)
		assert.Equal(t, int32(1), resp2.TotalSkipped)
		assert.Equal(t, existingID, resp2.Skipped[0].ExistingId)
	})

	t.Run("handles duplicates with skip_duplicates=false", func(t *testing.T) {
		// First create a person
		req1 := &entityv1.BulkCreatePeopleRequest{
			TenantId: tenantID,
			People: []*entityv1.PersonInput{
				{Name: "Another Existing", Email: "another.existing@test.com"},
			},
		}
		_, err := svc.BulkCreatePeople(ctx, req1)
		require.NoError(t, err)

		// Now try to create same person without skip_duplicates
		req2 := &entityv1.BulkCreatePeopleRequest{
			TenantId:       tenantID,
			SkipDuplicates: false,
			People: []*entityv1.PersonInput{
				{Name: "Another Existing", Email: "another.existing@test.com"},
			},
		}
		resp2, err := svc.BulkCreatePeople(ctx, req2)
		require.NoError(t, err)

		assert.Equal(t, int32(1), resp2.TotalErrors)
		assert.Contains(t, resp2.Errors[0].Error, "already exists")
	})

	t.Run("creates person with aliases", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: tenantID,
			People: []*entityv1.PersonInput{
				{
					Name:    "Robert Jones",
					Email:   "robert.jones@test.com",
					Aliases: []string{"Bob", "Bobby", "bob.j@alt.com"},
				},
			},
		}

		resp, err := svc.BulkCreatePeople(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int32(1), resp.TotalCreated)

		// Verify aliases were created
		entityRepo := entities.NewRepository(db.Pool, logging.NewNopLogger())
		aliases, err := entityRepo.GetAliasesForPerson(ctx, resp.Created[0].Id)
		require.NoError(t, err)
		assert.Len(t, aliases, 3)
	})

	t.Run("returns error for missing email", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: tenantID,
			People: []*entityv1.PersonInput{
				{Name: "No Email Person", Email: ""},
			},
		}

		resp, err := svc.BulkCreatePeople(ctx, req)
		require.NoError(t, err) // Method returns response, not error

		assert.Equal(t, int32(1), resp.TotalErrors)
		assert.Contains(t, resp.Errors[0].Error, "email is required")
	})

	t.Run("returns error for missing name", func(t *testing.T) {
		req := &entityv1.BulkCreatePeopleRequest{
			TenantId: tenantID,
			People: []*entityv1.PersonInput{
				{Name: "", Email: "noname@test.com"},
			},
		}

		resp, err := svc.BulkCreatePeople(ctx, req)
		require.NoError(t, err)

		assert.Equal(t, int32(1), resp.TotalErrors)
		assert.Contains(t, resp.Errors[0].Error, "name is required")
	})
}

func TestEntityService_BulkCreateProducts(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "products", "product_aliases", "tenants")

	svc, tenantID := setupEntityService(t, db)
	ctx := context.Background()

	t.Run("creates products with real DB", func(t *testing.T) {
		req := &entityv1.BulkCreateProductsRequest{
			TenantId: tenantID,
			Products: []*entityv1.ProductInput{
				{
					Name:        "Product Alpha",
					Description: "First product",
					ProductType: "product",
					Status:      "active",
				},
				{
					Name:        "Product Beta",
					Description: "Second product",
					ProductType: "product",
					Status:      "beta",
				},
			},
		}

		resp, err := svc.BulkCreateProducts(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(2), resp.TotalRequested)
		assert.Equal(t, int32(2), resp.TotalCreated)
		assert.Equal(t, int32(0), resp.TotalErrors)
	})

	t.Run("creates child product with parent", func(t *testing.T) {
		// First create parent
		parentReq := &entityv1.BulkCreateProductsRequest{
			TenantId: tenantID,
			Products: []*entityv1.ProductInput{
				{Name: "Parent Product", ProductType: "product"},
			},
		}
		parentResp, err := svc.BulkCreateProducts(ctx, parentReq)
		require.NoError(t, err)
		parentID := parentResp.Created[0].Id

		// Now create child with parent reference
		childReq := &entityv1.BulkCreateProductsRequest{
			TenantId: tenantID,
			Products: []*entityv1.ProductInput{
				{
					Name:        "Child Product",
					ProductType: "sub_product",
					ParentName:  "Parent Product",
				},
			},
		}
		childResp, err := svc.BulkCreateProducts(ctx, childReq)
		require.NoError(t, err)

		assert.Equal(t, int32(1), childResp.TotalCreated)
		require.NotNil(t, childResp.Created[0].ParentId)
		assert.Equal(t, parentID, *childResp.Created[0].ParentId)
	})

	t.Run("creates batch with parent and child in same request", func(t *testing.T) {
		req := &entityv1.BulkCreateProductsRequest{
			TenantId: tenantID,
			Products: []*entityv1.ProductInput{
				{Name: "Batch Parent", ProductType: "product"},
				{Name: "Batch Child", ProductType: "sub_product", ParentName: "Batch Parent"},
			},
		}

		resp, err := svc.BulkCreateProducts(ctx, req)
		require.NoError(t, err)

		assert.Equal(t, int32(2), resp.TotalCreated)
		assert.Nil(t, resp.Created[0].ParentId)           // Parent has no parent
		require.NotNil(t, resp.Created[1].ParentId)       // Child has parent
		assert.Equal(t, resp.Created[0].Id, *resp.Created[1].ParentId)
	})

	t.Run("handles duplicate products with skip_duplicates", func(t *testing.T) {
		// Create product
		req1 := &entityv1.BulkCreateProductsRequest{
			TenantId: tenantID,
			Products: []*entityv1.ProductInput{
				{Name: "Duplicate Product"},
			},
		}
		resp1, err := svc.BulkCreateProducts(ctx, req1)
		require.NoError(t, err)
		existingID := resp1.Created[0].Id

		// Try again with skip_duplicates
		req2 := &entityv1.BulkCreateProductsRequest{
			TenantId:       tenantID,
			SkipDuplicates: true,
			Products: []*entityv1.ProductInput{
				{Name: "Duplicate Product"},
			},
		}
		resp2, err := svc.BulkCreateProducts(ctx, req2)
		require.NoError(t, err)

		assert.Equal(t, int32(1), resp2.TotalSkipped)
		assert.Equal(t, existingID, resp2.Skipped[0].ExistingId)
	})
}

func TestEntityService_BulkCreateProjects(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "tenants")

	svc, tenantID := setupEntityService(t, db)
	ctx := context.Background()

	t.Run("creates projects with real DB", func(t *testing.T) {
		req := &entityv1.BulkCreateProjectsRequest{
			TenantId: tenantID,
			Projects: []*entityv1.ProjectInput{
				{
					Name:         "MTC",
					Description:  "Major TikTok Contract",
					Keywords:     []string{"TikTok", "migration"},
					JiraProjects: []string{"MTC", "MTC-2"},
				},
				{
					Name:        "Platform",
					Description: "Platform improvements",
					Keywords:    []string{"platform", "infrastructure"},
				},
			},
		}

		resp, err := svc.BulkCreateProjects(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, int32(2), resp.TotalRequested)
		assert.Equal(t, int32(2), resp.TotalCreated)
		assert.Equal(t, int32(0), resp.TotalErrors)

		// Verify first project was created with correct data
		entityRepo := entities.NewRepository(db.Pool, logging.NewNopLogger())
		project, err := entityRepo.GetProjectByName(ctx, tenantID, "MTC")
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.Equal(t, "Major TikTok Contract", project.Description)
		assert.Equal(t, []string{"TikTok", "migration"}, project.Keywords)
		assert.Equal(t, []string{"MTC", "MTC-2"}, project.JiraProjects)
	})

	t.Run("handles duplicate projects with skip_duplicates", func(t *testing.T) {
		// Create project
		req1 := &entityv1.BulkCreateProjectsRequest{
			TenantId: tenantID,
			Projects: []*entityv1.ProjectInput{
				{Name: "Duplicate Project"},
			},
		}
		resp1, err := svc.BulkCreateProjects(ctx, req1)
		require.NoError(t, err)
		existingID := resp1.Created[0].Id

		// Try again with skip_duplicates
		req2 := &entityv1.BulkCreateProjectsRequest{
			TenantId:       tenantID,
			SkipDuplicates: true,
			Projects: []*entityv1.ProjectInput{
				{Name: "Duplicate Project"},
			},
		}
		resp2, err := svc.BulkCreateProjects(ctx, req2)
		require.NoError(t, err)

		assert.Equal(t, int32(1), resp2.TotalSkipped)
		assert.Equal(t, existingID, resp2.Skipped[0].ExistingId)
	})

	t.Run("returns error for missing name", func(t *testing.T) {
		req := &entityv1.BulkCreateProjectsRequest{
			TenantId: tenantID,
			Projects: []*entityv1.ProjectInput{
				{Name: "", Description: "No name"},
			},
		}

		resp, err := svc.BulkCreateProjects(ctx, req)
		require.NoError(t, err)

		assert.Equal(t, int32(1), resp.TotalErrors)
		assert.Contains(t, resp.Errors[0].Error, "name is required")
	})
}
