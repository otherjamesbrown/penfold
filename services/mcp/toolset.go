package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/protobuf/proto"
)

// ToolDef defines a single tool within a toolset.
type ToolDef struct {
	Name        string
	Description string
	Request     proto.Message // Zero value — schema is derived from this type's descriptor
	Handler     server.ToolHandlerFunc
	Annotations mcp.ToolAnnotation
}

// Toolset groups related tools under a named category.
type Toolset struct {
	Name        string
	Description string
	Tools       []ToolDef
}

// ToolsetInfo describes a toolset's metadata for listing.
type ToolsetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ToolCount   int    `json:"tool_count"`
	Enabled     bool   `json:"enabled"`
}

// ToolInfo describes a single tool within a toolset.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
}

// ToolsetManager manages toolset state per session.
type ToolsetManager struct {
	mcpServer *server.MCPServer
	toolsets  map[string]*Toolset // All registered toolset definitions (immutable after init)
	mu        sync.RWMutex
	sessions  map[string]map[string]bool // sessionID -> enabled toolset names
	hooks     *server.Hooks
}

// NewToolsetManager creates a new ToolsetManager with the given toolsets.
func NewToolsetManager(srv *server.MCPServer, toolsets []*Toolset) *ToolsetManager {
	tm := &ToolsetManager{
		mcpServer: srv,
		toolsets:  make(map[string]*Toolset, len(toolsets)),
		sessions:  make(map[string]map[string]bool),
		hooks:     &server.Hooks{},
	}
	for _, ts := range toolsets {
		tm.toolsets[ts.Name] = ts
	}
	return tm
}

// RegisterHooks attaches session lifecycle hooks to the MCP server.
// Must be called before the server starts accepting connections.
func (tm *ToolsetManager) RegisterHooks() {
	tm.hooks.AddOnRegisterSession(func(ctx context.Context, session server.ClientSession) {
		sessionID := session.SessionID()
		tm.mu.Lock()
		tm.sessions[sessionID] = make(map[string]bool)
		tm.mu.Unlock()
	})

	tm.hooks.AddOnUnregisterSession(func(ctx context.Context, session server.ClientSession) {
		sessionID := session.SessionID()
		tm.mu.Lock()
		delete(tm.sessions, sessionID)
		tm.mu.Unlock()
	})

	// Wire hooks into the server by creating a new server with these hooks.
	// Since MCPServer.WithHooks is set at construction time via server.WithHooks,
	// we store the hooks and expose them for use in NewMCPServer.
}

// Hooks returns the server.Hooks instance for use with server.WithHooks.
func (tm *ToolsetManager) Hooks() *server.Hooks {
	return tm.hooks
}

// EnableToolset enables the named toolset for the session found in ctx.
// Returns an error if the toolset doesn't exist, is already enabled, or
// no session is present in ctx.
func (tm *ToolsetManager) EnableToolset(ctx context.Context, name string) error {
	ts, ok := tm.toolsets[name]
	if !ok {
		return fmt.Errorf("toolset %q not found", name)
	}

	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return fmt.Errorf("no session in context")
	}
	sessionID := session.SessionID()

	tm.mu.Lock()
	enabled, exists := tm.sessions[sessionID]
	if !exists {
		// Session not tracked yet — initialise its state.
		enabled = make(map[string]bool)
		tm.sessions[sessionID] = enabled
	}
	if enabled[name] {
		tm.mu.Unlock()
		return fmt.Errorf("toolset %q is already enabled", name)
	}
	enabled[name] = true
	tm.mu.Unlock()

	// Build server.ServerTool slice from ToolDefs.
	serverTools := make([]server.ServerTool, 0, len(ts.Tools))
	for _, def := range ts.Tools {
		schemaJSON, err := buildToolSchema(def)
		if err != nil {
			return fmt.Errorf("build schema for tool %q: %w", def.Name, err)
		}
		tool := mcp.NewToolWithRawSchema(def.Name, def.Description, schemaJSON)
		tool.Annotations = def.Annotations
		serverTools = append(serverTools, server.ServerTool{
			Tool:    tool,
			Handler: withLogging(def.Name, name, def.Handler),
		})
	}

	if err := tm.mcpServer.AddSessionTools(sessionID, serverTools...); err != nil {
		// Roll back the state change.
		tm.mu.Lock()
		delete(tm.sessions[sessionID], name)
		tm.mu.Unlock()
		return fmt.Errorf("add session tools: %w", err)
	}

	slog.Info("toolset_enabled",
		"toolset", name,
		"tools_added", len(ts.Tools),
		"session_id", sessionID,
	)
	return nil
}

