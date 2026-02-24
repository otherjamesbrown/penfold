// Package activities provides tests for extraction activities.
package activities

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
	"github.com/otherjamesbrown/penfold/services/worker/workflows"
)

// mockAIClient is a mock implementation of the AIClient interface for testing.
type mockAIClient struct {
	extractEntitiesFn    func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error)
	extractAssertionsFn  func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error)
	generateEmbeddingFn  func(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error)
	generateSummaryFn    func(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error)
	triageContentFn      func(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error)
	deepAnalyzeFn        func(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error)
}

func (m *mockAIClient) ExtractEntities(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
	if m.extractEntitiesFn != nil {
		return m.extractEntitiesFn(ctx, req)
	}
	return &aiv1.ExtractEntitiesResponse{}, nil
}

func (m *mockAIClient) ExtractAssertions(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
	if m.extractAssertionsFn != nil {
		return m.extractAssertionsFn(ctx, req)
	}
	return &aiv1.AssertionResponse{}, nil
}

func (m *mockAIClient) GenerateEmbedding(ctx context.Context, req *aiv1.EmbeddingRequest) (*aiv1.EmbeddingResponse, error) {
	if m.generateEmbeddingFn != nil {
		return m.generateEmbeddingFn(ctx, req)
	}
	return &aiv1.EmbeddingResponse{}, nil
}

func (m *mockAIClient) GenerateSummary(ctx context.Context, req *aiv1.SummaryRequest) (*aiv1.SummaryResponse, error) {
	if m.generateSummaryFn != nil {
		return m.generateSummaryFn(ctx, req)
	}
	return &aiv1.SummaryResponse{}, nil
}

func (m *mockAIClient) TriageContent(ctx context.Context, req *aiv1.TriageContentRequest) (*aiv1.TriageContentResponse, error) {
	if m.triageContentFn != nil {
		return m.triageContentFn(ctx, req)
	}
	return &aiv1.TriageContentResponse{}, nil
}

func (m *mockAIClient) DeepAnalyze(ctx context.Context, req *aiv1.DeepAnalyzeRequest) (*aiv1.DeepAnalyzeResponse, error) {
	if m.deepAnalyzeFn != nil {
		return m.deepAnalyzeFn(ctx, req)
	}
	return &aiv1.DeepAnalyzeResponse{}, nil
}

func TestExtractEntities_ShortContent(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			// Verify request
			require.NotEmpty(t, req.Content)
			require.Less(t, len([]rune(req.Content)), 3000)

			return &aiv1.ExtractEntitiesResponse{
				People: []*aiv1.PersonEntity{
					{Name: "Alice Smith", Role: "Project Manager"},
					{Name: "Bob Johnson", Role: "Engineer"},
				},
				Dates: []*aiv1.DateEntity{
					{Date: "2024-03-15", Context: "project deadline"},
				},
				Projects:      []string{"Project Alpha"},
				Organisations: []string{"Acme Corp"},
				ActionItems: []*aiv1.ActionItemEntity{
					{Assignee: "Alice", Action: "Review design doc", Due: "2024-03-10"},
				},
				Decisions:            []string{"Approved budget increase"},
				Risks:                []string{"Resource constraint"},
				DetailedRisks:        []*aiv1.RiskEntity{},
				QualityGateTriggered: false,
				ModelUsed:            "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID: "test-tenant",
		SourceID: 123,
		JobID:    "job-123",
		Content:  "Alice Smith, the Project Manager, discussed the project deadline of March 15th with Bob Johnson, the Engineer. They work on Project Alpha at Acme Corp. Alice will review the design doc by March 10th. They approved a budget increase but noted resource constraints.",
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 2, len(output.People))
	require.Equal(t, 1, len(output.Dates))
	require.Equal(t, 1, len(output.Projects))
	require.Equal(t, 1, len(output.Organisations))
	require.Equal(t, 1, len(output.ActionItems))
	require.Equal(t, 1, len(output.Decisions))
	require.Equal(t, 1, len(output.Risks))
	require.Equal(t, 0, len(output.DetailedRisks))
	require.False(t, output.QualityGateTriggered)
	require.Equal(t, "test-model", output.ModelUsed)
}

func TestExtractEntities_MediumContent(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create content between 3K-6K chars
	mediumContent := strings.Repeat("This is a test sentence about Project Beta and some people. ", 70)
	require.Greater(t, len([]rune(mediumContent)), 3000)
	require.Less(t, len([]rune(mediumContent)), 6000)

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			return &aiv1.ExtractEntitiesResponse{
				People:               []*aiv1.PersonEntity{{Name: "Test Person", Role: ""}},
				Dates:                []*aiv1.DateEntity{},
				Projects:             []string{"Project Beta"},
				Organisations:        []string{},
				ActionItems:          []*aiv1.ActionItemEntity{},
				Decisions:            []string{},
				Risks:                []string{},
				DetailedRisks:        []*aiv1.RiskEntity{},
				QualityGateTriggered: false,
				ModelUsed:            "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID: "test-tenant",
		SourceID: 123,
		JobID:    "job-123",
		Content:  mediumContent,
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 1, len(output.People))
	require.Equal(t, 1, len(output.Projects))
}

