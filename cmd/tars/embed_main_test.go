package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/localembed"
	"github.com/devlikebear/tars/internal/memory"
)

func writeEmbedTestConfig(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	path := filepath.Join(dir, "config.yaml")
	content := "workspace_dir: " + workspace + "\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, workspace
}

func runEmbed(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &out, io.Discard)
	cmd.SetArgs(append([]string{"embed"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

// fakeEmbedManagers makes install steps record instead of running uv and
// Python, and points the health probe at a closed port.
func fakeEmbedManagers(t *testing.T) *[]string {
	t.Helper()
	var steps []string
	prev := embedManagerFactory
	t.Cleanup(func() { embedManagerFactory = prev })
	uv := filepath.Join(t.TempDir(), "uv")
	if err := os.WriteFile(uv, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	embedManagerFactory = func(opts localembed.Options) (*localembed.Manager, error) {
		if _, err := localembed.New(opts); err != nil {
			return nil, err
		}
		opts.BaseURL = "http://127.0.0.1:1/v1"
		opts.UVPath = uv
		opts.Runner = func(_ context.Context, name string, args, env []string, _ io.Writer) error {
			steps = append(steps, filepath.Base(name)+" "+args[0])
			if args[0] == "venv" {
				// uv creates the interpreter the marker check looks for.
				python := filepath.Join(opts.Dir, "venv", "bin", "python")
				if runtime.GOOS == "windows" {
					python = filepath.Join(opts.Dir, "venv", "Scripts", "python.exe")
				}
				if err := os.MkdirAll(filepath.Dir(python), 0o755); err != nil {
					return err
				}
				return os.WriteFile(python, nil, 0o755)
			}
			if filepath.Base(name) == "python" || filepath.Base(name) == "python.exe" {
				snap := filepath.Join(opts.Dir, "hf", "hub", "models--"+strings.ReplaceAll(opts.Model, "/", "--"), "snapshots", "x")
				if err := os.MkdirAll(snap, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(snap, "model.safetensors"), []byte("w"), 0o644)
			}
			return nil
		}
		return localembed.New(opts)
	}
	return &steps
}

func TestEmbedCommand_EnableDisableRoundTrip(t *testing.T) {
	fakeEmbedManagers(t)
	path, _ := writeEmbedTestConfig(t, strings.Join([]string{
		"memory_embed_provider: gemini",
		"memory_embed_model: gemini-embedding-2-preview",
		"memory_embed_dimensions: 768",
		"",
	}, "\n"))

	out, err := runEmbed(t, "status", "--config", path)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "enabled:   no") || !strings.Contains(out, config.DefaultLocalEmbedModel) || !strings.Contains(out, "installed: no") {
		t.Fatalf("unexpected status output:\n%s", out)
	}

	out, err = runEmbed(t, "enable", "--config", path)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !strings.Contains(out, "not installed yet") {
		t.Fatalf("expected install hint, got:\n%s", out)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.MemorySemanticEnabled || cfg.MemoryEmbedProvider != "local" || cfg.MemoryEmbedModel != config.DefaultLocalEmbedModel ||
		cfg.MemoryEmbedBaseURL != config.DefaultLocalEmbedBaseURL || cfg.MemoryEmbedDimensions != 1024 {
		t.Fatalf("expected local defaults after enable, got %+v", cfg.MemoryConfig)
	}
	out, _ = runEmbed(t, "status", "--config", path)
	if !strings.Contains(out, "enabled:   yes") {
		t.Fatalf("expected enabled status, got:\n%s", out)
	}

	if _, err := runEmbed(t, "disable", "--config", path); err != nil {
		t.Fatalf("disable: %v", err)
	}
	cfg, _ = config.Load(path)
	if cfg.MemorySemanticEnabled || cfg.MemoryEmbedProvider != "local" {
		t.Fatalf("expected semantic memory off with provider kept, got %+v", cfg.MemoryConfig)
	}
}

func TestEmbedCommand_InstallAndRemove(t *testing.T) {
	steps := fakeEmbedManagers(t)
	path, workspace := writeEmbedTestConfig(t, "")

	out, err := runEmbed(t, "install", "--config", path)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Installed.") || len(*steps) != 3 {
		t.Fatalf("expected three install steps and a done line, got %v\n%s", *steps, out)
	}
	out, _ = runEmbed(t, "status", "--config", path)
	if !strings.Contains(out, "installed: yes (1 B)") || !strings.Contains(out, "python:    ready") {
		t.Fatalf("expected installed status, got:\n%s", out)
	}

	if out, err = runEmbed(t, "remove", "--config", path); err != nil || !strings.Contains(out, "downloads it again") {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	out, _ = runEmbed(t, "status", "--config", path)
	if !strings.Contains(out, "installed: no") || !strings.Contains(out, "python:    ready") {
		t.Fatalf("expected model gone, env kept:\n%s", out)
	}

	if out, err = runEmbed(t, "remove", "--all", "--config", path); err != nil || !strings.Contains(out, "Python environment") {
		t.Fatalf("remove --all: %v\n%s", err, out)
	}
	if _, err := os.Stat(localembed.DirFor(workspace)); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar dir removed, got %v", err)
	}
}

func TestEmbedCommand_BadConfig(t *testing.T) {
	fakeEmbedManagers(t)
	path, _ := writeEmbedTestConfig(t, "memory_embed_provider: local\nmemory_embed_base_url: http://10.0.0.1:43181/v1\n")
	if _, err := runEmbed(t, "status", "--config", path); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected loopback error, got %v", err)
	}
}

func TestHumanBytes(t *testing.T) {
	for n, want := range map[int64]string{12: "12 B", 2048: "2.0 KB", 2582995040: "2.4 GB"} {
		if got := humanBytes(n); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestDoctorLocalEmbed_ReportsInstallState(t *testing.T) {
	steps := fakeEmbedManagers(t)
	var cfg config.Config
	cfg.WorkspaceDir = t.TempDir()
	cfg.MemorySemanticEnabled = true
	cfg.MemoryEmbedProvider = "local"
	cfg.MemoryEmbedModel = config.DefaultLocalEmbedModel
	cfg.MemoryEmbedBaseURL = config.DefaultLocalEmbedBaseURL
	cfg.MemoryEmbedDimensions = config.DefaultLocalEmbedDimensions

	report := doctorReport{}
	checkDoctorSemanticMemory(&report, cfg, "config.yaml")
	if report.lastStatusFor("semantic memory") != "warn" {
		t.Fatalf("expected warn before install, got %+v", report)
	}

	mgr, err := embedManagerFactory(localembed.OptionsForConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Install(context.Background(), nil); err != nil || len(*steps) != 3 {
		t.Fatalf("install: %v %v", err, *steps)
	}
	report = doctorReport{}
	checkDoctorSemanticMemory(&report, cfg, "config.yaml")
	if report.lastStatusFor("semantic memory") != "ok" {
		t.Fatalf("expected ok after install, got %+v", report)
	}

	cfg.MemoryEmbedBaseURL = "http://10.0.0.1:43181/v1"
	report = doctorReport{}
	checkDoctorSemanticMemory(&report, cfg, "config.yaml")
	if report.lastStatusFor("semantic memory") != "fail" {
		t.Fatalf("expected fail for non-loopback base_url, got %+v", report)
	}
}

func TestEmbedCommand_ReindexRequiresEnabled(t *testing.T) {
	fakeEmbedManagers(t)
	path, _ := writeEmbedTestConfig(t, "")
	if _, err := runEmbed(t, "reindex", "--config", path); err == nil || !strings.Contains(err.Error(), "tars embed enable") {
		t.Fatalf("expected enable hint, got %v", err)
	}
	if _, err := runEmbed(t, "enable", "--config", path); err != nil {
		t.Fatal(err)
	}
	if _, err := runEmbed(t, "reindex", "--config", path); !errors.Is(err, localembed.ErrNotInstalled) {
		t.Fatalf("expected ErrNotInstalled, got %v", err)
	}
}

func TestEmbedCommand_ReindexEmbedsLoggedExperiences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "model": config.DefaultLocalEmbedModel})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": []float64{1, 0}}}})
	}))
	defer srv.Close()
	path, workspace := writeEmbedTestConfig(t, strings.Join([]string{
		"memory_semantic_enabled: true",
		"memory_embed_provider: local",
		"memory_embed_base_url: " + srv.URL + "/v1",
		"memory_embed_dimensions: 2",
		"",
	}, "\n"))
	if err := memory.EnsureWorkspace(workspace); err != nil {
		t.Fatal(err)
	}
	if err := memory.AppendExperience(workspace, memory.Experience{Timestamp: time.Now(), Summary: "prefers tea"}); err != nil {
		t.Fatal(err)
	}
	out, err := runEmbed(t, "reindex", "--config", path)
	if err != nil || !strings.Contains(out, "Reindexed 1 memories.") {
		t.Fatalf("reindex: %v\n%s", err, out)
	}

	srv.Close()
	if _, err := runEmbed(t, "reindex", "--config", path); err == nil {
		t.Fatal("expected an error once the sidecar is gone and nothing is installed")
	}
}
