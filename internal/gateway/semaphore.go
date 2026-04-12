package gateway

import "context"

type weightedSemaphore struct {
	ch chan struct{}
}

func newWeightedSemaphore(limit int) *weightedSemaphore {
	if limit <= 0 {
		limit = 1
	}
	return &weightedSemaphore{ch: make(chan struct{}, limit)}
}

func (s *weightedSemaphore) Acquire(ctx context.Context) error {
	if s == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.ch <- struct{}{}:
		return nil
	}
}

func (s *weightedSemaphore) Release() {
	if s == nil {
		return
	}
	select {
	case <-s.ch:
	default:
	}
}
