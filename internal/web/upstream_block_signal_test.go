package web

import (
	"encoding/json"
	"strings"
	"testing"

	"m365-copilot2api/internal/chathub"
)

func TestIsUpstreamBlockedSignalWrapper(t *testing.T) {
	if !isUpstreamBlockedSignal("<block>no</block>") {
		t.Fatal("expected bare block marker to be detected")
	}
	if isUpstreamBlockedSignal("M365 answered normally") {
		t.Fatal("normal text must not be treated as a block signal")
	}
	if isUpstreamBlockedSignal("很抱歉，我无法响应。我可以提供其他方面的帮助吗？") {
		t.Fatal("content-policy refusal has its own detector")
	}
}

// The rejection log records the request shape, so it must reflect the real
// upstream payload rather than a recomputed guess.
func TestUpstreamBlockDiagnosticsShape(t *testing.T) {
	body := oaiReq{
		Messages: []oaiMsg{{Role: "user", Content: "hello"}},
		Tools: []chathub.Tool{
			{Type: "function", Function: json.RawMessage(`{"name":"Read","description":"read a file","parameters":{"type":"object"}}`)},
		},
	}
	got := upstreamBlockDiagnostics(chathub.Result{Text: "<block>no</block>"}, "abcd", body)
	for _, want := range []string{"prompt_len=4", "messages=1", "declared_tools=1", "plugins=1", "upstream_bytes=17"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics %q missing %q", got, want)
		}
	}

	empty := upstreamBlockDiagnostics(chathub.Result{}, "", oaiReq{})
	if !strings.Contains(empty, "declared_tools=0") || !strings.Contains(empty, "plugins=1") {
		t.Fatalf("tool-less request must report the injected built-in plugin, got %q", empty)
	}
}
