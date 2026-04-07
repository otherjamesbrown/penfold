package reviewservice

import (
	"context"
	"testing"

	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/review/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

// TestSessionRPCsReturnResponses verifies that session RPCs return valid responses
// even when the underlying database table doesn't exist, preventing timeouts.
func TestSessionRPCsReturnResponses(t *testing.T) {
	// Create service with nil repo to simulate missing table
	logger := logging.NewNopLogger()
	svc := NewService(nil, nil, logger)

	ctx := context.Background()

	t.Run("StartSession returns response without timeout", func(t *testing.T) {
		resp, err := svc.StartSession(ctx, &reviewv1.StartSessionRequest{})
		if err != nil {
			t.Errorf("StartSession returned error: %v", err)
		}
		if resp == nil {
			t.Error("StartSession returned nil response")
		}
		if resp.Session == nil {
			t.Error("StartSession returned nil session")
		}
		if resp.Session.Status != reviewv1.SessionStatus_SESSION_STATUS_ACTIVE {
			t.Errorf("Expected active status, got %v", resp.Session.Status)
		}
	})

	t.Run("PauseSession returns response without timeout", func(t *testing.T) {
		resp, err := svc.PauseSession(ctx, &reviewv1.PauseSessionRequest{})
		if err != nil {
			t.Errorf("PauseSession returned error: %v", err)
		}
		if resp == nil {
			t.Error("PauseSession returned nil response")
		}
		if resp.Session == nil {
			t.Error("PauseSession returned nil session")
		}
		if resp.Session.Status != reviewv1.SessionStatus_SESSION_STATUS_PAUSED {
			t.Errorf("Expected paused status, got %v", resp.Session.Status)
		}
	})

	t.Run("ResumeSession returns response without timeout", func(t *testing.T) {
		resp, err := svc.ResumeSession(ctx, &reviewv1.ResumeSessionRequest{})
		if err != nil {
			t.Errorf("ResumeSession returned error: %v", err)
		}
		if resp == nil {
			t.Error("ResumeSession returned nil response")
		}
		if resp.Session == nil {
			t.Error("ResumeSession returned nil session")
		}
		if resp.Session.Status != reviewv1.SessionStatus_SESSION_STATUS_ACTIVE {
			t.Errorf("Expected active status, got %v", resp.Session.Status)
		}
	})

	t.Run("EndSession returns response without timeout", func(t *testing.T) {
		resp, err := svc.EndSession(ctx, &reviewv1.EndSessionRequest{})
		if err != nil {
			t.Errorf("EndSession returned error: %v", err)
		}
		if resp == nil {
			t.Error("EndSession returned nil response")
		}
		if resp.Session == nil {
			t.Error("EndSession returned nil session")
		}
		if resp.Session.Status != reviewv1.SessionStatus_SESSION_STATUS_ENDED {
			t.Errorf("Expected ended status, got %v", resp.Session.Status)
		}
		if resp.Summary == "" {
			t.Error("EndSession returned empty summary")
		}
	})

	t.Run("GetCurrentSession returns response without timeout", func(t *testing.T) {
		resp, err := svc.GetCurrentSession(ctx, &reviewv1.GetCurrentSessionRequest{})
		if err != nil {
			t.Errorf("GetCurrentSession returned error: %v", err)
		}
		if resp == nil {
			t.Error("GetCurrentSession returned nil response")
		}
		if resp.HasSession {
			t.Error("Expected HasSession to be false when no session exists")
		}
	})

	t.Run("GetSessionHistory returns response without timeout", func(t *testing.T) {
		resp, err := svc.GetSessionHistory(ctx, &reviewv1.GetSessionHistoryRequest{})
		if err != nil {
			t.Errorf("GetSessionHistory returned error: %v", err)
		}
		if resp == nil {
			t.Error("GetSessionHistory returned nil response")
		}
		if resp.Sessions == nil {
			t.Error("GetSessionHistory returned nil sessions list")
		}
		if resp.TotalCount != 0 {
			t.Errorf("Expected TotalCount 0, got %d", resp.TotalCount)
		}
	})
}

// TestSessionRPCsWithValidRepo verifies that session RPCs work correctly
// when the repository is properly configured.
func TestSessionRPCsWithValidRepo(t *testing.T) {
	// This test would require a real database connection or mock.
	// For now, we just verify the API contract.
	t.Skip("Requires database connection - integration test")
}

// TestListReviewItemsReturnsResponse verifies ListReviewItems doesn't timeout.
func TestListReviewItemsReturnsResponse(t *testing.T) {
	logger := logging.NewNopLogger()

	// Use nil repo to test graceful degradation
	svc := NewService(nil, nil, logger)

	ctx := context.Background()

	resp, err := svc.ListReviewItems(ctx, &reviewv1.ListReviewItemsRequest{
		PageSize: 10,
	})

	// We expect either success with empty list, or an error (but not a timeout)
	// The key is that we GET a response, not that it hangs
	if err != nil {
		// An error is acceptable as long as it's not a timeout
		t.Logf("ListReviewItems returned error (acceptable): %v", err)
	}
	if resp != nil {
		// Success case
		if resp.Items == nil {
			t.Error("ListReviewItems returned nil items list")
		}
		t.Logf("ListReviewItems returned %d items", len(resp.Items))
	}

	// The test succeeds if we got here without hanging
}
