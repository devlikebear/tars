package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAssistantCommand_InstallLaunchAgentLoadsWithLaunchctlHook(t *testing.T) {
	originalGOOS := assistantRuntimeGOOS
	originalLaunchctl := assistantLaunchctlRun
	t.Cleanup(func() {
		assistantRuntimeGOOS = originalGOOS
		assistantLaunchctlRun = originalLaunchctl
	})

	assistantRuntimeGOOS = "darwin"
	var calls [][]string
	assistantLaunchctlRun = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		if len(args) > 0 && args[0] == "load" {
			return []byte("load failed"), errors.New("exit status 1")
		}
		return nil, nil
	}

	plistPath := filepath.Join(t.TempDir(), "io.tars.assistant.test.plist")
	err := runAssistantCommand(context.Background(), assistantOptions{
		action:          "install-launchagent",
		serverURL:       "http://127.0.0.1:43180",
		workspace:       t.TempDir(),
		hotkey:          "cmd+shift+space",
		audioInput:      "default",
		whisperBin:      "whisper-cli",
		whisperLanguage: "ko",
		ffmpegBin:       "ffmpeg",
		ttsBin:          "say",
		label:           "io.tars.assistant.test",
		plistPath:       plistPath,
		installLoad:     true,
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "launchctl load failed") {
		t.Fatalf("expected launchctl load failure, got %v", err)
	}

	if len(calls) != 2 || calls[0][0] != "unload" || calls[1][0] != "load" {
		t.Fatalf("unexpected launchctl calls: %#v", calls)
	}
}

func TestRunAssistantLaunchctlUsesSystemLaunchctl(t *testing.T) {
	_, err := runAssistantLaunchctl(context.Background(), "__tars_test_invalid__")
	if err == nil {
		t.Fatal("expected invalid launchctl invocation to fail")
	}
}
