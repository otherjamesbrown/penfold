//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/otherjamesbrown/penfold/pkg/glossary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlossaryRepository_ScanNullDefinition reproduces bug pf-9fd01b:
// GetByTerm() crashes on NULL definition with:
// "can't scan into dest[4] (col: definition): cannot scan NULL into *string"
//
// This test MUST FAIL against unfixed code to be valid.
func TestGlossaryRepository_ScanNullDefinition(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()

	// Insert a term directly with NULL definition
	// (Can't use repo.Create() because it would set definition to empty string)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, created_at, updated_at)
		VALUES ($1, 'NULLTEST', 'Null Test Term', NULL, '["test"]'::jsonb, NOW(), NOW())
	`, db.TenantID)
	require.NoError(t, err, "Failed to insert term with NULL definition")

	// This should NOT crash - but the unfixed code will crash with:
	// "can't scan into dest[4] (col: definition): cannot scan NULL into *string"
	repo := glossary.NewRepository(db.Pool)
	term, err := repo.GetByTerm(ctx, db.TenantID, "NULLTEST")
	require.NoError(t, err, "GetByTerm should handle NULL definition without crashing")
	require.NotNil(t, term, "Term should be found")
	assert.Equal(t, "NULLTEST", term.Term)
	assert.Equal(t, "Null Test Term", term.Expansion)
	assert.Empty(t, term.Definition, "NULL definition should map to empty string")
}

// TestGlossaryRepository_ScanBareStringContext reproduces bug pf-9fd01b:
// GetByTerm() crashes on bare-string context JSONB with:
// "can't scan into dest[5] (col: context): json: cannot unmarshal string into Go value of type []string"
//
// This test MUST FAIL against unfixed code to be valid.
func TestGlossaryRepository_ScanBareStringContext(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()

	// Insert a term with bare-string context (not an array)
	// This can happen from old migrations or manual SQL inserts
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, created_at, updated_at)
		VALUES ($1, 'BARESTR', 'Bare String Test', 'Test definition', '"meetings"'::jsonb, NOW(), NOW())
	`, db.TenantID)
	require.NoError(t, err, "Failed to insert term with bare-string context")

	// This should NOT crash - but the unfixed code will crash with:
	// "can't scan into dest[5] (col: context): json: cannot unmarshal string into Go value of type []string"
	repo := glossary.NewRepository(db.Pool)
	term, err := repo.GetByTerm(ctx, db.TenantID, "BARESTR")
	require.NoError(t, err, "GetByTerm should handle bare-string context without crashing")
	require.NotNil(t, term, "Term should be found")
	assert.Equal(t, "BARESTR", term.Term)
	assert.Equal(t, "Bare String Test", term.Expansion)
	// The bare string should be converted to a single-element array
	assert.Equal(t, []string{"meetings"}, term.Context)
}

// TestGlossaryRepository_Get_ScanEdgeCases verifies Get() handles NULL/bare-string edge cases
func TestGlossaryRepository_Get_ScanEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Insert term with NULL definition and bare-string context
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, created_at, updated_at)
		VALUES ($1, 'GETTEST', 'Get Test Term', NULL, '"bare-context"'::jsonb, NOW(), NOW())
		RETURNING id
	`, db.TenantID)
	require.NoError(t, err)

	// Get the term by querying for its ID
	var termID int64
	err = db.Pool.QueryRow(ctx, `SELECT id FROM glossary WHERE tenant_id = $1 AND term = 'GETTEST'`, db.TenantID).Scan(&termID)
	require.NoError(t, err)

	// This should NOT crash - but the unfixed code will crash
	term, err := repo.Get(ctx, termID, db.TenantID)
	require.NoError(t, err, "Get should handle NULL definition and bare-string context")
	require.NotNil(t, term)
	assert.Equal(t, "GETTEST", term.Term)
	assert.Empty(t, term.Definition)
	assert.Equal(t, []string{"bare-context"}, term.Context)
}

// TestGlossaryRepository_Update_ScanEdgeCases verifies Update() handles NULL/bare-string edge cases
func TestGlossaryRepository_Update_ScanEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Create a normal term
	created, err := repo.Create(ctx, glossary.TermInput{
		Term:       "UPDATETEST",
		Expansion:  "Update Test",
		Definition: "Original",
	})
	require.NoError(t, err)

	// Manually corrupt the DB to set NULL definition and bare-string context
	_, err = db.Pool.Exec(ctx, `
		UPDATE glossary SET definition = NULL, context = '"corrupted"'::jsonb WHERE id = $1
	`, created.ID)
	require.NoError(t, err)

	// Now call Update() - it returns the updated row which should NOT crash when scanning
	updated, err := repo.Update(ctx, created.ID, glossary.TermInput{
		Term:       "UPDATETEST",
		Expansion:  "Update Test Modified",
		Definition: "New definition",
		Context:    []string{"fixed"},
	})
	require.NoError(t, err, "Update should handle scanning the RETURNING row with NULL/bare-string data")
	require.NotNil(t, updated)
	assert.Equal(t, "New definition", updated.Definition)
	assert.Equal(t, []string{"fixed"}, updated.Context)
}

// TestGlossaryRepository_LookupTerm_ScanEdgeCases verifies LookupTerm() handles NULL/bare-string edge cases
func TestGlossaryRepository_LookupTerm_ScanEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Insert term with NULL definition and bare-string context/aliases
	// Note: aliases must be a JSONB array containing the alias string for @> operator to work
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, aliases, created_at, updated_at)
		VALUES ($1, 'LOOKUPTEST', 'Lookup Test', NULL, '"meetings"'::jsonb, '["alias1"]'::jsonb, NOW(), NOW())
	`, db.TenantID)
	require.NoError(t, err)

	// LookupTerm searches by alias - should NOT crash
	// It first tries GetByTerm (which fails on NULL), then tries alias query (which also has scan bug)
	term, err := repo.LookupTerm(ctx, db.TenantID, "alias1")
	require.NoError(t, err, "LookupTerm should handle NULL definition and bare-string context/aliases")
	require.NotNil(t, term)
	assert.Equal(t, "LOOKUPTEST", term.Term)
	assert.Empty(t, term.Definition)
	assert.Equal(t, []string{"meetings"}, term.Context)
	assert.Contains(t, term.Aliases, "alias1")
}

