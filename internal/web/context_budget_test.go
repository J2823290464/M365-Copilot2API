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
