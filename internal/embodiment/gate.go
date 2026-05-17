package embodiment

import (
	"strings"
	"sync"
	"time"
)

type GateConfig struct {
	MinTriggerInterval  time.Duration
	MaxTriggersPerHour  int
	TriggerObservations bool
	Now                 func() time.Time
}

type Gate struct {
	cfg GateConfig

	mu          sync.Mutex
	lastTrigger map[string]time.Time
	recent      map[string][]time.Time
}

func NewGate(cfg GateConfig) *Gate {
	return &Gate{
		cfg:         cfg,
		lastTrigger: map[string]time.Time{},
		recent:      map[string][]time.Time{},
	}
}

func (g *Gate) Decide(percept Percept) GateDecision {
	if g == nil {
		return GateDecision{Mode: GateModeObservation, Reason: GateReasonDisabled}
	}
	if !percept.IsSelfSensory {
		return GateDecision{Mode: GateModeObservation, Reason: GateReasonExternal}
	}
	key := gateKey(percept)
	now := g.now()
	mode := GateModeObservation
	reason := GateReasonObservation
	shouldTrigger := false
	if percept.Owner == OwnerOwner && percept.Modality == ModalityAudio {
		mode = GateModeDirective
		reason = GateReasonOwnerVoice
		shouldTrigger = true
	} else if g.cfg.TriggerObservations {
		shouldTrigger = true
	}
	if !shouldTrigger {
		return GateDecision{Mode: mode, Reason: reason}
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.rateLimitedLocked(key, now) {
		return GateDecision{Mode: GateModeObservation, Reason: GateReasonRateLimited}
	}
	if g.debouncedLocked(key, now) {
		return GateDecision{Mode: GateModeObservation, Reason: GateReasonDebounce}
	}
	g.recordLocked(key, now)
	return GateDecision{Trigger: true, Mode: mode, Reason: reason}
}

func (g *Gate) now() time.Time {
	if g.cfg.Now != nil {
		return g.cfg.Now().UTC()
	}
	return time.Now().UTC()
}

func (g *Gate) rateLimitedLocked(key string, now time.Time) bool {
	max := g.cfg.MaxTriggersPerHour
	if max <= 0 {
		return false
	}
	cutoff := now.Add(-time.Hour)
	recent := g.recent[key]
	kept := recent[:0]
	for _, ts := range recent {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	g.recent[key] = kept
	return len(kept) >= max
}

func (g *Gate) debouncedLocked(key string, now time.Time) bool {
	minInterval := g.cfg.MinTriggerInterval
	if minInterval <= 0 {
		return false
	}
	last := g.lastTrigger[key]
	return !last.IsZero() && now.Sub(last) < minInterval
}

func (g *Gate) recordLocked(key string, now time.Time) {
	g.lastTrigger[key] = now
	g.recent[key] = append(g.recent[key], now)
}

func gateKey(percept Percept) string {
	provider := normalizeName(percept.Provider)
	sessionID := strings.TrimSpace(percept.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(percept.ThreadID)
	}
	if sessionID == "" {
		sessionID = "default"
	}
	return provider + ":" + sessionID
}
