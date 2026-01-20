package reviewqueue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQuestionType_Values(t *testing.T) {
	// Verify question type constants are correct
	assert.Equal(t, QuestionType("acronym"), QuestionTypeAcronym)
	assert.Equal(t, QuestionType("person"), QuestionTypePerson)
	assert.Equal(t, QuestionType("entity"), QuestionTypeEntity)
	assert.Equal(t, QuestionType("duplicate"), QuestionTypeDuplicate)
	assert.Equal(t, QuestionType("other"), QuestionTypeOther)
}

func TestPriority_Values(t *testing.T) {
	// Verify priority constants are correct
	assert.Equal(t, Priority("high"), PriorityHigh)
	assert.Equal(t, Priority("medium"), PriorityMedium)
	assert.Equal(t, Priority("low"), PriorityLow)
}

func TestStatus_Values(t *testing.T) {
	// Verify status constants are correct
	assert.Equal(t, Status("pending"), StatusPending)
	assert.Equal(t, Status("resolved"), StatusResolved)
	assert.Equal(t, Status("dismissed"), StatusDismissed)
	assert.Equal(t, Status("deferred"), StatusDeferred)
}

func TestAcronymQuestion_ToInput(t *testing.T) {
	tests := []struct {
		name             string
		question         AcronymQuestion
		expectedPriority Priority
		expectedQuestion string
	}{
		{
			name: "high confidence -> high priority",
			question: AcronymQuestion{
				Term:            "TER",
				Context:         "Attended TER meeting today",
				SourceType:      "meeting",
				SourceID:        123,
				SourceReference: "TER meeting 2025-01-15",
				Confidence:      0.9,
			},
			expectedPriority: PriorityHigh,
			expectedQuestion: "What does \"TER\" mean?",
		},
		{
			name: "medium confidence -> medium priority",
			question: AcronymQuestion{
				Term:       "DBaaS",
				Context:    "DBaaS team update",
				Confidence: 0.5,
			},
			expectedPriority: PriorityMedium,
			expectedQuestion: "What does \"DBaaS\" mean?",
		},
		{
			name: "low confidence -> low priority",
			question: AcronymQuestion{
				Term:       "FAQ",
				Context:    "See FAQ",
				Confidence: 0.2,
			},
			expectedPriority: PriorityLow,
			expectedQuestion: "What does \"FAQ\" mean?",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.question.ToInput()

			assert.Equal(t, QuestionTypeAcronym, input.QuestionType)
			assert.Equal(t, tc.expectedPriority, input.Priority)
			assert.Equal(t, tc.expectedQuestion, input.Question)
			assert.Equal(t, tc.question.Context, input.Context)
			assert.Equal(t, tc.question.SourceType, input.SourceType)
			assert.Equal(t, tc.question.SourceID, input.SourceID)
			assert.Equal(t, tc.question.SourceReference, input.SourceReference)
			assert.Equal(t, tc.question.Term, input.SuggestedTerm)
			assert.Equal(t, tc.question.Confidence, input.Confidence)
		})
	}
}

func TestPersonQuestion_ToInput(t *testing.T) {
	tests := []struct {
		name     string
		question PersonQuestion
		contains string
	}{
		{
			name: "with candidates",
			question: PersonQuestion{
				MatchedText:     "John",
				Context:         "John mentioned the project",
				CandidateIDs:    []int64{1, 2, 3},
				CandidateNames:  []string{"John Smith", "John Doe", "John Williams"},
				SourceType:      "email",
				SourceID:        456,
				SourceReference: "Email from 2025-01-15",
				Confidence:      0.7,
			},
			contains: "John Smith",
		},
		{
			name: "without candidates",
			question: PersonQuestion{
				MatchedText: "Alice",
				Context:     "Alice joined the call",
				Confidence:  0.8,
			},
			contains: "Alice",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.question.ToInput()

			assert.Equal(t, QuestionTypePerson, input.QuestionType)
			assert.Equal(t, PriorityHigh, input.Priority) // Person questions are always high priority
			assert.Contains(t, input.Question, tc.question.MatchedText)
			if tc.contains != "" {
				assert.Contains(t, input.Question, tc.contains)
			}
			assert.Equal(t, tc.question.Context, input.Context)
			assert.Equal(t, tc.question.MatchedText, input.MatchedText)
			assert.Equal(t, tc.question.CandidateIDs, input.CandidatePersonIDs)
		})
	}
}

