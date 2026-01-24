package entities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Repository provides database operations for entity resolution.
type Repository struct {
	pool   *pgxpool.Pool
	logger logging.Logger
}

// NewRepository creates a new entity repository.
func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
	return &Repository{
		pool:   pool,
		logger: logger.With(logging.F("component", "entity_repository")),
	}
}

// ==================== People Operations ====================

// CreatePerson creates a new person record.
func (r *Repository) CreatePerson(ctx context.Context, p *Person) error {
	query := `
		INSERT INTO people (
			tenant_id, canonical_name, primary_email,
			job_title, department, company, is_internal, account_type,
			confidence_score, needs_review, auto_created,
			reviewed_at, reviewed_by, potential_duplicates,
			created_at, updated_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7, $8,
			$9, $10, $11,
			$12, $13, $14,
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		p.TenantID,
		p.CanonicalName,
		p.PrimaryEmail,
		nullIfEmpty(p.Title),
		nullIfEmpty(p.Department),
		nullIfEmpty(p.Company),
		p.IsInternal,
		p.AccountType,
		p.Confidence,
		p.NeedsReview,
		p.AutoCreated,
		p.ReviewedAt,
		nullIfEmpty(p.ReviewedBy),
		p.PotentialDuplicates,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create person: %w", err)
	}

	r.logger.Debug("Person created",
		logging.F("id", p.ID),
		logging.F("email", p.PrimaryEmail))

	return nil
}

// GetPersonByID retrieves a person by ID.
func (r *Repository) GetPersonByID(ctx context.Context, id int64) (*Person, error) {
	query := `
		SELECT
			id, tenant_id, canonical_name, primary_email,
			job_title as title, department, company, is_internal, account_type,
			confidence_score as confidence, needs_review, auto_created,
			reviewed_at, reviewed_by, potential_duplicates,
			created_at, updated_at
		FROM people
		WHERE id = $1
	`
	return r.scanPerson(ctx, query, id)
}

// GetPersonByEmail retrieves a person by primary email.
func (r *Repository) GetPersonByEmail(ctx context.Context, tenantID, email string) (*Person, error) {
	query := `
		SELECT
			id, tenant_id, canonical_name, primary_email,
			job_title as title, department, company, is_internal, account_type,
			confidence_score as confidence, needs_review, auto_created,
			reviewed_at, reviewed_by, potential_duplicates,
			created_at, updated_at
		FROM people
		WHERE tenant_id = $1 AND primary_email = $2
	`
	return r.scanPerson(ctx, query, tenantID, email)
}

// GetPersonByAlias retrieves a person by any alias value.
func (r *Repository) GetPersonByAlias(ctx context.Context, tenantID, aliasValue string) (*Person, error) {
	query := `
		SELECT
			p.id, p.tenant_id, p.canonical_name, p.primary_email,
			p.job_title as title, p.department, p.company, p.is_internal, p.account_type,
			p.confidence_score as confidence, p.needs_review, p.auto_created,
			p.reviewed_at, p.reviewed_by, p.potential_duplicates,
			p.created_at, p.updated_at
		FROM people p
		JOIN person_aliases a ON a.person_id = p.id
		WHERE p.tenant_id = $1 AND a.alias_value = $2
		LIMIT 1
	`
	return r.scanPerson(ctx, query, tenantID, aliasValue)
}

// SearchPeopleByName searches for people by name similarity.
func (r *Repository) SearchPeopleByName(ctx context.Context, tenantID, name string, limit int) ([]*Person, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	// Use ILIKE for basic search - could be enhanced with trigram similarity
	query := `
		SELECT
			id, tenant_id, canonical_name, primary_email,
			job_title as title, department, company, is_internal, account_type,
			confidence_score as confidence, needs_review, auto_created,
			reviewed_at, reviewed_by, potential_duplicates,
			created_at, updated_at
		FROM people
		WHERE tenant_id = $1 AND canonical_name ILIKE '%' || $2 || '%'
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, tenantID, name, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search people: %w", err)
	}
	defer rows.Close()

	return r.scanPeople(rows)
}

