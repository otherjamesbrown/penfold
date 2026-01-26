package projects

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== Project Struct Tests ====================

func TestProjectStructure(t *testing.T) {
	now := time.Now()
	desc := "Test description"

	project := &Project{
		ID:           42,
		TenantID:     "tenant-123",
		Name:         "Test Project",
		Description:  &desc,
		Keywords:     []string{"test", "example"},
		JiraProjects: []string{"PROJ-1", "PROJ-2"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	assert.Equal(t, int64(42), project.ID)
	assert.Equal(t, "tenant-123", project.TenantID)
	assert.Equal(t, "Test Project", project.Name)
	require.NotNil(t, project.Description)
	assert.Equal(t, "Test description", *project.Description)
	assert.Len(t, project.Keywords, 2)
	assert.Contains(t, project.Keywords, "test")
	assert.Len(t, project.JiraProjects, 2)
	assert.Contains(t, project.JiraProjects, "PROJ-1")
}

func TestProjectWithNilFields(t *testing.T) {
	project := &Project{
		ID:       1,
		TenantID: "tenant-123",
		Name:     "Minimal Project",
	}

	assert.Nil(t, project.Description)
	assert.Nil(t, project.Keywords)
	assert.Nil(t, project.JiraProjects)
}

// ==================== ProjectMember Tests ====================

func TestProjectMemberStructure(t *testing.T) {
	now := time.Now()
	personID := int64(100)

	member := &ProjectMember{
		ID:          1,
		ProjectID:   42,
		PersonID:    &personID,
		TeamID:      nil,
		Role:        "contributor",
		AddedAt:     now,
		PersonName:  "John Doe",
		PersonEmail: "john@example.com",
	}

	assert.Equal(t, int64(1), member.ID)
	assert.Equal(t, int64(42), member.ProjectID)
	require.NotNil(t, member.PersonID)
	assert.Equal(t, int64(100), *member.PersonID)
	assert.Nil(t, member.TeamID)
	assert.Equal(t, "contributor", member.Role)
	assert.Equal(t, "John Doe", member.PersonName)
	assert.Equal(t, "john@example.com", member.PersonEmail)
}

func TestProjectMemberWithTeam(t *testing.T) {
	teamID := int64(200)

	member := &ProjectMember{
		ID:        2,
		ProjectID: 42,
		PersonID:  nil,
		TeamID:    &teamID,
		Role:      "owner",
		TeamName:  "Engineering",
	}

	assert.Nil(t, member.PersonID)
	require.NotNil(t, member.TeamID)
	assert.Equal(t, int64(200), *member.TeamID)
	assert.Equal(t, "Engineering", member.TeamName)
}

// ==================== ProjectFilter Tests ====================

func TestProjectFilterDefaults(t *testing.T) {
	filter := ProjectFilter{}

	assert.Empty(t, filter.TenantID)
	assert.Empty(t, filter.NameSearch)
	assert.Empty(t, filter.Keyword)
	assert.Empty(t, filter.JiraProject)
	assert.Equal(t, 0, filter.Limit)
	assert.Equal(t, 0, filter.Offset)
}

func TestProjectFilterWithAllFields(t *testing.T) {
	filter := ProjectFilter{
		TenantID:    "tenant-123",
		NameSearch:  "search term",
		Keyword:     "test",
		JiraProject: "PROJ-1",
		Limit:       50,
		Offset:      100,
	}

	assert.Equal(t, "tenant-123", filter.TenantID)
	assert.Equal(t, "search term", filter.NameSearch)
	assert.Equal(t, "test", filter.Keyword)
	assert.Equal(t, "PROJ-1", filter.JiraProject)
	assert.Equal(t, 50, filter.Limit)
	assert.Equal(t, 100, filter.Offset)
}

// ==================== Error Constants Tests ====================

func TestErrorConstants(t *testing.T) {
	// Error constants use centralized domain errors from pkg/errors
	assert.Equal(t, "not found", ErrNotFound.Error())
	assert.Equal(t, "conflict", ErrConflict.Error())
}
