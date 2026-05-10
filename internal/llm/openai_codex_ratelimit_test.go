package llm

import (
	"net/http"
	"testing"
)

func TestParseCodexRateLimitHeaders_ParsesPrimaryAndSecondary(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-primary-used-percent", "42.5")
	h.Set("x-codex-primary-window-minutes", "300")
	h.Set("x-codex-primary-reset-after-seconds", "1800")
	h.Set("x-codex-secondary-used-percent", "12")
	h.Set("x-codex-secondary-window-minutes", "10080")
	h.Set("x-codex-secondary-reset-after-seconds", "604000")

	snap := parseCodexRateLimitHeaders(h)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Primary == nil {
		t.Fatal("expected primary window")
	}
	if got, want := snap.Primary.UsedPercent, 42.5; got != want {
		t.Errorf("primary used_percent: got %v, want %v", got, want)
	}
	if got, want := snap.Primary.WindowMinutes, 300; got != want {
		t.Errorf("primary window_minutes: got %v, want %v", got, want)
	}
	if got, want := snap.Primary.ResetAfterSeconds, 1800; got != want {
		t.Errorf("primary reset_after_seconds: got %v, want %v", got, want)
	}
	if snap.Secondary == nil {
		t.Fatal("expected secondary window")
	}
	if got, want := snap.Secondary.UsedPercent, 12.0; got != want {
		t.Errorf("secondary used_percent: got %v, want %v", got, want)
	}
	if got, want := snap.Secondary.WindowMinutes, 10080; got != want {
		t.Errorf("secondary window_minutes: got %v, want %v", got, want)
	}
	if snap.CapturedAt.IsZero() {
		t.Error("expected captured_at to be set")
	}
}

func TestParseCodexRateLimitHeaders_PartialPrimaryOnly(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-primary-used-percent", "5.0")

	snap := parseCodexRateLimitHeaders(h)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Primary == nil || snap.Primary.UsedPercent != 5.0 {
		t.Errorf("primary not parsed: %+v", snap.Primary)
	}
	if snap.Secondary != nil {
		t.Errorf("expected nil secondary, got %+v", snap.Secondary)
	}
}

func TestParseCodexRateLimitHeaders_NoCodexHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-Other-Provider", "stuff")

	if snap := parseCodexRateLimitHeaders(h); snap != nil {
		t.Errorf("expected nil for non-codex headers, got %+v", snap)
	}
}

func TestParseCodexRateLimitHeaders_PreservesUnknownCodexHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-experimental-flag", "true")
	h.Set("x-codex-credits-remaining", "1234")

	snap := parseCodexRateLimitHeaders(h)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Primary != nil || snap.Secondary != nil {
		t.Errorf("unexpected typed windows: %+v %+v", snap.Primary, snap.Secondary)
	}
	if got := snap.RawHeaders["x-codex-experimental-flag"]; got != "true" {
		t.Errorf("raw experimental-flag: got %q, want true", got)
	}
	if got := snap.RawHeaders["x-codex-credits-remaining"]; got != "1234" {
		t.Errorf("raw credits-remaining: got %q, want 1234", got)
	}
}

func TestParseCodexRateLimitHeaders_IgnoresMalformedNumbers(t *testing.T) {
	h := http.Header{}
	h.Set("x-codex-primary-used-percent", "not-a-number")
	h.Set("x-codex-primary-window-minutes", "300")

	snap := parseCodexRateLimitHeaders(h)
	if snap == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if snap.Primary == nil {
		t.Fatal("primary should still be present from window_minutes")
	}
	if snap.Primary.UsedPercent != 0 {
		t.Errorf("malformed used_percent should default to 0, got %v", snap.Primary.UsedPercent)
	}
	if snap.Primary.WindowMinutes != 300 {
		t.Errorf("primary window_minutes lost: got %v", snap.Primary.WindowMinutes)
	}
	// Malformed value preserved in RawHeaders for debugging.
	if got := snap.RawHeaders["x-codex-primary-used-percent"]; got != "not-a-number" {
		t.Errorf("raw header for malformed value should be preserved, got %q", got)
	}
}
