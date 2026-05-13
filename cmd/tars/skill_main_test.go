package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/devlikebear/tars/internal/skillhub/sources/openclaw"
)

const sampleOpenclawSKILL = `---
name: github
description: "Use gh for GitHub issues."
metadata:
  {
    "openclaw":
      {
        "requires": { "bins": ["gh"] },
        "install":
          [
            {
              "id": "brew",
              "kind": "brew",
              "formula": "gh",
              "bins": ["gh"],
              "label": "Install GitHub CLI (brew)",
            },
          ],
      },
  }
---

# GitHub skill body
`

// newMockOpenclawServers spins up httptest servers that mimic GitHub Contents
// API and raw.githubusercontent.com for an openclaw install test.
func newMockOpenclawServers(t *testing.T) (apiURL, rawURL string, cleanup func()) {
	t.Helper()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/openclaw/openclaw/contents/skills":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "github", "path": "skills/github", "type": "dir"},
			})
		case "/repos/openclaw/openclaw/contents/skills/github":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "SKILL.md", "path": "skills/github/SKILL.md", "type": "file", "size": int64(len(sampleOpenclawSKILL))},
			})
		default:
			http.NotFound(w, r)
		}
	}))

	rawSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openclaw/openclaw/main/skills/github/SKILL.md":
			_, _ = w.Write([]byte(sampleOpenclawSKILL))
		case "/openclaw/openclaw/main/LICENSE":
			_, _ = w.Write([]byte("MIT License\n\nCopyright (c) 2025 Test\n\nPermission is hereby granted, free of charge, to any person obtaining a copy"))
		default:
			http.NotFound(w, r)
		}
	}))

	cleanup = func() {
		apiSrv.Close()
		rawSrv.Close()
	}
	return apiSrv.URL, rawSrv.URL, cleanup
}

// installerFactoryWithOpenclaw returns a newSkillInstaller-equivalent that
// uses the mock httptest URLs instead of the real GitHub.
func installerFactoryWithOpenclaw(apiURL, rawURL string) func(string) *skillhub.Installer {
	return func(workspaceDir string) *skillhub.Installer {
		inst := skillhub.NewInstaller(workspaceDir)
		mock := openclaw.New()
		mock.APIBaseURL = apiURL
		mock.RawBaseURL = rawURL
		mock.TokenEnv = "TARS_TEST_TOKEN_UNSET"
		_ = inst.Sources.Register(mock)
		return inst
	}
}

func TestBuildSkillRef(t *testing.T) {
	tests := []struct {
		from string
		name string
		want string
	}{
		{"", "foo", "foo"},
		{"openclaw", "foo", "openclaw:foo"},
		{"  openclaw  ", "foo", "openclaw:foo"},
		{"openclaw", "  foo  ", "openclaw:foo"},
	}
	for _, tt := range tests {
		if got := buildSkillRef(tt.from, tt.name); got != tt.want {
			t.Errorf("buildSkillRef(%q, %q) = %q, want %q", tt.from, tt.name, got, tt.want)
		}
	}
}

// runSkillInstallE2E exercises the Install callback path used by
// `tars skill install`, but bypasses cobra and the global registry wiring
// by invoking newHubResourceCommand directly with a factory we control.
//
// It returns the captured stdout/stderr plus the workspace it materialized into.
func runSkillInstallE2E(t *testing.T, factory func(string) *skillhub.Installer, args ...string) (stdoutS, stderrS, workspace string, err error) {
	t.Helper()

	workspace = t.TempDir()
	var stdout, stderr bytes.Buffer

	spec := hubResourceSpec{
		Use:        "skill",
		Short:      "test",
		Noun:       "skill",
		PluralNoun: "skills",
		Search: func(_ context.Context, _ io.Writer, _, _ string) error {
			return nil
		},
		Install: func(ctx context.Context, stdout, stderr io.Writer, workspaceDir, name string, opts HubInstallOptions) error {
			inst := factory(workspaceDir)
			ref := buildSkillRef(opts.From, name)
			_, err := inst.InstallWithOptions(ctx, ref, skillhub.InstallOptions{Yes: opts.Yes})
			if err != nil {
				return err
			}
			fprintf(stdout, "Installed skill %q to %s/skills/%s\n", name, workspaceDir, name)
			return nil
		},
		Uninstall: func(_, _ io.Writer, _, _ string) error { return nil },
		List:      func(_ io.Writer, _ string) error { return nil },
		Update:    func(_ context.Context, _, _ io.Writer, _ string) error { return nil },
		Info:      func(_ context.Context, _ io.Writer, _, _ string) error { return nil },
	}

	cmd := newHubResourceCommand(spec, &stdout, &stderr)
	full := append([]string{"install"}, args...)
	full = append(full, "--workspace-dir", workspace)
	cmd.SetArgs(full)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err = cmd.Execute()
	return stdout.String(), stderr.String(), workspace, err
}

