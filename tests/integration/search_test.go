//go:build integration

package integration

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVectorSearch_CreateEmbedding(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Check if pgvector is available
	var extExists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
	`).Scan(&extExists)
	require.NoError(t, err)

	if !extExists {
		t.Skip("pgvector extension not installed - skipping vector tests")
	}

	// Create a test table for embeddings
	_, err = db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_embeddings (
			id SERIAL PRIMARY KEY,
			content_id INT NOT NULL,
			embedding vector(384),
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_embeddings")
	})

	// Insert test embeddings (384-dimensional like mxbai-embed-large)
	testVectors := []struct {
		contentID int
		embedding string // Simplified 3D vectors for testing (will use actual dimensions in real tests)
	}{
		{1, generateTestVector(384, 0.1)},
		{2, generateTestVector(384, 0.2)},
		{3, generateTestVector(384, 0.9)},
	}

	for _, tv := range testVectors {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO test_embeddings (content_id, embedding)
			VALUES ($1, $2::vector)
		`, tv.contentID, tv.embedding)
		require.NoError(t, err)
	}

	// Verify insertions
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_embeddings").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestVectorSearch_SimilarityQuery(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Check if pgvector is available
	var extExists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
	`).Scan(&extExists)
	require.NoError(t, err)

	if !extExists {
		t.Skip("pgvector extension not installed - skipping vector tests")
	}

	// Create test table
	_, err = db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_similarity (
			id SERIAL PRIMARY KEY,
			name TEXT,
			embedding vector(3)
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_similarity")
	})

	// Insert test data with known vectors
	testData := []struct {
		name      string
		embedding string
	}{
		{"doc_a", "[1,0,0]"},
		{"doc_b", "[0.9,0.1,0]"},
		{"doc_c", "[0,1,0]"},
		{"doc_d", "[0,0,1]"},
	}

	for _, td := range testData {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO test_similarity (name, embedding)
			VALUES ($1, $2::vector)
		`, td.name, td.embedding)
		require.NoError(t, err)
	}

	// Query for similar documents to [1,0,0] using cosine distance
	queryVector := "[1,0,0]"
	rows, err := db.Pool.Query(ctx, `
		SELECT name, 1 - (embedding <=> $1::vector) as similarity
		FROM test_similarity
		ORDER BY embedding <=> $1::vector
		LIMIT 3
	`, queryVector)
	require.NoError(t, err)
	defer rows.Close()

	var results []struct {
		Name       string
		Similarity float64
	}
	for rows.Next() {
		var r struct {
			Name       string
			Similarity float64
		}
		err := rows.Scan(&r.Name, &r.Similarity)
		require.NoError(t, err)
		results = append(results, r)
	}

	require.Len(t, results, 3)
	// doc_a should be most similar (exact match)
	assert.Equal(t, "doc_a", results[0].Name)
	assert.InDelta(t, 1.0, results[0].Similarity, 0.01)
	// doc_b should be second (very similar)
	assert.Equal(t, "doc_b", results[1].Name)
	assert.Greater(t, results[1].Similarity, 0.9)
}

func TestVectorSearch_L2Distance(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Check if pgvector is available
	var extExists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
	`).Scan(&extExists)
	require.NoError(t, err)

	if !extExists {
		t.Skip("pgvector extension not installed - skipping vector tests")
	}

	// Create test table
	_, err = db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_l2 (
			id SERIAL PRIMARY KEY,
			name TEXT,
			embedding vector(3)
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_l2")
	})

	// Insert test data
	testData := []struct {
		name      string
		embedding string
	}{
		{"point_origin", "[0,0,0]"},
		{"point_near", "[0.1,0.1,0.1]"},
		{"point_far", "[10,10,10]"},
	}

	for _, td := range testData {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO test_l2 (name, embedding)
			VALUES ($1, $2::vector)
		`, td.name, td.embedding)
		require.NoError(t, err)
	}

	// Query using L2 distance
	queryVector := "[0,0,0]"
	rows, err := db.Pool.Query(ctx, `
		SELECT name, embedding <-> $1::vector as distance
		FROM test_l2
		ORDER BY embedding <-> $1::vector
	`, queryVector)
	require.NoError(t, err)
	defer rows.Close()

	var results []struct {
		Name     string
		Distance float64
	}
	for rows.Next() {
		var r struct {
			Name     string
			Distance float64
		}
		err := rows.Scan(&r.Name, &r.Distance)
		require.NoError(t, err)
		results = append(results, r)
	}

	require.Len(t, results, 3)
	// Origin should be closest to itself
	assert.Equal(t, "point_origin", results[0].Name)
	assert.InDelta(t, 0.0, results[0].Distance, 0.001)
	// Near point should be second
	assert.Equal(t, "point_near", results[1].Name)
	// Far point should be last
	assert.Equal(t, "point_far", results[2].Name)
}

func TestVectorSearch_InnerProduct(t *testing.T) {
	db := SetupTestDB(t)
	ctx := context.Background()

	// Check if pgvector is available
	var extExists bool
	err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
	`).Scan(&extExists)
	require.NoError(t, err)

	if !extExists {
		t.Skip("pgvector extension not installed - skipping vector tests")
	}

	// Create test table
	_, err = db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_ip (
			id SERIAL PRIMARY KEY,
			name TEXT,
			embedding vector(3)
		)
	`)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, "DROP TABLE IF EXISTS test_ip")
	})

	// Insert normalized test vectors
	testData := []struct {
		name      string
		embedding string
	}{
		{"same_dir", "[0.577,0.577,0.577]"},     // normalized [1,1,1]
		{"opposite", "[-0.577,-0.577,-0.577]"},  // normalized [-1,-1,-1]
		{"orthogonal", "[0.707,0,-0.707]"},       // normalized [1,0,-1]
	}

	for _, td := range testData {
		_, err := db.Pool.Exec(ctx, `
			INSERT INTO test_ip (name, embedding)
			VALUES ($1, $2::vector)
		`, td.name, td.embedding)
		require.NoError(t, err)
	}

	// Query using inner product (negative because pgvector uses <#> for max inner product)
	queryVector := "[0.577,0.577,0.577]"
	rows, err := db.Pool.Query(ctx, `
		SELECT name, (embedding <#> $1::vector) * -1 as similarity
		FROM test_ip
		ORDER BY embedding <#> $1::vector
	`, queryVector)
	require.NoError(t, err)
	defer rows.Close()

	var results []struct {
		Name       string
		Similarity float64
	}
	for rows.Next() {
		var r struct {
			Name       string
			Similarity float64
		}
		err := rows.Scan(&r.Name, &r.Similarity)
		require.NoError(t, err)
		results = append(results, r)
	}

	require.Len(t, results, 3)
	// same_dir should have highest similarity
	assert.Equal(t, "same_dir", results[0].Name)
}

// generateTestVector creates a test vector string for pgvector
func generateTestVector(dimensions int, baseValue float64) string {
	result := "["
	for i := 0; i < dimensions; i++ {
		if i > 0 {
			result += ","
		}
		// Vary the value slightly based on position
		val := baseValue + float64(i%10)*0.01
		result += floatToString(val)
	}
	result += "]"
	return result
}

func floatToString(f float64) string {
	return strconv.FormatFloat(f, 'f', 4, 64)
}
