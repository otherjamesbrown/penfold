package classifyservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	classifyv1 "github.com/otherjamesbrown/penfold/api/proto/classify/v1"
	"github.com/otherjamesbrown/penfold/pkg/classify"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// ============ Mock Repository ============

// MockSuggestionRepository is a mock implementation of the classification suggestion repository.
type MockSuggestionRepository struct {
	ListPendingFunc  func(ctx context.Context) ([]*classify.ClassificationSuggestion, error)
	UpdateStatusFunc func(ctx context.Context, id int, status string) error
}

func (m *MockSuggestionRepository) ListPending(ctx context.Context) ([]*classify.ClassificationSuggestion, error) {
	if m.ListPendingFunc != nil {
		return m.ListPendingFunc(ctx)
	}
	return nil, errors.New("ListPendingFunc not implemented")
}

func (m *MockSuggestionRepository) UpdateStatus(ctx context.Context, id int, status string) error {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, id, status)
	}
	return errors.New("UpdateStatusFunc not implemented")
}

// ============ Test Helpers ============

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error" // Quiet logging for tests
	return logging.NewLogger(cfg)
}

// ============ ListSuggestions Tests ============

func TestListSuggestions_ReturnsPending(t *testing.T) {
	// Create test data
	now := time.Now()

	mockRepo := &MockSuggestionRepository{
		ListPendingFunc: func(ctx context.Context) ([]*classify.ClassificationSuggestion, error) {
			return []*classify.ClassificationSuggestion{
				{
					ID:           1,
					ContentID:    "content-abc-123",
					SourceSystem: "gmail",
					Pattern:      "from:.*@acme\\.com",
					Confidence:   0.92,
					Status:       "pending",
					CreatedAt:    now.Add(-2 * time.Hour),
				},
				{
					ID:           2,
					ContentID:    "content-def-456",
					SourceSystem: "salesforce",
					Pattern:      "subject:.*\\[CUSTOMER\\].*",
					Confidence:   0.85,
					Status:       "pending",
					CreatedAt:    now.Add(-1 * time.Hour),
				},
			}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.ListSuggestionsRequest{}

	resp, err := svc.ListSuggestions(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Suggestions, 2)

	// Verify first suggestion mapping
	sug1 := resp.Suggestions[0]
	assert.Equal(t, int32(1), sug1.Id)
	assert.Equal(t, "content-abc-123", sug1.ContentId)
	assert.Equal(t, "gmail", sug1.SourceSystem)
	assert.Equal(t, "from:.*@acme\\.com", sug1.Pattern)
	assert.InDelta(t, 0.92, sug1.Confidence, 0.001)
	assert.Equal(t, "pending", sug1.Status)
	assert.NotNil(t, sug1.CreatedAt)

	// Verify second suggestion mapping
	sug2 := resp.Suggestions[1]
	assert.Equal(t, int32(2), sug2.Id)
	assert.Equal(t, "content-def-456", sug2.ContentId)
	assert.Equal(t, "salesforce", sug2.SourceSystem)
	assert.InDelta(t, 0.85, sug2.Confidence, 0.001)
}

func TestListSuggestions_EmptyResult(t *testing.T) {
	mockRepo := &MockSuggestionRepository{
		ListPendingFunc: func(ctx context.Context) ([]*classify.ClassificationSuggestion, error) {
			return []*classify.ClassificationSuggestion{}, nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.ListSuggestionsRequest{}

	resp, err := svc.ListSuggestions(context.Background(), req)

	require.NoError(t, err, "Empty result should not return an error")
	require.NotNil(t, resp)
	assert.Empty(t, resp.Suggestions, "Should return empty list, not nil")
}

func TestListSuggestions_RepositoryError(t *testing.T) {
	mockRepo := &MockSuggestionRepository{
		ListPendingFunc: func(ctx context.Context) ([]*classify.ClassificationSuggestion, error) {
			return nil, errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.ListSuggestionsRequest{}

	resp, err := svc.ListSuggestions(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to list suggestions")
}

// ============ ApproveSuggestion Tests ============

func TestApproveSuggestion_Success(t *testing.T) {
	var capturedID int
	var capturedStatus string

	mockRepo := &MockSuggestionRepository{
		UpdateStatusFunc: func(ctx context.Context, id int, status string) error {
			capturedID = id
			capturedStatus = status
			return nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.ApproveSuggestionRequest{
		Id: 42,
	}

	resp, err := svc.ApproveSuggestion(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)

	// Verify UpdateStatus was called with correct parameters
	assert.Equal(t, 42, capturedID)
	assert.Equal(t, "approved", capturedStatus)
}

func TestApproveSuggestion_InvalidID(t *testing.T) {
	mockRepo := &MockSuggestionRepository{
		UpdateStatusFunc: func(ctx context.Context, id int, status string) error {
			return errors.New("suggestion with id 999 not found")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.ApproveSuggestionRequest{
		Id: 999,
	}

	resp, err := svc.ApproveSuggestion(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "suggestion not found")
}

func TestApproveSuggestion_RepositoryError(t *testing.T) {
	mockRepo := &MockSuggestionRepository{
		UpdateStatusFunc: func(ctx context.Context, id int, status string) error {
			return errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.ApproveSuggestionRequest{
		Id: 42,
	}

	resp, err := svc.ApproveSuggestion(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to approve suggestion")
}

// ============ RejectSuggestion Tests ============

func TestRejectSuggestion_Success(t *testing.T) {
	var capturedID int
	var capturedStatus string

	mockRepo := &MockSuggestionRepository{
		UpdateStatusFunc: func(ctx context.Context, id int, status string) error {
			capturedID = id
			capturedStatus = status
			return nil
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.RejectSuggestionRequest{
		Id: 24,
	}

	resp, err := svc.RejectSuggestion(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)

	// Verify UpdateStatus was called with correct parameters
	assert.Equal(t, 24, capturedID)
	assert.Equal(t, "rejected", capturedStatus)
}

func TestRejectSuggestion_InvalidID(t *testing.T) {
	mockRepo := &MockSuggestionRepository{
		UpdateStatusFunc: func(ctx context.Context, id int, status string) error {
			return errors.New("suggestion with id 777 not found")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.RejectSuggestionRequest{
		Id: 777,
	}

	resp, err := svc.RejectSuggestion(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "suggestion not found")
}

func TestRejectSuggestion_RepositoryError(t *testing.T) {
	mockRepo := &MockSuggestionRepository{
		UpdateStatusFunc: func(ctx context.Context, id int, status string) error {
			return errors.New("database connection failed")
		},
	}

	svc := NewService(mockRepo, testLogger())

	req := &classifyv1.RejectSuggestionRequest{
		Id: 24,
	}

	resp, err := svc.RejectSuggestion(context.Background(), req)

	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "failed to reject suggestion")
}

// ============ Interface Compliance Test ============

func TestServiceImplementsInterface(t *testing.T) {
	// This will fail to compile until the proto is generated and service is implemented
	var _ classifyv1.ClassificationSuggestionServiceServer = (*Service)(nil)
}
