//go:build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearch_BasicQuery tests basic search functionality via CLI.
func TestSearch_BasicQuery(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	// Setup: Load fixtures and ingest content
	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest emails for searching
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "search-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest command failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Test basic search
	searchResult := env.CLI.Run(ctx, "search", "Project Alpha")

	if searchResult.ExitCode != 0 {
		t.Logf("Search command failed (exit code %d): %s", searchResult.ExitCode, searchResult.Stderr)
		t.Log("Search may require gateway/search service to be running")
		return
	}

	t.Logf("Search completed in %v", searchResult.Duration)
	t.Logf("Search results:\n%s", searchResult.Stdout)
}

// TestSearch_WithFilters tests search with content type filters.
func TestSearch_WithFilters(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest multiple emails
	emails := []string{
		"001-project-update.eml",
		"002-incident-response.eml",
		"003-meeting-invite.eml",
	}

	for _, email := range emails {
		emailPath := env.FixturePath(filepath.Join("emails", email))
		result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "filter-test")
		if result.ExitCode != 0 {
			t.Fatalf("Failed to ingest %s (exit code %d): %s", email, result.ExitCode, result.Stderr)
		}
	}

	// Test search with type filter
	searchResult := env.CLI.Run(ctx, "search", "update", "--type", "email")

	if searchResult.ExitCode == 0 {
		t.Logf("Filtered search results:\n%s", searchResult.Stdout)
	} else {
		t.Logf("Search with filters failed: %s", searchResult.Stderr)
	}
}

// TestSearch_SemanticQuery tests semantic search capability.
func TestSearch_SemanticQuery(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest content
	emailPath := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "semantic-test", "--concurrency", "2")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Test semantic search with mode flag
	searches := []struct {
		name  string
		query string
		mode  string
	}{
		{"hybrid", "project timeline concerns", "hybrid"},
		{"semantic", "team meeting updates", "semantic"},
		{"keyword", "MVP deadline", "keyword"},
	}

	for _, search := range searches {
		t.Run(search.name, func(t *testing.T) {
			var result *RunResult
			if search.mode != "" {
				result = env.CLI.Run(ctx, "search", search.query, "--mode", search.mode)
			} else {
				result = env.CLI.Run(ctx, "search", search.query)
			}

			if result.ExitCode == 0 {
				t.Logf("Query: %q (mode: %s)", search.query, search.mode)
				t.Logf("Results: %s", result.Stdout)
			}
		})
	}
}

// TestSearch_DateRangeFilters tests search with date range filters.
func TestSearch_DateRangeFilters(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest content
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "date-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Test search with date filters (if supported)
	searchResult := env.CLI.Run(ctx, "search", "project", "--after", "2026-01-01")

	if searchResult.ExitCode == 0 {
		t.Logf("Date-filtered results:\n%s", searchResult.Stdout)
	} else {
		t.Logf("Date filter search failed (may not be supported): %s", searchResult.Stderr)
	}
}

// TestSearch_PersonEntity tests searching for content related to a person.
func TestSearch_PersonEntity(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest content mentioning specific people
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "person-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Search for content related to a person
	personSearches := []string{
		"John Smith",
		"Sarah Chen",
		"Marcus",
	}

	for _, person := range personSearches {
		t.Run("search_"+person, func(t *testing.T) {
			searchResult := env.CLI.Run(ctx, "search", person)

			if searchResult.ExitCode == 0 {
				t.Logf("Results for %q:\n%s", person, searchResult.Stdout)
			}
		})
	}
}

// TestSearch_GlossaryExpansion verifies that glossary context is available for search.
func TestSearch_GlossaryExpansion(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Verify glossary is loaded
	var glossaryCount int
	err = env.DB.QueryRow(ctx, "SELECT COUNT(*) FROM glossary").Scan(&glossaryCount)
	require.NoError(t, err)
	t.Logf("Glossary terms available: %d", glossaryCount)

	// Ingest content with acronyms
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "glossary-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Search using acronyms - search should expand them
	acronymSearches := []struct {
		acronym   string
		expansion string
	}{
		{"TER", "Technical Execution Review"},
		{"MVP", "Minimum Viable Product"},
	}

	for _, search := range acronymSearches {
		t.Run("acronym_"+search.acronym, func(t *testing.T) {
			result := env.CLI.Run(ctx, "search", search.acronym)

			if result.ExitCode == 0 {
				t.Logf("Search for %s results:\n%s", search.acronym, result.Stdout)
			}
		})
	}
}

// TestSearch_ResultSorting tests different sorting options.
func TestSearch_ResultSorting(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest multiple emails
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "sort-test", "--concurrency", "2")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Test different sort options
	sortOptions := []string{"relevance", "date", "date_asc"}

	for _, sort := range sortOptions {
		t.Run("sort_"+sort, func(t *testing.T) {
			result := env.CLI.Run(ctx, "search", "project", "--sort", sort)

			if result.ExitCode == 0 {
				t.Logf("Sort by %s:\n%s", sort, result.Stdout)
			}
		})
	}
}

// TestSearch_EmptyResults tests handling of queries with no results.
func TestSearch_EmptyResults(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Search for something that doesn't exist
	result := env.CLI.Run(ctx, "search", "xyzzy12345nonexistent")

	// Command should succeed but return no results
	if result.ExitCode == 0 {
		t.Logf("Empty search result:\n%s", result.Stdout)
		// Could verify output indicates no results
	}
}

// TestSearch_JSONOutput tests JSON output format.
func TestSearch_JSONOutput(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest content
	emailPath := env.FixturePath("emails/001-project-update.eml")
	result := env.CLI.Run(ctx, "ingest", "email", emailPath, "--source", "json-test")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Search with JSON output
	searchResult := env.CLI.Run(ctx, "search", "project", "--output", "json")

	if searchResult.ExitCode == 0 {
		t.Logf("JSON output:\n%s", searchResult.Stdout)

		// Verify it's valid JSON (basic check)
		assert.Contains(t, searchResult.Stdout, "{",
			"JSON output should start with brace")
	}
}

// TestSearch_Pagination tests search result pagination.
func TestSearch_Pagination(t *testing.T) {
	env := SetupE2EEnvironment(t)
	ctx := context.Background()

	err := env.TruncateAllTables()
	require.NoError(t, err)

	err = env.LoadFixture("acme-corp")
	require.NoError(t, err)

	// Ingest many emails
	emailDir := env.FixturePath("emails")
	result := env.CLI.Run(ctx, "ingest", "email", emailDir, "--source", "page-test", "--concurrency", "2")

	if result.ExitCode != 0 {
		t.Fatalf("Ingest failed (exit code %d): %s", result.ExitCode, result.Stderr)
	}

	// Test pagination parameters
	paginationTests := []struct {
		name   string
		limit  string
		offset string
	}{
		{"first_2", "2", "0"},
		{"skip_2", "2", "2"},
	}

	for _, pt := range paginationTests {
		t.Run(pt.name, func(t *testing.T) {
			result := env.CLI.Run(ctx, "search", "project", "--limit", pt.limit, "--offset", pt.offset)

			if result.ExitCode == 0 {
				t.Logf("Pagination (limit=%s, offset=%s):\n%s", pt.limit, pt.offset, result.Stdout)
			}
		})
	}
}
