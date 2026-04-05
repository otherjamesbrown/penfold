package conversationservice

// Tests for GetConversationProcessingStatus RPC.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	"github.com/otherjamesbrown/penfold/pkg/pipeline"
)

// ============ Pipeline Repository Mock ============

// MockPipelineRepository is a mock for the pipeline data access layer used by
// GetConversationProcessingStatus. The service will depend on this interface
// (or the concrete *pipeline.Repository) to fetch pipeline runs per source.
type MockPipelineRepository struct {
	GetSourceByContentIDFunc func(ctx context.Context, contentID string) (*pipeline.PendingSource, error)
	ListSourceHistoryFunc    func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error)
}

func (m *MockPipelineRepository) GetSourceByContentID(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
	if m.GetSourceByContentIDFunc != nil {
		return m.GetSourceByContentIDFunc(ctx, contentID)
	}
	return nil, errors.New("GetSourceByContentIDFunc not implemented")
}

func (m *MockPipelineRepository) ListSourceHistory(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
	if m.ListSourceHistoryFunc != nil {
		return m.ListSourceHistoryFunc(ctx, sourceID, stageFilter)
	}
	return nil, errors.New("ListSourceHistoryFunc not implemented")
}

// ============ Test helpers ============

func intPtr(i int) *int {
	return &i
}

// makeCompletedRun builds a completed PipelineRun for a given stage with token counts.
func makeCompletedRun(sourceID int64, stage string, inputTokens, outputTokens int) pipeline.PipelineRun {
	return pipeline.PipelineRun{
		ID:           1,
		SourceID:     sourceID,
		Stage:        stage,
		Status:       "completed",
		CreatedAt:    time.Now(),
		InputTokens:  intPtr(inputTokens),
		OutputTokens: intPtr(outputTokens),
	}
}

// makeTriageRunWithContribution builds a triage PipelineRun with ParsedData
// that includes content_contribution and contribution_reason fields.
func makeTriageRunWithContribution(sourceID int64, inputTokens, outputTokens int, contribution, reason string) pipeline.PipelineRun {
	triageData := map[string]interface{}{
		"content_contribution": contribution,
		"contribution_reason":  reason,
	}
	parsedJSON, _ := json.Marshal(triageData)
	return pipeline.PipelineRun{
		ID:           1,
		SourceID:     sourceID,
		Stage:        "triage",
		Status:       "completed",
		CreatedAt:    time.Now(),
		InputTokens:  &inputTokens,
		OutputTokens: &outputTokens,
		ParsedData:   parsedJSON,
	}
}

// makeSkippedRun builds a skipped PipelineRun for a given stage.
func makeSkippedRun(sourceID int64, stage, skipReason string) pipeline.PipelineRun {
	return pipeline.PipelineRun{
		ID:         1,
		SourceID:   sourceID,
		Stage:      stage,
		Status:     "skipped",
		CreatedAt:  time.Now(),
		SkipReason: &skipReason,
	}
}

// makeFailedRun builds a failed PipelineRun for a given stage.
func makeFailedRun(sourceID int64, stage string) pipeline.PipelineRun {
	return pipeline.PipelineRun{
		ID:        1,
		SourceID:  sourceID,
		Stage:     stage,
		Status:    "failed",
		CreatedAt: time.Now(),
	}
}

// ============ GetConversationProcessingStatus Tests ============

