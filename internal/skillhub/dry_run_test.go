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

func TestPreviewInstall_ExternalHub_NoMaterialize(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	preview, err := inst.PreviewInstall(context.Background(), "demo:foo")
	if err != nil {
		t.Fatalf("PreviewInstall: %v", err)
	}
	if preview.SourceID != "demo" {
		t.Errorf("source_id = %q, want demo", preview.SourceID)
	}
	if preview.OriginalName != "foo" {
		t.Errorf("original_name = %q", preview.OriginalName)
	}
	if preview.ConvertedSkill.Name != "foo" {
		t.Errorf("converted_skill.name = %q, want foo (parsed from converted SKILL.md)", preview.ConvertedSkill.Name)
	}
	if !preview.ConvertedSkill.UserInvocable {
		t.Errorf("converted skill should be user_invocable=true")
	}
	if len(preview.Files) < 2 {
		t.Errorf("expected at least 2 files (SKILL.md + ATTRIBUTION.md), got %v", preview.Files)
	}
	for _, fp := range preview.Files {
		if fp.SHA256 == "" {
			t.Errorf("file %q missing sha256", fp.Path)
		}
	}
	if preview.LicenseSource != AttributionFilename {
		t.Errorf("preview should record ATTRIBUTION.md as license source")
	}
	if preview.LicenseLabel != LicenseMIT {
		t.Errorf("license_label = %q, want %q", preview.LicenseLabel, LicenseMIT)
	}

	// Workspace must be untouched.
	if _, err := os.Stat(filepath.Join(inst.WorkspaceDir, "skills")); !os.IsNotExist(err) {
		t.Errorf("workspace should not contain skills/ after preview, stat err = %v", err)
	}
}

func TestInstallWithOptions_DryRun_ReturnsPreview(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	var onPreviewSeen *DryRunResult
	result, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{
		DryRun:    true,
		OnPreview: func(p *DryRunResult) { onPreviewSeen = p },
	})
	if err != nil {
		t.Fatalf("InstallWithOptions DryRun: %v", err)
	}
	if result.DryRunPreview == nil {
		t.Fatalf("DryRunPreview should be set")
	}
	if onPreviewSeen == nil {
		t.Fatalf("OnPreview should have been called")
	}
	if onPreviewSeen != result.DryRunPreview {
		t.Errorf("OnPreview should receive the same preview that's returned")
	}
	if _, err := os.Stat(filepath.Join(inst.WorkspaceDir, "skills")); !os.IsNotExist(err) {
		t.Errorf("workspace must remain empty under DryRun")
	}
}

func TestDryRunResult_JSONMarshalable(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	preview, err := inst.PreviewInstall(context.Background(), "demo:foo")
	if err != nil {
		t.Fatalf("PreviewInstall: %v", err)
	}
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(body), `"source_id":"demo"`) {
		t.Errorf("JSON missing source_id: %s", body)
	}
	if !strings.Contains(string(body), `"sha256"`) {
		t.Errorf("JSON missing per-file sha256: %s", body)
	}
}

// TestInstallWithOptions_ConfirmReturnsError exercises the (false, error)
// branch of opts.Confirm in InstallWithOptions.
func TestInstallWithOptions_ConfirmReturnsError(t *testing.T) {
	src := newExternalStub()
	inst := newInstallerWithSource(t, src)

	wantErr := errors.New("simulated confirm failure")
	_, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{
		Confirm: func(*DryRunResult) (bool, error) { return false, wantErr },
	})
	if err == nil {
		t.Fatalf("expected error from Confirm")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}

// TestInstallWithOptions_RequiresPluginPropagates verifies that when a skill
// entry advertises a requires_plugin and that plugin is NOT already
// installed, InstallResult.RequiresPlugin is populated.
func TestInstallWithOptions_RequiresPluginPropagates(t *testing.T) {
	stub := newExternalStub()
	stub.entries["foo"] = RegistryEntry{
		Name:           "foo",
		Path:           "skills/foo",
		RequiresPlugin: "some-plugin",
	}
	inst := newInstallerWithSource(t, stub)

	result, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{Yes: true})
	if err != nil {
		t.Fatalf("InstallWithOptions: %v", err)
	}
	if result.RequiresPlugin != "some-plugin" {
		t.Errorf("RequiresPlugin = %q, want some-plugin", result.RequiresPlugin)
	}
}

