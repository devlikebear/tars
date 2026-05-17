package embodiment

import (
	"context"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestSubsystemNoop(t *testing.T) {
	t.Run("disabled config does not start", func(t *testing.T) {
		before := runtime.NumGoroutine()
		subsystem := New(config.EmbodimentConfig{}, zerolog.New(io.Discard))
		if err := subsystem.Start(context.Background()); err != nil {
			t.Fatalf("start disabled subsystem: %v", err)
		}
		subsystem.Stop()
		after := runtime.NumGoroutine()
		if after > before {
			t.Fatalf("disabled subsystem started goroutines: before=%d after=%d", before, after)
		}
		if status := subsystem.Status(); status.Enabled {
			t.Fatalf("disabled subsystem status = %+v", status)
		}
	})

	t.Run("enabled config without enabled providers stays inactive", func(t *testing.T) {
		subsystem := New(config.EmbodimentConfig{
			Enabled: true,
			Providers: []config.EmbodimentProviderConfig{{
				Name:         "host",
				Enabled:      false,
				Transport:    "webhook",
				Capabilities: []string{"hearing"},
			}},
		}, zerolog.New(io.Discard))
		if err := subsystem.Start(context.Background()); err != nil {
			t.Fatalf("start empty subsystem: %v", err)
		}
		defer subsystem.Stop()
		if status := subsystem.Status(); status.Enabled {
			t.Fatalf("empty subsystem status = %+v", status)
		}
	})
}

func TestSubsystemIngestOwnerVoiceTriggersCognition(t *testing.T) {
	rt := &fakeCognitionRuntime{waitCh: make(chan struct{})}
	subsystem := NewWithOptions(config.EmbodimentConfig{
		Enabled: true,
		Providers: []config.EmbodimentProviderConfig{{
			Name:               "stackchan",
			Enabled:            true,
			Transport:          "webhook",
			Capabilities:       []string{"hearing", "speech"},
			MinTriggerInterval: "0s",
		}},
	}, zerolog.New(io.Discard), Options{
		Runtime:          rt,
		DefaultSessionID: "sess_main",
		DefaultAgent:     "embodied",
		Now:              func() time.Time { return time.Date(2026, 5, 17, 12, 30, 0, 0, time.UTC) },
	})

	result, err := subsystem.IngestPayload(context.Background(), "stackchan", map[string]any{
		"x-embodiment": true,
		"modality":     "audio",
		"owner":        "owner",
		"summary":      "Owner asked for a status update.",
	})
	if err != nil {
		t.Fatalf("IngestPayload: %v", err)
	}
	if !result.Decision.Trigger || result.Decision.Mode != GateModeDirective {
		t.Fatalf("expected directive trigger, got %+v", result.Decision)
	}
	if !result.CognitionResult.Triggered || result.CognitionResult.RunID == "" {
		t.Fatalf("expected cognition run, got %+v", result.CognitionResult)
	}
	req := rt.lastSpawn()
	if req.SessionID != "sess_main" || req.Agent != "embodied" {
		t.Fatalf("unexpected spawn request: %+v", req)
	}
	if !strings.Contains(req.SystemPromptAppend, "너는 몸이 있다") {
		t.Fatalf("expected embodied system prompt append, got %q", req.SystemPromptAppend)
	}
	close(rt.waitCh)
}

func TestSubsystemIngestAmbientObservationDoesNotTrigger(t *testing.T) {
	rt := &fakeCognitionRuntime{waitCh: make(chan struct{})}
	subsystem := NewWithOptions(config.EmbodimentConfig{
		Enabled: true,
		Providers: []config.EmbodimentProviderConfig{{
			Name:         "host",
			Enabled:      true,
			Transport:    "webhook",
			Capabilities: []string{"hearing"},
		}},
	}, zerolog.New(io.Discard), Options{Runtime: rt})

	result, err := subsystem.IngestPayload(context.Background(), "host", map[string]any{
		"x-embodiment": true,
		"modality":     "sensor",
		"owner":        "none",
		"summary":      "Ambient noise.",
	})
	if err != nil {
		t.Fatalf("IngestPayload: %v", err)
	}
	if result.Decision.Trigger || result.Decision.Mode != GateModeObservation {
		t.Fatalf("ambient should stay observation, got %+v", result.Decision)
	}
	if rt.spawnCount() != 0 {
		t.Fatalf("ambient observation spawned %d runs", rt.spawnCount())
	}
	close(rt.waitCh)
}
