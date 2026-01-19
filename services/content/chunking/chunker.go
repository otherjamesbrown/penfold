// Package chunking provides content chunking strategies for the processing pipeline.
// It supports multiple chunking approaches: token-based, sentence-based, paragraph-based,
// semantic, and hierarchical chunking for different use cases.
package chunking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// Chunk represents a semantic chunk of content with metadata.
type Chunk struct {
	// ID is a unique identifier for this chunk
	ID string

	// Index is the position of this chunk in the sequence
	Index int

	// Text contains the actual chunk content
	Text string

	// StartPos is the starting character position in the original text
	StartPos int

	// EndPos is the ending character position in the original text
	EndPos int

	// TokenCount is the estimated number of tokens in this chunk
	TokenCount int

	// Embedding stores the vector embedding if computed
	Embedding []float32

	// Metadata contains additional chunk properties
	Metadata ChunkMetadata
}

// ChunkMetadata contains metadata about a chunk's context and position.
type ChunkMetadata struct {
	// ContentID is the ID of the parent content
	ContentID string

	// ChunkIndex is the index of this chunk
	ChunkIndex int

	// TotalChunks is the total number of chunks in the content
	TotalChunks int

	// PreviousChunkID is the ID of the preceding chunk (if any)
	PreviousChunkID string

	// NextChunkID is the ID of the following chunk (if any)
	NextChunkID string

	// ParentChunkID is the ID of the parent chunk for hierarchical chunking
	ParentChunkID string

	// Level indicates the hierarchy level (0 = top level)
	Level int

	// BoundaryType describes how this chunk boundary was determined
	BoundaryType string

	// OverlapTokens indicates tokens shared with adjacent chunks
	OverlapTokens int

	// Custom allows arbitrary key-value metadata
	Custom map[string]string
}

// Chunker defines the interface for content chunking implementations.
type Chunker interface {
	// Chunk splits text into chunks according to the implementation strategy.
	Chunk(ctx context.Context, text string, contentID string) ([]Chunk, error)

	// Name returns the name of this chunking strategy.
	Name() string

	// GetConfig returns the current configuration.
	GetConfig() ChunkSizeConfig
}

// ChunkerOption configures a chunker during construction.
type ChunkerOption func(interface{})

// BaseChunker provides common functionality for all chunker implementations.
type BaseChunker struct {
	config    ChunkSizeConfig
	tokenizer Tokenizer
	mu        sync.RWMutex
}

// NewBaseChunker creates a base chunker with the given configuration.
func NewBaseChunker(config ChunkSizeConfig, tokenizer Tokenizer) *BaseChunker {
	if tokenizer == nil {
		tokenizer = NewApproximateTokenizer(1.3)
	}
	return &BaseChunker{
		config:    config,
		tokenizer: tokenizer,
	}
}

// GetConfig returns the chunk size configuration.
func (b *BaseChunker) GetConfig() ChunkSizeConfig {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.config
}

// generateChunkID creates a deterministic ID for a chunk.
func (b *BaseChunker) generateChunkID(contentID string, index int, text string) string {
	data := fmt.Sprintf("%s:%d:%s", contentID, index, text[:min(100, len(text))])
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%s-chunk-%d-%s", contentID, index, hex.EncodeToString(hash[:8]))
}

// createChunk creates a Chunk with common metadata populated.
func (b *BaseChunker) createChunk(text string, contentID string, index, startPos, endPos int, boundaryType string) Chunk {
	tokenCount := b.tokenizer.CountTokens(text)
	return Chunk{
		ID:         b.generateChunkID(contentID, index, text),
		Index:      index,
		Text:       text,
		StartPos:   startPos,
		EndPos:     endPos,
		TokenCount: tokenCount,
		Metadata: ChunkMetadata{
			ContentID:    contentID,
			ChunkIndex:   index,
			BoundaryType: boundaryType,
			Custom:       make(map[string]string),
		},
	}
}

// populateNeighborMetadata fills in neighbor references after all chunks are created.
func (b *BaseChunker) populateNeighborMetadata(chunks []Chunk) []Chunk {
	totalChunks := len(chunks)
	for i := range chunks {
		chunks[i].Metadata.TotalChunks = totalChunks
		if i > 0 {
			chunks[i].Metadata.PreviousChunkID = chunks[i-1].ID
		}
		if i < totalChunks-1 {
			chunks[i].Metadata.NextChunkID = chunks[i+1].ID
		}
	}
	return chunks
}

