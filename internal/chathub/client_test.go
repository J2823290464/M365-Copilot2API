package chathub

import "testing"

func TestNewClientInitializesConnectionPool(t *testing.T) {
	client := NewClient()
	if client.Pool == nil {
		t.Fatal("NewClient() did not initialize the connection pool")
	}
	defer client.Pool.Close()

	if client.Pool.dialer != client.Dialer {
		t.Fatal("connection pool does not use the client's WebSocket dialer")
	}
	if got, want := client.Pool.header.Get("Origin"), client.HTTPHeader.Get("Origin"); got != want {
		t.Fatalf("connection pool Origin header = %q, want %q", got, want)
	}
}
