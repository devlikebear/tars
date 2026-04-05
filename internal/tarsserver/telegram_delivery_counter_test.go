package tarsserver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestDeliveryCounter(cap int, now func() time.Time) *telegramDeliveryCounter {
	c := newTelegramDeliveryCounter(cap)
	if now != nil {
		c.now = now
	}
	return c
}

func TestDeliveryCounter_RecordAndStats(t *testing.T) {
	base := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	var now time.Time
	c := newTestDeliveryCounter(10, func() time.Time { return now })

	now = base
	c.Record(true, "")
	now = base.Add(1 * time.Minute)
	c.Record(false, "network")
	now = base.Add(2 * time.Minute)
	c.Record(false, "unauthorized")
	now = base.Add(3 * time.Minute)
	c.Record(true, "")

	total, succ, fail, latestErr := c.Stats()
	if total != 4 || succ != 2 || fail != 2 {
		t.Fatalf("stats total/succ/fail = %d/%d/%d, want 4/2/2", total, succ, fail)
	}
	if latestErr != "unauthorized" {
		t.Errorf("latestErr = %q, want %q", latestErr, "unauthorized")
	}
}

func TestDeliveryCounter_FailuresWithin(t *testing.T) {
	base := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	var now time.Time
	c := newTestDeliveryCounter(10, func() time.Time { return now })

	now = base.Add(-20 * time.Minute)
	c.Record(false, "old") // outside window
	now = base.Add(-5 * time.Minute)
	c.Record(false, "recent-1")
	now = base.Add(-3 * time.Minute)
	c.Record(true, "") // success, not counted
	now = base.Add(-1 * time.Minute)
	c.Record(false, "recent-2")

	now = base
	if got := c.FailuresWithin(10 * time.Minute); got != 2 {
		t.Errorf("FailuresWithin(10m) = %d, want 2", got)
	}
	if got := c.FailuresWithin(30 * time.Minute); got != 3 {
		t.Errorf("FailuresWithin(30m) = %d, want 3", got)
	}
	if got := c.FailuresWithin(0); got != 0 {
		t.Errorf("FailuresWithin(0) = %d, want 0", got)
	}
}

func TestDeliveryCounter_RingBufferOverwritesOldest(t *testing.T) {
	base := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)
	var now time.Time
	c := newTestDeliveryCounter(3, func() time.Time { return now })

	now = base
	c.Record(true, "")
	now = base.Add(1 * time.Minute)
	c.Record(false, "first-fail")
	now = base.Add(2 * time.Minute)
	c.Record(false, "second-fail")
	now = base.Add(3 * time.Minute)
	c.Record(false, "third-fail") // overwrites first success

	total, succ, fail, _ := c.Stats()
	if total != 3 || succ != 0 || fail != 3 {
		t.Fatalf("after overflow total/succ/fail = %d/%d/%d, want 3/0/3", total, succ, fail)
	}
}

func TestDeliveryCounter_NilReceiverSafe(t *testing.T) {
	var c *telegramDeliveryCounter
	// Must not panic.
	c.Record(false, "x")
	if got := c.FailuresWithin(time.Hour); got != 0 {
		t.Errorf("nil FailuresWithin = %d, want 0", got)
	}
	total, s, f, e := c.Stats()
	if total != 0 || s != 0 || f != 0 || e != "" {
		t.Errorf("nil Stats returned non-zero values")
	}
}

func TestDeliveryCounter_Concurrent(t *testing.T) {
	c := newTelegramDeliveryCounter(1000)
	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				c.Record(i%2 == 0, "err")
			}
		}(g)
	}
	wg.Wait()
	total, _, _, _ := c.Stats()
	if total != goroutines*iterations {
		t.Fatalf("total = %d, want %d", total, goroutines*iterations)
	}
}

type fakeTelegramSender struct {
	sendErr       error
	sendCalls     int
	chatActionErr error
}

func (f *fakeTelegramSender) Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
	f.sendCalls++
	if f.sendErr != nil {
		return telegramSendResult{}, f.sendErr
	}
	return telegramSendResult{MessageID: 42, ChatID: req.ChatID, Text: req.Text}, nil
}

func (f *fakeTelegramSender) SendChatAction(ctx context.Context, req telegramChatActionRequest) error {
	return f.chatActionErr
}

func TestCountingSender_RecordsSuccess(t *testing.T) {
	inner := &fakeTelegramSender{}
	counter := newTelegramDeliveryCounter(10)
	wrapped := newTelegramCountingSender(inner, counter)
	_, err := wrapped.Send(context.Background(), telegramSendRequest{ChatID: "c", Text: "hi"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	total, succ, fail, _ := counter.Stats()
	if total != 1 || succ != 1 || fail != 0 {
		t.Errorf("after success total/succ/fail = %d/%d/%d, want 1/1/0", total, succ, fail)
	}
}

func TestCountingSender_RecordsFailure(t *testing.T) {
	inner := &fakeTelegramSender{sendErr: errors.New("boom")}
	counter := newTelegramDeliveryCounter(10)
	wrapped := newTelegramCountingSender(inner, counter)
	_, err := wrapped.Send(context.Background(), telegramSendRequest{ChatID: "c", Text: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
	total, succ, fail, latest := counter.Stats()
	if total != 1 || succ != 0 || fail != 1 {
		t.Errorf("after failure total/succ/fail = %d/%d/%d, want 1/0/1", total, succ, fail)
	}
	if latest != "boom" {
		t.Errorf("latestErr = %q, want boom", latest)
	}
}

func TestCountingSender_PassesThroughChatAction(t *testing.T) {
	inner := &fakeTelegramSender{}
	counter := newTelegramDeliveryCounter(10)
	wrapped := newTelegramCountingSender(inner, counter)
	if err := wrapped.SendChatAction(context.Background(), telegramChatActionRequest{ChatID: "c"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	total, _, _, _ := counter.Stats()
	if total != 0 {
		t.Errorf("SendChatAction should not record: total=%d", total)
	}
}

func TestCountingSender_NilInnerReturnsNil(t *testing.T) {
	counter := newTelegramDeliveryCounter(10)
	if s := newTelegramCountingSender(nil, counter); s != nil {
		t.Errorf("nil inner should return nil sender, got %T", s)
	}
}

func TestCountingSender_NilCounterReturnsInnerUnchanged(t *testing.T) {
	inner := &fakeTelegramSender{}
	if s := newTelegramCountingSender(inner, nil); s != telegramSender(inner) {
		t.Errorf("nil counter should return inner unchanged, got %T", s)
	}
}
