package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDAcceptsClientSuppliedID(t *testing.T) {
	h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := requestIDFrom(r); got != "client-supplied" {
			t.Fatalf("requestIDFrom=%q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(requestIDHeader, "client-supplied")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Header().Get(requestIDHeader); got != "client-supplied" {
		t.Fatalf("request ID header=%q, want client-supplied", got)
	}
}

func TestRequestIDNormalizesInvalidClientValue(t *testing.T) {
	for _, bad := range []string{"", "  ", "bad id!", "a\nb", string(make([]byte, 129))} {
		h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(requestIDHeader, bad)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		got := w.Header().Get(requestIDHeader)
		if got == "" || got == bad {
			t.Fatalf("bad value %q produced request ID %q", bad, got)
		}
	}
}

func TestRequestIDGeneratedByServerWhenMissing(t *testing.T) {
	h := requestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := w.Header().Get(requestIDHeader); got == "" {
			t.Fatal("request ID missing inside handler")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Header().Get(requestIDHeader); got == "" {
		t.Fatal("request ID missing in response")
	}
}
