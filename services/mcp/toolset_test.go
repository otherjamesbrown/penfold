package main

import (
	"context"
	"maps"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
)

// mockSession implements both ClientSession and SessionWithTools for testing.
type mockSession struct {
	id           string
	ch           chan mcp.JSONRPCNotification
	inited       bool
	sessionTools map[string]server.ServerTool
	mu           sync.RWMutex
}

func newMockSession(id string) *mockSession {
	return &mockSession{
		id: id,
		ch: make(chan mcp.JSONRPCNotification, 100),
	}
}

func (m *mockSession) SessionID() string { return m.id }
func (m *mockSession) NotificationChannel() chan<- mcp.JSONRPCNotification { return m.ch }
func (m *mockSession) Initialize()  { m.inited = true }
func (m *mockSession) Initialized() bool { return m.inited }

func (m *mockSession) GetSessionTools() map[string]server.ServerTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.sessionTools == nil {
		return nil
	}
	cp := make(map[string]server.ServerTool, len(m.sessionTools))
	maps.Copy(cp, m.sessionTools)
	return cp
}

func (m *mockSession) SetSessionTools(tools map[string]server.ServerTool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tools == nil {
		m.sessionTools = nil
		return
	}
	cp := make(map[string]server.ServerTool, len(tools))
	maps.Copy(cp, tools)
	m.sessionTools = cp
}

// dummyHandler is a no-op tool handler for test toolsets.
func dummyHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("dummy"), nil
}

// testToolsets returns a small set of toolsets for unit tests.
func testToolsets() []*Toolset {
	return []*Toolset{
		{
			Name:        "search",
			Description: "Full-text and semantic search",
			Tools: []ToolDef{
				{
					Name:        "penfold_search",
					Description: "Search the knowledge base",
					Request:     &searchv1.SearchRequest{},
					Handler:     dummyHandler,
					Annotations: mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)},
				},
			},
		},
		{
			Name:        "knowledge",
			Description: "Access extracted knowledge",
			Tools:       []ToolDef{},
		},
		{
			Name:        "entities",
			Description: "Browse people and organizations",
			Tools:       []ToolDef{},
		},
		{
			Name:        "content",
			Description: "Read full email text",
			Tools:       []ToolDef{},
		},
		{
			Name:        "ops",
			Description: "System operations",
			Tools:       []ToolDef{},
		},
		{
			Name:        "workflow",
			Description: "Pipeline monitoring",
			Tools:       []ToolDef{},
		},
	}
}

// setupManager builds a ToolsetManager backed by a real MCPServer for testing.
func setupManager(t *testing.T) (*ToolsetManager, *server.MCPServer) {
	t.Helper()
	ts := testToolsets()
	// Temporary manager to obtain hooks before creating the server.
	tmp := NewToolsetManager(nil, ts)
	tmp.RegisterHooks()
	srv := server.NewMCPServer("test-penfold", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithHooks(tmp.Hooks()),
	)
	tmp.mcpServer = srv
	return tmp, srv
}

// registerSession registers a mock session and returns a context bound to it.
func registerSession(t *testing.T, srv *server.MCPServer, sess *mockSession) context.Context {
	t.Helper()
	err := srv.RegisterSession(context.Background(), sess)
	require.NoError(t, err)
	ctx := srv.WithContext(context.Background(), sess)
	return ctx
}

// TestListToolsets verifies all toolsets are returned with correct metadata.
func TestListToolsets(t *testing.T) {
	tm, srv := setupManager(t)
	sess := newMockSession("sess-list")
	ctx := registerSession(t, srv, sess)

	infos := tm.ListToolsets(ctx)
	require.Len(t, infos, 6, "expected 6 toolsets")

	names := make(map[string]ToolsetInfo, len(infos))
	for _, info := range infos {
		names[info.Name] = info
	}

	assert.Contains(t, names, "search")
	assert.Contains(t, names, "knowledge")
	assert.Equal(t, "Full-text and semantic search", names["search"].Description)
	assert.Equal(t, 1, names["search"].ToolCount)
	assert.Equal(t, 0, names["knowledge"].ToolCount)
	assert.False(t, names["search"].Enabled, "search should start disabled")
}

