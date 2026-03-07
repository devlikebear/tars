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

func TestResolveSchedule(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.FixedZone("KST", 9*3600))

	gotExplicit, err := ResolveSchedule(" at:2026-03-01T15:00:00+09:00 ", "", "Asia/Seoul", now)
	if err != nil {
		t.Fatalf("resolve explicit failed: %v", err)
	}
	if want := "at:2026-03-01T06:00:00Z"; gotExplicit != want {
		t.Fatalf("expected explicit %q, got %q", want, gotExplicit)
	}

	gotNatural, err := ResolveSchedule("", "1분뒤 테스트", "Asia/Seoul", now)
	if err != nil {
		t.Fatalf("resolve natural failed: %v", err)
	}
	if want := "at:" + now.Add(1*time.Minute).Format(time.RFC3339); gotNatural != want {
		t.Fatalf("expected natural %q, got %q", want, gotNatural)
	}
}

func TestParseNaturalSchedule_UsesTimezoneFallback(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)

	got, err := ParseNaturalSchedule("내일 오후 3시", "Invalid/Timezone", now)
	if err != nil {
		t.Fatalf("parse natural failed: %v", err)
	}
	if want := "at:2026-02-28T15:00:00+09:00"; got != want {
		t.Fatalf("expected fallback timezone schedule %q, got %q", want, got)
	}
}