// TestGetConversationProcessingStatus_HappyPath verifies aggregated counts and
// per-item status are populated correctly for a conversation with items at
// various processing states (completed, processing/running, failed, pending).
//
// Maps to acceptance criterion:
//   - total_items = 24 (24 items in the conversation)
//   - completed + processing + failed + pending = 24
//   - Each item has content_contribution and stage breakdown
//   - total_input_tokens and total_output_tokens are summed across all items
func TestGetConversationProcessingStatus_HappyPath(t *testing.T) {
	now := time.Now()

	// Build 24 items with mixed states matching the spec fixture.
	// Distribution: 10 completed, 5 processing, 5 failed, 4 pending.
	items := []ConversationItem{}
	for i := 0; i < 24; i++ {
		sourceID := int64(1000 + i)
		items = append(items, ConversationItem{
			ConversationID: "conv-thread-63",
			ContentID:      fmt.Sprintf("content-%03d", i),
			SourceID:       &sourceID,
			AddedAt:        now.Add(-time.Duration(i) * time.Hour),
			TenantID:       "tenant-abc",
		})
	}

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			assert.Equal(t, "conv-thread-63", conversationID)
			return &ConversationDetail{
				ID:       "conv-thread-63",
				TenantID: tenantID,
				Topic:    "Q3 Planning Thread",
				Items:    items,
			}, nil
		},
	}

	// For items 0-9 (completed): all stages done, medium contribution
	// For items 10-14 (processing): partial stages, running last stage
	// For items 15-19 (failed): at least one stage failed
	// For items 20-23 (pending): no pipeline runs yet
	pipelineRepo := &MockPipelineRepository{
		GetSourceByContentIDFunc: func(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
			// Extract index from contentID "content-NNN"
			var idx int
			_, _ = fmt.Sscanf(contentID, "content-%d", &idx)
			sourceID := int64(1000 + idx)
			return &pipeline.PendingSource{
				ID:        sourceID,
				ContentID: contentID,
			}, nil
		},
		ListSourceHistoryFunc: func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
			idx := int(sourceID - 1000)
			switch {
			case idx < 10: // completed items — triage includes contribution data
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "parse", 0, 0),
					makeTriageRunWithContribution(sourceID, 120, 40, "MEDIUM", "relevant business content"),
					makeCompletedRun(sourceID, "segment", 0, 0),
					makeCompletedRun(sourceID, "extract_ner", 200, 80),
					makeCompletedRun(sourceID, "analyze", 300, 100),
					makeCompletedRun(sourceID, "persist", 0, 0),
				}, nil
			case idx < 15: // processing items — last stage still running
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "parse", 0, 0),
					makeCompletedRun(sourceID, "triage", 100, 30),
					{SourceID: sourceID, Stage: "segment", Status: "running", CreatedAt: time.Now()},
				}, nil
			case idx < 20: // failed items — one stage failed
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "parse", 0, 0),
					makeFailedRun(sourceID, "triage"),
				}, nil
			default: // pending items — no pipeline runs
				return []pipeline.PipelineRun{}, nil
			}
		},
	}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-thread-63",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify aggregate counts
	assert.Equal(t, "conv-thread-63", resp.ConversationId)
	assert.Equal(t, "Q3 Planning Thread", resp.Topic)
	assert.Equal(t, int32(24), resp.TotalItems)
	assert.Equal(t, int32(10), resp.Completed, "expected 10 completed items")
	assert.Equal(t, int32(5), resp.Processing, "expected 5 processing items")
	assert.Equal(t, int32(5), resp.Failed, "expected 5 failed items")
	assert.Equal(t, int32(4), resp.Pending, "expected 4 pending items")

	// Counts must add up to total
	totalAccounted := resp.Completed + resp.Processing + resp.Failed + resp.Pending
	assert.Equal(t, resp.TotalItems, totalAccounted,
		"completed + processing + failed + pending must equal total_items")

	// Each item must have per-item details
	require.Len(t, resp.Items, 24, "all 24 items must have per-item status entries")

	// Spot-check: first item (completed) has contribution and stage results
	firstItem := resp.Items[0]
	assert.NotEmpty(t, firstItem.ContentId)
	assert.NotEmpty(t, firstItem.ContentContribution,
		"completed item must have content_contribution set")
	assert.NotEmpty(t, firstItem.ContributionReason,
		"completed item must have contribution_reason set")
	assert.NotEmpty(t, firstItem.Stages,
		"completed item must have stage breakdown")

	// Token totals: 10 completed items each with triage(120 in, 40 out) +
	// extract_ner(200 in, 80 out) + analyze(300 in, 100 out) = 620 in, 220 out per item
	// Plus 5 processing items each with triage(100 in, 30 out) = 500 in, 150 out
	// Total input = 10*620 + 5*100 = 6700; total output = 10*220 + 5*30 = 2350
	require.NotNil(t, resp.TotalInputTokens, "total_input_tokens must be populated")
	require.NotNil(t, resp.TotalOutputTokens, "total_output_tokens must be populated")
	assert.Equal(t, int32(6700), *resp.TotalInputTokens,
		"total_input_tokens must be summed across all items")
	assert.Equal(t, int32(2350), *resp.TotalOutputTokens,
		"total_output_tokens must be summed across all items")
}

