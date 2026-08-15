package tarsserver

import (
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// freeAddress returns an address nothing is listening on.
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release address: %v", err)
	}
	return addr
}

func TestAwaitPredecessorExitIsNoOpWithoutMarker(t *testing.T) {
	if _, ok := os.LookupEnv(restartPredecessorEnv); ok {
		t.Fatalf("%s leaked into the test environment", restartPredecessorEnv)
	}
	// A busy address proves this returned without probing: had it waited, it
	// would have blocked until the timeout.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	done := make(chan struct{})
	go func() {
		awaitPredecessorExit(listener.Addr().String(), zerolog.New(io.Discard))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected an immediate return when the marker is unset")
	}
}

func TestAwaitPredecessorExitReturnsOncePortIsFree(t *testing.T) {
	t.Setenv(restartPredecessorEnv, "4242")
	addr := freeAddress(t)

	awaitPredecessorExit(addr, zerolog.New(io.Discard))

	// The marker is cleared so a later restart of this process does not wait on
	// a predecessor that is long gone.
	if value, ok := os.LookupEnv(restartPredecessorEnv); ok {
		t.Fatalf("expected the marker to be cleared, still set to %q", value)
	}
}

func TestAwaitPredecessorExitGivesUpWhenPortStaysBusy(t *testing.T) {
	t.Setenv(restartPredecessorEnv, "4242")
	previous := restartHandoffTimeout
	restartHandoffTimeout = 100 * time.Millisecond
	t.Cleanup(func() { restartHandoffTimeout = previous })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	started := time.Now()
	awaitPredecessorExit(listener.Addr().String(), zerolog.New(io.Discard))
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("expected the wait to be bounded, took %s", elapsed)
	}
}
