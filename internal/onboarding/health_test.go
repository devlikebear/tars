package onboarding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForHealthz_ImmediateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/healthz") {
			t.Errorf("expected /v1/healthz, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := WaitForHealthz(ctx, srv.URL, 50*time.Millisecond); err != nil {
		t.Fatalf("wait for healthz: %v", err)
	}
}

func TestWaitForHealthz_RetriesUntilReady(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := WaitForHealthz(ctx, srv.URL, 20*time.Millisecond); err != nil {
		t.Fatalf("wait for healthz: %v", err)
	}
	if attempts.Load() < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", attempts.Load())
	}
}

func TestWaitForHealthz_TimeoutBubblesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := WaitForHealthz(ctx, srv.URL, 30*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestWaitForHealthz_RejectsEmptyBaseURL(t *testing.T) {
	if err := WaitForHealthz(context.Background(), "", 10*time.Millisecond); err == nil {
		t.Fatal("expected error for empty base url")
	}
}
