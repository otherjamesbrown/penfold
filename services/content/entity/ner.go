package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AIClient defines the interface for AI service operations used by NER.
type AIClient interface {
	// Complete sends a prompt to the AI and returns the completion.
	Complete(ctx context.Context, prompt string, opts *CompletionOptions) (*CompletionResponse, error)
}

// CompletionOptions configures AI completion requests.
type CompletionOptions struct {
	// Model specifies which model to use.
	Model string

	// Temperature controls randomness (0.0-1.0).
	Temperature float32

	// MaxTokens limits the response length.
	MaxTokens int

	// SystemPrompt is the system message.
	SystemPrompt string
}

// CompletionResponse contains the AI completion result.
type CompletionResponse struct {
	// Text is the generated text.
	Text string

	// Model is the model that was used.
	Model string

	// TokensUsed is the number of tokens consumed.
	TokensUsed int
}

// NERExtractor extracts entities using AI-based named entity recognition.
type NERExtractor struct {
	client         AIClient
	model          string
	maxTokens      int
	temperature    float32
	systemPrompt   string
	maxTextLength  int
	chunkOverlap   int
}

// NERConfig holds configuration for the NER extractor.
type NERConfig struct {
	// Model is the AI model to use for NER.
	Model string

	// MaxTokens limits the AI response length.
	MaxTokens int

	// Temperature controls AI randomness.
	Temperature float32

	// SystemPrompt is a custom system prompt (optional).
	SystemPrompt string

	// MaxTextLength is the maximum text length to process at once.
	MaxTextLength int

	// ChunkOverlap is the overlap when splitting long text.
	ChunkOverlap int
}

// DefaultNERConfig returns default NER configuration.
func DefaultNERConfig() *NERConfig {
	return &NERConfig{
		Model:         "gemini-1.5-flash",
		MaxTokens:     4096,
		Temperature:   0.1,
		SystemPrompt:  "",
		MaxTextLength: 10000,
		ChunkOverlap:  200,
	}
}

// NewNERExtractor creates a new AI-based NER extractor.
func NewNERExtractor(client AIClient, cfg *NERConfig) *NERExtractor {
	if cfg == nil {
		cfg = DefaultNERConfig()
	}

	return &NERExtractor{
		client:        client,
		model:         cfg.Model,
		maxTokens:     cfg.MaxTokens,
		temperature:   cfg.Temperature,
		systemPrompt:  cfg.SystemPrompt,
		maxTextLength: cfg.MaxTextLength,
		chunkOverlap:  cfg.ChunkOverlap,
	}
}

// NERPromptTemplate is the default prompt for entity extraction.
const NERPromptTemplate = `Extract all named entities from the following text. Return the results as a JSON array with objects containing these fields:
- "type": one of "person", "organization", "location", "date", "money", "event", "product", "misc"
- "value": the exact text of the entity as it appears
- "confidence": a float from 0.0 to 1.0 indicating confidence
- "start": character offset where the entity starts (optional)
- "end": character offset where the entity ends (optional)
- "context": brief context about the entity (optional)

Focus on:
- People names (full names, titles, nicknames)
- Organizations (companies, institutions, teams, departments)
- Locations (cities, countries, addresses, landmarks)
- Dates and times (specific dates, date ranges, relative dates)
- Money amounts (with currency)
- Events (meetings, conferences, holidays)
- Products (product names, software, services)

Rules:
- Do NOT extract email addresses, URLs, or phone numbers (these are handled separately)
- Extract each entity only once, even if it appears multiple times
- For ambiguous entities, use "misc" type
- Return ONLY valid JSON, no explanations

Text to analyze:
"""
%s
"""

JSON response:`

// Extract extracts entities from text using AI-based NER.
func (e *NERExtractor) Extract(ctx context.Context, text string, opts ExtractionOptions) ([]Entity, error) {
	if text == "" {
		return nil, nil
	}

	if e.client == nil {
		return nil, fmt.Errorf("AI client not configured")
	}

	// Split long text into chunks
	chunks := e.splitText(text)

	var allEntities []Entity
	for _, chunk := range chunks {
		entities, err := e.extractFromChunk(ctx, chunk, opts)
		if err != nil {
			// Log error but continue with other chunks
			continue
		}
		allEntities = append(allEntities, entities...)
	}

	// Deduplicate entities from multiple chunks
	if opts.DeduplicateEntities {
		allEntities = deduplicateEntities(allEntities)
	}

	// Filter by entity types
	if len(opts.EntityTypes) > 0 {
		var filtered []Entity
		for _, e := range allEntities {
			if opts.ShouldExtractType(e.Type) {
				filtered = append(filtered, e)
			}
		}
		allEntities = filtered
	}

	// Filter by confidence
	if opts.MinConfidence > 0 {
		var filtered []Entity
		for _, e := range allEntities {
			if e.Confidence >= opts.MinConfidence {
				filtered = append(filtered, e)
			}
		}
		allEntities = filtered
	}

	// Limit number of entities
	if opts.MaxEntities > 0 && len(allEntities) > opts.MaxEntities {
		allEntities = allEntities[:opts.MaxEntities]
	}

	return allEntities, nil
}

