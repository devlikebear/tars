package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRun_MissingTargetCommandFails(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{}, stdout, stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "target_command is required") {
		t.Fatalf("expected target_command error, stderr=%q", stderr.String())
	}
}

func TestRun_InvalidConfigPathFails(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")}, stdout, stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "failed to load cased config") {
		t.Fatalf("expected config load error, stderr=%q", stderr.String())
	}
}

func TestRun_UsesDefaultConfigPathWhenPresent(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	path := filepath.Join(configDir, "cased.yaml")
	if err := os.WriteFile(path, []byte("target_command: ./cmd/tarsd\nautostart: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}
	defer func() { _ = os.Chdir(wd) }()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := run([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", code, stderr.String())
	}
}
