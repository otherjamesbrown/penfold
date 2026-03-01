package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// withLogging wraps a tool handler with structured logging.
// It measures duration, captures the result status, and logs after the handler returns.
func withLogging(toolName, toolsetName string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		result, err := handler(ctx, req)

		duration := time.Since(start)
		attrs := []slog.Attr{
			slog.String("tool", toolName),
			slog.String("toolset", toolsetName),
			slog.Int64("duration_ms", duration.Milliseconds()),
		}

		// Extract session ID if available.
		if session := server.ClientSessionFromContext(ctx); session != nil {
			attrs = append(attrs, slog.String("session_id", session.SessionID()))
		}

		if err != nil {
			attrs = append(attrs,
				slog.String("error", err.Error()),
			)
			slog.LogAttrs(ctx, slog.LevelWarn, "tool_call_error", attrs...)
			return result, err
		}

		// Check if the result is an error result (isError flag set by MCP).
		if result != nil && result.IsError {
			// Extract error text from first content item if available.
			errText := ""
			if len(result.Content) > 0 {
				if tc, ok := result.Content[0].(mcp.TextContent); ok {
					errText = tc.Text
				}
			}
			attrs = append(attrs, slog.String("error", errText))
			slog.LogAttrs(ctx, slog.LevelWarn, "tool_call_error", attrs...)
			return result, nil
		}

		// Estimate response size from content.
		var responseBytes int
		for _, c := range result.Content {
			if tc, ok := c.(mcp.TextContent); ok {
				responseBytes += len(tc.Text)
			}
		}
		attrs = append(attrs, slog.Int("response_bytes", responseBytes))

		slog.LogAttrs(ctx, slog.LevelInfo, "tool_call", attrs...)
		return result, nil
	}
}
