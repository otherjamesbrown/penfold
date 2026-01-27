//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/projects"
	"github.com/otherjamesbrown/penfold/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestTenant creates a test tenant and returns its ID
func setupTestTenant(t *testing.T, db *TestDB) string {
	t.Helper()
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	created, err := repo.Create(ctx, tenant.TenantInput{
		Name: "Project Test Tenant",
		Slug: "project-test-tenant",
	})
	require.NoError(t, err)
	return created.ID
}

func TestProjectRepository_Create(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	tests := []struct {
		name    string
		project *projects.Project
		wantErr bool
	}{
		{
			name: "create simple project",
			project: &projects.Project{
				TenantID: tenantID,
				Name:     "Project Alpha",
			},
			wantErr: false,
		},
		{
			name: "create project with description",
			project: &projects.Project{
				TenantID:    tenantID,
				Name:        "Project Beta",
				Description: strPtr("A comprehensive project for beta testing"),
			},
			wantErr: false,
		},
		{
			name: "create project with keywords",
			project: &projects.Project{
				TenantID: tenantID,
				Name:     "Project Gamma",
				Keywords: []string{"api", "backend", "critical"},
			},
			wantErr: false,
		},
		{
			name: "create project with jira keys",
			project: &projects.Project{
				TenantID:     tenantID,
				Name:         "Project Delta",
				JiraProjects: []string{"DELTA-1", "DELTA-2"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.project)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotZero(t, tt.project.ID)
			assert.NotZero(t, tt.project.CreatedAt)
			assert.NotZero(t, tt.project.UpdatedAt)
		})
	}
}

func TestProjectRepository_CreateDuplicate(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create first project
	project := &projects.Project{
		TenantID: tenantID,
		Name:     "Duplicate Project",
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	// Attempt to create duplicate
	duplicate := &projects.Project{
		TenantID: tenantID,
		Name:     "Duplicate Project",
	}
	err = repo.Create(ctx, duplicate)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestProjectRepository_GetByID(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID:    tenantID,
		Name:        "GetByID Test",
		Description: strPtr("Test project"),
		Keywords:    []string{"test"},
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	// Get by ID
	found, err := repo.GetByID(ctx, project.ID)
	require.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, project.ID, found.ID)
	assert.Equal(t, project.Name, found.Name)
	assert.Equal(t, *project.Description, *found.Description)
	assert.Equal(t, project.Keywords, found.Keywords)
}

func TestProjectRepository_GetByIDNotFound(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Attempt to get non-existent project
	_, err := repo.GetByID(ctx, 999999)
	assert.Error(t, err)
}

func TestProjectRepository_GetByName(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID: tenantID,
		Name:     "Named Project",
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	// Get by name (case-insensitive)
	tests := []struct {
		name     string
		lookup   string
		wantNil  bool
	}{
		{
			name:    "exact match",
			lookup:  "Named Project",
			wantNil: false,
		},
		{
			name:    "lowercase match",
			lookup:  "named project",
			wantNil: false,
		},
		{
			name:    "non-existent",
			lookup:  "Non-existent Project",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := repo.GetByName(ctx, tenantID, tt.lookup)
			if tt.wantNil {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, found)
				assert.Equal(t, project.ID, found.ID)
			}
		})
	}
}

func TestProjectRepository_Resolve(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID: tenantID,
		Name:     "Resolvable Project",
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	tests := []struct {
		name       string
		identifier string
		wantErr    bool
	}{
		{
			name:       "resolve by ID",
			identifier: "1",
			wantErr:    true, // ID won't match unless it's actually 1
		},
		{
			name:       "resolve by name",
			identifier: "Resolvable Project",
			wantErr:    false,
		},
		{
			name:       "resolve non-existent",
			identifier: "Non-existent",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := repo.Resolve(ctx, tenantID, tt.identifier)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, found)
				assert.Equal(t, "Resolvable Project", found.Name)
			}
		})
	}
}

func TestProjectRepository_Update(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID:    tenantID,
		Name:        "Update Test",
		Description: strPtr("Original description"),
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)
	originalUpdatedAt := project.UpdatedAt

	// Update the project
	project.Name = "Updated Name"
	project.Description = strPtr("Updated description")
	project.Keywords = []string{"updated", "new"}

	err = repo.Update(ctx, project)
	require.NoError(t, err)

	// Verify updates
	found, err := repo.GetByID(ctx, project.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.Name)
	assert.Equal(t, "Updated description", *found.Description)
	assert.Equal(t, []string{"updated", "new"}, found.Keywords)
	assert.True(t, found.UpdatedAt.After(originalUpdatedAt) || found.UpdatedAt.Equal(originalUpdatedAt))
}

