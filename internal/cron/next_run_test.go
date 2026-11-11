package cron

import (
	"testing"
	"time"
)

func TestJobNextRunIn(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	from := time.Date(2026, 11, 11, 8, 0, 0, 0, kst)
	to := time.Date(2026, 11, 12, 0, 0, 0, 0, kst)
	ran := from.Add(-time.Hour)
	tests := []struct {
		name  string
		job   Job
		at    time.Time
		every time.Duration
		ok    bool
	}{
		{"cron expression in the job zone", Job{Enabled: true, Schedule: "30 9 * * *", Timezone: "Asia/Seoul"}, time.Date(2026, 11, 11, 9, 30, 0, 0, kst), 0, true},
		{"due exactly at from", Job{Enabled: true, Schedule: "0 8 * * *", Timezone: "Asia/Seoul"}, from, 0, true},
		{"next run is tomorrow", Job{Enabled: true, Schedule: "0 7 * * *", Timezone: "Asia/Seoul"}, time.Time{}, 0, false},
		{"disabled", Job{Schedule: "30 9 * * *"}, time.Time{}, 0, false},
		{"interval", Job{Enabled: true, Schedule: "every:30m"}, time.Time{}, 30 * time.Minute, true},
		{"zero interval", Job{Enabled: true, Schedule: "every:0s"}, time.Time{}, 0, false},
		{"one-shot today", Job{Enabled: true, Schedule: "at:2026-11-11T05:00:00Z"}, time.Date(2026, 11, 11, 5, 0, 0, 0, time.UTC), 0, true},
		{"one-shot already ran", Job{Enabled: true, Schedule: "at:2026-11-11T05:00:00Z", LastRunAt: &ran}, time.Time{}, 0, false},
		{"one-shot tomorrow", Job{Enabled: true, Schedule: "at:2026-11-12T05:00:00Z"}, time.Time{}, 0, false},
		{"bad one-shot", Job{Enabled: true, Schedule: "at:soon"}, time.Time{}, 0, false},
		{"bad expression", Job{Enabled: true, Schedule: "whenever"}, time.Time{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at, every, ok := tt.job.NextRunIn(from, to)
			if ok != tt.ok || every != tt.every || !at.Equal(tt.at) {
				t.Fatalf("NextRunIn = %v %v %v, want %v %v %v", at, every, ok, tt.at, tt.every, tt.ok)
			}
		})
	}
	if _, _, ok := (Job{Enabled: true, Schedule: "every:1m"}).NextRunIn(to, from); ok {
		t.Fatal("an empty span has no runs")
	}
}
