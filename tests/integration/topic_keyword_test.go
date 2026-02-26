//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/pkg/topics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopicRepository_ListForContext_KeywordMatch verifies that ListForContext
// matches topics by keyword containment in candidate names, not just exact name match.
// Bug: pf-8700ec — "Oslo NLB Workflow" should match topic "Oslo" via keyword "oslo".
func TestTopicRepository_ListForContext_KeywordMatch(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	db.CleanupTables(t, "topics")

	repo := topics.NewRepository(db.Pool, logging.NewNopLogger())
	ctx := context.Background()
	tenantID := IntegrationTestTenantID

	// Create topics with keywords
	oslo := &topics.Topic{
		TenantID:    tenantID,
		Name:        "Oslo",
		Description: strPtr("Dedicated Linode region for MTC"),
		Keywords:    []string{"oslo", "linode", "region", "mtc", "tiktok", "dedicated"},
	}
	devcloud := &topics.Topic{
		TenantID:    tenantID,
		Name:        "DevCloud",
		Description: strPtr("Internal testing environment shared across teams"),
		Keywords:    []string{"devcloud", "testing"},
	}
	clic := &topics.Topic{
		TenantID:    tenantID,
		Name:        "CLIC",
		Description: strPtr("Cloud Infrastructure Committee"),
		Keywords:    []string{"clic", "infrastructure"},
	}

	require.NoError(t, repo.Create(ctx, oslo))
	require.NoError(t, repo.Create(ctx, devcloud))
	require.NoError(t, repo.Create(ctx, clic))

	t.Run("exact name match still works", func(t *testing.T) {
		results, err := repo.ListForContext(ctx, tenantID, []string{"Oslo", "DevCloud"})
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("keyword in candidate matches topic", func(t *testing.T) {
		// "Oslo NLB Workflow" contains keyword "oslo" → should match topic Oslo
		results, err := repo.ListForContext(ctx, tenantID, []string{"Oslo NLB Workflow", "CLIC", "DevCloud"})
		require.NoError(t, err)
		require.Len(t, results, 3, "expected Oslo (via keyword), CLIC (exact), DevCloud (exact)")

		names := make(map[string]bool)
		for _, r := range results {
			names[r.Name] = true
		}
		assert.True(t, names["Oslo"], "Oslo should match via keyword 'oslo' in 'oslo nlb workflow'")
		assert.True(t, names["CLIC"], "CLIC should match by exact name")
		assert.True(t, names["DevCloud"], "DevCloud should match by exact name")
	})

	t.Run("keyword match is case insensitive", func(t *testing.T) {
		// "OSLO Project" uppercased — keyword "oslo" should still match
		results, err := repo.ListForContext(ctx, tenantID, []string{"OSLO Project"})
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Oslo", results[0].Name)
	})

	t.Run("no match returns empty", func(t *testing.T) {
		results, err := repo.ListForContext(ctx, tenantID, []string{"Unknown Thing", "Random Name"})
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})

	t.Run("no duplicate when both name and keyword match", func(t *testing.T) {
		// "DevCloud" matches by exact name AND by keyword "devcloud"
		results, err := repo.ListForContext(ctx, tenantID, []string{"DevCloud"})
		require.NoError(t, err)
		assert.Len(t, results, 1, "should not return duplicates when both name and keyword match")
	})

	t.Run("keyword substring in longer candidate", func(t *testing.T) {
		// "linode" is a keyword for Oslo — candidate "Linode Kubernetes Engine" contains it
		results, err := repo.ListForContext(ctx, tenantID, []string{"Linode Kubernetes Engine"})
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Oslo", results[0].Name)
	})

	t.Run("empty names returns nil", func(t *testing.T) {
		results, err := repo.ListForContext(ctx, tenantID, []string{})
		require.NoError(t, err)
		assert.Nil(t, results)
	})
}

