package embodiment

import (
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
			if got.Salience != tt.want.Salience {
				t.Fatalf("salience = %v, want %v", got.Salience, tt.want.Salience)
			}
			if len(got.Raw) == 0 {
				t.Fatalf("expected raw payload copy")
			}
		})
	}
}
