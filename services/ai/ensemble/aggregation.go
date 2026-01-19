package ensemble

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// AggregatorConfig holds configuration for the Aggregator.
type AggregatorConfig struct {
	// DefaultConsensusThreshold is the default threshold for consensus.
	DefaultConsensusThreshold float64

	// DisagreementThreshold is when responses are considered to disagree.
	DisagreementThreshold float64

	// EnableExplanations enables detailed aggregation explanations.
	EnableExplanations bool
}

// DefaultAggregatorConfig returns a configuration with sensible defaults.
func DefaultAggregatorConfig() *AggregatorConfig {
	return &AggregatorConfig{
		DefaultConsensusThreshold: 0.7,
		DisagreementThreshold:     0.5,
		EnableExplanations:        true,
	}
}

// Aggregator handles result aggregation from multiple models.
type Aggregator struct {
	mu     sync.RWMutex
	config *AggregatorConfig
	logger logging.Logger
}

// NewAggregator creates a new Aggregator with the given configuration.
func NewAggregator(cfg *AggregatorConfig, logger logging.Logger) *Aggregator {
	if cfg == nil {
		cfg = DefaultAggregatorConfig()
	}

	if logger == nil {
		logger = logging.MustGlobal()
	}

	return &Aggregator{
		config: cfg,
		logger: logger.With(logging.F("component", "aggregator")),
	}
}

// UpdateConfig updates the aggregator configuration.
func (a *Aggregator) UpdateConfig(cfg *AggregatorConfig) {
	if cfg == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = cfg
}

// AggregateResults performs generic result aggregation.
func (a *Aggregator) AggregateResults(results []ModelResult) *EnsembleResult {
	if len(results) == 0 {
		return &EnsembleResult{
			ConsensusScore:    0,
			IndividualResults: results,
			Explanation:       "No results to aggregate",
		}
	}

	if len(results) == 1 {
		return &EnsembleResult{
			FinalResponse:     results[0].Response,
			SelectedModel:     results[0].ModelID,
			ConsensusScore:    1.0,
			IndividualResults: results,
			TotalTokensUsed:   results[0].TokensUsed,
			Explanation:       "Single model response",
		}
	}

	// Default to weighted voting for generic aggregation
	modelQualities := make(map[string]float64)
	for _, r := range results {
		modelQualities[r.ModelID] = r.Confidence
	}

	return a.AggregateWeightedVoting(results, modelQualities)
}

// AggregateMajorityVoting selects the most common response.
func (a *Aggregator) AggregateMajorityVoting(results []ModelResult) *EnsembleResult {
	if len(results) == 0 {
		return &EnsembleResult{
			ConsensusScore: 0,
			Explanation:    "No results to aggregate",
		}
	}

	// Hash responses for comparison
	responseHashes := make(map[string][]ModelResult)
	for _, r := range results {
		hash := hashResponse(r.Response)
		responseHashes[hash] = append(responseHashes[hash], r)
	}

	// Find the most common response
	var maxCount int
	var majorityResults []ModelResult

	for _, rs := range responseHashes {
		if len(rs) > maxCount {
			maxCount = len(rs)
			majorityResults = rs
		}
	}

	// Calculate consensus score
	consensusScore := float64(maxCount) / float64(len(results))

	// Detect disagreements
	disagreements := a.detectDisagreements(results, responseHashes)

	// Select the highest confidence response from the majority
	var bestResult *ModelResult
	for i := range majorityResults {
		if bestResult == nil || majorityResults[i].Confidence > bestResult.Confidence {
			bestResult = &majorityResults[i]
		}
	}

	// Calculate total tokens
	totalTokens := 0
	for _, r := range results {
		totalTokens += r.TokensUsed
	}

	explanation := fmt.Sprintf("Majority voting: %d/%d models agreed (%.1f%% consensus)",
		maxCount, len(results), consensusScore*100)

	return &EnsembleResult{
		FinalResponse:     bestResult.Response,
		SelectedModel:     bestResult.ModelID,
		ConsensusScore:    consensusScore,
		IndividualResults: results,
		TotalTokensUsed:   totalTokens,
		Disagreements:     disagreements,
		Explanation:       explanation,
	}
}

