// Package activities provides activity implementations for the Temporal worker.
package activities

import (
	"context"
	"fmt"

	"github.com/otherjamesbrown/penfold/pkg/enrichment/entities"
)

// PersonLookupAdapter wraps entities.Repository and implements the PersonLookup interface (pf-2059f5).
type PersonLookupAdapter struct {
	repo *entities.Repository
}

// NewPersonLookupAdapter creates a PersonLookupAdapter from an entities.Repository.
func NewPersonLookupAdapter(repo *entities.Repository) *PersonLookupAdapter {
	return &PersonLookupAdapter{repo: repo}
}

// GetPeopleByEmails looks up people by primary email and converts entities.Person to PersonInfo.
func (a *PersonLookupAdapter) GetPeopleByEmails(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error) {
	people, err := a.repo.GetPeopleByEmails(ctx, tenantID, emails)
	if err != nil {
		return nil, fmt.Errorf("person lookup: %w", err)
	}

	result := make(map[string]*PersonInfo, len(people))
	for email, p := range people {
		result[email] = &PersonInfo{
			ID:            p.ID,
			CanonicalName: p.CanonicalName,
			PrimaryEmail:  p.PrimaryEmail,
			Title:         p.Title,
			Department:    p.Department,
			IsInternal:    p.IsInternal,
		}
	}
	return result, nil
}

// GetNameAliasesByPersonIDs delegates directly to the underlying repository.
func (a *PersonLookupAdapter) GetNameAliasesByPersonIDs(ctx context.Context, personIDs []int64) (map[int64][]string, error) {
	return a.repo.GetNameAliasesByPersonIDs(ctx, personIDs)
}

// GetMetadataByPersonIDs delegates directly to the underlying repository.
func (a *PersonLookupAdapter) GetMetadataByPersonIDs(ctx context.Context, personIDs []int64) (map[int64]map[string]string, error) {
	return a.repo.GetMetadataByPersonIDs(ctx, personIDs)
}
