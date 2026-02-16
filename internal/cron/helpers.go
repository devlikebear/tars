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

func computeBackoffDuration(schedule string, failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	base := 30 * time.Second
	if interval, ok := parseEveryDuration(schedule); ok && interval > base {
		base = interval
	}
	multiplier := failures - 1
	if multiplier > 6 {
		multiplier = 6
	}
	backoff := base * time.Duration(1<<multiplier)
	capDur := 12 * time.Hour
	if backoff > capDur {
		return capDur
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func runPath(runsDir, jobID string) string {
	return filepath.Join(runsDir, strings.TrimSpace(jobID)+".jsonl")
}
