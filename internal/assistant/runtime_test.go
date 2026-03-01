package assistant

import (
	"context"
	"errors"
	"testing"
)

func TestResolveAVFoundationAudioInput_Default(t *testing.T) {
	if got := resolveAVFoundationAudioInput(""); got != ":default" {
		t.Fatalf("expected :default for empty input, got %q", got)
	}
	if got := resolveAVFoundationAudioInput("default"); got != ":default" {
		t.Fatalf("expected :default for default input, got %q", got)
	}
}

func TestResolveAVFoundationAudioInput_Custom(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "1", want: ":1"},
		{input: ":2", want: ":2"},
		{input: "none:0", want: "none:0"},
	}
	for _, tc := range tests {
		if got := resolveAVFoundationAudioInput(tc.input); got != tc.want {
			t.Fatalf("input=%q expected %q, got %q", tc.input, tc.want, got)
		}
	}
}

type fakeTranscriber struct {
	text string
	err  error
}

func (f fakeTranscriber) Transcribe(_ context.Context, _ string) (string, error) {
	return f.text, f.err
}

type fakeChat struct {
	reply   string
	session string
	err     error
}

func (f fakeChat) Chat(_ context.Context, _ string, _ string) (string, string, error) {
	return f.reply, f.session, f.err
}

type fakeSpeaker struct {
	err error
}

func (f fakeSpeaker) Speak(_ context.Context, _ string) error {
	return f.err
}

func TestRunVoiceTurn_TTSFailureFallsBackToText(t *testing.T) {
	result, err := RunVoiceTurn(context.Background(), VoiceTurnDeps{
		Transcriber: fakeTranscriber{text: "회의 준비 알려줘"},
		ChatClient:  fakeChat{reply: "내일 오후 3시에 알려드릴게요", session: "s1"},
		Speaker:     fakeSpeaker{err: errors.New("tts failed")},
		SessionID:   "s0",
	}, "sample.wav")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Transcript != "회의 준비 알려줘" {
		t.Fatalf("unexpected transcript: %q", result.Transcript)
	}
	if result.AssistantReply == "" {
		t.Fatalf("expected assistant reply")
	}
	if result.SessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", result.SessionID)
	}
	if result.TTSError == "" {
		t.Fatalf("expected TTSError to be captured")
	}
}

func TestRunVoiceTurn_STTFailureReturnsError(t *testing.T) {
	_, err := RunVoiceTurn(context.Background(), VoiceTurnDeps{
		Transcriber: fakeTranscriber{err: errors.New("stt failed")},
		ChatClient:  fakeChat{reply: "ignored", session: "s1"},
		Speaker:     fakeSpeaker{},
		SessionID:   "s0",
	}, "sample.wav")
	if err == nil {
		t.Fatalf("expected error")
	}
}
