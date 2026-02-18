package chunking

import (
	"strings"
	"testing"
)

// defaultTokenEstimator is the standard heuristic: len(text)/4
func defaultTokenEstimator(text string) int {
	return len(text) / 4
}

// Test Case 1: Short text (< 400 tokens) -> 1 chunk, Index=0
func TestChunkContent_ShortText(t *testing.T) {
	// 100 tokens = ~400 characters
	text := strings.Repeat("word ", 80) // ~400 chars, ~100 tokens

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short text, got %d", len(chunks))
	}

	chunk := chunks[0]
	if chunk.Index != 0 {
		t.Errorf("expected Index=0, got %d", chunk.Index)
	}

	if chunk.Text != text {
		t.Errorf("expected chunk text to match original text")
	}

	if chunk.Start != 0 {
		t.Errorf("expected Start=0, got %d", chunk.Start)
	}

	if chunk.End != len(text) {
		t.Errorf("expected End=%d, got %d", len(text), chunk.End)
	}
}

// Test Case 2: Two-paragraph text (~600 tokens) -> 2 chunks with overlap
func TestChunkContent_TwoParagraphs(t *testing.T) {
	// First paragraph: ~350 tokens (~1400 chars)
	para1 := strings.Repeat("First paragraph content. ", 56) // ends with period+space
	// Second paragraph: ~300 tokens (~1200 chars)
	para2 := strings.Repeat("Second paragraph content. ", 48)

	text := para1 + "\n\n" + para2

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	// Chunk 0
	if chunks[0].Index != 0 {
		t.Errorf("chunk 0: expected Index=0, got %d", chunks[0].Index)
	}
	if chunks[0].Start != 0 {
		t.Errorf("chunk 0: expected Start=0, got %d", chunks[0].Start)
	}

	// Chunk 1
	if chunks[1].Index != 1 {
		t.Errorf("chunk 1: expected Index=1, got %d", chunks[1].Index)
	}

	// Verify overlap: chunk 1 should start with tail of chunk 0
	// The overlap should be present in both chunks
	chunk0Text := chunks[0].Text
	chunk1Text := chunks[1].Text

	// Find last few sentences of chunk 0
	lastSentences := extractLastSentences(chunk0Text, 2)
	if !strings.HasPrefix(chunk1Text, lastSentences) {
		t.Errorf("chunk 1 should start with overlap from chunk 0's tail")
	}
}

// Test Case 3: Long text (2000+ tokens) -> multiple chunks, each under MaxTokens
func TestChunkContent_LongText(t *testing.T) {
	// 2500 tokens = ~10000 characters
	text := strings.Repeat("This is a long document with many sentences. ", 210) // ~10k chars

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	// Should produce 6-8 chunks (2500 tokens / 350 effective per chunk)
	if len(chunks) < 5 || len(chunks) > 10 {
		t.Errorf("expected 5-10 chunks for 2500 token text, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Errorf("chunk %d: expected Index=%d, got %d", i, i, chunk.Index)
		}

		tokens := opts.TokenEstimator(chunk.Text)
		if tokens > opts.MaxTokens {
			t.Errorf("chunk %d: token count %d exceeds MaxTokens %d", i, tokens, opts.MaxTokens)
		}
	}

	// Verify chunks cover the full text
	if chunks[0].Start != 0 {
		t.Errorf("first chunk should start at 0, got %d", chunks[0].Start)
	}

	lastChunk := chunks[len(chunks)-1]
	if lastChunk.End != len(text) {
		t.Errorf("last chunk should end at %d, got %d", len(text), lastChunk.End)
	}
}

// Test Case 4: Single very long paragraph -> splits on sentence boundaries
func TestChunkContent_LongParagraphNoDoubleNewline(t *testing.T) {
	// Create a single long paragraph with many sentences (no \n\n breaks)
	// 2000 tokens = ~8000 chars
	text := strings.Repeat("This is sentence one. This is sentence two. This is sentence three. ", 115)

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	if len(chunks) < 5 {
		t.Errorf("expected at least 5 chunks for long paragraph, got %d", len(chunks))
	}

	// Each chunk should end with sentence-ending punctuation or be the last chunk
	for i, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk.Text)
		if i < len(chunks)-1 { // Not the last chunk
			lastChar := trimmed[len(trimmed)-1]
			if lastChar != '.' && lastChar != '!' && lastChar != '?' {
				t.Errorf("chunk %d should end with sentence-ending punctuation, got: %c", i, lastChar)
			}
		}

		tokens := opts.TokenEstimator(chunk.Text)
		if tokens > opts.MaxTokens {
			t.Errorf("chunk %d: token count %d exceeds MaxTokens %d", i, tokens, opts.MaxTokens)
		}
	}
}

