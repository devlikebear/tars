package cron

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func defaultJobName(prompt string) string {
	if prompt == "" {
		return "cron job"
	}
	line := strings.TrimSpace(strings.Split(prompt, "\n")[0])
	if line == "" {
		return "cron job"
	}
	if len(line) > 48 {
		return line[:48] + "..."
	}
	return line
}

func resolveDefaultDeleteAfterRun(schedule string, requested bool, explicitlySet bool) bool {
	if explicitlySet || requested {
		return requested
	}
	if _, isAt, err := parseAtTime(schedule); isAt && err == nil {
		return true
	}
	return looksOneShotCronSchedule(schedule)
}

// Cron retry backoff parameters. The backoff doubles per failure starting
// from backoffBaseDuration, capped at backoffMaxMultiplier doublings (so the
// largest multiplier is 1<<6 = 64x), and never exceeds backoffMaxDuration.
// For schedules with a fixed interval larger than backoffBaseDuration, that
// interval is used as the base instead.
const (
	backoffBaseDuration  = 30 * time.Second
	backoffMaxMultiplier = 6
	backoffMaxDuration   = 12 * time.Hour
)

func computeBackoffDuration(schedule string, failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	base := backoffBaseDuration
	if interval, ok := parseEveryDuration(schedule); ok && interval > base {
		base = interval
	}
	multiplier := min(failures-1, backoffMaxMultiplier)
	backoff := base * time.Duration(1<<multiplier)
	if backoff > backoffMaxDuration {
		return backoffMaxDuration
	}
	return backoff
}

func newJobID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("job_%d", time.Now().UTC().UnixNano())
	}
	return "job_" + hex.EncodeToString(b[:])
}

func runPath(runsDir, jobID string) string {
	return filepath.Join(runsDir, strings.TrimSpace(jobID)+".jsonl")
}
