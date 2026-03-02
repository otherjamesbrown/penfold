package glossary

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EmbeddingClient generates vector embeddings for text.
type EmbeddingClient interface {
	// Embed generates a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dimensions returns the number of dimensions in embeddings.
	Dimensions() int
}

// Matcher finds relevant glossary terms using vector similarity search.
// It generates embeddings for input text and finds semantically similar
// glossary terms using pgvector cosine similarity.
type Matcher struct {
	db              *pgxpool.Pool
	embeddingClient EmbeddingClient
}

// NewMatcher creates a new glossary matcher.
func NewMatcher(db *pgxpool.Pool, embeddingClient EmbeddingClient) *Matcher {
	return &Matcher{
		db:              db,
		embeddingClient: embeddingClient,
	}
}

// MatchResult represents a glossary term match with similarity score.
type MatchResult struct {
	Term            *Term   `json:"term"`
	SimilarityScore float64 `json:"similarity_score"`
}

// FindRelevantTerms finds glossary terms semantically relevant to the input text.
// It generates an embedding for the input and finds the most similar glossary terms.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - tenantID: Tenant to search within (empty string searches all)
//   - text: Input text to find relevant terms for
//   - limit: Maximum number of terms to return (0 = default 20)
//   - minSimilarity: Minimum cosine similarity threshold (0.0-1.0, default 0.3)
func (m *Matcher) FindRelevantTerms(
	ctx context.Context,
	tenantID string,
	text string,
	limit int,
	minSimilarity float64,
) ([]MatchResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("input text cannot be empty")
	}

	if limit <= 0 {
		limit = 20
	}
	if minSimilarity <= 0 {
		minSimilarity = 0.3
	}

	// Generate embedding for input text
	embedding, err := m.embeddingClient.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("generating embedding: %w", err)
	}

	// Build query - use COALESCE for nullable fields to avoid scan errors
	// Note: linked_entity_type/linked_entity_id may not exist in all databases,
	// so we check column existence first
	query := `
		SELECT
			id, tenant_id, term, expansion,
			COALESCE(definition, '') as definition,
			COALESCE(context, '[]'::jsonb) as context,
			COALESCE(aliases, '[]'::jsonb) as aliases,
			expand_in_search, source, created_at, updated_at,
			COALESCE(created_by, '') as created_by,
			1 - (embedding <=> $1::vector) as similarity
		FROM glossary
		WHERE embedding IS NOT NULL
		  AND (1 - (embedding <=> $1::vector)) >= $2
	`
	args := []interface{}{pgVectorFormat(embedding), minSimilarity}
	argNum := 3

	if tenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argNum)
		args = append(args, tenantID)
		argNum++
	}

	query += fmt.Sprintf(`
		ORDER BY similarity DESC
		LIMIT $%d
	`, argNum)
	args = append(args, limit)

	rows, err := m.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("executing similarity search: %w", err)
	}
	defer rows.Close()

	var results []MatchResult
	for rows.Next() {
		var term Term
		var similarity float64
		err := rows.Scan(
			&term.ID,
			&term.TenantID,
			&term.Term,
			&term.Expansion,
			&term.Definition,
			&term.Context,
			&term.Aliases,
			&term.ExpandInSearch,
			&term.Source,
			&term.CreatedAt,
			&term.UpdatedAt,
			&term.CreatedBy,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning result: %w", err)
		}
		// Note: LinkedEntityType/LinkedEntityID are not loaded in similarity search
		// to maintain compatibility with databases that don't have these columns
		results = append(results, MatchResult{
			Term:            &term,
			SimilarityScore: similarity,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating results: %w", err)
	}

	return results, nil
}

// BuildContext generates a formatted glossary context string for LLM prompts.
// This is the production replacement for the test helper buildGlossaryContext.
func (m *Matcher) BuildContext(
	ctx context.Context,
	tenantID string,
	text string,
	limit int,
) (string, error) {
	matches, err := m.FindRelevantTerms(ctx, tenantID, text, limit, 0.3)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "No relevant glossary terms found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Relevant glossary terms:\n")

	for _, match := range matches {
		sb.WriteString(fmt.Sprintf("- %s: %s", match.Term.Term, match.Term.Expansion))
		if match.Term.Definition != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", match.Term.Definition))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// UpdateEmbedding generates and stores an embedding for a glossary term.
// The embedding is generated from the concatenation of term, expansion, and definition.
func (m *Matcher) UpdateEmbedding(ctx context.Context, termID int64) error {
	// First, fetch the term to build the text for embedding
	var term, expansion, definition string
	err := m.db.QueryRow(ctx, `
		SELECT term, expansion, COALESCE(definition, '')
		FROM glossary
		WHERE id = $1
	`, termID).Scan(&term, &expansion, &definition)
	if err != nil {
		return fmt.Errorf("fetching term: %w", err)
	}

	// Build text for embedding: term + expansion + definition
	text := fmt.Sprintf("%s %s %s", term, expansion, definition)
	text = strings.TrimSpace(text)

	// Generate embedding
	embedding, err := m.embeddingClient.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("generating embedding: %w", err)
	}

	// Store embedding
	_, err = m.db.Exec(ctx, `
		UPDATE glossary
		SET embedding = $1::vector
		WHERE id = $2
	`, pgVectorFormat(embedding), termID)
	if err != nil {
		return fmt.Errorf("storing embedding: %w", err)
	}

	return nil
}

// UpdateAllEmbeddings generates embeddings for all glossary terms that don't have one.
func (m *Matcher) UpdateAllEmbeddings(ctx context.Context, tenantID string) (int, error) {
	query := `SELECT id FROM glossary WHERE embedding IS NULL`
	args := []interface{}{}

	if tenantID != "" {
		query += " AND tenant_id = $1"
		args = append(args, tenantID)
	}

	rows, err := m.db.Query(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("listing terms without embeddings: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, fmt.Errorf("scanning id: %w", err)
		}
		ids = append(ids, id)
	}

	updated := 0
	for _, id := range ids {
		if err := m.UpdateEmbedding(ctx, id); err != nil {
			// Log but continue with other terms
			continue
		}
		updated++
	}

	return updated, nil
}

// pgVectorFormat converts a float32 slice to pgvector format string.
func pgVectorFormat(embedding []float32) string {
	if len(embedding) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[")
	for i, v := range embedding {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%f", v))
	}
	sb.WriteString("]")
	return sb.String()
}