func TestExtractEntities_LongContent(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create content over 6K chars to trigger chunking
	longContent := strings.Repeat("This is a test sentence about Project Gamma and important people like Charlie and Dana. ", 200)
	require.Greater(t, len([]rune(longContent)), 6000)

	callCount := 0
	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			callCount++
			// Verify we're getting chunks, not the full content
			require.Less(t, len([]rune(req.Content)), len([]rune(longContent)))

			// Return different results per chunk to test merging
			if callCount == 1 {
				return &aiv1.ExtractEntitiesResponse{
					People:               []*aiv1.PersonEntity{{Name: "Charlie", Role: "Lead"}},
					Dates:                []*aiv1.DateEntity{},
					Projects:             []string{"Project Gamma"},
					Organisations:        []string{},
					ActionItems:          []*aiv1.ActionItemEntity{},
					Decisions:            []string{},
					Risks:                []string{},
					DetailedRisks:        []*aiv1.RiskEntity{},
					QualityGateTriggered: false,
					ModelUsed:            "test-model",
				}, nil
			}
			return &aiv1.ExtractEntitiesResponse{
				People:               []*aiv1.PersonEntity{{Name: "Dana", Role: "Manager"}},
				Dates:                []*aiv1.DateEntity{},
				Projects:             []string{"Project Gamma"}, // Duplicate
				Organisations:        []string{},
				ActionItems:          []*aiv1.ActionItemEntity{},
				Decisions:            []string{},
				Risks:                []string{},
				DetailedRisks:        []*aiv1.RiskEntity{},
				QualityGateTriggered: false,
				ModelUsed:            "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID: "test-tenant",
		SourceID: 123,
		JobID:    "job-123",
		Content:  longContent,
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify multiple chunks were processed
	require.Greater(t, callCount, 1, "Expected multiple RPC calls for long content")

	// Verify merged results
	require.Equal(t, 2, len(output.People), "Expected 2 people from merged chunks")
	require.Equal(t, 1, len(output.Projects), "Expected 1 project after deduplication")
}

func TestExtractEntities_EmptyContent(t *testing.T) {
	logger := logging.NewNopLogger()
	mockClient := &mockAIClient{}
	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID: "test-tenant",
		SourceID: 123,
		JobID:    "job-123",
		Content:  "",
	}

	// pf-479452: Empty content now returns empty extraction result (metadata-only)
	// This supports calendar invites with no body text
	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "metadata-only", output.ModelUsed)
	require.Equal(t, 0, len(output.People))
	require.Equal(t, 0, len(output.ActionItems))
}

