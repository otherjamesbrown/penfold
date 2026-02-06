// Package activities provides tests for analysis activities.
package activities

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestDeepAnalyze_Success(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create mock AI client that returns full deep analysis response
	mockClient := &mockAIClient{
		deepAnalyzeFn: func(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
			// Verify request fields
			require.NotEmpty(t, req.Content)
			require.Equal(t, "PROJECT_UPDATE", req.TriageCategory)
			require.Equal(t, "HIGH", req.TriageImportance)
			require.NotEmpty(t, req.BackgroundContext)

			// Verify extraction results were converted to proto
			require.Len(t, req.VerifiedPeople, 2)
			require.Equal(t, "Alice Smith", req.VerifiedPeople[0].Name)
			require.Equal(t, "Project Manager", req.VerifiedPeople[0].Role)

			require.Len(t, req.VerifiedDates, 1)
			require.Equal(t, "2024-03-15", req.VerifiedDates[0].Date)

			require.Len(t, req.VerifiedProjects, 1)
			require.Equal(t, "Project Alpha", req.VerifiedProjects[0])

			require.Len(t, req.PreliminaryActionItems, 1)
			require.Equal(t, "Review design doc", req.PreliminaryActionItems[0].Action)

			// Return full analysis response
			return &aiv1.DeepAnalyzeResponse{
				Summary: "Comprehensive project update discussing timeline and deliverables.",
				Sentiment: &aiv1.DeepSentiment{
					Score:       0.8,
					Label:       "positive",
					Confidence:  0.9,
					Indicators:  []string{"on track", "good progress"},
					Explanation: "Team is making good progress on project goals.",
				},
				TopicMappings: []*aiv1.TopicMapping{
					{
						Topic:          "Project Timeline",
						RelatedProject: "Project Alpha",
						Relationship:   "status update",
						Confidence:     0.95,
					},
				},
				VerifiedActionItems: []*aiv1.VerifiedActionItem{
					{
						Description:    "Complete design review by end of week",
						Assignee:       "Alice Smith",
						Due:            "2024-03-10",
						Priority:       "high",
						ContextExcerpt: "Alice will review the design doc by Friday",
						Status:         "confirmed",
					},
				},
				VerifiedDecisions: []*aiv1.VerifiedDecision{
					{
						Description:    "Approved budget increase for Q2",
						ContextExcerpt: "The team agreed to increase the budget by 10%",
						Status:         "confirmed",
					},
				},
				RiskReferences: []*aiv1.RiskReference{
					{
						Description:    "Resource constraint may impact timeline",
						Significance:   "secondary",
						ContextExcerpt: "We might face resource issues in April",
						IsNew:          true,
					},
				},
				StrategicInsights: []string{
					"Team velocity is improving",
					"Consider hiring additional resources for Q2",
				},
				ImplicitActionItems: []*aiv1.ImplicitActionItem{
					{
						Description:    "Schedule resource planning meeting",
						Reasoning:      "Resource constraint mentioned without explicit follow-up",
						ContextExcerpt: "We might face resource issues in April",
					},
				},
				ModelUsed: "gpt-4-turbo",
			}, nil
		},
	}

	activities := NewAnalysisActivities(logger, mockClient, nil)

	input := DeepAnalyzeInput{
		TenantID:         "test-tenant",
		SourceID:         123,
		ContentID:        "em-abc123",
		JobID:            "job-123",
		Content:          "Project update email discussing progress and timeline.",
		ContentType:      "email",
		TriageCategory:   "PROJECT_UPDATE",
		TriageImportance: "HIGH",
		ExtractionResult: &ExtractEntitiesOutput{
			People: []PersonResult{
				{Name: "Alice Smith", Role: "Project Manager"},
				{Name: "Bob Jones", Role: "Engineer"},
			},
			Dates: []DateResult{
				{Date: "2024-03-15", Context: "project deadline"},
			},
			Projects: []string{"Project Alpha"},
			ActionItems: []ActionItemResult{
				{Assignee: "Alice", Action: "Review design doc", Due: "2024-03-10"},
			},
			Decisions: []string{"Approved budget increase"},
			Risks:     []string{"Resource constraint"},
			ModelUsed: "test-extraction-model",
		},
		BackgroundContext: "Previous discussions about project timeline and budget.",
	}

	output, err := activities.DeepAnalyze(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify output fields
	require.Equal(t, "Comprehensive project update discussing timeline and deliverables.", output.Summary)
	require.Equal(t, "gpt-4-turbo", output.ModelUsed)

	// Verify sentiment
	require.NotNil(t, output.Sentiment)
	require.Equal(t, float32(0.8), output.Sentiment.Score)
	require.Equal(t, "positive", output.Sentiment.Label)
	require.Equal(t, float32(0.9), output.Sentiment.Confidence)
	require.Len(t, output.Sentiment.Indicators, 2)

	// Verify topic mappings
	require.Len(t, output.TopicMappings, 1)
	require.Equal(t, "Project Timeline", output.TopicMappings[0].Topic)
	require.Equal(t, "Project Alpha", output.TopicMappings[0].RelatedProject)

	// Verify verified actions
	require.Len(t, output.VerifiedActions, 1)
	require.Equal(t, "Complete design review by end of week", output.VerifiedActions[0].Description)
	require.Equal(t, "high", output.VerifiedActions[0].Priority)

	// Verify verified decisions
	require.Len(t, output.VerifiedDecisions, 1)
	require.Equal(t, "Approved budget increase for Q2", output.VerifiedDecisions[0].Description)

	// Verify risk references
	require.Len(t, output.RiskReferences, 1)
	require.True(t, output.RiskReferences[0].IsNew)
	require.Equal(t, "secondary", output.RiskReferences[0].Significance)

	// Verify insights
	require.Len(t, output.Insights, 2)

	// Verify implicit actions
	require.Len(t, output.ImplicitActions, 1)
	require.Equal(t, "Schedule resource planning meeting", output.ImplicitActions[0].Description)
}

func TestDeepAnalyze_EmptyContent(t *testing.T) {
	logger := logging.NewNopLogger()
	mockClient := &mockAIClient{}
	activities := NewAnalysisActivities(logger, mockClient, nil)

	input := DeepAnalyzeInput{
		TenantID:         "test-tenant",
		SourceID:         123,
		JobID:            "job-123",
		Content:          "", // Empty content
		ContentType:      "email",
		TriageCategory:   "PROJECT_UPDATE",
		TriageImportance: "HIGH",
	}

	output, err := activities.DeepAnalyze(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "content is empty")
}

func TestDeepAnalyze_AIClientError(t *testing.T) {
	logger := logging.NewNopLogger()

	expectedErr := errors.New("AI service unavailable")
	mockClient := &mockAIClient{
		deepAnalyzeFn: func(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
			return nil, expectedErr
		},
	}

	activities := NewAnalysisActivities(logger, mockClient, nil)

	input := DeepAnalyzeInput{
		TenantID:         "test-tenant",
		SourceID:         123,
		JobID:            "job-123",
		Content:          "Some content",
		ContentType:      "email",
		TriageCategory:   "PROJECT_UPDATE",
		TriageImportance: "HIGH",
	}

	output, err := activities.DeepAnalyze(context.Background(), input)
	require.Error(t, err)
	require.Nil(t, output)
	require.Contains(t, err.Error(), "failed to perform deep analysis")
	require.ErrorIs(t, err, expectedErr)
}

func TestDeepAnalyze_ProtoConversion(t *testing.T) {
	logger := logging.NewNopLogger()

	// Mock that verifies the proto conversion is correct
	mockClient := &mockAIClient{
		deepAnalyzeFn: func(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
			// Verify all extraction results were properly converted to proto types

			// Check people
			require.Len(t, req.VerifiedPeople, 3)
			require.Equal(t, "Alice", req.VerifiedPeople[0].Name)
			require.Equal(t, "Manager", req.VerifiedPeople[0].Role)
			require.Equal(t, "Bob", req.VerifiedPeople[1].Name)
			require.Equal(t, "Engineer", req.VerifiedPeople[1].Role)
			require.Equal(t, "Charlie", req.VerifiedPeople[2].Name)
			require.Equal(t, "", req.VerifiedPeople[2].Role)

			// Check dates
			require.Len(t, req.VerifiedDates, 2)
			require.Equal(t, "2024-03-15", req.VerifiedDates[0].Date)
			require.Equal(t, "deadline", req.VerifiedDates[0].Context)
			require.Equal(t, "2024-04-01", req.VerifiedDates[1].Date)
			require.Equal(t, "launch date", req.VerifiedDates[1].Context)

			// Check projects
			require.Len(t, req.VerifiedProjects, 2)
			require.Equal(t, "Project Alpha", req.VerifiedProjects[0])
			require.Equal(t, "Project Beta", req.VerifiedProjects[1])

			// Check organisations
			require.Len(t, req.VerifiedOrganisations, 1)
			require.Equal(t, "Acme Corp", req.VerifiedOrganisations[0])

			// Check preliminary action items
			require.Len(t, req.PreliminaryActionItems, 2)
			require.Equal(t, "Review doc", req.PreliminaryActionItems[0].Action)
			require.Equal(t, "Alice", req.PreliminaryActionItems[0].Assignee)
			require.Equal(t, "2024-03-10", req.PreliminaryActionItems[0].Due)

			// Check preliminary decisions
			require.Len(t, req.PreliminaryDecisions, 2)
			require.Equal(t, "Approved budget", req.PreliminaryDecisions[0])
			require.Equal(t, "Rejected proposal", req.PreliminaryDecisions[1])

			// Check preliminary risks
			require.Len(t, req.PreliminaryRisks, 1)
			require.Equal(t, "Resource constraint", req.PreliminaryRisks[0])

			// Check triage metadata
			require.Equal(t, "RISK_ISSUE", req.TriageCategory)
			require.Equal(t, "HIGH", req.TriageImportance)

			// Check background context
			require.Equal(t, "Previous context about the project", req.BackgroundContext)

			// Return minimal response
			return &aiv1.DeepAnalyzeResponse{
				Summary:   "Test summary",
				ModelUsed: "test-model",
			}, nil
		},
	}

	activities := NewAnalysisActivities(logger, mockClient, nil)

	input := DeepAnalyzeInput{
		TenantID:         "test-tenant",
		SourceID:         456,
		JobID:            "job-456",
		Content:          "Test content for proto conversion",
		ContentType:      "email",
		TriageCategory:   "RISK_ISSUE",
		TriageImportance: "HIGH",
		ExtractionResult: &ExtractEntitiesOutput{
			People: []PersonResult{
				{Name: "Alice", Role: "Manager"},
				{Name: "Bob", Role: "Engineer"},
				{Name: "Charlie", Role: ""}, // No role
			},
			Dates: []DateResult{
				{Date: "2024-03-15", Context: "deadline"},
				{Date: "2024-04-01", Context: "launch date"},
			},
			Projects:      []string{"Project Alpha", "Project Beta"},
			Organisations: []string{"Acme Corp"},
			ActionItems: []ActionItemResult{
				{Action: "Review doc", Assignee: "Alice", Due: "2024-03-10"},
				{Action: "Update status", Assignee: "Bob", Due: ""},
			},
			Decisions: []string{"Approved budget", "Rejected proposal"},
			Risks:     []string{"Resource constraint"},
			ModelUsed: "test-extraction-model",
		},
		BackgroundContext: "Previous context about the project",
	}

	output, err := activities.DeepAnalyze(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "Test summary", output.Summary)
	require.Equal(t, "test-model", output.ModelUsed)
}
