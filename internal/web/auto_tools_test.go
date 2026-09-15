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

	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"Read", "Edit"}) {
		t.Fatalf("names = %v, want [Read Edit]", names)
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

	if names := toolNames(t, got); !reflect.DeepEqual(names, []string{"Grep"}) {
		t.Fatalf("names = %v, want [Grep]", names)
	}
}