// extractFromChunk extracts entities from a single text chunk.
func (e *NERExtractor) extractFromChunk(ctx context.Context, text string, opts ExtractionOptions) ([]Entity, error) {
	prompt := fmt.Sprintf(NERPromptTemplate, text)

	systemPrompt := e.systemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a named entity recognition system. Extract entities accurately and return only valid JSON."
	}

	resp, err := e.client.Complete(ctx, prompt, &CompletionOptions{
		Model:        e.model,
		Temperature:  e.temperature,
		MaxTokens:    e.maxTokens,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return nil, fmt.Errorf("AI completion failed: %w", err)
	}

	// Parse the JSON response
	entities, err := e.parseNERResponse(resp.Text, text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse NER response: %w", err)
	}

	return entities, nil
}

// nerResponseEntity represents an entity in the NER JSON response.
type nerResponseEntity struct {
	Type       string  `json:"type"`
	Value      string  `json:"value"`
	Confidence float32 `json:"confidence"`
	Start      int     `json:"start,omitempty"`
	End        int     `json:"end,omitempty"`
	Context    string  `json:"context,omitempty"`
}

// parseNERResponse parses the AI response into entities.
func (e *NERExtractor) parseNERResponse(response, originalText string) ([]Entity, error) {
	// Clean up the response - extract JSON if wrapped in markdown code blocks
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
	}
	if strings.HasSuffix(response, "```") {
		response = strings.TrimSuffix(response, "```")
	}
	response = strings.TrimSpace(response)

	// Parse JSON array
	var nerEntities []nerResponseEntity
	if err := json.Unmarshal([]byte(response), &nerEntities); err != nil {
		// Try to find JSON array in the response
		startIdx := strings.Index(response, "[")
		endIdx := strings.LastIndex(response, "]")
		if startIdx >= 0 && endIdx > startIdx {
			jsonStr := response[startIdx : endIdx+1]
			if err := json.Unmarshal([]byte(jsonStr), &nerEntities); err != nil {
				return nil, fmt.Errorf("invalid JSON response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("no valid JSON array found in response")
		}
	}

	// Convert to Entity structs
	entities := make([]Entity, 0, len(nerEntities))
	for _, ne := range nerEntities {
		entityType := mapNERTypeToEntityType(ne.Type)

		entity := Entity{
			ID:         uuid.New().String(),
			Type:       entityType,
			Value:      ne.Value,
			Confidence: ne.Confidence,
			Source:     ExtractionSourceNER,
			Metadata:   make(map[string]string),
			CreatedAt:  time.Now(),
		}

		// Set position if provided
		if ne.Start > 0 || ne.End > 0 {
			entity.Position = TextPosition{
				Start: ne.Start,
				End:   ne.End,
			}
		} else {
			// Try to find position in original text
			idx := strings.Index(originalText, ne.Value)
			if idx >= 0 {
				entity.Position = TextPosition{
					Start: idx,
					End:   idx + len(ne.Value),
				}
			}
		}

		if ne.Context != "" {
			entity.Metadata["context"] = ne.Context
		}

		entities = append(entities, entity)
	}

	return entities, nil
}

// mapNERTypeToEntityType maps NER response types to EntityType.
func mapNERTypeToEntityType(nerType string) EntityType {
	switch strings.ToLower(nerType) {
	case "person", "per", "people":
		return EntityTypePerson
	case "organization", "org", "company":
		return EntityTypeOrganization
	case "location", "loc", "place", "gpe":
		return EntityTypeLocation
	case "date", "time", "datetime":
		return EntityTypeDate
	case "money", "currency", "monetary":
		return EntityTypeMoney
	case "event", "meeting":
		return EntityTypeEvent
	case "product", "service":
		return EntityTypeProduct
	case "misc", "miscellaneous", "other":
		return EntityTypeMisc
	default:
		return EntityTypeUnknown
	}
}

// splitText splits long text into manageable chunks.
func (e *NERExtractor) splitText(text string) []string {
	if len(text) <= e.maxTextLength {
		return []string{text}
	}

	var chunks []string
	start := 0

	for start < len(text) {
		end := start + e.maxTextLength

		// Try to break at a sentence boundary
		if end < len(text) {
			// Look for sentence-ending punctuation
			for i := end; i > start+e.maxTextLength/2; i-- {
				if text[i] == '.' || text[i] == '!' || text[i] == '?' {
					end = i + 1
					break
				}
			}
			// If no sentence boundary found, try whitespace
			if end == start+e.maxTextLength {
				for i := end; i > start+e.maxTextLength/2; i-- {
					if text[i] == ' ' || text[i] == '\n' {
						end = i
						break
					}
				}
			}
		}

		if end > len(text) {
			end = len(text)
		}

		chunks = append(chunks, text[start:end])

		// Next chunk starts with overlap
		start = end - e.chunkOverlap
		if start < 0 {
			start = 0
		}
		// Prevent infinite loop - if we haven't moved forward, force it
		if start < end-e.chunkOverlap {
			start = end
		}
	}

	return chunks
}

// deduplicateEntities removes duplicate entities based on value and type.
func deduplicateEntities(entities []Entity) []Entity {
	seen := make(map[string]Entity)

	for _, e := range entities {
		key := strings.ToLower(string(e.Type) + ":" + e.Value)

		if existing, exists := seen[key]; exists {
			// Keep the one with higher confidence
			if e.Confidence > existing.Confidence {
				seen[key] = e
			}
		} else {
			seen[key] = e
		}
	}

	result := make([]Entity, 0, len(seen))
	for _, e := range seen {
		result = append(result, e)
	}

	return result
}

// ===== Coreference Resolution Hints =====

// CoreferenceHint provides a hint for coreference resolution.
type CoreferenceHint struct {
	// Mention is the text mention (e.g., "he", "the company").
	Mention string

	// ReferentID is the ID of the entity it refers to.
	ReferentID string

	// Confidence is the confidence of the coreference.
	Confidence float32
}

// CoreferenceResolver attempts to resolve pronouns and references to entities.
type CoreferenceResolver struct {
	// pronounMap maps common pronouns to their likely entity types.
	pronounMap map[string][]EntityType
}

// NewCoreferenceResolver creates a new coreference resolver.
func NewCoreferenceResolver() *CoreferenceResolver {
	return &CoreferenceResolver{
		pronounMap: map[string][]EntityType{
			"he":    {EntityTypePerson},
			"him":   {EntityTypePerson},
			"his":   {EntityTypePerson},
			"she":   {EntityTypePerson},
			"her":   {EntityTypePerson},
			"they":  {EntityTypePerson, EntityTypeOrganization},
			"them":  {EntityTypePerson, EntityTypeOrganization},
			"their": {EntityTypePerson, EntityTypeOrganization},
			"it":    {EntityTypeOrganization, EntityTypeProduct},
		},
	}
}

// ResolveHints generates coreference hints for entities in text.
func (r *CoreferenceResolver) ResolveHints(entities []Entity, text string) []CoreferenceHint {
	// This is a simplified implementation that provides basic hints
	// A full implementation would use more sophisticated NLP

	var hints []CoreferenceHint

	// Find pronouns in the text
	words := strings.Fields(strings.ToLower(text))

	for i, word := range words {
		// Clean punctuation
		word = strings.Trim(word, ".,;:!?\"'")

		if expectedTypes, isPronoun := r.pronounMap[word]; isPronoun {
			// Find the most recent matching entity before this position
			approxPos := i * 6 // Rough estimate of character position

			var bestMatch *Entity
			var bestDistance int = -1

			for idx := range entities {
				e := &entities[idx]
				// Check if entity type matches expected types
				typeMatches := false
				for _, t := range expectedTypes {
					if e.Type == t {
						typeMatches = true
						break
					}
				}

				if !typeMatches {
					continue
				}

				// Prefer entities that appear before the pronoun
				if e.Position.End <= approxPos {
					distance := approxPos - e.Position.End
					if bestMatch == nil || distance < bestDistance {
						bestMatch = e
						bestDistance = distance
					}
				}
			}

			if bestMatch != nil {
				hints = append(hints, CoreferenceHint{
					Mention:    word,
					ReferentID: bestMatch.ID,
					Confidence: 0.6, // Low confidence for simple heuristic
				})
			}
		}
	}

	return hints
}

// ===== Entity Disambiguation =====

// DisambiguationContext provides context for entity disambiguation.
type DisambiguationContext struct {
	// TenantID for knowledge base lookups.
	TenantID string

	// KnownEntities are entities already known to the system.
	KnownEntities []Entity

	// DocumentDomain is the domain of the document (e.g., "finance", "tech").
	DocumentDomain string

	// DocumentDate is the date of the document for temporal disambiguation.
	DocumentDate time.Time
}

// DisambiguateEntities attempts to disambiguate entities based on context.
// This is useful when the same name could refer to multiple people/things.
func DisambiguateEntities(entities []Entity, context DisambiguationContext) []Entity {
	// For now, this is a simple implementation that:
	// 1. Adds domain hints to metadata
	// 2. Links to known entities where possible

	result := make([]Entity, len(entities))
	copy(result, entities)

	for i := range result {
		if context.DocumentDomain != "" {
			result[i].Metadata["document_domain"] = context.DocumentDomain
		}

		// Try to match with known entities
		for _, known := range context.KnownEntities {
			if result[i].Type == known.Type &&
				strings.EqualFold(result[i].Value, known.Value) {
				result[i].LinkedEntityID = known.ID
				result[i].Aliases = known.Aliases
				break
			}
		}
	}

	return result
}
