package assistant

import "testing"

func TestParsePromptDialogOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want popupResult
	}{
		{
			name: "send with text",
			raw:  "button returned:Send, text returned:hello world",
			want: popupResult{Action: popupActionSend, Text: "hello world"},
		},
		{
			name: "mic ignores text",
			raw:  "button returned:Mic, text returned:",
			want: popupResult{Action: popupActionMic},
		},
		{
			name: "cancel",
			raw:  "button returned:Cancel, text returned:draft",
			want: popupResult{Action: popupActionCancel, Text: "draft"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePromptDialogOutput(tc.raw)
			if err != nil {
				t.Fatalf("parse prompt output: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestParseRecordingDialogOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "stop", raw: "button returned:Stop", want: true},
		{name: "cancel", raw: "button returned:Cancel", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRecordingDialogOutput(tc.raw)
			if err != nil {
				t.Fatalf("parse recording output: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}

func TestPopupPreviewText_TrimsAndLimitsLength(t *testing.T) {
	got := popupPreviewText("line1\n\nline2\nline3", 12)
	if got != "line1 lin..." {
		t.Fatalf("unexpected preview text: %q", got)
	}
}
