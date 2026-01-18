package categorization

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// Common errors for classification.
var (
	ErrClassifierNotReady = errors.New("classifier not ready")
	ErrEmptyLabels        = errors.New("no candidate labels provided")
	ErrEmptyText          = errors.New("text is empty")
	ErrEmbeddingFailed    = errors.New("embedding generation failed")
)

// =============================================================================
// Zero-Shot Classification
// =============================================================================

// ZeroShotClassifier performs classification without training examples.
type ZeroShotClassifier struct {
	client           LLMClient
	logger           logging.Logger
	promptTemplate   string
	maxLabelsPerCall int
	mu               sync.RWMutex
}

// LLMClient is the interface for language model interaction.
type LLMClient interface {
	// Generate produces a response for the given prompt.
	Generate(ctx context.Context, prompt string) (string, error)

	// GenerateWithFormat produces a structured response.
	GenerateWithFormat(ctx context.Context, prompt string, format string) (string, error)
}

// ZeroShotConfig configures the zero-shot classifier.
type ZeroShotConfig struct {
	Client           LLMClient
	Logger           logging.Logger
	PromptTemplate   string
	MaxLabelsPerCall int
}

// DefaultZeroShotPrompt is the default prompt template for zero-shot classification.
const DefaultZeroShotPrompt = `You are a text classification system. Classify the following text into one or more of the given categories.

Text to classify:
"""
{{TEXT}}
"""

Available categories:
{{LABELS}}

Instructions:
1. Analyze the text content carefully
2. Select the most appropriate category or categories
3. Assign a confidence score (0.0 to 1.0) to each selected category
4. Only include categories with confidence > 0.3

Respond in this exact JSON format:
{
  "classifications": [
    {"label": "category_name", "confidence": 0.85},
    {"label": "another_category", "confidence": 0.65}
  ]
}

Only output the JSON, no other text.`

// NewZeroShotClassifier creates a new zero-shot classifier.
func NewZeroShotClassifier(config *ZeroShotConfig) (*ZeroShotClassifier, error) {
	if config.Client == nil {
		return nil, errors.New("LLM client is required")
	}
	if config.PromptTemplate == "" {
		config.PromptTemplate = DefaultZeroShotPrompt
	}
	if config.MaxLabelsPerCall == 0 {
		config.MaxLabelsPerCall = 20
	}

	return &ZeroShotClassifier{
		client:           config.Client,
		logger:           config.Logger.With(logging.F("classifier", "zero-shot")),
		promptTemplate:   config.PromptTemplate,
		maxLabelsPerCall: config.MaxLabelsPerCall,
	}, nil
}

// Classify performs zero-shot classification.
func (c *ZeroShotClassifier) Classify(ctx context.Context, text string, candidateLabels []string) ([]ClassificationScore, error) {
	if text == "" {
		return nil, ErrEmptyText
	}
	if len(candidateLabels) == 0 {
		return nil, ErrEmptyLabels
	}

	// Build the prompt
	labelsStr := strings.Join(candidateLabels, "\n- ")
	labelsStr = "- " + labelsStr

	prompt := strings.ReplaceAll(c.promptTemplate, "{{TEXT}}", text)
	prompt = strings.ReplaceAll(prompt, "{{LABELS}}", labelsStr)

	// Truncate text if too long (preserve beginning and end)
	maxTextLen := 4000
	if len(text) > maxTextLen {
		halfLen := maxTextLen / 2
		text = text[:halfLen] + "\n...[truncated]...\n" + text[len(text)-halfLen:]
		prompt = strings.ReplaceAll(c.promptTemplate, "{{TEXT}}", text)
		prompt = strings.ReplaceAll(prompt, "{{LABELS}}", labelsStr)
	}

	// Call LLM
	response, err := c.client.GenerateWithFormat(ctx, prompt, "json")
	if err != nil {
		c.logger.Error("LLM generation failed", logging.Err(err))
		return nil, err
	}

	// Parse response
	scores, err := c.parseClassificationResponse(response, candidateLabels)
	if err != nil {
		c.logger.Warn("failed to parse classification response",
			logging.Err(err),
			logging.F("response", response),
		)
		return nil, err
	}

	// Sort by confidence
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Confidence > scores[j].Confidence
	})

	return scores, nil
}

