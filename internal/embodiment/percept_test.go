package embodiment

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizePercept(t *testing.T) {
	now := time.Date(2026, 5, 17, 11, 30, 0, 0, time.UTC)
	tests := []struct {
		name      string
		provider  string
		knownBody bool
		payload   map[string]any
		want      Percept
		wantErr   string
	}{
		{
			name:      "owner voice from legacy stackchan webhook",
			provider:  "stackchan",
			knownBody: true,
			payload: map[string]any{
				"source":              "stackchan",
				"ts":                  float64(1779000000123),
				"trigger":             "event",
				"salience":            float64(0.92),
				"summary":             "The owner asked for status.",
				"text":                "The owner asked for status.",
				"identity":            "owner",
				"identity_confidence": float64(0.86),
				"identity_modality":   "voice",
				"audio_ref":           "obs-1779000000123.wav",
				"session_id":          "sess_body",
			},
			want: Percept{
				Provider:      "stackchan",
				Modality:      ModalityAudio,
				Owner:         OwnerOwner,
				Summary:       "The owner asked for status.",
				MediaRef:      "obs-1779000000123.wav",
				Trigger:       "event",
				Salience:      0.92,
				SessionID:     "sess_body",
				IsSelfSensory: true,
				CapturedAt:    time.UnixMilli(1779000000123).UTC(),
			},
		},
		{
			name:      "ambient sensor stays observation",
			provider:  "host",
			knownBody: true,
			payload: map[string]any{
				"x-embodiment": true,
				"modality":     "sensor",
				"owner":        "none",
				"summary":      "Ambient keyboard noise.",
			},
			want: Percept{
				Provider:      "host",
				Modality:      ModalitySensor,
				Owner:         OwnerNone,
				Summary:       "Ambient keyboard noise.",
				IsSelfSensory: true,
				CapturedAt:    now,
			},
		},
		{
			name:     "provider and summary can come from legacy payload",
			provider: "",
			payload: map[string]any{
				"source":      " StackChan ",
				"captured_at": "2026-05-17T11:31:00+09:00",
				"message": map[string]any{
					"summary": "Owner appeared in the room.",
				},
				"identity":  "unknown",
				"image_ref": "frame-1778999460000.jpg",
			},
			want: Percept{
				Provider:   "stackchan",
				Modality:   ModalityVision,
				Owner:      OwnerUnknown,
				Summary:    "Owner appeared in the room.",
				MediaRef:   "frame-1778999460000.jpg",
				CapturedAt: time.Date(2026, 5, 17, 2, 31, 0, 0, time.UTC),
			},
		},
		{
			name:     "explicit media and labels are normalized",
			provider: "host",
			payload: map[string]any{
				"id":         json.Number("42"),
				"timestamp":  int64(1779000060),
				"modality":   "camera",
				"owner":      "ambient",
				"summary":    "Screen brightness changed.",
				"media_ref":  "capture://screen/1",
				"salience":   "0.45",
				"thread_id":  "thread_body",
				"labels":     []any{" Screen ", "screen", 7, true, ""},
				"extra_note": testStringer("note"),
			},
			want: Percept{
				ID:         "42",
				Provider:   "host",
				Modality:   ModalityVision,
				Owner:      OwnerNone,
				Summary:    "Screen brightness changed.",
				Labels:     []string{"Screen", "7", "true"},
				MediaRef:   "capture://screen/1",
				Salience:   0.45,
				ThreadID:   "thread_body",
				CapturedAt: time.Unix(1779000060, 0).UTC(),
			},
		},
		{
			name:      "malformed missing summary",
			provider:  "host",
			knownBody: true,
			payload: map[string]any{
				"x-embodiment": true,
				"owner":        "owner",
			},
			wantErr: "summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePercept(tt.provider, tt.payload, NormalizeOptions{
				KnownBody: tt.knownBody,
				Now:       func() time.Time { return now },
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("NormalizePercept error = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePercept: %v", err)
			}
			if got.Provider != tt.want.Provider ||
				got.Modality != tt.want.Modality ||
				got.Owner != tt.want.Owner ||
				got.Summary != tt.want.Summary ||
				got.MediaRef != tt.want.MediaRef ||
				got.Trigger != tt.want.Trigger ||
				got.SessionID != tt.want.SessionID ||
				got.IsSelfSensory != tt.want.IsSelfSensory ||
				!got.CapturedAt.Equal(tt.want.CapturedAt) {
				t.Fatalf("percept mismatch\n got: %+v\nwant: %+v", got, tt.want)
			}
			if got.ID != tt.want.ID || got.ThreadID != tt.want.ThreadID {
				t.Fatalf("percept ids mismatch\n got: %+v\nwant: %+v", got, tt.want)
			}
			if got.Salience != tt.want.Salience {
				t.Fatalf("salience = %v, want %v", got.Salience, tt.want.Salience)
			}
			if strings.Join(got.Labels, ",") != strings.Join(tt.want.Labels, ",") {
				t.Fatalf("labels = %+v, want %+v", got.Labels, tt.want.Labels)
			}
			if len(got.Raw) == 0 {
				t.Fatalf("expected raw payload copy")
			}
		})
	}
}

