//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestGroupMember_AddListRemove(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "entity_group_members", "people", "person_aliases", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create a distribution list (group entity)
	group := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Test DL",
		PrimaryEmail:  "test-dl@example.com",
		AccountType:   entities.AccountTypeDistribution,
		Confidence:    1.0,
	}
	require.NoError(t, repo.CreatePerson(ctx, group))
	require.NotZero(t, group.ID)

	// Create two member entities
	member1 := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Alice Member",
		PrimaryEmail:  "alice@example.com",
		AccountType:   entities.AccountTypePerson,
		Confidence:    1.0,
	}
	require.NoError(t, repo.CreatePerson(ctx, member1))

	member2 := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Bob Member",
		PrimaryEmail:  "bob@example.com",
		AccountType:   entities.AccountTypePerson,
		Confidence:    1.0,
	}
	require.NoError(t, repo.CreatePerson(ctx, member2))

	// Add both members
	id1, err := repo.AddGroupMember(ctx, tenantID, group.ID, member1.ID, "manual")
	require.NoError(t, err)
	assert.NotZero(t, id1, "should return membership ID")

	id2, err := repo.AddGroupMember(ctx, tenantID, group.ID, member2.ID, "pipeline")
	require.NoError(t, err)
	assert.NotZero(t, id2)

	// List active members
	members, err := repo.ListGroupMembers(ctx, tenantID, group.ID, false)
	require.NoError(t, err)
	assert.Len(t, members, 2)
	assert.Equal(t, "Alice Member", members[0].MemberName)
	assert.Equal(t, "alice@example.com", members[0].MemberEmail)
	assert.Equal(t, "manual", members[0].Source)

	// Remove member2
	removed, err := repo.RemoveGroupMember(ctx, tenantID, group.ID, member2.ID)
	require.NoError(t, err)
	assert.True(t, removed)

	// Removing again should return false (already removed)
	removed, err = repo.RemoveGroupMember(ctx, tenantID, group.ID, member2.ID)
	require.NoError(t, err)
	assert.False(t, removed, "second remove should return false")

	// List active should show 1
	members, err = repo.ListGroupMembers(ctx, tenantID, group.ID, false)
	require.NoError(t, err)
	assert.Len(t, members, 1)

	// List with history should show 2
	members, err = repo.ListGroupMembers(ctx, tenantID, group.ID, true)
	require.NoError(t, err)
	assert.Len(t, members, 2)
	// Second member should have removed_at set
	assert.NotNil(t, members[1].RemovedAt)
}

func TestGroupMember_GetEntityGroups(t *testing.T) {
	db := SetupTestDB(t)
	db.TruncateTables(t, "entity_group_members", "people", "person_aliases", "tenants")

	tenantID := setupEntityTestTenant(t, db)
	logger := logging.NewNopLogger()
	repo := entities.NewRepository(db.Pool, logger)
	ctx := context.Background()

	// Create 2 groups and 1 member
	group1 := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Group Alpha",
		PrimaryEmail:  "alpha@example.com",
		AccountType:   entities.AccountTypeDistribution,
		Confidence:    1.0,
	}
	require.NoError(t, repo.CreatePerson(ctx, group1))

	group2 := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Group Beta",
		PrimaryEmail:  "beta@example.com",
		AccountType:   entities.AccountTypeDistribution,
		Confidence:    1.0,
	}
	require.NoError(t, repo.CreatePerson(ctx, group2))

	member := &entities.Person{
		TenantID:      tenantID,
		CanonicalName: "Charlie Member",
		PrimaryEmail:  "charlie@example.com",
		AccountType:   entities.AccountTypePerson,
		Confidence:    1.0,
	}
	require.NoError(t, repo.CreatePerson(ctx, member))

	// Add member to both groups
	_, err := repo.AddGroupMember(ctx, tenantID, group1.ID, member.ID, "")
	require.NoError(t, err)
	_, err = repo.AddGroupMember(ctx, tenantID, group2.ID, member.ID, "")
	require.NoError(t, err)

	// GetEntityGroups should return both
	groups, err := repo.GetEntityGroups(ctx, tenantID, member.ID, false)
	require.NoError(t, err)
	assert.Len(t, groups, 2)

	// The returned MemberName/MemberEmail should be the GROUP's name/email (not the member's)
	names := []string{groups[0].MemberName, groups[1].MemberName}
	assert.Contains(t, names, "Group Alpha")
	assert.Contains(t, names, "Group Beta")
}
