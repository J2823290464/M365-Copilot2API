package chathub

import (
	"testing"
)

func TestDerivedTurnCleanupRunsAndIsOptional(t *testing.T) {
	called := 0
	c := NewClient()
	c.DeleteConversation = func(id string) error {
		called++
		if id != "conv-x" {
			t.Fatalf("unexpected id %q", id)
		}
		return nil
	}
	c.deleteConversation("conv-x")
	if called != 1 {
		t.Fatalf("expected delete to be invoked once, got %d", called)
	}

	// No callback installed (pool/standalone use) must not panic.
	plain := NewClient()
	plain.deleteConversation("conv-y")
	plain.deleteConversation("")
}
