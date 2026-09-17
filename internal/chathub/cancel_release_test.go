package chathub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestChatCancelReleasesConnection verifies the acceptance contract for
// request cancellation: once the client disconnects, the Chat goroutine must
// return within 1-2 seconds and the upstream connection must be closed so
// concurrency slots are released and pooled sockets never linger.
func TestChatCancelReleasesConnection(t *testing.T) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	serverConnCh := make(chan *websocket.Conn, 1)
	serverClosed := make(chan struct{})
	var closedOnce sync.Once

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		serverConnCh <- conn
		// Answer the SignalR handshake once, then hold the connection open
		// without any business traffic.
		go func() {
			defer func() {
				closedOnce.Do(func() { close(serverClosed) })
			}()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`+rs)); err != nil {
					return
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
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = client.Chat(ctx, Account{AccessToken: "token", OID: "oid", TID: "tid"}, Request{Text: "hello"})
	}()

	// Wait until the upstream server accepted the WebSocket connection.
	select {
	case <-serverConnCh:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream connection was not established")
	}

	// Simulate the client disconnecting.
	start := time.Now()
	cancel()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Chat goroutine took %v to exit after cancel, want <= 2s", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Chat goroutine did not exit within 5s of cancel")
	}

	select {
	case <-serverClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream connection was not closed after client cancel")
	}
}
