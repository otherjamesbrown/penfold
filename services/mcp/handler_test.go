package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapGRPCError_NotFound(t *testing.T) {
	err := status.Error(codes.NotFound, "document xyz not found")
	result := mapGRPCError(err)
	assert.Equal(t, "Not found: document xyz not found", result)
}

func TestMapGRPCError_Unavailable(t *testing.T) {
	err := status.Error(codes.Unavailable, "connection refused")
	result := mapGRPCError(err)
	assert.Equal(t, "Penfold backend is temporarily unavailable.", result)
}

func TestMapGRPCError_InvalidArgument(t *testing.T) {
	err := status.Error(codes.InvalidArgument, "query cannot be empty")
	result := mapGRPCError(err)
	assert.Equal(t, "Invalid parameter: query cannot be empty", result)
}

func TestMapGRPCError_Internal(t *testing.T) {
	err := status.Error(codes.Internal, "database error")
	result := mapGRPCError(err)
	assert.Equal(t, "Internal error: Internal", result)
}