func TestLooksLikePerceptPayload(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		knownBody bool
		payload   map[string]any
		want      bool
	}{
		{name: "empty", provider: "host", knownBody: true, payload: map[string]any{}, want: false},
		{name: "explicit bool", provider: "host", payload: map[string]any{"x-embodiment": true}, want: true},
		{name: "explicit string", provider: "host", payload: map[string]any{"embodiment": "true"}, want: true},
		{name: "legacy known source with summary", provider: "stackchan", knownBody: true, payload: map[string]any{"source": "stackchan", "summary": "Owner spoke."}, want: true},
		{name: "legacy unknown source ignored", provider: "stackchan", knownBody: false, payload: map[string]any{"source": "stackchan", "summary": "Owner spoke."}, want: false},
		{name: "modality requires summary", provider: "host", payload: map[string]any{"modality": "audio"}, want: false},
		{name: "modality with nested message", provider: "host", payload: map[string]any{"modality": "audio", "message": map[string]any{"text": "Owner spoke."}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikePerceptPayload(tt.provider, tt.payload, tt.knownBody); got != tt.want {
				t.Fatalf("LooksLikePerceptPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizePerceptRequiresProvider(t *testing.T) {
	_, err := NormalizePercept("", map[string]any{"summary": "Owner spoke."}, NormalizeOptions{})
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("NormalizePercept error = %v, want provider error", err)
	}
}

func TestPerceptHelpers(t *testing.T) {
	t.Run("string conversion covers webhook primitives", func(t *testing.T) {
		values := []struct {
			in   any
			want string
		}{
			{in: nil, want: ""},
			{in: " owner ", want: " owner "},
			{in: json.Number("7.5"), want: "7.5"},
			{in: testStringer("spoken"), want: "spoken"},
			{in: float64(7), want: "7"},
			{in: float64(7.25), want: "7.25"},
			{in: int(3), want: "3"},
			{in: int64(4), want: "4"},
			{in: true, want: "true"},
			{in: false, want: "false"},
			{in: []string{"x"}, want: "[x]"},
		}
		for _, tt := range values {
			if got := asStringAny(tt.in); got != tt.want {
				t.Fatalf("asStringAny(%T) = %q, want %q", tt.in, got, tt.want)
			}
		}
	})

	t.Run("salience conversion accepts numeric encodings", func(t *testing.T) {
		values := []struct {
			in   any
			want float64
		}{
			{in: float64(0.1), want: 0.1},
			{in: float32(0.2), want: float64(float32(0.2))},
			{in: int(1), want: 1},
			{in: int64(2), want: 2},
			{in: json.Number("0.75"), want: 0.75},
			{in: "0.9", want: 0.9},
			{in: "not-a-number", want: 0},
		}
		for _, tt := range values {
			if got := floatFromAny(tt.in); got != tt.want {
				t.Fatalf("floatFromAny(%T) = %v, want %v", tt.in, got, tt.want)
			}
		}
	})

	t.Run("captured time accepts common encodings", func(t *testing.T) {
		fallback := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
		values := []struct {
			payload map[string]any
			want    time.Time
		}{
			{payload: map[string]any{"captured_at": ""}, want: fallback},
			{payload: map[string]any{"captured_at": "2026-05-17T21:01:00+09:00"}, want: time.Date(2026, 5, 17, 12, 1, 0, 0, time.UTC)},
			{payload: map[string]any{"timestamp": "1779000000"}, want: time.Unix(1779000000, 0).UTC()},
			{payload: map[string]any{"ts": json.Number("1779000000123")}, want: time.UnixMilli(1779000000123).UTC()},
			{payload: map[string]any{"ts": int(1779000060)}, want: time.Unix(1779000060, 0).UTC()},
			{payload: map[string]any{"ts": int64(1779000120)}, want: time.Unix(1779000120, 0).UTC()},
			{payload: map[string]any{"ts": "not-a-time"}, want: fallback},
			{payload: map[string]any{}, want: fallback},
		}
		for _, tt := range values {
			got := capturedAtFromPayload(tt.payload, fallback)
			if !got.Equal(tt.want) {
				t.Fatalf("capturedAtFromPayload(%+v) = %s, want %s", tt.payload, got, tt.want)
			}
		}
	})

	t.Run("modalities owners and media refs normalize aliases", func(t *testing.T) {
		if normalizeOwnerState("stranger") != OwnerStranger ||
			normalizeOwnerState("unknown") != OwnerUnknown ||
			normalizeOwnerState("nobody") != "" {
			t.Fatalf("owner aliases did not normalize")
		}
		if normalizeModality("vision") != ModalityVision ||
			normalizeModality("voice") != ModalityAudio ||
			normalizeModality("sensors") != ModalitySensor ||
			normalizeModality("taste") != "" {
			t.Fatalf("modality aliases did not normalize")
		}
		if inferModality(map[string]any{"audio_ref": "a.wav"}) != ModalityAudio {
			t.Fatal("audio_ref should infer audio")
		}
		if mediaRefFromPayload(map[string]any{"audio_ref": "a.wav"}, ModalitySensor) != "a.wav" {
			t.Fatal("default media ref should prefer audio")
		}
		if mediaRefFromPayload(map[string]any{"image_ref": "i.jpg"}, ModalitySensor) != "i.jpg" {
			t.Fatal("default media ref should fall back to image")
		}
	})

	t.Run("string slices and payload copies are defensive", func(t *testing.T) {
		if strings.Join(stringSliceFromAny([]string{" speech ", "Speech", "", "vision"}), ",") != "speech,vision" {
			t.Fatalf("[]string labels were not normalized")
		}
		if got := stringSliceFromAny("speech"); got != nil {
			t.Fatalf("non-slice labels = %+v, want nil", got)
		}
		if got := clonePayload(map[string]any{}); got != nil {
			t.Fatalf("empty clone = %+v, want nil", got)
		}
	})
}

type testStringer string

func (s testStringer) String() string {
	return string(s)
}
