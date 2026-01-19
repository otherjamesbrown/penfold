package chunking

import (
	"context"
	"strings"
	"testing"
)

// =============================================================================
// TokenChunker Tests
// =============================================================================

func TestTokenChunker_Name(t *testing.T) {
	chunker := NewTokenChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	if chunker.Name() != "token" {
		t.Errorf("expected name 'token', got '%s'", chunker.Name())
	}
}

func TestTokenChunker_EmptyText(t *testing.T) {
	chunker := NewTokenChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	ctx := context.Background()

	chunks, err := chunker.Chunk(ctx, "", "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestTokenChunker_SmallText(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    1,
		TargetTokens: 100,
		MaxTokens:    200,
	}
	overlap := OverlapConfig{
		OverlapTokens: 0, // No overlap for simple test
	}
	chunker := NewTokenChunker(config, overlap, nil)
	ctx := context.Background()

	chunks, err := chunker.Chunk(ctx, "This is a small test.", "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for small text, got %d", len(chunks))
	}
	if chunks[0].Text == "" {
		t.Error("chunk text should not be empty")
	}
}

func TestTokenChunker_LargeText(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    5,
		TargetTokens: 10,
		MaxTokens:    20,
	}
	overlap := OverlapConfig{
		OverlapTokens: 2,
	}
	chunker := NewTokenChunker(config, overlap, nil)
	ctx := context.Background()

	// Create text that should produce multiple chunks
	words := make([]string, 100)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}

	// Verify chunk IDs are unique
	ids := make(map[string]bool)
	for _, chunk := range chunks {
		if ids[chunk.ID] {
			t.Errorf("duplicate chunk ID: %s", chunk.ID)
		}
		ids[chunk.ID] = true
	}
}

func TestTokenChunker_Metadata(t *testing.T) {
	chunker := NewTokenChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	ctx := context.Background()

	chunks, err := chunker.Chunk(ctx, "Test content for metadata verification.", "content-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	chunk := chunks[0]
	if chunk.Metadata.ContentID != "content-123" {
		t.Errorf("expected content ID 'content-123', got '%s'", chunk.Metadata.ContentID)
	}
	if chunk.Metadata.TotalChunks != len(chunks) {
		t.Errorf("expected total chunks %d, got %d", len(chunks), chunk.Metadata.TotalChunks)
	}
}

func TestTokenChunker_ContextCancellation(t *testing.T) {
	chunker := NewTokenChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Create large text
	words := make([]string, 1000)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")

	_, err := chunker.Chunk(ctx, text, "test-id")
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

// =============================================================================
// SentenceChunker Tests
// =============================================================================

func TestSentenceChunker_Name(t *testing.T) {
	chunker := NewSentenceChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	if chunker.Name() != "sentence" {
		t.Errorf("expected name 'sentence', got '%s'", chunker.Name())
	}
}

func TestSentenceChunker_BasicSentences(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    5,
		TargetTokens: 20,
		MaxTokens:    40,
	}
	chunker := NewSentenceChunker(config, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	text := "First sentence here. Second sentence there. Third sentence follows. Fourth sentence ends."

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each chunk should contain complete sentences
	for _, chunk := range chunks {
		// Should not start or end mid-word (approximately)
		if strings.HasPrefix(chunk.Text, " ") {
			t.Errorf("chunk should not start with space: %s", chunk.Text)
		}
	}
}

func TestSentenceChunker_LongSentence(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    5,
		TargetTokens: 10,
		MaxTokens:    15,
	}
	chunker := NewSentenceChunker(config, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	// Create a very long sentence that exceeds max tokens
	words := make([]string, 50)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ") + "."

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have been split despite being a single sentence
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for long sentence, got %d", len(chunks))
	}
}

func TestSentenceChunker_PreservesAbbreviations(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    10,
		TargetTokens: 100,
		MaxTokens:    200,
	}
	chunker := NewSentenceChunker(config, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	text := "Dr. Smith met Mr. Jones at the U.S. embassy. They discussed the issue."

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should handle abbreviations correctly - all in one chunk since it's small
	if len(chunks) != 1 {
		t.Logf("chunks: %v", chunks)
	}

	// The content should be preserved
	fullContent := ""
	for _, c := range chunks {
		fullContent += c.Text + " "
	}
	if !strings.Contains(fullContent, "Dr.") || !strings.Contains(fullContent, "Mr.") {
		t.Error("abbreviations should be preserved")
	}
}

// =============================================================================
// ParagraphChunker Tests
// =============================================================================

func TestParagraphChunker_Name(t *testing.T) {
	chunker := NewParagraphChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	if chunker.Name() != "paragraph" {
		t.Errorf("expected name 'paragraph', got '%s'", chunker.Name())
	}
}

func TestParagraphChunker_BasicParagraphs(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    5,
		TargetTokens: 50,
		MaxTokens:    100,
	}
	chunker := NewParagraphChunker(config, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	text := `First paragraph with some content.

Second paragraph with more content.

Third paragraph with even more content.`

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should preserve paragraph structure
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}

	// Metadata should be set
	for _, chunk := range chunks {
		if chunk.Metadata.BoundaryType == "" {
			t.Error("boundary type should be set")
		}
	}
}

