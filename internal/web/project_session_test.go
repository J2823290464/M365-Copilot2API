package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func projectRequest(apiKey, projectID, sessionID string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+apiKey)
	if projectID != "" {
		r.Header.Set(projectHeaderName, projectID)
	}
	if sessionID != "" {
		r.Header.Set("X-M365-Session-Id", sessionID)
	}
	return r
}

func TestProjectSessionsRemainIndependent(t *testing.T) {
	sr := newTenantResolver(t)
	body := &oaiReq{Messages: []oaiMsg{{Role: "user", Content: "project context"}}}
	sr.Bind("local-a", "conv-project", "acc", body, "answer", projectRequest("key", "proj-a", "thread-a"))
	got := sr.Resolve(projectRequest("key", "proj-a", "thread-b"), body)
	if !got.IsNew {
		t.Fatalf("different local sessions must not reuse cloud conversation: %+v", got)
	}
	got = sr.Resolve(projectRequest("key", "proj-a", "thread-a"), body)
	if got.IsNew || got.ConversationID != "conv-project" {
		t.Fatalf("same local session should reuse its cloud conversation: %+v", got)
	}
}

func TestProjectAndNonProjectSessionsAreIsolated(t *testing.T) {
	sr := newTenantResolver(t)
	body := &oaiReq{Messages: []oaiMsg{{Role: "user", Content: "same content"}}}
	sr.Bind("project", "conv-project", "acc", body, "answer", projectRequest("key", "proj-a", "thread"))
	if got := sr.Resolve(projectRequest("key", "proj-b", "thread"), body); !got.IsNew {
		t.Fatalf("different project leaked conversation: %+v", got)
	}
	if got := sr.Resolve(projectRequest("key", "", "thread"), body); !got.IsNew {
		t.Fatalf("non-project session leaked project conversation: %+v", got)
	}
}

func TestResponsesNamespaceUsesProjectScope(t *testing.T) {
	a := projectRequest("key", "proj-a", "thread-a")
	b := projectRequest("key", "proj-a", "thread-b")
	c := projectRequest("key", "proj-b", "thread-a")
	if responseSessionID(a, nil) == responseSessionID(b, nil) {
		t.Fatal("different local sessions must not share Responses API scope")
	}
	if responseSessionID(a, nil) == responseSessionID(c, nil) {
		t.Fatal("different projects must not share Responses API scope")
	}
}

func TestRequestMetadataProvidesProjectAndThreadScope(t *testing.T) {
	body := &oaiReq{Metadata: &oaiMetadata{ProjectID: "project-a", ThreadID: "thread-a"}}
	r := projectRequest("key", "", "")
	if got := projectIDFromRequest(r, body); got != "project-a" {
		t.Fatalf("project metadata not recognized: %q", got)
	}
	if got := sessionIDFromRequest(r, body); got != "thread-a" {
		t.Fatalf("thread metadata not recognized: %q", got)
	}
}
