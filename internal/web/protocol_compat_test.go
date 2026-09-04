package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestResponsesToOpenAI(t *testing.T) {
	r := responsesRequest{Model: "m", Input: "what time", Tools: []map[string]any{{"type": "function", "name": "clock", "parameters": map[string]any{"type": "object"}}}}
	o, err := r.openAI()
	if err != nil || len(o.Messages) != 1 || len(o.Tools) != 1 {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestResponsesCustomExecToOpenAI(t *testing.T) {
	r := responsesRequest{Model: "m", Input: "inspect", Tools: []map[string]any{{"type": "custom", "name": "exec", "description": "run a command", "format": map[string]any{"type": "grammar"}}}}
	o, err := r.openAI()
	if err != nil || len(o.Tools) != 1 || o.Tools[0].Type != "custom" {
		t.Fatalf("tools=%+v err=%v", o.Tools, err)
	}
	if string(o.Tools[0].Function) == "" || !containsJSON(o.Tools[0].Function, "input") {
		t.Fatalf("custom exec did not receive an input schema: %s", o.Tools[0].Function)
	}
}

func TestResponsesCustomExecIsExclusiveTool(t *testing.T) {
	r := responsesRequest{Input: "edit the project", Tools: []map[string]any{
		{"type": "custom", "name": "exec", "description": "local execution"},
		{"type": "function", "name": "m365_search", "description": "native search"},
	}}
	o, err := r.openAI()
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Tools) != 1 || o.Tools[0].Type != "custom" {
		t.Fatalf("tools=%#v, want only custom exec", o.Tools)
	}
	if !strings.Contains(fmt.Sprint(o.Messages[0].Content), "Never use") {
		t.Fatalf("missing native-tool prohibition: %#v", o.Messages)
	}
}

func TestResponsesInstructionsAndCustomExecPolicyAreSystemMessages(t *testing.T) {
	r := responsesRequest{
		Instructions: "Use the repository selected by the caller.",
		Input:        "inspect the repository",
		Tools:        []map[string]any{{"type": "custom", "name": "exec", "description": "run a command"}},
	}
	o, err := r.openAI()
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Messages) != 3 {
		t.Fatalf("messages=%#v", o.Messages)
	}
	if o.Messages[0].Role != "system" || o.Messages[0].Content != customExecWorkspaceInstruction {
		t.Fatalf("missing custom exec policy: %#v", o.Messages[0])
	}
	if o.Messages[1].Role != "system" || o.Messages[1].Content != r.Instructions {
		t.Fatalf("instructions not preserved: %#v", o.Messages[1])
	}
	if o.Messages[2].Role != "user" || o.Messages[2].Content != r.Input {
		t.Fatalf("input ordering changed: %#v", o.Messages[2])
	}
}

func TestResponsesCustomToolOutputToOpenAI(t *testing.T) {
	r := responsesRequest{Input: []any{
		map[string]any{"type": "custom_tool_call", "call_id": "call_exec", "name": "exec", "input": "uname -s"},
		map[string]any{"type": "custom_tool_call_output", "call_id": "call_exec", "output": "Linux"},
	}}
	o, err := r.openAI()
	if err != nil || len(o.Messages) != 2 || o.Messages[0].Role != "assistant" || o.Messages[0].ToolCalls[0]["type"] != "custom" || o.Messages[1].Role != "tool" || o.Messages[1].ToolCallID != "call_exec" {
		t.Fatalf("messages=%+v err=%v", o.Messages, err)
	}
	if err := validateToolConversation(o.Messages); err != nil {
		t.Fatalf("custom tool continuation rejected: %v", err)
	}
}

func TestAnthropicToOpenAI(t *testing.T) {
	r := anthropicRequest{Model: "m", System: any("be concise"), Messages: []anthropicMessage{{Role: "user", Content: any("weather")}}, Tools: []anthropicTool{{Name: "weather", InputSchema: map[string]any{"type": "object"}}}}
	o, err := r.openAI()
	if err != nil || len(o.Messages) != 2 || len(o.Tools) != 1 {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestAnthropicToolResult(t *testing.T) {
	r := anthropicRequest{Messages: []anthropicMessage{{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": "x", "name": "f", "input": map[string]any{}}}}, {Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": "x", "content": "ok"}}}}}
	o, err := r.openAI()
	if err != nil || len(o.Messages) != 2 || o.Messages[1].ToolCallID != "x" {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestAnthropicUserAndMetadataToOpenAI(t *testing.T) {
	r := anthropicRequest{
		Model:    "m",
		Messages: []anthropicMessage{{Role: "user", Content: any("hi")}},
		User:     "user-abc",
		Metadata: &oaiMetadata{ProjectID: "proj-1", ThreadID: "thread-9", SessionID: "sess-7"},
	}
	o, err := r.openAI()
	if err != nil {
		t.Fatal(err)
	}
	if o.User != "user-abc" {
		t.Fatalf("user=%q, want user-abc", o.User)
	}
	if o.Metadata == nil || o.Metadata.ProjectID != "proj-1" || o.Metadata.ThreadID != "thread-9" || o.Metadata.SessionID != "sess-7" {
		t.Fatalf("metadata=%#v, want project/thread/session preserved", o.Metadata)
	}
}

func TestAnthropicMetadataCamelCaseResolvesFromRequest(t *testing.T) {
	// Claude/AI SDKs often send camelCase keys inside metadata; both snake_case
	// and camelCase must survive the Anthropic -> openAI round trip so the
	// session resolver can read project/thread.
	raw := `{"model":"m","messages":[{"role":"user","content":"hi"}],"metadata":{"projectId":"p-c","threadId":"t-c","sessionId":"s-c"}}`
	var ar anthropicRequest
	if err := json.Unmarshal([]byte(raw), &ar); err != nil {
		t.Fatal(err)
	}
	o, err := ar.openAI()
	if err != nil {
		t.Fatal(err)
	}
	if o.Metadata == nil || o.Metadata.ProjectIDC != "p-c" || o.Metadata.ThreadIDC != "t-c" || o.Metadata.SessionIDC != "s-c" {
		t.Fatalf("metadata=%#v", o.Metadata)
	}
	if got := projectIDFromRequest(&http.Request{Header: make(http.Header)}, &o); got != "p-c" {
		t.Fatalf("projectID=%q, want p-c", got)
	}
	if got := sessionIDFromRequest(&http.Request{Header: make(http.Header)}, &o); got != "s-c" {
		t.Fatalf("sessionID=%q, want s-c (thread= %q)", got, metadataThreadID(o.Metadata))
	}
}
