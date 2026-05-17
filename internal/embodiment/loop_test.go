package embodiment

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestEmbodimentLoopMock(t *testing.T) {
	rt := &fakeCognitionRuntime{
		waitCh: make(chan struct{}),
		waitRun: agentruntime.Run{
			ID:     "run_1",
			Status: agentruntime.RunStatusCompleted,
			Response: "```tars-body-action\n" +
				`[` +
				`{"kind":"speak","payload":{"text":"네, 상태를 확인할게요."}},` +
				`{"kind":"express","payload":{"emotion":"happy"}}` +
				`]` +
				"\n```",
		},
	}
	dispatcher := &recordingActionDispatcher{}
	subsystem := NewWithOptions(config.EmbodimentConfig{
		Enabled: true,
		Providers: []config.EmbodimentProviderConfig{{
			Name:               "mock",
			Enabled:            true,
			Transport:          "mcp",
			Endpoint:           "mock",
			Capabilities:       []string{"hearing", "speech"},
			MinTriggerInterval: "0s",
		}},
	}, zerolog.New(io.Discard), Options{
		Runtime:          rt,
		ActionDispatcher: dispatcher,
		DefaultSessionID: "sess_body",
		DefaultAgent:     "embodied",
		Now:              func() time.Time { return time.Date(2026, 5, 17, 13, 30, 0, 0, time.UTC) },
	})

	result, err := subsystem.IngestPayload(context.Background(), "mock", map[string]any{
		"x-embodiment": true,
		"modality":     "audio",
		"owner":        "owner",
		"summary":      "Owner asked TARS to report status aloud.",
	})
	if err != nil {
		t.Fatalf("IngestPayload: %v", err)
	}
	if !result.CognitionResult.Triggered {
		t.Fatalf("cognition did not trigger: %+v", result)
	}

	close(rt.waitCh)
	eventually(t, time.Second, func() bool { return !subsystem.cognition.InFlight("mock", "sess_body") })
	if len(dispatcher.actions) != 1 {
		t.Fatalf("delivered actions = %+v, want exactly one speak action", dispatcher.actions)
	}
	if dispatcher.actions[0].Kind != ActionSpeak || dispatcher.actions[0].Payload["text"] != "네, 상태를 확인할게요." {
		t.Fatalf("delivered action = %+v", dispatcher.actions[0])
	}
}
