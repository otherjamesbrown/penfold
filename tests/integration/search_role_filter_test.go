//go:build integration

package integration

import (
	"context"
	"testing"

	mentionsv1 "github.com/otherjamesbrown/penfold/api/proto/mentions/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/otherjamesbrown/penfold/services/gateway/searchservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRoleFilterTestData creates test sources, people, and content_mentions
// with participation roles for search filter integration testing.
//
// Creates:
//   - 2 people: Alice (alice@test.com) and Bob (bob@test.com)
//   - 3 sources with keyword "integration-role-filter-test" in raw_content:
//     source A: Alice is FROM, Bob is TO
//     source B: Bob is FROM, Alice is CC
//     source C: Alice is TO (no FROM person in test data)
//
// Returns (aliceID, bobID, sourceAID, sourceBID, sourceCID).
func setupRoleFilterTestData(t *testing.T, db *TestDB) (int64, int64, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	tid := db.TenantID

	// Clean up previous test data
	db.CleanupTables(t, "content_mentions", "sources", "people")
	db.EnsureTenantExists(t)

	// Create people
	var aliceID, bobID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO people (tenant_id, canonical_name, email_addresses, account_type)
		VALUES ($1, 'Alice TestRole', ARRAY['alice@test.com'], 'person')
		RETURNING id
	`, tid).Scan(&aliceID)
	require.NoError(t, err)

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO people (tenant_id, canonical_name, email_addresses, account_type)
		VALUES ($1, 'Bob TestRole', ARRAY['bob@test.com'], 'person')
		RETURNING id
	`, tid).Scan(&bobID)
	require.NoError(t, err)

	// Create sources with searchable content
	var sourceAID, sourceBID, sourceCID int64

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO sources (tenant_id, external_id, source_system, content_type, raw_content, content_hash, source_timestamp, created_at)
		VALUES ($1, 'role-filter-A', 'gmail', 'email',
			'integration-role-filter-test email A about network deployment from Alice to Bob',
			'a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0a0',
			NOW(), NOW())
		RETURNING id
	`, tid).Scan(&sourceAID)
	require.NoError(t, err)

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO sources (tenant_id, external_id, source_system, content_type, raw_content, content_hash, source_timestamp, created_at)
		VALUES ($1, 'role-filter-B', 'gmail', 'email',
			'integration-role-filter-test email B about network deployment from Bob to team with Alice on CC',
			'b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0',
			NOW(), NOW())
		RETURNING id
	`, tid).Scan(&sourceBID)
	require.NoError(t, err)

	err = db.Pool.QueryRow(ctx, `
		INSERT INTO sources (tenant_id, external_id, source_system, content_type, raw_content, content_hash, source_timestamp, created_at)
		VALUES ($1, 'role-filter-C', 'gmail', 'email',
			'integration-role-filter-test email C about network deployment sent to Alice',
			'c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0',
			NOW(), NOW())
		RETURNING id
	`, tid).Scan(&sourceCID)
	require.NoError(t, err)

	// Create content_mentions with participation roles
	mentions := []struct {
		contentID int64
		entityID  int64
		role      int // maps to ParticipationRole enum
		text      string
	}{
		// Source A: Alice FROM, Bob TO
		{sourceAID, aliceID, 1, "Alice TestRole"}, // FROM
		{sourceAID, bobID, 2, "Bob TestRole"},      // TO
		// Source B: Bob FROM, Alice CC
		{sourceBID, bobID, 1, "Bob TestRole"},      // FROM
		{sourceBID, aliceID, 3, "Alice TestRole"},   // CC
		// Source C: Alice TO
		{sourceCID, aliceID, 2, "Alice TestRole"},   // TO
	}

	for _, m := range mentions {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO content_mentions (tenant_id, content_id, entity_type, mentioned_text, resolved_entity_id, participation_role, status, resolution_confidence, resolution_source)
			VALUES ($1, $2, 'person', $3, $4, $5, 'auto_resolved', 1.0, 'exact_match')
		`, tid, m.contentID, m.text, m.entityID, m.role)
		require.NoError(t, err)
	}

	return aliceID, bobID, sourceAID, sourceBID, sourceCID
}

func TestSearchRoleFilter_FromByEmail(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	aliceID, _, sourceAID, _, _ := setupRoleFilterTestData(t, db)
	_ = aliceID

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search for content where alice@test.com is FROM
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "alice@test.com"},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_FROM},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Should return only source A (Alice is FROM on source A only)
	assert.Equal(t, 1, len(resp.Results), "from:alice should return exactly 1 result")
	if len(resp.Results) > 0 {
		// Verify it's source A by checking the source_id
		assert.Equal(t, "role-filter-A", resp.Results[0].SourceId)
		_ = sourceAID
	}
}

func TestSearchRoleFilter_FromByEntityID(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	_, bobID, _, sourceBID, _ := setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search for content where Bob is FROM, using entity ID
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_EntityId{EntityId: bobID},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_FROM},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Should return only source B (Bob is FROM on source B only)
	assert.Equal(t, 1, len(resp.Results), "from:bob should return exactly 1 result")
	if len(resp.Results) > 0 {
		assert.Equal(t, "role-filter-B", resp.Results[0].SourceId)
		_ = sourceBID
	}
}

func TestSearchRoleFilter_ToStrict(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search for content where Alice is TO (strict — not CC)
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "alice@test.com"},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_TO},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Alice is FROM on source A (not TO), CC on source B (not TO), TO on source C
	assert.Equal(t, 1, len(resp.Results), "to:alice should return 1 result (C only)")

	sourceIDs := make(map[string]bool)
	for _, r := range resp.Results {
		sourceIDs[r.SourceId] = true
	}
	assert.True(t, sourceIDs["role-filter-C"], "source C should be included (Alice is TO)")
	assert.False(t, sourceIDs["role-filter-A"], "source A should NOT be included (Alice is FROM, not TO)")
	assert.False(t, sourceIDs["role-filter-B"], "source B should NOT be included (Alice is CC, not TO)")
}

func TestSearchRoleFilter_RecipientBroad(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search for content where Alice is any kind of recipient (TO, CC, BCC, GROUP_MEMBER)
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "alice@test.com"},
					Roles: []mentionsv1.ParticipationRole{
						mentionsv1.ParticipationRole_PARTICIPATION_ROLE_TO,
						mentionsv1.ParticipationRole_PARTICIPATION_ROLE_CC,
						mentionsv1.ParticipationRole_PARTICIPATION_ROLE_BCC,
						mentionsv1.ParticipationRole_PARTICIPATION_ROLE_GROUP_MEMBER,
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Alice is CC on B and TO on C — both are recipient roles
	// Alice is FROM on A — FROM is not a recipient role
	assert.Equal(t, 2, len(resp.Results), "recipient:alice should return 2 results (B and C)")
}

func TestSearchRoleFilter_EntityNotFound(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search with a non-existent email — should return empty results, not error
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "nobody@test.com"},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_FROM},
				},
			},
		},
	})
	require.NoError(t, err, "non-existent entity should not error")
	require.NotNil(t, resp)
	assert.Equal(t, 0, len(resp.Results), "non-existent entity should return 0 results")
	assert.Equal(t, int64(0), resp.TotalCount)
}

func TestSearchRoleFilter_AnyRoleForEntity(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search with empty roles = any role for Alice
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "alice@test.com"},
					// No roles = match any role
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Alice has a mention on all 3 sources (FROM on A, CC on B, TO on C)
	assert.Equal(t, 3, len(resp.Results), "any-role:alice should return all 3 results")
}

func TestSearchRoleFilter_MultipleFiltersANDed(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search for content where Alice is FROM AND Bob is TO
	// Only source A matches (Alice FROM, Bob TO)
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "alice@test.com"},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_FROM},
				},
				{
					Entity: &searchv1.EntityRoleFilter_Email{Email: "bob@test.com"},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_TO},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, 1, len(resp.Results), "from:alice AND to:bob should match only source A")
	if len(resp.Results) > 0 {
		assert.Equal(t, "role-filter-A", resp.Results[0].SourceId)
	}
}

func TestSearchRoleFilter_NoFilterUnchanged(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search without any filters — should return all 3 sources
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, 3, len(resp.Results), "unfiltered search should return all 3 test sources")
}

func TestSearchRoleFilter_ContentTypeFilter(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Filter by content_type that doesn't match — should return 0
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			ContentTypes: []string{"meeting"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 0, len(resp.Results), "content_type=meeting should return 0 results (all are email)")

	// Filter by content_type that matches
	resp2, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			ContentTypes: []string{"email"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp2)
	assert.Equal(t, 3, len(resp2.Results), "content_type=email should return all 3 results")
}

func TestSearchRoleFilter_ByEntityName(t *testing.T) {
	db := SetupTestDBNoMigrations(t)
	setupRoleFilterTestData(t, db)

	ctx := context.Background()
	svc := searchservice.NewService(db.Pool, nil)

	// Search by canonical name instead of email
	resp, err := svc.Search(ctx, &searchv1.SearchRequest{
		Query:    "integration-role-filter-test",
		TenantId: db.TenantID,
		Limit:    10,
		Filters: &searchv1.FilterOptions{
			EntityRoleFilters: []*searchv1.EntityRoleFilter{
				{
					Entity: &searchv1.EntityRoleFilter_EntityName{EntityName: "Alice TestRole"},
					Roles:  []mentionsv1.ParticipationRole{mentionsv1.ParticipationRole_PARTICIPATION_ROLE_FROM},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, 1, len(resp.Results), "from:'Alice TestRole' should return 1 result")
	if len(resp.Results) > 0 {
		assert.Equal(t, "role-filter-A", resp.Results[0].SourceId)
	}
}
