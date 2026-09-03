package web

import (
	"testing"
	"time"

	"m365-copilot2api/internal/chathub"
)

func TestConversationCacheUsesSessionFingerprint(t *testing.T) {
	cache := newConversationCache()
	messages := []oaiMsg{{Role: "system", Content: "dynamic system"}, {Role: "user", Content: "initial task"}}
	finger := sessionFingerprint(messages)
	entry := &cachedConversation{ConversationID: "conv-1", MessageCount: len(messages), LastUsedAt: time.Now()}
	cache.Store("account-1", "claude-opus-5", finger, entry)

	if got := cache.Lookup("account-1", "claude-opus-5", sessionFingerprint(append(messages, oaiMsg{Role: "assistant", Content: "answer"}))); got == nil {
		t.Fatal("expected growing history to reuse the same cached session")
	}
	if got := cache.Lookup("account-1", "claude-opus-5", sessionFingerprint([]oaiMsg{{Role: "user", Content: "different task"}})); got != nil {
		t.Fatal("different initial tasks must not share a cached session")
	}
}

func TestStoreConvCacheTracksFullHistory(t *testing.T) {
	s := &Server{convCache: newConversationCache()}
	messages := []oaiMsg{{Role: "system", Content: "system"}, {Role: "user", Content: "task"}, {Role: "assistant", Content: "answer"}}
	s.storeConvCache("account-1", "gpt-5.6-sol", chathub.Result{ConversationID: "conv-1"}, "tone", messages, false)
	entry := s.convCache.Lookup("account-1", "gpt-5.6-sol", sessionFingerprint(messages))
	if entry == nil || entry.MessageCount != len(messages) {
		t.Fatalf("cache entry = %#v, want message count %d", entry, len(messages))
	}
}
