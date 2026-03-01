package main

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// newDummyClients creates gRPC clients from a dummy connection.
// These are only used for constructing the toolset definition — no
// calls are made in unit tests.
func newDummyClients(t *testing.T) (
	searchv1.SearchServiceClient,
	assertionsv1.AssertionsServiceClient,
	contentv1.ContentProcessorServiceClient,
	threadsv1.ThreadsServiceClient,
) {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///dummy",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return searchv1.NewSearchServiceClient(conn),
		assertionsv1.NewAssertionsServiceClient(conn),
		contentv1.NewContentProcessorServiceClient(conn),
		threadsv1.NewThreadsServiceClient(conn)
}

func TestSearchToolset_ToolCount(t *testing.T) {
	ts := searchToolset(newDummyClients(t))
	assert.Equal(t, "search", ts.Name)
	assert.Len(t, ts.Tools, 6, "search toolset should have 6 tools")
}

func TestSearchToolset_ToolNames(t *testing.T) {
	ts := searchToolset(newDummyClients(t))
	expected := []string{
		"penfold_search",
		"penfold_semantic_search",
		"penfold_search_assertions",
		"penfold_get_content",
		"penfold_get_content_text",
		"penfold_get_thread",
	}
	names := make([]string, len(ts.Tools))
	for i, td := range ts.Tools {
		names[i] = td.Name
	}
	assert.Equal(t, expected, names)
}

func TestSearchToolset_AllReadOnly(t *testing.T) {
	ts := searchToolset(newDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Annotations.ReadOnlyHint, "tool %s should have ReadOnlyHint set", td.Name)
		assert.True(t, *td.Annotations.ReadOnlyHint, "tool %s should be read-only", td.Name)
	}
}

func TestSearchToolset_AllHaveHandlers(t *testing.T) {
	ts := searchToolset(newDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotNil(t, td.Handler, "tool %s should have a handler", td.Name)
	}
}

func TestSearchToolset_AllHaveDescriptions(t *testing.T) {
	ts := searchToolset(newDummyClients(t))
	for _, td := range ts.Tools {
		assert.NotEmpty(t, td.Description, "tool %s should have a description", td.Name)
	}
}

func TestSearchToolset_AllHaveSchemas(t *testing.T) {
	ts := searchToolset(newDummyClients(t))
	for _, td := range ts.Tools {
		require.NotNil(t, td.Request, "tool %s should have a Request proto for schema generation", td.Name)
		schema, err := buildToolSchema(td)
		require.NoError(t, err, "tool %s schema build failed", td.Name)
		assert.Contains(t, string(schema), `"type"`, "tool %s schema should contain type field", td.Name)
	}
}

func TestSearchToolset_EnableDisable(t *testing.T) {
	ts := searchToolset(newDummyClients(t))

	// Build a ToolsetManager with the real search toolset.
	allToolsets := []*Toolset{
		ts,
		{Name: "knowledge", Description: "stub"},
	}
	tm := NewToolsetManager(nil, allToolsets)
	tm.RegisterHooks()

	srv := server.NewMCPServer("test-penfold", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(tm.Hooks()),
	)
	tm.mcpServer = srv

	sess := newMockSession("sess-search-toggle")
	ctx := registerSession(t, srv, sess)

	// Enable: all 6 tools should appear.
	err := tm.EnableToolset(ctx, "search")
	require.NoError(t, err)
	sessionTools := sess.GetSessionTools()
	assert.Len(t, sessionTools, 6, "enabling search should add 6 tools")
	assert.Contains(t, sessionTools, "penfold_search")
	assert.Contains(t, sessionTools, "penfold_semantic_search")
	assert.Contains(t, sessionTools, "penfold_search_assertions")
	assert.Contains(t, sessionTools, "penfold_get_content")
	assert.Contains(t, sessionTools, "penfold_get_content_text")
	assert.Contains(t, sessionTools, "penfold_get_thread")

	// Disable: all 6 tools should be removed.
	err = tm.DisableToolset(ctx, "search")
	require.NoError(t, err)
	sessionTools = sess.GetSessionTools()
	assert.Empty(t, sessionTools, "disabling search should remove all tools")
}