// TestGetConversationProcessingStatus_ContributionGating verifies that items
// with LOW contribution have STAGE_ANALYZE marked SKIPPED, while items with
// MEDIUM or HIGH contribution have STAGE_ANALYZE marked COMPLETED.
//
// Maps to acceptance criteria:
//   - Items with contribution=LOW show STAGE_ANALYZE as SKIPPED
//   - Items with contribution=MEDIUM/HIGH show STAGE_ANALYZE as COMPLETED
func TestGetConversationProcessingStatus_ContributionGating(t *testing.T) {
	lowSourceID := int64(2001)
	medSourceID := int64(2002)
	highSourceID := int64(2003)

	items := []ConversationItem{
		{
			ConversationID: "conv-gating-test",
			ContentID:      "content-low",
			SourceID:       &lowSourceID,
			TenantID:       "tenant-abc",
		},
		{
			ConversationID: "conv-gating-test",
			ContentID:      "content-medium",
			SourceID:       &medSourceID,
			TenantID:       "tenant-abc",
		},
		{
			ConversationID: "conv-gating-test",
			ContentID:      "content-high",
			SourceID:       &highSourceID,
			TenantID:       "tenant-abc",
		},
	}

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-gating-test",
				TenantID: tenantID,
				Topic:    "Contribution Gating Test",
				Items:    items,
			}, nil
		},
	}

	pipelineRepo := &MockPipelineRepository{
		GetSourceByContentIDFunc: func(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
			switch contentID {
			case "content-low":
				return &pipeline.PendingSource{ID: lowSourceID, ContentID: contentID}, nil
			case "content-medium":
				return &pipeline.PendingSource{ID: medSourceID, ContentID: contentID}, nil
			case "content-high":
				return &pipeline.PendingSource{ID: highSourceID, ContentID: contentID}, nil
			}
			return nil, fmt.Errorf("unknown content: %s", contentID)
		},
		ListSourceHistoryFunc: func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
			switch sourceID {
			case lowSourceID:
				// LOW contribution: triage has contribution=LOW, analyze stage is SKIPPED
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "parse", 0, 0),
					makeTriageRunWithContribution(sourceID, 100, 30, "LOW", "not relevant"),
					makeSkippedRun(sourceID, "analyze", "contribution_gating:LOW"),
					makeCompletedRun(sourceID, "persist", 0, 0),
				}, nil
			case medSourceID:
				// MEDIUM contribution: triage has contribution=MEDIUM, analyze stage is COMPLETED
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "parse", 0, 0),
					makeTriageRunWithContribution(sourceID, 150, 50, "MEDIUM", "moderate relevance"),
					makeCompletedRun(sourceID, "analyze", 400, 120),
					makeCompletedRun(sourceID, "persist", 0, 0),
				}, nil
			case highSourceID:
				// HIGH contribution: triage has contribution=HIGH, analyze stage is COMPLETED
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "parse", 0, 0),
					makeTriageRunWithContribution(sourceID, 180, 60, "HIGH", "highly relevant"),
					makeCompletedRun(sourceID, "analyze", 500, 150),
					makeCompletedRun(sourceID, "persist", 0, 0),
				}, nil
			}
			return nil, fmt.Errorf("unknown source: %d", sourceID)
		},
	}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-gating-test",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Items, 3)

	// Find each item by content_id
	var lowItem, medItem, highItem *conversationv1.ContentProcessingSummary
	for _, item := range resp.Items {
		switch item.ContentId {
		case "content-low":
			lowItem = item
		case "content-medium":
			medItem = item
		case "content-high":
			highItem = item
		}
	}

	require.NotNil(t, lowItem, "low contribution item must be present")
	require.NotNil(t, medItem, "medium contribution item must be present")
	require.NotNil(t, highItem, "high contribution item must be present")

	// LOW: contribution is LOW, analyze stage is SKIPPED
	assert.Equal(t, "LOW", lowItem.ContentContribution,
		"low item must report contribution=LOW")
	analyzeStage := findStage(t, lowItem.Stages, "analyze")
	require.NotNil(t, analyzeStage, "low item must have an analyze stage entry")
	assert.Equal(t, contentv1.StageStatus_STAGE_STATUS_SKIPPED, analyzeStage.Status,
		"LOW contribution item: analyze stage must be SKIPPED")

	// MEDIUM: contribution is MEDIUM, analyze stage is COMPLETED
	assert.Equal(t, "MEDIUM", medItem.ContentContribution,
		"medium item must report contribution=MEDIUM")
	analyzeStage = findStage(t, medItem.Stages, "analyze")
	require.NotNil(t, analyzeStage, "medium item must have an analyze stage entry")
	assert.Equal(t, contentv1.StageStatus_STAGE_STATUS_COMPLETED, analyzeStage.Status,
		"MEDIUM contribution item: analyze stage must be COMPLETED")

	// HIGH: contribution is HIGH, analyze stage is COMPLETED
	assert.Equal(t, "HIGH", highItem.ContentContribution,
		"high item must report contribution=HIGH")
	analyzeStage = findStage(t, highItem.Stages, "analyze")
	require.NotNil(t, analyzeStage, "high item must have an analyze stage entry")
	assert.Equal(t, contentv1.StageStatus_STAGE_STATUS_COMPLETED, analyzeStage.Status,
		"HIGH contribution item: analyze stage must be COMPLETED")
}

