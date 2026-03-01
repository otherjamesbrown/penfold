package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// RegisterMetaTools registers the four toolset management meta-tools on the
// MCP server globally (not per-session). These tools are always available.
func (tm *ToolsetManager) RegisterMetaTools() {
	// penfold_list_toolsets — no parameters
	listTool := mcp.NewToolWithRawSchema(
		"penfold_list_toolsets",
		"List all available Penfold toolsets and their status. Call this first to discover what capabilities are available, then enable the toolsets you need.",
		[]byte(`{"type":"object"}`),
	)
	listTool.Annotations = mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}
	tm.mcpServer.AddTool(listTool, tm.handleListToolsets)

	// penfold_get_toolset_tools — toolset_name required
	getTool := mcp.NewToolWithRawSchema(
		"penfold_get_toolset_tools",
		"Get detailed information about the tools available in a specific toolset, including their names and descriptions.",
		[]byte(`{"type":"object","properties":{"toolset_name":{"type":"string","description":"Name of the toolset to inspect"}},"required":["toolset_name"]}`),
	)
	getTool.Annotations = mcp.ToolAnnotation{ReadOnlyHint: boolPtr(true)}
	tm.mcpServer.AddTool(getTool, tm.handleGetToolsetTools)

	// penfold_enable_toolset — toolset_name required
	enableTool := mcp.NewToolWithRawSchema(
		"penfold_enable_toolset",
		"Enable a toolset for the current session. Once enabled, the toolset's tools will appear in your tool list and be callable.",
		[]byte(`{"type":"object","properties":{"toolset_name":{"type":"string","description":"Name of the toolset to enable"}},"required":["toolset_name"]}`),
	)
	enableTool.Annotations = mcp.ToolAnnotation{ReadOnlyHint: boolPtr(false)}
	tm.mcpServer.AddTool(enableTool, tm.handleEnableToolset)

	// penfold_disable_toolset — toolset_name required
	disableTool := mcp.NewToolWithRawSchema(
		"penfold_disable_toolset",
		"Disable a toolset for the current session. The toolset's tools will be removed from your tool list.",
		[]byte(`{"type":"object","properties":{"toolset_name":{"type":"string","description":"Name of the toolset to disable"}},"required":["toolset_name"]}`),
	)
	disableTool.Annotations = mcp.ToolAnnotation{ReadOnlyHint: boolPtr(false)}
	tm.mcpServer.AddTool(disableTool, tm.handleDisableToolset)
}

func (tm *ToolsetManager) handleListToolsets(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	infos := tm.ListToolsets(ctx)
	data, err := json.Marshal(infos)
	if err != nil {
		return mcp.NewToolResultError("failed to marshal toolset list: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (tm *ToolsetManager) handleGetToolsetTools(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.GetArguments()["toolset_name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("toolset_name is required"), nil
	}

	tools, err := tm.GetToolsetTools(name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	data, err := json.Marshal(tools)
	if err != nil {
		return mcp.NewToolResultError("failed to marshal tool list: " + err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (tm *ToolsetManager) handleEnableToolset(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.GetArguments()["toolset_name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("toolset_name is required"), nil
	}

	if err := tm.EnableToolset(ctx, name); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ts := tm.toolsets[name]
	toolNames := make([]string, 0, len(ts.Tools))
	for _, def := range ts.Tools {
		toolNames = append(toolNames, def.Name)
	}

	msg := fmt.Sprintf("Enabled toolset %q. %d tool(s) added: %v", name, len(toolNames), toolNames)
	return mcp.NewToolResultText(msg), nil
}

func (tm *ToolsetManager) handleDisableToolset(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := req.GetArguments()["toolset_name"].(string)
	if !ok || name == "" {
		return mcp.NewToolResultError("toolset_name is required"), nil
	}

	if err := tm.DisableToolset(ctx, name); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Disabled toolset %q.", name)), nil
}
