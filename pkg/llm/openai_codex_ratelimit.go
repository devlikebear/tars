package llm

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CodexRateLimitWindow holds parsed values for one rate-limit window
// (primary = 5h-ish, secondary = weekly) reported by OpenAI Codex via
// `x-codex-*` response headers on /codex/responses calls.
type CodexRateLimitWindow struct {
	UsedPercent       float64 `json:"used_percent"`
	WindowMinutes     int     `json:"window_minutes,omitempty"`
	ResetAfterSeconds int     `json:"reset_after_seconds,omitempty"`
}

// CodexRateLimitSnapshot is the most recently observed Codex subscription
// usage for a given client. RawHeaders preserves every `x-codex-*` header
// (including ones we don't yet model) so the API surface is forward-compatible
// when OpenAI ships new headers.
type CodexRateLimitSnapshot struct {
	Primary    *CodexRateLimitWindow `json:"primary,omitempty"`
	Secondary  *CodexRateLimitWindow `json:"secondary,omitempty"`
	RawHeaders map[string]string     `json:"raw_headers,omitempty"`
	CapturedAt time.Time             `json:"captured_at"`
}

// CodexRateLimitSource is implemented by clients (and wrappers) that can
// surface the most recently observed Codex rate-limit snapshot. Used by the
// admin handler to fish the snapshot out of whatever client the router holds
// (raw OpenAICodexClient, or a TrackedClient wrapping one).
type CodexRateLimitSource interface {
	LastCodexRateLimit() (CodexRateLimitSnapshot, bool)
}

const codexHeaderPrefix = "x-codex-"

// parseCodexRateLimitHeaders extracts rate-limit info from response headers.
// Returns nil if no `x-codex-*` header is present, otherwise returns a
// snapshot with whatever could be parsed. Malformed numeric values fall back
// to zero in the typed fields but are preserved verbatim in RawHeaders so
// debugging can still see them.
func parseCodexRateLimitHeaders(h http.Header) *CodexRateLimitSnapshot {
	raw := map[string]string{}
	for key, values := range h {
		lower := strings.ToLower(key)
		if !strings.HasPrefix(lower, codexHeaderPrefix) {
			continue
		}
		if len(values) == 0 {
			continue
		}
		raw[lower] = strings.TrimSpace(values[0])
	}
	if len(raw) == 0 {
		return nil
	}

	snap := &CodexRateLimitSnapshot{
		RawHeaders: raw,
		CapturedAt: time.Now().UTC(),
	}
	snap.Primary = parseCodexWindow(raw, "primary")
	snap.Secondary = parseCodexWindow(raw, "secondary")
	return snap
}

func parseCodexWindow(raw map[string]string, kind string) *CodexRateLimitWindow {
	used, hasUsed := raw[codexHeaderPrefix+kind+"-used-percent"]
	window, hasWindow := raw[codexHeaderPrefix+kind+"-window-minutes"]
	reset, hasReset := raw[codexHeaderPrefix+kind+"-reset-after-seconds"]
	if !hasUsed && !hasWindow && !hasReset {
		return nil
	}
	w := &CodexRateLimitWindow{}
	if hasUsed {
		if v, err := strconv.ParseFloat(used, 64); err == nil {
			w.UsedPercent = v
		}
	}
	if hasWindow {
		if v, err := strconv.Atoi(window); err == nil {
			w.WindowMinutes = v
		}
	}
	if hasReset {
		if v, err := strconv.Atoi(reset); err == nil {
			w.ResetAfterSeconds = v
		}
	}
	return w
}
