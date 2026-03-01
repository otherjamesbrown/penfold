package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractResultText pulls the text from the first content item of a CallToolResult.
func extractResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok, "expected TextContent, got %T", result.Content[0])
	return tc.Text
}

// --- truncateFields tests ---

func TestTruncateFields_ShortString(t *testing.T) {
	input := "hello world"
	result := truncateFields(input, 500)
	assert.Equal(t, "hello world", result)
}

func TestTruncateFields_LongString(t *testing.T) {
	long := strings.Repeat("a", 600)
	result := truncateFields(long, 500)
	s, ok := result.(string)
	require.True(t, ok)
	assert.Equal(t, 500, len(s[:500]))
	assert.Contains(t, s, truncateMarker)
	assert.Equal(t, strings.Repeat("a", 500)+truncateMarker, s)
}

func TestTruncateFields_NestedObject(t *testing.T) {
	long := strings.Repeat("b", 600)
	input := map[string]any{
		"outer": map[string]any{
			"inner": long,
		},
	}
	result := truncateFields(input, 500)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	outer, ok := m["outer"].(map[string]any)
	require.True(t, ok)
	s, ok := outer["inner"].(string)
	require.True(t, ok)
	assert.True(t, strings.HasSuffix(s, truncateMarker))
	assert.Equal(t, len(strings.Repeat("b", 500))+len(truncateMarker), len(s))
}

func TestTruncateFields_Array(t *testing.T) {
	long := strings.Repeat("c", 600)
	input := []any{long, "short", long}
	result := truncateFields(input, 500)
	arr, ok := result.([]any)
	require.True(t, ok)
	require.Len(t, arr, 3)

	// First element should be truncated
	s0, ok := arr[0].(string)
	require.True(t, ok)
	assert.True(t, strings.HasSuffix(s0, truncateMarker))

	// Second element should be unchanged
	assert.Equal(t, "short", arr[1])

	// Third element should be truncated
	s2, ok := arr[2].(string)
	require.True(t, ok)
	assert.True(t, strings.HasSuffix(s2, truncateMarker))
}

// --- DefaultFormatter tests ---

func TestDefaultFormatter_OmitsEmptyFields(t *testing.T) {
	// SearchResponse with only some fields set — zero/empty fields should not appear
	resp := &searchv1.SearchResponse{
		Results: []*searchv1.SearchResult{
			{
				DocumentId: "doc-1",
				Snippet:    "test snippet",
				// Score is 0 — should be omitted by EmitDefaultValues:false
			},
		},
		// TotalCount is 0 — should be omitted
		// QueryTimeMs is 0 — should be omitted
	}

	formatter := DefaultFormatter()
	result, err := formatter(resp)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := extractResultText(t, result)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))

	// Results should be present
	resultsRaw, ok := parsed["results"]
	require.True(t, ok, "expected 'results' field in output")
	results, ok := resultsRaw.([]any)
	require.True(t, ok)
	require.Len(t, results, 1)

	// totalCount=0 and queryTimeMs=0 should be absent (EmitDefaultValues:false)
	assert.NotContains(t, parsed, "totalCount", "zero int should be omitted")
	assert.NotContains(t, parsed, "queryTimeMs", "zero float should be omitted")
}

// --- ListFormatter tests ---

func TestListFormatter_TruncatesArray(t *testing.T) {
	results := make([]*searchv1.SearchResult, 10)
	for i := range results {
		results[i] = &searchv1.SearchResult{
			DocumentId: "doc",
			Snippet:    "snippet",
		}
	}
	resp := &searchv1.SearchResponse{Results: results}

	formatter := ListFormatter(3)
	result, err := formatter(resp)
	require.NoError(t, err)
	require.NotNil(t, result)

	text := extractResultText(t, result)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))

	resultsRaw, ok := parsed["results"]
	require.True(t, ok)
	arr, ok := resultsRaw.([]any)
	require.True(t, ok)

	// Should have 3 results + 1 note string = 4 elements
	require.Len(t, arr, 4, "expected 3 results + 1 note")

	// Last element should be the note string
	note, ok := arr[3].(string)
	require.True(t, ok, "last element should be a string note")
	assert.Contains(t, note, "showing 3 of 10")
	assert.Contains(t, note, "page_token")
}

func TestListFormatter_ShortArray(t *testing.T) {
	results := make([]*searchv1.SearchResult, 2)
	for i := range results {
		results[i] = &searchv1.SearchResult{
			DocumentId: "doc",
			Snippet:    "snippet",
		}
	}
	resp := &searchv1.SearchResponse{Results: results}

	formatter := ListFormatter(5)
	result, err := formatter(resp)
	require.NoError(t, err)
	require.NotNil(t, result)

	text := extractResultText(t, result)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))

	resultsRaw, ok := parsed["results"]
	require.True(t, ok)
	arr, ok := resultsRaw.([]any)
	require.True(t, ok)

	// With only 2 results and maxResults=5, no truncation should occur
	assert.Len(t, arr, 2)
	// Verify no note string was appended
	for _, elem := range arr {
		_, isStr := elem.(string)
		assert.False(t, isStr, "unexpected string note in short array")
	}
}
