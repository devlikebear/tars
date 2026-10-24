package skillhub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// Focus templates from the hub. A template is text only — stage names,
// kinds and instructions, no commands — so its install sandbox is the same
// parse-and-validate LoadTemplates runs. The installer writes it to
// <workspace>/focus-templates through focuspipeline.SaveTemplate, where
// the focus pages pick it up like any user template.

const (
	focusTemplateManifest    = "template.yaml"
	focusTemplatePackageType = "focus_template"
)

// InstallFocusTemplate downloads, validates and installs a focus template
// from the registry. It refuses to replace a local template of the same id
// that the hub did not install.
func (inst *Installer) InstallFocusTemplate(ctx context.Context, name string) (*InstallResult, error) {
	entry, err := inst.Registry.FindFocusTemplateByName(ctx, name)
	if err != nil {
		return nil, err
	}
	installed, report, err := inst.installFocusTemplateEntry(ctx, entry)
	if err != nil {
		return nil, err
	}
	if err := inst.addFocusTemplateToDB(installed); err != nil {
		return nil, err
	}
	return &InstallResult{Sandbox: report}, nil
}

// UninstallFocusTemplate removes a hub focus template and its file.
func (inst *Installer) UninstallFocusTemplate(name string) error {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("focus template %q is not installed", name)
		}
		return err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	found := false
	remaining := db.FocusTemplates[:0:0]
	for _, row := range db.FocusTemplates {
		if strings.ToLower(row.Name) != key {
			remaining = append(remaining, row)
			continue
		}
		found = true
		if inst.focusTemplateFileExists(row) {
			if err := focuspipeline.DeleteTemplate(inst.focusTemplateDir(), row.Name); err != nil {
				return fmt.Errorf("remove focus template %q: %w", row.Name, err)
			}
		}
	}
	if !found {
		return fmt.Errorf("focus template %q is not installed", name)
	}
	db.FocusTemplates = remaining
	return inst.saveDB(db)
}

// ListFocusTemplates returns the hub focus templates whose file is still
// in the workspace. A template deleted from the focus templates page drops
// out here without touching the installed DB.
func (inst *Installer) ListFocusTemplates() ([]InstalledFocusTemplate, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []InstalledFocusTemplate
	for _, row := range db.FocusTemplates {
		if inst.focusTemplateFileExists(row) {
			out = append(out, row)
		}
	}
	return out, nil
}

// UpdateFocusTemplates reinstalls hub focus templates that have a newer
// registry version. A template edited since it was installed is skipped,
// so an update never overwrites the user's changes.
func (inst *Installer) UpdateFocusTemplates(ctx context.Context) (UpdateResult, error) {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return UpdateResult{}, nil
		}
		return UpdateResult{}, err
	}
	var result UpdateResult
	for i, row := range db.FocusTemplates {
		entry, err := inst.Registry.FindFocusTemplateByName(ctx, row.Name)
		if err != nil {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: row.Name, Err: err})
			continue
		}
		if entry.Version == row.Version {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: row.Name, Reason: reasonUpToDate})
			continue
		}
		if skip := inst.focusTemplateUpdateBlocker(row); skip != "" {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: row.Name, Reason: skip})
			continue
		}
		next, _, err := inst.installFocusTemplateEntry(ctx, entry)
		if err != nil {
			result.Failed = append(result.Failed, UpdateDiagnostic{Name: row.Name, Err: err})
			continue
		}
		db.FocusTemplates[i] = next
		result.Updated = append(result.Updated, row.Name)
	}
	return result, errors.Join(updateFailuresError("focus templates", result.Failed), inst.saveUpdatedDB(db, result, "focus templates"))
}

// focusTemplateUpdateBlocker says why an installed template must not be
// replaced by an update, or "" when it may be.
func (inst *Installer) focusTemplateUpdateBlocker(row InstalledFocusTemplate) string {
	if !inst.focusTemplateFileExists(row) {
		return "template file was removed; install it again to restore it"
	}
	raw, err := os.ReadFile(inst.focusTemplatePath(row))
	if err != nil {
		return "template file cannot be read: " + err.Error()
	}
	if row.SHA256 != "" && computeSHA256Hex(raw) != row.SHA256 {
		return "edited locally; uninstall and install again to replace your changes"
	}
	return ""
}

