package main

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mapGRPCError translates a gRPC error into an MCP CallToolResult with IsError set.
// Each status code returns a message with LLM-friendly guidance on what to do next.
func mapGRPCError(err error) *mcp.CallToolResult {
	st, ok := status.FromError(err)
	if !ok {
		return mcp.NewToolResultError("Internal error: " + err.Error())
	}

	switch st.Code() {
	case codes.NotFound:
		msg := st.Message()
		return mcp.NewToolResultError(
			fmt.Sprintf("No content found with ID '%s'. Try penfold_search to find the right ID.", msg),
		)
	case codes.Unavailable:
		return mcp.NewToolResultError(
			"Penfold backend is temporarily unavailable. Use penfold_health to check system status, or try again in a moment.",
		)
	case codes.InvalidArgument:
		return mcp.NewToolResultError(
			fmt.Sprintf("Invalid parameter: %s. Check the tool description for expected formats.", st.Message()),
		)
	case codes.PermissionDenied:
		return mcp.NewToolResultError(
			"Permission denied. Check tenant configuration.",
		)
	case codes.DeadlineExceeded:
		return mcp.NewToolResultError(
			"Request timed out. The operation may still be running. Try again or check penfold_health.",
		)
	case codes.Internal:
		return mcp.NewToolResultError(
			"An internal error occurred. Check penfold_health for system status.",
		)
	default:
		return mcp.NewToolResultError(
			fmt.Sprintf("Error (%s): %s", st.Code(), st.Message()),
		)
	}
}
