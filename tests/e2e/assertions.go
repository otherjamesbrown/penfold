//go:build e2e

package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ResolvedMention represents a mention that was resolved to an entity.
type ResolvedMention struct {
	OriginalText string
	ResolvedTo   *Entity
	Confidence   float64
	MatchType    string // "canonical", "alias", "fuzzy"
}

// Entity represents a resolved entity (person, team, project, etc.)
type Entity struct {
	ID         int64
	EntityType string // "person", "team", "project", "product"
	Name       string
}

// GlossaryMatch represents a glossary term found in content.
type GlossaryMatch struct {
	Term      string
	Expansion string
	Context   string
}

// SearchResult represents a search result.
type SearchResult struct {
	DocumentID  string
	ContentType string
	Score       float64
	Snippet     string
}

// IngestResult contains the result of ingesting content.
type IngestResult struct {
	ContentID        string
	ResolvedMentions []ResolvedMention
	GlossaryMatches  []GlossaryMatch
	ProcessingTimeMS int64
}

// Assertions provides semantic assertion helpers for E2E tests.
type Assertions struct {
	t *testing.T
}

// NewAssertions creates a new Assertions helper.
func NewAssertions(t *testing.T) *Assertions {
	return &Assertions{t: t}
}

// AssertMentionResolved verifies a mention was resolved to the expected person.
func (a *Assertions) AssertMentionResolved(mention ResolvedMention, expectedPersonID int64) {
	a.t.Helper()

	assert.NotNil(a.t, mention.ResolvedTo, "mention '%s' should be resolved", mention.OriginalText)
	if mention.ResolvedTo != nil {
		assert.Equal(a.t, expectedPersonID, mention.ResolvedTo.ID,
			"mention '%s' should resolve to person %d", mention.OriginalText, expectedPersonID)
	}
}

// AssertMentionNotResolved verifies a mention was NOT resolved.
func (a *Assertions) AssertMentionNotResolved(mention ResolvedMention) {
	a.t.Helper()

	assert.Nil(a.t, mention.ResolvedTo,
		"mention '%s' should not be resolved", mention.OriginalText)
}

// AssertMentionConfidence verifies a mention has at least the expected confidence.
func (a *Assertions) AssertMentionConfidence(mention ResolvedMention, minConfidence float64) {
	a.t.Helper()

	assert.GreaterOrEqual(a.t, mention.Confidence, minConfidence,
		"mention '%s' should have confidence >= %.2f, got %.2f",
		mention.OriginalText, minConfidence, mention.Confidence)
}

// AssertGlossaryExpanded verifies a glossary term was expanded correctly.
func (a *Assertions) AssertGlossaryExpanded(matches []GlossaryMatch, term, expansion string) {
	a.t.Helper()

	var found *GlossaryMatch
	for i := range matches {
		if matches[i].Term == term {
			found = &matches[i]
			break
		}
	}

	assert.NotNil(a.t, found, "glossary term '%s' should be in matches", term)
	if found != nil {
		assert.Equal(a.t, expansion, found.Expansion,
			"glossary term '%s' should expand to '%s'", term, expansion)
	}
}

// AssertGlossaryNotExpanded verifies a term was NOT found in glossary matches.
func (a *Assertions) AssertGlossaryNotExpanded(matches []GlossaryMatch, term string) {
	a.t.Helper()

	for _, match := range matches {
		if match.Term == term {
			a.t.Errorf("glossary term '%s' should not be in matches", term)
			return
		}
	}
}

// AssertSearchContains verifies search results contain the expected document.
func (a *Assertions) AssertSearchContains(results []SearchResult, documentID string) {
	a.t.Helper()

	for _, result := range results {
		if result.DocumentID == documentID {
			return
		}
	}
	a.t.Errorf("search results should contain document '%s'", documentID)
}

// AssertSearchRankedHigher verifies one document ranks higher than another.
func (a *Assertions) AssertSearchRankedHigher(results []SearchResult, higherID, lowerID string) {
	a.t.Helper()

	higherIdx := -1
	lowerIdx := -1

	for i, result := range results {
		if result.DocumentID == higherID {
			higherIdx = i
		}
		if result.DocumentID == lowerID {
			lowerIdx = i
		}
	}

	if higherIdx == -1 {
		a.t.Errorf("document '%s' not found in results", higherID)
		return
	}
	if lowerIdx == -1 {
		a.t.Errorf("document '%s' not found in results", lowerID)
		return
	}

	assert.Less(a.t, higherIdx, lowerIdx,
		"document '%s' (idx=%d) should rank higher than '%s' (idx=%d)",
		higherID, higherIdx, lowerID, lowerIdx)
}

// AssertSearchScoreAbove verifies a document has at least the expected score.
func (a *Assertions) AssertSearchScoreAbove(results []SearchResult, documentID string, minScore float64) {
	a.t.Helper()

	for _, result := range results {
		if result.DocumentID == documentID {
			assert.GreaterOrEqual(a.t, result.Score, minScore,
				"document '%s' should have score >= %.2f, got %.2f",
				documentID, minScore, result.Score)
			return
		}
	}
	a.t.Errorf("document '%s' not found in results", documentID)
}

// AssertEntityLinked verifies an entity was linked in the ingest result.
func (a *Assertions) AssertEntityLinked(result *IngestResult, entityType string, entityID int64) {
	a.t.Helper()

	for _, mention := range result.ResolvedMentions {
		if mention.ResolvedTo != nil &&
			mention.ResolvedTo.EntityType == entityType &&
			mention.ResolvedTo.ID == entityID {
			return
		}
	}
	a.t.Errorf("entity %s:%d should be linked in result", entityType, entityID)
}
