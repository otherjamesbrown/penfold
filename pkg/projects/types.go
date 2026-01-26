// Package projects provides types and repository for project management.
// Projects represent business initiatives with keywords and Jira integration for auto-tagging.
package projects

import "time"

// Project represents a business project with keywords and Jira integration.
type Project struct {
	ID           int64     `json:"id,omitempty"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	Description  *string   `json:"description,omitempty"`
	Keywords     []string  `json:"keywords,omitempty"`
	JiraProjects []string  `json:"jira_projects,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ProjectMember represents membership in a project (person or team).
type ProjectMember struct {
	ID          int64     `json:"id,omitempty"`
	ProjectID   int64     `json:"project_id"`
	PersonID    *int64    `json:"person_id,omitempty"`
	TeamID      *int64    `json:"team_id,omitempty"`
	Role        string    `json:"role"`
	AddedAt     time.Time `json:"added_at"`
	PersonName  string    `json:"person_name,omitempty"`
	PersonEmail string    `json:"person_email,omitempty"`
	TeamName    string    `json:"team_name,omitempty"`
}

// ProjectFilter contains filtering options for project queries.
type ProjectFilter struct {
	TenantID    string
	NameSearch  string
	Keyword     string
	JiraProject string
	Limit       int
	Offset      int
}
