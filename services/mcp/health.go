package main

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
)

// registerHealthTool registers the penfold_health tool globally on the MCP server.
// It is not part of any toolset and is always available.
func registerHealthTool(srv *server.MCPServer, healthClient gatewayv1.GatewayServiceClient) {
	tool := mcp.NewToolWithRawSchema(
		"penfold_health",
		"Check overall system health — gateway, worker, AI coordinator, search index. Call this before enabling toolsets to verify the backend is available.",
		[]byte(`{"type":"object"}`),
	)
	tool.Annotations = mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}

	srv.AddTool(tool, withLogging("penfold_health", "global", func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := healthClient.HealthCheck(ctx, &gatewayv1.HealthCheckRequest{
			IncludeDependencies: true,
		})
		if err != nil {
			return mapGRPCError(err), nil
		}

		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return mcp.NewToolResultError("failed to format health response: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}))
}
