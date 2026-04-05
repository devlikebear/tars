package tarsserver

import (
	"context"
	"sync"
	"time"
)

// telegramDeliveryAttempt is a single record in the delivery counter ring
// buffer. Only minimal information is retained — enough for the pulse
// watchdog to count recent failures without keeping an unbounded history.
type telegramDeliveryAttempt struct {
	At      time.Time
	Success bool
	Err     string
}

// telegramDeliveryCounter tracks recent telegram send outcomes in a fixed
// ring buffer. It is safe for concurrent use. The zero value is not usable;
// construct it with newTelegramDeliveryCounter.
//
// Only attempts to Send (message sends) are recorded. SendChatAction is a
// best-effort UX signal and is intentionally excluded to avoid skewing the
// failure count during transient connectivity blips.
type telegramDeliveryCounter struct {
	mu  sync.Mutex
	cap int
	now func() time.Time
	// buf is a circular buffer; len(buf) == cap once full.
	buf  []telegramDeliveryAttempt
	head int // index where the next attempt will be written
	size int // number of valid entries (<= cap)
}

func newTelegramDeliveryCounter(capacity int) *telegramDeliveryCounter {
	if capacity <= 0 {
		capacity = 100
	}
	return &telegramDeliveryCounter{
		cap: capacity,
		now: time.Now,
		buf: make([]telegramDeliveryAttempt, capacity),
	}
}

// Record appends a delivery attempt to the ring. errMsg is ignored when
// success is true.
func (c *telegramDeliveryCounter) Record(success bool, errMsg string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := telegramDeliveryAttempt{
		At:      c.now(),
		Success: success,
	}
	if !success {
		entry.Err = errMsg
	}
	c.buf[c.head] = entry
	c.head = (c.head + 1) % c.cap
	if c.size < c.cap {
		c.size++
	}
}

// FailuresWithin returns the number of failed attempts whose timestamp is
// within the last window duration. A zero or negative window returns 0.
func (c *telegramDeliveryCounter) FailuresWithin(window time.Duration) int {
	if c == nil || window <= 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := c.now().Add(-window)
	count := 0
	for i := 0; i < c.size; i++ {
		e := c.buf[i]
		if e.Success {
			continue
		}
		if e.At.Before(cutoff) {
			continue
		}
		count++
	}
	return count
}

// Stats returns total attempts, successes, failures, and the latest error
// message (empty if none). Only entries currently in the ring are counted.
func (c *telegramDeliveryCounter) Stats() (total, successes, failures int, latestErr string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var latestAt time.Time
	for i := 0; i < c.size; i++ {
		e := c.buf[i]
		total++
		if e.Success {
			successes++
			continue
		}
		failures++
		if e.At.After(latestAt) {
			latestAt = e.At
			latestErr = e.Err
		}
	}
	return
}

// telegramCountingSender wraps an existing telegramSender and records
// each Send outcome in a delivery counter. SendChatAction is passed
// through unmodified (see counter doc for the rationale).
type telegramCountingSender struct {
	inner   telegramSender
	counter *telegramDeliveryCounter
}

func newTelegramCountingSender(inner telegramSender, counter *telegramDeliveryCounter) telegramSender {
	if inner == nil {
		return nil
	}
	if counter == nil {
		return inner
	}
	return &telegramCountingSender{inner: inner, counter: counter}
}

func (s *telegramCountingSender) Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
	result, err := s.inner.Send(ctx, req)
	if err != nil {
		s.counter.Record(false, err.Error())
		return result, err
	}
	s.counter.Record(true, "")
	return result, nil
}

func (s *telegramCountingSender) SendChatAction(ctx context.Context, req telegramChatActionRequest) error {
	return s.inner.SendChatAction(ctx, req)
}
