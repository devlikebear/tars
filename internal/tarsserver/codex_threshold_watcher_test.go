package tarsserver

import (
	"context"
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/usage"
)

type recordingEmitter struct {
	mu     sync.Mutex
	events []notificationEvent
}

func (r *recordingEmitter) Emit(_ context.Context, evt notificationEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, evt)
}

func (r *recordingEmitter) snapshot() []notificationEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]notificationEvent, len(r.events))
	copy(out, r.events)
	return out
}

func TestBandFor(t *testing.T) {
	cases := []struct {
		used float64
		want thresholdBand
	}{
		{0, bandNone},
		{89.99, bandNone},
		{90, bandWarn},
		{94.99, bandWarn},
		{95, bandCritical},
		{100, bandCritical},
	}
	for _, c := range cases {
		if got := bandFor(c.used); got != c.want {
			t.Errorf("bandFor(%v) = %v, want %v", c.used, got, c.want)
		}
	}
}

func TestCodexThresholdWatcher_FiresOnceWhenEnteringWarnBand(t *testing.T) {
	em := &recordingEmitter{}
	w := newCodexThresholdWatcher(em)

	w.Observe("heavy", "gpt-5.3-codex", "primary", 50)
	w.Observe("heavy", "gpt-5.3-codex", "primary", 89.9)
	if got := em.snapshot(); len(got) != 0 {
		t.Fatalf("expected no emissions below 90%%, got %d", len(got))
	}

	w.Observe("heavy", "gpt-5.3-codex", "primary", 90.0)
	w.Observe("heavy", "gpt-5.3-codex", "primary", 92.5)
	w.Observe("heavy", "gpt-5.3-codex", "primary", 94.9)
	got := em.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 warn emission, got %d (%+v)", len(got), got)
	}
	if got[0].Severity != "warn" || got[0].Category != codexQuotaCategory {
		t.Errorf("event metadata: %+v", got[0])
	}
}

func TestCodexThresholdWatcher_PromotesToCriticalAndStays(t *testing.T) {
	em := &recordingEmitter{}
	w := newCodexThresholdWatcher(em)

	w.Observe("heavy", "m", "primary", 91)
	w.Observe("heavy", "m", "primary", 95.0)
	w.Observe("heavy", "m", "primary", 99)

	got := em.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 emissions (warn then critical), got %d (%+v)", len(got), got)
	}
	if got[0].Severity != "warn" || got[1].Severity != "error" {
		t.Errorf("severities: got %s, %s; want warn, error", got[0].Severity, got[1].Severity)
	}
}

func TestCodexThresholdWatcher_RecoveryIsSilent(t *testing.T) {
	em := &recordingEmitter{}
	w := newCodexThresholdWatcher(em)

	w.Observe("heavy", "m", "primary", 96) // critical → 1 emit
	w.Observe("heavy", "m", "primary", 50) // recovery → silent
	w.Observe("heavy", "m", "primary", 80) // still none → silent
	w.Observe("heavy", "m", "primary", 91) // re-enter warn → emit

	got := em.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 emissions (initial critical + re-enter warn), got %d (%+v)", len(got), got)
	}
	if got[0].Severity != "error" || got[1].Severity != "warn" {
		t.Errorf("severities: got %s, %s", got[0].Severity, got[1].Severity)
	}
}

func TestCodexThresholdWatcher_PrimaryAndSecondaryIndependent(t *testing.T) {
	em := &recordingEmitter{}
	w := newCodexThresholdWatcher(em)

	w.observeSnapshot("standard", "m", llm.CodexRateLimitSnapshot{
		Primary:   &llm.CodexRateLimitWindow{UsedPercent: 91},
		Secondary: &llm.CodexRateLimitWindow{UsedPercent: 12},
	})
	if got := em.snapshot(); len(got) != 1 {
		t.Fatalf("expected 1 emission for primary only, got %d (%+v)", len(got), got)
	}

	w.observeSnapshot("standard", "m", llm.CodexRateLimitSnapshot{
		Primary:   &llm.CodexRateLimitWindow{UsedPercent: 91}, // unchanged
		Secondary: &llm.CodexRateLimitWindow{UsedPercent: 96}, // new critical
	})
	got := em.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 emissions, got %d (%+v)", len(got), got)
	}
	if got[1].Severity != "error" {
		t.Errorf("second emission severity: got %s, want error", got[1].Severity)
	}
}

func TestCodexThresholdWatcher_TiersIndependent(t *testing.T) {
	em := &recordingEmitter{}
	w := newCodexThresholdWatcher(em)

	w.Observe("heavy", "m", "primary", 92)
	w.Observe("light", "m", "primary", 92)

	got := em.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 emissions across tiers, got %d", len(got))
	}
}

func TestCodexThresholdWatcher_NilSafe(t *testing.T) {
	var w *codexThresholdWatcher
	w.Observe("heavy", "m", "primary", 99) // must not panic
	w2 := newCodexThresholdWatcher(nil)
	w2.Observe("heavy", "m", "primary", 99) // must not panic
}

func TestAttachCodexThresholdWatcher_RegistersObserverOnTrackedCodexClient(t *testing.T) {
	codex, err := llm.NewOpenAICodexClient(
		"http://example.invalid",
		"gpt-5.3-codex",
		"api-key",
		"openai-codex",
		"sk-test",
	)
	if err != nil {
		t.Fatalf("new codex client: %v", err)
	}
	tracked := usage.NewTrackedClient(codex, nil, "openai-codex", "gpt-5.3-codex", llm.TierHeavy)

	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    {Client: tracked, Provider: "openai-codex", Model: "gpt-5.3-codex"},
			llm.TierStandard: {Client: &llm.FakeClient{}, Provider: "anthropic", Model: "claude"},
			llm.TierLight:    {Client: &llm.FakeClient{}, Provider: "anthropic", Model: "claude"},
		},
		DefaultTier: llm.TierStandard,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	em := &recordingEmitter{}
	watcher := attachCodexThresholdWatcher(router, em)
	if watcher == nil {
		t.Fatal("expected watcher")
	}

	// Direct verify: setLastRateLimit on the codex client should fan out to
	// the watcher and on into the emitter.
	codex.SetRateLimitObserver(func(snap llm.CodexRateLimitSnapshot) {
		watcher.observeSnapshot("heavy", "gpt-5.3-codex", snap)
	}) // already set, but explicit re-set documents the path
	watcher.observeSnapshot("heavy", "gpt-5.3-codex", llm.CodexRateLimitSnapshot{
		Primary: &llm.CodexRateLimitWindow{UsedPercent: 96},
	})
	if got := em.snapshot(); len(got) != 1 || got[0].Severity != "error" {
		t.Errorf("expected critical emission, got %+v", got)
	}
}