// driftingSource returns different converted bytes on every conversion
// after the first, the way a real converter does when it stamps the import
// time or upstream moves between preview and confirm.
type driftingSource struct {
	externalStubSource
	converted int
}

func (s *driftingSource) ConvertSkillContent(_ *RegistryEntry, _ []byte) ([]byte, []string, error) {
	s.converted++
	if s.converted > 1 {
		return []byte("---\nname: foo\ndescription: drifted\n---\nchanged body"), s.warnings, nil
	}
	return s.externalStubSource.converted, s.warnings, nil
}

// TestInstallWithOptions_MaterializesApprovedPreview pins the contract that
// the bytes written to disk are the ones the user approved: the source is
// fetched and converted once, and every installed file hashes to the
// sha256 shown in the preview.
func TestInstallWithOptions_MaterializesApprovedPreview(t *testing.T) {
	stub := &driftingSource{externalStubSource: *newExternalStub()}
	inst := newInstallerWithSource(t, stub)

	var approved *DryRunResult
	_, err := inst.InstallWithOptions(context.Background(), "demo:foo", InstallOptions{
		Confirm: func(p *DryRunResult) (bool, error) {
			approved = p
			return true, nil
		},
	})
	if err != nil {
		t.Fatalf("InstallWithOptions: %v", err)
	}
	if approved == nil {
		t.Fatalf("Confirm was not called")
	}
	if stub.converted != 1 {
		t.Errorf("ConvertSkillContent called %d times, want 1 (no post-confirm re-download)", stub.converted)
	}

	skillDir := filepath.Join(inst.WorkspaceDir, "skills", "foo")
	manifest, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	if string(manifest) != string(stub.externalStubSource.converted) {
		t.Errorf("installed SKILL.md = %q, want the previewed conversion %q", manifest, stub.externalStubSource.converted)
	}
	for _, fp := range approved.Files {
		body, err := os.ReadFile(filepath.Join(skillDir, filepath.FromSlash(fp.Path)))
		if err != nil {
			t.Fatalf("read installed %s: %v", fp.Path, err)
		}
		if got := computeSHA256Hex(body); got != fp.SHA256 {
			t.Errorf("installed %s sha256 = %s, preview showed %s", fp.Path, got, fp.SHA256)
		}
	}
}

func TestExpectedChecksumMap(t *testing.T) {
	entry := &RegistryEntry{
		Files: RegistryFiles{
			{Path: "SKILL.md", SHA256: "abc"},
			{Path: "scripts/foo.sh", SHA256: ""}, // empty → omitted
			{Path: "templates/x.md", SHA256: "def"},
		},
	}
	got := expectedChecksumMap(entry)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (empty sha256 skipped), got %d: %v", len(got), got)
	}
	if got["SKILL.md"] != "abc" {
		t.Errorf("SKILL.md = %q, want abc", got["SKILL.md"])
	}
	if _, has := got["scripts/foo.sh"]; has {
		t.Errorf("scripts/foo.sh should be omitted when sha256 is empty")
	}
}

func TestParseSkillPreview_Lenient(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no frontmatter", "no fm here", ""},
		{"unterminated", "---\nname: foo\n", ""},
		{"empty body", "---\nname: foo\ndescription: bar\n---\n", "foo"},
		{"with tags", "---\nname: foo\ndescription: bar\ntags: [a, b]\n---\n", "foo"},
	}
	for _, tt := range cases {
		got := parseSkillPreview([]byte(tt.in))
		if got.Name != tt.want {
			t.Errorf("%s: Name = %q, want %q", tt.name, got.Name, tt.want)
		}
	}
}

func TestDetectAttributionLabel(t *testing.T) {
	body := []byte("# Attribution\n\n- **Original name**: foo\n- **License**: MIT\n\n## MIT License\n...")
	if got := detectAttributionLabel(body); got != "MIT" {
		t.Errorf("detectAttributionLabel = %q, want MIT", got)
	}
	if got := detectAttributionLabel([]byte("no license line here")); got != "" {
		t.Errorf("expected empty when license line is absent, got %q", got)
	}
}