// TestGlossaryRepository_LinkEntity_ScanEdgeCases verifies LinkEntity() handles NULL/bare-string edge cases
func TestGlossaryRepository_LinkEntity_ScanEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Insert term with NULL definition and bare-string context
	var termID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, created_at, updated_at)
		VALUES ($1, 'LINKTEST', 'Link Test', NULL, '"project"'::jsonb, NOW(), NOW())
		RETURNING id
	`, db.TenantID).Scan(&termID)
	require.NoError(t, err)

	// LinkEntity returns the updated row - should NOT crash when scanning
	linked, err := repo.LinkEntity(ctx, termID, db.TenantID, "product", 42)
	require.NoError(t, err, "LinkEntity should handle scanning RETURNING row with NULL/bare-string data")
	require.NotNil(t, linked)
	assert.Equal(t, "LINKTEST", linked.Term)
	assert.Empty(t, linked.Definition)
	assert.Equal(t, []string{"project"}, linked.Context)
	assert.NotNil(t, linked.LinkedEntityType)
	assert.Equal(t, "product", *linked.LinkedEntityType)
}

// TestGlossaryRepository_UnlinkEntity_ScanEdgeCases verifies UnlinkEntity() handles NULL/bare-string edge cases
func TestGlossaryRepository_UnlinkEntity_ScanEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Insert linked term with NULL definition and bare-string context
	var termID int64
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, linked_entity_type, linked_entity_id, created_at, updated_at)
		VALUES ($1, 'UNLINKTEST', 'Unlink Test', NULL, '"project"'::jsonb, 'product', 123, NOW(), NOW())
		RETURNING id
	`, db.TenantID).Scan(&termID)
	require.NoError(t, err)

	// UnlinkEntity returns the updated row - should NOT crash when scanning
	unlinked, err := repo.UnlinkEntity(ctx, termID, db.TenantID)
	require.NoError(t, err, "UnlinkEntity should handle scanning RETURNING row with NULL/bare-string data")
	require.NotNil(t, unlinked)
	assert.Equal(t, "UNLINKTEST", unlinked.Term)
	assert.Empty(t, unlinked.Definition)
	assert.Equal(t, []string{"project"}, unlinked.Context)
	assert.Nil(t, unlinked.LinkedEntityType)
}

// TestGlossaryRepository_ListLinked_ScanEdgeCases verifies ListLinked() handles NULL/bare-string edge cases
func TestGlossaryRepository_ListLinked_ScanEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Insert linked terms with NULL definition and bare-string context
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, linked_entity_type, linked_entity_id, created_at, updated_at)
		VALUES
			($1, 'LINK1', 'Link One', NULL, '"product"'::jsonb, 'product', 1, NOW(), NOW()),
			($1, 'LINK2', 'Link Two', NULL, '"project"'::jsonb, 'project', 2, NOW(), NOW())
	`, db.TenantID)
	require.NoError(t, err)

	// ListLinked should NOT crash when scanning rows with NULL/bare-string data
	terms, count, err := repo.ListLinked(ctx, glossary.LinkedTermFilter{TenantID: db.TenantID})
	require.NoError(t, err, "ListLinked should handle NULL definition and bare-string context")
	assert.Equal(t, int64(2), count)
	assert.Len(t, terms, 2)

	for _, term := range terms {
		assert.Empty(t, term.Definition, "NULL definition should map to empty string")
		assert.NotEmpty(t, term.Context, "Bare-string context should be converted to array")
	}
}

// TestGlossaryRepository_List_HandlesEdgeCases verifies List() already handles edge cases correctly
// This test should PASS even on unfixed code, proving List() has the correct pattern
func TestGlossaryRepository_List_HandlesEdgeCases(t *testing.T) {
	db := SetupTestDB(t)
	db.EnsureTenantExists(t)
	db.TruncateTables(t, "glossary")

	ctx := context.Background()
	repo := glossary.NewRepository(db.Pool)

	// Insert terms with NULL definition and bare-string context
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO glossary (tenant_id, term, expansion, definition, context, created_at, updated_at)
		VALUES
			($1, 'NULLDEF', 'Null Definition Test', NULL, '["test"]'::jsonb, NOW(), NOW()),
			($1, 'BARESTR', 'Bare String Test', 'Has definition', '"meetings"'::jsonb, NOW(), NOW())
	`, db.TenantID)
	require.NoError(t, err)

	// List() already uses sql.NullString and []byte - should NOT crash
	terms, err := repo.List(ctx, glossary.TermFilter{TenantID: db.TenantID})
	require.NoError(t, err, "List should handle NULL definition and bare-string context")
	assert.Len(t, terms, 2)

	// Verify the correct handling
	for _, term := range terms {
		if term.Term == "NULLDEF" {
			assert.Empty(t, term.Definition)
			assert.Equal(t, []string{"test"}, term.Context)
		}
		if term.Term == "BARESTR" {
			assert.Equal(t, "Has definition", term.Definition)
			assert.Equal(t, []string{"meetings"}, term.Context)
		}
	}
}