// TokenChunker splits text based on a fixed number of tokens.
type TokenChunker struct {
	*BaseChunker
	overlap OverlapConfig
}

// NewTokenChunker creates a new token-based chunker.
func NewTokenChunker(config ChunkSizeConfig, overlap OverlapConfig, tokenizer Tokenizer) *TokenChunker {
	return &TokenChunker{
		BaseChunker: NewBaseChunker(config, tokenizer),
		overlap:     overlap,
	}
}

// Name returns the chunker name.
func (c *TokenChunker) Name() string {
	return "token"
}

// Chunk splits text into fixed-token-size chunks with overlap.
func (c *TokenChunker) Chunk(ctx context.Context, text string, contentID string) ([]Chunk, error) {
	if text == "" {
		return nil, nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil, nil
	}

	targetTokens := c.config.TargetTokens
	overlapTokens := c.overlap.GetOverlapTokens(targetTokens)

	var chunks []Chunk
	currentStart := 0
	charPos := 0

	for currentStart < len(words) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Find the end of this chunk
		currentEnd := currentStart
		tokenCount := 0

		for currentEnd < len(words) && tokenCount < targetTokens {
			wordTokens := c.tokenizer.CountTokens(words[currentEnd])
			if tokenCount+wordTokens > c.config.MaxTokens && currentEnd > currentStart {
				break
			}
			tokenCount += wordTokens
			currentEnd++
		}

		// Build chunk text
		chunkWords := words[currentStart:currentEnd]
		chunkText := strings.Join(chunkWords, " ")

		// Calculate character positions
		startCharPos := charPos
		for i := 0; i < currentStart; i++ {
			startCharPos = strings.Index(text[charPos:], words[i])
			if startCharPos >= 0 {
				charPos += startCharPos + len(words[i])
			}
		}

		chunk := c.createChunk(chunkText, contentID, len(chunks), startCharPos, startCharPos+len(chunkText), "token")
		chunk.Metadata.OverlapTokens = overlapTokens
		chunks = append(chunks, chunk)

		// Move to next chunk with overlap
		stepSize := currentEnd - currentStart - overlapTokens
		if stepSize < 1 {
			stepSize = 1
		}
		currentStart += stepSize

		// Ensure we make progress
		if currentStart <= currentEnd-len(chunkWords) {
			currentStart = currentEnd
		}
	}

	return c.populateNeighborMetadata(chunks), nil
}

// SentenceChunker splits text at sentence boundaries.
type SentenceChunker struct {
	*BaseChunker
	overlap  OverlapConfig
	splitter *TextSplitter
}

// NewSentenceChunker creates a new sentence-boundary-aware chunker.
func NewSentenceChunker(config ChunkSizeConfig, overlap OverlapConfig, tokenizer Tokenizer) *SentenceChunker {
	return &SentenceChunker{
		BaseChunker: NewBaseChunker(config, tokenizer),
		overlap:     overlap,
		splitter:    NewTextSplitter(),
	}
}

// Name returns the chunker name.
func (c *SentenceChunker) Name() string {
	return "sentence"
}

