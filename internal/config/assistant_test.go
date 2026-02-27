package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_AssistantDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if cfg.AssistantHotkey != "Ctrl+Option+Space" {
		t.Fatalf("expected default assistant hotkey, got %q", cfg.AssistantHotkey)
	}
	if cfg.AssistantWhisperBin == "" || cfg.AssistantFFmpegBin == "" || cfg.AssistantTTSBin == "" {
		t.Fatalf("expected default assistant binaries, got whisper=%q ffmpeg=%q tts=%q", cfg.AssistantWhisperBin, cfg.AssistantFFmpegBin, cfg.AssistantTTSBin)
	}
}

func TestLoad_AssistantEnvOverrides(t *testing.T) {
	t.Setenv("TARS_ASSISTANT_HOTKEY", "Ctrl+Shift+Space")
	t.Setenv("TARS_ASSISTANT_WHISPER_BIN", "whisper-custom")
	t.Setenv("TARS_ASSISTANT_FFMPEG_BIN", "ffmpeg-custom")
	t.Setenv("TARS_ASSISTANT_TTS_BIN", "say-custom")

	dir := t.TempDir()
	path := filepath.Join(dir, "assistant.yaml")
	if err := os.WriteFile(path, []byte("assistant_hotkey: Ctrl+Option+Space\nassistant_whisper_bin: whisper-yaml\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AssistantHotkey != "Ctrl+Shift+Space" {
		t.Fatalf("expected env override hotkey, got %q", cfg.AssistantHotkey)
	}
	if cfg.AssistantWhisperBin != "whisper-custom" {
		t.Fatalf("expected env override whisper bin, got %q", cfg.AssistantWhisperBin)
	}
	if cfg.AssistantFFmpegBin != "ffmpeg-custom" {
		t.Fatalf("expected env override ffmpeg bin, got %q", cfg.AssistantFFmpegBin)
	}
	if cfg.AssistantTTSBin != "say-custom" {
		t.Fatalf("expected env override tts bin, got %q", cfg.AssistantTTSBin)
	}
}
