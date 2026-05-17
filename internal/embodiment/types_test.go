package embodiment

import (
	"testing"
	"time"
)

func TestEmbodimentTypes(t *testing.T) {
	capturedAt := time.Date(2026, 5, 17, 9, 30, 0, 0, time.UTC)
	percept := Percept{
		ID:         "p-1",
		Provider:   "host",
		Modality:   ModalityAudio,
		Owner:      OwnerOwner,
		Summary:    "owner asked TARS to wake up",
		Labels:     []string{"directive"},
		MediaRef:   "provider://host/audio/p-1",
		CapturedAt: capturedAt,
		Raw:        map[string]any{"sound_level": 0.83},
	}
	if percept.Modality != "audio" || percept.Owner != "owner" || !percept.CapturedAt.Equal(capturedAt) {
		t.Fatalf("percept fields not preserved: %+v", percept)
	}

	desc := ProviderDescriptor{
		Name:         "host",
		Enabled:      true,
		Transport:    TransportWebhook,
		Endpoint:     "http://127.0.0.1:43180/v1/embodiment/percepts",
		Capabilities: []Capability{CapabilityHearing, CapabilitySpeech},
	}
	if desc.Capabilities[0] != "hearing" || desc.Transport != "webhook" {
		t.Fatalf("descriptor constants not stable: %+v", desc)
	}

	action := BodyAction{
		Kind:    ActionSpeak,
		Payload: map[string]any{"text": "hello"},
	}
	if action.Kind != "speak" || action.Payload["text"] != "hello" {
		t.Fatalf("body action fields not preserved: %+v", action)
	}
}
