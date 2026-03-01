package main

import (
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// extractText retrieves the text from the first content item of a CallToolResult.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return tc.Text
}

func TestMapGRPCError_NotFound(t *testing.T) {
	err := status.Error(codes.NotFound, "document xyz not found")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "document xyz not found")
	assert.Contains(t, text, "Try penfold_search")
}

func TestMapGRPCError_Unavailable(t *testing.T) {
	err := status.Error(codes.Unavailable, "connection refused")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "temporarily unavailable")
	assert.Contains(t, text, "penfold_health")
}

func TestMapGRPCError_InvalidArgument(t *testing.T) {
	err := status.Error(codes.InvalidArgument, "query cannot be empty")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "query cannot be empty")
	assert.Contains(t, text, "Invalid parameter")
}

func TestMapGRPCError_DeadlineExceeded(t *testing.T) {
	err := status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "may still be running")
	assert.Contains(t, text, "penfold_health")
}

func TestMapGRPCError_PermissionDenied(t *testing.T) {
	err := status.Error(codes.PermissionDenied, "access denied")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "Permission denied")
	assert.Contains(t, text, "tenant")
}

func TestMapGRPCError_Internal(t *testing.T) {
	err := status.Error(codes.Internal, "database error")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "An internal error occurred")
	assert.Contains(t, text, "penfold_health")
}

func TestMapGRPCError_ReturnsIsError(t *testing.T) {
	testCases := []codes.Code{
		codes.NotFound,
		codes.Unavailable,
		codes.InvalidArgument,
		codes.PermissionDenied,
		codes.DeadlineExceeded,
		codes.Internal,
		codes.Unimplemented,
	}
	for _, code := range testCases {
		err := status.Error(code, "some message")
		result := mapGRPCError(err)
		assert.True(t, result.IsError, "expected IsError=true for code %s", code)
	}
}

func TestMapGRPCError_UnknownCode(t *testing.T) {
	err := status.Error(codes.Unimplemented, "not yet built")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "Unimplemented")
	assert.Contains(t, text, "not yet built")
}

func TestMapGRPCError_NonGRPC(t *testing.T) {
	err := errors.New("something went wrong")
	result := mapGRPCError(err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
	text := extractText(t, result)
	assert.Contains(t, text, "Internal error:")
	assert.Contains(t, text, "something went wrong")
}
