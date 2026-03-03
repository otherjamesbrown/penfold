package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors - aliases to centralized errors for backward compatibility.
var (
	// ErrNotFound is returned when a project is not found.
	ErrNotFound = pferrors.ErrNotFound

	// ErrConflict is returned when a project name already exists.
	ErrConflict = pferrors.ErrConflict
)

// Repository provides database operations for projects.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new project repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "project_repository")),
	}
}

// Pool returns the underlying database pool.
func (r *Repository) Pool() *pgxpool.Pool {
	return r.pool
}

// ==================== Project CRUD ====================

// Create creates a new project.
func (r *Repository) Create(ctx context.Context, p *Project) error {
	query := `
		INSERT INTO projects (
			tenant_id, name, description, keywords, jira_projects,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	// Handle nil slices for postgres array types
	keywords := p.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	jiraProjects := p.JiraProjects
	if jiraProjects == nil {
		jiraProjects = []string{}
	}

	err := r.pool.QueryRow(ctx, query,
		p.TenantID,
		p.Name,
		p.Description,
		keywords,
		jiraProjects,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "projects_tenant_name_unique") {
			return fmt.Errorf("%w: project name '%s' already exists", ErrConflict, p.Name)
		}
		return fmt.Errorf("failed to create project: %w", err)
	}

	r.logger.Debug("Project created",
		logging.F("id", p.ID),
		logging.F("name", p.Name))

	return nil
}

// GetByID retrieves a project by ID.
func (r *Repository) GetByID(ctx context.Context, id int64) (*Project, error) {
	query := `
		SELECT
			id, tenant_id, name, description, keywords, jira_projects,
			status, timeline, metadata, created_at, updated_at
		FROM projects
		WHERE id = $1
	`
	return r.scanProject(ctx, query, id)
}

// GetByName retrieves a project by name within a tenant.
func (r *Repository) GetByName(ctx context.Context, tenantID, name string) (*Project, error) {
	query := `
		SELECT
			id, tenant_id, name, description, keywords, jira_projects,
			status, timeline, metadata, created_at, updated_at
		FROM projects
		WHERE tenant_id = $1 AND LOWER(name) = LOWER($2)
	`
	return r.scanProject(ctx, query, tenantID, name)
}

// Resolve resolves a project by ID or name.
func (r *Repository) Resolve(ctx context.Context, tenantID, identifier string) (*Project, error) {
	// Try by numeric ID first
	var id int64
	if _, err := fmt.Sscanf(identifier, "%d", &id); err == nil {
		p, err := r.GetByID(ctx, id)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	// Try by name
	return r.GetByName(ctx, tenantID, identifier)
}

// Update updates an existing project.
func (r *Repository) Update(ctx context.Context, p *Project) error {
	// Handle nil slices for postgres array types
	keywords := p.Keywords
	if keywords == nil {
		keywords = []string{}
	}
	jiraProjects := p.JiraProjects
	if jiraProjects == nil {
		jiraProjects = []string{}
	}

	query := `
		UPDATE projects SET
			name = $2,
			description = $3,
			keywords = $4,
			jira_projects = $5,
			timeline = $6,
			metadata = $7,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		p.ID,
		p.Name,
		p.Description,
		keywords,
		jiraProjects,
		p.Timeline,
		p.Metadata,
	).Scan(&p.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if strings.Contains(err.Error(), "projects_tenant_name_unique") {
			return fmt.Errorf("%w: project name '%s' already exists", ErrConflict, p.Name)
		}
		return fmt.Errorf("failed to update project: %w", err)
	}

	return nil
}