// AggregateWeightedVoting weights responses by confidence and model quality.
func (a *Aggregator) AggregateWeightedVoting(results []ModelResult, modelQualities map[string]float64) *EnsembleResult {
	if len(results) == 0 {
		return &EnsembleResult{
			ConsensusScore: 0,
			Explanation:    "No results to aggregate",
		}
	}

	// Calculate weighted scores for each response
	type weightedResponse struct {
		result      *ModelResult
		totalWeight float64
		supporters  []string
	}

	responseWeights := make(map[string]*weightedResponse)

	for i := range results {
		r := &results[i]
		hash := hashResponse(r.Response)

		// Calculate weight: confidence * model quality
		quality := modelQualities[r.ModelID]
		if quality == 0 {
			quality = 0.5 // Default quality
		}
		weight := r.Confidence * quality

		if wr, exists := responseWeights[hash]; exists {
			wr.totalWeight += weight
			wr.supporters = append(wr.supporters, r.ModelID)
			// Keep the highest confidence result for this response
			if r.Confidence > wr.result.Confidence {
				wr.result = r
			}
		} else {
			responseWeights[hash] = &weightedResponse{
				result:      r,
				totalWeight: weight,
				supporters:  []string{r.ModelID},
			}
		}
	}

	// Find the response with highest total weight
	var bestWeighted *weightedResponse
	var totalWeight float64

	for _, wr := range responseWeights {
		totalWeight += wr.totalWeight
		if bestWeighted == nil || wr.totalWeight > bestWeighted.totalWeight {
			bestWeighted = wr
		}
	}

	// Calculate consensus score
	consensusScore := bestWeighted.totalWeight / totalWeight

	// Detect disagreements
	responseHashes := make(map[string][]ModelResult)
	for _, r := range results {
		hash := hashResponse(r.Response)
		responseHashes[hash] = append(responseHashes[hash], r)
	}
	disagreements := a.detectDisagreements(results, responseHashes)

	// Calculate total tokens
	totalTokens := 0
	for _, r := range results {
		totalTokens += r.TokensUsed
	}

	explanation := fmt.Sprintf("Weighted voting: selected response supported by %v with %.2f weight (%.1f%% of total)",
		bestWeighted.supporters, bestWeighted.totalWeight, consensusScore*100)

	return &EnsembleResult{
		FinalResponse:     bestWeighted.result.Response,
		SelectedModel:     bestWeighted.result.ModelID,
		ConsensusScore:    consensusScore,
		IndividualResults: results,
		TotalTokensUsed:   totalTokens,
		Disagreements:     disagreements,
		Explanation:       explanation,
	}
}

// AggregateParallel combines parallel results using weighted voting.
func (a *Aggregator) AggregateParallel(results []ModelResult, modelQualities map[string]float64) *EnsembleResult {
	// For parallel strategy, we use weighted voting as the aggregation method
	result := a.AggregateWeightedVoting(results, modelQualities)
	result.Explanation = "Parallel execution with " + result.Explanation
	return result
}

// AggregateDebate aggregates results from a debate strategy.
func (a *Aggregator) AggregateDebate(results []ModelResult, modelQualities map[string]float64) *EnsembleResult {
	// Use weighted voting but with emphasis on the final round responses
	result := a.AggregateWeightedVoting(results, modelQualities)
	result.Explanation = "Debate conclusion: " + result.Explanation
	return result
}