// stageTestNameToEnum maps stage name strings to proto ProcessingStage enum values,
// used by findStage to look up stages by name in tests.
var stageTestNameToEnum = map[string]contentv1.ProcessingStage{
	"parse":            contentv1.ProcessingStage_PROCESSING_STAGE_PARSE,
	"segment":          contentv1.ProcessingStage_PROCESSING_STAGE_SEGMENT,
	"triage":           contentv1.ProcessingStage_PROCESSING_STAGE_TRIAGE,
	"extract_ner":      contentv1.ProcessingStage_PROCESSING_STAGE_EXTRACT_NER,
	"extract_semantic": contentv1.ProcessingStage_PROCESSING_STAGE_EXTRACT_SEMANTIC,
	"resolve":          contentv1.ProcessingStage_PROCESSING_STAGE_RESOLVE,
	"analyze":          contentv1.ProcessingStage_PROCESSING_STAGE_ANALYZE,
	"persist":          contentv1.ProcessingStage_PROCESSING_STAGE_PERSIST,
	"embed":            contentv1.ProcessingStage_PROCESSING_STAGE_EMBED,
}

// findStage is a helper that locates a StageResult by stage name.
func findStage(t *testing.T, stages []*contentv1.StageResult, stageName string) *contentv1.StageResult {
	t.Helper()
	protoStage, ok := stageTestNameToEnum[stageName]
	if !ok {
		t.Errorf("findStage: unknown stage name %q", stageName)
		return nil
	}
	for _, s := range stages {
		if s.Stage == protoStage {
			return s
		}
	}
	return nil
}

// TestGetConversationProcessingStatus_ConversationNotFound verifies that when
// the repository reports no conversation, the RPC returns codes.NotFound.
func TestGetConversationProcessingStatus_ConversationNotFound(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return nil, nil // Repository returns nil — conversation does not exist
		},
	}

	pipelineRepo := &MockPipelineRepository{}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-does-not-exist",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a gRPC status error")
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "conversation not found")
}

// TestGetConversationProcessingStatus_MissingConversationID verifies that an
// empty conversation_id is rejected with codes.InvalidArgument.
func TestGetConversationProcessingStatus_MissingConversationID(t *testing.T) {
	mockRepo := &MockRepository{}
	pipelineRepo := &MockPipelineRepository{}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "", // Missing conversation ID
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok, "error must be a gRPC status error")
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "conversation_id")
}

