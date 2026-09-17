package chathub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestBusinessIdleNotExtendedByPingOnly verifies that a keep-alive-only
// upstream (periodic SignalR pings with no business content) does NOT hold
// the request open forever: the business idle deadline is based on real
// progress, not on transport frames.
func TestBusinessIdleNotExtendedByPingOnly(t *testing.T) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		// Answer the handshake once, then keep sending only ping frames.
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`+rs)); err != nil {
			return
		}
		serverConnCh <- conn
		go func() {
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-time.After(3 * time.Second):
					return
				case <-ticker.C:
					if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":6}`+rs)); err != nil {
						return
					}
				}
			}
		}()
	}))
	defer server.Close()

	oldWSBase := wsBase
	wsBase = "ws" + server.URL[len("http"):]
	defer func() { wsBase = oldWSBase }()

	dialer := *websocket.DefaultDialer
	client := &Client{
		HTTPHeader: make(http.Header),
		Dialer:     &dialer,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.Chat(ctx, Account{AccessToken: "token", OID: "oid", TID: "tid"}, Request{Text: "hello", IdleTimeout: 500 * time.Millisecond})
		done <- err
	}()

	select {
	case <-serverConnCh:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream connection was not established")
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrUpstreamBusinessIdle) {
			t.Fatalf("expected ErrUpstreamBusinessIdle, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ping-only upstream kept the request alive past the business idle deadline")
	}
}

// TestBusinessProgressExtendsIdle verifies that real business frames (text
// deltas) keep postponing the business idle deadline, so a healthy stream is
// not killed by the watchdog while content is still flowing.
func TestBusinessProgressExtendsIdle(t *testing.T) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`+rs)); err != nil {
			return
		}
		serverConnCh <- conn
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			var snapshot strings.Builder
			for {
				select {
				case <-time.After(3 * time.Second):
					return
				case <-ticker.C:
					snapshot.WriteString("token ")
					frame := `{"type":1,"target":"update","arguments":[{"messages":[{"author":"bot","text":"` + snapshot.String() + `","messageType":""}]}]}` + rs
					if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
						return
					}
				}
			}
		}()
	}))
	defer server.Close()

	oldWSBase := wsBase
	wsBase = "ws" + server.URL[len("http"):]
	defer func() { wsBase = oldWSBase }()

	dialer := *websocket.DefaultDialer
	client := &Client{
		HTTPHeader: make(http.Header),
		Dialer:     &dialer,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.Chat(ctx, Account{AccessToken: "token", OID: "oid", TID: "tid"}, Request{Text: "hello", IdleTimeout: 400 * time.Millisecond})
		done <- err
	}()

	select {
	case <-serverConnCh:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream connection was not established")
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, ErrUpstreamBusinessIdle) {
			t.Fatalf("unexpected error: %v", err)
		}
		if err != nil {
			t.Fatalf("business progress did not prevent idle timeout: %v", err)
		}
	case <-time.After(1500 * time.Millisecond):
		// Still streaming after 1.5s > idle timeout: business frames worked.
		cancel()
	}
}
