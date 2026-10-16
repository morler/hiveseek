package acp

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedBuffer is a race-safe io.Reader/Writer pair for tests that poll the
// transport output while a call is in flight.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Read(p)
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestCallClientMethodTimesOutWithoutResponse guards the bound that keeps a
// client which never answers session/request_permission from wedging its
// session's turn forever (the turn holds the session's prompt mutex, and an
// in-flight session is neither evictable nor deletable).
func TestCallClientMethodTimesOutWithoutResponse(t *testing.T) {
	defer func(prev time.Duration) { ClientCallTimeout = prev }(ClientCallTimeout)
	ClientCallTimeout = 20 * time.Millisecond

	io := &lockedBuffer{}
	tr := NewTransportWithIO(io, io, io)
	start := time.Now()
	_, err := tr.CallClientMethod("sess-1", "session/request_permission", map[string]interface{}{"sessionId": "sess-1"})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the timeout to wrap context.DeadlineExceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("call did not return promptly: %s", elapsed)
	}
	if !strings.Contains(io.String(), "session/request_permission") {
		t.Fatalf("the request was not written to the transport: %q", io.String())
	}
}

// TestCallClientMethodHonorsCallerContext verifies a cancelled caller context
// abandons the wait without writing a request nobody will answer.
func TestCallClientMethodHonorsCallerContext(t *testing.T) {
	io := &lockedBuffer{}
	tr := NewTransportWithIO(io, io, io)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tr.CallClientMethodContext(ctx, "sess-1", "session/request_permission", map[string]interface{}{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if io.Len() != 0 {
		t.Fatalf("no request should be written for an already-cancelled context, got %q", io.String())
	}
}

// TestCallClientMethodCancelSessionReachesWait verifies that a session/cancel
// arriving while a client call is in flight unblocks that call: the cancel is
// registered before the request is written, so the wait is always reachable.
func TestCallClientMethodCancelSessionReachesWait(t *testing.T) {
	defer func(prev time.Duration) { ClientCallTimeout = prev }(ClientCallTimeout)
	ClientCallTimeout = 30 * time.Second

	io := &lockedBuffer{}
	tr := NewTransportWithIO(io, io, io)
	errCh := make(chan error, 1)
	go func() {
		_, err := tr.CallClientMethod("sess-9", "session/request_permission", map[string]interface{}{})
		errCh <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for io.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if io.Len() == 0 {
		t.Fatal("the call never wrote its request")
	}
	tr.CancelSession("sess-9")
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("expected a cancellation error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("CancelSession did not reach the in-flight call")
	}
}