// TestGetConversationProcessingStatus_EmptyConversation verifies that a
// conversation that exists but has no items returns zero counts and an empty
// items list.
func TestGetConversationProcessingStatus_EmptyConversation(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-empty",
				TenantID: tenantID,
				Topic:    "Empty Thread",
				Items:    []ConversationItem{}, // No items
			}, nil
		},
	}

	pipelineRepo := &MockPipelineRepository{}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-empty",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "conv-empty", resp.ConversationId)
	assert.Equal(t, "Empty Thread", resp.Topic)
	assert.Equal(t, int32(0), resp.TotalItems)
	assert.Equal(t, int32(0), resp.Completed)
	assert.Equal(t, int32(0), resp.Processing)
	assert.Equal(t, int32(0), resp.Failed)
	assert.Equal(t, int32(0), resp.Pending)
	assert.Empty(t, resp.Items, "items list must be empty for a conversation with no items")

	// Token totals are nil or zero when there are no items to aggregate
	// (implementation may return nil optional fields or zero values)
	if resp.TotalInputTokens != nil {
		assert.Equal(t, int32(0), *resp.TotalInputTokens)
	}
	if resp.TotalOutputTokens != nil {
		assert.Equal(t, int32(0), *resp.TotalOutputTokens)
	}
}

// TestGetConversationProcessingStatus_RepositoryError verifies that a
// repository failure returns codes.Internal.
func TestGetConversationProcessingStatus_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return nil, errors.New("database connection failed")
		},
	}

	pipelineRepo := &MockPipelineRepository{}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-thread-63",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to get conversation")
}

// TestGetConversationProcessingStatus_TokenSummation verifies that token counts
// from individual pipeline runs are correctly summed across all stages and items.
// This test uses a small, controlled dataset to make exact verification easy.
func TestGetConversationProcessingStatus_TokenSummation(t *testing.T) {
	src1 := int64(3001)
	src2 := int64(3002)

	items := []ConversationItem{
		{ConversationID: "conv-tokens", ContentID: "ct-1", SourceID: &src1, TenantID: "t"},
		{ConversationID: "conv-tokens", ContentID: "ct-2", SourceID: &src2, TenantID: "t"},
	}

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-tokens",
				TenantID: tenantID,
				Topic:    "Token Test",
				Items:    items,
			}, nil
		},
	}

	pipelineRepo := &MockPipelineRepository{
		GetSourceByContentIDFunc: func(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
			switch contentID {
			case "ct-1":
				return &pipeline.PendingSource{ID: src1, ContentID: contentID}, nil
			case "ct-2":
				return &pipeline.PendingSource{ID: src2, ContentID: contentID}, nil
			}
			return nil, fmt.Errorf("unknown: %s", contentID)
		},
		ListSourceHistoryFunc: func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
			switch sourceID {
			case src1:
				// item 1: triage=50in/20out, extract_ner=100in/40out, analyze=200in/80out
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "triage", 50, 20),
					makeCompletedRun(sourceID, "extract_ner", 100, 40),
					makeCompletedRun(sourceID, "analyze", 200, 80),
				}, nil
			case src2:
				// item 2: triage=75in/30out, extract_ner=150in/60out, analyze=250in/90out
				return []pipeline.PipelineRun{
					makeCompletedRun(sourceID, "triage", 75, 30),
					makeCompletedRun(sourceID, "extract_ner", 150, 60),
					makeCompletedRun(sourceID, "analyze", 250, 90),
				}, nil
			}
			return nil, fmt.Errorf("unknown source: %d", sourceID)
		},
	}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-tokens",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)

	// item1: 50+100+200=350 in, 20+40+80=140 out
	// item2: 75+150+250=475 in, 30+60+90=180 out
	// total:  825 in, 320 out
	require.NotNil(t, resp.TotalInputTokens, "total_input_tokens must be set")
	require.NotNil(t, resp.TotalOutputTokens, "total_output_tokens must be set")
	assert.Equal(t, int32(825), *resp.TotalInputTokens,
		"input tokens must be summed across all stages and items")
	assert.Equal(t, int32(320), *resp.TotalOutputTokens,
		"output tokens must be summed across all stages and items")
}