// CalculateConsensusScore calculates how much models agree.
func (a *Aggregator) CalculateConsensusScore(results []ModelResult) float64 {
	if len(results) <= 1 {
		return 1.0
	}

	// Group by response hash
	responseHashes := make(map[string]int)
	for _, r := range results {
		hash := hashResponse(r.Response)
		responseHashes[hash]++
	}

	// Find the most common response
	maxCount := 0
	for _, count := range responseHashes {
		if count > maxCount {
			maxCount = count
		}
	}

	return float64(maxCount) / float64(len(results))
}

// detectDisagreements identifies significant disagreements between models.
func (a *Aggregator) detectDisagreements(results []ModelResult, responseHashes map[string][]ModelResult) []Disagreement {
	var disagreements []Disagreement

	a.mu.RLock()
	threshold := a.config.DisagreementThreshold
	a.mu.RUnlock()

	// If there's only one unique response, no disagreements
	if len(responseHashes) <= 1 {
		return disagreements
	}

	// Get response groups sorted by size
	type responseGroup struct {
		hash    string
		results []ModelResult
	}

	var groups []responseGroup
	for hash, rs := range responseHashes {
		groups = append(groups, responseGroup{hash: hash, results: rs})
	}

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i].results) > len(groups[j].results)
	})

	// Compare major groups
	for i := 0; i < len(groups)-1; i++ {
		for j := i + 1; j < len(groups); j++ {
			groupI := groups[i]
			groupJ := groups[j]

			// Calculate severity based on confidence difference
			avgConfI := averageConfidence(groupI.results)
			avgConfJ := averageConfidence(groupJ.results)
			severity := 1.0 - (avgConfI - avgConfJ)
			if severity < 0 {
				severity = -severity
			}

			// Only report significant disagreements
			if severity > threshold {
				for _, rI := range groupI.results {
					for _, rJ := range groupJ.results {
						disagreements = append(disagreements, Disagreement{
							ModelA:      rI.ModelID,
							ModelB:      rJ.ModelID,
							Description: fmt.Sprintf("Different responses with confidence %.2f vs %.2f", rI.Confidence, rJ.Confidence),
							Severity:    severity,
						})
					}
				}
			}
		}
	}

	return disagreements
}

// SynthesizeExplanation creates a human-readable explanation of the aggregation.
func (a *Aggregator) SynthesizeExplanation(result *EnsembleResult) string {
	if result == nil {
		return "No result to explain"
	}

	var parts []string

	// Strategy used
	if result.Strategy != "" {
		parts = append(parts, fmt.Sprintf("Strategy: %s", result.Strategy))
	}

	// Model count
	successCount := 0
	for _, r := range result.IndividualResults {
		if r.IsSuccess() {
			successCount++
		}
	}
	parts = append(parts, fmt.Sprintf("Models queried: %d (%d successful)", len(result.IndividualResults), successCount))

	// Consensus
	parts = append(parts, fmt.Sprintf("Consensus: %.1f%%", result.ConsensusScore*100))

	// Selected model
	if result.SelectedModel != "" {
		parts = append(parts, fmt.Sprintf("Selected: %s", result.SelectedModel))
	}

	// Disagreements
	if len(result.Disagreements) > 0 {
		parts = append(parts, fmt.Sprintf("Disagreements: %d", len(result.Disagreements)))
	}

	// Processing time
	if result.TotalProcessingTime > 0 {
		parts = append(parts, fmt.Sprintf("Time: %v", result.TotalProcessingTime))
	}

	// Cost
	if result.EstimatedCost > 0 {
		parts = append(parts, fmt.Sprintf("Cost: $%.6f", result.EstimatedCost))
	}

	return strings.Join(parts, " | ")
}

