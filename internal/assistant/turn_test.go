package assistant

import (
	"context"
	"errors"
	"testing"
)

func TestRunTextTurn_TTSFailureFallsBackToText(t *testing.T) {
	result, err := RunTextTurn(context.Background(), VoiceTurnDeps{
		ChatClient: fakeChat{reply: "설정을 정리했어요", session: "s1"},
		Speaker:    fakeSpeaker{err: errors.New("tts failed")},
		SessionID:  "s0",
	}, "설정 정리해줘")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Transcript != "설정 정리해줘" {
		t.Fatalf("unexpected transcript: %q", result.Transcript)
	}
	if result.AssistantReply != "설정을 정리했어요" {
		t.Fatalf("unexpected assistant reply: %q", result.AssistantReply)
	}
	if result.SessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", result.SessionID)
	}
	if result.TTSError == "" {
		t.Fatalf("expected TTSError to be captured")
	}
}

func TestRunTextTurn_UsesExistingSessionWhenReplyOmitsSessionID(t *testing.T) {
	result, err := RunTextTurn(context.Background(), VoiceTurnDeps{
		ChatClient: fakeChat{reply: "다음 장면 초안을 만들었어요", session: ""},
		SessionID:  "s-existing",
	}, "다음 장면 이어서 써줘")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.SessionID != "s-existing" {
		t.Fatalf("expected existing session id, got %q", result.SessionID)
	}
}

func TestRunTextTurn_EmptyInputReturnsError(t *testing.T) {
	_, err := RunTextTurn(context.Background(), VoiceTurnDeps{
		ChatClient: fakeChat{reply: "ignored", session: "s1"},
	}, "   ")
	if err == nil {
		t.Fatalf("expected error")
	}
}
