package web

import (
	"testing"
	"time"
)

func TestSessionEndsWithBlockedTurn(t *testing.T) {
	cases := []struct {
		name    string
		history []oaiMsg
		want    bool
	}{
		{
			name: "blocked marker as final assistant turn",
			history: []oaiMsg{
				{Role: "user", Content: "inspect the repo"},
				{Role: "assistant", Content: "<block>no</block>"},
			},
			want: true,
		},
		{
			name: "real answer after earlier block",
			history: []oaiMsg{
				{Role: "user", Content: "inspect the repo"},
				{Role: "assistant", Content: "<block>no</block>"},
				{Role: "user", Content: "try again"},
				{Role: "assistant", Content: "The repository contains three packages."},
			},
			want: false,
		},
		{
			name: "trailing tool result after a real answer",
			history: []oaiMsg{
				{Role: "user", Content: "list files"},
				{Role: "assistant", Content: "Found two files."},
				{Role: "tool", Content: "a.go\nb.go"},
			},
			want: false,
		},
		{
			name:    "no assistant turn at all",
			history: []oaiMsg{{Role: "user", Content: "hello"}},
			want:    false,
		},
		{
			name:    "empty history",
			history: nil,
			want:    false,
		},
	}
	for _, c := range cases {
		if got := sessionEndsWithBlockedTurn(sessionBinding{ContextHistory: c.history}); got != c.want {
			t.Fatalf("%s: got %t want %t", c.name, got, c.want)
		}
	}
}

// A session whose latest turn is a bare block marker must lose the recent
// fallback even when it is the most recent binding for the tenant.
func TestTenantFallbackRejectsPoisonedSession(t *testing.T) {
	sr := openSessionResolver()
	poisoned := sessionBinding{
		SessionID:      "sess-poisoned",
		ConversationID: "conv-poisoned",
		AccountID:      "acc1",
		Tenant:         "ten",
		ProjectID:      "",
		CreatedAt:      time.Now().UTC(),
		LastUsedAt:     time.Now().UTC(),
		ContextHistory: []oaiMsg{
			{Role: "user", Content: "inspect the repo"},
			{Role: "assistant", Content: "<block>no</block>"},
		},
	}
	sr.mu.Lock()
	sr.sessions["sess-poisoned"] = poisoned
	sr.mu.Unlock()
	if got := sr.recentActiveSessionForTenant("ten", "", "acc1"); got != "" {
		t.Fatalf("poisoned session must not win the fallback, got %q", got)
	}
}