// Test Case 5: Text with no sentence-ending punctuation -> falls back to word boundaries
func TestChunkContent_NoSentencePunctuation(t *testing.T) {
	// Text with no periods, just spaces between words
	// 1600 tokens = ~6400 chars
	text := strings.Repeat("word word word word word word word word ", 200)

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	if len(chunks) < 4 {
		t.Errorf("expected at least 4 chunks, got %d", len(chunks))
	}

	// Each chunk should respect MaxTokens
	for i, chunk := range chunks {
		tokens := opts.TokenEstimator(chunk.Text)
		if tokens > opts.MaxTokens {
			t.Errorf("chunk %d: token count %d exceeds MaxTokens %d", i, tokens, opts.MaxTokens)
		}

		// Chunks should not split mid-word (should end with space or end of text)
		if i < len(chunks)-1 { // Not the last chunk
			trimmed := strings.TrimRight(chunk.Text, " \t\n\r")
			if len(trimmed) > 0 && !strings.HasSuffix(chunk.Text, " ") {
				// If text was trimmed, original should have ended with whitespace
				t.Errorf("chunk %d may have split mid-word", i)
			}
		}
	}
}

// Test Case 6: Empty text -> returns empty slice
func TestChunkContent_EmptyText(t *testing.T) {
	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent("", opts)

	if chunks == nil {
		t.Error("expected non-nil empty slice, got nil")
	}

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty text, got %d", len(chunks))
	}
}

// Test Case 7: Overlap verification - chunk N+1 starts with tail of chunk N
func TestChunkContent_OverlapVerification(t *testing.T) {
	// Create text that will definitely produce multiple chunks
	paragraphs := make([]string, 10)
	for i := range paragraphs {
		// Each paragraph ~200 tokens (~800 chars)
		paragraphs[i] = strings.Repeat("Paragraph content goes here. ", 26)
	}
	text := strings.Join(paragraphs, "\n\n")

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	if len(chunks) < 4 {
		t.Fatalf("expected at least 4 chunks for test, got %d", len(chunks))
	}

	// For each consecutive pair of chunks, verify overlap
	for i := 0; i < len(chunks)-1; i++ {
		chunk0 := chunks[i]
		chunk1 := chunks[i+1]

		// Extract last ~50 tokens from chunk0
		overlapText := extractLastTokens(chunk0.Text, opts.OverlapTokens, opts.TokenEstimator)

		// Chunk1 should start with this overlap
		if !strings.HasPrefix(chunk1.Text, overlapText) {
			t.Errorf("chunks %d and %d: expected overlap not found", i, i+1)
			t.Logf("Expected overlap: %q", overlapText[:min(50, len(overlapText))])
			t.Logf("Chunk %d start: %q", i+1, chunk1.Text[:min(50, len(chunk1.Text))])
		}
	}
}

