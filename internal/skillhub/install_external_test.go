package skillhub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// externalStubSource is a HubSource that pretends to be an external hub.
// It implements SkillContentConverter, LicenseFetcher, and
// CompanionFileLister so the external install branch exercises every
// optional capability the installer routes through.
type externalStubSource struct {
	stubSource
	manifest   []byte
	converted  []byte
	companion  map[string][]byte
	license    []byte
	licenseLbl string
	warnings   []string
}

func (s *externalStubSource) FetchSkillContent(_ context.Context, _ *RegistryEntry) ([]byte, error) {
	return s.manifest, nil
}

func (s *externalStubSource) FetchSkillFile(_ context.Context, _ *RegistryEntry, rel string) ([]byte, error) {
	body, ok := s.companion[rel]
	if !ok {
		return nil, errors.New("not found")
	}
	return body, nil
}

func (s *externalStubSource) ConvertSkillContent(_ *RegistryEntry, _ []byte) ([]byte, []string, error) {
	return s.converted, s.warnings, nil
}

func (s *externalStubSource) FetchLicense(_ context.Context, _ *RegistryEntry) ([]byte, string, error) {
	return s.license, s.licenseLbl, nil
}

func (s *externalStubSource) ListCompanionFiles(_ context.Context, _ *RegistryEntry) ([]string, error) {
	out := make([]string, 0, len(s.companion))
	for k := range s.companion {
		out = append(out, k)
	}
	return out, nil
}

func TestInstallWithOptions_ExternalHub_PromptApproved(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	var previewSeen *InstallPreview
	result, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{
		Confirm: func(p *InstallPreview) (bool, error) {
			previewSeen = p
			return true, nil
		},
	})
	if err != nil {
		t.Fatalf("InstallWithOptions: %v", err)
	}
	if result == nil {
		t.Fatalf("nil result")
	}
	if previewSeen == nil {
		t.Fatalf("Confirm was never called")
	}
	if previewSeen.SourceID != "demo" {
		t.Errorf("preview source = %q, want demo", previewSeen.SourceID)
	}
	if !previewSeen.AttributionPresent {
		t.Errorf("preview should advertise ATTRIBUTION.md")
	}
	if len(previewSeen.AdapterWarnings) != 1 {
		t.Errorf("expected 1 warning, got %v", previewSeen.AdapterWarnings)
	}

	skillDir := filepath.Join(inst.WorkspaceDir, "skills", "foo")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "ATTRIBUTION.md")); err != nil {
		t.Errorf("ATTRIBUTION.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "scripts", "run.sh")); err != nil {
		t.Errorf("companion file missing: %v", err)
	}

	// DB row should record the external source ID.
	skills, err := inst.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 1 || skills[0].Source != "demo" {
		t.Fatalf("expected one demo skill, got %+v", skills)
	}
}

func TestInstallWithOptions_ExternalHub_PromptRejected(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	_, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{
		Confirm: func(*InstallPreview) (bool, error) { return false, nil },
	})
	if !errors.Is(err, ErrInstallAborted) {
		t.Fatalf("expected ErrInstallAborted, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(inst.WorkspaceDir, "skills", "foo")); !os.IsNotExist(err) {
		t.Errorf("workspace should be clean after abort, got stat err = %v", err)
	}
}

func TestInstallWithOptions_ExternalHub_RequiresConfirmation(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	_, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{})
	if err == nil {
		t.Fatalf("expected error when neither Yes nor Confirm is provided")
	}
	if !strings.Contains(err.Error(), "confirmation") {
		t.Errorf("error %q does not mention confirmation", err)
	}
}

func TestInstallWithOptions_ExternalHub_AutoYes(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	if _, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true}); err != nil {
		t.Fatalf("InstallWithOptions with Yes: %v", err)
	}
}

