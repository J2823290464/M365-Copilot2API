package web

import "testing"

func TestSplitContextForCompressionPreservesCurrentTurn(t *testing.T) {
	messages := []oaiMsg{
		{Role: "system", Content: "rules"},
		{Role: "user", Content: "old request"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "current request"},
	}
	system, oldHistory, currentTurn, ok := splitContextForCompression(messages)
	if !ok || len(system) != 1 || len(oldHistory) != 2 || len(currentTurn) != 1 {
		t.Fatalf("unexpected split: ok=%t system=%d old=%d current=%d", ok, len(system), len(oldHistory), len(currentTurn))
	}
	if contentToString(currentTurn[0].Content) != "current request" {
		t.Fatalf("current turn was not preserved")
	}
}