// Chunk splits text at sentence boundaries while respecting token limits.
func (c *SentenceChunker) Chunk(ctx context.Context, text string, contentID string) ([]Chunk, error) {
	if text == "" {
		return nil, nil
	}

	sentences := c.splitter.SplitBySentence(text)
	if len(sentences) == 0 {
		return nil, nil
	}

	overlapTokens := c.overlap.GetOverlapTokens(c.config.TargetTokens)
	var chunks []Chunk
	var currentSentences []string
	currentTokens := 0
	charPos := 0

	flushChunk := func() {
		if len(currentSentences) == 0 {
			return
		}

		chunkText := strings.Join(currentSentences, " ")
		startPos := strings.Index(text[charPos:], currentSentences[0])
		if startPos >= 0 {
			startPos += charPos
		} else {
			startPos = charPos
		}

		chunk := c.createChunk(chunkText, contentID, len(chunks), startPos, startPos+len(chunkText), "sentence")
		chunk.Metadata.OverlapTokens = overlapTokens
		chunks = append(chunks, chunk)

		// Keep overlap sentences for next chunk
		overlapSentences := []string{}
		overlapCount := 0
		for i := len(currentSentences) - 1; i >= 0 && overlapCount < overlapTokens; i-- {
			sentTokens := c.tokenizer.CountTokens(currentSentences[i])
			if overlapCount+sentTokens > overlapTokens && len(overlapSentences) > 0 {
				break
			}
			overlapSentences = append([]string{currentSentences[i]}, overlapSentences...)
			overlapCount += sentTokens
		}

		currentSentences = overlapSentences
		currentTokens = overlapCount
		charPos = startPos + len(chunkText)
	}

	for _, sentence := range sentences {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sentenceTokens := c.tokenizer.CountTokens(sentence)

		// If single sentence exceeds max, we need to split it
		if sentenceTokens > c.config.MaxTokens {
			flushChunk()
			// Fall back to token-based splitting for this sentence
			tokenChunker := NewTokenChunker(c.config, c.overlap, c.tokenizer)
			subChunks, err := tokenChunker.Chunk(ctx, sentence, contentID)
			if err != nil {
				return nil, err
			}
			for _, sc := range subChunks {
				sc.Index = len(chunks)
				sc.Metadata.BoundaryType = "sentence-overflow"
				chunks = append(chunks, sc)
			}
			continue
		}

		// Check if adding this sentence would exceed target
		if currentTokens+sentenceTokens > c.config.TargetTokens && len(currentSentences) > 0 {
			flushChunk()
		}

		currentSentences = append(currentSentences, sentence)
		currentTokens += sentenceTokens
	}

	// Flush remaining content
	flushChunk()

	return c.populateNeighborMetadata(chunks), nil
}

// ParagraphChunker splits text at paragraph boundaries.
type ParagraphChunker struct {
	*BaseChunker
	overlap  OverlapConfig
	splitter *TextSplitter
}

// NewParagraphChunker creates a new paragraph-based chunker.
func NewParagraphChunker(config ChunkSizeConfig, overlap OverlapConfig, tokenizer Tokenizer) *ParagraphChunker {
	return &ParagraphChunker{
		BaseChunker: NewBaseChunker(config, tokenizer),
		overlap:     overlap,
		splitter:    NewTextSplitter(),
	}
}

// Name returns the chunker name.
func (c *ParagraphChunker) Name() string {
	return "paragraph"
}

// Chunk splits text at paragraph boundaries while respecting token limits.
func (c *ParagraphChunker) Chunk(ctx context.Context, text string, contentID string) ([]Chunk, error) {
	if text == "" {
		return nil, nil
	}

	paragraphs := c.splitter.SplitByParagraph(text)
	if len(paragraphs) == 0 {
		return nil, nil
	}

	var chunks []Chunk
	var currentParagraphs []string
	currentTokens := 0
	charPos := 0

	flushChunk := func() {
		if len(currentParagraphs) == 0 {
			return
		}

		chunkText := strings.Join(currentParagraphs, "\n\n")
		startPos := charPos

		chunk := c.createChunk(chunkText, contentID, len(chunks), startPos, startPos+len(chunkText), "paragraph")
		chunks = append(chunks, chunk)

		currentParagraphs = nil
		currentTokens = 0
		charPos = startPos + len(chunkText) + 2 // +2 for \n\n
	}

	for _, para := range paragraphs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		paraTokens := c.tokenizer.CountTokens(para)

		// If single paragraph exceeds max, use sentence chunker
		if paraTokens > c.config.MaxTokens {
			flushChunk()
			sentenceChunker := NewSentenceChunker(c.config, c.overlap, c.tokenizer)
			subChunks, err := sentenceChunker.Chunk(ctx, para, contentID)
			if err != nil {
				return nil, err
			}
			for _, sc := range subChunks {
				sc.Index = len(chunks)
				sc.Metadata.BoundaryType = "paragraph-overflow"
				chunks = append(chunks, sc)
			}
			continue
		}

		// Check if adding this paragraph would exceed target
		if currentTokens+paraTokens > c.config.TargetTokens && len(currentParagraphs) > 0 {
			flushChunk()
		}

		currentParagraphs = append(currentParagraphs, para)
		currentTokens += paraTokens
	}

	// Flush remaining content
	flushChunk()

	return c.populateNeighborMetadata(chunks), nil
}

