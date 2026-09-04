package web

import "testing"

// Regression: /v1/messages (Anthropic) requests convert to oaiReq without a
// metadata block. session-trace logging dereferenced body.Metadata directly,
// panicking with "invalid memory address or nil pointer dereference". The
// lookup must be nil-safe.
func TestMetadataThreadIDNilSafe(t *testing.T) {
	if got := metadataThreadID(nil); got != "" {
		t.Fatalf("metadataThreadID(nil) = %q, want empty", got)
	}
	positive := metadataThreadID(&oaiMetadata{ThreadID: "thread-1", ThreadIDC: "thread-2"})
	if positive != "thread-1" {
		t.Fatalf("metadataThreadID(thread) = %q, want thread-1", positive)
	}
	camel := metadataThreadID(&oaiMetadata{ThreadIDC: "thread-2"})
	if camel != "thread-2" {
		t.Fatalf("metadataThreadID(threadId) = %q, want thread-2", camel)
	}
}
