package chathub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type traceKeyType struct{}

// RequestTrace keeps transport and business progress separate. Transport
// frames and pings keep the socket alive, while only content, tool activity,
// or explicit state changes postpone the business idle deadline.
type RequestTrace struct {
	RequestID string

	mu                   sync.Mutex
	lastTransport        time.Time
	lastBusiness         time.Time
	firstBusiness        time.Time
	websocketConnected   time.Time
	firstServiceResponse time.Time
	transportFrames      atomic.Uint64
	businessEvents       atomic.Uint64
}

func WithTrace(ctx context.Context, trace *RequestTrace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, traceKeyType{}, trace)
}

func TraceFromContext(ctx context.Context) *RequestTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceKeyType{}).(*RequestTrace)
	return trace
}

func (t *RequestTrace) now() time.Time { return time.Now() }

func (t *RequestTrace) MarkTransport() {
	if t == nil {
		return
	}
	t.transportFrames.Add(1)
	t.mu.Lock()
	t.lastTransport = t.now()
	t.mu.Unlock()
}

func (t *RequestTrace) MarkWebsocketConnected() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.websocketConnected = t.now()
	t.mu.Unlock()
}

func (t *RequestTrace) MarkFirstServiceResponse() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.firstServiceResponse.IsZero() {
		t.firstServiceResponse = t.now()
	}
	t.mu.Unlock()
}

func (t *RequestTrace) MarkBusiness() {
	if t == nil {
		return
	}
	t.businessEvents.Add(1)
	now := t.now()
	t.mu.Lock()
	t.lastBusiness = now
	if t.firstBusiness.IsZero() {
		t.firstBusiness = now
	}
	t.mu.Unlock()
}

func (t *RequestTrace) Snapshot() (lastTransport, lastBusiness time.Time, transportFrames, businessEvents uint64) {
	if t == nil {
		return time.Time{}, time.Time{}, 0, 0
	}
	t.mu.Lock()
	lastTransport, lastBusiness = t.lastTransport, t.lastBusiness
	t.mu.Unlock()
	return lastTransport, lastBusiness, t.transportFrames.Load(), t.businessEvents.Load()
}

func (t *RequestTrace) LastBusiness() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastBusiness
}

func (t *RequestTrace) FirstBusiness() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.firstBusiness
}

func (t *RequestTrace) WebsocketConnectedAt() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.websocketConnected
}

func (t *RequestTrace) FirstServiceResponseAt() time.Time {
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.firstServiceResponse
}
