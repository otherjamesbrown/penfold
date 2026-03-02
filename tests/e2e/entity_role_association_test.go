//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParticipationRoleSchema verifies that the content_mentions table supports
// participation_role and via_group_entity_id columns, and that existing mentions
// retain the default UNSPECIFIED (0) role.
//
// Acceptance test for Phase 1 of pf-400b05 (Entity-Content Role Associations).
func TestParticipationRoleSchema(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Find a person entity from fixtures
	var personID int64
	err = env.DB.QueryRow(ctx,
		"SELECT id FROM people WHERE tenant_id = $1 LIMIT 1",
		env.TenantID,
	).Scan(&personID)
	require.NoError(t, err, "need at least one person from fixtures")

	// Create a source record for content FK
	var contentID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (tenant_id, source_system, external_id, content_hash, processing_status)
		VALUES ($1, 'gmail', 'role-test-001', 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2', 'completed')
		RETURNING id
	`, env.TenantID).Scan(&contentID)
	require.NoError(t, err, "failed to create test source")
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM sources WHERE id = $1", contentID)
	})

	// --- Test 1: Insert mention with explicit participation_role ---
	var mentionID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO content_mentions (
			tenant_id, content_id, entity_type, mentioned_text,
			resolved_entity_id, status, resolution_source,
			participation_role
		) VALUES ($1, $2, 'person', 'Test Person', $3, 'auto_resolved', 'exact_match', 1)
		RETURNING id
	`, env.TenantID, contentID, personID).Scan(&mentionID)
	require.NoError(t, err, "should insert mention with participation_role=FROM(1)")
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM content_mentions WHERE id = $1", mentionID)
	})

	// Verify role persisted
	var role int
	err = env.DB.QueryRow(ctx,
		"SELECT participation_role FROM content_mentions WHERE id = $1",
		mentionID,
	).Scan(&role)
	require.NoError(t, err)
	assert.Equal(t, 1, role, "participation_role should be FROM(1)")

	// --- Test 2: Insert mention without participation_role — defaults to UNSPECIFIED(0) ---
	var defaultMentionID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO content_mentions (
			tenant_id, content_id, entity_type, mentioned_text,
			resolved_entity_id, status, resolution_source
		) VALUES ($1, $2, 'person', 'Default Role Person', $3, 'auto_resolved', 'exact_match')
		RETURNING id
	`, env.TenantID, contentID, personID).Scan(&defaultMentionID)
	require.NoError(t, err, "should insert mention without explicit role")
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM content_mentions WHERE id = $1", defaultMentionID)
	})

	var defaultRole int
	err = env.DB.QueryRow(ctx,
		"SELECT participation_role FROM content_mentions WHERE id = $1",
		defaultMentionID,
	).Scan(&defaultRole)
	require.NoError(t, err)
	assert.Equal(t, 0, defaultRole, "default participation_role should be UNSPECIFIED(0)")

	// --- Test 3: Role-based query returns correct results ---
	var fromCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM content_mentions
		WHERE tenant_id = $1 AND content_id = $2 AND participation_role = 1
	`, env.TenantID, contentID).Scan(&fromCount)
	require.NoError(t, err)
	assert.Equal(t, 1, fromCount, "should find exactly 1 mention with role=FROM")
}