// TestGetToolsetTools verifies tool info for a known toolset.
func TestGetToolsetTools(t *testing.T) {
	tm, _ := setupManager(t)

	tools, err := tm.GetToolsetTools("search")
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "penfold_search", tools[0].Name)
	assert.Equal(t, "Search the knowledge base", tools[0].Description)
	assert.True(t, tools[0].ReadOnly)
}

// TestGetToolsetTools_NonExistent verifies an error for unknown toolsets.
func TestGetToolsetTools_NonExistent(t *testing.T) {
	tm, _ := setupManager(t)

	_, err := tm.GetToolsetTools("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestEnableDisableToolset verifies that enabling adds tools and disabling removes them.
func TestEnableDisableToolset(t *testing.T) {
	tm, srv := setupManager(t)
	sess := newMockSession("sess-toggle")
	ctx := registerSession(t, srv, sess)

	// Enable: tools should be present on the session.
	err := tm.EnableToolset(ctx, "search")
	require.NoError(t, err)
	assert.True(t, tm.isEnabled("sess-toggle", "search"))
	sessionTools := sess.GetSessionTools()
	assert.Contains(t, sessionTools, "penfold_search", "tool should be in session after enable")

	// ListToolsets should show it as enabled.
	infos := tm.ListToolsets(ctx)
	for _, info := range infos {
		if info.Name == "search" {
			assert.True(t, info.Enabled)
		}
	}

	// Disable: tools should be removed.
	err = tm.DisableToolset(ctx, "search")
	require.NoError(t, err)
	assert.False(t, tm.isEnabled("sess-toggle", "search"))
	sessionTools = sess.GetSessionTools()
	assert.NotContains(t, sessionTools, "penfold_search", "tool should be absent after disable")
}

// TestEnableAlreadyEnabled verifies an error when enabling a toolset twice.
func TestEnableAlreadyEnabled(t *testing.T) {
	tm, srv := setupManager(t)
	sess := newMockSession("sess-double-enable")
	ctx := registerSession(t, srv, sess)

	require.NoError(t, tm.EnableToolset(ctx, "search"))
	err := tm.EnableToolset(ctx, "search")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already enabled")
}

// TestDisableNotEnabled verifies an error when disabling a toolset that is not enabled.
func TestDisableNotEnabled(t *testing.T) {
	tm, srv := setupManager(t)
	sess := newMockSession("sess-disable-not-enabled")
	ctx := registerSession(t, srv, sess)

	err := tm.DisableToolset(ctx, "search")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not enabled")
}

// TestSessionIsolation verifies that two sessions have independent toolset state.
func TestSessionIsolation(t *testing.T) {
	tm, srv := setupManager(t)

	sessA := newMockSession("sess-isolation-a")
	sessB := newMockSession("sess-isolation-b")
	ctxA := registerSession(t, srv, sessA)
	ctxB := registerSession(t, srv, sessB)

	// Enable search for session A only.
	require.NoError(t, tm.EnableToolset(ctxA, "search"))

	assert.True(t, tm.isEnabled("sess-isolation-a", "search"), "A: search should be enabled")
	assert.False(t, tm.isEnabled("sess-isolation-b", "search"), "B: search should NOT be enabled")

	// Session A tool list shows the tool; session B does not.
	assert.Contains(t, sessA.GetSessionTools(), "penfold_search")
	assert.NotContains(t, sessB.GetSessionTools(), "penfold_search")

	// Enable search for session B independently.
	require.NoError(t, tm.EnableToolset(ctxB, "search"))
	assert.True(t, tm.isEnabled("sess-isolation-b", "search"), "B: search should now be enabled")

	// Disable on A does not affect B.
	require.NoError(t, tm.DisableToolset(ctxA, "search"))
	assert.False(t, tm.isEnabled("sess-isolation-a", "search"), "A: search should be disabled")
	assert.True(t, tm.isEnabled("sess-isolation-b", "search"), "B: search should still be enabled")
}
