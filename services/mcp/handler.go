package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// grpcHandler creates an MCP tool handler that forwards calls to a gRPC method.
// It unmarshals JSON arguments into a proto request, injects tenant metadata,
// calls the gRPC method, and formats the response.
func grpcHandler[Req proto.Message, Resp proto.Message](
	newReq func() Req,
	call func(ctx context.Context, req Req, opts ...grpc.CallOption) (Resp, error),
	format func(Resp) (*mcp.CallToolResult, error),
) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		req := newReq()

		// Unmarshal MCP arguments into proto message.
		// Inject tenant ID into arguments so it's set on the proto request.
		args := request.GetArguments()
		if args == nil {
			args = map[string]any{}
		}
		if tenantID := os.Getenv("PENFOLD_TENANT_ID"); tenantID != "" {
			args["tenantId"] = tenantID
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return mcp.NewToolResultError("Invalid parameters: " + err.Error()), nil
		}
		if err := protojson.Unmarshal(argsJSON, req); err != nil {
			return mcp.NewToolResultError("Invalid parameters: " + err.Error()), nil
		}

		// Also set tenant ID as gRPC metadata
		if tenantID := os.Getenv("PENFOLD_TENANT_ID"); tenantID != "" {
			md := metadata.New(map[string]string{"x-tenant-id": tenantID})
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		// Call the gRPC method
		resp, err := call(ctx, req)
		if err != nil {
			return mcp.NewToolResultError(mapGRPCError(err)), nil
		}

		return format(resp)
	}
}

// defaultFormatter marshals a proto response to JSON text.
func defaultFormatter[Resp proto.Message](resp Resp) (*mcp.CallToolResult, error) {
	data, err := protojson.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("format response: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

// mapGRPCError translates gRPC status codes to human-readable MCP error text.
func mapGRPCError(err error) string {
	st, ok := status.FromError(err)
	if !ok {
		return "Internal error: " + err.Error()
	}
	switch st.Code() {
	case codes.NotFound:
		return "Not found: " + st.Message()
	case codes.Unavailable:
		return "Penfold backend is temporarily unavailable."
	case codes.InvalidArgument:
		return "Invalid parameter: " + st.Message()
	default:
		return fmt.Sprintf("Internal error: %s", st.Code())
	}
}
