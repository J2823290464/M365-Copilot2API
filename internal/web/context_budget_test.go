package web

import (
	"strings"
	"testing"
)

func TestSlidingWindowTruncatesLongAgentHistory(t *testing.T) {
	messages := []oaiMsg{{Role: "system", Content: "stable instructions"}}
	for i := 0; i < 80; i++ {
		messages = append(messages,
			oaiMsg{Role: "user", Content: strings.Repeat("user context ", 80)},
			oaiMsg{Role: "assistant", Content: strings.Repeat("assistant context ", 80)},
		)
	}
	messages = append(messages, oaiMsg{Role: "user", Content: "current task"})
	trimmed, truncated, err := slidingWindow(messages, 4096)
	if err != nil {
		t.Fatalf("slidingWindow returned error: %v", err)
	}
	if !truncated || len(trimmed) >= len(messages) {
		t.Fatalf("window did not truncate history: truncated=%t got=%d want<%d", truncated, len(trimmed), len(messages))
	}
	if contentToString(trimmed[0].Content) != "stable instructions" || contentToString(trimmed[len(trimmed)-1].Content) != "current task" {
		t.Fatal("window must preserve system instructions and current task")
	}
}

func TestSlidingWindowSoftTruncatesOversizedToolResult(t *testing.T) {
	messages := []oaiMsg{
		{Role: "system", Content: "stable instructions"},
		{Role: "user", Content: "current task"},
		{Role: "assistant", Content: nil, ToolCalls: []map[string]any{{"id": "call_1", "type": "function", "function": map[string]any{"name": "search", "arguments": "{}"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: strings.Repeat("result data ", 4000)},
	}
	trimmed, truncated, err := slidingWindow(messages, 200)
	if err != nil {
		t.Fatalf("slidingWindow returned error instead of soft truncation: %v", err)
	}
	if !truncated {
		t.Fatal("window should mark the oversized tool result as truncated")
	}
	if len(trimmed) == 0 || contentToString(trimmed[0].Content) != "stable instructions" {
		t.Fatal("soft truncation must preserve system instructions")
	}
	last := trimmed[len(trimmed)-1]
	if last.Role != "tool" {
		t.Fatalf("last message should remain the tool result, got role=%q", last.Role)
	}
	got := contentToString(last.Content)
	if len(got) >= len("result data ")*4000 {
		t.Fatalf("tool result was not shortened: len=%d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("truncated tool result should carry a marker, got %q", got)
	}
}

func TestSlidingWindowKeepsErrorWhenNothingToClamp(t *testing.T) {
	messages := []oaiMsg{
		{Role: "system", Content: strings.Repeat("very long system instructions ", 5000)},
		{Role: "user", Content: "task"},
	}
	_, _, err := slidingWindow(messages, 64)
	if err == nil {
		t.Fatal("window must return context_length_exceeded when no tool result can be clamped")
	}
}