func TestExtractEntities_MergeDedup(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create content over 6K chars to trigger chunking
	longContent := strings.Repeat("Test content for chunking and deduplication. ", 300)
	require.Greater(t, len([]rune(longContent)), 6000)

	callCount := 0
	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			callCount++
			// Return overlapping results to test deduplication
			return &aiv1.ExtractEntitiesResponse{
				People: []*aiv1.PersonEntity{
					{Name: "Alice Smith", Role: "Manager"},
					{Name: "alice smith", Role: "Project Manager"}, // Case variation
					{Name: "Bob Jones", Role: "Engineer"},
				},
				Dates: []*aiv1.DateEntity{
					{Date: "2024-03-15", Context: "deadline"},
					{Date: "2024-03-15", Context: "project end"}, // Duplicate date
				},
				Projects: []string{"Project X", "project x", "Project Y"}, // Case variations
				ActionItems: []*aiv1.ActionItemEntity{
					{Assignee: "Alice", Action: "review design", Due: "2024-03-10"},
					{Assignee: "Alice", Action: "Review Design", Due: "2024-03-10"}, // Case variation
				},
				Decisions:            []string{"Approved", "approved", "Rejected"}, // Case variations
				Risks:                []string{"Budget risk", "BUDGET RISK", "Schedule risk"},
				DetailedRisks:        []*aiv1.RiskEntity{},
				QualityGateTriggered: false,
				ModelUsed:            "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID: "test-tenant",
		SourceID: 123,
		JobID:    "job-123",
		Content:  longContent,
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify deduplication worked
	require.Equal(t, 2, len(output.People), "Expected 2 people after case-insensitive dedup (Alice + Bob)")
	require.Equal(t, 1, len(output.Dates), "Expected 1 date after dedup")
	require.Equal(t, 2, len(output.Projects), "Expected 2 projects after case-insensitive dedup")
	require.Equal(t, 1, len(output.ActionItems), "Expected 1 action item after dedup")
	require.Equal(t, 2, len(output.Decisions), "Expected 2 decisions after dedup")
	require.Equal(t, 2, len(output.Risks), "Expected 2 risks after dedup")
}

func TestExtractEntities_QualityGate(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			// Verify triage_category was passed
			require.NotNil(t, req.TriageCategory)
			require.Equal(t, "RISK_ISSUE", *req.TriageCategory)

			return &aiv1.ExtractEntitiesResponse{
				People:        []*aiv1.PersonEntity{},
				Dates:         []*aiv1.DateEntity{},
				Projects:      []string{},
				Organisations: []string{},
				ActionItems:   []*aiv1.ActionItemEntity{},
				Decisions:     []string{},
				Risks:         []string{}, // 0 risks triggers quality gate
				DetailedRisks: []*aiv1.RiskEntity{
					{Description: "Critical security vulnerability", SeverityHint: "HIGH", OwnerHint: "Security Team"},
				},
				QualityGateTriggered: true,
				ModelUsed:            "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID:       "test-tenant",
		SourceID:       123,
		JobID:          "job-123",
		Content:        "Email about a critical security issue that needs immediate attention.",
		TriageCategory: "RISK_ISSUE",
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.True(t, output.QualityGateTriggered)
	require.Equal(t, 1, len(output.DetailedRisks))
	require.Equal(t, "Critical security vulnerability", output.DetailedRisks[0].Description)
}

func TestExtractEntities_QualityGateNotTriggered(t *testing.T) {
	logger := logging.NewNopLogger()

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			return &aiv1.ExtractEntitiesResponse{
				People:               []*aiv1.PersonEntity{},
				Dates:                []*aiv1.DateEntity{},
				Projects:             []string{},
				Organisations:        []string{},
				ActionItems:          []*aiv1.ActionItemEntity{},
				Decisions:            []string{},
				Risks:                []string{"Budget concern", "Timeline risk"}, // Risks found, no gate
				DetailedRisks:        []*aiv1.RiskEntity{},
				QualityGateTriggered: false,
				ModelUsed:            "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID:       "test-tenant",
		SourceID:       123,
		JobID:          "job-123",
		Content:        "Email about budget concerns and timeline risks.",
		TriageCategory: "RISK_ISSUE",
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.False(t, output.QualityGateTriggered)
	require.Equal(t, 0, len(output.DetailedRisks))
	require.Equal(t, 2, len(output.Risks))
}

func TestMergeExtractionResults_Empty(t *testing.T) {
	results := []*aiv1.ExtractEntitiesResponse{}
	output := mergeExtractionResults(results)

	require.NotNil(t, output)
	require.Equal(t, 0, len(output.People))
	require.Equal(t, 0, len(output.Dates))
	require.Equal(t, 0, len(output.Projects))
	require.Equal(t, 0, len(output.Organisations))
	require.Equal(t, 0, len(output.ActionItems))
	require.Equal(t, 0, len(output.Decisions))
	require.Equal(t, 0, len(output.Risks))
	require.Equal(t, 0, len(output.DetailedRisks))
	require.False(t, output.QualityGateTriggered)
	require.Empty(t, output.ModelUsed)
}

func TestMergeExtractionResults_Single(t *testing.T) {
	results := []*aiv1.ExtractEntitiesResponse{
		{
			People: []*aiv1.PersonEntity{
				{Name: "Alice", Role: "Manager"},
			},
			Dates: []*aiv1.DateEntity{
				{Date: "2024-03-15", Context: "deadline"},
			},
			Projects:             []string{"Project A"},
			Organisations:        []string{"Acme"},
			ActionItems:          []*aiv1.ActionItemEntity{{Action: "Review", Assignee: "Alice"}},
			Decisions:            []string{"Approved"},
			Risks:                []string{"Budget risk"},
			DetailedRisks:        []*aiv1.RiskEntity{},
			QualityGateTriggered: false,
			ModelUsed:            "test-model",
		},
	}
	output := mergeExtractionResults(results)

	require.NotNil(t, output)
	require.Equal(t, 1, len(output.People))
	require.Equal(t, 1, len(output.Dates))
	require.Equal(t, 1, len(output.Projects))
	require.Equal(t, 1, len(output.Organisations))
	require.Equal(t, 1, len(output.ActionItems))
	require.Equal(t, 1, len(output.Decisions))
	require.Equal(t, 1, len(output.Risks))
	require.Equal(t, "test-model", output.ModelUsed)
}

func TestMergeExtractionResults_Multiple(t *testing.T) {
	results := []*aiv1.ExtractEntitiesResponse{
		{
			People: []*aiv1.PersonEntity{
				{Name: "Alice", Role: "Manager"},
				{Name: "Bob", Role: "Engineer"},
			},
			Dates: []*aiv1.DateEntity{
				{Date: "2024-03-15", Context: "deadline"},
			},
			Projects:             []string{"Project A"},
			Organisations:        []string{"Acme"},
			ActionItems:          []*aiv1.ActionItemEntity{{Action: "Review", Assignee: "Alice"}},
			Decisions:            []string{"Approved"},
			Risks:                []string{"Budget risk"},
			DetailedRisks:        []*aiv1.RiskEntity{},
			QualityGateTriggered: false,
			ModelUsed:            "test-model",
		},
		{
			People: []*aiv1.PersonEntity{
				{Name: "alice", Role: "Project Manager"}, // Duplicate with different case
				{Name: "Charlie", Role: "Designer"},
			},
			Dates: []*aiv1.DateEntity{
				{Date: "2024-03-15", Context: "project end"}, // Duplicate date
				{Date: "2024-04-01", Context: "launch"},
			},
			Projects:             []string{"Project A", "Project B"}, // Duplicate + new
			Organisations:        []string{"Acme", "TechCorp"},       // Duplicate + new
			ActionItems:          []*aiv1.ActionItemEntity{{Action: "review", Assignee: "Bob"}}, // Case variation
			Decisions:            []string{"Rejected"},
			Risks:                []string{"Schedule risk"},
			DetailedRisks:        []*aiv1.RiskEntity{},
			QualityGateTriggered: false,
			ModelUsed:            "test-model",
		},
	}
	output := mergeExtractionResults(results)

	require.NotNil(t, output)
	require.Equal(t, 3, len(output.People), "Expected 3 unique people (Alice, Bob, Charlie)")
	require.Equal(t, 2, len(output.Dates), "Expected 2 unique dates")
	require.Equal(t, 2, len(output.Projects), "Expected 2 unique projects")
	require.Equal(t, 2, len(output.Organisations), "Expected 2 unique organisations")
	require.Equal(t, 2, len(output.ActionItems), "Expected 2 unique action items")
	require.Equal(t, 2, len(output.Decisions), "Expected 2 unique decisions")
	require.Equal(t, 2, len(output.Risks), "Expected 2 unique risks")
}

func TestMergeExtractionResults_QualityGate(t *testing.T) {
	results := []*aiv1.ExtractEntitiesResponse{
		{
			People:               []*aiv1.PersonEntity{},
			Dates:                []*aiv1.DateEntity{},
			Projects:             []string{},
			Organisations:        []string{},
			ActionItems:          []*aiv1.ActionItemEntity{},
			Decisions:            []string{},
			Risks:                []string{},
			DetailedRisks:        []*aiv1.RiskEntity{},
			QualityGateTriggered: false,
			ModelUsed:            "test-model",
		},
		{
			People:        []*aiv1.PersonEntity{},
			Dates:         []*aiv1.DateEntity{},
			Projects:      []string{},
			Organisations: []string{},
			ActionItems:   []*aiv1.ActionItemEntity{},
			Decisions:     []string{},
			Risks:         []string{},
			DetailedRisks: []*aiv1.RiskEntity{
				{Description: "Critical issue", SeverityHint: "HIGH"},
			},
			QualityGateTriggered: true, // One chunk triggered it
			ModelUsed:            "test-model",
		},
	}
	output := mergeExtractionResults(results)

	require.NotNil(t, output)
	require.True(t, output.QualityGateTriggered, "Expected quality gate to be triggered if any chunk triggered it")
	require.Equal(t, 1, len(output.DetailedRisks))
}

func TestNormalizeString(t *testing.T) {
	require.Equal(t, "hello world", normalizeString("Hello World"))
	require.Equal(t, "hello world", normalizeString("  Hello World  "))
	require.Equal(t, "hello world", normalizeString("HELLO WORLD"))
	require.Equal(t, "", normalizeString("   "))
}

func TestBuildEmailHeaderBlock_FullHeaders(t *testing.T) {
	input := ExtractEntitiesInput{
		ContentType: "email",
		SenderName:  "Ponec, Miroslav",
		SenderEmail: "mponec@akamai.com",
		Subject:     "Immediate CLIC Action Required on Juniper Router Issues Impacting MTC Revenue",
		Participants: []workflows.Participant{
			{Email: "tdunn@akamai.com", DisplayName: "Dunn, Tim", HeaderRole: "to"},
			{Email: "jdement@akamai.com", DisplayName: "DeMent, James", HeaderRole: "to"},
			{Email: "sweisman@akamai.com", DisplayName: "Weisman, Sara", HeaderRole: "cc"},
			{Email: "jabrown@akamai.com", DisplayName: "Brown, James", HeaderRole: "cc"},
		},
	}

	result := buildEmailHeaderBlock(input)

	require.Contains(t, result, "EMAIL METADATA:\n")
	require.Contains(t, result, "From: Ponec, Miroslav <mponec@akamai.com>")
	require.Contains(t, result, "To: Dunn, Tim <tdunn@akamai.com>; DeMent, James <jdement@akamai.com>")
	require.Contains(t, result, "CC: Weisman, Sara <sweisman@akamai.com>; Brown, James <jabrown@akamai.com>")
	require.Contains(t, result, "Subject: Immediate CLIC Action Required")
	require.Contains(t, result, "---\nBODY:\n")
}

func TestBuildEmailHeaderBlock_NoCC(t *testing.T) {
	input := ExtractEntitiesInput{
		ContentType: "email",
		SenderName:  "Alice",
		SenderEmail: "alice@example.com",
		Subject:     "Hello",
		Participants: []workflows.Participant{
			{Email: "bob@example.com", DisplayName: "Bob", HeaderRole: "to"},
		},
	}

	result := buildEmailHeaderBlock(input)

	require.Contains(t, result, "From: Alice <alice@example.com>")
	require.Contains(t, result, "To: Bob <bob@example.com>")
	require.NotContains(t, result, "CC:")
}

func TestBuildEmailHeaderBlock_NoDisplayName(t *testing.T) {
	input := ExtractEntitiesInput{
		ContentType: "email",
		SenderEmail: "alice@example.com",
		Participants: []workflows.Participant{
			{Email: "bob@example.com", HeaderRole: "to"},
		},
	}

	result := buildEmailHeaderBlock(input)

	require.Contains(t, result, "From: alice@example.com")
	require.Contains(t, result, "To: bob@example.com")
}

func TestBuildEmailHeaderBlock_EmptyParticipants(t *testing.T) {
	input := ExtractEntitiesInput{
		ContentType: "email",
		SenderName:  "Alice",
		SenderEmail: "alice@example.com",
		Subject:     "Hello",
	}

	result := buildEmailHeaderBlock(input)

	require.Contains(t, result, "From: Alice <alice@example.com>")
	require.NotContains(t, result, "To:")
	require.NotContains(t, result, "CC:")
	require.Contains(t, result, "Subject: Hello")
}

func TestBuildEmailHeaderBlock_NoMetadata(t *testing.T) {
	input := ExtractEntitiesInput{
		ContentType: "email",
	}

	result := buildEmailHeaderBlock(input)
	require.Empty(t, result)
}

func TestBuildEmailHeaderBlock_DefaultToRole(t *testing.T) {
	// Participants with empty HeaderRole default to "to"
	input := ExtractEntitiesInput{
		ContentType: "email",
		SenderEmail: "alice@example.com",
		Participants: []workflows.Participant{
			{Email: "bob@example.com", DisplayName: "Bob"},
		},
	}

	result := buildEmailHeaderBlock(input)
	require.Contains(t, result, "To: Bob <bob@example.com>")
}

func TestExtractEntities_EmailHeadersPrepended(t *testing.T) {
	logger := logging.NewNopLogger()
	var capturedContent string

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			capturedContent = req.Content
			return &aiv1.ExtractEntitiesResponse{
				People:        []*aiv1.PersonEntity{{Name: "Ponec, Miroslav", Role: "sender"}},
				Dates:         []*aiv1.DateEntity{},
				Projects:      []string{},
				Organisations: []string{},
				ActionItems:   []*aiv1.ActionItemEntity{},
				Decisions:     []string{},
				Risks:         []string{},
				DetailedRisks: []*aiv1.RiskEntity{},
				ModelUsed:     "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		JobID:       "job-123",
		Content:     "Please review the router issues.",
		ContentType: "email",
		SenderName:  "Ponec, Miroslav",
		SenderEmail: "mponec@akamai.com",
		Subject:     "Router Issues",
		Participants: []workflows.Participant{
			{Email: "tdunn@akamai.com", DisplayName: "Dunn, Tim", HeaderRole: "to"},
			{Email: "sweisman@akamai.com", DisplayName: "Weisman, Sara", HeaderRole: "cc"},
		},
	}

	output, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify the AI received content with headers prepended
	require.True(t, strings.HasPrefix(capturedContent, "EMAIL METADATA:\n"), "Content should start with EMAIL METADATA header block")
	require.Contains(t, capturedContent, "From: Ponec, Miroslav <mponec@akamai.com>")
	require.Contains(t, capturedContent, "To: Dunn, Tim <tdunn@akamai.com>")
	require.Contains(t, capturedContent, "CC: Weisman, Sara <sweisman@akamai.com>")
	require.Contains(t, capturedContent, "Subject: Router Issues")
	require.Contains(t, capturedContent, "---\nBODY:\nPlease review the router issues.")
}

func TestExtractEntities_NonEmailNoHeaders(t *testing.T) {
	logger := logging.NewNopLogger()
	var capturedContent string

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			capturedContent = req.Content
			return &aiv1.ExtractEntitiesResponse{
				People:        []*aiv1.PersonEntity{},
				Dates:         []*aiv1.DateEntity{},
				Projects:      []string{},
				Organisations: []string{},
				ActionItems:   []*aiv1.ActionItemEntity{},
				Decisions:     []string{},
				Risks:         []string{},
				DetailedRisks: []*aiv1.RiskEntity{},
				ModelUsed:     "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID:    "test-tenant",
		SourceID:    123,
		JobID:       "job-123",
		Content:     "Meeting transcript content here.",
		ContentType: "meeting",
	}

	_, err := activities.ExtractEntities(context.Background(), input)
	require.NoError(t, err)

	// Non-email content should not have headers prepended
	require.Equal(t, "Meeting transcript content here.", capturedContent)
}

// mockPersonLookup is a mock implementation of the PersonLookup interface for testing.
type mockPersonLookup struct {
	getPeopleByEmailsFn         func(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error)
	getNameAliasesByPersonIDsFn func(ctx context.Context, personIDs []int64) (map[int64][]string, error)
	getMetadataByPersonIDsFn    func(ctx context.Context, personIDs []int64) (map[int64]map[string]string, error)
}

func (m *mockPersonLookup) GetPeopleByEmails(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error) {
	if m.getPeopleByEmailsFn != nil {
		return m.getPeopleByEmailsFn(ctx, tenantID, emails)
	}
	return map[string]*PersonInfo{}, nil
}

func (m *mockPersonLookup) GetNameAliasesByPersonIDs(ctx context.Context, personIDs []int64) (map[int64][]string, error) {
	if m.getNameAliasesByPersonIDsFn != nil {
		return m.getNameAliasesByPersonIDsFn(ctx, personIDs)
	}
	return map[int64][]string{}, nil
}

func (m *mockPersonLookup) GetMetadataByPersonIDs(ctx context.Context, personIDs []int64) (map[int64]map[string]string, error) {
	if m.getMetadataByPersonIDsFn != nil {
		return m.getMetadataByPersonIDsFn(ctx, personIDs)
	}
	return map[int64]map[string]string{}, nil
}

func TestEnrichHeaderParticipants_WithMatches(t *testing.T) {
	logger := logging.NewNopLogger()

	pl := &mockPersonLookup{
		getPeopleByEmailsFn: func(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error) {
			return map[string]*PersonInfo{
				"hvarma@example.com": {ID: 1, CanonicalName: "Hrishikesh Varma", PrimaryEmail: "hvarma@example.com", Title: "VP Engineering"},
				"alice@example.com":  {ID: 2, CanonicalName: "Alice Smith", PrimaryEmail: "alice@example.com", Title: "Senior Director, Hardware Engineering", IsInternal: true},
			}, nil
		},
		getNameAliasesByPersonIDsFn: func(ctx context.Context, personIDs []int64) (map[int64][]string, error) {
			return map[int64][]string{
				1: {"Rishi", "Varma"},
				// person 2 has no aliases
			}, nil
		},
	}

	acts := NewExtractionActivities(logger, &mockAIClient{}, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	acts.WithPersonLookup(pl)

	participants := []workflows.Participant{
		{Email: "alice@example.com", DisplayName: "Smith, Alice", HeaderRole: "to"},
		{Email: "bob@example.com", DisplayName: "Jones, Bob", HeaderRole: "cc"},
	}

	enrichedSender, enrichedParticipants := acts.enrichHeaderParticipants(
		context.Background(), "tenant1", "hvarma@example.com", "Varma, Hrishikesh", participants, logger,
	)

	// Sender should be enriched with canonical name, title, and aliases
	require.Equal(t, "Hrishikesh Varma [VP Engineering] (also known as: Rishi, Varma)", enrichedSender)

	// alice@example.com participant should be enriched with title (no aliases)
	require.Equal(t, "Alice Smith [Senior Director, Hardware Engineering]", enrichedParticipants[0].DisplayName)

	// bob@example.com not in person DB — should keep original display name
	require.Equal(t, "Jones, Bob", enrichedParticipants[1].DisplayName)

	// Header roles must be preserved
	require.Equal(t, "to", enrichedParticipants[0].HeaderRole)
	require.Equal(t, "cc", enrichedParticipants[1].HeaderRole)
}

func TestEnrichHeaderParticipants_NoMatches(t *testing.T) {
	logger := logging.NewNopLogger()

	pl := &mockPersonLookup{
		getPeopleByEmailsFn: func(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error) {
			// No matches in person DB
			return map[string]*PersonInfo{}, nil
		},
	}

	acts := NewExtractionActivities(logger, &mockAIClient{}, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	acts.WithPersonLookup(pl)

	participants := []workflows.Participant{
		{Email: "alice@example.com", DisplayName: "Smith, Alice", HeaderRole: "to"},
	}

	enrichedSender, enrichedParticipants := acts.enrichHeaderParticipants(
		context.Background(), "tenant1", "hvarma@example.com", "Varma, Hrishikesh", participants, logger,
	)

	// Nothing found — original values must be returned unchanged
	require.Equal(t, "Varma, Hrishikesh", enrichedSender)
	require.Equal(t, "Smith, Alice", enrichedParticipants[0].DisplayName)
}

func TestEnrichHeaderParticipants_NilPersonLookup(t *testing.T) {
	logger := logging.NewNopLogger()
	var capturedContent string

	mockClient := &mockAIClient{
		extractEntitiesFn: func(ctx context.Context, req *aiv1.ExtractEntitiesRequest) (*aiv1.ExtractEntitiesResponse, error) {
			capturedContent = req.Content
			return &aiv1.ExtractEntitiesResponse{
				People:        []*aiv1.PersonEntity{},
				Dates:         []*aiv1.DateEntity{},
				Projects:      []string{},
				Organisations: []string{},
				ActionItems:   []*aiv1.ActionItemEntity{},
				Decisions:     []string{},
				Risks:         []string{},
				DetailedRisks: []*aiv1.RiskEntity{},
				ModelUsed:     "test-model",
			}, nil
		},
	}

	// No WithPersonLookup call — personLookup is nil
	acts := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := ExtractEntitiesInput{
		TenantID:    "tenant1",
		SourceID:    1,
		JobID:       "job-1",
		Content:     "Please review the attached document.",
		ContentType: "email",
		SenderName:  "Varma, Hrishikesh",
		SenderEmail: "hvarma@example.com",
		Subject:     "Review needed",
		Participants: []workflows.Participant{
			{Email: "alice@example.com", DisplayName: "Smith, Alice", HeaderRole: "to"},
		},
	}

	_, err := acts.ExtractEntities(context.Background(), input)
	require.NoError(t, err)

	// Raw names must appear in the header block (no enrichment applied)
	require.Contains(t, capturedContent, "From: Varma, Hrishikesh <hvarma@example.com>")
	require.Contains(t, capturedContent, "To: Smith, Alice <alice@example.com>")
}

func TestEnrichHeaderParticipants_LookupError(t *testing.T) {
	logger := logging.NewNopLogger()

	pl := &mockPersonLookup{
		getPeopleByEmailsFn: func(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error) {
			return nil, context.DeadlineExceeded
		},
	}

	acts := NewExtractionActivities(logger, &mockAIClient{}, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	acts.WithPersonLookup(pl)

	participants := []workflows.Participant{
		{Email: "alice@example.com", DisplayName: "Smith, Alice", HeaderRole: "to"},
	}

	// Error must be swallowed; original values must be returned
	enrichedSender, enrichedParticipants := acts.enrichHeaderParticipants(
		context.Background(), "tenant1", "hvarma@example.com", "Varma, Hrishikesh", participants, logger,
	)

	require.Equal(t, "Varma, Hrishikesh", enrichedSender)
	require.Equal(t, "Smith, Alice", enrichedParticipants[0].DisplayName)
}

// TestExtractAssertions_MeetingSkipped verifies that assertion extraction is skipped
// for meeting transcripts. The email-oriented assertion prompt returns 0 results for
// conversational transcript format. Meeting assertions are created by Stage 4.5
// (PersistFindings) from the analysis output instead.
func TestExtractAssertions_MeetingSkipped(t *testing.T) {
	logger := logging.NewNopLogger()

	aiCalled := false
	mockClient := &mockAIClient{
		extractAssertionsFn: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
			aiCalled = true
			return &aiv1.AssertionResponse{}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := workflows.ExtractAssertionsInput{
		TenantID:    "test-tenant",
		SourceID:    456,
		ContentID:   "mt-test123",
		JobID:       "job-456",
		Content:     "00:00 Speaker A: Let's discuss the roadmap.\n00:05 Speaker B: I agree we need to prioritize.",
		ContentType: "meeting",
	}

	count, err := activities.ExtractAssertions(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 0, count)
	require.False(t, aiCalled, "AI service should NOT be called for meeting transcripts")
}

// TestExtractAssertions_EmailNotSkipped verifies that assertion extraction runs normally
// for email content (not skipped like meetings).
func TestExtractAssertions_EmailNotSkipped(t *testing.T) {
	logger := logging.NewNopLogger()

	aiCalled := false
	mockClient := &mockAIClient{
		extractAssertionsFn: func(ctx context.Context, req *aiv1.AssertionRequest) (*aiv1.AssertionResponse, error) {
			aiCalled = true
			return &aiv1.AssertionResponse{
				ModelUsed: "test-model",
			}, nil
		},
	}

	activities := NewExtractionActivities(logger, mockClient, &mockAssertionRepository{}, &mockEntityRepository{}, nil)

	input := workflows.ExtractAssertionsInput{
		TenantID:    "test-tenant",
		SourceID:    789,
		ContentID:   "em-test456",
		JobID:       "job-789",
		Content:     "We decided to proceed with Option B for the Q3 rollout.",
		ContentType: "email",
	}

	_, err := activities.ExtractAssertions(context.Background(), input)
	require.NoError(t, err)
	require.True(t, aiCalled, "AI service should be called for email content")
}

func TestBuildEmailHeaderBlock_EnrichedParticipants(t *testing.T) {
	// Full flow test: enriched participants -> buildEmailHeaderBlock
	// Simulates the "Varma, Hrishikesh" -> "Hrishikesh Varma [VP Engineering] (also known as: Rishi)" case
	input := ExtractEntitiesInput{
		ContentType: "email",
		SenderName:  "Hrishikesh Varma [VP Engineering] (also known as: Rishi)",
		SenderEmail: "hvarma@example.com",
		Subject:     "Q3 Planning",
		Participants: []workflows.Participant{
			{Email: "alice@example.com", DisplayName: "Alice Smith [Senior Director, Hardware Engineering]", HeaderRole: "to"},
		},
	}

	result := buildEmailHeaderBlock(input)

	require.Contains(t, result, "From: Hrishikesh Varma [VP Engineering] (also known as: Rishi) <hvarma@example.com>")
	require.Contains(t, result, "To: Alice Smith [Senior Director, Hardware Engineering] <alice@example.com>")
	require.Contains(t, result, "Subject: Q3 Planning")
}

func TestEnrichHeaderParticipants_WithMetadata(t *testing.T) {
	logger := logging.NewNopLogger()

	pl := &mockPersonLookup{
		getPeopleByEmailsFn: func(ctx context.Context, tenantID string, emails []string) (map[string]*PersonInfo, error) {
			return map[string]*PersonInfo{
				"sweisman@akamai.com": {ID: 8004, CanonicalName: "Sara Weisman", PrimaryEmail: "sweisman@akamai.com", Title: "MTC Program Solution Lead", IsInternal: true},
				"tdunn@akamai.com":    {ID: 1001, CanonicalName: "Tim Dunn", PrimaryEmail: "tdunn@akamai.com", Title: "Senior Director, Hardware Engineering", IsInternal: true},
			}, nil
		},
		getNameAliasesByPersonIDsFn: func(ctx context.Context, personIDs []int64) (map[int64][]string, error) {
			return map[int64][]string{}, nil
		},
		getMetadataByPersonIDsFn: func(ctx context.Context, personIDs []int64) (map[int64]map[string]string, error) {
			return map[int64]map[string]string{
				8004: {"reports_to": "James Brown", "notes": "CLIC program lead"},
				// 1001 has no metadata
			}, nil
		},
	}

	acts := NewExtractionActivities(logger, &mockAIClient{}, &mockAssertionRepository{}, &mockEntityRepository{}, nil)
	acts.WithPersonLookup(pl)

	participants := []workflows.Participant{
		{Email: "tdunn@akamai.com", DisplayName: "Dunn, Tim", HeaderRole: "to"},
	}

	enrichedSender, enrichedParticipants := acts.enrichHeaderParticipants(
		context.Background(), "tenant1", "sweisman@akamai.com", "Weisman, Sara", participants, logger,
	)

	// Sara Weisman should include title + metadata (reports_to and notes from whitelist)
	require.Equal(t, "Sara Weisman [MTC Program Solution Lead, reports to James Brown, notes CLIC program lead]", enrichedSender)

	// Tim Dunn has no metadata — just title
	require.Equal(t, "Tim Dunn [Senior Director, Hardware Engineering]", enrichedParticipants[0].DisplayName)
}

func TestFormatEnrichedName_MetadataWhitelist(t *testing.T) {
	// Only whitelisted keys should appear
	person := &PersonInfo{
		ID:            1,
		CanonicalName: "Test Person",
		Title:         "Engineer",
		Metadata: map[string]string{
			"reports_to":     "Boss Name",
			"team":           "Platform",
			"notes":          "Key contributor",
			"secret_field":   "should not appear",
			"internal_notes": "also hidden",
		},
	}

	result := formatEnrichedName(person, nil)

	require.Contains(t, result, "Engineer")
	require.Contains(t, result, "reports to Boss Name")
	require.Contains(t, result, "notes Key contributor")
	require.Contains(t, result, "team Platform")
	require.NotContains(t, result, "secret_field")
	require.NotContains(t, result, "internal_notes")
}

func TestFormatEnrichedName_NoMetadata(t *testing.T) {
	// No metadata — same behavior as before
	person := &PersonInfo{
		ID:            1,
		CanonicalName: "Alice",
		Title:         "Director",
	}

	aliases := map[int64][]string{1: {"Ali"}}
	result := formatEnrichedName(person, aliases)
	require.Equal(t, "Alice [Director] (also known as: Ali)", result)
}

func TestFormatEnrichedName_MetadataNoTitle(t *testing.T) {
	// Metadata without title
	person := &PersonInfo{
		ID:            1,
		CanonicalName: "Bob",
		Metadata:      map[string]string{"reports_to": "Alice"},
	}

	result := formatEnrichedName(person, nil)
	require.Equal(t, "Bob [reports to Alice]", result)
}
