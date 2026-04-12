package gateway

import "context"

type executionSemaphore struct {
	ch chan struct{}
}

func newExecutionSemaphore(limit int) *executionSemaphore {
	if limit <= 0 {
		limit = 1
	}
	return &executionSemaphore{ch: make(chan struct{}, limit)}
}

func (s *executionSemaphore) Acquire(ctx context.Context) error {
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

func (s *executionSemaphore) Release() {
	if s == nil {
		return
	}
	select {
	case <-s.ch:
	default:
	}
}
