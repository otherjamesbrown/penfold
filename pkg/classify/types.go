// Package classify provides classification suggestion types and repository.
package classify

import "time"

// ClassificationSuggestion represents an LLM-generated source system classification suggestion.
type ClassificationSuggestion struct {
	ID           int
	ContentID    string
	SourceSystem string
	Pattern      string
	Confidence   float64
	Status       string
	CreatedAt    time.Time
}