// parseClassificationResponse parses the LLM response into scores.
func (c *ZeroShotClassifier) parseClassificationResponse(response string, validLabels []string) ([]ClassificationScore, error) {
	// Simple JSON parsing - in production use encoding/json
	// This is a simplified parser for the expected format
	response = strings.TrimSpace(response)

	// Create valid labels map for validation
	validLabelMap := make(map[string]bool)
	for _, l := range validLabels {
		validLabelMap[strings.ToLower(l)] = true
	}

	var scores []ClassificationScore

	// Find classifications array
	if !strings.Contains(response, "classifications") {
		return scores, errors.New("invalid response format: missing classifications")
	}

	// Extract label-confidence pairs
	// Look for patterns like "label": "value", "confidence": number
	lines := strings.Split(response, "\n")
	var currentLabel string
	var currentConfidence float64

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for label
		if strings.Contains(line, `"label"`) {
			// Extract value between quotes after the colon
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				value := strings.Trim(strings.TrimSpace(parts[1]), `",`)
				currentLabel = value
			}
		}

		// Look for confidence
		if strings.Contains(line, `"confidence"`) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				value := strings.Trim(strings.TrimSpace(parts[1]), `",}`)
				_, err := fmt.Sscanf(value, "%f", &currentConfidence)
				if err == nil && currentLabel != "" {
					// Validate label
					if validLabelMap[strings.ToLower(currentLabel)] {
						scores = append(scores, ClassificationScore{
							Label:      currentLabel,
							Confidence: currentConfidence,
						})
					}
					currentLabel = ""
					currentConfidence = 0
				}
			}
		}
	}

	return scores, nil
}

// =============================================================================
// Few-Shot Classification
// =============================================================================

// Example represents a training example for few-shot classification.
type Example struct {
	Text  string
	Label string
}

// FewShotClassifier performs classification with examples.
type FewShotClassifier struct {
	client         LLMClient
	logger         logging.Logger
	examples       map[string][]Example
	maxExamples    int
	promptTemplate string
	mu             sync.RWMutex
}

// FewShotConfig configures the few-shot classifier.
type FewShotConfig struct {
	Client         LLMClient
	Logger         logging.Logger
	MaxExamples    int
	PromptTemplate string
}

// DefaultFewShotPrompt is the default prompt for few-shot classification.
const DefaultFewShotPrompt = `You are a text classification system. Learn from the examples below and classify the new text.

Examples:
{{EXAMPLES}}

Now classify this text:
"""
{{TEXT}}
"""

Available categories:
{{LABELS}}

Respond in this exact JSON format:
{
  "classifications": [
    {"label": "category_name", "confidence": 0.85}
  ]
}

Only output the JSON, no other text.`

// NewFewShotClassifier creates a new few-shot classifier.
func NewFewShotClassifier(config *FewShotConfig) (*FewShotClassifier, error) {
	if config.Client == nil {
		return nil, errors.New("LLM client is required")
	}
	if config.MaxExamples == 0 {
		config.MaxExamples = 5
	}
	if config.PromptTemplate == "" {
		config.PromptTemplate = DefaultFewShotPrompt
	}

	return &FewShotClassifier{
		client:         config.Client,
		logger:         config.Logger.With(logging.F("classifier", "few-shot")),
		examples:       make(map[string][]Example),
		maxExamples:    config.MaxExamples,
		promptTemplate: config.PromptTemplate,
	}, nil
}

