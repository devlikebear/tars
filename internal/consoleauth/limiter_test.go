package consoleauth

import (
	"testing"
	"time"
)

func TestFailureLimiterLocksAfterMaxFailuresAndExpires(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	limiter := NewFailureLimiter(5, 2*time.Minute, WithLimiterNow(func() time.Time { return now }))

	for i := 0; i < 4; i++ {
		if allowed, _ := limiter.Allow(FailurePurposeLogin, RoleUser, "iphone"); !allowed {
			t.Fatalf("attempt %d should still be allowed", i+1)
		}
		limiter.RecordFailure(FailurePurposeLogin, RoleUser, "iphone")
	}
	if allowed, _ := limiter.Allow(FailurePurposeLogin, RoleUser, "iphone"); !allowed {
		t.Fatalf("fifth attempt should be allowed before the failure is recorded")
	}
	lockedUntil := limiter.RecordFailure(FailurePurposeLogin, RoleUser, "iphone")
	if !lockedUntil.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("expected lockout until %s, got %s", now.Add(2*time.Minute), lockedUntil)
	}
	if allowed, until := limiter.Allow(FailurePurposeLogin, RoleUser, "iphone"); allowed || !until.Equal(lockedUntil) {
		t.Fatalf("expected locked bucket, allowed=%v until=%s", allowed, until)
	}

	now = now.Add(2*time.Minute + time.Second)
	if allowed, _ := limiter.Allow(FailurePurposeLogin, RoleUser, "iphone"); !allowed {
		t.Fatalf("lockout should expire")
	}
}

func TestFailureLimiterIsScopedByPurposeRoleAndRemoteKey(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	limiter := NewFailureLimiter(1, time.Minute, WithLimiterNow(func() time.Time { return now }))
	limiter.RecordFailure(FailurePurposeLogin, RoleUser, "iphone")

	if allowed, _ := limiter.Allow(FailurePurposeLogin, RoleUser, "iphone"); allowed {
		t.Fatalf("same purpose role and remote key should be locked")
	}
	for _, tc := range []struct {
		purpose string
		role    string
		remote  string
	}{
		{FailurePurposePairing, RoleUser, "iphone"},
		{FailurePurposeLogin, RoleAdmin, "iphone"},
		{FailurePurposeLogin, RoleUser, "macbook"},
	} {
		if allowed, _ := limiter.Allow(tc.purpose, tc.role, tc.remote); !allowed {
			t.Fatalf("scope %+v should remain allowed", tc)
		}
	}
}

func TestFailureLimiterRecordSuccessClearsFailures(t *testing.T) {
	now := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	limiter := NewFailureLimiter(2, time.Minute, WithLimiterNow(func() time.Time { return now }))
	limiter.RecordFailure(FailurePurposeLogin, RoleUser, "iphone")
	limiter.RecordSuccess(FailurePurposeLogin, RoleUser, "iphone")
	limiter.RecordFailure(FailurePurposeLogin, RoleUser, "iphone")

	if allowed, _ := limiter.Allow(FailurePurposeLogin, RoleUser, "iphone"); !allowed {
		t.Fatalf("success should clear previous failures")
	}
}
