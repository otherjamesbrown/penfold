package watchlistservice

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	watchlistv1 "github.com/otherjamesbrown/penfold/api/proto/watchlist/v1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func TestService_AddWatchItem_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	tests := []struct {
		name        string
		req         *watchlistv1.AddWatchItemRequest
		expectCode  codes.Code
		expectError string
	}{
		{
			name: "missing_user_id",
			req: &watchlistv1.AddWatchItemRequest{
				TenantId: "test",
				UserId:   "",
			},
			expectCode:  codes.InvalidArgument,
			expectError: "user_id is required",
		},
		{
			name: "missing_both_targets",
			req: &watchlistv1.AddWatchItemRequest{
				TenantId: "test",
				UserId:   "user123",
				// Neither assertion_root_id nor project_id set
			},
			expectCode:  codes.InvalidArgument,
			expectError: "at least one of assertion_root_id or project_id must be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.AddWatchItem(context.Background(), tt.req)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Expected gRPC status error, got: %v", err)
			}
			if st.Code() != tt.expectCode {
				t.Errorf("Expected code %v, got %v", tt.expectCode, st.Code())
			}
			if st.Message() != tt.expectError {
				t.Errorf("Expected message %q, got %q", tt.expectError, st.Message())
			}
		})
	}
}

func TestService_RemoveWatchItem_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	req := &watchlistv1.RemoveWatchItemRequest{
		TenantId: "test",
		Id:       0, // Invalid
	}

	_, err := svc.RemoveWatchItem(context.Background(), req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", st.Code())
	}
	if st.Message() != "id is required" {
		t.Errorf("Expected 'id is required', got %q", st.Message())
	}
}

func TestService_ListWatchItems_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	req := &watchlistv1.ListWatchItemsRequest{
		TenantId: "test",
		UserId:   "", // Invalid
	}

	_, err := svc.ListWatchItems(context.Background(), req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", st.Code())
	}
	if st.Message() != "user_id is required" {
		t.Errorf("Expected 'user_id is required', got %q", st.Message())
	}
}

func TestService_UpdateWatchItem_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	req := &watchlistv1.UpdateWatchItemRequest{
		TenantId: "test",
		Id:       0, // Invalid
		Notes:    "updated notes",
	}

	_, err := svc.UpdateWatchItem(context.Background(), req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", st.Code())
	}
	if st.Message() != "id is required" {
		t.Errorf("Expected 'id is required', got %q", st.Message())
	}
}

func TestService_SetTrust_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	tests := []struct {
		name        string
		req         *watchlistv1.SetTrustRequest
		expectCode  codes.Code
		expectError string
	}{
		{
			name: "missing_person_id",
			req: &watchlistv1.SetTrustRequest{
				TenantId:   "test",
				PersonId:   0,
				TrustLevel: 3,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "person_id is required",
		},
		{
			name: "trust_level_too_low",
			req: &watchlistv1.SetTrustRequest{
				TenantId:   "test",
				PersonId:   123,
				TrustLevel: -1,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "trust_level must be between 0 and 5, got -1",
		},
		{
			name: "trust_level_too_high",
			req: &watchlistv1.SetTrustRequest{
				TenantId:   "test",
				PersonId:   123,
				TrustLevel: 6,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "trust_level must be between 0 and 5, got 6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SetTrust(context.Background(), tt.req)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Expected gRPC status error, got: %v", err)
			}
			if st.Code() != tt.expectCode {
				t.Errorf("Expected code %v, got %v", tt.expectCode, st.Code())
			}
			if st.Message() != tt.expectError {
				t.Errorf("Expected message %q, got %q", tt.expectError, st.Message())
			}
		})
	}
}

func TestService_SetSeniority_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	tests := []struct {
		name        string
		req         *watchlistv1.SetSeniorityRequest
		expectCode  codes.Code
		expectError string
	}{
		{
			name: "missing_person_id",
			req: &watchlistv1.SetSeniorityRequest{
				TenantId:      "test",
				PersonId:      0,
				SeniorityTier: 5,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "person_id is required",
		},
		{
			name: "seniority_tier_too_low",
			req: &watchlistv1.SetSeniorityRequest{
				TenantId:      "test",
				PersonId:      123,
				SeniorityTier: 0,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "seniority_tier must be between 1 and 7, got 0",
		},
		{
			name: "seniority_tier_too_high",
			req: &watchlistv1.SetSeniorityRequest{
				TenantId:      "test",
				PersonId:      123,
				SeniorityTier: 8,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "seniority_tier must be between 1 and 7, got 8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SetSeniority(context.Background(), tt.req)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Expected gRPC status error, got: %v", err)
			}
			if st.Code() != tt.expectCode {
				t.Errorf("Expected code %v, got %v", tt.expectCode, st.Code())
			}
			if st.Message() != tt.expectError {
				t.Errorf("Expected message %q, got %q", tt.expectError, st.Message())
			}
		})
	}
}

func TestService_GetBriefingAssertions_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	tests := []struct {
		name        string
		req         *watchlistv1.GetBriefingAssertionsRequest
		expectCode  codes.Code
		expectError string
	}{
		{
			name: "missing_user_id",
			req: &watchlistv1.GetBriefingAssertionsRequest{
				TenantId:  "test",
				UserId:    "",
				ProjectId: 1,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "user_id is required",
		},
		{
			name: "missing_project_id",
			req: &watchlistv1.GetBriefingAssertionsRequest{
				TenantId:  "test",
				UserId:    "user123",
				ProjectId: 0,
			},
			expectCode:  codes.InvalidArgument,
			expectError: "project_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetBriefingAssertions(context.Background(), tt.req)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Expected gRPC status error, got: %v", err)
			}
			if st.Code() != tt.expectCode {
				t.Errorf("Expected code %v, got %v", tt.expectCode, st.Code())
			}
			if st.Message() != tt.expectError {
				t.Errorf("Expected message %q, got %q", tt.expectError, st.Message())
			}
		})
	}
}

func TestService_GetSeniorityEscalations_Validation(t *testing.T) {
	logger := logging.NewLogger(&logging.Config{Level: logging.LevelDebug})
	svc := NewService(nil, logger)

	req := &watchlistv1.GetSeniorityEscalationsRequest{
		TenantId: "test",
		SourceId: 0, // Invalid
	}

	_, err := svc.GetSeniorityEscalations(context.Background(), req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Expected gRPC status error, got: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument, got %v", st.Code())
	}
	if st.Message() != "source_id is required" {
		t.Errorf("Expected 'source_id is required', got %q", st.Message())
	}
}

func TestConversionHelpers(t *testing.T) {
	// Test nil handling
	if watchItemToProto(nil) != nil {
		t.Error("watchItemToProto(nil) should return nil")
	}
	if personTrustToProto(nil) != nil {
		t.Error("personTrustToProto(nil) should return nil")
	}
	if personSeniorityToProto(nil) != nil {
		t.Error("personSeniorityToProto(nil) should return nil")
	}
	if briefingAssertionToProto(nil) != nil {
		t.Error("briefingAssertionToProto(nil) should return nil")
	}
	if seniorityEscalationToProto(nil) != nil {
		t.Error("seniorityEscalationToProto(nil) should return nil")
	}
}
