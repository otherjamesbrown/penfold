package entities

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPeopleSelectColumnConsistency is a fast, DB-free regression guard for
// PEN-17. It catches the class of bug where a SELECT feeding the full Person
// scanner (scanPeople / scanPerson) drifts out of sync with the scan
// destinations after a migration adds columns.
//
// Invariant: every "SELECT ... FROM people" block that lists the message-count
// columns (sent_count/received_count) is scanning a full Person row, so it MUST
// also list the later entity-model columns the scanner reads:
// communication_patterns, expertise_areas, org_position. Aggregate/context
// queries (COUNT(*), id+name-only) don't select sent_count and are ignored.
//
// Pre-fix, four functions (SearchEntities x3 branches, SearchPeopleByName,
// GetPeopleByDomain, ListPeopleNeedingReview) listed 22 columns and triggered
// pgx "got 22 and 25" at scan time. This test fails for each such query.
func TestPeopleSelectColumnConsistency(t *testing.T) {
	projectRoot := findProjectRoot(t)
	repoPath := filepath.Join(projectRoot, "pkg", "enrichment", "entities", "repository.go")

	content, err := os.ReadFile(repoPath)
	require.NoError(t, err, "failed to read repository.go")
	src := string(content)

	// Match each `SELECT ... FROM people` block (non-greedy, across newlines).
	selectRe := regexp.MustCompile(`(?is)SELECT(.*?)FROM\s+people`)
	matches := selectRe.FindAllStringSubmatch(src, -1)
	require.NotEmpty(t, matches, "expected at least one SELECT ... FROM people in repository.go")

	driftColumns := []string{"communication_patterns", "expertise_areas", "org_position"}

	checked := 0
	for _, m := range matches {
		columns := m[1]
		// Only full-Person scans select the message-count columns.
		if !strings.Contains(columns, "sent_count") {
			continue
		}
		checked++
		for _, col := range driftColumns {
			require.Containsf(t, columns, col,
				"a SELECT ... FROM people scanning a full Person row is missing %q "+
					"(column/scan drift — see PEN-17). Offending SELECT list:\n%s",
				col, strings.TrimSpace(columns))
		}
	}

	require.Positive(t, checked, "expected to find full-Person SELECT queries to validate")
	t.Logf("validated %d full-Person SELECT queries for column/scan parity", checked)
}