// Delete deletes a project by ID.
func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lists projects with optional filtering.
func (r *Repository) List(ctx context.Context, filter ProjectFilter) ([]*Project, error) {
	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, filter.TenantID)
	argIdx++

	if filter.NameSearch != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(name) LIKE '%%' || LOWER($%d) || '%%'", argIdx))
		args = append(args, filter.NameSearch)
		argIdx++
	}

	if filter.Keyword != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(keywords)", argIdx))
		args = append(args, filter.Keyword)
		argIdx++
	}

	if filter.JiraProject != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(jira_projects)", argIdx))
		args = append(args, filter.JiraProject)
		argIdx++
	}

	// Status filter (unless always-include names are provided, handled separately)
	if filter.Status != "" && len(filter.AlwaysIncludeNames) == 0 {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}

	// Build ORDER BY clause
	orderBy := "name"
	switch filter.SortBy {
	case "activity":
		// Sort by most recent activity (updated_at descending)
		orderBy = "updated_at DESC, name"
	case "created":
		orderBy = "created_at DESC, name"
	case "name", "":
		orderBy = "name"
	}

	// Build main query
	baseQuery := fmt.Sprintf(`
		SELECT
			id, tenant_id, name, description, keywords, jira_projects,
			status, timeline, metadata, created_at, updated_at
		FROM projects
		WHERE %s
		ORDER BY %s
	`, strings.Join(conditions, " AND "), orderBy)

	// Enforce default limits
	limit := 100
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
	}

	// Handle always-include logic
	var projects []*Project
	if len(filter.AlwaysIncludeNames) > 0 {
		// First, get projects matching the filter
		mainQuery := baseQuery + fmt.Sprintf(" LIMIT %d", limit)
		if filter.Offset > 0 {
			mainQuery += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}

		rows, err := r.pool.Query(ctx, mainQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to list projects: %w", err)
		}
		projects, err = r.scanProjects(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}

		// Now fetch always-include projects
		alwaysIncludeQuery := `
			SELECT
				id, tenant_id, name, description, keywords, jira_projects,
				status, created_at, updated_at
			FROM projects
			WHERE tenant_id = $1 AND LOWER(name) = ANY($2)
		`
		lowerNames := make([]string, len(filter.AlwaysIncludeNames))
		for i, name := range filter.AlwaysIncludeNames {
			lowerNames[i] = strings.ToLower(name)
		}

		alwaysRows, err := r.pool.Query(ctx, alwaysIncludeQuery, filter.TenantID, lowerNames)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch always-include projects: %w", err)
		}
		alwaysProjects, err := r.scanProjects(alwaysRows)
		alwaysRows.Close()
		if err != nil {
			return nil, err
		}

		// Merge, avoiding duplicates
		projectMap := make(map[int64]*Project)
		for _, p := range projects {
			projectMap[p.ID] = p
		}
		for _, p := range alwaysProjects {
			if _, exists := projectMap[p.ID]; !exists {
				projects = append(projects, p)
			}
		}
	} else {
		// Standard query without always-include
		query := baseQuery + fmt.Sprintf(" LIMIT %d", limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}

		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to list projects: %w", err)
		}
		defer rows.Close()

		projects, err = r.scanProjects(rows)
		if err != nil {
			return nil, err
		}
	}

	return projects, nil
}

// GetProjectContext retrieves comprehensive project context for drill-down.
func (r *Repository) GetProjectContext(ctx context.Context, tenantID, identifier string) (*ProjectContext, error) {
	// First resolve the project
	project, err := r.Resolve(ctx, tenantID, identifier)
	if err != nil {
		return nil, err
	}

	context := &ProjectContext{
		Project: project,
	}

	// Query recent meeting count
	meetingQuery := `
		SELECT COUNT(*)
		FROM sources
		WHERE tenant_id = $1
		  AND source_system = 'meeting'
		  AND content::jsonb->>'project' = $2
		  AND created_at > NOW() - INTERVAL '30 days'
	`
	if err := r.pool.QueryRow(ctx, meetingQuery, tenantID, project.Name).Scan(&context.RecentMeetingCount); err != nil {
		r.logger.Debug("Failed to get meeting count", logging.Err(err))
	}

	// Query open action count (from triage with assertion_type='action' and status not 'completed')
	actionQuery := `
		SELECT COUNT(*)
		FROM triage
		WHERE tenant_id = $1
		  AND assertion_type = 'action'
		  AND status != 'completed'
		  AND content::jsonb->>'project' = $2
	`
	if err := r.pool.QueryRow(ctx, actionQuery, tenantID, project.Name).Scan(&context.OpenActionCount); err != nil {
		r.logger.Debug("Failed to get action count", logging.Err(err))
	}

	// Query risk count
	riskQuery := `
		SELECT COUNT(*)
		FROM triage
		WHERE tenant_id = $1
		  AND assertion_type = 'risk'
		  AND content::jsonb->>'project' = $2
	`
	if err := r.pool.QueryRow(ctx, riskQuery, tenantID, project.Name).Scan(&context.RiskCount); err != nil {
		r.logger.Debug("Failed to get risk count", logging.Err(err))
	}

	// Query recent decisions
	decisionQuery := `
		SELECT content::jsonb->>'text'
		FROM triage
		WHERE tenant_id = $1
		  AND assertion_type = 'decision'
		  AND content::jsonb->>'project' = $2
		ORDER BY created_at DESC
		LIMIT 5
	`
	rows, err := r.pool.Query(ctx, decisionQuery, tenantID, project.Name)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var decision *string
			if err := rows.Scan(&decision); err == nil && decision != nil {
				context.RecentDecisions = append(context.RecentDecisions, *decision)
			}
		}
	}

	// Get last activity timestamp
	activityQuery := `
		SELECT MAX(created_at)
		FROM sources
		WHERE tenant_id = $1
		  AND content::jsonb->>'project' = $2
	`
	if err := r.pool.QueryRow(ctx, activityQuery, tenantID, project.Name).Scan(&context.LastActivity); err != nil {
		r.logger.Debug("Failed to get last activity", logging.Err(err))
	}

	return context, nil
}

