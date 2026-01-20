// Package reviewqueue provides a queue for AI questions requiring human review.
package reviewqueue

import "time"

// QuestionType defines the type of review question.
type QuestionType string

const (
	QuestionTypeAcronym   QuestionType = "acronym"   // Unknown acronym needs definition
	QuestionTypePerson    QuestionType = "person"    // Ambiguous person reference
	QuestionTypeEntity    QuestionType = "entity"    // Entity needs confirmation
	QuestionTypeDuplicate QuestionType = "duplicate" // Potential duplicate
	QuestionTypeOther     QuestionType = "other"     // General question
)

// Priority defines the urgency of a review item.
type Priority string

const (
	PriorityHigh   Priority = "high"   // Blocks understanding or processing
	PriorityMedium Priority = "medium" // Would improve search/correlation
	PriorityLow    Priority = "low"    // Nice to have
)

// Status defines the state of a review item.
type Status string

const (
	StatusPending  Status = "pending"  // Awaiting review
	StatusResolved Status = "resolved" // User provided answer
	StatusDismissed Status = "dismissed" // User dismissed as not needed
	StatusDeferred Status = "deferred" // User deferred for later
)

// ReviewItem represents a single item in the review queue.
type ReviewItem struct {
	ID       int64  `json:"id"`
	TenantID string `json:"tenant_id"`

	// Question details
	QuestionType QuestionType `json:"question_type"`
	Priority     Priority     `json:"priority"`
	Question     string       `json:"question"`
	Context      string       `json:"context,omitempty"`

	// Source reference
	SourceType      string `json:"source_type,omitempty"`
	SourceID        int64  `json:"source_id,omitempty"`
	SourceReference string `json:"source_reference,omitempty"`

	// Acronym-specific fields
	SuggestedTerm      string `json:"suggested_term,omitempty"`
	SuggestedExpansion string `json:"suggested_expansion,omitempty"`

	// Person disambiguation fields
	CandidatePersonIDs []int64 `json:"candidate_person_ids,omitempty"`
	MatchedText        string  `json:"matched_text,omitempty"`

	// Resolution
	Status     Status     `json:"status"`
	Resolution string     `json:"resolution,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy string     `json:"resolved_by,omitempty"`

	// AI confidence
	Confidence float64 `json:"confidence"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Audit
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ReviewItemInput is used for creating a new review item.
type ReviewItemInput struct {
	QuestionType       QuestionType           `json:"question_type"`
	Priority           Priority               `json:"priority"`
	Question           string                 `json:"question"`
	Context            string                 `json:"context,omitempty"`
	SourceType         string                 `json:"source_type,omitempty"`
	SourceID           int64                  `json:"source_id,omitempty"`
	SourceReference    string                 `json:"source_reference,omitempty"`
	SuggestedTerm      string                 `json:"suggested_term,omitempty"`
	SuggestedExpansion string                 `json:"suggested_expansion,omitempty"`
	CandidatePersonIDs []int64                `json:"candidate_person_ids,omitempty"`
	MatchedText        string                 `json:"matched_text,omitempty"`
	Confidence         float64                `json:"confidence"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// ReviewFilter specifies criteria for listing review items.
type ReviewFilter struct {
	Status       Status       // Filter by status (empty = pending)
	QuestionType QuestionType // Filter by question type
	Priority     Priority     // Filter by priority
	SourceType   string       // Filter by source type
	TenantID     string       // Filter by tenant
	Limit        int          // Max results (0 = default 50)
	Offset       int          // Pagination offset
}

// QueueStats contains summary statistics for the review queue.
type QueueStats struct {
	TotalPending  int            `json:"total_pending"`
	ByPriority    map[string]int `json:"by_priority"`
	ByType        map[string]int `json:"by_type"`
	ResolvedToday int            `json:"resolved_today"`
	OldestPending *time.Time     `json:"oldest_pending,omitempty"`
}

// AcronymQuestion is a helper for creating acronym review items.
type AcronymQuestion struct {
	Term            string
	Context         string
	SourceType      string
	SourceID        int64
	SourceReference string
	Confidence      float64
}

// ToInput converts an AcronymQuestion to a ReviewItemInput.
func (a AcronymQuestion) ToInput() ReviewItemInput {
	priority := PriorityMedium
	if a.Confidence > 0.8 {
		priority = PriorityHigh
	} else if a.Confidence < 0.3 {
		priority = PriorityLow
	}

	return ReviewItemInput{
		QuestionType:    QuestionTypeAcronym,
		Priority:        priority,
		Question:        "What does \"" + a.Term + "\" mean?",
		Context:         a.Context,
		SourceType:      a.SourceType,
		SourceID:        a.SourceID,
		SourceReference: a.SourceReference,
		SuggestedTerm:   a.Term,
		Confidence:      a.Confidence,
	}
}

// PersonQuestion is a helper for creating person disambiguation review items.
type PersonQuestion struct {
	MatchedText     string
	Context         string
	CandidateIDs    []int64
	CandidateNames  []string
	SourceType      string
	SourceID        int64
	SourceReference string
	Confidence      float64
}

// ToInput converts a PersonQuestion to a ReviewItemInput.
func (p PersonQuestion) ToInput() ReviewItemInput {
	priority := PriorityHigh // Person disambiguation is usually important

	question := "Who is \"" + p.MatchedText + "\"?"
	if len(p.CandidateNames) > 0 {
		question += " Candidates: " + joinStrings(p.CandidateNames, ", ")
	}

	return ReviewItemInput{
		QuestionType:       QuestionTypePerson,
		Priority:           priority,
		Question:           question,
		Context:            p.Context,
		SourceType:         p.SourceType,
		SourceID:           p.SourceID,
		SourceReference:    p.SourceReference,
		CandidatePersonIDs: p.CandidateIDs,
		MatchedText:        p.MatchedText,
		Confidence:         p.Confidence,
	}
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}