func TestReviewItemInput_Defaults(t *testing.T) {
	input := ReviewItemInput{
		QuestionType: QuestionTypeAcronym,
		Priority:     PriorityMedium,
		Question:     "What does TEST mean?",
	}

	assert.Equal(t, QuestionTypeAcronym, input.QuestionType)
	assert.Equal(t, PriorityMedium, input.Priority)
	assert.Equal(t, "What does TEST mean?", input.Question)
	assert.Empty(t, input.Context)
	assert.Empty(t, input.SourceType)
	assert.Equal(t, int64(0), input.SourceID)
	assert.Empty(t, input.SuggestedTerm)
	assert.Nil(t, input.CandidatePersonIDs)
	assert.Equal(t, float64(0), input.Confidence)
	assert.Nil(t, input.Metadata)
}

func TestReviewFilter_Defaults(t *testing.T) {
	filter := ReviewFilter{}

	// Empty filter should have zero values
	assert.Empty(t, filter.Status)
	assert.Empty(t, filter.QuestionType)
	assert.Empty(t, filter.Priority)
	assert.Empty(t, filter.SourceType)
	assert.Empty(t, filter.TenantID)
	assert.Equal(t, 0, filter.Limit)
	assert.Equal(t, 0, filter.Offset)
}

func TestQueueStats_Initialization(t *testing.T) {
	stats := QueueStats{
		TotalPending:  10,
		ByPriority:    map[string]int{"high": 3, "medium": 5, "low": 2},
		ByType:        map[string]int{"acronym": 7, "person": 3},
		ResolvedToday: 5,
	}

	assert.Equal(t, 10, stats.TotalPending)
	assert.Equal(t, 3, stats.ByPriority["high"])
	assert.Equal(t, 5, stats.ByPriority["medium"])
	assert.Equal(t, 2, stats.ByPriority["low"])
	assert.Equal(t, 7, stats.ByType["acronym"])
	assert.Equal(t, 3, stats.ByType["person"])
	assert.Equal(t, 5, stats.ResolvedToday)
	assert.Nil(t, stats.OldestPending)
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		name     string
		strs     []string
		sep      string
		expected string
	}{
		{
			name:     "empty slice",
			strs:     []string{},
			sep:      ", ",
			expected: "",
		},
		{
			name:     "single element",
			strs:     []string{"one"},
			sep:      ", ",
			expected: "one",
		},
		{
			name:     "multiple elements",
			strs:     []string{"one", "two", "three"},
			sep:      ", ",
			expected: "one, two, three",
		},
		{
			name:     "different separator",
			strs:     []string{"a", "b", "c"},
			sep:      " | ",
			expected: "a | b | c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := joinStrings(tc.strs, tc.sep)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestReviewItem_Fields(t *testing.T) {
	item := ReviewItem{
		ID:                 1,
		TenantID:           "tenant-123",
		QuestionType:       QuestionTypeAcronym,
		Priority:           PriorityHigh,
		Question:           "What does TER mean?",
		Context:            "Context here",
		SourceType:         "meeting",
		SourceID:           100,
		SourceReference:    "TER meeting",
		SuggestedTerm:      "TER",
		SuggestedExpansion: "Technical Execution Review",
		CandidatePersonIDs: []int64{1, 2},
		MatchedText:        "text",
		Status:             StatusPending,
		Resolution:         "",
		Confidence:         0.85,
		Metadata:           map[string]interface{}{"key": "value"},
	}

	assert.Equal(t, int64(1), item.ID)
	assert.Equal(t, "tenant-123", item.TenantID)
	assert.Equal(t, QuestionTypeAcronym, item.QuestionType)
	assert.Equal(t, PriorityHigh, item.Priority)
	assert.Equal(t, "What does TER mean?", item.Question)
	assert.Equal(t, "Context here", item.Context)
	assert.Equal(t, "meeting", item.SourceType)
	assert.Equal(t, int64(100), item.SourceID)
	assert.Equal(t, "TER meeting", item.SourceReference)
	assert.Equal(t, "TER", item.SuggestedTerm)
	assert.Equal(t, "Technical Execution Review", item.SuggestedExpansion)
	assert.Equal(t, []int64{1, 2}, item.CandidatePersonIDs)
	assert.Equal(t, "text", item.MatchedText)
	assert.Equal(t, StatusPending, item.Status)
	assert.Empty(t, item.Resolution)
	assert.Nil(t, item.ResolvedAt)
	assert.Empty(t, item.ResolvedBy)
	assert.Equal(t, float64(0.85), item.Confidence)
	assert.Equal(t, "value", item.Metadata["key"])
}
