package embodiment

import (
	"strings"
	"testing"
)

func TestExtractBodyActions(t *testing.T) {
	response := strings.Join([]string{
		"Done.",
		"```tars-body-action",
		`[`,
		`  {"kind":"speak","payload":{"text":"알겠습니다.","volume":0.3}},`,
		`  {"kind":"express","payload":{"expression":"happy"}},`,
		`  {"kind":"move","payload":{"pan_deg":10,"tilt_deg":30,"speed":0.5}},`,
		`  {"kind":"led","payload":{"pattern":"solid","color":"#00AEEF","brightness":0.5}}`,
		`]`,
		"```",
	}, "\n")

	actions, err := ExtractBodyActions(response)
	if err != nil {
		t.Fatalf("ExtractBodyActions: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("actions len = %d, want 4: %+v", len(actions), actions)
	}
	if actions[0].Kind != ActionSpeak || actions[0].Payload["text"] != "알겠습니다." {
		t.Fatalf("speak action not normalized: %+v", actions[0])
	}
	if actions[1].Kind != ActionExpress || actions[1].Payload["emotion"] != "happy" {
		t.Fatalf("express action not normalized: %+v", actions[1])
	}
	if actions[2].Kind != ActionMove || actions[2].Payload["pan_deg"] == nil {
		t.Fatalf("move action not normalized: %+v", actions[2])
	}
	if actions[3].Kind != ActionLED || actions[3].Payload["color"] != "#00AEEF" {
		t.Fatalf("led action not normalized: %+v", actions[3])
	}
}

func TestExtractBodyActionsSingleObjectAndNoMarker(t *testing.T) {
	actions, err := ExtractBodyActions("```tars-body-action\n{\"kind\":\"speak\",\"text\":\"hello\"}\n```")
	if err != nil {
		t.Fatalf("ExtractBodyActions single object: %v", err)
	}
	if len(actions) != 1 || actions[0].Payload["text"] != "hello" {
		t.Fatalf("single action = %+v", actions)
	}

	actions, err = ExtractBodyActions("plain response")
	if err != nil {
		t.Fatalf("plain response should not error: %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("plain response actions = %+v, want empty", actions)
	}
}

func TestNormalizeBodyActionRejectsMalformed(t *testing.T) {
	tests := []struct {
		name   string
		action BodyAction
	}{
		{name: "unknown kind", action: BodyAction{Kind: "dance", Payload: map[string]any{"text": "x"}}},
		{name: "empty speak text", action: BodyAction{Kind: ActionSpeak, Payload: map[string]any{"text": "   "}}},
		{name: "empty expression", action: BodyAction{Kind: ActionExpress, Payload: map[string]any{"emotion": ""}}},
		{name: "empty move", action: BodyAction{Kind: ActionMove, Payload: map[string]any{}}},
		{name: "empty led", action: BodyAction{Kind: ActionLED, Payload: map[string]any{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NormalizeBodyAction(tt.action); err == nil {
				t.Fatalf("NormalizeBodyAction(%+v) succeeded, want error", tt.action)
			}
		})
	}
}

func TestExtractBodyActionsRejectsInvalidMarker(t *testing.T) {
	_, err := ExtractBodyActions("```tars-body-action\nnot-json\n```")
	if err == nil {
		t.Fatal("expected invalid marker JSON to fail")
	}
}
