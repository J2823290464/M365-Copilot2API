package chathub

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolProtocolPromptDoesNotIndentProtocolMarkup(t *testing.T) {
	tool := Tool{
		Type: "function",
		Function: mustToolJSON(t, map[string]any{
			"name":        "shell_command",
			"description": "Run a command",
			"parameters":  map[string]any{"type": "object"},
		}),
	}

	prompt := toolProtocolPrompt("Run tests", []Tool{tool}, "auto")
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "    ") {
			t.Fatalf("prompt contains Go-source indentation: %q", line)
		}
	}
	if !strings.Contains(prompt, "\n<tools>\n") || !strings.Contains(prompt, "\n</tools>\n") {
		t.Fatalf("protocol markup must use unindented line breaks:\n%s", prompt)
	}
}

func mustToolJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal tool: %v", err)
	}
	return data
}