// SemanticChunker splits text based on semantic similarity boundaries.
// It uses embedding similarity to detect topic shifts.
type SemanticChunker struct {
	*BaseChunker
	overlap         OverlapConfig
	splitter        *TextSplitter
	similarityThreshold float64
	embedder        Embedder
}

// Embedder generates embeddings for text.
type Embedder interface {
	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewSemanticChunker creates a new semantic-similarity-based chunker.
func NewSemanticChunker(config ChunkSizeConfig, overlap OverlapConfig, tokenizer Tokenizer, embedder Embedder, threshold float64) *SemanticChunker {
	if threshold <= 0 {
		threshold = 0.5 // Default similarity threshold
	}
	return &SemanticChunker{
		BaseChunker:         NewBaseChunker(config, tokenizer),
		overlap:             overlap,
		splitter:            NewTextSplitter(),
		similarityThreshold: threshold,
		embedder:            embedder,
	}
}

// Name returns the chunker name.
func (c *SemanticChunker) Name() string {
	return "semantic"
}

// Chunk splits text at semantic boundaries detected by embedding similarity.
func (c *SemanticChunker) Chunk(ctx context.Context, text string, contentID string) ([]Chunk, error) {
	if text == "" {
		return nil, nil
	}

	// If no embedder is available, fall back to sentence chunking
	if c.embedder == nil {
		fallback := NewSentenceChunker(c.config, c.overlap, c.tokenizer)
		return fallback.Chunk(ctx, text, contentID)
	}

	sentences := c.splitter.SplitBySentence(text)
	if len(sentences) == 0 {
		return nil, nil
	}

	// Get embeddings for all sentences
	embeddings := make([][]float32, len(sentences))
	for i, sent := range sentences {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		emb, err := c.embedder.Embed(ctx, sent)
		if err != nil {
			// Fall back to sentence chunking on embedding error
			fallback := NewSentenceChunker(c.config, c.overlap, c.tokenizer)
			return fallback.Chunk(ctx, text, contentID)
		}
		embeddings[i] = emb
	}

	// Find semantic boundaries based on similarity drops
	boundaries := c.findSemanticBoundaries(sentences, embeddings)

	// Create chunks at boundaries
	var chunks []Chunk
	var currentSentences []string
	currentTokens := 0
	charPos := 0
	boundaryIdx := 0

	flushChunk := func() {
		if len(currentSentences) == 0 {
			return
		}

		chunkText := strings.Join(currentSentences, " ")
		chunk := c.createChunk(chunkText, contentID, len(chunks), charPos, charPos+len(chunkText), "semantic")
		chunks = append(chunks, chunk)

		charPos += len(chunkText) + 1
		currentSentences = nil
		currentTokens = 0
	}

	for i, sent := range sentences {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sentTokens := c.tokenizer.CountTokens(sent)

		// Check for semantic boundary
		isBoundary := boundaryIdx < len(boundaries) && boundaries[boundaryIdx] == i
		if isBoundary {
			boundaryIdx++
		}

		// Flush if at boundary or exceeding target tokens
		shouldFlush := (isBoundary && len(currentSentences) > 0) ||
			(currentTokens+sentTokens > c.config.TargetTokens && len(currentSentences) > 0)

		if shouldFlush {
			flushChunk()
		}

		currentSentences = append(currentSentences, sent)
		currentTokens += sentTokens
	}

	flushChunk()

	return c.populateNeighborMetadata(chunks), nil
}

// findSemanticBoundaries identifies indices where semantic shifts occur.
func (c *SemanticChunker) findSemanticBoundaries(sentences []string, embeddings [][]float32) []int {
	if len(sentences) < 2 {
		return nil
	}

	var boundaries []int

	for i := 1; i < len(embeddings); i++ {
		similarity := cosineSimilarity(embeddings[i-1], embeddings[i])
		if similarity < c.similarityThreshold {
			boundaries = append(boundaries, i)
		}
	}

	return boundaries
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

// sqrt is a simple square root approximation for float64.
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// HierarchicalChunker creates nested chunks at multiple levels.
type HierarchicalChunker struct {
	*BaseChunker
	overlap  OverlapConfig
	splitter *TextSplitter
	levels   []ChunkSizeConfig // Different sizes for each level
}

// NewHierarchicalChunker creates a chunker that produces nested chunks.
func NewHierarchicalChunker(levels []ChunkSizeConfig, overlap OverlapConfig, tokenizer Tokenizer) *HierarchicalChunker {
	if len(levels) == 0 {
		levels = []ChunkSizeConfig{
			{MinTokens: 100, TargetTokens: 500, MaxTokens: 1000},   // Level 0: Large sections
			{MinTokens: 50, TargetTokens: 200, MaxTokens: 400},    // Level 1: Paragraphs
			{MinTokens: 20, TargetTokens: 100, MaxTokens: 200},    // Level 2: Sentences
		}
	}
	return &HierarchicalChunker{
		BaseChunker: NewBaseChunker(levels[0], tokenizer),
		overlap:     overlap,
		splitter:    NewTextSplitter(),
		levels:      levels,
	}
}

// Name returns the chunker name.
func (c *HierarchicalChunker) Name() string {
	return "hierarchical"
}

// Chunk creates a hierarchy of nested chunks.
func (c *HierarchicalChunker) Chunk(ctx context.Context, text string, contentID string) ([]Chunk, error) {
	if text == "" {
		return nil, nil
	}

	// Start with the top level (largest chunks)
	allChunks := make([]Chunk, 0)
	return c.chunkAtLevel(ctx, text, contentID, 0, "", &allChunks)
}

func (c *HierarchicalChunker) chunkAtLevel(ctx context.Context, text, contentID string, level int, parentID string, allChunks *[]Chunk) ([]Chunk, error) {
	if level >= len(c.levels) {
		return *allChunks, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	config := c.levels[level]
	var chunker Chunker

	// Use different strategies for different levels
	switch level {
	case 0:
		chunker = NewParagraphChunker(config, c.overlap, c.tokenizer)
	default:
		chunker = NewSentenceChunker(config, c.overlap, c.tokenizer)
	}

	levelChunks, err := chunker.Chunk(ctx, text, contentID)
	if err != nil {
		return nil, err
	}

	// Update metadata and recurse
	for i := range levelChunks {
		levelChunks[i].Metadata.Level = level
		levelChunks[i].Metadata.ParentChunkID = parentID
		levelChunks[i].ID = fmt.Sprintf("%s-L%d", levelChunks[i].ID, level)

		*allChunks = append(*allChunks, levelChunks[i])

		// Recurse to create child chunks if not at the deepest level
		if level < len(c.levels)-1 && c.tokenizer.CountTokens(levelChunks[i].Text) > c.levels[level+1].MinTokens {
			_, err := c.chunkAtLevel(ctx, levelChunks[i].Text, contentID, level+1, levelChunks[i].ID, allChunks)
			if err != nil {
				return nil, err
			}
		}
	}

	return *allChunks, nil
}

// ChunkerFactory creates chunkers based on strategy name.
type ChunkerFactory struct {
	defaultConfig    ChunkSizeConfig
	defaultOverlap   OverlapConfig
	defaultTokenizer Tokenizer
}

// NewChunkerFactory creates a new chunker factory.
func NewChunkerFactory(config ChunkSizeConfig, overlap OverlapConfig, tokenizer Tokenizer) *ChunkerFactory {
	if tokenizer == nil {
		tokenizer = NewApproximateTokenizer(1.3)
	}
	return &ChunkerFactory{
		defaultConfig:    config,
		defaultOverlap:   overlap,
		defaultTokenizer: tokenizer,
	}
}

// Create creates a chunker based on the strategy name.
func (f *ChunkerFactory) Create(strategy string) Chunker {
	switch strings.ToLower(strategy) {
	case "token":
		return NewTokenChunker(f.defaultConfig, f.defaultOverlap, f.defaultTokenizer)
	case "sentence":
		return NewSentenceChunker(f.defaultConfig, f.defaultOverlap, f.defaultTokenizer)
	case "paragraph":
		return NewParagraphChunker(f.defaultConfig, f.defaultOverlap, f.defaultTokenizer)
	case "semantic":
		return NewSemanticChunker(f.defaultConfig, f.defaultOverlap, f.defaultTokenizer, nil, 0.5)
	case "hierarchical":
		return NewHierarchicalChunker(nil, f.defaultOverlap, f.defaultTokenizer)
	default:
		// Default to sentence chunking
		return NewSentenceChunker(f.defaultConfig, f.defaultOverlap, f.defaultTokenizer)
	}
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
