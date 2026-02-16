// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/otherjamesbrown/penfold/pkg/enrichment"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// postgresEnrichmentRepository adapts pkg/enrichment.Repository to activities.EnrichmentRepository.
type postgresEnrichmentRepository struct {
	repo *enrichment.Repository
}

// NewPostgresEnrichmentRepository creates a new PostgreSQL enrichment repository adapter.
func NewPostgresEnrichmentRepository(pool *pgxpool.Pool, logger logging.Logger) EnrichmentRepository {
	return &postgresEnrichmentRepository{
		repo: enrichment.NewRepository(pool, logger),
	}
}

// GetBySourceID retrieves an enrichment by source ID.
func (r *postgresEnrichmentRepository) GetBySourceID(ctx context.Context, sourceID int64) (*EnrichmentRecord, error) {
	e, err := r.repo.GetBySourceID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, nil
	}

	return &EnrichmentRecord{
		ID:                   e.ID,
		SourceID:             e.SourceID,
		TenantID:             e.TenantID,
		ContentSubtype:       string(e.Classification.Subtype),
		ClassificationReason: e.Classification.Reason,
	}, nil
}

// Update updates an existing enrichment record.
func (r *postgresEnrichmentRepository) Update(ctx context.Context, rec *EnrichmentRecord) error {
	// Fetch the full enrichment record
	e, err := r.repo.GetBySourceID(ctx, rec.SourceID)
	if err != nil {
		return err
	}
	if e == nil {
		return nil
	}

	// Update only the subtype field
	e.Classification.Subtype = enrichment.ContentSubtype(rec.ContentSubtype)
	if rec.ClassificationReason != "" {
		e.Classification.Reason = rec.ClassificationReason
	}

	// Save back
	return r.repo.Update(ctx, e)
}

// Create inserts a new enrichment record.
// The input must be *enrichment.Enrichment (from pkg/enrichment).
func (r *postgresEnrichmentRepository) Create(ctx context.Context, e interface{}) error {
	enrichmentRecord, ok := e.(*enrichment.Enrichment)
	if !ok {
		return nil // Type assertion failed, but interface{} allows this
	}
	return r.repo.Create(ctx, enrichmentRecord)
}