// ==================== Project Member Operations ====================

// AddMember adds a person or team to a project.
func (r *Repository) AddMember(ctx context.Context, m *ProjectMember) error {
	query := `
		INSERT INTO project_members (project_id, person_id, team_id, role, added_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, added_at
	`

	role := m.Role
	if role == "" {
		role = "contributor"
	}

	err := r.pool.QueryRow(ctx, query,
		m.ProjectID,
		m.PersonID,
		m.TeamID,
		role,
	).Scan(&m.ID, &m.AddedAt)

	if err != nil {
		if strings.Contains(err.Error(), "project_members_unique_person") ||
			strings.Contains(err.Error(), "project_members_unique_team") {
			return fmt.Errorf("%w: member already exists in project", ErrConflict)
		}
		return fmt.Errorf("failed to add member: %w", err)
	}

	return nil
}

// RemoveMember removes a member from a project.
func (r *Repository) RemoveMember(ctx context.Context, memberID int64) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM project_members WHERE id = $1", memberID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMembers lists members of a project with person/team details populated.
func (r *Repository) ListMembers(ctx context.Context, projectID int64) ([]*ProjectMember, error) {
	query := `
		SELECT
			pm.id,
			pm.project_id,
			pm.person_id,
			pm.team_id,
			pm.role,
			pm.added_at,
			COALESCE(p.canonical_name, '') as person_name,
			COALESCE(p.primary_email, '') as person_email,
			COALESCE(t.name, '') as team_name
		FROM project_members pm
		LEFT JOIN people p ON pm.person_id = p.id
		LEFT JOIN teams t ON pm.team_id = t.id
		WHERE pm.project_id = $1
		ORDER BY pm.added_at DESC
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	var members []*ProjectMember
	for rows.Next() {
		m := &ProjectMember{}
		err := rows.Scan(
			&m.ID,
			&m.ProjectID,
			&m.PersonID,
			&m.TeamID,
			&m.Role,
			&m.AddedAt,
			&m.PersonName,
			&m.PersonEmail,
			&m.TeamName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}
		members = append(members, m)
	}

	return members, rows.Err()
}

// ==================== Helper Functions ====================

func (r *Repository) scanProject(ctx context.Context, query string, args ...any) (*Project, error) {
	p := &Project{}
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&p.ID,
		&p.TenantID,
		&p.Name,
		&p.Description,
		&p.Keywords,
		&p.JiraProjects,
		&p.Status,
		&p.Timeline,
		&p.Metadata,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan project: %w", err)
	}
	return p, nil
}

func (r *Repository) scanProjects(rows pgx.Rows) ([]*Project, error) {
	var projects []*Project
	for rows.Next() {
		p := &Project{}
		err := rows.Scan(
			&p.ID,
			&p.TenantID,
			&p.Name,
			&p.Description,
			&p.Keywords,
			&p.JiraProjects,
			&p.Status,
			&p.Timeline,
			&p.Metadata,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}
