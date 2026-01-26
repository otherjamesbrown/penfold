package tenant

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pferrors "github.com/otherjamesbrown/penfold/pkg/errors"
)

// Repository provides access to tenant data.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new tenant repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// slugify converts a name to a URL-safe slug.
func slugify(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)
	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove non-alphanumeric characters except hyphens
	reg := regexp.MustCompile("[^a-z0-9-]+")
	slug = reg.ReplaceAllString(slug, "")
	// Remove consecutive hyphens
	reg = regexp.MustCompile("-+")
	slug = reg.ReplaceAllString(slug, "-")
	// Trim hyphens from ends
	slug = strings.Trim(slug, "-")
	// Limit length
	if len(slug) > 63 {
		slug = slug[:63]
	}
	return slug
}

// Create adds a new tenant.
func (r *Repository) Create(ctx context.Context, input TenantInput) (*Tenant, error) {
	slug := input.Slug
	if slug == "" {
		slug = slugify(input.Name)
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	settings := "{}"
	if input.Settings != nil && *input.Settings != "" {
		settings = *input.Settings
	}

	var tenant Tenant
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenants (name, slug, description, is_active, settings)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id, name, slug, description, is_active, settings::text, created_at, updated_at
	`,
		input.Name,
		slug,
		input.Description,
		isActive,
		settings,
	).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.Description,
		&tenant.IsActive,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			return nil, fmt.Errorf("%w: tenant with slug '%s' already exists", pferrors.ErrAlreadyExists, slug)
		}
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	return &tenant, nil
}

// Get retrieves a tenant by ID.
func (r *Repository) Get(ctx context.Context, id int64) (*Tenant, error) {
	var tenant Tenant
	err := r.db.QueryRow(ctx, `
		SELECT
			t.id, t.name, t.slug, COALESCE(t.description, ''), t.is_active,
			COALESCE(t.settings::text, '{}'), t.created_at, t.updated_at,
			COALESCE((SELECT COUNT(*) FROM tenant_integrations WHERE tenant_id = t.id AND enabled), 0) AS integration_count,
			COALESCE((SELECT COUNT(*) FROM tenant_processing_rules WHERE tenant_id = t.id AND enabled), 0) AS rule_count
		FROM tenants t
		WHERE t.id = $1
	`, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.Description,
		&tenant.IsActive,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.IntegrationCount,
		&tenant.RuleCount,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &tenant, nil
}

// GetBySlug retrieves a tenant by its slug.
func (r *Repository) GetBySlug(ctx context.Context, slug string) (*Tenant, error) {
	var tenant Tenant
	err := r.db.QueryRow(ctx, `
		SELECT
			t.id, t.name, t.slug, COALESCE(t.description, ''), t.is_active,
			COALESCE(t.settings::text, '{}'), t.created_at, t.updated_at,
			COALESCE((SELECT COUNT(*) FROM tenant_integrations WHERE tenant_id = t.id AND enabled), 0) AS integration_count,
			COALESCE((SELECT COUNT(*) FROM tenant_processing_rules WHERE tenant_id = t.id AND enabled), 0) AS rule_count
		FROM tenants t
		WHERE t.slug = $1
	`, slug).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.Description,
		&tenant.IsActive,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.IntegrationCount,
		&tenant.RuleCount,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant by slug: %w", err)
	}
	return &tenant, nil
}

// List retrieves tenants matching the filter.
func (r *Repository) List(ctx context.Context, filter TenantFilter) ([]*Tenant, int64, error) {
	// Build base query for count
	countQuery := `SELECT COUNT(*) FROM tenants t WHERE 1=1`
	query := `
		SELECT
			t.id, t.name, t.slug, COALESCE(t.description, ''), t.is_active,
			COALESCE(t.settings::text, '{}'), t.created_at, t.updated_at,
			COALESCE((SELECT COUNT(*) FROM tenant_integrations WHERE tenant_id = t.id AND enabled), 0) AS integration_count,
			COALESCE((SELECT COUNT(*) FROM tenant_processing_rules WHERE tenant_id = t.id AND enabled), 0) AS rule_count
		FROM tenants t
		WHERE 1=1
	`
	args := []interface{}{}
	argNum := 1

	if filter.IsActive != nil {
		condition := fmt.Sprintf(" AND t.is_active = $%d", argNum)
		query += condition
		countQuery += condition
		args = append(args, *filter.IsActive)
		argNum++
	}

	if filter.Search != "" {
		condition := fmt.Sprintf(` AND (
			t.name ILIKE $%d OR
			t.slug ILIKE $%d OR
			t.description ILIKE $%d
		)`, argNum, argNum, argNum)
		query += condition
		countQuery += condition
		searchTerm := "%" + filter.Search + "%"
		args = append(args, searchTerm)
		argNum++
	}

	// Get total count
	var totalCount int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("count tenants: %w", err)
	}

	// Add ordering and pagination
	query += " ORDER BY t.name ASC"

	limit := 50
	if filter.Limit > 0 && filter.Limit <= 1000 {
		limit = filter.Limit
	} else if filter.Limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*Tenant
	for rows.Next() {
		var tenant Tenant
		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Slug,
			&tenant.Description,
			&tenant.IsActive,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&tenant.IntegrationCount,
			&tenant.RuleCount,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, &tenant)
	}

	return tenants, totalCount, nil
}

// Update modifies an existing tenant.
func (r *Repository) Update(ctx context.Context, id int64, input TenantInput) (*Tenant, error) {
	// Get existing tenant first
	existing, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pferrors.ErrNotFound
	}

	// Apply updates
	name := existing.Name
	if input.Name != "" {
		name = input.Name
	}

	description := existing.Description
	if input.Description != "" {
		description = input.Description
	}

	isActive := existing.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	settings := existing.Settings
	if input.Settings != nil {
		settings = *input.Settings
	}

	var tenant Tenant
	err = r.db.QueryRow(ctx, `
		UPDATE tenants
		SET name = $2, description = $3, is_active = $4, settings = $5::jsonb, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, slug, COALESCE(description, ''), is_active, COALESCE(settings::text, '{}'), created_at, updated_at
	`,
		id,
		name,
		description,
		isActive,
		settings,
	).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.Description,
		&tenant.IsActive,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, pferrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update tenant: %w", err)
	}

	return &tenant, nil
}

// Delete soft-deletes a tenant by ID.
// Note: The current schema uses hard delete. For soft delete, the schema would need
// deleted_at, is_deleted, deleted_by, deletion_reason columns.
// For now, we perform a hard delete but the API indicates soft delete semantics.
func (r *Repository) Delete(ctx context.Context, id int64, reason string) error {
	// First check if tenant exists
	existing, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return pferrors.ErrNotFound
	}

	// Perform delete (hard delete for now since schema doesn't have soft delete columns)
	result, err := r.db.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}
	return nil
}

// GetByRef retrieves a tenant by either ID (as string) or slug.
func (r *Repository) GetByRef(ctx context.Context, ref string) (*Tenant, error) {
	// First try to parse as ID
	var id int64
	if _, err := fmt.Sscanf(ref, "%d", &id); err == nil {
		return r.Get(ctx, id)
	}
	// Otherwise treat as slug
	return r.GetBySlug(ctx, ref)
}
