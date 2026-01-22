//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureLoaderLoadPeople(t *testing.T) {
	db := SetupTestDB(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up first
	err := loader.TruncateAll(ctx)
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

	// Verify people were loaded
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 20, "expected at least 20 people")

	// Verify specific person
	var name, email string
	err = db.Pool.QueryRow(ctx, `
		SELECT canonical_name, primary_email FROM people WHERE id = 1
	`).Scan(&name, &email)
	require.NoError(t, err)
	assert.Equal(t, "John Smith", name)
	assert.Equal(t, "john.smith@acme.com", email)
}

func TestFixtureLoaderLoadTeams(t *testing.T) {
	db := SetupTestDB(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up first
	err := loader.TruncateAll(ctx)
	require.NoError(t, err)

	// Load teams without leads (people don't exist yet)
	err = loader.LoadTeamsWithoutLeads(ctx)
	require.NoError(t, err)

	// Verify teams were loaded
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM teams").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 5, "expected at least 5 teams")

	// Verify hierarchy
	var parentID *int64
	err = db.Pool.QueryRow(ctx, `
		SELECT parent_id FROM teams WHERE slug = 'platform'
	`).Scan(&parentID)
	require.NoError(t, err)
	assert.NotNil(t, parentID)
	assert.Equal(t, int64(1), *parentID, "Platform team should be under Engineering (id=1)")
}

func TestFixtureLoaderLoadGlossary(t *testing.T) {
	db := SetupTestDB(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up first
	err := loader.TruncateAll(ctx)
	require.NoError(t, err)

	// Load glossary
	err = loader.LoadGlossary(ctx)
	require.NoError(t, err)

	// Verify glossary terms were loaded
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM glossary").Scan(&count)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 50, "expected at least 50 glossary terms")

	// Verify specific term
	var expansion string
	err = db.Pool.QueryRow(ctx, `
		SELECT expansion FROM glossary WHERE term = 'TER'
	`).Scan(&expansion)
	require.NoError(t, err)
	assert.Equal(t, "Technical Execution Review", expansion)
}

func TestFixtureLoaderLoadAcmeCorp(t *testing.T) {
	db := SetupTestDB(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Clean up first
	err := loader.TruncateAll(ctx)
	require.NoError(t, err)

	// Load all Acme Corp fixtures
	err = loader.LoadAcmeCorp(ctx)
	require.NoError(t, err)

	// Verify all fixture types were loaded
	tables := map[string]int{
		"teams":          5,
		"people":         20,
		"projects":       10,
		"products":       3,
		"glossary": 50,
	}

	for table, minCount := range tables {
		var count int
		err := db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count)
		require.NoError(t, err, "failed to count %s", table)
		assert.GreaterOrEqual(t, count, minCount, "expected at least %d rows in %s", minCount, table)
	}
}

func TestFixtureTruncateAll(t *testing.T) {
	db := SetupTestDB(t)
	loader := db.FixtureLoader()

	ctx := context.Background()

	// Load some data
	err := loader.LoadAcmeCorp(ctx)
	require.NoError(t, err)

	// Verify data exists
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&count)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// Truncate all
	err = loader.TruncateAll(ctx)
	require.NoError(t, err)

	// Verify data is gone
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
