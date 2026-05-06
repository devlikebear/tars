package consoleauth

import (
	"strings"
	"sync"
	"time"
)

const (
	FailurePurposeLogin   = "login"
	FailurePurposePairing = "pairing"
)

type FailureLimiter struct {
	mu          sync.Mutex
	maxFailures int
	lockout     time.Duration
	now         func() time.Time
	buckets     map[failureKey]failureBucket
}

type FailureLimiterOption func(*FailureLimiter)

func WithLimiterNow(now func() time.Time) FailureLimiterOption {
	return func(l *FailureLimiter) {
		if now != nil {
			l.now = now
		}
	}
}

type failureKey struct {
	purpose string
	role    string
	remote  string
}

type failureBucket struct {
	failures    int
	lockedUntil time.Time
}

func NewFailureLimiter(maxFailures int, lockout time.Duration, opts ...FailureLimiterOption) *FailureLimiter {
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if lockout <= 0 {
		lockout = time.Minute
	}
	limiter := &FailureLimiter{
		maxFailures: maxFailures,
		lockout:     lockout,
		now: func() time.Time {
			return time.Now().UTC()
		},
		buckets: map[failureKey]failureBucket{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(limiter)
		}
	}
	return limiter
}

func (l *FailureLimiter) Allow(purpose, role, remoteKey string) (bool, time.Time) {
	if l == nil {
		return true, time.Time{}
	}
	key := newFailureKey(purpose, role, remoteKey)
	now := l.now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok {
		return true, time.Time{}
	}
	if bucket.lockedUntil.IsZero() {
		return true, time.Time{}
	}
	if now.Before(bucket.lockedUntil) {
		return false, bucket.lockedUntil
	}
	delete(l.buckets, key)
	return true, time.Time{}
}

func (l *FailureLimiter) RecordFailure(purpose, role, remoteKey string) time.Time {
	if l == nil {
		return time.Time{}
	}
	key := newFailureKey(purpose, role, remoteKey)
	now := l.now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.buckets[key]
	if !bucket.lockedUntil.IsZero() && !now.Before(bucket.lockedUntil) {
		bucket = failureBucket{}
	}
	bucket.failures++
	if bucket.failures >= l.maxFailures {
		bucket.lockedUntil = now.Add(l.lockout)
	}
	l.buckets[key] = bucket
	return bucket.lockedUntil
}

func (l *FailureLimiter) RecordSuccess(purpose, role, remoteKey string) {
	if l == nil {
		return
	}
	key := newFailureKey(purpose, role, remoteKey)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

func newFailureKey(purpose, role, remoteKey string) failureKey {
	return failureKey{
		purpose: normalizeFailurePart(purpose, FailurePurposeLogin),
		role:    normalizeFailurePart(role, RoleUser),
		remote:  normalizeFailurePart(remoteKey, "unknown"),
	}
}

func normalizeFailurePart(value, fallback string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return fallback
	}
	return normalized
}
