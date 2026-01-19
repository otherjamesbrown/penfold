// Package pipeline provides content processing stages for the pipeline.
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// =============================================================================
// Preprocess Stage
// =============================================================================

// PreprocessConfig holds configuration for the preprocess stage.
type PreprocessConfig struct {
	// StripHTML removes HTML tags from content.
	StripHTML bool

	// NormalizeWhitespace collapses multiple whitespace characters.
	NormalizeWhitespace bool

	// ConvertToLowercase converts text to lowercase.
	ConvertToLowercase bool

	// RemoveNonPrintable removes non-printable characters.
	RemoveNonPrintable bool

	// TrimContent removes leading/trailing whitespace.
	TrimContent bool

	// MaxLength truncates content beyond this length (0 = no limit).
	MaxLength int

	// PreserveStructure keeps paragraph breaks.
	PreserveStructure bool
}

// DefaultPreprocessConfig returns sensible defaults for preprocessing.
func DefaultPreprocessConfig() *PreprocessConfig {
	return &PreprocessConfig{
		StripHTML:           true,
		NormalizeWhitespace: true,
		ConvertToLowercase:  false, // Preserve case by default
		RemoveNonPrintable:  true,
		TrimContent:         true,
		MaxLength:           0,
		PreserveStructure:   true,
	}
}

// PreprocessStage normalizes and cleans content text.
type PreprocessStage struct {
	config *PreprocessConfig
	logger logging.Logger

	// Compiled regex patterns
	htmlTagPattern    *regexp.Regexp
	whitespacePattern *regexp.Regexp
	urlPattern        *regexp.Regexp
}

// NewPreprocessStage creates a new preprocessing stage.
func NewPreprocessStage(cfg *PreprocessConfig, logger logging.Logger) *PreprocessStage {
	if cfg == nil {
		cfg = DefaultPreprocessConfig()
	}

	return &PreprocessStage{
		config:            cfg,
		logger:            logger.With(logging.F("stage", "preprocess")),
		htmlTagPattern:    regexp.MustCompile(`<[^>]*>`),
		whitespacePattern: regexp.MustCompile(`[\t ]+`),
		urlPattern:        regexp.MustCompile(`https?://[^\s<>"{}|\\^` + "`" + `\[\]]+`),
	}
}

// Name returns the stage name.
func (s *PreprocessStage) Name() string {
	return "preprocess"
}

// Type returns the stage type.
func (s *PreprocessStage) Type() StageType {
	return StageTypePreprocess
}

// Process normalizes the content text.
func (s *PreprocessStage) Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
	if content == nil || content.Item == nil {
		return nil, ErrEmptyContent
	}

	text := content.Item.GetRawContent()
	if text == "" {
		return nil, ErrEmptyContent
	}

	s.logger.Debug("Starting preprocessing",
		logging.F("content_id", content.Item.GetId()),
		logging.F("input_length", len(text)),
	)

	// Strip HTML tags
	if s.config.StripHTML {
		text = s.stripHTML(text)
	}

	// Remove non-printable characters
	if s.config.RemoveNonPrintable {
		text = s.removeNonPrintable(text)
	}

	// Normalize whitespace
	if s.config.NormalizeWhitespace {
		text = s.normalizeWhitespace(text, s.config.PreserveStructure)
	}

	// Convert to lowercase
	if s.config.ConvertToLowercase {
		text = strings.ToLower(text)
	}

	// Trim content
	if s.config.TrimContent {
		text = strings.TrimSpace(text)
	}

	// Apply max length
	if s.config.MaxLength > 0 && len(text) > s.config.MaxLength {
		text = s.truncateAtWordBoundary(text, s.config.MaxLength)
	}

	content.NormalizedText = text

	s.logger.Debug("Preprocessing completed",
		logging.F("content_id", content.Item.GetId()),
		logging.F("output_length", len(text)),
	)

	return content, nil
}

// Rollback for preprocess stage (no external changes to undo).
func (s *PreprocessStage) Rollback(ctx context.Context, content *ProcessedContent) error {
	// Preprocessing doesn't make external changes
	content.NormalizedText = ""
	return nil
}

// SupportsParallel returns false as preprocessing must happen first.
func (s *PreprocessStage) SupportsParallel() bool {
	return false
}