// TestGetConversationProcessingStatus_ItemsWithNoSourceID verifies graceful
// handling when a ConversationItem has no SourceID (source not yet ingested).
// Such items should be counted as PENDING with no stage details.
func TestGetConversationProcessingStatus_ItemsWithNoSourceID(t *testing.T) {
	src1 := int64(4001)

	items := []ConversationItem{
		{ConversationID: "conv-nosrc", ContentID: "c-has-source", SourceID: &src1, TenantID: "t"},
		{ConversationID: "conv-nosrc", ContentID: "c-no-source", SourceID: nil, TenantID: "t"}, // No source
	}

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-nosrc",
				TenantID: tenantID,
				Topic:    "Mixed Source Thread",
				Items:    items,
			}, nil
		},
	}

	pipelineRepo := &MockPipelineRepository{
		GetSourceByContentIDFunc: func(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
			if contentID == "c-has-source" {
				return &pipeline.PendingSource{ID: src1, ContentID: contentID}, nil
			}
			// Simulate: content ID lookup fails because source not yet created
			return nil, fmt.Errorf("source not found for content: %s", contentID)
		},
		ListSourceHistoryFunc: func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
			return []pipeline.PipelineRun{
				makeCompletedRun(sourceID, "parse", 0, 0),
				makeCompletedRun(sourceID, "triage", 100, 40),
			}, nil
		},
	}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-nosrc",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(2), resp.TotalItems)

	// The no-source item must be represented with pending state
	var noSourceItem *conversationv1.ContentProcessingSummary
	for _, item := range resp.Items {
		if item.ContentId == "c-no-source" {
			noSourceItem = item
			break
		}
	}
	require.NotNil(t, noSourceItem, "item with no source must appear in results")
	assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_PENDING, noSourceItem.State,
		"item with no source must be PENDING")
	assert.Empty(t, noSourceItem.Stages, "item with no source must have no stage details")
}

// ============ Bug Regression Test: pf-a5912d ============

// TestGetConversationProcessingStatus_ReprocessedItemShowsStaleCompleted is a
// regression test for pf-a5912d.
//
// Bug: GetConversationProcessingStatus derives state exclusively from
// pipeline_runs via DeriveProcessingState. When ReprocessContent resets
// sources.processing_status to 'pending', the old pipeline_runs from the
// previous processing cycle still show all stages as 'completed'. The handler
// therefore returns COMPLETED for the item instead of PENDING.
//
// The single-item GetProcessingStatus (contentservice) correctly reads the
// authoritative processing_status field from the source record. This
// conversation-level handler does not.
//
// Fix path (DO NOT implement here — see pf-a5912d):
//  1. Add ProcessingStatus field to pipeline.PendingSource in pkg/pipeline/types.go
//  2. Update GetSourceByContentID in pkg/pipeline/repository.go to SELECT processing_status
//  3. In GetConversationProcessingStatus, call GetSourceByContentID and check its
//     ProcessingStatus before calling DeriveProcessingState on the old pipeline_runs.
//     If ProcessingStatus is 'pending' or 'processing', use that instead.
//
// After the fix, this test must pass. It currently FAILS because the handler
// returns PROCESSING_STATE_COMPLETED instead of PROCESSING_STATE_PENDING.
//
// NOTE: When the fix lands, update GetSourceByContentIDFunc below to set
// ProcessingStatus: "pending" on the returned *pipeline.PendingSource once that
// field has been added to the struct (pkg/pipeline/types.go).
func TestGetConversationProcessingStatus_ReprocessedItemShowsStaleCompleted(t *testing.T) {
	// Scenario: a single conversation item whose source was reprocessed.
	// The source's processing_status has been reset to 'pending' by ReprocessContent,
	// but the pipeline_runs table still contains the completed runs from the previous cycle.
	reprocessedSourceID := int64(9001)

	items := []ConversationItem{
		{
			ConversationID: "conv-reprocess-bug",
			ContentID:      "content-reprocessed",
			SourceID:       &reprocessedSourceID,
			TenantID:       "tenant-test",
		},
	}

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-reprocess-bug",
				TenantID: tenantID,
				Topic:    "Reprocess Bug Repro",
				Items:    items,
			}, nil
		},
	}

	pipelineRepo := &MockPipelineRepository{
		// GetSourceByContentIDFunc returns the source record with processing_status='pending'.
		// The current handler NEVER calls this function — it only calls ListSourceHistory.
		// After the fix, the handler must call this and honour the returned ProcessingStatus.
		//
		// IMPORTANT: Once pipeline.PendingSource gains a ProcessingStatus field (part of the
		// fix in pkg/pipeline/types.go), update this func to set:
		//   ProcessingStatus: "pending"
		// on the returned *pipeline.PendingSource so the fixed handler can read it.
		GetSourceByContentIDFunc: func(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
			// Returns the source as it exists after ReprocessContent reset its status.
			// ProcessingStatus is "pending" because ReprocessContent reset it.
			return &pipeline.PendingSource{
				ID:               reprocessedSourceID,
				ContentID:        contentID,
				ProcessingStatus: "pending",
			}, nil
		},

		// ListSourceHistoryFunc returns old completed runs from the PREVIOUS processing cycle.
		// These runs are stale — the source was reprocessed and is pending again, but
		// the pipeline_runs table has not yet been updated for the new cycle.
		ListSourceHistoryFunc: func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
			// All stages from the prior run: completed. This is what DeriveProcessingState
			// sees and it incorrectly concludes the item is COMPLETED.
			return []pipeline.PipelineRun{
				makeCompletedRun(sourceID, "parse", 0, 0),
				makeTriageRunWithContribution(sourceID, 100, 30, "MEDIUM", "relevant"),
				makeCompletedRun(sourceID, "segment", 0, 0),
				makeCompletedRun(sourceID, "extract_ner", 200, 80),
				makeCompletedRun(sourceID, "analyze", 300, 100),
				makeCompletedRun(sourceID, "persist", 0, 0),
			}, nil
		},
	}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-reprocess-bug",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Items, 1, "conversation must have exactly 1 item")

	item := resp.Items[0]
	assert.Equal(t, "content-reprocessed", item.ContentId)

	// The source was just reprocessed: processing_status='pending' in the DB.
	// Old pipeline_runs from the prior cycle all show 'completed'.
	// Correct behaviour: the item must be reported as PENDING.
	// Bug behaviour (current code): the item is reported as COMPLETED because
	// DeriveProcessingState only reads the stale pipeline_runs.
	assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_PENDING, item.State,
		"reprocessed item must be PENDING even when old pipeline_runs show completed: "+
			"GetConversationProcessingStatus must check sources.processing_status (via GetSourceByContentID), "+
			"not only derive state from stale pipeline_runs")

	// Aggregate counters must reflect the pending state.
	assert.Equal(t, int32(1), resp.TotalItems)
	assert.Equal(t, int32(0), resp.Completed,
		"completed count must be 0 — the item was reprocessed and is pending")
	assert.Equal(t, int32(1), resp.Pending,
		"pending count must be 1 — the item is awaiting its new processing cycle")
	assert.Equal(t, int32(0), resp.Processing)
	assert.Equal(t, int32(0), resp.Failed)
}

