package web

import "testing"

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
