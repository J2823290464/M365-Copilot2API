package web

import (
	"encoding/json"
	"reflect"
	"testing"

	"m365-copilot2api/internal/chathub"
)

func testClientTool(t *testing.T, name, toolType string) chathub.Tool {
	t.Helper()
	functionJSON, err := json.Marshal(map[string]any{
		"name":        name,
		"description": "client-declared tool",
		"parameters": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return chathub.Tool{Type: toolType, Function: functionJSON}
}

func toolNames(t *testing.T, tools []chathub.Tool) []string {
	t.Helper()
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		definition := parseClientToolDefinition(tool)
		if definition == nil {
			t.Fatalf("invalid tool: %#v", tool)
		}
		names = append(names, definition.Name)
	}
	return names
}

func TestAutoInjectCodexUsesDeclaredCustomExec(t *testing.T) {
	tools := []chathub.Tool{testClientTool(t, "exec", "custom")}
	got := autoSelectClientTools("edit the project", tools, nil)

	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"exec"}) {
		t.Fatalf("names = %v, want [exec]", names)
	}
}

func TestAutoInjectClaudeUsesDeclaredNamedTools(t *testing.T) {
	tools := []chathub.Tool{
		testClientTool(t, "Read", "function"),
		testClientTool(t, "Grep", "function"),
		testClientTool(t, "Edit", "function"),
		testClientTool(t, "Bash", "function"),
	}
	got := autoSelectClientTools("edit the project", tools, nil)

	// The write intent expands to the search/read/write bundle so the model
	// can locate, inspect and modify the target in one turn.
	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"Read", "Grep", "Edit"}) {
		t.Fatalf("names = %v, want [Read Grep Edit]", names)
	}
}

func TestAutoInjectOpenAICanUseShellBridge(t *testing.T) {
	tools := []chathub.Tool{testClientTool(t, "shell_command", "function")}
	got := autoSelectClientTools("run go test", tools, nil)

	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"shell_command"}) {
		t.Fatalf("names = %v, want [shell_command]", names)
	}
}

func TestAutoInjectWithoutExecutableCapabilityInjectsNothing(t *testing.T) {
	tools := []chathub.Tool{testClientTool(t, "get_weather", "function")}
	got := autoSelectClientTools("edit the project", tools, nil)

	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"get_weather"}) {
		t.Fatalf("names = %v, want original weather tool only", names)
	}
}

func TestAutoInjectKeepsOnlyIntentMatchingTools(t *testing.T) {
	tools := []chathub.Tool{
		testClientTool(t, "Read", "function"),
		testClientTool(t, "Grep", "function"),
		testClientTool(t, "Bash", "function"),
		testClientTool(t, "get_weather", "function"),
	}
	got := autoSelectClientTools("search the project", tools, nil)

	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"Read", "Grep"}) {
		t.Fatalf("names = %v, want [Read Grep]", names)
	}
}

