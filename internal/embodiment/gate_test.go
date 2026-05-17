package embodiment

import (
	"testing"
	"time"
)

func TestGateDecision(t *testing.T) {
	now := time.Date(2026, 5, 17, 11, 45, 0, 0, time.UTC)
	clock := now
	gate := NewGate(GateConfig{
		MinTriggerInterval: 30 * time.Second,
		MaxTriggersPerHour: 2,
		Now:                func() time.Time { return clock },
	})

	ownerVoice := Percept{
		Provider:      "stackchan",
		Modality:      ModalityAudio,
		Owner:         OwnerOwner,
		Summary:       "Owner said wake up.",
		IsSelfSensory: true,
		CapturedAt:    now,
	}
	first := gate.Decide(ownerVoice)
	if !first.Trigger || first.Mode != GateModeDirective {
		t.Fatalf("owner voice should trigger directive, got %+v", first)
	}

	second := gate.Decide(ownerVoice)
	if second.Trigger || second.Mode != GateModeObservation || second.Reason != GateReasonDebounce {
		t.Fatalf("immediate duplicate should debounce, got %+v", second)
	}

	clock = clock.Add(time.Minute)
	stranger := ownerVoice
	stranger.Owner = OwnerStranger
	third := gate.Decide(stranger)
	if third.Trigger || third.Mode != GateModeObservation {
		t.Fatalf("stranger should stay observation by default, got %+v", third)
	}

	clock = clock.Add(time.Minute)
	fourth := gate.Decide(ownerVoice)
	if !fourth.Trigger {
		t.Fatalf("owner voice after debounce should trigger, got %+v", fourth)
	}

	clock = clock.Add(time.Minute)
	fifth := gate.Decide(ownerVoice)
	if fifth.Trigger || fifth.Reason != GateReasonRateLimited {
		t.Fatalf("third owner trigger inside hour should rate-limit, got %+v", fifth)
	}
}

func TestGateObservationCanBeConfiguredToTrigger(t *testing.T) {
	gate := NewGate(GateConfig{TriggerObservations: true})
	decision := gate.Decide(Percept{
		Provider:      "host",
		Modality:      ModalitySensor,
		Owner:         OwnerUnknown,
		Summary:       "A loud ambient sound occurred.",
		IsSelfSensory: true,
	})
	if !decision.Trigger || decision.Mode != GateModeObservation {
		t.Fatalf("configured observation trigger = %+v", decision)
	}
}
