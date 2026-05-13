package skillhub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInstall_ExplicitSourceRouting exercises the "<source>:<name>" branch of
// resolveSkillSource, the ensureSources lazy init, and the TarsHubSource
// delegation methods (SearchSkills / FetchSkillContent / FetchSkillFile).
func TestInstall_ExplicitSourceRouting(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	tmpDir := t.TempDir()
	inst := &Installer{
		WorkspaceDir: tmpDir,
		Registry: &Registry{
			RegistryURL:  srv.URL + "/registry.json",
			SkillBaseURL: srv.URL,
			HTTPClient:   srv.Client(),
		},
	}

	// Install via "<source>:<name>" — exercises explicit-source resolveSkillSource path.
	if _, err := inst.Install(context.Background(), "tars-hub:project-start"); err != nil {
		t.Fatalf("Install with explicit source: %v", err)
	}

	skills, err := inst.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 1 || skills[0].Source != DefaultSourceID {
		t.Fatalf("expected one tars-hub skill, got %+v", skills)
	}

	// After ensureSources ran, the registered TarsHubSource is reachable.
	src, ok := inst.Sources.Get(DefaultSourceID)
	if !ok {
		t.Fatalf("Sources.Get(tars-hub) returned !ok")
	}

	// Cover TarsHubSource.SearchSkills delegation.
	results, err := src.SearchSkills(context.Background(), "")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("SearchSkills returned no entries")
	}

	// Cover TarsHubSource.FetchSkillContent and FetchSkillFile via FindSkillByName.
	entry, err := src.FindSkillByName(context.Background(), "project-start")
	if err != nil {
		t.Fatalf("FindSkillByName: %v", err)
	}
	if _, err := src.FetchSkillContent(context.Background(), entry); err != nil {
		t.Fatalf("FetchSkillContent: %v", err)
	}
	if _, err := src.FetchSkillFile(context.Background(), entry, "SKILL.md"); err != nil {
		t.Fatalf("FetchSkillFile: %v", err)
	}
}

// TestInstall_UnknownSource verifies resolveSkillSource returns a clear
// error when the explicit source ID isn't registered, listing the known IDs.
func TestInstall_UnknownSource(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Registry: &Registry{
			RegistryURL:  srv.URL + "/registry.json",
			SkillBaseURL: srv.URL,
			HTTPClient:   srv.Client(),
		},
	}

	_, err := inst.Install(context.Background(), "openclaw:foo")
	if err == nil {
		t.Fatalf("expected unknown-source error")
	}
	if !strings.Contains(err.Error(), "openclaw") || !strings.Contains(err.Error(), "tars-hub") {
		t.Fatalf("error %q does not mention both source and known list", err)
	}
}

// TestInstall_ImplicitMultiSourceAmbiguity exercises the "no explicit source +
// multiple registered sources both have the name" branch of resolveSkillSource.
func TestInstall_ImplicitMultiSourceAmbiguity(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	tarsRegistry := &Registry{
		RegistryURL:  srv.URL + "/registry.json",
		SkillBaseURL: srv.URL,
		HTTPClient:   srv.Client(),
	}

	sources := NewSourceRegistry()
	if err := sources.Register(&TarsHubSource{Registry: tarsRegistry}); err != nil {
		t.Fatalf("register tars-hub: %v", err)
	}
	if err := sources.Register(&stubSource{
		id: "openclaw",
		entries: map[string]RegistryEntry{
			"project-start": {Name: "project-start", Version: "1.0.0", Path: "skills/project-start"},
		},
	}); err != nil {
		t.Fatalf("register openclaw: %v", err)
	}

	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Registry:     tarsRegistry,
		Sources:      sources,
	}

	_, err := inst.Install(context.Background(), "project-start")
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error %q does not mention ambiguity", err)
	}
	if !strings.Contains(err.Error(), "openclaw") || !strings.Contains(err.Error(), "tars-hub") {
		t.Fatalf("error %q does not list both candidates", err)
	}
}

// TestInstall_NoMatchInAnySource exercises the "no hit anywhere" branch.
func TestInstall_NoMatchInAnySource(t *testing.T) {
	inst := &Installer{
		WorkspaceDir: t.TempDir(),
		Sources: func() *SourceRegistry {
			r := NewSourceRegistry()
			_ = r.Register(&stubSource{id: "openclaw", entries: map[string]RegistryEntry{}})
			return r
		}(),
	}

	_, err := inst.Install(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatalf("expected not-found error")
	}
}

// TestLoadDB_MigratesLegacyEmptySource writes a skillhub.json that uses the
// pre-federation schema (empty Source on every row) and verifies loadDB
// backfills DefaultSourceID for skills, plugins, and MCPs.
func TestLoadDB_MigratesLegacyEmptySource(t *testing.T) {
	tmpDir := t.TempDir()
	legacy := InstalledDB{
		Skills:  []InstalledSkill{{Name: "novelist", Version: "0.6.0", Source: "", Dir: filepath.Join(tmpDir, "skills/novelist")}},
		Plugins: []InstalledPlugin{{Name: "telegram", Version: "1.0.0", Source: "", Dir: filepath.Join(tmpDir, "plugins/telegram")}},
		MCPs:    []InstalledMCP{{Name: "fs", Version: "1.0.0", Source: "", Dir: filepath.Join(tmpDir, "mcps/fs")}},
	}
	data, err := json.MarshalIndent(&legacy, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dbPath := filepath.Join(tmpDir, installedDBFile)
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatalf("write legacy db: %v", err)
	}

	inst := &Installer{WorkspaceDir: tmpDir}
	db, err := inst.loadDB()
	if err != nil {
		t.Fatalf("loadDB: %v", err)
	}
	if db.Skills[0].Source != DefaultSourceID {
		t.Errorf("skill Source = %q, want %q", db.Skills[0].Source, DefaultSourceID)
	}
	if db.Plugins[0].Source != DefaultSourceID {
		t.Errorf("plugin Source = %q, want %q", db.Plugins[0].Source, DefaultSourceID)
	}
	if db.MCPs[0].Source != DefaultSourceID {
		t.Errorf("mcp Source = %q, want %q", db.MCPs[0].Source, DefaultSourceID)
	}
}

// TestEnsureSources_NilRegistryStillWorks covers the inst.Registry == nil
// branch of ensureSources where it constructs a default Registry.
func TestEnsureSources_NilRegistryStillWorks(t *testing.T) {
	inst := &Installer{WorkspaceDir: t.TempDir()}
	sources := inst.ensureSources()
	if sources == nil || sources.Len() == 0 {
		t.Fatalf("ensureSources did not lazy-init: %+v", sources)
	}
	if inst.Registry == nil {
		t.Fatalf("ensureSources left Registry nil")
	}

	// stubSource that *never* finds anything; the lazy-init source
	// (TarsHubSource backed by a real Registry pointing at nothing) will
	// also fail. We only need to confirm Install returns an error rather
	// than panicking.
	_, err := inst.Install(context.Background(), "nope")
	if err == nil {
		t.Fatalf("expected error from lazy-init install of unknown skill")
	}
	// Sanity: the error should not be a nil-deref/panic message.
	if errors.Is(err, nil) {
		t.Fatalf("unexpected nil error")
	}
}