// stripHTML removes HTML tags from text.
func (s *PreprocessStage) stripHTML(text string) string {
	// Remove script and style elements entirely
	scriptPattern := regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`)
	text = scriptPattern.ReplaceAllString(text, "")

	stylePattern := regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`)
	text = stylePattern.ReplaceAllString(text, "")

	// Convert common block elements to newlines
	blockPattern := regexp.MustCompile(`(?i)</?(div|p|br|h[1-6]|li|tr)[^>]*>`)
	text = blockPattern.ReplaceAllString(text, "\n")

	// Remove remaining HTML tags
	text = s.htmlTagPattern.ReplaceAllString(text, "")

	// Decode common HTML entities
	text = s.decodeHTMLEntities(text)

	return text
}

// decodeHTMLEntities converts common HTML entities to their characters.
func (s *PreprocessStage) decodeHTMLEntities(text string) string {
	entities := map[string]string{
		"&nbsp;":  " ",
		"&amp;":   "&",
		"&lt;":    "<",
		"&gt;":    ">",
		"&quot;":  "\"",
		"&apos;":  "'",
		"&#39;":   "'",
		"&mdash;": "-",
		"&ndash;": "-",
		"&rsquo;": "'",
		"&lsquo;": "'",
		"&rdquo;": "\"",
		"&ldquo;": "\"",
		"&hellip;": "...",
	}

	for entity, char := range entities {
		text = strings.ReplaceAll(text, entity, char)
	}

	return text
}

