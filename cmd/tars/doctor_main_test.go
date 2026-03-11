package main

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommand_DoctorFailsForMissingStarterState(t *testing.T) {
	clearDoctorEnv(t)

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected doctor to fail when starter config and workspace are missing")
	}

	out := stdout.String()
	if !strings.Contains(out, "doctor: FAIL") {
		t.Fatalf("expected FAIL summary, got:\n%s", out)
	}
	if !strings.Contains(out, "config file") {
		t.Fatalf("expected config file check in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--fix") {
		t.Fatalf("expected fix guidance in output, got:\n%s", out)
	}
}

func TestRootCommand_DoctorFixCreatesStarterWorkspaceButStillRequiresBYOK(t *testing.T) {
	clearDoctorEnv(t)

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	workspaceAbs, err := filepath.Abs(workspaceDir)
	if err != nil {
		t.Fatalf("workspace abs path: %v", err)
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir, "--fix"})

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected doctor --fix to keep failing until BYOK is configured")
	}
	if !strings.Contains(err.Error(), "failing checks") {
		t.Fatalf("expected failing checks error, got %v", err)
	}

	configPath := filepath.Join(workspaceAbs, "config", "tars.config.yaml")
	assertPathExists(t, configPath)
	assertPathExists(t, filepath.Join(workspaceAbs, "memory"))
	assertPathExists(t, filepath.Join(workspaceAbs, "MEMORY.md"))

	out := stdout.String()
	if !strings.Contains(out, "[fixed] config file") {
		t.Fatalf("expected fixed config check in output, got:\n%s", out)
	}
	if !strings.Contains(out, "OPENAI_API_KEY") {
		t.Fatalf("expected OPENAI_API_KEY guidance in output, got:\n%s", out)
	}
}

func TestRootCommand_DoctorPassesWhenStarterWorkspaceAndBYOKPresent(t *testing.T) {
	clearDoctorEnv(t)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")

	workspaceDir := filepath.Join(t.TempDir(), "doctor-workspace")
	var initStdout strings.Builder
	initCmd := newRootCommand(strings.NewReader(""), &initStdout, io.Discard)
	initCmd.SetArgs([]string{"init", "--workspace-dir", workspaceDir})
	if err := initCmd.Execute(); err != nil {
		t.Fatalf("init command: %v", err)
	}

	var doctorStdout strings.Builder
	doctorCmd := newRootCommand(strings.NewReader(""), &doctorStdout, io.Discard)
	doctorCmd.SetArgs([]string{"doctor", "--workspace-dir", workspaceDir})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor command: %v", err)
	}

	out := doctorStdout.String()
	if !strings.Contains(out, "doctor: PASS") {
		t.Fatalf("expected PASS summary, got:\n%s", out)
	}
	if strings.Contains(out, "[fail]") {
		t.Fatalf("expected no failing checks, got:\n%s", out)
	}
}

func clearDoctorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
		"GEMINI_API_KEY",
		"OPENAI_CODEX_OAUTH_TOKEN",
		"TARS_OPENAI_CODEX_OAUTH_TOKEN",
		"LLM_API_KEY",
		"TARS_LLM_API_KEY",
		"TARS_WORKSPACE_DIR",
		"TARS_CONFIG",
		"TARS_CONFIG_PATH",
	} {
		t.Setenv(key, "")
	}
}
