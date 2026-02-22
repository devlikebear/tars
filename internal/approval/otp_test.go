package approval

import (
	"context"
	"testing"
	"time"
)

func TestOTPManager_RequestAndConsume(t *testing.T) {
	mgr := NewOTPManager(func() time.Time { return time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan string, 1)
	go func() {
		code, err := mgr.Request(ctx, "1001", 2*time.Second)
		if err != nil {
			done <- ""
			return
		}
		done <- code
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for !mgr.HasPending("1001") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !mgr.HasPending("1001") {
		t.Fatalf("expected pending otp request")
	}
	if ok := mgr.Consume("1001", "123456"); !ok {
		t.Fatalf("expected consume to succeed")
	}
	select {
	case code := <-done:
		if code != "123456" {
			t.Fatalf("expected 123456, got %q", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting otp code")
	}
}
