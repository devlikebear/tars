package assistant

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestTreatRecordingStopError_AllowsExit255WhenWAVExists(t *testing.T) {
	dir := t.TempDir()
	wavPath := filepath.Join(dir, "sample.wav")
	if err := os.WriteFile(wavPath, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}

	err := treatRecordingStopError(errors.New("exit status 255"), wavPath)
	if err != nil {
		t.Fatalf("expected exit status 255 to be ignored when wav exists, got %v", err)
	}
}

func TestTreatRecordingStopError_RejectsTinyOrMissingWAV(t *testing.T) {
	dir := t.TempDir()
	tinyPath := filepath.Join(dir, "tiny.wav")
	if err := os.WriteFile(tinyPath, []byte("tiny"), 0o644); err != nil {
		t.Fatalf("write tiny wav: %v", err)
	}

	if err := treatRecordingStopError(errors.New("exit status 255"), tinyPath); err == nil {
		t.Fatalf("expected tiny wav to still fail")
	}
	if err := treatRecordingStopError(errors.New("exit status 255"), filepath.Join(dir, "missing.wav")); err == nil {
		t.Fatalf("expected missing wav to still fail")
	}
}

func TestBuildWhisperArgs_IncludesModelWhenConfigured(t *testing.T) {
	args := buildWhisperArgs("sample.wav", "/tmp/ggml-base.bin", "")
	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %#v", args)
	}
	if args[0] != "-m" || args[1] != "/tmp/ggml-base.bin" || args[2] != "-np" || args[3] != "-nt" || args[4] != "sample.wav" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildWhisperArgs_UsesQuietTranscriptFlagsWithoutModel(t *testing.T) {
	args := buildWhisperArgs("sample.wav", "", "")
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %#v", args)
	}
	if args[0] != "-np" || args[1] != "-nt" || args[2] != "sample.wav" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildWhisperArgs_IncludesLanguageWhenConfigured(t *testing.T) {
	args := buildWhisperArgs("sample.wav", "/tmp/ggml-base.bin", "ko")
	if len(args) != 7 {
		t.Fatalf("expected 7 args, got %#v", args)
	}
	if args[0] != "-m" || args[1] != "/tmp/ggml-base.bin" || args[2] != "-np" || args[3] != "-nt" || args[4] != "-l" || args[5] != "ko" || args[6] != "sample.wav" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestExtractTranscriptOutput_StripsWhisperLogs(t *testing.T) {
	raw := "whisper_init_from_file_with_params_no_state: loading model from 'models/ggml-base.bin'\n" +
		"whisper_backend_init: using BLAS backend\n" +
		"\n" +
		" 안녕하세요 반갑습니다.  \n"
	got := extractTranscriptOutput(raw)
	if got != "안녕하세요 반갑습니다." {
		t.Fatalf("unexpected transcript: %q", got)
	}
}