// ============ Bug Regression Test: pf-5648b0 ============

// TestGetConversationProcessingStatus_RejectedItemNotMaskedAsCompleted is a
// reproduction test for pf-5648b0.
//
// Bug: GetConversationProcessingStatus has a switch on sources.processing_status
// (lines 207-212 of service.go) that only guards "pending" and "processing" as
// authoritative DB states. The value "rejected" falls through to the default
// branch, which calls DeriveProcessingState(runs). When the item has 7
// completed pipeline_runs (all stages done), DeriveProcessingState returns
// PROCESSING_STATE_COMPLETED — silently discarding the authoritative "rejected"
// status from the sources table.
//
// Secondary bug: The counter switch (lines 222-231) has no case for
// PROCESSING_STATE_REJECTED, so a correctly-mapped REJECTED item would land in
// the default branch (totalPending) rather than totalFailed.
//
// This test MUST FAIL on unpatched code because:
//   - The switch lets "rejected" fall through to DeriveProcessingState.
//   - DeriveProcessingState sees 7 completed runs and returns COMPLETED.
//   - The item therefore reports State = PROCESSING_STATE_COMPLETED.
//
// After the fix, "rejected" must be handled explicitly in the state switch
// (mapping to PROCESSING_STATE_REJECTED) and in the counter switch (mapping to
// totalFailed). This test will then pass.
func TestGetConversationProcessingStatus_RejectedItemNotMaskedAsCompleted(t *testing.T) {
	// Scenario: a single conversation item whose source was permanently rejected
	// (e.g. with failure_category="timeout_heartbeat"). The pipeline_runs table
	// contains 7 completed runs from before the rejection was recorded, which is
	// the typical state after the source is marked rejected post-processing.
	rejectedSourceID := int64(5648)

	items := []ConversationItem{
		{
			ConversationID: "conv-rejected-bug",
			ContentID:      "content-rejected",
			SourceID:       &rejectedSourceID,
			TenantID:       "tenant-test",
		},
	}

	mockRepo := &MockRepository{
		GetConversationFunc: func(ctx context.Context, tenantID, conversationID string) (*ConversationDetail, error) {
			return &ConversationDetail{
				ID:       "conv-rejected-bug",
				TenantID: tenantID,
				Topic:    "Rejected Item Bug Repro",
				Items:    items,
			}, nil
		},
	}

	pipelineRepo := &MockPipelineRepository{
		// GetSourceByContentIDFunc returns the source record with
		// processing_status="rejected". The current handler switch only guards
		// "pending" and "processing" — "rejected" falls through to
		// DeriveProcessingState, masking the authoritative DB state.
		GetSourceByContentIDFunc: func(ctx context.Context, contentID string) (*pipeline.PendingSource, error) {
			return &pipeline.PendingSource{
				ID:               rejectedSourceID,
				ContentID:        contentID,
				ProcessingStatus: "rejected",
			}, nil
		},

		// ListSourceHistoryFunc returns 7 completed pipeline runs — all stages
		// finished. This is what DeriveProcessingState sees and incorrectly uses
		// to conclude COMPLETED, discarding the authoritative "rejected" status.
		ListSourceHistoryFunc: func(ctx context.Context, sourceID int64, stageFilter string) ([]pipeline.PipelineRun, error) {
			return []pipeline.PipelineRun{
				makeCompletedRun(sourceID, "parse", 0, 0),
				makeTriageRunWithContribution(sourceID, 100, 30, "MEDIUM", "relevant"),
				makeCompletedRun(sourceID, "segment", 0, 0),
				makeCompletedRun(sourceID, "extract_ner", 200, 80),
				makeCompletedRun(sourceID, "analyze", 300, 100),
				makeCompletedRun(sourceID, "persist", 0, 0),
				makeCompletedRun(sourceID, "embed", 0, 0),
			}, nil
		},
	}

	svc := NewServiceWithPipeline(mockRepo, pipelineRepo, testLogger())

	req := &conversationv1.GetConversationProcessingStatusRequest{
		ConversationId: "conv-rejected-bug",
	}

	resp, err := svc.GetConversationProcessingStatus(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Items, 1, "conversation must have exactly 1 item")

	item := resp.Items[0]
	assert.Equal(t, "content-rejected", item.ContentId)

	// The source is permanently rejected: processing_status="rejected" in the DB.
	// The 7 completed pipeline_runs are from the processing cycle that ended in rejection.
	// Correct behaviour: the item must be reported as REJECTED, not COMPLETED.
	// Bug behaviour (current code): the item is reported as COMPLETED because
	// the switch lets "rejected" fall through to DeriveProcessingState, which
	// sees all-completed runs and returns PROCESSING_STATE_COMPLETED.
	assert.Equal(t, contentv1.ProcessingState_PROCESSING_STATE_REJECTED, item.State,
		"rejected item must be reported as PROCESSING_STATE_REJECTED, not COMPLETED: "+
			"the switch in GetConversationProcessingStatus (service.go:207) only guards "+
			"'pending' and 'processing'; 'rejected' falls through to DeriveProcessingState "+
			"which incorrectly returns COMPLETED when all pipeline_runs are completed")

	// Aggregate counters must reflect the rejected state.
	// A rejected item is a terminal failure — it must increment Failed, not Completed.
	// Secondary bug: the counter switch (service.go:222) has no PROCESSING_STATE_REJECTED
	// case, so even if state were correctly set to REJECTED, it would land in the
	// default branch (totalPending) rather than totalFailed.
	assert.Equal(t, int32(1), resp.TotalItems)
	assert.Equal(t, int32(0), resp.Completed,
		"completed count must be 0 — the item was rejected, not completed")
	assert.Equal(t, int32(0), resp.Pending,
		"pending count must be 0 — a rejected item is terminal, not pending")
	assert.Equal(t, int32(0), resp.Processing,
		"processing count must be 0 — the item is not in-flight")
	assert.GreaterOrEqual(t, resp.Failed, int32(1),
		"failed count must be >= 1 — rejected items are a terminal failure state "+
			"and must be counted under Failed in the absence of a dedicated Rejected counter")
}
