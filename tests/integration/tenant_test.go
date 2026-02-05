//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenantRepository_Create(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	// Use unique IDs for each test run to avoid conflicts
	testID := uniqueTestID()

	tests := []struct {
		name    string
		input   tenant.TenantInput
		wantErr bool
	}{
		{
			name: "create simple tenant",
			input: tenant.TenantInput{
				Name:        "Acme Corp " + testID,
				Slug:        "acme-" + testID,
				Description: "Test tenant for Acme Corporation",
			},
			wantErr: false,
		},
		{
			name: "create tenant with custom slug",
			input: tenant.TenantInput{
				Name:        "Beta Corp " + testID,
				Slug:        "beta-" + testID,
				Description: "Tenant with custom slug",
			},
			wantErr: false,
		},
		{
			name: "create tenant with settings",
			input: tenant.TenantInput{
				Name:     "Gamma Inc " + testID,
				Slug:     "gamma-" + testID,
				Settings: ptrStr(`{"theme": "dark", "timezone": "UTC"}`),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := repo.Create(ctx, tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, created.ID)
			assert.Equal(t, tt.input.Name, created.Name)
			assert.True(t, created.IsActive)
			assert.NotEmpty(t, created.Slug)

			// Cleanup after test
			t.Cleanup(func() {
				_ = repo.Delete(ctx, created.ID, "test cleanup")
			})
		})
	}
}

func TestTenantRepository_CreateDuplicate(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()

	// Create first tenant with unique slug
	input := tenant.TenantInput{
		Name: "Duplicate Test Corp",
		Slug: "dup-test-" + testID,
	}
	created, err := repo.Create(ctx, input)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.Delete(ctx, created.ID, "test cleanup")
	})

	// Attempt to create duplicate with same slug
	_, err = repo.Create(ctx, input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestTenantRepository_Get(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()

	// Create a tenant with unique name
	input := tenant.TenantInput{
		Name:        "Get Test Corp " + testID,
		Slug:        "get-test-" + testID,
		Description: "Test tenant for get operations",
	}
	created, err := repo.Create(ctx, input)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.Delete(ctx, created.ID, "test cleanup")
	})

	tests := []struct {
		name    string
		id      string
		wantNil bool
	}{
		{
			name:    "get by valid UUID",
			id:      created.ID,
			wantNil: false,
		},
		{
			name:    "get by non-existent UUID",
			id:      "00000000-0000-0000-0000-000000000000",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.Get(ctx, tt.id)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, created.ID, result.ID)
				assert.Equal(t, created.Name, result.Name)
			}
		})
	}
}

func TestTenantRepository_GetBySlug(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()
	testSlug := "slug-test-" + testID

	// Create a tenant with known slug
	input := tenant.TenantInput{
		Name: "Slug Test Corp " + testID,
		Slug: testSlug,
	}
	created, err := repo.Create(ctx, input)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.Delete(ctx, created.ID, "test cleanup")
	})

	tests := []struct {
		name    string
		slug    string
		wantNil bool
	}{
		{
			name:    "get by valid slug",
			slug:    testSlug,
			wantNil: false,
		},
		{
			name:    "get by non-existent slug",
			slug:    "non-existent-slug",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetBySlug(ctx, tt.slug)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, created.ID, result.ID)
				assert.Equal(t, testSlug, result.Slug)
			}
		})
	}
}

func TestTenantRepository_GetByRef(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()
	testSlug := "ref-test-" + testID

	// Create a tenant
	input := tenant.TenantInput{
		Name: "Ref Test Corp " + testID,
		Slug: testSlug,
	}
	created, err := repo.Create(ctx, input)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.Delete(ctx, created.ID, "test cleanup")
	})

	tests := []struct {
		name    string
		ref     string
		wantNil bool
	}{
		{
			name:    "get by UUID",
			ref:     created.ID,
			wantNil: false,
		},
		{
			name:    "get by slug",
			ref:     testSlug,
			wantNil: false,
		},
		{
			name:    "get by non-existent ref",
			ref:     "non-existent",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByRef(ctx, tt.ref)
			require.NoError(t, err)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, created.ID, result.ID)
			}
		})
	}
}

