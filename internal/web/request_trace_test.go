package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"m365-copilot2api/internal/chathub"
)

func TestMaskAccountIDNeverLeaksRawID(t *testing.T) {
	for _, raw := range []string{"user-abc", "user-with-long-identifier-0123456789abcdef"} {
		masked := maskAccountID(raw)
		if masked == "" {
			t.Fatalf("maskAccountID(%q) empty", raw)
		}
		if masked == raw {
			t.Fatalf("maskAccountID(%q) leaked raw ID", raw)
		}
		if len(masked) > accountIdentifierMaxSize+32 {
			t.Fatalf("maskAccountID(%q) too long: %q", raw, masked)
		}
	}
	if got := maskAccountID(""); got != "" {
		t.Fatalf("maskAccountID empty got %q", got)
	}
	if got := maskAccountID("  "); got != "" {
		t.Fatalf("maskAccountID whitespace got %q", got)
	}
}

func TestSetM365RequestHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	setM365RequestHeaders(rec, 120, 2, "secret-account-id")
	if got := rec.Header().Get(m365TimeoutHeader); got != "120" {
		t.Fatalf("timeout header=%q", got)
	}
	if got := rec.Header().Get(m365AttemptCountHeader); got != "2" {
		t.Fatalf("attempt header=%q", got)
	}
	acc := rec.Header().Get(m365AccountIDHeader)
	if acc == "" || acc == "secret-account-id" {
		t.Fatalf("account header leaked or empty: %q", acc)
	}
}

func TestRequestIDMiddlewareStoresAndSetsHeaders(t *testing.T) {
	h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFrom(r); got != "client-123" {
			t.Fatalf("requestIDFrom=%q", got)
		}
		if trace := chathub.TraceFromContext(r.Context()); trace == nil || trace.RequestID != "client-123" {
			t.Fatalf("trace missing or mismatched: %+v", trace)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(requestIDHeader, "client-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Header().Get(requestIDHeader); got != "client-123" {
		t.Fatalf("response request ID=%q", got)
	}
	if got := w.Header().Get(m365TimeoutHeader); got == "" {
		t.Fatal("timeout header missing on default response")
	}
	if got := w.Header().Get(m365AttemptCountHeader); got == "" {
		t.Fatal("attempt header missing on default response")
	}
}
