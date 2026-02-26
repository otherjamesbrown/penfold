package config

import (
	"os"

	"github.com/google/uuid"
)

const (
	defaultTenantUUID = "c3170310-78bd-409c-b186-126f40bfa6ad"
	safetyTenantUUID  = "00000001-0000-0000-0000-000000000001"
)

// DefaultTenantID returns the production tenant UUID.
// Reads PENFOLD_DEFAULT_TENANT_ID env var if set, otherwise falls back to
// the canonical production tenant.
func DefaultTenantID() uuid.UUID {
	if v := os.Getenv("PENFOLD_DEFAULT_TENANT_ID"); v != "" {
		return uuid.MustParse(v)
	}
	return uuid.MustParse(defaultTenantUUID)
}

// SafetyTenantID returns the safety/test tenant UUID.
// Operations that lack explicit tenant context fall back to this UUID to
// prevent accidental writes to production tenant data. This is intentional —
// do not replace with DefaultTenantID().
func SafetyTenantID() uuid.UUID {
	return uuid.MustParse(safetyTenantUUID)
}
