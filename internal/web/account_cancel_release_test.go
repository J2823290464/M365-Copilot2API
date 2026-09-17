package web

import (
	"context"
	"m365-copilot2api/internal/auth"
	"m365-copilot2api/internal/chathub"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestChatCancelReleasesConcurrencySlot exercises the full web-layer path:
// chatWithAccountAttempt acquires the account slot, dials upstream, then the
// client cancels. The Chat call must return quickly and the slot must be
// released immediately so subsequent requests are not blocked.
func TestChatCancelReleasesConcurrencySlot(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	serverConnCh := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		select {
		case serverConnCh <- struct{}{}:
		default:
		}
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`+"\x1e")); err != nil {
					return
				}
			}
		}()
	}))
	defer server.Close()

	oldWSBase := chathub.SetWSBaseForTest("ws" + server.URL[len("http"):])
	defer chathub.SetWSBaseForTest(oldWSBase)

	dialer := *websocket.DefaultDialer
	chat := &chathub.Client{HTTPHeader: make(http.Header), Dialer: &dialer}

	rs := defaultRuntimeSettings()
	rs.AccountQueueTimeoutSeconds = 5
	store := &auth.Store{}
	store.Upsert(auth.TokenSet{HomeOID: "oid", TenantID: "tid", AccessToken: "token"})
	s := &Server{
		tokens:             store,
		accountConcurrency: newAccountConcurrency(),
		accountPool:        newAccountHealth(),
		chat:               chat,
		settings:           &settingsStore{v: rs},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = s.chatWithAccountAttempt(ctx, "oid", chathub.Account{AccessToken: "token", OID: "oid", TID: "tid"}, chathub.Request{Text: "hello"}, 1)
	}()

	select {
	case <-serverConnCh:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream connection was not established")
	}
	if inflight := s.accountConcurrency.Inflight("oid"); inflight != 1 {
		t.Fatalf("inflight = %d after connect, want 1", inflight)
	}

	start := time.Now()
	cancel()
	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("chatWithAccountAttempt took %v to return after cancel, want <= 2s", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("chatWithAccountAttempt did not return within 5s of cancel")
	}
	if inflight := s.accountConcurrency.Inflight("oid"); inflight != 0 {
		t.Fatalf("inflight = %d after cancel, want 0 (slot must be released)", inflight)
	}
}
