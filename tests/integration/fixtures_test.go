//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureLoaderLoadPeople(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up test tenant's data first
	err := loader.CleanupAll(ctx)
	require.NoError(t, err)

	// Load teams first without leads (people haven't been loaded yet)
	err = loader.LoadTeamsWithoutLeads(ctx)
	require.NoError(t, err)

	// Load people
	err = loader.LoadPeople(ctx)
	require.NoError(t, err)

	// Now update team leads since people exist
	err = loader.UpdateTeamLeads(ctx)
	require.NoError(t, err)

	// Verify people were loaded for test tenant
	var count int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM people WHERE tenant_id = $1", db.TenantID).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 20, "expected at least 20 people for test tenant")

	// Verify specific person exists for test tenant
	var name, email string
	err = db.Pool.QueryRow(ctx, `
		SELECT canonical_name, primary_email FROM people
		WHERE tenant_id = $1 AND canonical_name = 'John Smith'
	`, db.TenantID).Scan(&name, &email)
	require.NoError(t, err)
	assert.Equal(t, "John Smith", name)
	assert.Equal(t, "john.smith@acme.com", email)
}

func TestFixtureLoaderLoadTeams(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up test tenant's data first
	err := loader.CleanupAll(ctx)
	require.NoError(t, err)

	// Load teams without leads (people don't exist yet)
	err = loader.LoadTeamsWithoutLeads(ctx)
	require.NoError(t, err)

	// Verify teams were loaded for test tenant
	var count int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM teams WHERE tenant_id = $1", db.TenantID).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 5, "expected at least 5 teams for test tenant")

	// Verify a specific team exists by name for test tenant
	var name string
	err = db.Pool.QueryRow(ctx, `
		SELECT name FROM teams WHERE tenant_id = $1 AND name = 'Engineering'
	`, db.TenantID).Scan(&name)
	require.NoError(t, err)
	assert.Equal(t, "Engineering", name)
}

func TestFixtureLoaderLoadGlossary(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up test tenant's data first
	err := loader.CleanupAll(ctx)
	require.NoError(t, err)

	// Load glossary
	err = loader.LoadGlossary(ctx)
	require.NoError(t, err)

	// Verify glossary terms were loaded for test tenant
	var count int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM glossary WHERE tenant_id = $1", db.TenantID).Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 50, "expected at least 50 glossary terms for test tenant")

	// Verify specific term for test tenant
	var expansion string
	err = db.Pool.QueryRow(ctx, `
		SELECT expansion FROM glossary WHERE tenant_id = $1 AND term = 'TER'
	`, db.TenantID).Scan(&expansion)
	require.NoError(t, err)
	assert.Equal(t, "Technical Execution Review", expansion)
}

func TestFixtureLoaderLoadAcmeCorp(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up test tenant's data first
	err := loader.CleanupAll(ctx)
	require.NoError(t, err)

	// Load all Acme Corp fixtures
	err = loader.LoadAcmeCorp(ctx)
	require.NoError(t, err)

	// Verify all fixture types were loaded for test tenant
	tables := map[string]int{
		"teams":    5,
		"people":   20,
		"projects": 10,
		"products": 3,
		"glossary": 50,
	}

	for table, minCount := range tables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE tenant_id = $1", table)
		err := db.Pool.QueryRow(ctx, query, db.TenantID).Scan(&count)
		require.NoError(t, err, "failed to count %s for test tenant", table)
		assert.GreaterOrEqual(t, count, minCount,
			"expected at least %d rows in %s for test tenant", minCount, table)
	}
}

func TestFixtureCleanupAll(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Load some data for test tenant
	err := loader.LoadAcmeCorp(ctx)
	require.NoError(t, err)

	// Verify data exists for test tenant
	var count int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM people WHERE tenant_id = $1", db.TenantID).Scan(&count)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "test tenant should have people before cleanup")

	// Count other tenants' data before cleanup
	var otherCountBefore int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM people WHERE tenant_id != $1", db.TenantID).Scan(&otherCountBefore)
	require.NoError(t, err)

	// Cleanup test tenant's data only
	err = loader.CleanupAll(ctx)
	require.NoError(t, err)

	// Verify test tenant's data is gone
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM people WHERE tenant_id = $1", db.TenantID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "test tenant should have no people after cleanup")

	// Verify other tenants' data is preserved
	var otherCountAfter int
	err = db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM people WHERE tenant_id != $1", db.TenantID).Scan(&otherCountAfter)
	require.NoError(t, err)
	assert.Equal(t, otherCountBefore, otherCountAfter,
		"other tenants' data should be preserved after cleanup")
}