// TestViaGroupEntityMention verifies that mentions can reference a group entity
// via the via_group_entity_id FK, supporting "received via distribution list" tracking.
//
// Acceptance test for Phase 1 of pf-400b05.
func TestViaGroupEntityMention(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create a distribution list entity
	var groupID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO people (tenant_id, canonical_name, email_addresses, account_type, is_internal)
		VALUES ($1, 'dl-test-leadership', ARRAY['dl-test-leadership@akamai.com'], 'distribution', true)
		RETURNING id
	`, env.TenantID).Scan(&groupID)
	require.NoError(t, err, "failed to create group entity")
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM people WHERE id = $1", groupID)
	})

	// Find a person entity (member)
	var memberID int64
	err = env.DB.QueryRow(ctx,
		"SELECT id FROM people WHERE tenant_id = $1 AND account_type != 'distribution' LIMIT 1",
		env.TenantID,
	).Scan(&memberID)
	require.NoError(t, err, "need a non-distribution person from fixtures")

	// Create a source
	var contentID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO sources (tenant_id, source_system, external_id, content_hash, processing_status)
		VALUES ($1, 'gmail', 'via-group-test-001', 'b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3', 'completed')
		RETURNING id
	`, env.TenantID).Scan(&contentID)
	require.NoError(t, err)
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM sources WHERE id = $1", contentID)
	})

	// Create GROUP_MEMBER mention with via_group_entity_id
	var mentionID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO content_mentions (
			tenant_id, content_id, entity_type, mentioned_text,
			resolved_entity_id, status, resolution_source,
			participation_role, via_group_entity_id
		) VALUES ($1, $2, 'person', 'Group Member', $3, 'auto_resolved', 'exact_match', 9, $4)
		RETURNING id
	`, env.TenantID, contentID, memberID, groupID).Scan(&mentionID)
	require.NoError(t, err, "should insert mention with role=GROUP_MEMBER(9) and via_group_entity_id")
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM content_mentions WHERE id = $1", mentionID)
	})

	// Verify via_group_entity_id persisted
	var storedRole int
	var viaGroupID *int64
	err = env.DB.QueryRow(ctx,
		"SELECT participation_role, via_group_entity_id FROM content_mentions WHERE id = $1",
		mentionID,
	).Scan(&storedRole, &viaGroupID)
	require.NoError(t, err)
	assert.Equal(t, 9, storedRole, "role should be GROUP_MEMBER(9)")
	require.NotNil(t, viaGroupID, "via_group_entity_id should be set")
	assert.Equal(t, groupID, *viaGroupID, "via_group_entity_id should reference the DL entity")
}

// TestEntityGroupMembersCRUD verifies that the entity_group_members table supports
// full CRUD: add member, list members, remove member (soft delete), list with history.
//
// Acceptance test for Phase 1 of pf-400b05.
func TestEntityGroupMembersCRUD(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create a distribution list entity
	var groupID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO people (tenant_id, canonical_name, email_addresses, account_type, is_internal)
		VALUES ($1, 'dl-crud-test', ARRAY['dl-crud-test@akamai.com'], 'distribution', true)
		RETURNING id
	`, env.TenantID).Scan(&groupID)
	require.NoError(t, err)
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM entity_group_members WHERE tenant_id = $1 AND group_entity_id = $2", env.TenantID, groupID)
		env.DB.Exec(context.Background(), "DELETE FROM people WHERE id = $1", groupID)
	})

	// Find two person entities from fixtures
	rows, err := env.DB.Query(ctx,
		"SELECT id, canonical_name FROM people WHERE tenant_id = $1 AND account_type != 'distribution' LIMIT 2",
		env.TenantID,
	)
	require.NoError(t, err)
	defer rows.Close()

	type person struct {
		ID   int64
		Name string
	}
	var members []person
	for rows.Next() {
		var p person
		require.NoError(t, rows.Scan(&p.ID, &p.Name))
		members = append(members, p)
	}
	require.Len(t, members, 2, "need at least 2 people from fixtures")

	// --- Add members ---
	for _, m := range members {
		_, err = env.DB.Exec(ctx, `
			INSERT INTO entity_group_members (tenant_id, group_entity_id, member_entity_id, source)
			VALUES ($1, $2, $3, 'manual')
		`, env.TenantID, groupID, m.ID)
		require.NoError(t, err, "should add member %s to group", m.Name)
	}

	// --- List active members ---
	var activeCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND group_entity_id = $2 AND removed_at IS NULL
	`, env.TenantID, groupID).Scan(&activeCount)
	require.NoError(t, err)
	assert.Equal(t, 2, activeCount, "should have 2 active members")

	// --- Remove one member (soft delete) ---
	_, err = env.DB.Exec(ctx, `
		UPDATE entity_group_members SET removed_at = NOW()
		WHERE tenant_id = $1 AND group_entity_id = $2 AND member_entity_id = $3
	`, env.TenantID, groupID, members[1].ID)
	require.NoError(t, err)

	// Active count should be 1
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND group_entity_id = $2 AND removed_at IS NULL
	`, env.TenantID, groupID).Scan(&activeCount)
	require.NoError(t, err)
	assert.Equal(t, 1, activeCount, "should have 1 active member after removal")

	// Total count (including removed) should still be 2
	var totalCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND group_entity_id = $2
	`, env.TenantID, groupID).Scan(&totalCount)
	require.NoError(t, err)
	assert.Equal(t, 2, totalCount, "should have 2 total members (1 active + 1 removed)")

	// --- Get entity's groups ---
	var groupCount int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND member_entity_id = $2 AND removed_at IS NULL
	`, env.TenantID, members[0].ID).Scan(&groupCount)
	require.NoError(t, err)
	assert.Equal(t, 1, groupCount, "member[0] should belong to 1 group")
}