// GetPeopleByDomain retrieves all people with a given email domain.
func (r *Repository) GetPeopleByDomain(ctx context.Context, tenantID, domain string) ([]*Person, error) {
	query := `
		SELECT
			id, tenant_id, canonical_name, primary_email,
			job_title as title, department, company, is_internal, account_type,
			confidence_score as confidence, needs_review, auto_created,
			reviewed_at, reviewed_by, potential_duplicates,
			created_at, updated_at
		FROM people
		WHERE tenant_id = $1 AND primary_email LIKE '%@' || $2
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, tenantID, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get people by domain: %w", err)
	}
	defer rows.Close()

	return r.scanPeople(rows)
}

// ListPeopleNeedingReview lists people that need review.
func (r *Repository) ListPeopleNeedingReview(ctx context.Context, tenantID string, limit int) ([]*Person, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	query := `
		SELECT
			id, tenant_id, canonical_name, primary_email,
			job_title as title, department, company, is_internal, account_type,
			confidence_score as confidence, needs_review, auto_created,
			reviewed_at, reviewed_by, potential_duplicates,
			created_at, updated_at
		FROM people
		WHERE tenant_id = $1 AND needs_review = TRUE
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list people needing review: %w", err)
	}
	defer rows.Close()

	return r.scanPeople(rows)
}

// UpdatePerson updates a person record.
func (r *Repository) UpdatePerson(ctx context.Context, p *Person) error {
	query := `
		UPDATE people SET
			canonical_name = $2,
			primary_email = $3,
			job_title = $4,
			department = $5,
			is_internal = $6,
			account_type = $7,
			confidence_score = $8,
			needs_review = $9,
			reviewed_at = $10,
			reviewed_by = $11,
			potential_duplicates = $12,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		p.ID,
		p.CanonicalName,
		p.PrimaryEmail,
		nullIfEmpty(p.Title),
		nullIfEmpty(p.Department),
		p.IsInternal,
		p.AccountType,
		p.Confidence,
		p.NeedsReview,
		p.ReviewedAt,
		nullIfEmpty(p.ReviewedBy),
		p.PotentialDuplicates,
	).Scan(&p.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to update person: %w", err)
	}

	return nil
}

// MarkPersonReviewed marks a person as reviewed.
func (r *Repository) MarkPersonReviewed(ctx context.Context, id int64, reviewedBy string) error {
	query := `
		UPDATE people SET
			needs_review = FALSE,
			reviewed_at = NOW(),
			reviewed_by = $2,
			confidence_score = 1.0,
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.pool.Exec(ctx, query, id, reviewedBy)
	if err != nil {
		return fmt.Errorf("failed to mark person reviewed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("person not found: %d", id)
	}

	return nil
}

// ==================== Alias Operations ====================

// CreateAlias creates a new person alias.
func (r *Repository) CreateAlias(ctx context.Context, alias *PersonAlias) error {
	query := `
		INSERT INTO person_aliases (
			person_id, alias_type, alias_value,
			confidence, source, discovered_at
		) VALUES ($1, $2, $3, $4, $5, NOW())
		RETURNING id, discovered_at
	`

	err := r.pool.QueryRow(ctx, query,
		alias.PersonID,
		alias.AliasType,
		alias.AliasValue,
		alias.Confidence,
		alias.Source,
	).Scan(&alias.ID, &alias.DiscoveredAt)

	if err != nil {
		return fmt.Errorf("failed to create alias: %w", err)
	}

	return nil
}

// GetAliasesForPerson retrieves all aliases for a person.
func (r *Repository) GetAliasesForPerson(ctx context.Context, personID int64) ([]PersonAlias, error) {
	query := `
		SELECT id, person_id, alias_type, alias_value, confidence, source, discovered_at
		FROM person_aliases
		WHERE person_id = $1
		ORDER BY confidence DESC, discovered_at DESC
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, personID)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases: %w", err)
	}
	defer rows.Close()

	var aliases []PersonAlias
	for rows.Next() {
		var a PersonAlias
		if err := rows.Scan(
			&a.ID, &a.PersonID, &a.AliasType, &a.AliasValue,
			&a.Confidence, &a.Source, &a.DiscoveredAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan alias: %w", err)
		}
		aliases = append(aliases, a)
	}

	return aliases, rows.Err()
}

// ==================== Team Operations ====================

// CreateTeam creates a new team.
func (r *Repository) CreateTeam(ctx context.Context, t *Team) error {
	query := `
		INSERT INTO teams (tenant_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		t.TenantID,
		t.Name,
		nullIfEmpty(t.Description),
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create team: %w", err)
	}

	return nil
}

// GetTeamByID retrieves a team by ID.
func (r *Repository) GetTeamByID(ctx context.Context, id int64) (*Team, error) {
	query := `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM teams
		WHERE id = $1
	`

	t := &Team{}
	var description *string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.TenantID, &t.Name, &description, &t.CreatedAt, &t.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}

	if description != nil {
		t.Description = *description
	}

	return t, nil
}

