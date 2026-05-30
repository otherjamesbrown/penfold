//go:build integration

package entities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestPeopleScanColumnParity is a regression test for PEN-17.
//
// Bug: scanPeople scans 25 destinations (including the migration-added
// communication_patterns, expertise_areas, org_position columns), but several
// SELECT queries that feed it only listed 22 columns. pgx then failed every
// row scan with "number of field descriptions must equal number of
// destinations, got 22 and 25", breaking entity search, name search, domain
// lookup, and the review queue for every tenant.
//
// This test exercises each affected function against the real migrated schema.
// Pre-fix every sub-test fails on the first row scan; post-fix they all pass.
func TestPeopleScanColumnParity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, getTestDatabaseURL())
	if err != nil {
		t.Skipf("Skipping: test database not available: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Skipping: test database not available: %v", err)
	}

	logger := logging.MustGlobal()
	repo := NewRepository(pool, logger)

	// tenant_id is a UUID column; use a fixed, test-only UUID.
	const tenantID = "0e177777-0e17-0e17-0e17-0e1700000017"
	person := &Person{
		TenantID:      tenantID,
		CanonicalName: "Penelope Seventeen",
		PrimaryEmail:  "penelope@masterofmalt.example",
		IsInternal:    false,
		AccountType:   AccountTypePerson,
		Confidence:    0.8,
		NeedsReview:   true, // so ListPeopleNeedingReview returns the row
		AutoCreated:   true,
	}

	if err := repo.CreatePerson(ctx, person); err != nil {
		t.Fatalf("Failed to create test person: %v", err)
	}
	defer cleanupPerson(t, ctx, pool, person.ID)

	// containsID asserts the inserted person was scanned back without error.
	containsID := func(t *testing.T, people []*Person, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("query failed (scan column mismatch regression?): %v", err)
		}
		for _, p := range people {
			if p.ID == person.ID {
				return
			}
		}
		t.Fatalf("expected person id=%d in results, got %d rows", person.ID, len(people))
	}

	t.Run("SearchEntities/default", func(t *testing.T) {
		got, err := repo.SearchEntities(ctx, tenantID, "Penelope", "", 50)
		containsID(t, got, err)
	})

	t.Run("SearchEntities/name", func(t *testing.T) {
		got, err := repo.SearchEntities(ctx, tenantID, "Penelope", "name", 50)
		containsID(t, got, err)
	})

	t.Run("SearchEntities/email", func(t *testing.T) {
		got, err := repo.SearchEntities(ctx, tenantID, "masterofmalt", "email", 50)
		containsID(t, got, err)
	})

	t.Run("SearchPeopleByName", func(t *testing.T) {
		got, err := repo.SearchPeopleByName(ctx, tenantID, "Penelope", 50)
		containsID(t, got, err)
	})

	t.Run("GetPeopleByDomain", func(t *testing.T) {
		got, err := repo.GetPeopleByDomain(ctx, tenantID, "masterofmalt.example")
		containsID(t, got, err)
	})

	t.Run("ListPeopleNeedingReview", func(t *testing.T) {
		got, err := repo.ListPeopleNeedingReview(ctx, tenantID, 50)
		containsID(t, got, err)
	})
}
