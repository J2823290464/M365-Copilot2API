package web

import (
	"testing"

	"m365-copilot2api/internal/chathub"
)

func TestLocalTitleRequest(t *testing.T) {
	body := &oaiReq{Messages: []oaiMsg{
		{Role: "system", Content: "Generate a concise title"},
		{Role: "user", Content: "Return a JSON title for reviewing README.md"},
	}}
	if !localTitleRequest(body, "Generate a concise conversation title as JSON") {
		t.Fatal("title request was not detected")
	}
	if got := localTitle(body.Messages); got != "README.md" {
		t.Fatalf("title=%q", got)
	}
	body.Tools = []chathub.Tool{{Type: "function"}}
	if localTitleRequest(body, "Generate a concise conversation title as JSON") {
		t.Fatal("request with tools was classified as a local title request")
	}
}