// removeNonPrintable removes non-printable characters except newlines and tabs.
func (s *PreprocessStage) removeNonPrintable(text string) string {
	var result strings.Builder
	result.Grow(len(text))

	for _, r := range text {
		if unicode.IsPrint(r) || r == '\n' || r == '\t' || r == '\r' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// normalizeWhitespace collapses multiple whitespace characters.
func (s *PreprocessStage) normalizeWhitespace(text string, preserveStructure bool) string {
	if preserveStructure {
		// Normalize spaces/tabs within lines but keep paragraph breaks
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = s.whitespacePattern.ReplaceAllString(line, " ")
			lines[i] = strings.TrimSpace(lines[i])
		}

		// Collapse multiple blank lines into single blank line
		var result []string
		prevEmpty := false
		for _, line := range lines {
			isEmpty := line == ""
			if isEmpty && prevEmpty {
				continue
			}
			result = append(result, line)
			prevEmpty = isEmpty
		}

		return strings.Join(result, "\n")
	}

	// Collapse all whitespace
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// truncateAtWordBoundary truncates text at a word boundary.
func (s *PreprocessStage) truncateAtWordBoundary(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Find the last space before maxLen
	truncated := text[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		return truncated[:lastSpace] + "..."
	}

	return truncated + "..."
}

// =============================================================================
// Chunk Stage
// =============================================================================

// ChunkConfig holds configuration for the chunking stage.
type ChunkConfig struct {
	// MaxChunkSize is the maximum size of each chunk in characters.
	MaxChunkSize int

	// MinChunkSize is the minimum size of a chunk (smaller chunks are merged).
	MinChunkSize int

	// ChunkOverlap is the number of characters to overlap between chunks.
	ChunkOverlap int

	// SplitOnSentences tries to split at sentence boundaries.
	SplitOnSentences bool

	// SplitOnParagraphs tries to split at paragraph boundaries.
	SplitOnParagraphs bool

	// PreserveWholeWords avoids splitting in the middle of words.
	PreserveWholeWords bool
}

// DefaultChunkConfig returns sensible defaults for chunking.
func DefaultChunkConfig() *ChunkConfig {
	return &ChunkConfig{
		MaxChunkSize:       1000,
		MinChunkSize:       100,
		ChunkOverlap:       100,
		SplitOnSentences:   true,
		SplitOnParagraphs:  true,
		PreserveWholeWords: true,
	}
}

// ChunkStage splits content into semantic chunks.
type ChunkStage struct {
	config          *ChunkConfig
	logger          logging.Logger
	sentencePattern *regexp.Regexp
}

// NewChunkStage creates a new chunking stage.
func NewChunkStage(cfg *ChunkConfig, logger logging.Logger) *ChunkStage {
	if cfg == nil {
		cfg = DefaultChunkConfig()
	}

	return &ChunkStage{
		config:          cfg,
		logger:          logger.With(logging.F("stage", "chunk")),
		sentencePattern: regexp.MustCompile(`[.!?]+[\s]+`),
	}
}

// Name returns the stage name.
func (s *ChunkStage) Name() string {
	return "chunk"
}

// Type returns the stage type.
func (s *ChunkStage) Type() StageType {
	return StageTypeChunk
}

// Process splits content into chunks.
func (s *ChunkStage) Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
	if content == nil || content.NormalizedText == "" {
		return nil, ErrEmptyContent
	}

	text := content.NormalizedText

	s.logger.Debug("Starting chunking",
		logging.F("content_id", content.Item.GetId()),
		logging.F("text_length", len(text)),
	)

	var chunks []Chunk

	// Try paragraph-based chunking first
	if s.config.SplitOnParagraphs {
		chunks = s.chunkByParagraphs(text)
	} else {
		chunks = s.chunkBySize(text)
	}

	// Generate chunk IDs and metadata
	contentID := content.Item.GetId()
	for i := range chunks {
		chunks[i].ID = fmt.Sprintf("%s-chunk-%d", contentID, i)
		chunks[i].Index = i
		chunks[i].Metadata = map[string]string{
			"content_id": contentID,
			"chunk_idx":  fmt.Sprintf("%d", i),
			"total_chunks": fmt.Sprintf("%d", len(chunks)),
		}
	}

	content.Chunks = chunks

	s.logger.Debug("Chunking completed",
		logging.F("content_id", content.Item.GetId()),
		logging.F("num_chunks", len(chunks)),
	)

	return content, nil
}

// chunkByParagraphs splits text into paragraph-based chunks.
func (s *ChunkStage) chunkByParagraphs(text string) []Chunk {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var currentChunk strings.Builder
	var startPos int
	currentStart := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// Check if adding this paragraph would exceed max size
		if currentChunk.Len() > 0 && currentChunk.Len()+len(para)+2 > s.config.MaxChunkSize {
			// Finalize current chunk
			chunkText := strings.TrimSpace(currentChunk.String())
			if len(chunkText) >= s.config.MinChunkSize {
				chunks = append(chunks, Chunk{
					Text:     chunkText,
					StartPos: currentStart,
					EndPos:   startPos,
				})
			}
			currentChunk.Reset()
			currentStart = startPos
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
		startPos += len(para) + 2 // Account for \n\n
	}

	// Add remaining content
	if currentChunk.Len() > 0 {
		chunkText := strings.TrimSpace(currentChunk.String())
		if len(chunkText) > 0 {
			chunks = append(chunks, Chunk{
				Text:     chunkText,
				StartPos: currentStart,
				EndPos:   startPos,
			})
		}
	}

	// If we got no chunks or they're too large, fall back to size-based chunking
	if len(chunks) == 0 || (len(chunks) == 1 && len(chunks[0].Text) > s.config.MaxChunkSize) {
		return s.chunkBySize(text)
	}

	return chunks
}

// chunkBySize splits text into fixed-size chunks with overlap.
func (s *ChunkStage) chunkBySize(text string) []Chunk {
	var chunks []Chunk
	textLen := len(text)

	if textLen <= s.config.MaxChunkSize {
		return []Chunk{{
			Text:     text,
			StartPos: 0,
			EndPos:   textLen,
		}}
	}

	startPos := 0
	for startPos < textLen {
		endPos := startPos + s.config.MaxChunkSize
		if endPos > textLen {
			endPos = textLen
		}

		// Try to find a sentence boundary
		if s.config.SplitOnSentences && endPos < textLen {
			segment := text[startPos:endPos]
			matches := s.sentencePattern.FindAllStringIndex(segment, -1)
			if len(matches) > 0 {
				// Use the last sentence boundary
				lastMatch := matches[len(matches)-1]
				endPos = startPos + lastMatch[1]
			} else if s.config.PreserveWholeWords {
				// Find word boundary
				endPos = s.findWordBoundary(text, startPos, endPos)
			}
		}

		chunkText := strings.TrimSpace(text[startPos:endPos])
		if len(chunkText) > 0 {
			chunks = append(chunks, Chunk{
				Text:     chunkText,
				StartPos: startPos,
				EndPos:   endPos,
			})
		}

		// Move to next chunk with overlap
		newStartPos := endPos - s.config.ChunkOverlap
		if newStartPos <= startPos {
			// Ensure we always make progress
			newStartPos = endPos
		}
		startPos = newStartPos
	}

	return chunks
}

// findWordBoundary finds the nearest word boundary before endPos.
func (s *ChunkStage) findWordBoundary(text string, startPos, endPos int) int {
	// Look for the last space before endPos
	for i := endPos - 1; i > startPos+s.config.MinChunkSize; i-- {
		if text[i] == ' ' || text[i] == '\n' {
			return i + 1
		}
	}
	return endPos
}

// Rollback clears the chunks.
func (s *ChunkStage) Rollback(ctx context.Context, content *ProcessedContent) error {
	content.Chunks = nil
	return nil
}

// SupportsParallel returns false as chunking depends on preprocessing.
func (s *ChunkStage) SupportsParallel() bool {
	return false
}

// =============================================================================
// Analyze Stage
// =============================================================================

// AnalyzeConfig holds configuration for the analysis stage.
type AnalyzeConfig struct {
	// DetectLanguage enables language detection.
	DetectLanguage bool

	// ExtractEntities enables entity extraction.
	ExtractEntities bool

	// ExtractKeywords enables keyword extraction.
	ExtractKeywords bool

	// MaxKeywords limits the number of keywords to extract.
	MaxKeywords int

	// MinEntityConfidence sets minimum confidence for entity extraction.
	MinEntityConfidence float64
}

// DefaultAnalyzeConfig returns sensible defaults for analysis.
func DefaultAnalyzeConfig() *AnalyzeConfig {
	return &AnalyzeConfig{
		DetectLanguage:      true,
		ExtractEntities:     true,
		ExtractKeywords:     true,
		MaxKeywords:         20,
		MinEntityConfidence: 0.5,
	}
}

// AnalyzeStage extracts metadata and performs content analysis.
type AnalyzeStage struct {
	config logging.Logger
	logger logging.Logger
	cfg    *AnalyzeConfig

	// Common patterns for entity extraction
	emailPattern *regexp.Regexp
	phonePattern *regexp.Regexp
	datePattern  *regexp.Regexp
}

// NewAnalyzeStage creates a new analysis stage.
func NewAnalyzeStage(cfg *AnalyzeConfig, logger logging.Logger) *AnalyzeStage {
	if cfg == nil {
		cfg = DefaultAnalyzeConfig()
	}

	return &AnalyzeStage{
		cfg:          cfg,
		logger:       logger.With(logging.F("stage", "analyze")),
		emailPattern: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		phonePattern: regexp.MustCompile(`[\+]?[(]?[0-9]{1,3}[)]?[-\s\.]?[(]?[0-9]{1,4}[)]?[-\s\.]?[0-9]{1,4}[-\s\.]?[0-9]{1,9}`),
		datePattern:  regexp.MustCompile(`\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|\d{4}[/-]\d{1,2}[/-]\d{1,2}`),
	}
}

// Name returns the stage name.
func (s *AnalyzeStage) Name() string {
	return "analyze"
}

// Type returns the stage type.
func (s *AnalyzeStage) Type() StageType {
	return StageTypeAnalyze
}

// Process analyzes the content and extracts metadata.
func (s *AnalyzeStage) Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
	if content == nil || content.NormalizedText == "" {
		return nil, ErrEmptyContent
	}

	text := content.NormalizedText

	s.logger.Debug("Starting analysis",
		logging.F("content_id", content.Item.GetId()),
		logging.F("text_length", len(text)),
	)

	analysis := &AnalysisResult{
		Entities: make([]Entity, 0),
		Topics:   make([]string, 0),
		Keywords: make([]string, 0),
		Metadata: make(map[string]string),
	}

	// Detect language
	if s.cfg.DetectLanguage {
		lang, confidence := s.detectLanguage(text)
		analysis.Language = lang
		analysis.DetectedLanguage = confidence
	}

	// Extract entities
	if s.cfg.ExtractEntities {
		entities := s.extractEntities(text)
		for _, e := range entities {
			if e.Confidence >= s.cfg.MinEntityConfidence {
				analysis.Entities = append(analysis.Entities, e)
			}
		}
	}

	// Extract keywords
	if s.cfg.ExtractKeywords {
		analysis.Keywords = s.extractKeywords(text, s.cfg.MaxKeywords)
	}

	// Detect content type
	analysis.ContentType = s.detectContentType(text, content.Item.GetSourceType())

	// Store additional metadata
	analysis.Metadata["char_count"] = fmt.Sprintf("%d", len(text))
	analysis.Metadata["word_count"] = fmt.Sprintf("%d", s.countWords(text))
	analysis.Metadata["chunk_count"] = fmt.Sprintf("%d", len(content.Chunks))
	analysis.Metadata["content_hash"] = s.computeHash(text)

	content.Analysis = analysis

	s.logger.Debug("Analysis completed",
		logging.F("content_id", content.Item.GetId()),
		logging.F("language", analysis.Language),
		logging.F("entity_count", len(analysis.Entities)),
		logging.F("keyword_count", len(analysis.Keywords)),
	)

	return content, nil
}

