package web

import (
	"encoding/json"
	"testing"

	"m365-copilot2api/internal/chathub"
	"m365-copilot2api/internal/mcp"
)

func clientTool(t *testing.T, name, description string) chathub.Tool {
	t.Helper()
	functionJSON, err := json.Marshal(map[string]any{
		"name":        name,
		"description": description,
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return chathub.Tool{
		Type:     "function",
		Function: functionJSON,
	}
}

func TestEffectiveClientToolsDefaultKeepsClientToolsOnly(t *testing.T) {
	clientTools := []chathub.Tool{
		clientTool(t, "read_file", "client version"),
	}
	registryTools := []mcp.Tool{
		{Name: "server_search", Description: "server tool"},
	}

	got := effectiveClientTools("default", clientTools, registryTools)

	if len(got) != 1 {
		t.Fatalf("got %d tools, want 1", len(got))
	}
	if name := chathubToolName(got[0]); name != "read_file" {
		t.Fatalf("got tool %q, want read_file", name)
	}
}

func TestEffectiveClientToolsFullAccessAddsRegistryTools(t *testing.T) {
	clientTools := []chathub.Tool{
		clientTool(t, "tool_1", "client tool 1"),
		clientTool(t, "tool_2", "client tool 2"),
		clientTool(t, "tool_3", "client tool 3"),
		clientTool(t, "tool_4", "client tool 4"),
		clientTool(t, "tool_5", "client tool 5"),
		clientTool(t, "tool_6", "client tool 6"),
		clientTool(t, "tool_7", "client tool 7"),
	}
	registryTools := []mcp.Tool{
		{Name: "server_tool_1"},
		{Name: "server_tool_2"},
		{Name: "server_tool_3"},
	}

	got := effectiveClientTools("full_access", clientTools, registryTools)

	if len(got) != 10 {
		t.Fatalf("got %d tools, want 10", len(got))
	}
}

func TestEffectiveClientToolsClientDeclarationWins(t *testing.T) {
	clientTools := []chathub.Tool{
		clientTool(t, "read_file", "client schema"),
	}
	registryTools := []mcp.Tool{
		{Name: "read_file", Description: "registry schema", InputSchema: map[string]any{"type": "object"}},
		{Name: "search_code"},
	}

	got := effectiveClientTools("full_access", clientTools, registryTools)

	if len(got) != 2 {
		t.Fatalf("got %d tools, want 2", len(got))
	}

	var function map[string]any
	if err := json.Unmarshal(got[0].Function, &function); err != nil {
		t.Fatal(err)
	}
	if function["description"] != "client schema" {
		t.Fatalf("duplicate tool did not preserve client declaration: %#v", function)
	}
}

func TestEffectiveClientToolsSkipsInvalidRegistryTools(t *testing.T) {
	clientTools := []chathub.Tool{}
	registryTools := []mcp.Tool{
		{Name: ""},
		{Name: "   "},
		{Name: "valid_tool"},
	}

	got := effectiveClientTools("full_access", clientTools, registryTools)

	if len(got) != 1 {
		t.Fatalf("got %d tools, want 1", len(got))
	}
	if name := chathubToolName(got[0]); name != "valid_tool" {
		t.Fatalf("got tool %q, want valid_tool", name)
	}
}

func TestFullAccessEffectiveToolsProduceMatchingToolMaps(t *testing.T) {
	clientTools := []chathub.Tool{
		clientTool(t, "client_tool", "client"),
	}
	registryTools := []mcp.Tool{
		{Name: "registry_tool"},
	}

	effective := effectiveClientTools("full_access", clientTools, registryTools)
	toolMaps := normalizeToolMaps(effective)

	if len(effective) != 2 {
		t.Fatalf("effective tools = %d, want 2", len(effective))
	}
	if len(toolMaps) != len(effective) {
		t.Fatalf(
			"tool maps = %d, effective tools = %d",
			len(toolMaps),
			len(effective),
		)
	}
}