// AddExample adds a training example for a category.
func (c *FewShotClassifier) AddExample(label string, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.examples[label] = append(c.examples[label], Example{
		Text:  text,
		Label: label,
	})

	// Limit examples per category
	if len(c.examples[label]) > c.maxExamples*2 {
		c.examples[label] = c.examples[label][len(c.examples[label])-c.maxExamples*2:]
	}
}

// AddExamples adds multiple training examples.
func (c *FewShotClassifier) AddExamples(examples []Example) {
	for _, ex := range examples {
		c.AddExample(ex.Label, ex.Text)
	}
}

// Classify performs few-shot classification.
func (c *FewShotClassifier) Classify(ctx context.Context, text string, candidateLabels []string) ([]ClassificationScore, error) {
	if text == "" {
		return nil, ErrEmptyText
	}
	if len(candidateLabels) == 0 {
		return nil, ErrEmptyLabels
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Build examples string
	var examplesBuilder strings.Builder
	for _, label := range candidateLabels {
		examples := c.examples[label]
		// Take up to maxExamples per category
		count := c.maxExamples
		if len(examples) < count {
			count = len(examples)
		}
		for i := 0; i < count; i++ {
			examplesBuilder.WriteString(fmt.Sprintf("Category: %s\nText: \"%s\"\n\n", label, truncateText(examples[i].Text, 200)))
		}
	}

	// Build labels string
	labelsStr := "- " + strings.Join(candidateLabels, "\n- ")

	// Build prompt
	prompt := strings.ReplaceAll(c.promptTemplate, "{{EXAMPLES}}", examplesBuilder.String())
	prompt = strings.ReplaceAll(prompt, "{{TEXT}}", text)
	prompt = strings.ReplaceAll(prompt, "{{LABELS}}", labelsStr)

	// Call LLM
	response, err := c.client.GenerateWithFormat(ctx, prompt, "json")
	if err != nil {
		return nil, err
	}

	// Parse response (reuse zero-shot parser logic)
	return parseClassificationJSON(response, candidateLabels)
}

// =============================================================================
// Embedding-Based Classification
// =============================================================================

// EmbeddingClient generates embeddings for text.
type EmbeddingClient interface {
	// Embed generates an embedding vector for the text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbeddingClassifier uses embedding similarity for classification.
type EmbeddingClassifier struct {
	client           EmbeddingClient
	logger           logging.Logger
	categoryEmbeds   map[string][]float32
	categoryExamples map[string][][]float32
	similarityThreshold float64
	mu               sync.RWMutex
}

// EmbeddingClassifierConfig configures the embedding classifier.
type EmbeddingClassifierConfig struct {
	Client              EmbeddingClient
	Logger              logging.Logger
	SimilarityThreshold float64
}

// NewEmbeddingClassifier creates a new embedding-based classifier.
func NewEmbeddingClassifier(config *EmbeddingClassifierConfig) (*EmbeddingClassifier, error) {
	if config.Client == nil {
		return nil, errors.New("embedding client is required")
	}
	if config.SimilarityThreshold == 0 {
		config.SimilarityThreshold = 0.7
	}

	return &EmbeddingClassifier{
		client:              config.Client,
		logger:              config.Logger.With(logging.F("classifier", "embedding")),
		categoryEmbeds:      make(map[string][]float32),
		categoryExamples:    make(map[string][][]float32),
		similarityThreshold: config.SimilarityThreshold,
	}, nil
}

// SetCategoryDescription sets the description for a category (used for embedding).
func (c *EmbeddingClassifier) SetCategoryDescription(ctx context.Context, categoryID, description string) error {
	embed, err := c.client.Embed(ctx, description)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	c.mu.Lock()
	c.categoryEmbeds[categoryID] = embed
	c.mu.Unlock()

	return nil
}

// AddExample adds an example embedding for a category.
func (c *EmbeddingClassifier) AddExample(ctx context.Context, categoryID, text string) error {
	embed, err := c.client.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	c.mu.Lock()
	c.categoryExamples[categoryID] = append(c.categoryExamples[categoryID], embed)
	c.mu.Unlock()

	return nil
}

// Classify performs embedding-based classification.
func (c *EmbeddingClassifier) Classify(ctx context.Context, text string, candidateLabels []string) ([]ClassificationScore, error) {
	if text == "" {
		return nil, ErrEmptyText
	}
	if len(candidateLabels) == 0 {
		return nil, ErrEmptyLabels
	}

	// Generate embedding for input text
	textEmbed, err := c.client.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var scores []ClassificationScore

	for _, label := range candidateLabels {
		var similarity float64 = 0

		// Check category description embedding
		if catEmbed, exists := c.categoryEmbeds[label]; exists {
			sim := cosineSimilarity(textEmbed, catEmbed)
			if sim > similarity {
				similarity = sim
			}
		}

		// Check example embeddings
		if examples, exists := c.categoryExamples[label]; exists {
			for _, exEmbed := range examples {
				sim := cosineSimilarity(textEmbed, exEmbed)
				if sim > similarity {
					similarity = sim
				}
			}
		}

		if similarity >= c.similarityThreshold {
			scores = append(scores, ClassificationScore{
				Label:      label,
				Confidence: similarity,
			})
		}
	}

	// Sort by confidence
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Confidence > scores[j].Confidence
	})

	return scores, nil
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// =============================================================================
// Ensemble Classifier
// =============================================================================

// EnsembleClassifier combines multiple classifiers.
type EnsembleClassifier struct {
	classifiers []MLClassifier
	weights     []float64
	logger      logging.Logger
	strategy    EnsembleStrategy
	mu          sync.RWMutex
}

// EnsembleStrategy defines how to combine classifier results.
type EnsembleStrategy string

const (
	// StrategyWeightedAverage averages confidences with weights.
	StrategyWeightedAverage EnsembleStrategy = "weighted_average"

	// StrategyMaxConfidence takes the highest confidence.
	StrategyMaxConfidence EnsembleStrategy = "max_confidence"

	// StrategyVoting uses majority voting.
	StrategyVoting EnsembleStrategy = "voting"
)

// EnsembleConfig configures the ensemble classifier.
type EnsembleConfig struct {
	Classifiers []MLClassifier
	Weights     []float64
	Logger      logging.Logger
	Strategy    EnsembleStrategy
}

// NewEnsembleClassifier creates a new ensemble classifier.
func NewEnsembleClassifier(config *EnsembleConfig) (*EnsembleClassifier, error) {
	if len(config.Classifiers) == 0 {
		return nil, errors.New("at least one classifier is required")
	}

	weights := config.Weights
	if len(weights) == 0 {
		// Equal weights
		weights = make([]float64, len(config.Classifiers))
		for i := range weights {
			weights[i] = 1.0 / float64(len(config.Classifiers))
		}
	}

	if len(weights) != len(config.Classifiers) {
		return nil, errors.New("weights count must match classifiers count")
	}

	strategy := config.Strategy
	if strategy == "" {
		strategy = StrategyWeightedAverage
	}

	return &EnsembleClassifier{
		classifiers: config.Classifiers,
		weights:     weights,
		logger:      config.Logger.With(logging.F("classifier", "ensemble")),
		strategy:    strategy,
	}, nil
}

// Classify combines results from all classifiers.
func (c *EnsembleClassifier) Classify(ctx context.Context, text string, candidateLabels []string) ([]ClassificationScore, error) {
	if text == "" {
		return nil, ErrEmptyText
	}
	if len(candidateLabels) == 0 {
		return nil, ErrEmptyLabels
	}

	// Collect results from all classifiers
	allResults := make([][]ClassificationScore, len(c.classifiers))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, classifier := range c.classifiers {
		wg.Add(1)
		go func(idx int, clf MLClassifier) {
			defer wg.Done()

			results, err := clf.Classify(ctx, text, candidateLabels)
			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			allResults[idx] = results
			mu.Unlock()
		}(i, classifier)
	}

	wg.Wait()

	// Combine results based on strategy
	switch c.strategy {
	case StrategyWeightedAverage:
		return c.weightedAverage(allResults, candidateLabels), nil
	case StrategyMaxConfidence:
		return c.maxConfidence(allResults), nil
	case StrategyVoting:
		return c.voting(allResults, candidateLabels), nil
	default:
		return c.weightedAverage(allResults, candidateLabels), nil
	}
}

// weightedAverage combines scores using weighted average.
func (c *EnsembleClassifier) weightedAverage(allResults [][]ClassificationScore, labels []string) []ClassificationScore {
	labelScores := make(map[string]float64)

	for i, results := range allResults {
		weight := c.weights[i]
		for _, score := range results {
			labelScores[score.Label] += score.Confidence * weight
		}
	}

	var scores []ClassificationScore
	for label, confidence := range labelScores {
		scores = append(scores, ClassificationScore{
			Label:      label,
			Confidence: confidence,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Confidence > scores[j].Confidence
	})

	return scores
}

// maxConfidence takes the highest confidence for each label.
func (c *EnsembleClassifier) maxConfidence(allResults [][]ClassificationScore) []ClassificationScore {
	labelScores := make(map[string]float64)

	for _, results := range allResults {
		for _, score := range results {
			if score.Confidence > labelScores[score.Label] {
				labelScores[score.Label] = score.Confidence
			}
		}
	}

	var scores []ClassificationScore
	for label, confidence := range labelScores {
		scores = append(scores, ClassificationScore{
			Label:      label,
			Confidence: confidence,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Confidence > scores[j].Confidence
	})

	return scores
}

// voting uses majority voting with confidence as tiebreaker.
func (c *EnsembleClassifier) voting(allResults [][]ClassificationScore, labels []string) []ClassificationScore {
	votes := make(map[string]int)
	maxConfidences := make(map[string]float64)

	for _, results := range allResults {
		if len(results) > 0 {
			// Vote for top result
			top := results[0]
			votes[top.Label]++
			if top.Confidence > maxConfidences[top.Label] {
				maxConfidences[top.Label] = top.Confidence
			}
		}
	}

	var scores []ClassificationScore
	for label, voteCount := range votes {
		// Normalize vote count to confidence
		confidence := float64(voteCount) / float64(len(c.classifiers))
		// Blend with max confidence
		confidence = (confidence + maxConfidences[label]) / 2

		scores = append(scores, ClassificationScore{
			Label:      label,
			Confidence: confidence,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Confidence > scores[j].Confidence
	})

	return scores
}

// =============================================================================
// Helper Functions
// =============================================================================

// truncateText truncates text to maxLen, adding ellipsis.
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

// parseClassificationJSON parses classification JSON response.
func parseClassificationJSON(response string, validLabels []string) ([]ClassificationScore, error) {
	response = strings.TrimSpace(response)

	validLabelMap := make(map[string]bool)
	for _, l := range validLabels {
		validLabelMap[strings.ToLower(l)] = true
	}

	var scores []ClassificationScore

	if !strings.Contains(response, "classifications") {
		return scores, errors.New("invalid response format")
	}

	lines := strings.Split(response, "\n")
	var currentLabel string
	var currentConfidence float64

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, `"label"`) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				value := strings.Trim(strings.TrimSpace(parts[1]), `",`)
				currentLabel = value
			}
		}

		if strings.Contains(line, `"confidence"`) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				value := strings.Trim(strings.TrimSpace(parts[1]), `",}`)
				_, err := fmt.Sscanf(value, "%f", &currentConfidence)
				if err == nil && currentLabel != "" {
					if validLabelMap[strings.ToLower(currentLabel)] {
						scores = append(scores, ClassificationScore{
							Label:      currentLabel,
							Confidence: currentConfidence,
						})
					}
					currentLabel = ""
					currentConfidence = 0
				}
			}
		}
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Confidence > scores[j].Confidence
	})

	return scores, nil
}
