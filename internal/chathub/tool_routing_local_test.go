package chathub

import (
	"strings"
	"testing"
)

func pascalTool(t *testing.T, name string) Tool {
	t.Helper()
	return Tool{
		Type: "function",
		Function: mustToolJSON(t, map[string]any{
			"name":        name,
			"description": "tool " + name,
			"parameters":  map[string]any{"type": "object"},
		}),
	}
}

// TestSplitRoutedToolsTreatsPascalCaseNamesAsLocal reproduces the reported
// failure: a Claude Code client declares Grep/Read/Bash/Write, and the
// case-sensitive lookup classified every one of them as a cloud plugin. The
// local tools then never reached the inlined tool protocol prompt, so the
// model answered that the workspace did not exist instead of calling the
// tool the client had actually declared.
func TestSplitRoutedToolsTreatsPascalCaseNamesAsLocal(t *testing.T) {
	tools := []Tool{
		pascalTool(t, "Grep"),
		pascalTool(t, "Read"),
		pascalTool(t, "Bash"),
		pascalTool(t, "Write"),
	}

	native, prompt := splitRoutedTools(tools)

	if len(native) != 0 {
		t.Fatalf("client-local tools must not be native plugins, got %v", toolNames(native))
	}
	if len(prompt) != len(tools) {
		t.Fatalf("client-local tools must be inlined into the prompt, got %v", toolNames(prompt))
	}
}

// TestSplitRoutedToolsKeepsCloudToolsNative guards the other direction: a
// genuinely cloud-hosted tool must still be routed to the plugin list.
func TestSplitRoutedToolsKeepsCloudToolsNative(t *testing.T) {
	tools := []Tool{pascalTool(t, "Grep"), pascalTool(t, "get_weather")}

	native, prompt := splitRoutedTools(tools)

	if names := toolNames(native); len(names) != 1 || names[0] != "get_weather" {
		t.Fatalf("cloud tool must stay native, got %v", names)
	}
	if names := toolNames(prompt); len(names) != 1 || names[0] != "Grep" {
		t.Fatalf("local tool must be inlined, got %v", names)
	}
}

// TestToolProtocolPromptIncludesPascalCaseTools is the end-to-end symptom
// check: the declared tool names must be visible to the model.
func TestToolProtocolPromptIncludesPascalCaseTools(t *testing.T) {
	tools := []Tool{pascalTool(t, "Grep"), pascalTool(t, "Read"), pascalTool(t, "Bash"), pascalTool(t, "Write")}
	_, prompt := splitRoutedTools(tools)

	got := toolProtocolPrompt("inspect C:\\Workspace\\yhd-sale", prompt, "auto")
	for _, name := range []string{"Grep", "Read", "Bash", "Write"} {
		if !strings.Contains(got, "```"+name+"\n") {
			t.Fatalf("tool %s missing from protocol prompt:\n%s", name, got)
		}
	}
}

func TestIsLocalClientToolMatchesCaseInsensitively(t *testing.T) {
	for _, name := range []string{"Read", "read", "READ", "Grep", "Glob", "Edit", "Bash", "BASH", "shell_command", "read_file"} {
		if !isLocalClientTool(pascalTool(t, name)) {
			t.Fatalf("%s must be treated as a local client tool", name)
		}
	}
	for _, name := range []string{"get_weather", "mcp__mysql__execute_query", "Agent", "WebSearch"} {
		if isLocalClientTool(pascalTool(t, name)) {
			t.Fatalf("%s must not be treated as a local client tool", name)
		}
	}
}