func TestTenantRepository_List(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()
	var createdIDs []string

	// Create multiple tenants with unique names
	tenants := []tenant.TenantInput{
		{Name: "Alpha Corp " + testID, Slug: "alpha-" + testID},
		{Name: "Beta Corp " + testID, Slug: "beta-" + testID},
		{Name: "Gamma Corp " + testID, Slug: "gamma-" + testID},
		{Name: "Delta Corp " + testID, Slug: "delta-" + testID},
	}

	for _, input := range tenants {
		created, err := repo.Create(ctx, input)
		require.NoError(t, err)
		createdIDs = append(createdIDs, created.ID)
	}

	t.Cleanup(func() {
		for _, id := range createdIDs {
			_ = repo.Delete(ctx, id, "test cleanup")
		}
	})

	// Deactivate one tenant for testing
	list, _, err := repo.List(ctx, tenant.TenantFilter{Limit: 1})
	require.NoError(t, err)
	require.NotEmpty(t, list)
	inactive := false
	_, err = repo.Update(ctx, list[0].ID, tenant.TenantInput{IsActive: &inactive})
	require.NoError(t, err)

	tests := []struct {
		name      string
		filter    tenant.TenantFilter
		wantCount int
	}{
		{
			name:      "list all tenants",
			filter:    tenant.TenantFilter{},
			wantCount: 4,
		},
		{
			name:      "list active tenants only",
			filter:    tenant.TenantFilter{IsActive: ptrBool(true)},
			wantCount: 3,
		},
		{
			name:      "list with search",
			filter:    tenant.TenantFilter{Search: "Alpha"},
			wantCount: 1,
		},
		{
			name:      "list with limit",
			filter:    tenant.TenantFilter{Limit: 2},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _, err := repo.List(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestTenantRepository_Update(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()

	// Create a tenant with unique name
	created, err := repo.Create(ctx, tenant.TenantInput{
		Name:        "Update Test Corp " + testID,
		Slug:        "update-test-" + testID,
		Description: "Original description",
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.Delete(ctx, created.ID, "test cleanup")
	})

	// Update the tenant
	newName := "Updated Corp " + testID
	newDesc := "Updated description"
	updated, err := repo.Update(ctx, created.ID, tenant.TenantInput{
		Name:        newName,
		Description: newDesc,
	})
	require.NoError(t, err)

	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, newName, updated.Name)
	assert.Equal(t, newDesc, updated.Description)
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt))
}

func TestTenantRepository_UpdateNonExistent(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	// Attempt to update non-existent tenant
	_, err := repo.Update(ctx, "00000000-0000-0000-0000-999999999999", tenant.TenantInput{
		Name: "Should Fail",
	})
	assert.Error(t, err)
}

func TestTenantRepository_Delete(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	testID := uniqueTestID()

	// Create a tenant with unique name
	created, err := repo.Create(ctx, tenant.TenantInput{
		Name: "Delete Test Corp " + testID,
		Slug: "delete-test-" + testID,
	})
	require.NoError(t, err)

	// Verify it exists
	found, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.NotNil(t, found)

	// Delete the tenant
	err = repo.Delete(ctx, created.ID, "test deletion")
	require.NoError(t, err)

	// Verify it's soft-deleted (Get should return nil)
	found, err = repo.Get(ctx, created.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestTenantRepository_DeleteNonExistent(t *testing.T) {
	db := SetupTestDB(t)
	repo := tenant.NewRepository(db.Pool)
	ctx := context.Background()

	// Attempt to delete non-existent tenant
	err := repo.Delete(ctx, "00000000-0000-0000-0000-999999999999", "test")
	assert.Error(t, err)
}

// Helper functions
func ptrStr(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}