// GetTeamByName retrieves a team by name.
func (r *Repository) GetTeamByName(ctx context.Context, tenantID, name string) (*Team, error) {
	query := `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM teams
		WHERE tenant_id = $1 AND name = $2
	`

	t := &Team{}
	var description *string
	err := r.pool.QueryRow(ctx, query, tenantID, name).Scan(
		&t.ID, &t.TenantID, &t.Name, &description, &t.CreatedAt, &t.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}

	if description != nil {
		t.Description = *description
	}

	return t, nil
}

// AddTeamMember adds a member to a team.
func (r *Repository) AddTeamMember(ctx context.Context, m *TeamMember) error {
	query := `
		INSERT INTO team_members (team_id, person_id, role, joined_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (team_id, person_id) DO UPDATE SET role = $3
		RETURNING id, joined_at
	`

	err := r.pool.QueryRow(ctx, query,
		m.TeamID,
		m.PersonID,
		m.Role,
	).Scan(&m.ID, &m.JoinedAt)

	if err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	return nil
}

// GetTeamMembers retrieves all members of a team.
func (r *Repository) GetTeamMembers(ctx context.Context, teamID int64) ([]TeamMember, error) {
	query := `
		SELECT tm.id, tm.team_id, tm.person_id, tm.role, tm.joined_at,
		       p.canonical_name, p.primary_email
		FROM team_members tm
		JOIN people p ON p.id = tm.person_id
		WHERE tm.team_id = $1
		ORDER BY tm.joined_at ASC
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to get team members: %w", err)
	}
	defer rows.Close()

	var members []TeamMember
	for rows.Next() {
		var m TeamMember
		m.Person = &Person{}
		if err := rows.Scan(
			&m.ID, &m.TeamID, &m.PersonID, &m.Role, &m.JoinedAt,
			&m.Person.CanonicalName, &m.Person.PrimaryEmail,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", err)
		}
		m.Person.ID = m.PersonID
		members = append(members, m)
	}

	return members, rows.Err()
}

// ==================== Project Operations ====================

// CreateProject creates a new project.
func (r *Repository) CreateProject(ctx context.Context, p *Project) error {
	query := `
		INSERT INTO projects (tenant_id, name, description, keywords, jira_projects, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(ctx, query,
		p.TenantID,
		p.Name,
		nullIfEmpty(p.Description),
		p.Keywords,
		p.JiraProjects,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	return nil
}

// GetProjectByID retrieves a project by ID.
func (r *Repository) GetProjectByID(ctx context.Context, id int64) (*Project, error) {
	query := `
		SELECT id, tenant_id, name, description, keywords, jira_projects, created_at, updated_at
		FROM projects
		WHERE id = $1
	`

	p := &Project{}
	var description *string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.TenantID, &p.Name, &description, &p.Keywords, &p.JiraProjects, &p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if description != nil {
		p.Description = *description
	}

	return p, nil
}

// GetProjectByName retrieves a project by name.
func (r *Repository) GetProjectByName(ctx context.Context, tenantID, name string) (*Project, error) {
	query := `
		SELECT id, tenant_id, name, description, keywords, jira_projects, created_at, updated_at
		FROM projects
		WHERE tenant_id = $1 AND name = $2
	`

	p := &Project{}
	var description *string
	err := r.pool.QueryRow(ctx, query, tenantID, name).Scan(
		&p.ID, &p.TenantID, &p.Name, &description, &p.Keywords, &p.JiraProjects, &p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if description != nil {
		p.Description = *description
	}

	return p, nil
}

// GetProjectByJiraKey retrieves a project by Jira project key.
func (r *Repository) GetProjectByJiraKey(ctx context.Context, tenantID, jiraKey string) (*Project, error) {
	query := `
		SELECT id, tenant_id, name, description, keywords, jira_projects, created_at, updated_at
		FROM projects
		WHERE tenant_id = $1 AND $2 = ANY(jira_projects)
	`

	p := &Project{}
	var description *string
	err := r.pool.QueryRow(ctx, query, tenantID, jiraKey).Scan(
		&p.ID, &p.TenantID, &p.Name, &description, &p.Keywords, &p.JiraProjects, &p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if description != nil {
		p.Description = *description
	}

	return p, nil
}

// GetProjectsWithKeywords retrieves all projects that have keywords defined.
func (r *Repository) GetProjectsWithKeywords(ctx context.Context, tenantID string) ([]*Project, error) {
	query := `
		SELECT id, tenant_id, name, description, keywords, jira_projects, created_at, updated_at
		FROM projects
		WHERE tenant_id = $1 AND array_length(keywords, 1) > 0
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects: %w", err)
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var description *string
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Name, &description, &p.Keywords, &p.JiraProjects, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		if description != nil {
			p.Description = *description
		}
		projects = append(projects, p)
	}

	return projects, rows.Err()
}

// AddProjectMember adds a member (person or team) to a project.
func (r *Repository) AddProjectMember(ctx context.Context, m *ProjectMember) error {
	query := `
		INSERT INTO project_members (project_id, person_id, team_id, role, added_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, added_at
	`

	err := r.pool.QueryRow(ctx, query,
		m.ProjectID,
		m.PersonID,
		m.TeamID,
		m.Role,
	).Scan(&m.ID, &m.AddedAt)

	if err != nil {
		return fmt.Errorf("failed to add project member: %w", err)
	}

	return nil
}

// GetProjectMemberIDs returns all person IDs associated with a project (directly or via teams).
func (r *Repository) GetProjectMemberIDs(ctx context.Context, projectID int64) ([]int64, error) {
	query := `
		SELECT DISTINCT person_id FROM (
			-- Direct members
			SELECT person_id FROM project_members WHERE project_id = $1 AND person_id IS NOT NULL
			UNION
			-- Team members
			SELECT tm.person_id
			FROM project_members pm
			JOIN team_members tm ON tm.team_id = pm.team_id
			WHERE pm.project_id = $1 AND pm.team_id IS NOT NULL
		) AS all_members
	`

	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project member IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan ID: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// ==================== Helper Functions ====================

func (r *Repository) scanPerson(ctx context.Context, query string, args ...interface{}) (*Person, error) {
	p := &Person{}
	var title, department, company, reviewedBy *string

	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&p.ID, &p.TenantID, &p.CanonicalName, &p.PrimaryEmail,
		&title, &department, &company, &p.IsInternal, &p.AccountType,
		&p.Confidence, &p.NeedsReview, &p.AutoCreated,
		&p.ReviewedAt, &reviewedBy, &p.PotentialDuplicates,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan person: %w", err)
	}

	if title != nil {
		p.Title = *title
	}
	if department != nil {
		p.Department = *department
	}
	if company != nil {
		p.Company = *company
	}
	if reviewedBy != nil {
		p.ReviewedBy = *reviewedBy
	}

	return p, nil
}

func (r *Repository) scanPeople(rows pgx.Rows) ([]*Person, error) {
	var people []*Person
	for rows.Next() {
		p := &Person{}
		var title, department, company, reviewedBy *string

		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.CanonicalName, &p.PrimaryEmail,
			&title, &department, &company, &p.IsInternal, &p.AccountType,
			&p.Confidence, &p.NeedsReview, &p.AutoCreated,
			&p.ReviewedAt, &reviewedBy, &p.PotentialDuplicates,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan person: %w", err)
		}

		if title != nil {
			p.Title = *title
		}
		if department != nil {
			p.Department = *department
		}
		if company != nil {
			p.Company = *company
		}
		if reviewedBy != nil {
			p.ReviewedBy = *reviewedBy
		}

		people = append(people, p)
	}

	return people, rows.Err()
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ==================== Context Formatting Functions ====================

// ListPeopleForContext returns people formatted for LLM prompt context.
// This is the production function that E2E tests should use.
func (r *Repository) ListPeopleForContext(ctx context.Context, tenantID string, limit int) (string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 20
	}

	query := `
		SELECT id, canonical_name, COALESCE(primary_email, ''), COALESCE(job_title, '')
		FROM people
		WHERE ($1 = '' OR tenant_id = $1::uuid)
		ORDER BY id
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to list people: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("People in the organization:\n")

	for rows.Next() {
		var id int64
		var name, email, title string
		if err := rows.Scan(&id, &name, &email, &title); err != nil {
			return "", fmt.Errorf("failed to scan person: %w", err)
		}
		sb.WriteString(fmt.Sprintf("- %s (ID: %d, Email: %s, Title: %s)\n", name, id, email, title))
	}

	return sb.String(), rows.Err()
}

// ListTeamsForContext returns teams formatted for LLM prompt context.
// This is the production function that E2E tests should use.
func (r *Repository) ListTeamsForContext(ctx context.Context, tenantID string, limit int) (string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 10
	}

	query := `
		SELECT id, name, COALESCE(description, '')
		FROM teams
		WHERE ($1 = '' OR tenant_id = $1::uuid)
		ORDER BY id
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to list teams: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Teams in the organization:\n")

	for rows.Next() {
		var id int64
		var name, description string
		if err := rows.Scan(&id, &name, &description); err != nil {
			return "", fmt.Errorf("failed to scan team: %w", err)
		}
		sb.WriteString("- " + name)
		if description != "" {
			sb.WriteString(" (" + description + ")")
		}
		sb.WriteString("\n")
	}

	return sb.String(), rows.Err()
}

// ListProjectsForContext returns projects formatted for LLM prompt context.
// This is the production function that E2E tests should use.
func (r *Repository) ListProjectsForContext(ctx context.Context, tenantID string, limit int) (string, error) {
	if limit <= 0 || limit > 1000 {
		limit = 15
	}

	query := `
		SELECT id, name, COALESCE(description, '')
		FROM projects
		WHERE ($1 = '' OR tenant_id = $1::uuid)
		ORDER BY id
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString("Projects in the organization:\n")

	for rows.Next() {
		var id int64
		var name, description string
		if err := rows.Scan(&id, &name, &description); err != nil {
			return "", fmt.Errorf("failed to scan project: %w", err)
		}
		sb.WriteString("- " + name)
		if description != "" {
			sb.WriteString(" (" + description + ")")
		}
		sb.WriteString("\n")
	}

	return sb.String(), rows.Err()
}

// Ensure time is imported
var _ = time.Now