// DisableToolset disables the named toolset for the session found in ctx.
func (tm *ToolsetManager) DisableToolset(ctx context.Context, name string) error {
	ts, ok := tm.toolsets[name]
	if !ok {
		return fmt.Errorf("toolset %q not found", name)
	}

	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return fmt.Errorf("no session in context")
	}
	sessionID := session.SessionID()

	tm.mu.Lock()
	enabled, exists := tm.sessions[sessionID]
	if !exists || !enabled[name] {
		tm.mu.Unlock()
		return fmt.Errorf("toolset %q is not enabled", name)
	}
	delete(enabled, name)
	tm.mu.Unlock()

	// Collect tool names to remove.
	names := make([]string, 0, len(ts.Tools))
	for _, def := range ts.Tools {
		names = append(names, def.Name)
	}

	if err := tm.mcpServer.DeleteSessionTools(sessionID, names...); err != nil {
		// Roll back the state change.
		tm.mu.Lock()
		tm.sessions[sessionID][name] = true
		tm.mu.Unlock()
		return fmt.Errorf("delete session tools: %w", err)
	}

	slog.Info("toolset_disabled",
		"toolset", name,
		"tools_removed", len(names),
		"session_id", sessionID,
	)
	return nil
}

// ListToolsets returns metadata for all registered toolsets, marking which
// are currently enabled for the session in ctx (if any).
func (tm *ToolsetManager) ListToolsets(ctx context.Context) []ToolsetInfo {
	var sessionEnabled map[string]bool
	if session := server.ClientSessionFromContext(ctx); session != nil {
		sessionID := session.SessionID()
		tm.mu.RLock()
		sessionEnabled = tm.sessions[sessionID]
		tm.mu.RUnlock()
	}

	infos := make([]ToolsetInfo, 0, len(tm.toolsets))
	for _, ts := range tm.toolsets {
		infos = append(infos, ToolsetInfo{
			Name:        ts.Name,
			Description: ts.Description,
			ToolCount:   len(ts.Tools),
			Enabled:     sessionEnabled[ts.Name],
		})
	}
	return infos
}

// GetToolsetTools returns the tools defined in a named toolset.
// This is not session-specific — it always returns the full list.
func (tm *ToolsetManager) GetToolsetTools(name string) ([]ToolInfo, error) {
	ts, ok := tm.toolsets[name]
	if !ok {
		return nil, fmt.Errorf("toolset %q not found", name)
	}
	infos := make([]ToolInfo, 0, len(ts.Tools))
	for _, def := range ts.Tools {
		readOnly := false
		if def.Annotations.ReadOnlyHint != nil {
			readOnly = *def.Annotations.ReadOnlyHint
		}
		infos = append(infos, ToolInfo{
			Name:        def.Name,
			Description: def.Description,
			ReadOnly:    readOnly,
		})
	}
	return infos, nil
}

// isEnabled reports whether a toolset is currently enabled for the given session.
func (tm *ToolsetManager) isEnabled(sessionID, toolsetName string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.sessions[sessionID][toolsetName]
}

// buildToolSchema generates the raw JSON Schema bytes for a ToolDef.
// If the ToolDef has a non-nil Request, the schema is derived from the proto
// message descriptor; otherwise an empty object schema is used.
func buildToolSchema(def ToolDef) ([]byte, error) {
	if def.Request == nil {
		return []byte(`{"type":"object"}`), nil
	}
	schema := schemaFromProto(def.Request.ProtoReflect().Descriptor())
	return json.Marshal(schema)
}