func TestProjectRepository_Delete(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID: tenantID,
		Name:     "Delete Test",
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	// Delete the project
	err = repo.Delete(ctx, project.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.GetByID(ctx, project.ID)
	assert.Error(t, err)
}

func TestProjectRepository_DeleteNonExistent(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Attempt to delete non-existent project
	err := repo.Delete(ctx, 999999)
	assert.Error(t, err)
}

func TestProjectRepository_List(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create multiple projects
	projectData := []struct {
		name     string
		keywords []string
		jira     []string
	}{
		{name: "Alpha Project", keywords: []string{"api", "backend"}},
		{name: "Beta Project", keywords: []string{"frontend"}},
		{name: "Gamma Project", jira: []string{"GAMMA-1"}},
		{name: "Delta Project", keywords: []string{"api", "critical"}},
	}

	for _, pd := range projectData {
		project := &projects.Project{
			TenantID:     tenantID,
			Name:         pd.name,
			Keywords:     pd.keywords,
			JiraProjects: pd.jira,
		}
		err := repo.Create(ctx, project)
		require.NoError(t, err)
	}

	tests := []struct {
		name      string
		filter    projects.ProjectFilter
		wantCount int
	}{
		{
			name: "list all projects",
			filter: projects.ProjectFilter{
				TenantID: tenantID,
			},
			wantCount: 4,
		},
		{
			name: "filter by name search",
			filter: projects.ProjectFilter{
				TenantID:   tenantID,
				NameSearch: "Alpha",
			},
			wantCount: 1,
		},
		{
			name: "filter by keyword",
			filter: projects.ProjectFilter{
				TenantID: tenantID,
				Keyword:  "api",
			},
			wantCount: 2,
		},
		{
			name: "filter by jira project",
			filter: projects.ProjectFilter{
				TenantID:    tenantID,
				JiraProject: "GAMMA-1",
			},
			wantCount: 1,
		},
		{
			name: "limit results",
			filter: projects.ProjectFilter{
				TenantID: tenantID,
				Limit:    2,
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := repo.List(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestProjectRepository_MemberOperations(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "people", "teams", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID: tenantID,
		Name:     "Member Test Project",
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	// Create a test person
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO people (tenant_id, canonical_name, primary_email, is_internal, account_type, confidence_score, needs_review, auto_created, created_at, updated_at)
		VALUES ($1, 'John Doe', 'john@test.com', true, 'person', 0.9, false, false, NOW(), NOW())
		RETURNING id
	`, tenantID)
	require.NoError(t, err)

	var personID int64
	err = db.Pool.QueryRow(ctx, "SELECT id FROM people WHERE primary_email = 'john@test.com'").Scan(&personID)
	require.NoError(t, err)

	// Add member
	member := &projects.ProjectMember{
		ProjectID: project.ID,
		PersonID:  &personID,
		Role:      "lead",
	}
	err = repo.AddMember(ctx, member)
	require.NoError(t, err)
	assert.NotZero(t, member.ID)

	// List members
	members, err := repo.ListMembers(ctx, project.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1)
	assert.Equal(t, "lead", members[0].Role)
	assert.Equal(t, "John Doe", members[0].PersonName)

	// Remove member
	err = repo.RemoveMember(ctx, member.ID)
	require.NoError(t, err)

	// Verify removal
	members, err = repo.ListMembers(ctx, project.ID)
	require.NoError(t, err)
	assert.Len(t, members, 0)
}

func TestProjectRepository_AddMemberDuplicate(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "projects", "project_members", "people", "tenants")

	tenantID := setupTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := projects.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a project
	project := &projects.Project{
		TenantID: tenantID,
		Name:     "Duplicate Member Test",
	}
	err := repo.Create(ctx, project)
	require.NoError(t, err)

	// Create a test person
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO people (tenant_id, canonical_name, primary_email, is_internal, account_type, confidence_score, needs_review, auto_created, created_at, updated_at)
		VALUES ($1, 'Jane Doe', 'jane@test.com', true, 'person', 0.9, false, false, NOW(), NOW())
	`, tenantID)
	require.NoError(t, err)

	var personID int64
	err = db.Pool.QueryRow(ctx, "SELECT id FROM people WHERE primary_email = 'jane@test.com'").Scan(&personID)
	require.NoError(t, err)

	// Add member first time
	member := &projects.ProjectMember{
		ProjectID: project.ID,
		PersonID:  &personID,
		Role:      "member",
	}
	err = repo.AddMember(ctx, member)
	require.NoError(t, err)

	// Attempt to add same member again
	duplicate := &projects.ProjectMember{
		ProjectID: project.ID,
		PersonID:  &personID,
		Role:      "member",
	}
	err = repo.AddMember(ctx, duplicate)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

// Helper function
func strPtr(s string) *string {
	return &s
}