func (inst *Installer) installFocusTemplateEntry(ctx context.Context, entry *FocusTemplateEntry) (InstalledFocusTemplate, SandboxReport, error) {
	files, err := inst.downloadVerifiedHubFiles(entry.Name, "focus template", entry.Files, focusTemplateManifest, func(relPath string) ([]byte, error) {
		return inst.Registry.FetchFocusTemplateFile(ctx, entry, relPath)
	})
	if err != nil {
		return InstalledFocusTemplate{}, SandboxReport{}, err
	}
	report := SandboxReport{
		PackageType:  focusTemplatePackageType,
		PackageName:  entry.Name,
		WorkspaceDir: inst.WorkspaceDir,
		Passed:       true,
	}
	tmpl, check := validateFocusTemplateManifest(entry.Name, files[focusTemplateManifest])
	report.Checks = append(report.Checks, check)
	if check.Status == SandboxCheckFailed {
		report.Passed = false
		return InstalledFocusTemplate{}, report, &SandboxError{Report: report}
	}

	dir := inst.focusTemplateDir()
	originalID, err := inst.ownedFocusTemplateID(dir, tmpl.ID)
	if err != nil {
		return InstalledFocusTemplate{}, report, err
	}
	saved, err := focuspipeline.SaveTemplate(dir, originalID, tmpl)
	if err != nil {
		return InstalledFocusTemplate{}, report, fmt.Errorf("save focus template %q: %w", entry.Name, err)
	}
	row := InstalledFocusTemplate{
		Name:    entry.Name,
		Version: entry.Version,
		Source:  DefaultSourceID,
		File:    saved.Source,
	}
	written, err := os.ReadFile(inst.focusTemplatePath(row))
	if err != nil {
		return InstalledFocusTemplate{}, report, fmt.Errorf("read installed focus template %q: %w", entry.Name, err)
	}
	row.SHA256 = computeSHA256Hex(written)
	return row, report, nil
}

// ownedFocusTemplateID returns id when the user template of that id in dir
// is the one the hub installed (so SaveTemplate may replace it), "" when
// there is none, and an error when a template of that id exists that the
// user made themselves.
func (inst *Installer) ownedFocusTemplateID(dir, id string) (string, error) {
	templates, _ := focuspipeline.LoadTemplates(dir)
	var existing *focuspipeline.Template
	for i := range templates {
		if !templates[i].Builtin && templates[i].ID == id {
			existing = &templates[i]
			break
		}
	}
	if existing == nil {
		return "", nil
	}
	db, err := inst.loadDB()
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if db != nil {
		for _, row := range db.FocusTemplates {
			if row.Name == id && row.File == existing.Source {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("a local focus template %q already exists (%s/%s); rename or delete it before installing it from the hub",
		id, focuspipeline.TemplateDirName, existing.Source)
}

func validateFocusTemplateManifest(entryName string, raw []byte) (focuspipeline.Template, SandboxCheck) {
	fail := func(err string) (focuspipeline.Template, SandboxCheck) {
		return focuspipeline.Template{}, SandboxCheck{Name: "focus_template", Status: SandboxCheckFailed, Error: err}
	}
	if raw == nil {
		return fail(focusTemplateManifest + " is missing")
	}
	tmpl, err := focuspipeline.ParseTemplate(raw, ".yaml", entryName)
	if err != nil {
		return fail(err.Error())
	}
	if tmpl.ID != entryName {
		return fail(fmt.Sprintf("template id %q does not match registry name %q", tmpl.ID, entryName))
	}
	return tmpl, SandboxCheck{
		Name:   "focus_template",
		Status: SandboxCheckPassed,
		Output: fmt.Sprintf("%s parsed: %d stages", focusTemplateManifest, len(tmpl.Stages)),
	}
}

func (inst *Installer) focusTemplateDir() string {
	return filepath.Join(inst.WorkspaceDir, focuspipeline.TemplateDirName)
}

// focusTemplatePath is the installed file of row. File is recorded by the
// installer from SaveTemplate's result, always "<id>.yaml"; its base name
// is taken again so a hand-edited DB cannot point outside the folder.
func (inst *Installer) focusTemplatePath(row InstalledFocusTemplate) string {
	return filepath.Join(inst.focusTemplateDir(), filepath.Base(filepath.Clean("/"+row.File)))
}

func (inst *Installer) focusTemplateFileExists(row InstalledFocusTemplate) bool {
	if strings.TrimSpace(row.File) == "" {
		return false
	}
	info, err := os.Stat(inst.focusTemplatePath(row))
	return err == nil && !info.IsDir()
}

func (inst *Installer) addFocusTemplateToDB(row InstalledFocusTemplate) error {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			db = &InstalledDB{}
		} else {
			return err
		}
	}
	key := strings.ToLower(row.Name)
	for i, existing := range db.FocusTemplates {
		if strings.ToLower(existing.Name) == key {
			db.FocusTemplates[i] = row
			return inst.saveDB(db)
		}
	}
	db.FocusTemplates = append(db.FocusTemplates, row)
	return inst.saveDB(db)
}