func TestInstall_LegacyEntry_DefaultsToYes(t *testing.T) {
	// Legacy `Install(ctx, ref)` must not require interactive confirmation
	// for external hubs — otherwise pre-federation tests and direct
	// programmatic call sites would break.
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	if _, err := inst.Install(context.Background(), "demo:foo"); err != nil {
		t.Fatalf("legacy Install: %v", err)
	}
}

// TestDownloadExternalSkillFiles_FetchError verifies that an error from
// FetchSkillContent is wrapped (not swallowed) and that the materialize
// step is never reached.
func TestDownloadExternalSkillFiles_FetchError(t *testing.T) {
	src := &fetchErrorSource{stubSource: stubSource{
		id: "demo",
		entries: map[string]RegistryEntry{
			"foo": {Name: "foo", Path: "skills/foo"},
		},
	}}
	inst := newInstallerWithSource(t, src)

	_, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true})
	if err == nil {
		t.Fatalf("expected fetch error")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("error %q does not mention fetch", err)
	}
}

// TestDownloadExternalSkillFiles_LicenseError verifies the LicenseFetcher
// failure path returns a wrapped error and skips materialize.
func TestDownloadExternalSkillFiles_LicenseError(t *testing.T) {
	src := &licenseErrorSource{externalStubSource: *newExternalStub()}
	src.licenseLbl = LicenseUnknown
	inst := newInstallerWithSource(t, src)

	_, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true})
	if err == nil {
		t.Fatalf("expected attribution error for unknown license")
	}
	if !strings.Contains(err.Error(), "attribution") {
		t.Errorf("error %q does not mention attribution", err)
	}
}

type fetchErrorSource struct {
	stubSource
}

func (s *fetchErrorSource) FetchSkillContent(_ context.Context, _ *RegistryEntry) ([]byte, error) {
	return nil, errors.New("simulated fetch failure")
}

func (s *fetchErrorSource) FetchSkillFile(_ context.Context, _ *RegistryEntry, _ string) ([]byte, error) {
	return nil, errors.New("simulated fetch failure")
}

// licenseErrorSource overrides only the LicenseFetcher behaviour of
// externalStubSource to return an Unknown label.
type licenseErrorSource struct {
	externalStubSource
}

func (s *licenseErrorSource) FetchLicense(_ context.Context, _ *RegistryEntry) ([]byte, string, error) {
	return s.license, LicenseUnknown, nil
}

func TestSkillFileChecksums(t *testing.T) {
	files := map[string][]byte{
		"a.txt": []byte("hello"),
		"b.txt": []byte("world"),
	}
	sums := SkillFileChecksums(files)
	if len(sums) != 2 {
		t.Fatalf("expected 2 sums, got %d", len(sums))
	}
	// sha256("hello") -> known constant
	if sums["a.txt"] != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Errorf("sha256(hello) mismatch: %s", sums["a.txt"])
	}
}

// helpers

func newExternalStub() *externalStubSource {
	return &externalStubSource{
		stubSource: stubSource{
			id: "demo",
			entries: map[string]RegistryEntry{
				"foo": {Name: "foo", Path: "skills/foo"},
			},
		},
		manifest:   []byte("---\nname: foo\ndescription: hi\n---\nraw body"),
		converted:  []byte("---\nname: foo\ndescription: hi\nuser_invocable: true\n---\nconverted body"),
		companion:  map[string][]byte{"scripts/run.sh": []byte("#!/bin/sh\n")},
		license:    []byte("MIT License\n\nCopyright (c) 2025 Demo\n\nPermission is hereby granted, free of charge"),
		licenseLbl: LicenseMIT,
		warnings:   []string{"install block skipped: brew install foo"},
	}
}

func newInstallerWithSource(t *testing.T, src HubSource) *Installer {
	t.Helper()
	sources := NewSourceRegistry()
	if err := sources.Register(src); err != nil {
		t.Fatalf("register stub source: %v", err)
	}
	return &Installer{
		WorkspaceDir: t.TempDir(),
		Sources:      sources,
	}
}
