package scheduleexpr

import (
	"testing"
	"time"
)

func TestNormalizeExpression(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty uses cron default",
			input: "",
			want:  "every:1h",
		},
		{
			name:  "at expression normalizes to utc",
			input: "at:2026-03-01T15:00:00+09:00",
			want:  "at:2026-03-01T06:00:00Z",
		},
		{
			name:  "every expression trims and preserves duration",
			input: " every:30m ",
			want:  "every:30m",
		},
		{
			name:  "cron expression stays as is",
			input: "*/5 * * * *",
			want:  "*/5 * * * *",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeExpression(tc.input)
			if err != nil {
				t.Fatalf("normalize failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestParseNaturalSchedule_TomorrowUsesTimezone(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	// "내일" is the day after today's date in the server's local zone.
	tomorrow := now.In(time.Local).AddDate(0, 0, 1)
	localWant := "at:" + time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 15, 0, 0, 0, time.Local).Format(time.RFC3339)
	tests := []struct {
		name     string
		timezone string
		want     string
	}{
		{name: "explicit iana zone", timezone: "Asia/Seoul", want: "at:2026-02-28T15:00:00+09:00"},
		{name: "empty means server local", timezone: "", want: localWant},
		{name: "Local means server local", timezone: "Local", want: localWant},
		{name: "unknown zone falls back to server local", timezone: "Invalid/Timezone", want: localWant},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseNaturalSchedule("내일 오후 3시", tc.timezone, now)
			if err != nil {
				t.Fatalf("parse natural failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseNaturalSchedule(tz=%q) = %q, want %q", tc.timezone, got, tc.want)
			}
		})
	}
}

func TestParseNaturalSchedule_WeeklyCarriesExplicitTimezone(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		timezone string
		want     string
	}{
		// The job's own timezone (server local for new jobs) evaluates a bare expression.
		{name: "empty", timezone: "", want: "0 9 * * 1"},
		{name: "Local", timezone: "Local", want: "0 9 * * 1"},
		{name: "unknown", timezone: "Invalid/Timezone", want: "0 9 * * 1"},
		// An explicit zone travels with the expression so it means the same hour anywhere.
		{name: "explicit iana zone", timezone: "Asia/Seoul", want: "CRON_TZ=Asia/Seoul 0 9 * * 1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseNaturalSchedule("매주 월요일 오전 9시", tc.timezone, now)
			if err != nil {
				t.Fatalf("parse natural failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ParseNaturalSchedule(tz=%q) = %q, want %q", tc.timezone, got, tc.want)
			}
			if _, err := NormalizeExpression(got); err != nil {
				t.Fatalf("weekly result %q is not a valid schedule: %v", got, err)
			}
		})
	}
}
