// Package tenant provides multi-tenant management for Penfold.
package tenant

import "time"

// Tenant represents a tenant organization in the system.
type Tenant struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	Description      string    `json:"description,omitempty"`
	IsActive         bool      `json:"is_active"`
	Settings         string    `json:"settings,omitempty"` // JSON string
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	IntegrationCount int32     `json:"integration_count,omitempty"`
	RuleCount        int32     `json:"rule_count,omitempty"`
}

// TenantInput is used for creating or updating a tenant.
type TenantInput struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug,omitempty"` // Optional - generated from name if empty
	Description string  `json:"description,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	Settings    *string `json:"settings,omitempty"`
}

// TenantFilter specifies criteria for listing/searching tenants.
type TenantFilter struct {
	Search   string // Full-text search across name, slug, description
	IsActive *bool  // Filter by active status
	Limit    int    // Max results (0 = default 50)
	Offset   int    // Pagination offset
}
