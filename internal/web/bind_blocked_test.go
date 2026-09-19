package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"m365-copilot2api/internal/auth"
	"m365-copilot2api/internal/chathub"
)

// A blocked upstream turn must not be registered as a resumable conversation.
// Before this guard, the bare <block>no</block> marker was bound into both the
// session resolver and the conversation manager, so the next request in the
// same session resumed a conversation M365 had already rejected.
func TestBindConversationSkipsUpstreamBlockedTurn(t *testing.T) {
	s := &Server{
		sessionResolver:     openSessionResolver(),
		conversationManager: openConversationManager(),
	}
	body := &oaiReq{}
	res := chathub.Result{
		Text:           "<block>no</block>",
		ConversationID: "conv-blocked",
		SessionID:      "sess-blocked",
	}
	if !isUpstreamBlockedSignal(res.Text) {
		t.Fatal("fixture must look like an upstream block marker")
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s.bindConversation(auth.AccountToken{ID: "acc-blocked"}, body, req, res, "prompt", time.Now())
	if s.conversationManager.IsWhitelisted("conv-blocked") {
		t.Fatal("blocked conversation must not be registered")
	}
	for _, sess := range s.sessionResolver.sessions {
		if sess.ConversationID == "conv-blocked" {
			t.Fatalf("blocked conversation leaked into resolver: %+v", sess)
		}
	}
}
