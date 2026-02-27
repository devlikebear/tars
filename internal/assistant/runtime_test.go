package assistant

import (
	"context"
	"errors"
	"testing"
)

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
