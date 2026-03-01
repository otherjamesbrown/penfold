//go:build integration

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// integEnv holds the shared test environment for integration tests.
type integEnv struct {
	tm            *ToolsetManager
	toolset       *Toolset
	contentClient contentv1.ContentProcessorServiceClient
	ctx           context.Context
}

// setupIntegration creates a full ToolsetManager with real gRPC clients
// connected to the gateway and returns the test environment.
func setupIntegration(t *testing.T) *integEnv {
	t.Helper()

	addr := os.Getenv("PENFOLD_GATEWAY_ADDR")
	if addr == "" {
		addr = "localhost:50051"
	}

	transportCreds := grpc.WithTransportCredentials(insecure.NewCredentials())
	if caPath := os.Getenv("PENFOLD_GATEWAY_TLS_CA"); caPath != "" {
		caCert, err := os.ReadFile(caPath)
		require.NoError(t, err, "failed to read CA cert")
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		transportCreds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs: pool,
		}))
	}

	conn, err := grpc.NewClient(addr, transportCreds)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	contentClient := contentv1.NewContentProcessorServiceClient(conn)
	ts := searchToolset(
		searchv1.NewSearchServiceClient(conn),
		assertionsv1.NewAssertionsServiceClient(conn),
		contentClient,
		threadsv1.NewThreadsServiceClient(conn),
	)

	toolsets := []*Toolset{ts}
	tm := NewToolsetManager(nil, toolsets)
	tm.RegisterHooks()

	srv := server.NewMCPServer("test-penfold", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(tm.Hooks()),
	)
	tm.mcpServer = srv

	sess := newMockSession("integ-session")
	ctx := registerSession(t, srv, sess)

	// Enable the search toolset.
	require.NoError(t, tm.EnableToolset(ctx, "search"))

	return &integEnv{tm: tm, toolset: ts, contentClient: contentClient, ctx: ctx}
}

// callTool invokes a tool handler by name from the toolset.
func (e *integEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	for _, td := range e.toolset.Tools {
		if td.Name == name {
			req := mcp.CallToolRequest{}
			req.Params.Name = name
			req.Params.Arguments = args
			result, err := td.Handler(e.ctx, req)
			require.NoError(t, err)
			return result
		}
	}
	t.Fatalf("tool %q not found in toolset", name)
	return nil
}

func TestIntegration_SearchToolset_Enable(t *testing.T) {
	env := setupIntegration(t)

	// Verify 6 tools are available.
	tools, err := env.tm.GetToolsetTools("search")
	require.NoError(t, err)
	assert.Len(t, tools, 6)

	// Verify the toolset is marked enabled.
	infos := env.tm.ListToolsets(env.ctx)
	for _, info := range infos {
		if info.Name == "search" {
			assert.True(t, info.Enabled)
			assert.Equal(t, 6, info.ToolCount)
		}
	}
}

func TestIntegration_SearchToolset_Search(t *testing.T) {
	env := setupIntegration(t)

	result := env.callTool(t, "penfold_search", map[string]any{
		"query": "test",
	})
	if result.IsError {
		errText := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("penfold_search returned error: %s", errText)
	}
	require.NotEmpty(t, result.Content, "should have content")

	// Parse the response text as JSON.
	text := result.Content[0].(mcp.TextContent).Text
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed), "response should be valid JSON")
}

func TestIntegration_SearchToolset_SearchResultLimit(t *testing.T) {
	env := setupIntegration(t)

	result := env.callTool(t, "penfold_search", map[string]any{
		"query": "the",
		"limit": 50,
	})
	require.False(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))

	// Check that ListFormatter capped the results at 10.
	for _, v := range parsed {
		if arr, ok := v.([]any); ok {
			assert.LessOrEqual(t, len(arr), 11, // 10 results + 1 truncation note
				"list formatter should cap results at 10 (plus optional note)")
		}
	}
}

