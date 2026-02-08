package resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/mentions"
)

// TestExecuteStage3_EntityIDAsNameString reproduces bug pf-90b5ea:
// When the LLM returns entity_id as a name string (e.g., "John") instead of an integer ID,
// FlexInt64 parsing fails with "cannot parse "John" as int64".
//
// Root cause: AIProvider routes through GenerateSummary RPC which does NOT use JSON mode,
// so Gemini Flash is not schema-constrained and returns entity_id: "John" instead of entity_id: 123.
//
// TEST STATUS: This test PASSES (correctly detects the bug by expecting an error).
// After the bug is fixed, this test should be updated to verify graceful handling
// (e.g., mapping name back to ID from candidate list).
func TestExecuteStage3_EntityIDAsNameString(t *testing.T) {
	// Setup: Create mock AI client that returns entity_id as a name string
	mockClient := new(MockAIClient)
	cfg := LLMConfig{
		Model:      "gemini-2.0-flash",
		MaxRetries: 0,
	}
	provider := NewAIProvider(mockClient, cfg)

	// Mock response: entity_id is "John Smith" instead of 123
	// This is what Gemini Flash returns when not using JSON mode
	invalidJSON := `{
		"resolutions": [
			{
				"mention_text": "John",
				"mention_position": 0,
				"decision": "resolve",
				"resolved_to": {
					"entity_type": "person",
					"entity_id": "John Smith",
					"entity_name": "John Smith"
				},
				"confidence": 0.95,
				"reasoning": "Direct name match"
			}
		]
	}`

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).
		Return(&aiv1.SummaryResponse{
			Summary:   invalidJSON,
			ModelUsed: "gemini-2.0-flash",
		}, nil)

	// Create executor with the AI provider
	executor := NewStageExecutor(provider, DefaultConfig())

	// Prepare test data
	understanding := &Stage1Understanding{
		Mentions: []MentionUnderstanding{
			{
				Text:       "John",
				EntityType: mentions.EntityTypePerson,
				Position:   0,
			},
		},
	}

	relationships := &Stage2CrossMention{
		ContentID:            123,
		UnifiedUnderstanding: "Discussion about John",
		MentionRelationships: []MentionRelationship{},
		ResolutionHints:      []string{},
	}

	candidates := map[string]*CandidateSet{
		"John": {
			MentionText: "John",
			Candidates: []CandidateWithHints{
				{
					EntityID:        123,
					EntityName:      "John Smith",
					EntityType:      mentions.EntityTypePerson,
					ConfidenceHints: map[string]interface{}{"match_type": "exact name match"},
				},
			},
		},
	}

	// Execute Stage 3
	result, err := executor.ExecuteStage3(context.Background(), understanding, relationships, candidates, "trace_test")

	// VERIFICATION: After fix, name strings should be mapped to IDs
	require.NoError(t, err, "Should handle entity_id as name string gracefully")
	require.NotNil(t, result, "Result should be valid")
	require.Len(t, result.Resolutions, 1)

	resolution := result.Resolutions[0]
	require.Equal(t, "John", resolution.MentionText)
	require.Equal(t, DecisionTypeResolve, resolution.Decision)
	require.NotNil(t, resolution.ResolvedTo)
	require.Equal(t, int64(123), resolution.ResolvedTo.EntityID.Int64(), "Name 'John Smith' should be mapped to ID 123")
	require.Equal(t, "John Smith", resolution.ResolvedTo.EntityName)

	t.Logf("SUCCESS: Name string 'John Smith' was mapped to entity ID 123")

	mockClient.AssertExpectations(t)
}

// TestExecuteStage3_EntityIDAsInteger verifies that valid integer entity_id works correctly.
// This test should PASS even with the current (unfixed) code.
func TestExecuteStage3_EntityIDAsInteger(t *testing.T) {
	// Setup: Create mock AI client that returns entity_id as a proper integer
	mockClient := new(MockAIClient)
	cfg := LLMConfig{
		Model:      "gemini-2.0-flash",
		MaxRetries: 0,
	}
	provider := NewAIProvider(mockClient, cfg)

	// Mock response: entity_id is 123 (correct format)
	validJSON := `{
		"resolutions": [
			{
				"mention_text": "John",
				"mention_position": 0,
				"decision": "resolve",
				"resolved_to": {
					"entity_type": "person",
					"entity_id": 123,
					"entity_name": "John Smith"
				},
				"confidence": 0.95,
				"reasoning": "Direct name match"
			}
		]
	}`

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).
		Return(&aiv1.SummaryResponse{
			Summary:   validJSON,
			ModelUsed: "gemini-2.0-flash",
		}, nil)

	// Create executor with the AI provider
	executor := NewStageExecutor(provider, DefaultConfig())

	// Prepare test data
	understanding := &Stage1Understanding{
		Mentions: []MentionUnderstanding{
			{
				Text:       "John",
				EntityType: mentions.EntityTypePerson,
				Position:   0,
			},
		},
	}

	relationships := &Stage2CrossMention{
		ContentID:            123,
		UnifiedUnderstanding: "Discussion about John",
		MentionRelationships: []MentionRelationship{},
		ResolutionHints:      []string{},
	}

	candidates := map[string]*CandidateSet{
		"John": {
			MentionText: "John",
			Candidates: []CandidateWithHints{
				{
					EntityID:        123,
					EntityName:      "John Smith",
					EntityType:      mentions.EntityTypePerson,
					ConfidenceHints: map[string]interface{}{"match_type": "exact name match"},
				},
			},
		},
	}

	// Execute Stage 3
	result, err := executor.ExecuteStage3(context.Background(), understanding, relationships, candidates, "trace_test")

	// EXPECTED: Test should PASS with proper integer entity_id
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Resolutions, 1)

	resolution := result.Resolutions[0]
	require.Equal(t, "John", resolution.MentionText)
	require.Equal(t, DecisionTypeResolve, resolution.Decision)
	require.NotNil(t, resolution.ResolvedTo)
	require.Equal(t, int64(123), resolution.ResolvedTo.EntityID.Int64())
	require.Equal(t, "John Smith", resolution.ResolvedTo.EntityName)

	mockClient.AssertExpectations(t)
}

