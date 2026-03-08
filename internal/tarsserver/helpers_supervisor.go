package tarsserver

import (
	"context"
	"sync"
	"time"
)

type serializedSupervisorOptions[T any] struct {
	nowFn   func() time.Time
	timeout time.Duration
	run     func(ctx context.Context, ranAt time.Time) (T, error)
	record  func(ranAt time.Time, result T, runErr error)
	emit    func(ctx context.Context, result T, runErr error)
}

func newSerializedSupervisorRunner[T any](opts serializedSupervisorOptions[T]) func(ctx context.Context) (T, error) {
	var zero T
	if opts.run == nil {
		return func(context.Context) (T, error) { return zero, nil }
	}
	nowFn := opts.nowFn
	if nowFn == nil {
		nowFn = time.Now
	}
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	runner := &serializedSupervisorRunner[T]{
		nowFn:   nowFn,
		timeout: timeout,
		run:     opts.run,
		record:  opts.record,
		emit:    opts.emit,
	}
	return func(ctx context.Context) (T, error) {
		return runner.runOnce(ctx)
	}
}

type serializedSupervisorRunner[T any] struct {
	mu      sync.Mutex
	nowFn   func() time.Time
	timeout time.Duration
	run     func(ctx context.Context, ranAt time.Time) (T, error)
	record  func(ranAt time.Time, result T, runErr error)
	emit    func(ctx context.Context, result T, runErr error)
}

func (r *serializedSupervisorRunner[T]) runOnce(ctx context.Context) (T, error) {
	var zero T
	if r == nil || r.run == nil {
		return zero, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	ranAt := r.nowFn().UTC()
	result, err := r.run(callCtx, ranAt)
	if r.record != nil {
		r.record(ranAt, result, err)
	}
	if r.emit != nil {
		r.emit(ctx, result, err)
	}
	return result, err
}
