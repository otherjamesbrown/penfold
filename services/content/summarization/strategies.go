package summarization

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ExtractiveStrategy implements TextRank-style extractive summarization.
// It extracts key sentences from the original content without generating new text.
type ExtractiveStrategy struct {
	logger logging.Logger
}

// NewExtractiveStrategy creates a new extractive summarization strategy.
func NewExtractiveStrategy(logger logging.Logger) *ExtractiveStrategy {
	return &ExtractiveStrategy{
		logger: logger.With(logging.F("strategy", "extractive")),
	}
}

// Name returns the strategy name.
func (s *ExtractiveStrategy) Name() string {
	return "TextRank Extractive"
}

// Type returns the strategy type.
func (s *ExtractiveStrategy) Type() StrategyType {
	return StrategyTypeExtractive
}

// SupportsBatch indicates that extractive supports batch processing.
func (s *ExtractiveStrategy) SupportsBatch() bool {
	return true
}

// Summarize generates an extractive summary using TextRank-style scoring.
func (s *ExtractiveStrategy) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	// Split content into sentences
	sentences := splitSentences(req.Content)
	if len(sentences) == 0 {
		return &SummaryResult{
			Summary:    req.Content,
			Confidence: 0.5,
		}, nil
	}

	// Calculate target sentence count based on length
	targetSentences := s.targetSentenceCount(len(sentences), req.Length)

	// Score sentences using TextRank-style algorithm
	scores := s.scoreSentences(sentences)

	// Select top sentences while preserving order
	selected := s.selectTopSentences(sentences, scores, targetSentences)

	// Build summary maintaining original order
	var summary strings.Builder
	for i, sent := range selected {
		if i > 0 {
			summary.WriteString(" ")
		}
		summary.WriteString(strings.TrimSpace(sent))
	}

	// Extract key points (top 3-5 sentences)
	keyPoints := s.extractKeyPoints(sentences, scores, 5)

	// Estimate tokens (roughly 4 chars per token)
	inputTokens := len(req.Content) / 4
	outputTokens := summary.Len() / 4

	return &SummaryResult{
		Summary:      summary.String(),
		KeyPoints:    keyPoints,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Confidence:   0.75, // Extractive methods have moderate confidence
	}, nil
}

// SummarizeBatch processes multiple requests.
func (s *ExtractiveStrategy) SummarizeBatch(ctx context.Context, reqs []*SummaryRequest) ([]*SummaryResult, error) {
	results := make([]*SummaryResult, len(reqs))
	for i, req := range reqs {
		result, err := s.Summarize(ctx, req)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// targetSentenceCount calculates how many sentences to extract.
func (s *ExtractiveStrategy) targetSentenceCount(total int, length SummaryLength) int {
	switch length {
	case SummaryLengthOneLine:
		return 1
	case SummaryLengthParagraph:
		target := total / 5
		if target < 2 {
			target = 2
		}
		if target > 5 {
			target = 5
		}
		return target
	case SummaryLengthFull:
		target := total / 3
		if target < 3 {
			target = 3
		}
		if target > 10 {
			target = 10
		}
		return target
	default:
		return 3
	}
}

// scoreSentences implements a simplified TextRank algorithm.
func (s *ExtractiveStrategy) scoreSentences(sentences []string) []float64 {
	n := len(sentences)
	if n == 0 {
		return nil
	}

	// Build similarity matrix
	similarity := make([][]float64, n)
	for i := range similarity {
		similarity[i] = make([]float64, n)
	}

	// Calculate sentence similarities
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			sim := sentenceSimilarity(sentences[i], sentences[j])
			similarity[i][j] = sim
			similarity[j][i] = sim
		}
	}

	// Initialize scores
	scores := make([]float64, n)
	for i := range scores {
		scores[i] = 1.0 / float64(n)
	}

	// Iterative scoring (simplified PageRank/TextRank)
	dampingFactor := 0.85
	iterations := 50

	for iter := 0; iter < iterations; iter++ {
		newScores := make([]float64, n)

		for i := 0; i < n; i++ {
			var sum float64
			for j := 0; j < n; j++ {
				if i != j && similarity[i][j] > 0 {
					// Sum of similarities for normalization
					var totalSim float64
					for k := 0; k < n; k++ {
						totalSim += similarity[j][k]
					}
					if totalSim > 0 {
						sum += similarity[i][j] / totalSim * scores[j]
					}
				}
			}
			newScores[i] = (1 - dampingFactor) + dampingFactor*sum
		}

		scores = newScores
	}

	// Add position bias (first and last sentences often important)
	for i := range scores {
		positionBonus := 0.0
		if i < 2 {
			positionBonus = 0.2 * (2 - float64(i)) / 2
		}
		if i >= n-2 {
			positionBonus += 0.1
		}
		scores[i] += positionBonus
	}

	return scores
}

