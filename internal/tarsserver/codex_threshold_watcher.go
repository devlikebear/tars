package tarsserver

import (
	"context"
	"fmt"
	"sync"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

// thresholdBand classifies a Codex window's used_percent into a discrete
// severity band. The watcher only emits notifications when the band climbs;
// recovery (band drops back) is silent.
type thresholdBand int

const (
	bandNone thresholdBand = iota
	bandWarn               // 90.0 <= used < 95.0
	bandCritical           // used >= 95.0
)

const (
	codexWarnThreshold     = 90.0
	codexCriticalThreshold = 95.0

	codexQuotaCategory = "codex_quota"
)

func bandFor(usedPercent float64) thresholdBand {
	switch {
	case usedPercent >= codexCriticalThreshold:
		return bandCritical
	case usedPercent >= codexWarnThreshold:
		return bandWarn
	default:
		return bandNone
	}
}

func (b thresholdBand) severity() string {
	switch b {
	case bandCritical:
		return "error"
	case bandWarn:
		return "warn"
	default:
		return "info"
	}
}

// codexThresholdEmitter is the minimal slice of notificationDispatcher we
// need; declared as an interface so tests can stub it.
type codexThresholdEmitter interface {
	Emit(ctx context.Context, evt notificationEvent)
}

type codexThresholdKey struct {
	tier   string
	window string // "primary" or "weekly"
}

// codexThresholdWatcher dedupes threshold transitions per (tier, window) so
// the SSE stream gets exactly one notification when used_percent first
// crosses 90% and another when it crosses 95%.
type codexThresholdWatcher struct {
	emitter codexThresholdEmitter

	mu    sync.Mutex
	bands map[codexThresholdKey]thresholdBand
}

func newCodexThresholdWatcher(emitter codexThresholdEmitter) *codexThresholdWatcher {
	return &codexThresholdWatcher{
		emitter: emitter,
		bands:   map[codexThresholdKey]thresholdBand{},
	}
}

// Observe records the latest used_percent for a (tier, window) and emits a
// notification if the band climbed since the last observation. tier is the
// LLM router tier label (heavy/standard/light); window is "primary" (5h) or
// "weekly".
func (w *codexThresholdWatcher) Observe(tier, model, window string, usedPercent float64) {
	if w == nil || w.emitter == nil {
		return
	}
	next := bandFor(usedPercent)
	key := codexThresholdKey{tier: tier, window: window}

	w.mu.Lock()
	prev := w.bands[key]
	if next == prev {
		w.mu.Unlock()
		return
	}
	w.bands[key] = next
	w.mu.Unlock()

	if next <= prev {
		// Recovery — band dropped. Silent reset; don't spam users when usage
		// drains naturally between requests.
		return
	}

	title := "Codex quota warning"
	if next == bandCritical {
		title = "Codex quota critical"
	}
	humanWindow := window
	if window == "weekly" {
		humanWindow = "weekly"
	}
	msg := fmt.Sprintf(
		"Codex %s window at %.1f%% (tier=%s, model=%s)",
		humanWindow, usedPercent, tier, model,
	)
	w.emitter.Emit(context.Background(), newNotificationEvent(codexQuotaCategory, next.severity(), title, msg))
}

// observeSnapshot wires a CodexRateLimitSnapshot into Observe for both
// windows. Convenience used by the rate-limit observer registered on each
// codex client.
func (w *codexThresholdWatcher) observeSnapshot(tier, model string, snap llm.CodexRateLimitSnapshot) {
	if w == nil {
		return
	}
	if snap.Primary != nil {
		w.Observe(tier, model, "primary", snap.Primary.UsedPercent)
	}
	if snap.Secondary != nil {
		w.Observe(tier, model, "weekly", snap.Secondary.UsedPercent)
	}
}

// attachCodexThresholdWatcher registers a rate-limit observer on every
// OpenAICodexClient reachable through the router. The observer forwards
// snapshots into the watcher, which de-dupes and emits SSE notifications.
// Returns the configured watcher so callers can introspect/stop it.
func attachCodexThresholdWatcher(router llm.Router, emitter codexThresholdEmitter) *codexThresholdWatcher {
	if router == nil || emitter == nil {
		return nil
	}
	watcher := newCodexThresholdWatcher(emitter)
	for _, tier := range llm.AllTiers() {
		client, resolution, err := router.ClientForTier(tier)
		if err != nil {
			continue
		}
		codex := unwrapCodexClient(client)
		if codex == nil {
			continue
		}
		tierLabel := string(tier)
		model := resolution.Model
		codex.SetRateLimitObserver(func(snap llm.CodexRateLimitSnapshot) {
			watcher.observeSnapshot(tierLabel, model, snap)
		})
	}
	return watcher
}

func unwrapCodexClient(client llm.Client) *llm.OpenAICodexClient {
	if direct, ok := client.(*llm.OpenAICodexClient); ok {
		return direct
	}
	if tracked, ok := client.(*usage.TrackedClient); ok {
		if direct, ok := tracked.Inner().(*llm.OpenAICodexClient); ok {
			return direct
		}
	}
	return nil
}