func TestIntegration_SearchToolset_SearchFieldTruncation(t *testing.T) {
	env := setupIntegration(t)

	result := env.callTool(t, "penfold_search", map[string]any{
		"query": "the",
	})
	require.False(t, result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	var parsed any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))

	// Walk the parsed JSON and check no string field exceeds 500 chars
	// (plus the truncation marker length).
	maxAllowed := 500 + len(truncateMarker)
	assertNoLongStrings(t, parsed, maxAllowed)
}

func assertNoLongStrings(t *testing.T, data any, maxLen int) {
	t.Helper()
	switch v := data.(type) {
	case string:
		assert.LessOrEqual(t, len(v), maxLen, "string field exceeds max length: %s...", v[:min(80, len(v))])
	case map[string]any:
		for _, val := range v {
			assertNoLongStrings(t, val, maxLen)
		}
	case []any:
		for _, elem := range v {
			assertNoLongStrings(t, elem, maxLen)
		}
	}
}

func TestIntegration_SearchToolset_GetContent(t *testing.T) {
	env := setupIntegration(t)

	// Get a known content ID by listing content items directly.
	contentID := findContentID(t, env)
	if contentID == "" {
		t.Skip("no content items found — cannot test get_content")
	}

	result := env.callTool(t, "penfold_get_content", map[string]any{
		"contentId": contentID,
	})
	if result.IsError {
		errText := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("penfold_get_content failed for ID %q: %s", contentID, errText)
	}

	respText := result.Content[0].(mcp.TextContent).Text
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(respText), &parsed))
	assert.NotEmpty(t, parsed, "content item response should not be empty")
}

func TestIntegration_SearchToolset_GetContentText(t *testing.T) {
	env := setupIntegration(t)

	contentID := findContentID(t, env)
	if contentID == "" {
		t.Skip("no content items found — cannot test get_content_text")
	}

	result := env.callTool(t, "penfold_get_content_text", map[string]any{
		"contentId": contentID,
	})
	if result.IsError {
		errText := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("penfold_get_content_text failed for ID %q: %s", contentID, errText)
	}
	require.NotEmpty(t, result.Content)
}

func TestIntegration_SearchToolset_ErrorNotFound(t *testing.T) {
	env := setupIntegration(t)

	result := env.callTool(t, "penfold_get_content", map[string]any{
		"contentId": "nonexistent-fake-id-12345",
	})
	require.True(t, result.IsError, "should return an error for fake ID")

	errText := result.Content[0].(mcp.TextContent).Text
	assert.Contains(t, errText, "penfold_search", "error should guide user to use penfold_search")
}

func TestIntegration_SearchToolset_Disable(t *testing.T) {
	env := setupIntegration(t)

	// Toolset is enabled by setupIntegration. Disable it.
	err := env.tm.DisableToolset(env.ctx, "search")
	require.NoError(t, err)

	// Verify toolset definition still exists but is disabled.
	tools, err := env.tm.GetToolsetTools("search")
	require.NoError(t, err)
	assert.Len(t, tools, 6, "toolset definition still has 6 tools")

	infos := env.tm.ListToolsets(env.ctx)
	for _, info := range infos {
		if info.Name == "search" {
			assert.False(t, info.Enabled, "search should be disabled")
		}
	}
}

// findContentID queries the content service for a real content_id via ListContentItems.
func findContentID(t *testing.T, env *integEnv) string {
	t.Helper()
	ctx := env.ctx
	if tenantID := os.Getenv("PENFOLD_TENANT_ID"); tenantID != "" {
		md := metadata.New(map[string]string{"x-tenant-id": tenantID})
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	resp, err := env.contentClient.ListContentItems(ctx, &contentv1.ListContentItemsRequest{
		TenantId: os.Getenv("PENFOLD_TENANT_ID"),
		PageSize: 1,
	})
	if err != nil {
		t.Logf("ListContentItems failed: %v", err)
		return ""
	}
	if len(resp.GetItems()) == 0 {
		return ""
	}
	return resp.GetItems()[0].GetId()
}