func TestSkillInstall_OpenclawWithFromAndYes(t *testing.T) {
	apiURL, rawURL, cleanup := newMockOpenclawServers(t)
	defer cleanup()

	stdout, _, ws, err := runSkillInstallE2E(t, installerFactoryWithOpenclaw(apiURL, rawURL),
		"github", "--from", "openclaw", "--yes")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(stdout, "Installed skill") {
		t.Errorf("stdout missing success line: %s", stdout)
	}

	for _, want := range []string{"SKILL.md", "ATTRIBUTION.md"} {
		path := filepath.Join(ws, "skills", "github", want)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing materialized file %s: %v", path, err)
		}
	}

	dbPath := filepath.Join(ws, "skillhub.json")
	dbBody, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read DB: %v", err)
	}
	if !strings.Contains(string(dbBody), `"source": "openclaw"`) {
		t.Errorf("DB does not record openclaw source: %s", dbBody)
	}
}

func TestSkillInstall_ExternalHubRefusesWithoutConfirmation(t *testing.T) {
	apiURL, rawURL, cleanup := newMockOpenclawServers(t)
	defer cleanup()

	// No --yes, no TTY → installer must error out (confirmation required).
	_, _, _, err := runSkillInstallE2E(t, installerFactoryWithOpenclaw(apiURL, rawURL),
		"github", "--from", "openclaw")
	if err == nil {
		t.Fatalf("expected error when neither --yes nor a Confirm callback is supplied")
	}
	if !strings.Contains(err.Error(), "confirmation") {
		t.Errorf("error %q does not mention confirmation", err)
	}
}

func TestSkillInstall_DryRunTextSkipsMaterialize(t *testing.T) {
	apiURL, rawURL, cleanup := newMockOpenclawServers(t)
	defer cleanup()
	workspace := t.TempDir()
	factory := installerFactoryWithOpenclaw(apiURL, rawURL)

	var stdout, stderr bytes.Buffer
	cmd := newSkillCommandWithFactory(&stdout, &stderr, factory)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"install", "github", "--from", "openclaw", "--dry-run", "--workspace-dir", workspace})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install --dry-run: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"Install preview", "github", "openclaw", "Dry run: no files were written"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n---\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "skills", "github")); !os.IsNotExist(err) {
		t.Errorf("workspace should be untouched after --dry-run, stat err = %v", err)
	}
}

func TestSkillInstall_DryRunJSONFormat(t *testing.T) {
	apiURL, rawURL, cleanup := newMockOpenclawServers(t)
	defer cleanup()
	workspace := t.TempDir()
	factory := installerFactoryWithOpenclaw(apiURL, rawURL)

	var stdout, stderr bytes.Buffer
	cmd := newSkillCommandWithFactory(&stdout, &stderr, factory)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"install", "github", "--from", "openclaw", "--dry-run", "--format", "json", "--workspace-dir", workspace})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install --dry-run --format json: %v", err)
	}
	if !strings.Contains(stdout.String(), `"source_id": "openclaw"`) {
		t.Errorf("JSON output missing expected key: %s", stdout.String())
	}
}

func TestSkillInstall_RejectsUnknownFormat(t *testing.T) {
	apiURL, rawURL, cleanup := newMockOpenclawServers(t)
	defer cleanup()
	factory := installerFactoryWithOpenclaw(apiURL, rawURL)

	var stdout, stderr bytes.Buffer
	cmd := newSkillCommandWithFactory(&stdout, &stderr, factory)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"install", "github", "--from", "openclaw", "--dry-run", "--format", "yaml", "--workspace-dir", t.TempDir()})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--format must be text or json") {
		t.Errorf("expected unknown-format error, got %v", err)
	}
}

func TestRenderDryRunText_IncludesEverything(t *testing.T) {
	p := &skillhub.DryRunResult{
		SourceID:     "openclaw",
		OriginalName: "github",
		OriginalPath: "skills/github",
		TargetDir:    "/tmp/ws/skills/github",
		ConvertedSkill: skillhub.SkillPreview{
			Name:          "github",
			Description:   "Use gh",
			Version:       "0.0.0",
			Author:        "openclaw",
			Tags:          []string{"github", "gh"},
			UserInvocable: true,
		},
		Files: []skillhub.FilePreview{
			{Path: "SKILL.md", Size: 100, SHA256: "deadbeef" + strings.Repeat("0", 56)},
			{Path: "ATTRIBUTION.md", Size: 200, SHA256: "cafebabe" + strings.Repeat("0", 56), ExpectedSHA256: "different" + strings.Repeat("0", 55)},
		},
		AdapterWarnings:  []string{"install block skipped: brew install gh"},
		ChecksumWarnings: []string{"checksum mismatch for ATTRIBUTION.md"},
		LicenseLabel:     "MIT",
		LicenseSource:    "ATTRIBUTION.md",
	}
	var buf bytes.Buffer
	renderDryRunText(&buf, p, true)
	out := buf.String()
	for _, want := range []string{
		"openclaw",
		"Skill      : github",
		"License    : MIT",
		"Converted frontmatter:",
		"description : Use gh",
		"version     : 0.0.0",
		"author      : openclaw",
		"tags        : [github, gh]",
		"Adapter warnings:",
		"install block skipped",
		"Checksum warnings:",
		"checksum mismatch",
		"ATTRIBUTION.md will be created",
		"Dry run: no files were written",
		"⚠ checksum mismatch",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q\n---\n%s", want, out)
		}
	}
}

