package modelservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/otherjamesbrown/penfold/api/proto/aiv1"
	"github.com/otherjamesbrown/penfold/pkg/logging"
)

func testLogger() logging.Logger {
	cfg := logging.DefaultConfig()
	cfg.Level = "error" // Quiet logging for tests
	return logging.NewLogger(cfg)
}

func stringPtr(s string) *string {
	return &s
}

// ============ NewService Tests ============

func TestNewService(t *testing.T) {
	t.Run("creates service with nil client", func(t *testing.T) {
		svc := NewService(nil, testLogger())
		require.NotNil(t, svc)
		assert.Nil(t, svc.aiClient)
	})

	t.Run("creates service with nil logger", func(t *testing.T) {
		svc := NewService(nil, nil)
		require.NotNil(t, svc)
		assert.NotNil(t, svc.logger)
	})
}

// ============ Unavailable Tests (nil client) ============

func TestServiceUnavailable(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	t.Run("ListModels returns Unavailable", func(t *testing.T) {
		resp, err := svc.ListModels(ctx, &aiv1.ListModelsRequest{})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
		assert.Contains(t, st.Message(), "AI service is not configured")
	})

	t.Run("RegisterModel returns Unavailable", func(t *testing.T) {
		resp, err := svc.RegisterModel(ctx, &aiv1.RegisterModelRequest{
			Name:         "test-model",
			Provider:     "ollama",
			ModelName:    "llama3.2",
			Type:         aiv1.ModelType_MODEL_TYPE_LLM,
			Capabilities: []string{"chat"},
		})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("UpdateModel returns Unavailable", func(t *testing.T) {
		resp, err := svc.UpdateModel(ctx, &aiv1.UpdateModelRequest{
			ModelId: "model-123",
		})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("DeleteModel returns Unavailable", func(t *testing.T) {
		resp, err := svc.DeleteModel(ctx, &aiv1.DeleteModelRequest{
			ModelId: "model-123",
		})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("GetRoutingRules returns Unavailable", func(t *testing.T) {
		resp, err := svc.GetRoutingRules(ctx, &aiv1.GetRoutingRulesRequest{})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("UpdateRoutingRule returns Unavailable", func(t *testing.T) {
		resp, err := svc.UpdateRoutingRule(ctx, &aiv1.UpdateRoutingRuleRequest{
			Name:              "test-rule",
			TaskType:          "embedding",
			PreferredModelIds: []string{"model-123"},
		})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})

	t.Run("GetModelStatus returns Unavailable", func(t *testing.T) {
		resp, err := svc.GetModelStatus(ctx, &aiv1.GetModelStatusRequest{})
		assert.Nil(t, resp)
		require.Error(t, err)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.Unavailable, st.Code())
	})
}

// ============ Validation Tests ============

func TestRegisterModelValidation(t *testing.T) {
	// Note: With nil client, validation happens after availability check.
	// These tests verify validation logic is correct by testing the error messages
	// when we bypass the availability check using a mock.
	// For now, we can only test that the service returns proper errors
	// for validation when the client check fails.

	// To properly test validation, we would need a mock AI client.
	// Since the AI client is a concrete type without an interface,
	// we'll skip detailed validation tests here but document expected behavior.

	t.Run("documentation: name is required", func(t *testing.T) {
		// RegisterModel should return InvalidArgument if name is empty
		// Error: "name is required"
	})

	t.Run("documentation: provider is required", func(t *testing.T) {
		// RegisterModel should return InvalidArgument if provider is empty
		// Error: "provider is required"
	})

	t.Run("documentation: model_name is required", func(t *testing.T) {
		// RegisterModel should return InvalidArgument if model_name is empty
		// Error: "model_name is required"
	})

	t.Run("documentation: type is required", func(t *testing.T) {
		// RegisterModel should return InvalidArgument if type is unspecified
		// Error: "type is required"
	})

	t.Run("documentation: at least one capability is required", func(t *testing.T) {
		// RegisterModel should return InvalidArgument if capabilities is empty
		// Error: "at least one capability is required"
	})
}

func TestUpdateModelValidation(t *testing.T) {
	t.Run("documentation: model_id is required", func(t *testing.T) {
		// UpdateModel should return InvalidArgument if model_id is empty
		// Error: "model_id is required"
	})
}

func TestDeleteModelValidation(t *testing.T) {
	t.Run("documentation: model_id is required", func(t *testing.T) {
		// DeleteModel should return InvalidArgument if model_id is empty
		// Error: "model_id is required"
	})
}

func TestUpdateRoutingRuleValidation(t *testing.T) {
	t.Run("documentation: name is required", func(t *testing.T) {
		// UpdateRoutingRule should return InvalidArgument if name is empty
		// Error: "name is required"
	})

	t.Run("documentation: task_type is required", func(t *testing.T) {
		// UpdateRoutingRule should return InvalidArgument if task_type is empty
		// Error: "task_type is required"
	})

	t.Run("documentation: at least one preferred_model_id is required", func(t *testing.T) {
		// UpdateRoutingRule should return InvalidArgument if preferred_model_ids is empty
		// Error: "at least one preferred_model_id is required"
	})
}

// ============ Interface Compliance Test ============

func TestServiceImplementsInterface(t *testing.T) {
	// Verify that Service implements the AICoordinatorServiceServer interface.
	var _ aiv1.AICoordinatorServiceServer = (*Service)(nil)
}

// ============ Model Management Tests (Gateway Feature) ============

// TestGetStageModels_NilDB verifies that when DB is nil, GetStageModels still returns
// all 5 pipeline stages with env/default sources, without attempting DB queries.
func TestGetStageModels_NilDB(t *testing.T) {
	svc := NewService(nil, testLogger())
	// DB is nil by default in NewService

	ctx := context.Background()
	req := &aiv1.GetStageModelsRequest{
		TenantId: stringPtr("default"),
	}

	resp, err := svc.GetStageModels(ctx, req)
	require.NoError(t, err, "GetStageModels should succeed even when DB is nil")
	require.NotNil(t, resp)

	// Should return all 5 stages
	require.Len(t, resp.Stages, 5, "should return 5 pipeline stages")

	// Verify stage names (in order: parse, triage, extract, context, analyze)
	expectedStages := []string{"parse", "triage", "extract", "context", "analyze"}
	for i, expectedStage := range expectedStages {
		assert.Equal(t, expectedStage, resp.Stages[i].StageName, "stage %d should be %s", i, expectedStage)
		assert.NotEmpty(t, resp.Stages[i].ModelName, "stage %s should have a model name", expectedStage)
		assert.Contains(t, []string{"env", "default"}, resp.Stages[i].Source, "source should be env or default when DB is nil")
		assert.NotEmpty(t, resp.Stages[i].Backend, "backend should be set (ollama or gemini)")
	}
}

// TestSetStageModel_Validation verifies that SetStageModel returns InvalidArgument
// for invalid stage names.
func TestSetStageModel_Validation(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	tests := []struct {
		name      string
		stageName string
		wantCode  codes.Code
		wantMsg   string
	}{
		{
			name:      "empty stage name",
			stageName: "",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "stage_name is required",
		},
		{
			name:      "invalid stage name",
			stageName: "invalid_stage",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "invalid stage_name",
		},
		{
			name:      "unknown stage",
			stageName: "unknown",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "invalid stage_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &aiv1.SetStageModelRequest{
				TenantId:  stringPtr("default"),
				StageName: tt.stageName,
				ModelName: "llama3.2",
			}

			resp, err := svc.SetStageModel(ctx, req)
			assert.Nil(t, resp)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok, "error should be a gRPC status")
			assert.Equal(t, tt.wantCode, st.Code(), "should return %s", tt.wantCode)
			assert.Contains(t, st.Message(), tt.wantMsg, "error message should contain %q", tt.wantMsg)
		})
	}
}

// TestSetStageModel_EmptyModel verifies that SetStageModel returns InvalidArgument
// when model_name is empty.
func TestSetStageModel_EmptyModel(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	req := &aiv1.SetStageModelRequest{
		TenantId:  stringPtr("default"),
		StageName: "parse",
		ModelName: "", // Empty model name
	}

	resp, err := svc.SetStageModel(ctx, req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "model_name is required")
}

// TestResetStageModel_Validation verifies that ResetStageModel returns InvalidArgument
// for invalid stage names.
func TestResetStageModel_Validation(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	tests := []struct {
		name      string
		stageName string
		wantCode  codes.Code
		wantMsg   string
	}{
		{
			name:      "empty stage name",
			stageName: "",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "stage_name is required",
		},
		{
			name:      "invalid stage name",
			stageName: "bad_stage",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "invalid stage_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &aiv1.ResetStageModelRequest{
				TenantId:  stringPtr("default"),
				StageName: tt.stageName,
			}

			resp, err := svc.ResetStageModel(ctx, req)
			assert.Nil(t, resp)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Contains(t, st.Message(), tt.wantMsg)
		})
	}
}

// TestGetAvailableModels_NilClient verifies that GetAvailableModels returns Unavailable
// when the AI client is nil.
func TestGetAvailableModels_NilClient(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	req := &aiv1.GetAvailableModelsRequest{
		TenantId: stringPtr("default"),
	}

	resp, err := svc.GetAvailableModels(ctx, req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Contains(t, st.Message(), "AI service is not configured")
}

// TestTestModel_NilClient verifies that TestModel returns Unavailable
// when the AI client is nil.
func TestTestModel_NilClient(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	req := &aiv1.TestModelRequest{
		TenantId:  stringPtr("default"),
		StageName: "parse",
		ModelName: "llama3.2",
	}

	resp, err := svc.TestModel(ctx, req)
	assert.Nil(t, resp)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Contains(t, st.Message(), "AI service is not configured")
}

// TestTestModel_Validation verifies that TestModel returns InvalidArgument
// for invalid inputs.
func TestTestModel_Validation(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()

	tests := []struct {
		name      string
		stageName string
		modelName string
		wantCode  codes.Code
		wantMsg   string
	}{
		{
			name:      "empty stage name",
			stageName: "",
			modelName: "llama3.2",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "stage_name is required",
		},
		{
			name:      "empty model name",
			stageName: "parse",
			modelName: "",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "model_name is required",
		},
		{
			name:      "invalid stage name",
			stageName: "invalid",
			modelName: "llama3.2",
			wantCode:  codes.InvalidArgument,
			wantMsg:   "invalid stage_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &aiv1.TestModelRequest{
				TenantId:  stringPtr("default"),
				StageName: tt.stageName,
				ModelName: tt.modelName,
			}

			resp, err := svc.TestModel(ctx, req)
			assert.Nil(t, resp)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, st.Code())
			assert.Contains(t, st.Message(), tt.wantMsg)
		})
	}
}
