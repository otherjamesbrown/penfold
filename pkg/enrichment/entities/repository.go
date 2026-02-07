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
// Note: primary_email is a generated column (email_addresses[1]), so we insert
// into email_addresses instead.
func (r *Repository) CreatePerson(ctx context.Context, p *Person) error {
	query := `
		INSERT INTO people (
			tenant_id, canonical_name, email_addresses,
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
		RETURNING id, primary_email, created_at, updated_at
	`

	// Build email_addresses array from PrimaryEmail
	var emailAddresses []string
	if p.PrimaryEmail != "" {
		emailAddresses = []string{p.PrimaryEmail}
	}

	err := r.pool.QueryRow(ctx, query,
		p.TenantID,
		p.CanonicalName,
		emailAddresses,
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
	).Scan(&p.ID, &p.PrimaryEmail, &p.CreatedAt, &p.UpdatedAt)

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
			reviewed_at, reviewed_by,
			rejected_at, rejected_reason, rejected_by,
			potential_duplicates,
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
			reviewed_at, reviewed_by,
			rejected_at, rejected_reason, rejected_by,
			potential_duplicates,
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
			p.reviewed_at, p.reviewed_by,
			p.rejected_at, p.rejected_reason, p.rejected_by,
			p.potential_duplicates,
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
			reviewed_at, reviewed_by,
			rejected_at, rejected_reason, rejected_by,
			potential_duplicates,
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
			reviewed_at, reviewed_by,
			rejected_at, rejected_reason, rejected_by,
			potential_duplicates,
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
			reviewed_at, reviewed_by,
			rejected_at, rejected_reason, rejected_by,
			potential_duplicates,
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
// Note: primary_email is a generated column (email_addresses[1]), so we update
// email_addresses instead.
func (r *Repository) UpdatePerson(ctx context.Context, p *Person) error {
	query := `
		UPDATE people SET
			canonical_name = $2,
			email_addresses = $3,
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

	// Build email_addresses array from PrimaryEmail
	var emailAddresses []string
	if p.PrimaryEmail != "" {
		emailAddresses = []string{p.PrimaryEmail}
	}

	err := r.pool.QueryRow(ctx, query,
		p.ID,
		p.CanonicalName,
		emailAddresses,
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

// ListTeams retrieves teams with optional filtering.
func (r *Repository) ListTeams(ctx context.Context, tenantID, nameSearch string, limit, offset int) ([]*Team, error) {
	query := `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM teams
		WHERE tenant_id = $1
		  AND ($2 = '' OR LOWER(name) LIKE LOWER('%' || $2 || '%'))
		ORDER BY name ASC
		LIMIT $3 OFFSET $4
	`

	// Default limit if not specified
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, query, tenantID, nameSearch, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		t := &Team{}
		var description *string
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.Name, &description, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan team: %w", err)
		}
		if description != nil {
			t.Description = *description
		}
		teams = append(teams, t)
	}

	return teams, rows.Err()
}

// RemoveTeamMember removes a member from a team.
func (r *Repository) RemoveTeamMember(ctx context.Context, memberID int64) error {
	query := `DELETE FROM team_members WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, memberID)
	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("team member not found: %d", memberID)
	}

	return nil
}

// DeleteTeam deletes a team by ID.
func (r *Repository) DeleteTeam(ctx context.Context, teamID int64) error {
	query := `DELETE FROM teams WHERE id = $1`

	result, err := r.pool.Exec(ctx, query, teamID)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("team not found: %d", teamID)
	}

	return nil
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
	var title, department, company, reviewedBy, rejectedReason, rejectedBy *string

	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&p.ID, &p.TenantID, &p.CanonicalName, &p.PrimaryEmail,
		&title, &department, &company, &p.IsInternal, &p.AccountType,
		&p.Confidence, &p.NeedsReview, &p.AutoCreated,
		&p.ReviewedAt, &reviewedBy,
		&p.RejectedAt, &rejectedReason, &rejectedBy,
		&p.PotentialDuplicates,
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
	if rejectedReason != nil {
		p.RejectedReason = *rejectedReason
	}
	if rejectedBy != nil {
		p.RejectedBy = *rejectedBy
	}

	return p, nil
}