func TestShortHash(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"deadbeefcafebabe", "deadbeefcafe"},
	}
	for _, tt := range cases {
		if got := shortHash(tt.in); got != tt.want {
			t.Errorf("shortHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSkillCobra_AllSubcommandsExercise drives the real newSkillCommand
// (cobra wiring + every callback) with a factory pointed at httptest
// servers. It covers search/install/list/info/uninstall in one pass so the
// closure-heavy newSkillCommand body picks up coverage.
func TestSkillCobra_AllSubcommandsExercise(t *testing.T) {
	apiURL, rawURL, cleanup := newMockOpenclawServers(t)
	defer cleanup()
	factory := installerFactoryWithOpenclaw(apiURL, rawURL)
	workspace := t.TempDir()

	run := func(args ...string) (string, string, error) {
		var stdout, stderr bytes.Buffer
		cmd := newSkillCommandWithFactory(&stdout, &stderr, factory)
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}

	// search → exercises Search closure (lists openclaw skill).
	stdout, _, err := run("search", "--from", "openclaw")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(stdout, "openclaw") || !strings.Contains(stdout, "github") {
		t.Errorf("search output missing openclaw/github: %s", stdout)
	}

	// install --yes → exercises Install + ConfirmFn skip path.
	stdout, _, err = run("install", "github", "--from", "openclaw", "--yes",
		"--workspace-dir", workspace)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(stdout, "Installed skill") {
		t.Errorf("install stdout missing success: %s", stdout)
	}

	// list → exercises List closure.
	stdout, _, err = run("list", "--workspace-dir", workspace)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "github") || !strings.Contains(stdout, "openclaw") {
		t.Errorf("list output missing github/openclaw: %s", stdout)
	}

	// info → exercises Info closure + LookupSkill.
	stdout, _, err = run("info", "github", "--from", "openclaw")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(stdout, "Source:") || !strings.Contains(stdout, "openclaw") {
		t.Errorf("info output missing source line: %s", stdout)
	}

	// uninstall → exercises Uninstall closure.
	stdout, _, err = run("uninstall", "github", "--workspace-dir", workspace)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(stdout, "Uninstalled") {
		t.Errorf("uninstall output missing line: %s", stdout)
	}
}

func TestSkillCobra_SearchEmpty(t *testing.T) {
	// Factory that returns an installer whose sole source matches nothing.
	factory := func(workspaceDir string) *skillhub.Installer {
		inst := skillhub.NewInstaller(workspaceDir)
		// Override sources to a single empty stub.
		inst.Sources = skillhub.NewSourceRegistry()
		_ = inst.Sources.Register(&emptyHubSource{})
		return inst
	}
	var stdout, stderr bytes.Buffer
	cmd := newSkillCommandWithFactory(&stdout, &stderr, factory)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"search"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(stdout.String(), "No skills found") {
		t.Errorf("expected empty-search message, got: %s", stdout.String())
	}
}

// emptyHubSource is a minimal HubSource that advertises nothing.
type emptyHubSource struct{}

func (emptyHubSource) ID() string { return "empty" }
func (emptyHubSource) SearchSkills(_ context.Context, _ string) ([]skillhub.RegistryEntry, error) {
	return nil, nil
}
func (emptyHubSource) FindSkillByName(_ context.Context, _ string) (*skillhub.RegistryEntry, error) {
	return nil, errEmptySource
}
func (emptyHubSource) FetchSkillContent(_ context.Context, _ *skillhub.RegistryEntry) ([]byte, error) {
	return nil, errEmptySource
}
func (emptyHubSource) FetchSkillFile(_ context.Context, _ *skillhub.RegistryEntry, _ string) ([]byte, error) {
	return nil, errEmptySource
}

var errEmptySource = errEmpty{}

type errEmpty struct{}

func (errEmpty) Error() string { return "not found" }

// TestNewSkillInstaller_ProductionFactory ensures the production factory
// returns a healthy installer with openclaw auto-registered.
func TestNewSkillInstaller_ProductionFactory(t *testing.T) {
	inst := newSkillInstaller(t.TempDir())
	if inst == nil {
		t.Fatalf("newSkillInstaller returned nil")
	}
	if _, ok := inst.Sources.Get("openclaw"); !ok {
		t.Errorf("openclaw not registered")
	}
	if _, ok := inst.Sources.Get("tars-hub"); !ok {
		t.Errorf("tars-hub not registered")
	}
}

// TestPrintHelpers exercises fprintln and fprint so they don't sit at 0%
// coverage. They are best-effort error swallowers — nothing meaningful to
// assert beyond "does not panic".
func TestPrintHelpers(t *testing.T) {
	var buf bytes.Buffer
	fprintln(&buf, "hello")
	fprint(&buf, "world")
	if !strings.Contains(buf.String(), "hello") || !strings.Contains(buf.String(), "world") {
		t.Errorf("helpers did not write expected text: %s", buf.String())
	}
}