// TestExecuteStage3_EntityIDAsNumericString verifies that numeric strings work (FlexInt64 feature).
// This test should PASS with the current code because FlexInt64 handles numeric strings.
func TestExecuteStage3_EntityIDAsNumericString(t *testing.T) {
	// Setup: Create mock AI client that returns entity_id as a numeric string
	mockClient := new(MockAIClient)
	cfg := LLMConfig{
		Model:      "gemini-2.0-flash",
		MaxRetries: 0,
	}
	provider := NewAIProvider(mockClient, cfg)

	// Mock response: entity_id is "123" (quoted number - FlexInt64 should handle this)
	validJSON := `{
		"resolutions": [
			{
				"mention_text": "John",
				"mention_position": 0,
				"decision": "resolve",
				"resolved_to": {
					"entity_type": "person",
					"entity_id": "123",
					"entity_name": "John Smith"
				},
				"confidence": 0.95,
				"reasoning": "Direct name match"
			}
		]
	}`

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).
		Return(&aiv1.SummaryResponse{
			Summary:   validJSON,
			ModelUsed: "gemini-2.0-flash",
		}, nil)

	// Create executor with the AI provider
	executor := NewStageExecutor(provider, DefaultConfig())

	// Prepare test data
	understanding := &Stage1Understanding{
		Mentions: []MentionUnderstanding{
			{
				Text:       "John",
				EntityType: mentions.EntityTypePerson,
				Position:   0,
			},
		},
	}

	relationships := &Stage2CrossMention{
		ContentID:            123,
		UnifiedUnderstanding: "Discussion about John",
		MentionRelationships: []MentionRelationship{},
		ResolutionHints:      []string{},
	}

	candidates := map[string]*CandidateSet{
		"John": {
			MentionText: "John",
			Candidates: []CandidateWithHints{
				{
					EntityID:        123,
					EntityName:      "John Smith",
					EntityType:      mentions.EntityTypePerson,
					ConfidenceHints: map[string]interface{}{"match_type": "exact name match"},
				},
			},
		},
	}

	// Execute Stage 3
	result, err := executor.ExecuteStage3(context.Background(), understanding, relationships, candidates, "trace_test")

	// EXPECTED: Test should PASS because FlexInt64 handles numeric strings
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Resolutions, 1)

	resolution := result.Resolutions[0]
	require.Equal(t, int64(123), resolution.ResolvedTo.EntityID.Int64())

	mockClient.AssertExpectations(t)
}

// TestExecuteStage3_EntityIDAsUnmatchedNameString verifies graceful handling when entity_id
// is a name string that doesn't match any candidate in the list.
func TestExecuteStage3_EntityIDAsUnmatchedNameString(t *testing.T) {
	// Setup: Create mock AI client that returns entity_id as an unmatched name string
	mockClient := new(MockAIClient)
	cfg := LLMConfig{
		Model:      "gemini-2.0-flash",
		MaxRetries: 0,
	}
	provider := NewAIProvider(mockClient, cfg)

	// Mock response: entity_id is "Jane Doe" which is NOT in the candidate list
	unmatchedJSON := `{
		"resolutions": [
			{
				"mention_text": "Jane",
				"mention_position": 0,
				"decision": "resolve",
				"resolved_to": {
					"entity_type": "person",
					"entity_id": "Jane Doe",
					"entity_name": "Jane Doe"
				},
				"confidence": 0.95,
				"reasoning": "Direct name match"
			}
		]
	}`

	mockClient.On("GenerateSummary", mock.Anything, mock.Anything).
		Return(&aiv1.SummaryResponse{
			Summary:   unmatchedJSON,
			ModelUsed: "gemini-2.0-flash",
		}, nil)

	// Create executor with the AI provider
	executor := NewStageExecutor(provider, DefaultConfig())

	// Prepare test data - only has "John Smith" as candidate, not "Jane Doe"
	understanding := &Stage1Understanding{
		Mentions: []MentionUnderstanding{
			{
				Text:       "Jane",
				EntityType: mentions.EntityTypePerson,
				Position:   0,
			},
		},
	}

	relationships := &Stage2CrossMention{
		ContentID:            123,
		UnifiedUnderstanding: "Discussion about Jane",
		MentionRelationships: []MentionRelationship{},
		ResolutionHints:      []string{},
	}

	candidates := map[string]*CandidateSet{
		"Jane": {
			MentionText: "Jane",
			Candidates: []CandidateWithHints{
				{
					EntityID:        123,
					EntityName:      "John Smith", // Different name
					EntityType:      mentions.EntityTypePerson,
					ConfidenceHints: map[string]interface{}{"match_type": "partial"},
				},
			},
		},
	}

	// Execute Stage 3
	result, err := executor.ExecuteStage3(context.Background(), understanding, relationships, candidates, "trace_test")

	// VERIFICATION: Should fail gracefully since "Jane Doe" is not in candidate list
	// The fallback mapping can't fix this, so it should return the original FlexInt64 error
	require.Error(t, err, "Should error when name string doesn't match any candidate")
	require.Nil(t, result)
	require.Contains(t, err.Error(), "FlexInt64", "Error should mention FlexInt64 parsing issue")

	t.Logf("EXPECTED: Got FlexInt64 error for unmatched name: %v", err)

	mockClient.AssertExpectations(t)
}

