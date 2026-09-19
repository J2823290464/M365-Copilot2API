package web

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolverTenantRecentFallback(t *testing.T) {
	t.Setenv("M365_SESSION_CACHE", "")
	sr := openSessionResolver()

	// Create a session for tenant "tenant-a"
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req1.Header.Set("Authorization", "Bearer key-a")
	sr.Bind("", "conv-a", "acc1",
		&oaiReq{Messages: []oaiMsg{{Role: "user", Content: "hello"}}},
		"",
		req1)

	// Verify fallback finds the session when no explicit ID is provided
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req2.Header.Set("Authorization", "Bearer key-a")
	res := sr.Resolve(req2, &oaiReq{Messages: []oaiMsg{{Role: "user", Content: "completely different message"}, {Role: "assistant", Content: "response"}, {Role: "user", Content: "follow up"}}})
	if res.IsNew {
		t.Fatal("expected tenant recent fallback to match existing session")
	}
	if res.MatchedBy != "tenant_recent_fallback" {
		t.Fatalf("expected matched_by=tenant_recent_fallback, got %s", res.MatchedBy)
	}
	if res.ConversationID != "conv-a" {
		t.Fatalf("expected conversation=conv-a, got %s", res.ConversationID)
	}

	// Verify fallback does not match across tenants
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req3.Header.Set("Authorization", "Bearer key-b")
	res3 := sr.Resolve(req3, &oaiReq{Messages: []oaiMsg{{Role: "user", Content: "hello"}}})
	if !res3.IsNew {
		t.Fatal("expected no cross-tenant fallback match")
	}
}

func TestResolverTenantFallbackBindsAccount(t *testing.T) {
	t.Setenv("M365_SESSION_CACHE", "")
	sr := openSessionResolver()

	req1 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req1.Header.Set("Authorization", "Bearer key-a")
	sr.Bind("", "conv-a", "acc1",
		&oaiReq{Messages: []oaiMsg{{Role: "user", Content: "hello"}}},
		"",
		req1)

	loopMessages := []oaiMsg{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "response"},
		{Role: "user", Content: "follow up"},
	}

	// Same tenant but a different pinned account must not reuse acc1's session:
	// account routing happens after session resolution, so an unbound fallback
	// would pair acc1's conversation with a request served by acc2.
	reqOther := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqOther.Header.Set("Authorization", "Bearer key-a")
	resOther := sr.Resolve(reqOther, &oaiReq{AccountID: "acc2", Messages: loopMessages})
	if !resOther.IsNew {
		t.Fatalf("expected no fallback for a different account, got conversation=%s matched_by=%s", resOther.ConversationID, resOther.MatchedBy)
	}

	// The same pinned account still resumes normally.
	reqSame := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	reqSame.Header.Set("Authorization", "Bearer key-a")
	resSame := sr.Resolve(reqSame, &oaiReq{AccountID: "acc1", Messages: loopMessages})
	if resSame.IsNew || resSame.ConversationID != "conv-a" {
		t.Fatalf("expected same-account fallback to conv-a, got new=%t conversation=%s", resSame.IsNew, resSame.ConversationID)
	}
}

func TestResolverTenantFallbackRespectsTTL(t *testing.T) {
	t.Setenv("M365_SESSION_CACHE", "")
	sr := openSessionResolver()

	// Create a session
	req1 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req1.Header.Set("Authorization", "Bearer key-a")
	sr.Bind("", "conv-old", "acc1",
		&oaiReq{Messages: []oaiMsg{{Role: "user", Content: "old"}}},
		"",
		req1)

	// Age the session beyond 5 minutes
	sr.mu.Lock()
	for id, sess := range sr.sessions {
		sess.LastUsedAt = time.Now().UTC().Add(-6 * time.Minute)
		sr.sessions[id] = sess
	}
	sr.mu.Unlock()

	// Fallback should not match aged session
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req2.Header.Set("Authorization", "Bearer key-a")
	res := sr.Resolve(req2, &oaiReq{Messages: []oaiMsg{{Role: "user", Content: "new"}}})
	if !res.IsNew {
		t.Fatal("expected no fallback match for aged session")
	}
}