func (r *Repository) scanPeople(rows pgx.Rows) ([]*Person, error) {
	var people []*Person
	for rows.Next() {
		p := &Person{}
		var title, department, company, reviewedBy, rejectedReason, rejectedBy *string

		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.CanonicalName, &p.PrimaryEmail,
			&title, &department, &company, &p.IsInternal, &p.AccountType,
			&p.Confidence, &p.NeedsReview, &p.AutoCreated,
			&p.ReviewedAt, &reviewedBy,
			&p.RejectedAt, &rejectedReason, &rejectedBy,
			&p.PotentialDuplicates,
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
		if rejectedReason != nil {
			p.RejectedReason = *rejectedReason
		}
		if rejectedBy != nil {
			p.RejectedBy = *rejectedBy
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

// ==================== Entity Lifecycle Operations ====================

// RejectPerson soft-deletes a person by setting rejected_at.
func (r *Repository) RejectPerson(ctx context.Context, tenantID string, personID int64, reason, rejectedBy string) error {
	query := `
		UPDATE people SET
			rejected_at = NOW(),
			rejected_reason = $3,
			rejected_by = $4,
			updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2 AND rejected_at IS NULL
	`

	result, err := r.pool.Exec(ctx, query, tenantID, personID, reason, rejectedBy)
	if err != nil {
		return fmt.Errorf("failed to reject person: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("person not found or already rejected: %d", personID)
	}

	r.logger.Info("Person rejected",
		logging.F("person_id", personID),
		logging.F("reason", reason),
		logging.F("rejected_by", rejectedBy))

	return nil
}

// RestorePerson removes the rejection from a person.
func (r *Repository) RestorePerson(ctx context.Context, tenantID string, personID int64) error {
	query := `
		UPDATE people SET
			rejected_at = NULL,
			rejected_reason = NULL,
			rejected_by = NULL,
			updated_at = NOW()
		WHERE tenant_id = $1 AND id = $2 AND rejected_at IS NOT NULL
	`

	result, err := r.pool.Exec(ctx, query, tenantID, personID)
	if err != nil {
		return fmt.Errorf("failed to restore person: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("person not found or not rejected: %d", personID)
	}

	r.logger.Info("Person restored",
		logging.F("person_id", personID))

	return nil
}

// BulkRejectByPattern rejects multiple people matching email or name patterns.
func (r *Repository) BulkRejectByPattern(ctx context.Context, tenantID, emailPattern, namePattern, reason, rejectedBy string) (int, error) {
	if emailPattern == "" && namePattern == "" {
		return 0, fmt.Errorf("at least one pattern (email or name) is required")
	}

	query := `
		UPDATE people SET
			rejected_at = NOW(),
			rejected_reason = $3,
			rejected_by = $4,
			updated_at = NOW()
		WHERE tenant_id = $1
			AND rejected_at IS NULL
			AND (
				($5 != '' AND primary_email LIKE $5) OR
				($6 != '' AND canonical_name LIKE $6)
			)
	`

	result, err := r.pool.Exec(ctx, query, tenantID, reason, reason, rejectedBy, emailPattern, namePattern)
	if err != nil {
		return 0, fmt.Errorf("failed to bulk reject: %w", err)
	}

	count := int(result.RowsAffected())
	r.logger.Info("Bulk rejected people",
		logging.F("count", count),
		logging.F("email_pattern", emailPattern),
		logging.F("name_pattern", namePattern),
		logging.F("reason", reason))

	return count, nil
}

// CreateFilterRule creates a new entity filter rule.
func (r *Repository) CreateFilterRule(ctx context.Context, rule *EntityFilterRule) error {
	query := `
		INSERT INTO entity_filter_rules (
			tenant_id, email_pattern, name_pattern, entity_type, reason, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(ctx, query,
		rule.TenantID,
		nullIfEmpty(rule.EmailPattern),
		nullIfEmpty(rule.NamePattern),
		nullIfEmpty(rule.EntityType),
		rule.Reason,
		nullIfEmpty(rule.CreatedBy),
	).Scan(&rule.ID, &rule.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create filter rule: %w", err)
	}

	r.logger.Info("Filter rule created",
		logging.F("rule_id", rule.ID),
		logging.F("email_pattern", rule.EmailPattern),
		logging.F("name_pattern", rule.NamePattern))

	return nil
}

// ListFilterRules retrieves all filter rules for a tenant.
func (r *Repository) ListFilterRules(ctx context.Context, tenantID string) ([]*EntityFilterRule, error) {
	query := `
		SELECT id, tenant_id, email_pattern, name_pattern, entity_type, reason, created_at, created_by
		FROM entity_filter_rules
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT 1000
	`

	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list filter rules: %w", err)
	}
	defer rows.Close()

	var rules []*EntityFilterRule
	for rows.Next() {
		rule := &EntityFilterRule{}
		var emailPattern, namePattern, entityType, createdBy *string

		if err := rows.Scan(
			&rule.ID, &rule.TenantID, &emailPattern, &namePattern, &entityType,
			&rule.Reason, &rule.CreatedAt, &createdBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan filter rule: %w", err)
		}

		if emailPattern != nil {
			rule.EmailPattern = *emailPattern
		}
		if namePattern != nil {
			rule.NamePattern = *namePattern
		}
		if entityType != nil {
			rule.EntityType = *entityType
		}
		if createdBy != nil {
			rule.CreatedBy = *createdBy
		}

		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

// DeleteFilterRule deletes a filter rule.
func (r *Repository) DeleteFilterRule(ctx context.Context, tenantID string, ruleID int64) error {
	query := `DELETE FROM entity_filter_rules WHERE tenant_id = $1 AND id = $2`

	result, err := r.pool.Exec(ctx, query, tenantID, ruleID)
	if err != nil {
		return fmt.Errorf("failed to delete filter rule: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("filter rule not found: %d", ruleID)
	}

	r.logger.Info("Filter rule deleted", logging.F("rule_id", ruleID))

	return nil
}

// TestFilterRule checks if an email/name would match any filter rules.
func (r *Repository) TestFilterRule(ctx context.Context, tenantID, email, name string) ([]*EntityFilterRule, error) {
	query := `
		SELECT id, tenant_id, email_pattern, name_pattern, entity_type, reason, created_at, created_by
		FROM entity_filter_rules
		WHERE tenant_id = $1
			AND (
				(email_pattern IS NOT NULL AND $2 LIKE email_pattern) OR
				(name_pattern IS NOT NULL AND $3 LIKE name_pattern)
			)
	`

	rows, err := r.pool.Query(ctx, query, tenantID, email, name)
	if err != nil {
		return nil, fmt.Errorf("failed to test filter rules: %w", err)
	}
	defer rows.Close()

	var matchingRules []*EntityFilterRule
	for rows.Next() {
		rule := &EntityFilterRule{}
		var emailPattern, namePattern, entityType, createdBy *string

		if err := rows.Scan(
			&rule.ID, &rule.TenantID, &emailPattern, &namePattern, &entityType,
			&rule.Reason, &rule.CreatedAt, &createdBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan filter rule: %w", err)
		}

		if emailPattern != nil {
			rule.EmailPattern = *emailPattern
		}
		if namePattern != nil {
			rule.NamePattern = *namePattern
		}
		if entityType != nil {
			rule.EntityType = *entityType
		}
		if createdBy != nil {
			rule.CreatedBy = *createdBy
		}

		matchingRules = append(matchingRules, rule)
	}

	return matchingRules, rows.Err()
}

// MatchesFilterRule returns true if the email/name matches any active filter rule.
// This is used during entity resolution to prevent creation of filtered entities.
func (r *Repository) MatchesFilterRule(ctx context.Context, tenantID, email, name string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM entity_filter_rules
			WHERE tenant_id = $1
				AND (
					(email_pattern IS NOT NULL AND $2 LIKE email_pattern) OR
					(name_pattern IS NOT NULL AND $3 LIKE name_pattern)
				)
		)
	`

	var matches bool
	err := r.pool.QueryRow(ctx, query, tenantID, email, name).Scan(&matches)
	if err != nil {
		return false, fmt.Errorf("failed to check filter rules: %w", err)
	}

	return matches, nil
}

// GetEntityStats returns statistics about entities in the system.
func (r *Repository) GetEntityStats(ctx context.Context, tenantID string) (*EntityStats, error) {
	stats := &EntityStats{
		ByAccountType: make(map[AccountType]int64),
		ByConfidence:  make(map[string]int64),
	}

	// Total counts
	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE rejected_at IS NOT NULL) as rejected,
			COUNT(*) FILTER (WHERE needs_review = TRUE) as needs_review,
			COUNT(*) FILTER (WHERE auto_created = TRUE) as auto_created,
			COUNT(*) FILTER (WHERE is_internal = TRUE) as internal,
			COUNT(*) FILTER (WHERE is_internal = FALSE) as external
		FROM people
		WHERE tenant_id = $1
	`

	err := r.pool.QueryRow(ctx, query, tenantID).Scan(
		&stats.TotalPeople,
		&stats.TotalRejected,
		&stats.NeedingReview,
		&stats.AutoCreated,
		&stats.Internal,
		&stats.External,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity stats: %w", err)
	}

	// By account type
	typeQuery := `
		SELECT account_type, COUNT(*)
		FROM people
		WHERE tenant_id = $1 AND rejected_at IS NULL
		GROUP BY account_type
	`

	rows, err := r.pool.Query(ctx, typeQuery, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account type stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accountType AccountType
		var count int64
		if err := rows.Scan(&accountType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan account type: %w", err)
		}
		stats.ByAccountType[accountType] = count
	}

	// By confidence
	confQuery := `
		SELECT
			COUNT(*) FILTER (WHERE confidence_score >= 0.8) as high,
			COUNT(*) FILTER (WHERE confidence_score >= 0.5 AND confidence_score < 0.8) as medium,
			COUNT(*) FILTER (WHERE confidence_score < 0.5) as low
		FROM people
		WHERE tenant_id = $1 AND rejected_at IS NULL
	`

	var high, medium, low int64
	err = r.pool.QueryRow(ctx, confQuery, tenantID).Scan(&high, &medium, &low)
	if err != nil {
		return nil, fmt.Errorf("failed to get confidence stats: %w", err)
	}

	stats.ByConfidence["high"] = high
	stats.ByConfidence["medium"] = medium
	stats.ByConfidence["low"] = low

	return stats, nil
}

// SearchEntities searches for entities by name or email substring.
// Field can be "name", "email", or empty for both.
func (r *Repository) SearchEntities(ctx context.Context, tenantID, query, field string, limit int) ([]*Person, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var sqlQuery string
	var args []interface{}

	switch field {
	case "email":
		sqlQuery = `
			SELECT
				id, tenant_id, canonical_name, primary_email,
				job_title as title, department, company, is_internal, account_type,
				confidence_score as confidence, needs_review, auto_created,
				reviewed_at, reviewed_by,
				rejected_at, rejected_reason, rejected_by,
				potential_duplicates,
				created_at, updated_at
			FROM people
			WHERE tenant_id = $1 AND primary_email ILIKE '%' || $2 || '%'
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{tenantID, query, limit}
	case "name":
		sqlQuery = `
			SELECT
				id, tenant_id, canonical_name, primary_email,
				job_title as title, department, company, is_internal, account_type,
				confidence_score as confidence, needs_review, auto_created,
				reviewed_at, reviewed_by,
				rejected_at, rejected_reason, rejected_by,
				potential_duplicates,
				created_at, updated_at
			FROM people
			WHERE tenant_id = $1 AND canonical_name ILIKE '%' || $2 || '%'
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{tenantID, query, limit}
	default:
		// Search both fields
		sqlQuery = `
			SELECT
				id, tenant_id, canonical_name, primary_email,
				job_title as title, department, company, is_internal, account_type,
				confidence_score as confidence, needs_review, auto_created,
				reviewed_at, reviewed_by,
				rejected_at, rejected_reason, rejected_by,
				potential_duplicates,
				created_at, updated_at
			FROM people
			WHERE tenant_id = $1
				AND (canonical_name ILIKE '%' || $2 || '%' OR primary_email ILIKE '%' || $2 || '%')
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{tenantID, query, limit}
	}

	rows, err := r.pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search entities: %w", err)
	}
	defer rows.Close()

	return r.scanPeople(rows)
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