// detectLanguage performs simple language detection based on character analysis.
func (s *AnalyzeStage) detectLanguage(text string) (string, float64) {
	// Simple heuristic: check for common patterns
	// In production, you'd use a proper language detection library

	// Count ASCII vs non-ASCII
	asciiCount := 0
	totalCount := 0

	for _, r := range text {
		if r < 128 && unicode.IsLetter(r) {
			asciiCount++
		}
		if unicode.IsLetter(r) {
			totalCount++
		}
	}

	if totalCount == 0 {
		return "unknown", 0.0
	}

	asciiRatio := float64(asciiCount) / float64(totalCount)

	// Simple heuristic: high ASCII ratio suggests English
	if asciiRatio > 0.9 {
		return "en", 0.8
	} else if asciiRatio > 0.5 {
		return "mixed", 0.5
	}

	return "other", 0.5
}

// extractEntities extracts named entities from text.
func (s *AnalyzeStage) extractEntities(text string) []Entity {
	var entities []Entity

	// Extract emails
	emails := s.emailPattern.FindAllStringIndex(text, -1)
	for _, match := range emails {
		entities = append(entities, Entity{
			Type:       "email",
			Value:      text[match[0]:match[1]],
			Confidence: 0.95,
			Position:   match[0],
		})
	}

	// Extract phone numbers
	phones := s.phonePattern.FindAllStringIndex(text, -1)
	for _, match := range phones {
		value := text[match[0]:match[1]]
		// Filter out numbers that are too short or too long
		if len(value) >= 7 && len(value) <= 20 {
			entities = append(entities, Entity{
				Type:       "phone",
				Value:      value,
				Confidence: 0.7,
				Position:   match[0],
			})
		}
	}

	// Extract dates
	dates := s.datePattern.FindAllStringIndex(text, -1)
	for _, match := range dates {
		entities = append(entities, Entity{
			Type:       "date",
			Value:      text[match[0]:match[1]],
			Confidence: 0.8,
			Position:   match[0],
		})
	}

	return entities
}

