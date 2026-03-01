package main

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	// defaultTruncateLen is the maximum length for string values before truncation.
	defaultTruncateLen = 500
	// truncateMarker is appended to truncated strings.
	truncateMarker = " [truncated — use penfold_get_content_text for full text]"
)

// ResponseFormatter transforms a proto.Message into an MCP tool result.
type ResponseFormatter func(resp proto.Message) (*mcp.CallToolResult, error)

// DefaultFormatter returns a ResponseFormatter that marshals a proto response to
// indented JSON, omitting zero/empty/null fields, and truncates any string
// field longer than 500 characters.
func DefaultFormatter() ResponseFormatter {
	return func(resp proto.Message) (*mcp.CallToolResult, error) {
		data, err := marshalProto(resp)
		if err != nil {
			return nil, err
		}
		data = truncateFields(data, defaultTruncateLen)
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("format response: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// ListFormatter returns a ResponseFormatter like DefaultFormatter but also
// truncates any top-level array field to maxResults elements, appending a
// note about the total count and how to get more results.
func ListFormatter(maxResults int) ResponseFormatter {
	return func(resp proto.Message) (*mcp.CallToolResult, error) {
		data, err := marshalProto(resp)
		if err != nil {
			return nil, err
		}

		// Scan top-level map for array fields and truncate if needed.
		if m, ok := data.(map[string]any); ok {
			for k, v := range m {
				if arr, ok := v.([]any); ok {
					total := len(arr)
					if total > maxResults {
						truncated := make([]any, maxResults+1)
						for i := 0; i < maxResults; i++ {
							truncated[i] = truncateFields(arr[i], defaultTruncateLen)
						}
						truncated[maxResults] = fmt.Sprintf(
							"[showing %d of %d results — use page_token for more]",
							maxResults, total,
						)
						m[k] = truncated
					} else {
						// Still apply truncateFields to each element.
						for i, elem := range arr {
							arr[i] = truncateFields(elem, defaultTruncateLen)
						}
						m[k] = arr
					}
				}
			}
			data = m
		} else {
			data = truncateFields(data, defaultTruncateLen)
		}

		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("format response: %w", err)
		}
		return mcp.NewToolResultText(string(out)), nil
	}
}

// marshalProto marshals a proto message to a map[string]any using protojson
// with EmitDefaultValues disabled so zero/empty fields are omitted.
func marshalProto(resp proto.Message) (any, error) {
	opts := protojson.MarshalOptions{EmitDefaultValues: false}
	raw, err := opts.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal proto: %w", err)
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse proto json: %w", err)
	}
	return parsed, nil
}

// truncateFields recursively walks the data structure and truncates any string
// value longer than maxLen by cutting it at maxLen and appending truncateMarker.
// Maps and slices are recursed into. Other types are returned unchanged.
func truncateFields(data any, maxLen int) any {
	switch v := data.(type) {
	case string:
		if len(v) > maxLen {
			return v[:maxLen] + truncateMarker
		}
		return v
	case map[string]any:
		for key, val := range v {
			v[key] = truncateFields(val, maxLen)
		}
		return v
	case []any:
		for i, elem := range v {
			v[i] = truncateFields(elem, maxLen)
		}
		return v
	default:
		return v
	}
}