func TestAutoSelectExpandsEditToReadWriteBundle(t *testing.T) {
	tools := []chathub.Tool{
		testClientTool(t, "Read", "function"),
		testClientTool(t, "Write", "function"),
		testClientTool(t, "Grep", "function"),
		testClientTool(t, "Bash", "function"),
		testClientTool(t, "get_weather", "function"),
	}
	got := autoSelectClientTools("edit the project configuration", tools, nil)

	names := toolNames(t, got)
	want := []string{"Read", "Write", "Grep"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestLocalClientToolNameRecognition(t *testing.T) {
	for _, name := range []string{
		"Grep", "Glob", "Read", "Write", "Edit", "Bash", "shell_command",
		"search_files", "read_file", "write_file", "apply_patch", "WebSearch",
		"mcp__mysql_execute_query",
	} {
		if name == "mcp__mysql_execute_query" && isLocalClientToolName(name) {
			t.Fatalf("mcp__ tool %q must not be treated as a local client tool", name)
		}
		if name != "mcp__mysql_execute_query" && !isLocalClientToolName(name) {
			t.Fatalf("local client tool %q was not recognized", name)
		}
	}
}

func TestDetectClientKind(t *testing.T) {
	cases := []struct {
		name  string
		tools []string
		want  clientKind
	}{
		{"codex", []string{"Bash", "Read", "Write", "Grep"}, clientKindCodex},
		{"codex_desktop", []string{"Agent", "Bash", "Read", "Write"}, clientKindCodexDesk},
		{"opencode", []string{"shell_command", "read_file", "write_file"}, clientKindOpencode},
		{"claude", []string{"Bash", "Read", "Write", "Glob", "Grep", "TodoWrite"}, clientKindClaude},
		{"generic", []string{"get_weather", "get_time"}, clientKindGeneric},
	}
	for _, tc := range cases {
		if got := detectClientKind(tc.tools); got != tc.want {
			t.Fatalf("detectClientKind(%v) = %s, want %s", tc.tools, got, tc.want)
		}
	}
}

func TestInjectFallbackCodexFillsWriteIntent(t *testing.T) {
	// Codex declares only Bash/Read; "edit" intent needs a write tool.
	tools := []chathub.Tool{testClientTool(t, "Bash", "function"), testClientTool(t, "Read", "function")}
	declared := toolNames(t, tools)
	got := injectFallbackTools(clientKindCodex, declared, []clientToolKind{clientToolKindWrite})
	names := toolNames(t, got)

	if len(names) != 1 || names[0] != "Write" {
		t.Fatalf("injected = %v, want [Write]", names)
	}
}

func TestInjectFallbackSkipsDeclaredAndMCP(t *testing.T) {
	// Claude already declares Bash/Read/Write; intent read/write must inject
	// nothing new. mcp__ tools are never touched.
	tools := []chathub.Tool{
		testClientTool(t, "Bash", "function"),
		testClientTool(t, "Read", "function"),
		testClientTool(t, "Write", "function"),
	}
	declared := toolNames(t, tools)
	if got := injectFallbackTools(clientKindClaude, declared, []clientToolKind{clientToolKindRead, clientToolKindWrite}); len(got) != 0 {
		t.Fatalf("expected no injection, got %v", toolNames(t, got))
	}

	// Generic clients never get fallback tools.
	if got := injectFallbackTools(clientKindGeneric, []string{"Bash"}, []clientToolKind{clientToolKindWrite}); len(got) != 0 {
		t.Fatalf("generic should not inject, got %v", toolNames(t, got))
	}

	// A client that only declared mcp__ tools has no local surface: fallback
	// must not inject, mcp tools stay on the gateway.
	if got := injectFallbackTools(clientKindCodex, []string{"mcp__mysql_execute_query"}, []clientToolKind{clientToolKindSearch}); len(got) != 0 {
		t.Fatalf("mcp-only client should not get fallback tools, got %v", toolNames(t, got))
	}

	// Codex declares a local Bash plus mcp tools; write intent still gets a
	// fallback Write, and mcp__ names are never injected.
	tools2 := []chathub.Tool{testClientTool(t, "Bash", "function"), testClientTool(t, "mcp__mysql_execute_query", "function")}
	declared2 := toolNames(t, tools2)
	got2 := injectFallbackTools(clientKindCodex, declared2, []clientToolKind{clientToolKindWrite})
	names2 := toolNames(t, got2)
	if len(names2) != 1 || names2[0] != "Write" {
		t.Fatalf("got %v, want [Write]", names2)
	}
}

func TestRouteClientToolsPassThroughKeepsAll(t *testing.T) {
	tools := []chathub.Tool{
		testClientTool(t, "get_weather", "function"),
		testClientTool(t, "Bash", "function"),
		testClientTool(t, "mcp__mysql_execute_query", "function"),
	}
	got := routeClientTools(true, "edit the project", tools)
	if !reflect.DeepEqual(got, tools) {
		t.Fatalf("pass-through must return tools unchanged, got %v", toolNames(t, got))
	}
}

func TestRouteClientToolsFilterInjectsMissingBase(t *testing.T) {
	// Codex declares only Bash; "edit" intent filters to writable scope and
	// injects a fallback Write (not Grep/Read) because they are uncovered.
	tools := []chathub.Tool{testClientTool(t, "Bash", "function"), testClientTool(t, "get_weather", "function")}
	got := routeClientTools(false, "edit the project", tools)
	names := toolNames(t, got)

	hasWrite := false
	for _, name := range names {
		if name == "Write" {
			hasWrite = true
		}
	}
	if !hasWrite {
		t.Fatalf("filter mode should inject Write for write intent, got %v", names)
	}
	if containsString(names, "get_weather") {
		t.Fatalf("non-matching declared tools should be filtered, got %v", names)
	}
}