// extractKeywords extracts keywords using simple TF analysis.
func (s *AnalyzeStage) extractKeywords(text string, maxKeywords int) []string {
	// Tokenize and count word frequencies
	wordCounts := make(map[string]int)
	words := strings.Fields(strings.ToLower(text))

	// Common stop words to filter out
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"as": true, "is": true, "was": true, "are": true, "were": true,
		"been": true, "be": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true, "i": true, "you": true, "he": true,
		"she": true, "they": true, "we": true, "me": true, "him": true,
		"her": true, "them": true, "us": true, "my": true, "your": true,
	}

	for _, word := range words {
		// Clean word
		word = strings.Trim(word, ".,!?;:\"'()[]{}")
		if len(word) < 3 {
			continue
		}
		if stopWords[word] {
			continue
		}
		if !utf8.ValidString(word) {
			continue
		}
		wordCounts[word]++
	}

	// Sort by frequency and select top keywords
	type wordFreq struct {
		word  string
		count int
	}

	var freqs []wordFreq
	for word, count := range wordCounts {
		if count >= 2 { // Minimum frequency
			freqs = append(freqs, wordFreq{word, count})
		}
	}

	// Sort by count descending
	for i := 0; i < len(freqs)-1; i++ {
		for j := i + 1; j < len(freqs); j++ {
			if freqs[j].count > freqs[i].count {
				freqs[i], freqs[j] = freqs[j], freqs[i]
			}
		}
	}

	// Take top N keywords
	keywords := make([]string, 0, maxKeywords)
	for i := 0; i < len(freqs) && i < maxKeywords; i++ {
		keywords = append(keywords, freqs[i].word)
	}

	return keywords
}

