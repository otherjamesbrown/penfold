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