// Test Case 8: Custom TokenEstimator works correctly
func TestChunkContent_CustomTokenEstimator(t *testing.T) {
	// Custom estimator: 1 token per character (extreme case)
	customEstimator := func(text string) int {
		return len(text)
	}

	// Text that's 500 chars (will be 500 "tokens" with custom estimator)
	text := strings.Repeat("x", 500)

	opts := ChunkOptions{
		MaxTokens:      100, // Very small limit
		OverlapTokens:  10,
		TokenEstimator: customEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	// Should produce 5+ chunks since 500 tokens / 100 max = 5 minimum
	if len(chunks) < 5 {
		t.Errorf("expected at least 5 chunks with custom estimator, got %d", len(chunks))
	}

	// Each chunk should respect the custom token limit
	for i, chunk := range chunks {
		tokens := customEstimator(chunk.Text)
		if tokens > opts.MaxTokens+opts.OverlapTokens {
			// Allow some tolerance for overlap
			t.Errorf("chunk %d: token count %d exceeds limit (MaxTokens=%d + some overlap)",
				i, tokens, opts.MaxTokens)
		}
	}
}

// Test Case 9: Mixed paragraph sizes - grouping small paragraphs
func TestChunkContent_MixedParagraphSizes(t *testing.T) {
	// Small paragraph (~50 tokens = ~200 chars)
	smallPara := strings.Repeat("Small. ", 28)
	// Medium paragraph (~200 tokens = ~800 chars)
	mediumPara := strings.Repeat("Medium sized paragraph. ", 32)

	// Alternate small and medium: small should be grouped together
	text := strings.Join([]string{
		smallPara, smallPara, smallPara, // These 3 should group into 1 chunk (~150 tokens)
		mediumPara, mediumPara,          // These 2 should form separate chunks or group
		smallPara, smallPara,
	}, "\n\n")

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	// Should efficiently group small paragraphs
	// Expected: fewer chunks than the number of paragraphs (7 paragraphs -> ~3-4 chunks)
	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	if len(chunks) > 5 {
		t.Errorf("expected efficient grouping (<=5 chunks), got %d chunks for 7 paragraphs", len(chunks))
	}

	// Each chunk should be under MaxTokens
	for i, chunk := range chunks {
		tokens := opts.TokenEstimator(chunk.Text)
		if tokens > opts.MaxTokens {
			t.Errorf("chunk %d: token count %d exceeds MaxTokens %d", i, tokens, opts.MaxTokens)
		}
	}
}

// Test Case 10: Verify Start/End byte offsets are correct
func TestChunkContent_ByteOffsets(t *testing.T) {
	para1 := "First paragraph."
	para2 := "Second paragraph."
	para3 := "Third paragraph."

	text := para1 + "\n\n" + para2 + "\n\n" + para3

	opts := ChunkOptions{
		MaxTokens:      400,
		OverlapTokens:  50,
		TokenEstimator: defaultTokenEstimator,
	}

	chunks := ChunkContent(text, opts)

	if chunks == nil {
		t.Fatal("expected non-nil chunks slice")
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	// Each chunk's text should match text[Start:End]
	for i, chunk := range chunks {
		if chunk.Start < 0 || chunk.End > len(text) {
			t.Errorf("chunk %d: invalid offsets Start=%d, End=%d (text len=%d)",
				i, chunk.Start, chunk.End, len(text))
			continue
		}

		if chunk.Start >= chunk.End {
			t.Errorf("chunk %d: Start (%d) should be < End (%d)", i, chunk.Start, chunk.End)
			continue
		}

		// Due to overlap, chunks may overlap in the source text
		// But the chunk text should be derivable from the original
		extractedText := text[chunk.Start:chunk.End]
		if chunk.Text != extractedText {
			t.Errorf("chunk %d: Text doesn't match text[Start:End]", i)
			t.Logf("Expected: %q", extractedText)
			t.Logf("Got:      %q", chunk.Text)
		}
	}
}

// Test Case 11: Default values when opts are zero
func TestChunkContent_DefaultOptions(t *testing.T) {
	text := strings.Repeat("Content here. ", 100)

	// Pass zero-value opts - should use defaults (400, 50, len/4)
	opts := ChunkOptions{} // All zeros

	chunks := ChunkContent(text, opts)

	// Should handle gracefully and use defaults
	// Exact behavior depends on implementation, but shouldn't panic or return nil for non-empty text
	if len(text) > 0 && chunks == nil {
		t.Error("expected non-nil chunks for non-empty text with default options")
	}
}

// Helper: extract last N sentences from text
func extractLastSentences(text string, n int) string {
	// Simple heuristic: split on ". " and take last n
	sentences := strings.Split(text, ". ")
	if len(sentences) <= n {
		return text
	}

	lastN := sentences[len(sentences)-n-1:] // -1 because split on ". " leaves trailing
	return strings.Join(lastN, ". ")
}

// Helper: extract last N tokens worth of text
func extractLastTokens(text string, n int, estimator func(string) int) string {
	// Simple approach: work backwards by sentences/words until we have ~n tokens
	if estimator(text) <= n {
		return text
	}

	// Split by sentence boundaries first
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	// Work backwards
	var result []string
	tokenCount := 0
	for i := len(parts) - 1; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part == "" {
			continue
		}

		partTokens := estimator(part)
		if tokenCount+partTokens > n && tokenCount > 0 {
			break
		}

		result = append([]string{part}, result...)
		tokenCount += partTokens
	}

	return strings.Join(result, ". ")
}

// Helper: min of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
