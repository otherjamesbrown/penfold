package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// grpcHandler creates an MCP tool handler that forwards calls to a gRPC method.
// It unmarshals JSON arguments into a proto request, injects tenant metadata,
// calls the gRPC method, and formats the response via the provided ResponseFormatter.
func grpcHandler[Req proto.Message, Resp proto.Message](
	newReq func() Req,
	call func(ctx context.Context, req Req, opts ...grpc.CallOption) (Resp, error),
	format ResponseFormatter,
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
		unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := unmarshaler.Unmarshal(argsJSON, req); err != nil {
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
			return mapGRPCError(err), nil
		}

		return format(resp)
	}
}
