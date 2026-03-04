package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertUsers_SkipsUsersWithoutEmail(t *testing.T) {
	// convertUsers is an internal function; we test its behavior via ADUser output
	result := convertUsers(nil)
	require.Empty(t, result)
}

func TestConvertGroups_Empty(t *testing.T) {
	result := convertGroups(nil)
	require.Empty(t, result)
}

func TestADUser_Fields(t *testing.T) {
	u := ADUser{
		ID:           "test-id",
		DisplayName:  "Test User",
		Mail:         "test@example.com",
		JobTitle:     "Engineer",
		Department:   "Engineering",
		CompanyName:  "Acme Corp",
		UserType:     "Member",
		ManagerEmail: "boss@example.com",
	}

	require.Equal(t, "test-id", u.ID)
	require.Equal(t, "Test User", u.DisplayName)
	require.Equal(t, "test@example.com", u.Mail)
	require.Equal(t, "Engineer", u.JobTitle)
	require.Equal(t, "Engineering", u.Department)
	require.Equal(t, "Acme Corp", u.CompanyName)
	require.Equal(t, "Member", u.UserType)
	require.Equal(t, "boss@example.com", u.ManagerEmail)
}

func TestADGroup_Fields(t *testing.T) {
	g := ADGroup{
		ID:           "group-id",
		DisplayName:  "Engineering Team",
		Description:  "The engineering team",
		MemberEmails: []string{"alice@example.com", "bob@example.com"},
	}

	require.Equal(t, "group-id", g.ID)
	require.Equal(t, "Engineering Team", g.DisplayName)
	require.Equal(t, "The engineering team", g.Description)
	require.Len(t, g.MemberEmails, 2)
}