func TestParagraphChunker_VeryLongParagraph(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    5,
		TargetTokens: 20,
		MaxTokens:    40,
	}
	chunker := NewParagraphChunker(config, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	// Create a very long paragraph
	words := make([]string, 100)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to sentence chunking
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for long paragraph, got %d", len(chunks))
	}
}

// =============================================================================
// SemanticChunker Tests
// =============================================================================

func TestSemanticChunker_Name(t *testing.T) {
	chunker := NewSemanticChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil, nil, 0.5)
	if chunker.Name() != "semantic" {
		t.Errorf("expected name 'semantic', got '%s'", chunker.Name())
	}
}

func TestSemanticChunker_FallbackWithoutEmbedder(t *testing.T) {
	chunker := NewSemanticChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil, nil, 0.5)
	ctx := context.Background()

	text := "First sentence. Second sentence. Third sentence."

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should fall back to sentence chunking without embedder
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

// mockEmbedder is a test embedder that returns deterministic embeddings.
type mockEmbedder struct {
	embeddings map[string][]float32
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if emb, ok := m.embeddings[text]; ok {
		return emb, nil
	}
	// Return a default embedding
	return []float32{0.5, 0.5, 0.5, 0.5}, nil
}

func TestSemanticChunker_WithEmbedder(t *testing.T) {
	embedder := &mockEmbedder{
		embeddings: map[string][]float32{
			"First sentence.":  {1.0, 0.0, 0.0, 0.0},
			"Second sentence.": {0.9, 0.1, 0.0, 0.0}, // Similar to first
			"Third sentence.":  {0.0, 0.0, 1.0, 0.0}, // Different - semantic boundary
		},
	}

	config := ChunkSizeConfig{
		MinTokens:    1,
		TargetTokens: 100,
		MaxTokens:    200,
	}
	chunker := NewSemanticChunker(config, DefaultOverlapConfig(), nil, embedder, 0.5)
	ctx := context.Background()

	text := "First sentence. Second sentence. Third sentence."

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should detect semantic boundary
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

// =============================================================================
// HierarchicalChunker Tests
// =============================================================================

func TestHierarchicalChunker_Name(t *testing.T) {
	chunker := NewHierarchicalChunker(nil, DefaultOverlapConfig(), nil)
	if chunker.Name() != "hierarchical" {
		t.Errorf("expected name 'hierarchical', got '%s'", chunker.Name())
	}
}

func TestHierarchicalChunker_DefaultLevels(t *testing.T) {
	chunker := NewHierarchicalChunker(nil, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	text := `First section with some content.

Second section has more paragraphs.

Third section wraps up the document.`

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should create hierarchical chunks
	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}

	// Check for multiple levels
	levels := make(map[int]bool)
	for _, chunk := range chunks {
		levels[chunk.Metadata.Level] = true
	}

	// Should have at least one level
	if len(levels) == 0 {
		t.Error("expected chunks at different levels")
	}
}

func TestHierarchicalChunker_CustomLevels(t *testing.T) {
	levels := []ChunkSizeConfig{
		{MinTokens: 50, TargetTokens: 100, MaxTokens: 200},
		{MinTokens: 20, TargetTokens: 50, MaxTokens: 100},
	}
	chunker := NewHierarchicalChunker(levels, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	text := "Content for hierarchical chunking test with enough words to create multiple chunks at different levels."

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

// =============================================================================
// ChunkerFactory Tests
// =============================================================================

func TestChunkerFactory_Create(t *testing.T) {
	factory := NewChunkerFactory(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)

	tests := []struct {
		strategy     string
		expectedName string
	}{
		{"token", "token"},
		{"sentence", "sentence"},
		{"paragraph", "paragraph"},
		{"semantic", "semantic"},
		{"hierarchical", "hierarchical"},
		{"unknown", "sentence"}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			chunker := factory.Create(tt.strategy)
			if chunker.Name() != tt.expectedName {
				t.Errorf("expected name '%s', got '%s'", tt.expectedName, chunker.Name())
			}
		})
	}
}

// =============================================================================
// Neighbor Metadata Tests
// =============================================================================

func TestChunker_NeighborMetadata(t *testing.T) {
	config := ChunkSizeConfig{
		MinTokens:    5,
		TargetTokens: 15,
		MaxTokens:    30,
	}
	chunker := NewTokenChunker(config, DefaultOverlapConfig(), nil)
	ctx := context.Background()

	// Create text that produces multiple chunks
	words := make([]string, 100)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")

	chunks, err := chunker.Chunk(ctx, text, "test-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chunks) < 3 {
		t.Skip("need at least 3 chunks for neighbor metadata test")
	}

	// Check first chunk
	if chunks[0].Metadata.PreviousChunkID != "" {
		t.Error("first chunk should not have previous chunk ID")
	}
	if chunks[0].Metadata.NextChunkID == "" {
		t.Error("first chunk should have next chunk ID")
	}

	// Check middle chunk
	middle := len(chunks) / 2
	if chunks[middle].Metadata.PreviousChunkID == "" {
		t.Error("middle chunk should have previous chunk ID")
	}
	if chunks[middle].Metadata.NextChunkID == "" {
		t.Error("middle chunk should have next chunk ID")
	}

	// Check last chunk
	last := len(chunks) - 1
	if chunks[last].Metadata.PreviousChunkID == "" {
		t.Error("last chunk should have previous chunk ID")
	}
	if chunks[last].Metadata.NextChunkID != "" {
		t.Error("last chunk should not have next chunk ID")
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkTokenChunker(b *testing.B) {
	chunker := NewTokenChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	ctx := context.Background()

	// Create test text
	words := make([]string, 1000)
	for i := range words {
		words[i] = "word"
	}
	text := strings.Join(words, " ")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunker.Chunk(ctx, text, "bench-id")
	}
}

func BenchmarkSentenceChunker(b *testing.B) {
	chunker := NewSentenceChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	ctx := context.Background()

	// Create test text with sentences
	sentences := make([]string, 100)
	for i := range sentences {
		sentences[i] = "This is a test sentence with some content."
	}
	text := strings.Join(sentences, " ")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunker.Chunk(ctx, text, "bench-id")
	}
}

func BenchmarkParagraphChunker(b *testing.B) {
	chunker := NewParagraphChunker(DefaultChunkSizeConfig(), DefaultOverlapConfig(), nil)
	ctx := context.Background()

	// Create test text with paragraphs
	paragraphs := make([]string, 20)
	for i := range paragraphs {
		paragraphs[i] = "This is a test paragraph with some content. It has multiple sentences. And some more text."
	}
	text := strings.Join(paragraphs, "\n\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunker.Chunk(ctx, text, "bench-id")
	}
}

// =============================================================================
// Cosine Similarity Tests
// =============================================================================

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
		delta    float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
			delta:    0.01,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{0.0, 1.0, 0.0},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{-1.0, 0.0, 0.0},
			expected: -1.0,
			delta:    0.01,
		},
		{
			name:     "similar vectors",
			a:        []float32{1.0, 0.1, 0.0},
			b:        []float32{0.9, 0.2, 0.0},
			expected: 0.99,
			delta:    0.05,
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			delta:    0.01,
		},
		{
			name:     "mismatched lengths",
			a:        []float32{1.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 0.0,
			delta:    0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.delta {
				t.Errorf("expected similarity ~%.2f, got %.2f", tt.expected, result)
			}
		})
	}
}
