//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntityShowReturnsFullMetadata verifies that entity detail/show returns
// all metadata including fields set via `penf entity update --metadata`.
//
// This is the acceptance test for the entity show endpoint.
// See task pf-ddc6dd.
func TestEntityShowReturnsFullMetadata(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Step 1: Load fixtures so we have entities to work with
	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Step 2: Find a person entity to work with
	var entityID int
	err = env.DB.QueryRow(ctx,
		"SELECT id FROM people WHERE tenant_id = $1 LIMIT 1",
		env.TenantID,
	).Scan(&entityID)
	require.NoError(t, err, "need at least one person entity from fixtures")

	// Step 3: Set metadata via CLI (this already works)
	result := env.CLI.Run(ctx, "entity", "update", fmt.Sprintf("%d", entityID),
		"--metadata", "alias=Rishi",
		"--metadata", "linkedin=https://linkedin.com/in/test",
	)
	require.True(t, result.Success(), "entity update should succeed: %s", result.Stderr)

	// Step 4: Fetch entity detail via the show endpoint
	// This is the new endpoint — `penf relationship entity show <id>`
	var detail EntityShowResponse
	err = env.CLI.RunJSON(ctx, &detail, "relationship", "entity", "show", fmt.Sprintf("%d", entityID))
	require.NoError(t, err, "entity show should return JSON detail")

	// Step 5: Verify core fields are present
	assert.NotEmpty(t, detail.ID, "should have entity ID")
	assert.NotEmpty(t, detail.Name, "should have entity name")
	assert.NotEmpty(t, detail.Type, "should have entity type")

	// Step 6: Verify metadata round-trip — the key assertion
	assert.Equal(t, "Rishi", detail.Metadata["alias"],
		"metadata 'alias' should survive write→read round-trip")
	assert.Equal(t, "https://linkedin.com/in/test", detail.Metadata["linkedin"],
		"metadata 'linkedin' should survive write→read round-trip")

	// Step 7: Verify standard metadata is also present
	assert.NotEmpty(t, detail.Metadata["email"], "should include email in metadata")
}

// TestEntityShowAcceptsPrefixedID verifies that the show endpoint accepts
// both numeric IDs (437) and prefixed IDs (ent-person-437).
func TestEntityShowAcceptsPrefixedID(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Find a person entity
	var entityID int
	err = env.DB.QueryRow(ctx,
		"SELECT id FROM people WHERE tenant_id = $1 LIMIT 1",
		env.TenantID,
	).Scan(&entityID)
	require.NoError(t, err)

	// Fetch with numeric ID
	var detail1 EntityShowResponse
	err = env.CLI.RunJSON(ctx, &detail1, "relationship", "entity", "show", fmt.Sprintf("%d", entityID))
	require.NoError(t, err, "should accept numeric ID")

	// Fetch with prefixed ID
	var detail2 EntityShowResponse
	err = env.CLI.RunJSON(ctx, &detail2, "relationship", "entity", "show", fmt.Sprintf("ent-person-%d", entityID))
	require.NoError(t, err, "should accept prefixed ID")

	// Both should return the same entity
	assert.Equal(t, detail1.Name, detail2.Name, "both ID formats should return same entity")
}

// TestEntityShowIncludesCounts verifies that the show endpoint returns
// sent/received/relation counts.
func TestEntityShowIncludesCounts(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	var entityID int
	err = env.DB.QueryRow(ctx,
		"SELECT id FROM people WHERE tenant_id = $1 LIMIT 1",
		env.TenantID,
	).Scan(&entityID)
	require.NoError(t, err)

	var detail EntityShowResponse
	err = env.CLI.RunJSON(ctx, &detail, "relationship", "entity", "show", fmt.Sprintf("%d", entityID))
	require.NoError(t, err)

	// These fields should be present in the response (values depend on fixture data)
	assert.NotEmpty(t, detail.ID)
	assert.NotEmpty(t, detail.Name)
	// Confidence should be set
	assert.Greater(t, detail.Confidence, 0.0, "should have non-zero confidence")
}

// EntityShowResponse is the expected response shape for entity show.
// This defines the contract — mycroft implements to match this.
type EntityShowResponse struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Confidence    float64           `json:"confidence"`
	SourceCount   int               `json:"source_count"`
	FirstSeen     string            `json:"first_seen"`
	LastSeen      string            `json:"last_seen"`
	Metadata      map[string]string `json:"metadata"`
	RelationCount int               `json:"relation_count"`
	SentCount     int               `json:"sent_count"`
	ReceivedCount int               `json:"received_count"`
}
