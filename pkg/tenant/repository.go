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
		INSERT INTO tenants (display_name, slug, description, is_active, settings, name, owner_email)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
		RETURNING id::text, display_name, slug, COALESCE(description, ''), is_active, COALESCE(settings::text, '{}'), created_at, updated_at
	`,
		input.Name,
		slug,
		input.Description,
		isActive,
		settings,
		slug,        // Use slug as the name field (category)
		"",          // owner_email - empty for now
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

// Get retrieves a tenant by ID (UUID).
func (r *Repository) Get(ctx context.Context, id string) (*Tenant, error) {
	var tenant Tenant
	err := r.db.QueryRow(ctx, `
		SELECT
			t.id::text, t.display_name, t.slug, COALESCE(t.description, ''), t.is_active,
			COALESCE(t.settings::text, '{}'), t.created_at, t.updated_at,
			0 AS integration_count,
			0 AS rule_count
		FROM tenants t
		WHERE t.id = $1::uuid AND NOT t.is_deleted
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
			t.id::text, t.display_name, t.slug, COALESCE(t.description, ''), t.is_active,
			COALESCE(t.settings::text, '{}'), t.created_at, t.updated_at,
			0 AS integration_count,
			0 AS rule_count
		FROM tenants t
		WHERE t.slug = $1 AND NOT t.is_deleted
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
	countQuery := `SELECT COUNT(*) FROM tenants t WHERE NOT t.is_deleted`
	query := `
		SELECT
			t.id::text, t.display_name, t.slug, COALESCE(t.description, ''), t.is_active,
			COALESCE(t.settings::text, '{}'), t.created_at, t.updated_at,
			0 AS integration_count,
			0 AS rule_count
		FROM tenants t
		WHERE NOT t.is_deleted
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
			t.display_name ILIKE $%d OR
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
	query += " ORDER BY t.display_name ASC"

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
func (r *Repository) Update(ctx context.Context, id string, input TenantInput) (*Tenant, error) {
	// Get existing tenant first
	existing, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, pferrors.ErrNotFound
	}

	// Apply updates
	displayName := existing.Name
	if input.Name != "" {
		displayName = input.Name
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
		SET display_name = $2, description = $3, is_active = $4, settings = $5::jsonb, updated_at = NOW()
		WHERE id = $1::uuid AND NOT is_deleted
		RETURNING id::text, display_name, slug, COALESCE(description, ''), is_active, COALESCE(settings::text, '{}'), created_at, updated_at
	`,
		id,
		displayName,
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
func (r *Repository) Delete(ctx context.Context, id string, reason string) error {
	// First check if tenant exists
	existing, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return pferrors.ErrNotFound
	}

	// Perform soft delete using existing schema columns
	result, err := r.db.Exec(ctx, `
		UPDATE tenants
		SET is_deleted = true, deleted_at = NOW(), deletion_reason = $2
		WHERE id = $1::uuid AND NOT is_deleted
	`, id, reason)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pferrors.ErrNotFound
	}
	return nil
}

// GetByRef retrieves a tenant by either ID (UUID string) or slug.
func (r *Repository) GetByRef(ctx context.Context, ref string) (*Tenant, error) {
	// First try as UUID (36 chars with hyphens, or 32 without)
	if len(ref) == 36 || len(ref) == 32 {
		tenant, err := r.Get(ctx, ref)
		if err == nil && tenant != nil {
			return tenant, nil
		}
	}
	// Otherwise treat as slug
	return r.GetBySlug(ctx, ref)
}
