package web

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseModelToolDecisionAutoAndParallel(t *testing.T) {
	calls, ok := parseModelToolDecision(`{"calls":[{"name":"get_weather","arguments":{"city":"Beijing"}},{"name":"get_time","arguments":{"city":"Beijing"}}]}`, testTools(), "auto")
	if !ok || len(calls) != 2 {
		t.Fatalf("calls=%v ok=%v", calls, ok)
	}
}
func TestParseModelToolDecisionNoCall(t *testing.T) {
	calls, ok := parseModelToolDecision(`{"calls":[]}`, testTools(), "auto")
	if !ok || len(calls) != 0 {
		t.Fatalf("calls=%v ok=%v", calls, ok)
	}
}
func TestModelToolRouterPromptMarksCompletedResults(t *testing.T) {
	p := modelToolRouterPrompt(`assistant tool_calls: [...]
tool[call_x]: 2026-07-18`, testTools(), "auto")
	if !strings.Contains(p, "Completed evidence must not be repeated") || !strings.Contains(p, "tool[call_x]: 2026-07-18") || !strings.Contains(p, "unfinished work remains") {
		t.Fatalf("missing multi-turn evidence constraint: %s", p)
	}
}

func TestModelToolRouterPromptBatchesReadOnlyOperations(t *testing.T) {
	p := modelToolRouterPrompt("inspect the project", testTools(), "auto")
	for _, phrase := range []string{"Batch independent read-only operations", "file search", "Keep dependent operations"} {
		if !strings.Contains(p, phrase) {
			t.Fatalf("missing batching guidance %q: %s", phrase, p)
		}
	}
}

func TestParseModelToolDecisionRejectsBadSchema(t *testing.T) {
	calls, ok := parseModelToolDecision("```json\n{\"calls\":[{\"name\":\"get_weather\",\"arguments\":{\"city\":2}}]}\n```", testTools(), "auto")
	if !ok || len(calls) != 0 {
		t.Fatalf("calls=%v ok=%v", calls, ok)
	}
}

func TestLocalToolIntent(t *testing.T) {
	if !localToolIntent("请检查 C:\\Workspace\\demo 项目源码") {
		t.Fatal("expected local task intent")
	}
	if localToolIntent("解释一下什么是接口") {
		t.Fatal("unexpected local task intent")
	}
}

func TestHasClientWorkspaceTool(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}}
	if !hasClientWorkspaceTool(tools) {
		t.Fatal("expected client workspace tool")
	}
}

func TestHasClientWorkspaceToolRejectsBroadCodeNames(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "code_interpreter"}}}
	if hasClientWorkspaceTool(tools) {
		t.Fatal("code_interpreter must not be treated as a client workspace tool")
	}
}

func TestHasClientWorkspaceToolAcceptsExplicitMetadata(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "run_task", "metadata": map[string]any{"execution_target": "client"}}}}
	if !hasClientWorkspaceTool(tools) {
		t.Fatal("explicit client metadata was not recognized")
	}
}

func TestModelToolRouterPromptStaysWithinBudget(t *testing.T) {
	tools := make([]map[string]any, 0, 50)
	for i := 0; i < 50; i++ {
		tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
			"name":        fmt.Sprintf("tool_%02d", i),
			"description": strings.Repeat("long description ", 40),
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []any{"a"}},
		}})
	}
	history := strings.Repeat("[user] earlier turn with lots of context\n", 4000)
	p := modelToolRouterPrompt(history, tools, "auto")
	if len(p) > maxRouterContextChars+16384 {
		t.Fatalf("router prompt too large: %d", len(p))
	}
	if !strings.Contains(p, "MODE: auto") || !strings.Contains(p, "older history omitted") {
		t.Fatalf("trimming markers missing: %d", len(p))
	}
	for _, name := range []string{"tool_00", "tool_49"} {
		if !strings.Contains(p, name) {
			t.Fatalf("compact tool defs missing %s", name)
		}
	}
}