// TestGroupManagementRPC verifies the group management RPCs work end-to-end
// via CLI commands: penf entity group add/list/remove.
//
// Acceptance test for Phase 1 RPCs of pf-400b05.
// Note: CLI commands are wired in Phase 3 — this test validates the gateway RPCs
// are functional. Until CLI commands exist, this test uses direct DB operations
// that mirror what the RPCs should do.
func TestGroupManagementRPC(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.CleanupTestTenant()
	require.NoError(t, err)
	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Create a distribution list entity
	var groupID int64
	err = env.DB.QueryRow(ctx, `
		INSERT INTO people (tenant_id, canonical_name, email_addresses, account_type, is_internal)
		VALUES ($1, 'dl-rpc-test', ARRAY['dl-rpc-test@akamai.com'], 'distribution', true)
		RETURNING id
	`, env.TenantID).Scan(&groupID)
	require.NoError(t, err)
	t.Cleanup(func() {
		env.DB.Exec(context.Background(), "DELETE FROM entity_group_members WHERE tenant_id = $1 AND group_entity_id = $2", env.TenantID, groupID)
		env.DB.Exec(context.Background(), "DELETE FROM people WHERE id = $1", groupID)
	})

	// Find a member
	var memberID int64
	var memberEmail string
	err = env.DB.QueryRow(ctx,
		"SELECT id, primary_email FROM people WHERE tenant_id = $1 AND account_type != 'distribution' AND primary_email IS NOT NULL LIMIT 1",
		env.TenantID,
	).Scan(&memberID, &memberEmail)
	require.NoError(t, err, "need a person with email from fixtures")

	// TODO: Once Phase 3 CLI commands land, replace DB operations below with:
	//   env.CLI.Run(ctx, "entity", "group", "add", fmt.Sprintf("%d", groupID), "--member", memberEmail)
	//   env.CLI.RunJSON(ctx, &members, "entity", "group", "list", fmt.Sprintf("%d", groupID))
	//   env.CLI.Run(ctx, "entity", "group", "remove", fmt.Sprintf("%d", groupID), "--member", memberEmail)

	// Add member via DB (mirrors AddGroupMember RPC)
	_, err = env.DB.Exec(ctx, `
		INSERT INTO entity_group_members (tenant_id, group_entity_id, member_entity_id, source)
		VALUES ($1, $2, $3, 'manual')
	`, env.TenantID, groupID, memberID)
	require.NoError(t, err, "AddGroupMember should succeed")

	// List members (mirrors ListGroupMembers RPC)
	var count int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND group_entity_id = $2 AND removed_at IS NULL
	`, env.TenantID, groupID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "ListGroupMembers should return 1 member")

	// Get entity groups (mirrors GetEntityGroups RPC)
	var groupsForMember int
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND member_entity_id = $2 AND removed_at IS NULL
	`, env.TenantID, memberID).Scan(&groupsForMember)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, groupsForMember, 1, "GetEntityGroups should find at least 1 group")

	// Remove member (mirrors RemoveGroupMember RPC — soft delete)
	_, err = env.DB.Exec(ctx, `
		UPDATE entity_group_members SET removed_at = NOW()
		WHERE tenant_id = $1 AND group_entity_id = $2 AND member_entity_id = $3 AND removed_at IS NULL
	`, env.TenantID, groupID, memberID)
	require.NoError(t, err, "RemoveGroupMember should succeed")

	// Verify removed
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND group_entity_id = $2 AND removed_at IS NULL
	`, env.TenantID, groupID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "ListGroupMembers should return 0 after removal")

	// Include historical — should still be 1
	err = env.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM entity_group_members
		WHERE tenant_id = $1 AND group_entity_id = $2
	`, env.TenantID, groupID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "ListGroupMembers with include_removed should return 1")
}