// MergeResults combines results from multiple ensemble operations.
func (a *Aggregator) MergeResults(results []*EnsembleResult) *EnsembleResult {
	if len(results) == 0 {
		return nil
	}

	if len(results) == 1 {
		return results[0]
	}

	// Collect all individual results
	var allIndividual []ModelResult
	var totalTokens int
	var totalCost float64

	for _, r := range results {
		allIndividual = append(allIndividual, r.IndividualResults...)
		totalTokens += r.TotalTokensUsed
		totalCost += r.EstimatedCost
	}

	// Collect all disagreements
	var allDisagreements []Disagreement
	for _, r := range results {
		allDisagreements = append(allDisagreements, r.Disagreements...)
	}

	// Use weighted voting on all collected results
	modelQualities := make(map[string]float64)
	for _, r := range allIndividual {
		// Use confidence as proxy for quality if not available
		if _, exists := modelQualities[r.ModelID]; !exists {
			modelQualities[r.ModelID] = r.Confidence
		}
	}

	merged := a.AggregateWeightedVoting(allIndividual, modelQualities)
	merged.IndividualResults = allIndividual
	merged.TotalTokensUsed = totalTokens
	merged.EstimatedCost = totalCost
	merged.Disagreements = allDisagreements
	merged.Explanation = fmt.Sprintf("Merged from %d ensemble results: %s", len(results), merged.Explanation)

	return merged
}

// CompareResponses compares two responses for similarity.
func (a *Aggregator) CompareResponses(r1, r2 interface{}) float64 {
	hash1 := hashResponse(r1)
	hash2 := hashResponse(r2)

	if hash1 == hash2 {
		return 1.0
	}

	// For string responses, calculate simple similarity
	s1, ok1 := r1.(string)
	s2, ok2 := r2.(string)
	if ok1 && ok2 {
		return stringSimilarity(s1, s2)
	}

	// Different hashes, assume no similarity
	return 0.0
}

// hashResponse creates a hash for response comparison.
func hashResponse(response interface{}) string {
	// Convert to string for hashing
	str := fmt.Sprintf("%v", response)
	// Normalize whitespace
	str = strings.TrimSpace(str)
	str = strings.ToLower(str)

	hash := sha256.Sum256([]byte(str))
	return hex.EncodeToString(hash[:8]) // Use first 8 bytes for shorter hash
}

// averageConfidence calculates the average confidence of results.
func averageConfidence(results []ModelResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	var total float64
	for _, r := range results {
		total += r.Confidence
	}
	return total / float64(len(results))
}

// stringSimilarity calculates simple string similarity using Jaccard index.
func stringSimilarity(s1, s2 string) float64 {
	words1 := strings.Fields(strings.ToLower(s1))
	words2 := strings.Fields(strings.ToLower(s2))

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}

	set1 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}

	set2 := make(map[string]bool)
	for _, w := range words2 {
		set2[w] = true
	}

	// Calculate intersection
	intersection := 0
	for w := range set1 {
		if set2[w] {
			intersection++
		}
	}

	// Calculate union
	union := len(set1)
	for w := range set2 {
		if !set1[w] {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// ResponseClassifier helps classify and group similar responses.
type ResponseClassifier struct {
	similarityThreshold float64
}

// NewResponseClassifier creates a new response classifier.
func NewResponseClassifier(threshold float64) *ResponseClassifier {
	if threshold <= 0 {
		threshold = 0.8
	}
	return &ResponseClassifier{similarityThreshold: threshold}
}

// ClassifyResponses groups similar responses together.
func (c *ResponseClassifier) ClassifyResponses(results []ModelResult) [][]ModelResult {
	if len(results) == 0 {
		return nil
	}

	var groups [][]ModelResult

	for _, r := range results {
		added := false
		for i, group := range groups {
			// Check if response belongs to this group
			if len(group) > 0 {
				// Compare with first response in group
				sim := stringSimilarity(
					fmt.Sprintf("%v", r.Response),
					fmt.Sprintf("%v", group[0].Response),
				)
				if sim >= c.similarityThreshold {
					groups[i] = append(groups[i], r)
					added = true
					break
				}
			}
		}

		if !added {
			groups = append(groups, []ModelResult{r})
		}
	}

	return groups
}
