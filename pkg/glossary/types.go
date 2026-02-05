// Package glossary provides domain terminology and acronym management for query expansion.
package glossary

import "time"

// Term represents a glossary entry for an acronym or domain-specific term.
type Term struct {
	ID             int64     `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Term           string    `json:"term"`           // The acronym/term (e.g., "TER")
	Expansion      string    `json:"expansion"`      // Full expansion (e.g., "Technical Execution Review")
	Definition     string    `json:"definition"`     // Optional longer description
	Context        []string  `json:"context"`        // Context tags (e.g., ["MTC", "meetings"])
	Aliases        []string  `json:"aliases"`        // Alternative forms (e.g., ["T.E.R."])
	ExpandInSearch bool      `json:"expand_in_search"`
	Source         string    `json:"source"` // "manual", "extracted", "suggested"

	// Linked entity - connects term to its canonical product/project/company
	LinkedEntityType *string `json:"linked_entity_type,omitempty"` // "product", "project", "company"
	LinkedEntityID   *int64  `json:"linked_entity_id,omitempty"`   // FK to the entity table

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy *string   `json:"created_by,omitempty"`
}

// TermInput is used for creating or updating a glossary term.
type TermInput struct {
	Term           string   `json:"term"`
	Expansion      string   `json:"expansion"`
	Definition     string   `json:"definition,omitempty"`
	Context        []string `json:"context,omitempty"`
	Aliases        []string `json:"aliases,omitempty"`
	ExpandInSearch *bool    `json:"expand_in_search,omitempty"` // nil means default true
	Source         string   `json:"source,omitempty"`
	CreatedBy      string   `json:"created_by,omitempty"`

	// Linked entity - connects term to its canonical product/project/company
	LinkedEntityType string `json:"linked_entity_type,omitempty"` // "product", "project", "company"
	LinkedEntityID   *int64 `json:"linked_entity_id,omitempty"`   // FK to the entity table
}

// TermFilter specifies criteria for listing/searching terms.
type TermFilter struct {
	Term       string   // Exact term match (case-insensitive)
	Search     string   // Full-text search across term, expansion, definition
	Context    []string // Filter by context tags (any match)
	Source     string   // Filter by source
	TenantID   string   // Filter by tenant
	Limit      int      // Max results (0 = default 100)
	Offset     int      // Pagination offset
	ExpandOnly bool     // Only terms with expand_in_search=true
}

// LinkedEntity represents the canonical entity a term links to.
type LinkedEntity struct {
	Type string `json:"type"` // "product", "project", "company"
	ID   int64  `json:"id"`
	Name string `json:"name,omitempty"` // Populated when fetching with entity details
}

// ExpansionResult represents a term expansion for query enhancement.
type ExpansionResult struct {
	OriginalTerm string   `json:"original_term"`
	Expansion    string   `json:"expansion"`
	Aliases      []string `json:"aliases,omitempty"`
	Definition   string   `json:"definition,omitempty"`

	// Linked entity - the canonical product/project/company this term represents
	LinkedEntity *LinkedEntity `json:"linked_entity,omitempty"`
}

// QueryExpansion contains all expansions for a query string.
type QueryExpansion struct {
	OriginalQuery  string            `json:"original_query"`
	ExpandedTerms  []ExpansionResult `json:"expanded_terms"`
	ExpandedQuery  string            `json:"expanded_query"`  // Query with expansions added
	AliasedQueries []string          `json:"aliased_queries"` // Alternative queries using aliases
}