// selectTopSentences selects top-scoring sentences while maintaining order.
func (s *ExtractiveStrategy) selectTopSentences(sentences []string, scores []float64, count int) []string {
	if count >= len(sentences) {
		return sentences
	}

	// Create index-score pairs
	type scoredSentence struct {
		index int
		score float64
	}

	scored := make([]scoredSentence, len(sentences))
	for i, score := range scores {
		scored[i] = scoredSentence{index: i, score: score}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Select top N indices
	selectedIndices := make([]int, count)
	for i := 0; i < count; i++ {
		selectedIndices[i] = scored[i].index
	}

	// Sort indices to maintain original order
	sort.Ints(selectedIndices)

	// Extract sentences
	result := make([]string, count)
	for i, idx := range selectedIndices {
		result[i] = sentences[idx]
	}

	return result
}

// extractKeyPoints extracts top sentences as key points.
func (s *ExtractiveStrategy) extractKeyPoints(sentences []string, scores []float64, count int) []string {
	if count > len(sentences) {
		count = len(sentences)
	}

	// Create index-score pairs
	type scoredSentence struct {
		index int
		score float64
	}

	scored := make([]scoredSentence, len(sentences))
	for i, score := range scores {
		scored[i] = scoredSentence{index: i, score: score}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Extract top sentences as key points
	keyPoints := make([]string, count)
	for i := 0; i < count; i++ {
		keyPoints[i] = strings.TrimSpace(sentences[scored[i].index])
	}

	return keyPoints
}

// AbstractiveStrategy uses AI to generate summaries.
type AbstractiveStrategy struct {
	aiClient AIClient
	logger   logging.Logger
}

// NewAbstractiveStrategy creates a new abstractive summarization strategy.
func NewAbstractiveStrategy(aiClient AIClient, logger logging.Logger) *AbstractiveStrategy {
	return &AbstractiveStrategy{
		aiClient: aiClient,
		logger:   logger.With(logging.F("strategy", "abstractive")),
	}
}

// Name returns the strategy name.
func (s *AbstractiveStrategy) Name() string {
	return "AI Abstractive"
}

// Type returns the strategy type.
func (s *AbstractiveStrategy) Type() StrategyType {
	return StrategyTypeAbstractive
}

// SupportsBatch indicates batch processing support.
func (s *AbstractiveStrategy) SupportsBatch() bool {
	return false // AI calls are typically sequential
}

// Summarize generates an AI-powered summary.
func (s *AbstractiveStrategy) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	if s.aiClient == nil {
		return nil, ErrAIClientRequired
	}

	// Build prompt based on request
	prompt := BuildSummaryPrompt(req)

	// Generate completion
	var response string
	var err error

	if req.ModelPreference != "" {
		response, err = s.aiClient.GenerateCompletionWithModel(ctx, prompt, req.TargetTokens()*2, req.ModelPreference)
	} else {
		response, err = s.aiClient.GenerateCompletion(ctx, prompt, req.TargetTokens()*2)
	}

	if err != nil {
		return nil, err
	}

	// Clean up response
	summary := cleanSummaryResponse(response)

	// Extract key points if requested
	var keyPoints []string
	if req.Format == SummaryFormatKeyPoints || req.Format == SummaryFormatBulletPoints {
		keyPoints = extractBulletPoints(summary)
	}

	// Estimate tokens
	inputTokens := s.aiClient.EstimateTokens(req.Content)
	outputTokens := s.aiClient.EstimateTokens(summary)

	modelUsed := s.aiClient.GetDefaultModel()
	if req.ModelPreference != "" {
		modelUsed = req.ModelPreference
	}

	return &SummaryResult{
		Summary:      summary,
		KeyPoints:    keyPoints,
		ModelUsed:    modelUsed,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Confidence:   0.85, // AI summaries have higher confidence
	}, nil
}

// SummarizeBatch processes multiple requests sequentially.
func (s *AbstractiveStrategy) SummarizeBatch(ctx context.Context, reqs []*SummaryRequest) ([]*SummaryResult, error) {
	results := make([]*SummaryResult, len(reqs))
	for i, req := range reqs {
		result, err := s.Summarize(ctx, req)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// HybridStrategy combines extractive and abstractive approaches.
type HybridStrategy struct {
	aiClient   AIClient
	extractive *ExtractiveStrategy
	logger     logging.Logger
}

// NewHybridStrategy creates a new hybrid summarization strategy.
func NewHybridStrategy(aiClient AIClient, logger logging.Logger) *HybridStrategy {
	return &HybridStrategy{
		aiClient:   aiClient,
		extractive: NewExtractiveStrategy(logger),
		logger:     logger.With(logging.F("strategy", "hybrid")),
	}
}

// Name returns the strategy name.
func (s *HybridStrategy) Name() string {
	return "Hybrid (Extractive + Abstractive)"
}

// Type returns the strategy type.
func (s *HybridStrategy) Type() StrategyType {
	return StrategyTypeHybrid
}

// SupportsBatch indicates batch processing support.
func (s *HybridStrategy) SupportsBatch() bool {
	return false
}

// Summarize combines extractive pre-processing with abstractive generation.
func (s *HybridStrategy) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	if s.aiClient == nil {
		// Fall back to extractive only
		return s.extractive.Summarize(ctx, req)
	}

	// Step 1: Extract key sentences first (reduces AI input size)
	extractiveReq := &SummaryRequest{
		Content:  req.Content,
		Length:   SummaryLengthFull, // Extract more for AI processing
		Strategy: StrategyTypeExtractive,
	}

	extractiveResult, err := s.extractive.Summarize(ctx, extractiveReq)
	if err != nil {
		return nil, err
	}

	// Step 2: Use extracted content for AI summary
	// This reduces token usage while preserving key information
	prompt := BuildHybridPrompt(req, extractiveResult.Summary, extractiveResult.KeyPoints)

	var response string
	if req.ModelPreference != "" {
		response, err = s.aiClient.GenerateCompletionWithModel(ctx, prompt, req.TargetTokens()*2, req.ModelPreference)
	} else {
		response, err = s.aiClient.GenerateCompletion(ctx, prompt, req.TargetTokens()*2)
	}

	if err != nil {
		// Fall back to extractive result
		s.logger.Warn("AI generation failed, using extractive fallback", logging.Err(err))
		return extractiveResult, nil
	}

	summary := cleanSummaryResponse(response)

	// Combine key points from both approaches
	keyPoints := extractiveResult.KeyPoints
	if req.Format == SummaryFormatKeyPoints || req.Format == SummaryFormatBulletPoints {
		aiKeyPoints := extractBulletPoints(summary)
		keyPoints = mergeKeyPoints(keyPoints, aiKeyPoints, 5)
	}

	modelUsed := s.aiClient.GetDefaultModel()
	if req.ModelPreference != "" {
		modelUsed = req.ModelPreference
	}

	return &SummaryResult{
		Summary:      summary,
		KeyPoints:    keyPoints,
		ModelUsed:    modelUsed,
		InputTokens:  s.aiClient.EstimateTokens(req.Content),
		OutputTokens: s.aiClient.EstimateTokens(summary),
		Confidence:   0.90, // Hybrid approach has highest confidence
	}, nil
}

// SummarizeBatch processes multiple requests.
func (s *HybridStrategy) SummarizeBatch(ctx context.Context, reqs []*SummaryRequest) ([]*SummaryResult, error) {
	results := make([]*SummaryResult, len(reqs))
	for i, req := range reqs {
		result, err := s.Summarize(ctx, req)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// HierarchicalStrategy generates multi-level summaries.
type HierarchicalStrategy struct {
	aiClient   AIClient
	extractive *ExtractiveStrategy
	logger     logging.Logger
}

// NewHierarchicalStrategy creates a new hierarchical summarization strategy.
func NewHierarchicalStrategy(aiClient AIClient, logger logging.Logger) *HierarchicalStrategy {
	return &HierarchicalStrategy{
		aiClient:   aiClient,
		extractive: NewExtractiveStrategy(logger),
		logger:     logger.With(logging.F("strategy", "hierarchical")),
	}
}

// Name returns the strategy name.
func (s *HierarchicalStrategy) Name() string {
	return "Hierarchical Multi-Level"
}

// Type returns the strategy type.
func (s *HierarchicalStrategy) Type() StrategyType {
	return StrategyTypeHierarchical
}

// SupportsBatch indicates batch processing support.
func (s *HierarchicalStrategy) SupportsBatch() bool {
	return false
}

// Summarize generates a single summary (implements Strategy interface).
func (s *HierarchicalStrategy) Summarize(ctx context.Context, req *SummaryRequest) (*SummaryResult, error) {
	hierarchical, err := s.SummarizeHierarchical(ctx, req.Content, req.TenantID)
	if err != nil {
		return nil, err
	}

	// Return appropriate level based on request
	var summary string
	switch req.Length {
	case SummaryLengthOneLine:
		summary = hierarchical.OneLine
	case SummaryLengthParagraph:
		summary = hierarchical.Paragraph
	case SummaryLengthFull:
		summary = hierarchical.Full
	default:
		summary = hierarchical.Paragraph
	}

	return &SummaryResult{
		Summary:   summary,
		KeyPoints: hierarchical.KeyPoints,
		ModelUsed: hierarchical.ModelUsed,
	}, nil
}

// SummarizeBatch processes multiple requests.
func (s *HierarchicalStrategy) SummarizeBatch(ctx context.Context, reqs []*SummaryRequest) ([]*SummaryResult, error) {
	results := make([]*SummaryResult, len(reqs))
	for i, req := range reqs {
		result, err := s.Summarize(ctx, req)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// SummarizeHierarchical generates all summary levels.
func (s *HierarchicalStrategy) SummarizeHierarchical(ctx context.Context, content string, tenantID string) (*HierarchicalSummary, error) {
	result := &HierarchicalSummary{}

	// Step 1: Extract key sentences for context
	extractiveReq := &SummaryRequest{
		Content: content,
		Length:  SummaryLengthFull,
	}
	extractive, err := s.extractive.Summarize(ctx, extractiveReq)
	if err != nil {
		return nil, err
	}
	result.KeyPoints = extractive.KeyPoints

	if s.aiClient == nil {
		// Fall back to extractive summaries at different lengths
		result.OneLine = s.extractSingleSentence(content)
		result.Paragraph = extractive.Summary

		// Full summary - just use more of the extractive result
		fullReq := &SummaryRequest{Content: content, Length: SummaryLengthFull}
		fullResult, _ := s.extractive.Summarize(ctx, fullReq)
		result.Full = fullResult.Summary

		return result, nil
	}

	// Generate all three levels with AI
	prompt := BuildHierarchicalPrompt(content, extractive.KeyPoints)

	response, err := s.aiClient.GenerateCompletion(ctx, prompt, 500)
	if err != nil {
		// Fall back to extractive
		result.OneLine = s.extractSingleSentence(content)
		result.Paragraph = extractive.Summary
		result.Full = extractive.Summary
		return result, nil
	}

	// Parse the hierarchical response
	result.OneLine, result.Paragraph, result.Full = parseHierarchicalResponse(response)
	result.ModelUsed = s.aiClient.GetDefaultModel()

	// Clean up any empty results
	if result.OneLine == "" {
		result.OneLine = s.extractSingleSentence(content)
	}
	if result.Paragraph == "" {
		result.Paragraph = extractive.Summary
	}
	if result.Full == "" {
		result.Full = extractive.Summary
	}

	return result, nil
}

// extractSingleSentence extracts the most important sentence.
func (s *HierarchicalStrategy) extractSingleSentence(content string) string {
	sentences := splitSentences(content)
	if len(sentences) == 0 {
		return content
	}

	scores := s.extractive.scoreSentences(sentences)
	if len(scores) == 0 {
		return sentences[0]
	}

	// Find highest scoring sentence
	maxIdx := 0
	maxScore := scores[0]
	for i, score := range scores {
		if score > maxScore {
			maxScore = score
			maxIdx = i
		}
	}

	return strings.TrimSpace(sentences[maxIdx])
}

// Helper functions

// splitSentences splits text into sentences.
func splitSentences(text string) []string {
	// Simple sentence splitting on . ! ?
	re := regexp.MustCompile(`[.!?]+\s+`)
	parts := re.Split(text, -1)

	var sentences []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) > 10 { // Filter out very short fragments
			sentences = append(sentences, part)
		}
	}

	return sentences
}

// sentenceSimilarity calculates word overlap similarity between sentences.
func sentenceSimilarity(s1, s2 string) float64 {
	words1 := tokenize(s1)
	words2 := tokenize(s2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0
	}

	// Count overlapping words
	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	overlap := 0
	for _, w := range words2 {
		if set1[w] {
			overlap++
		}
	}

	// Normalize by geometric mean of lengths
	denominator := math.Sqrt(float64(len(words1)) * float64(len(words2)))
	if denominator == 0 {
		return 0
	}

	return float64(overlap) / denominator
}

// tokenize splits text into lowercase words, filtering stopwords.
func tokenize(text string) []string {
	// Simple tokenization
	words := strings.FieldsFunc(strings.ToLower(text), func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	})

	// Filter stopwords
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"it": true, "its": true, "this": true, "that": true, "these": true,
		"those": true, "i": true, "you": true, "he": true, "she": true,
		"we": true, "they": true, "what": true, "which": true, "who": true,
		"when": true, "where": true, "why": true, "how": true, "all": true,
		"each": true, "every": true, "both": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "no": true,
		"not": true, "only": true, "same": true, "so": true, "than": true,
		"too": true, "very": true, "just": true, "also": true, "now": true,
	}

	var result []string
	for _, w := range words {
		if len(w) > 2 && !stopwords[w] {
			result = append(result, w)
		}
	}

	return result
}

// cleanSummaryResponse cleans up AI-generated summary text.
func cleanSummaryResponse(response string) string {
	// Remove common AI prefixes
	prefixes := []string{
		"Here is a summary:",
		"Here's a summary:",
		"Summary:",
		"Here is the summary:",
		"The summary is:",
	}

	text := strings.TrimSpace(response)
	for _, prefix := range prefixes {
		text = strings.TrimPrefix(text, prefix)
	}

	return strings.TrimSpace(text)
}

// extractBulletPoints extracts bullet points from text.
func extractBulletPoints(text string) []string {
	lines := strings.Split(text, "\n")
	var points []string

	bulletPrefixes := []string{"- ", "* ", "• ", "1. ", "2. ", "3. ", "4. ", "5. "}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for _, prefix := range bulletPrefixes {
			if strings.HasPrefix(line, prefix) {
				point := strings.TrimPrefix(line, prefix)
				points = append(points, strings.TrimSpace(point))
				break
			}
		}
	}

	return points
}

// mergeKeyPoints merges two lists of key points, deduplicating.
func mergeKeyPoints(list1, list2 []string, maxCount int) []string {
	seen := make(map[string]bool)
	var result []string

	// Add from first list
	for _, point := range list1 {
		normalized := strings.ToLower(strings.TrimSpace(point))
		if !seen[normalized] && len(result) < maxCount {
			seen[normalized] = true
			result = append(result, point)
		}
	}

	// Add from second list
	for _, point := range list2 {
		normalized := strings.ToLower(strings.TrimSpace(point))
		if !seen[normalized] && len(result) < maxCount {
			seen[normalized] = true
			result = append(result, point)
		}
	}

	return result
}

// parseHierarchicalResponse parses a structured hierarchical summary response.
func parseHierarchicalResponse(response string) (oneLine, paragraph, full string) {
	// Try to parse structured response with labels
	lines := strings.Split(response, "\n")
	currentSection := ""
	var currentContent strings.Builder

	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		if strings.HasPrefix(lower, "one-line:") || strings.HasPrefix(lower, "one line:") || strings.HasPrefix(lower, "tldr:") {
			if currentSection != "" && currentContent.Len() > 0 {
				assignSection(currentSection, currentContent.String(), &oneLine, &paragraph, &full)
			}
			currentSection = "oneline"
			currentContent.Reset()
			text := strings.TrimPrefix(line, strings.Split(line, ":")[0]+":")
			currentContent.WriteString(strings.TrimSpace(text))
		} else if strings.HasPrefix(lower, "paragraph:") || strings.HasPrefix(lower, "brief:") {
			if currentSection != "" && currentContent.Len() > 0 {
				assignSection(currentSection, currentContent.String(), &oneLine, &paragraph, &full)
			}
			currentSection = "paragraph"
			currentContent.Reset()
			text := strings.TrimPrefix(line, strings.Split(line, ":")[0]+":")
			currentContent.WriteString(strings.TrimSpace(text))
		} else if strings.HasPrefix(lower, "full:") || strings.HasPrefix(lower, "detailed:") {
			if currentSection != "" && currentContent.Len() > 0 {
				assignSection(currentSection, currentContent.String(), &oneLine, &paragraph, &full)
			}
			currentSection = "full"
			currentContent.Reset()
			text := strings.TrimPrefix(line, strings.Split(line, ":")[0]+":")
			currentContent.WriteString(strings.TrimSpace(text))
		} else if line != "" && currentSection != "" {
			if currentContent.Len() > 0 {
				currentContent.WriteString(" ")
			}
			currentContent.WriteString(line)
		}
	}

	// Handle last section
	if currentSection != "" && currentContent.Len() > 0 {
		assignSection(currentSection, currentContent.String(), &oneLine, &paragraph, &full)
	}

	// If parsing failed, use the whole response as full
	if oneLine == "" && paragraph == "" && full == "" {
		full = strings.TrimSpace(response)
		// Try to extract first sentence as one-line
		sentences := splitSentences(response)
		if len(sentences) > 0 {
			oneLine = sentences[0]
		}
		if len(sentences) > 1 {
			paragraph = strings.Join(sentences[:min(3, len(sentences))], ". ") + "."
		} else {
			paragraph = full
		}
	}

	return
}

// assignSection assigns content to the appropriate summary level.
func assignSection(section, content string, oneLine, paragraph, full *string) {
	content = strings.TrimSpace(content)
	switch section {
	case "oneline":
		*oneLine = content
	case "paragraph":
		*paragraph = content
	case "full":
		*full = content
	}
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
