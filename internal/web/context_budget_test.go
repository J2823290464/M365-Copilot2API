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

func TestSplitContextForCompressionHandlesToolLoop(t *testing.T) {
	messages := []oaiMsg{
		{Role: "system", Content: "stable instructions"},
		{Role: "user", Content: "inspect the repository"},
		{Role: "assistant", Content: nil, ToolCalls: []map[string]any{{"id": "call_1", "type": "function", "function": map[string]any{"name": "Grep", "arguments": "{}"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: "match result"},
		{Role: "assistant", Content: nil, ToolCalls: []map[string]any{{"id": "call_2", "type": "function", "function": map[string]any{"name": "Read", "arguments": "{}"}}}},
		{Role: "tool", ToolCallID: "call_2", Content: "file body"},
	}
	system, oldHistory, currentTurn, ok := splitContextForCompression(messages)
	// A single user turn that already ran several tool rounds must still be
	// compressible: the completed rounds become oldHistory and only the last
	// round stays live.
	if !ok {
		t.Fatal("multi-round tool loop should be compressible")
	}
	if len(system) != 1 || contentToString(system[0].Content) != "stable instructions" {
		t.Fatalf("system messages = %v", system)
	}
	if len(oldHistory) == 0 || contentToString(oldHistory[0].Content) != "inspect the repository" {
		t.Fatalf("completed rounds must be compressible, got %#v", oldHistory)
	}
	if len(currentTurn) != 2 || !isAssistantToolCall(currentTurn[0]) {
		t.Fatalf("live turn must keep only the trailing round, got %#v", currentTurn)
	}
}

func TestSplitContextForCompressionSeparatesCompletedRounds(t *testing.T) {
	messages := []oaiMsg{
		{Role: "system", Content: "stable instructions"},
		{Role: "user", Content: "first request"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second request"},
		{Role: "assistant", Content: nil, ToolCalls: []map[string]any{{"id": "call_1", "type": "function", "function": map[string]any{"name": "Grep", "arguments": "{}"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: "search output"},
	}
	system, oldHistory, currentTurn, ok := splitContextForCompression(messages)
	if !ok {
		t.Fatal("completed rounds plus a live turn should be compressible")
	}
	// "first request"/"first answer" plus the completed part of the live turn.
	if len(system) != 1 || len(oldHistory) != 3 {
		t.Fatalf("system=%d oldHistory=%d, want 1/3", len(system), len(oldHistory))
	}
	if contentToString(oldHistory[0].Content) != "first request" {
		t.Fatalf("oldest completed turn must be compressible, got %#v", oldHistory)
	}
	if len(currentTurn) != 2 || !isAssistantToolCall(currentTurn[0]) {
		t.Fatalf("currentTurn should cover only the trailing round, got %#v", currentTurn)
	}
}

func TestClampMessagesToByteBudgetTrimsOldestGroups(t *testing.T) {
	messages := []oaiMsg{{Role: "system", Content: "stable instructions"}}
	for i := 0; i < 40; i++ {
		messages = append(messages,
			oaiMsg{Role: "user", Content: strings.Repeat("history payload ", 400)},
			oaiMsg{Role: "assistant", Content: strings.Repeat("assistant payload ", 400)},
		)
	}
	messages = append(messages, oaiMsg{Role: "user", Content: "live request"})

	before := serializedMessagesBytes(messages)
	clamped, didClamp := clampMessagesToByteBudget(messages, 32*1024)
	if !didClamp {
		t.Fatalf("expected clamp for %d bytes", before)
	}
	after := serializedMessagesBytes(clamped)
	if after > 32*1024 {
		t.Fatalf("clamped size = %d, want <= %d", after, 32*1024)
	}
	if contentToString(clamped[0].Content) != "stable instructions" {
		t.Fatal("clamp must preserve leading system instructions")
	}
	if contentToString(clamped[len(clamped)-1].Content) != "live request" {
		t.Fatalf("clamp must preserve the live turn, got %q", contentToString(clamped[len(clamped)-1].Content))
	}
}

func TestClampMessagesToByteBudgetNoopWhenUnderLimit(t *testing.T) {
	messages := []oaiMsg{{Role: "user", Content: "small"}}
	clamped, didClamp := clampMessagesToByteBudget(messages, 32*1024)
	if didClamp {
		t.Fatal("small payload must not be clamped")
	}
	if len(clamped) != 1 {
		t.Fatalf("payload changed: %#v", clamped)
	}
}