// detectContentType attempts to classify the content type.
func (s *AnalyzeStage) detectContentType(text string, sourceType string) string {
	if sourceType != "" {
		return sourceType
	}

	// Simple heuristics
	lowerText := strings.ToLower(text)

	if strings.Contains(lowerText, "dear ") || strings.Contains(lowerText, "sincerely") ||
		strings.Contains(lowerText, "best regards") || strings.Contains(lowerText, "subject:") {
		return "email"
	}

	if strings.Contains(lowerText, "meeting") && (strings.Contains(lowerText, "agenda") ||
		strings.Contains(lowerText, "minutes") || strings.Contains(lowerText, "attendees")) {
		return "meeting"
	}

	return "document"
}

// countWords counts the number of words in text.
func (s *AnalyzeStage) countWords(text string) int {
	return len(strings.Fields(text))
}

// computeHash computes a SHA256 hash of the text.
func (s *AnalyzeStage) computeHash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

// Rollback clears the analysis results.
func (s *AnalyzeStage) Rollback(ctx context.Context, content *ProcessedContent) error {
	content.Analysis = nil
	return nil
}

// SupportsParallel returns true as analysis can run in parallel with scoring.
func (s *AnalyzeStage) SupportsParallel() bool {
	return true
}

// =============================================================================
// Score Stage
// =============================================================================

// ScoreConfig holds configuration for the scoring stage.
type ScoreConfig struct {
	// ImportanceWeight affects the importance score calculation.
	ImportanceWeight float64

	// RelevanceWeight affects the relevance score calculation.
	RelevanceWeight float64

	// QualityWeight affects the quality score calculation.
	QualityWeight float64

	// RecencyWeight affects the recency score calculation.
	RecencyWeight float64

	// RecencyDecayDays is the number of days for recency decay.
	RecencyDecayDays int
}

// DefaultScoreConfig returns sensible defaults for scoring.
func DefaultScoreConfig() *ScoreConfig {
	return &ScoreConfig{
		ImportanceWeight: 0.3,
		RelevanceWeight:  0.3,
		QualityWeight:    0.2,
		RecencyWeight:    0.2,
		RecencyDecayDays: 30,
	}
}

// ScoreStage calculates importance and relevance scores.
type ScoreStage struct {
	config *ScoreConfig
	logger logging.Logger
}

// NewScoreStage creates a new scoring stage.
func NewScoreStage(cfg *ScoreConfig, logger logging.Logger) *ScoreStage {
	if cfg == nil {
		cfg = DefaultScoreConfig()
	}

	return &ScoreStage{
		config: cfg,
		logger: logger.With(logging.F("stage", "score")),
	}
}

// Name returns the stage name.
func (s *ScoreStage) Name() string {
	return "score"
}

// Type returns the stage type.
func (s *ScoreStage) Type() StageType {
	return StageTypeScore
}

// Process calculates scores for the content.
func (s *ScoreStage) Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
	if content == nil || content.NormalizedText == "" {
		return nil, ErrEmptyContent
	}

	s.logger.Debug("Starting scoring",
		logging.F("content_id", content.Item.GetId()),
	)

	scores := &ScoreResult{
		Details: make(map[string]float64),
	}

	// Calculate importance score based on content characteristics
	scores.ImportanceScore = s.calculateImportance(content)
	scores.Details["importance"] = scores.ImportanceScore

	// Calculate quality score based on content structure
	scores.QualityScore = s.calculateQuality(content)
	scores.Details["quality"] = scores.QualityScore

	// Calculate recency score based on timestamp
	scores.RecencyScore = s.calculateRecency(content)
	scores.Details["recency"] = scores.RecencyScore

	// Relevance is typically calculated with context (query), so default to neutral
	scores.RelevanceScore = 0.5
	scores.Details["relevance"] = scores.RelevanceScore

	// Calculate overall weighted score
	scores.OverallScore = s.config.ImportanceWeight*scores.ImportanceScore +
		s.config.RelevanceWeight*scores.RelevanceScore +
		s.config.QualityWeight*scores.QualityScore +
		s.config.RecencyWeight*scores.RecencyScore

	scores.Details["overall"] = scores.OverallScore

	content.Scores = scores

	s.logger.Debug("Scoring completed",
		logging.F("content_id", content.Item.GetId()),
		logging.F("overall_score", scores.OverallScore),
	)

	return content, nil
}

