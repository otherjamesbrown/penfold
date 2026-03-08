package pipelineservice

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
)

// TestGetOperationalConfig_NilDB tests that nil DB returns Unavailable.
func TestGetOperationalConfig_NilDB(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.GetOperationalConfig(ctx, &pipelinev1.GetOperationalConfigRequest{Key: "email.inbound_whitelist"})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestGetOperationalConfig_EmptyKey tests that an empty key returns InvalidArgument.
func TestGetOperationalConfig_EmptyKey(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.GetOperationalConfig(ctx, &pipelinev1.GetOperationalConfigRequest{Key: ""})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "key is required")
}

// TestSetOperationalConfig_NilDB tests that nil DB returns Unavailable.
func TestSetOperationalConfig_NilDB(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.SetOperationalConfig(ctx, &pipelinev1.SetOperationalConfigRequest{
		Key:   "email.inbound_whitelist",
		Value: `["james@brown.chat"]`,
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestSetOperationalConfig_EmptyKey tests that an empty key returns InvalidArgument.
func TestSetOperationalConfig_EmptyKey(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.SetOperationalConfig(ctx, &pipelinev1.SetOperationalConfigRequest{
		Key:   "",
		Value: "some-value",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.InvalidArgument, st.Code())
	assert.Contains(t, st.Message(), "key is required")
}

// TestGetOperationalConfigResponse_Structure tests that the response proto has expected fields.
func TestGetOperationalConfigResponse_Structure(t *testing.T) {
	resp := &pipelinev1.GetOperationalConfigResponse{
		Key:   "email.inbound_whitelist",
		Value: `["james@brown.chat","jabrown@akamai.com"]`,
	}

	assert.Equal(t, "email.inbound_whitelist", resp.Key)
	assert.Equal(t, `["james@brown.chat","jabrown@akamai.com"]`, resp.Value)
}

// TestSetOperationalConfigResponse_Structure tests that the response proto has expected fields.
func TestSetOperationalConfigResponse_Structure(t *testing.T) {
	resp := &pipelinev1.SetOperationalConfigResponse{
		Updated: true,
	}

	assert.True(t, resp.Updated)
}

// TestSetOperationalConfig_ValidInput_ReachesDB tests validation passes before DB access.
func TestSetOperationalConfig_ValidInput_ReachesDB(t *testing.T) {
	ctx := context.Background()

	// Service with nil DB — validation passes, then fails at DB access (Unavailable not InvalidArgument)
	svc := NewService(nil, testLogger(), nil, nil, "default")

	resp, err := svc.SetOperationalConfig(ctx, &pipelinev1.SetOperationalConfigRequest{
		Key:   "email.inbound_whitelist",
		Value: `["james@brown.chat"]`,
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	st, ok := status.FromError(err)
	require.True(t, ok)
	// Should be Unavailable (DB not configured), not InvalidArgument (validation failed)
	assert.Equal(t, codes.Unavailable, st.Code())
}
