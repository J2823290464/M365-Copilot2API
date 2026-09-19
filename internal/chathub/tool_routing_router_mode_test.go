package chathub

import (
	"strings"
	"testing"
)

// TestRouterModeAnswerTurnExposesClientTools replays the failing request
// shape: router mode with an MCP gateway configured, four selected Claude Code
// tools, and tool_choice=required. In router mode the gateway replaces the
// native plugin list, so the inlined prompt is the only channel left that can
// tell the model these tools exist. When the names were matched
// case-sensitively, that channel was empty and the model reported the
// workspace as missing instead of calling the tool.
func TestRouterModeAnswerTurnExposesClientTools(t *testing.T) {
	tools := []Tool{
		pascalTool(t, "Grep"),
		pascalTool(t, "Read"),
		pascalTool(t, "Bash"),
		pascalTool(t, "Write"),
	}

	native, promptTools := splitRoutedTools(tools)
	if len(native) != 0 {
		t.Fatalf("router mode must not send client-local tools as native plugins, got %v", toolNames(native))
	}
	if len(promptTools) != len(tools) {
		t.Fatalf("router mode must inline every client-local tool, got %v", toolNames(promptTools))
	}

	text := toolProtocolPrompt("inspect C:\\Workspace\\yhd-sale\\yhd-service-crm", promptTools, "required")
	for _, name := range []string{"Grep", "Read", "Bash", "Write"} {
		if !strings.Contains(text, "```"+name+"\n") {
			t.Fatalf("tool %s missing from the model-visible prompt:\n%s", name, text)
		}
	}
}