// calculateImportance estimates content importance.
func (s *ScoreStage) calculateImportance(content *ProcessedContent) float64 {
	score := 0.5 // Base score

	// Longer content tends to be more important
	textLen := len(content.NormalizedText)
	if textLen > 5000 {
		score += 0.2
	} else if textLen > 1000 {
		score += 0.1
	}

	// Content with more entities tends to be more important
	if content.Analysis != nil {
		entityCount := len(content.Analysis.Entities)
		if entityCount > 10 {
			score += 0.2
		} else if entityCount > 3 {
			score += 0.1
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// calculateQuality estimates content quality.
func (s *ScoreStage) calculateQuality(content *ProcessedContent) float64 {
	score := 0.5 // Base score

	// Check for well-formed content
	text := content.NormalizedText

	// Content with proper structure (paragraphs) is higher quality
	paragraphs := strings.Count(text, "\n\n")
	if paragraphs > 3 {
		score += 0.15
	}

	// Content with reasonable word length is higher quality
	words := strings.Fields(text)
	if len(words) > 0 {
		totalLen := 0
		for _, w := range words {
			totalLen += len(w)
		}
		avgWordLen := float64(totalLen) / float64(len(words))
		if avgWordLen >= 4 && avgWordLen <= 10 {
			score += 0.1
		}
	}

	// Multiple chunks indicate substantive content
	if len(content.Chunks) > 1 {
		score += 0.1
	}

	// Language detection confidence affects quality
	if content.Analysis != nil && content.Analysis.DetectedLanguage > 0.7 {
		score += 0.1
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// calculateRecency calculates a time-decay score.
func (s *ScoreStage) calculateRecency(content *ProcessedContent) float64 {
	if content.Item == nil || content.Item.GetCreatedAt() == nil {
		return 0.5 // Default for unknown age
	}

	createdAt := content.Item.GetCreatedAt().AsTime()
	daysSince := time.Since(createdAt).Hours() / 24

	// Exponential decay
	decayFactor := daysSince / float64(s.config.RecencyDecayDays)
	score := 1.0 / (1.0 + decayFactor)

	return score
}

// Rollback clears the scores.
func (s *ScoreStage) Rollback(ctx context.Context, content *ProcessedContent) error {
	content.Scores = nil
	return nil
}

// SupportsParallel returns true as scoring can run in parallel.
func (s *ScoreStage) SupportsParallel() bool {
	return true
}

// =============================================================================
// Store Stage
// =============================================================================

// ContentStore defines the interface for content persistence.
type ContentStore interface {
	// Store persists processed content to the database.
	Store(ctx context.Context, content *ProcessedContent) error

	// StoreChunks persists chunks with their embeddings.
	StoreChunks(ctx context.Context, contentID string, chunks []Chunk) error

	// Delete removes content and associated data.
	Delete(ctx context.Context, contentID string) error
}

// StoreConfig holds configuration for the store stage.
type StoreConfig struct {
	// BatchSize for bulk operations.
	BatchSize int

	// StoreChunks enables chunk storage.
	StoreChunks bool

	// StoreAnalysis enables analysis metadata storage.
	StoreAnalysis bool

	// StoreScores enables score storage.
	StoreScores bool
}

// DefaultStoreConfig returns sensible defaults for storage.
func DefaultStoreConfig() *StoreConfig {
	return &StoreConfig{
		BatchSize:     100,
		StoreChunks:   true,
		StoreAnalysis: true,
		StoreScores:   true,
	}
}

// StoreStage persists processed content to the database.
type StoreStage struct {
	config *StoreConfig
	logger logging.Logger
	store  ContentStore
	mu     sync.Mutex
}

// NewStoreStage creates a new storage stage.
func NewStoreStage(cfg *StoreConfig, store ContentStore, logger logging.Logger) *StoreStage {
	if cfg == nil {
		cfg = DefaultStoreConfig()
	}

	return &StoreStage{
		config: cfg,
		logger: logger.With(logging.F("stage", "store")),
		store:  store,
	}
}

// Name returns the stage name.
func (s *StoreStage) Name() string {
	return "store"
}

// Type returns the stage type.
func (s *StoreStage) Type() StageType {
	return StageTypeStore
}

// Process stores the processed content.
func (s *StoreStage) Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
	if content == nil {
		return nil, ErrEmptyContent
	}

	// Skip storage in dry-run mode
	if content.DryRun {
		s.logger.Debug("Dry run mode - skipping storage",
			logging.F("content_id", content.Item.GetId()),
		)
		return content, nil
	}

	s.logger.Debug("Starting storage",
		logging.F("content_id", content.Item.GetId()),
		logging.F("chunk_count", len(content.Chunks)),
	)

	if s.store == nil {
		s.logger.Warn("No content store configured - skipping storage")
		return content, nil
	}

	// Store the main content
	if err := s.store.Store(ctx, content); err != nil {
		return content, fmt.Errorf("failed to store content: %w", err)
	}

	// Store chunks if configured
	if s.config.StoreChunks && len(content.Chunks) > 0 {
		if err := s.store.StoreChunks(ctx, content.Item.GetId(), content.Chunks); err != nil {
			return content, fmt.Errorf("failed to store chunks: %w", err)
		}
	}

	s.logger.Debug("Storage completed",
		logging.F("content_id", content.Item.GetId()),
	)

	return content, nil
}

// Rollback removes stored content.
func (s *StoreStage) Rollback(ctx context.Context, content *ProcessedContent) error {
	if content == nil || content.Item == nil {
		return nil
	}

	if content.DryRun {
		return nil
	}

	if s.store == nil {
		return nil
	}

	s.logger.Debug("Rolling back stored content",
		logging.F("content_id", content.Item.GetId()),
	)

	return s.store.Delete(ctx, content.Item.GetId())
}

// SupportsParallel returns false as storage must happen last.
func (s *StoreStage) SupportsParallel() bool {
	return false
}

// SetStore sets the content store (for late initialization).
func (s *StoreStage) SetStore(store ContentStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
}

// =============================================================================
// NoOp Stage (for testing)
// =============================================================================

// NoOpStage is a stage that does nothing (useful for testing).
type NoOpStage struct {
	name           string
	stageType      StageType
	supportsParallel bool
	processFunc    func(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error)
	rollbackFunc   func(ctx context.Context, content *ProcessedContent) error
}

// NewNoOpStage creates a no-op stage with the given name and type.
func NewNoOpStage(name string, stageType StageType) *NoOpStage {
	return &NoOpStage{
		name:           name,
		stageType:      stageType,
		supportsParallel: false,
	}
}

// Name returns the stage name.
func (s *NoOpStage) Name() string {
	return s.name
}

// Type returns the stage type.
func (s *NoOpStage) Type() StageType {
	return s.stageType
}

// Process does nothing and returns the content unchanged.
func (s *NoOpStage) Process(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error) {
	if s.processFunc != nil {
		return s.processFunc(ctx, content)
	}
	return content, nil
}

// Rollback does nothing.
func (s *NoOpStage) Rollback(ctx context.Context, content *ProcessedContent) error {
	if s.rollbackFunc != nil {
		return s.rollbackFunc(ctx, content)
	}
	return nil
}

// SupportsParallel returns the configured value.
func (s *NoOpStage) SupportsParallel() bool {
	return s.supportsParallel
}

// WithProcessFunc sets a custom process function.
func (s *NoOpStage) WithProcessFunc(fn func(ctx context.Context, content *ProcessedContent) (*ProcessedContent, error)) *NoOpStage {
	s.processFunc = fn
	return s
}

// WithRollbackFunc sets a custom rollback function.
func (s *NoOpStage) WithRollbackFunc(fn func(ctx context.Context, content *ProcessedContent) error) *NoOpStage {
	s.rollbackFunc = fn
	return s
}

// WithParallel sets whether the stage supports parallel execution.
func (s *NoOpStage) WithParallel(parallel bool) *NoOpStage {
	s.supportsParallel = parallel
	return s
}
