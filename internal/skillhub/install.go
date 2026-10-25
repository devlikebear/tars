package skillhub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

const (
	hubSkillsDir    = "skills"
	hubPluginsDir   = "plugins"
	installedDBFile = "skillhub.json"
	skillManifest   = "SKILL.md"
	pluginManifest  = "tars.plugin.json"
)

// InstalledDB tracks installed hub skills, plugins, and MCP packages.
type InstalledDB struct {
	Skills  []InstalledSkill  `json:"skills"`
	Plugins []InstalledPlugin `json:"plugins,omitempty"`
	MCPs    []InstalledMCP    `json:"mcps,omitempty"`
	// FocusTemplates are hub focus templates (focus_template.go).
	FocusTemplates []InstalledFocusTemplate `json:"focus_templates,omitempty"`
}

// Installer handles installing and managing hub skills.
type Installer struct {
	WorkspaceDir string
	Registry     *Registry
	// Sources routes skill operations to the appropriate HubSource. The
	// built-in tars-hub source is registered by NewInstaller; external
	// hubs are added in later phases.
	Sources *SourceRegistry

	// dbMu serializes every read-modify-write of skillhub.json. The server
	// shares one Installer across requests, and two console installs (or
	// an install during an update) would otherwise drop each other's rows.
	dbMu sync.Mutex
}

// packageNamePattern is what a hub package name must look like before it
// is used as a directory or file name. External hubs take the name from
// upstream frontmatter, so "../x" must never reach filepath.Join.
var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validatePackageName(label, name string) error {
	if !packageNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("%s name %q is not a safe package name (letters, digits, '.', '_' or '-')", label, name)
	}
	return nil
}

// NewInstaller creates an installer for the given workspace. It registers
// the built-in tars-hub source so existing call sites and tests keep
// working without modification. External hubs (openclaw, hermes, anthropic)
// are wired by the caller — see cmd/tars and internal/tarsserver — so the
// skillhub package does not import its own subpackages.
func NewInstaller(workspaceDir string) *Installer {
	reg := NewRegistry()
	sources := NewSourceRegistry()
	_ = sources.Register(&TarsHubSource{Registry: reg})
	return &Installer{
		WorkspaceDir: workspaceDir,
		Registry:     reg,
		Sources:      sources,
	}
}

// InstallResult contains the sandboxed result of a hub package installation.
type InstallResult struct {
	RequiresPlugin string        `json:"requires_plugin,omitempty"` // non-empty if the skill depends on a plugin
	Sandbox        SandboxReport `json:"sandbox_report"`
	// DryRunPreview is populated when InstallOptions.DryRun was true. In
	// that case the workspace is untouched and Sandbox is the zero value.
	DryRunPreview *DryRunResult `json:"dry_run_preview,omitempty"`
}

// ConfirmFn lets the caller approve or reject an external-hub install
// after downloading + converting (but before materialize). Returning false
// without an error aborts the install cleanly. The preview is the same
// struct PreviewInstall and OnPreview surface, so the CLI can render once
// and decide once with the same data.
type ConfirmFn func(*DryRunResult) (bool, error)

// InstallOptions controls how (and whether) Installer.InstallWithOptions
// asks the caller for confirmation before materializing an external-hub
// skill. Empty InstallOptions preserves the legacy Install behaviour.
type InstallOptions struct {
	// Yes auto-approves the install (Confirm is ignored). Used by `--yes`.
	Yes bool
	// Confirm receives the dry-run preview when an external-hub skill is
	// about to be materialized. Called only for non-tars-hub sources. If
	// nil and Yes is false, InstallWithOptions errors out instead of
	// materializing.
	Confirm ConfirmFn
	// DryRun runs everything Install would do up to but not including
	// sandbox + materialize. The returned InstallResult has DryRunPreview
	// set and a zero Sandbox report; the workspace is left untouched.
	DryRun bool
	// OnPreview receives the DryRunResult once it is built, regardless of
	// whether DryRun is set. Used by the CLI to render the preview before
	// the confirmation prompt. Called exactly once per install attempt and
	// only for non-tars-hub sources (the built-in hub keeps the legacy
	// no-preview flow).
	OnPreview func(*DryRunResult)
}

// ErrInstallAborted is returned when ConfirmFn declines an install.
var ErrInstallAborted = fmt.Errorf("skillhub: install aborted by user")

// Install downloads and installs a skill. The ref may be a bare name (any
// registered source is searched) or a "<source>:<name>" pair (e.g. "openclaw:foo").
//
// This is the legacy entry point: external-hub installs without explicit
// confirmation use the auto-approve path (Yes: true) to preserve existing
// callers that have no way to surface a prompt. CLI install grows a real
// prompt via InstallWithOptions.
func (inst *Installer) Install(ctx context.Context, ref string) (*InstallResult, error) {
	return inst.InstallWithOptions(ctx, ref, InstallOptions{Yes: true})
}

// InstallWithOptions is the source-aware install entry point. The built-in
// tars-hub source keeps the legacy flow (no prompt, no preview); external
// hubs route through the converter / license fetcher, expose adapter
// warnings, and only materialize after explicit approval (Yes or Confirm).
//
// When opts.DryRun is true the function downloads + converts, surfaces the
// preview through OnPreview, and returns early without touching disk. This
// is the contract the CLI's `--dry-run` flag relies on.
func (inst *Installer) InstallWithOptions(ctx context.Context, ref string, opts InstallOptions) (*InstallResult, error) {
	sourceID, bareName := ResolveSkillRef(ref)
	src, entry, err := inst.resolveSkillSource(ctx, sourceID, bareName)
	if err != nil {
		return nil, err
	}

	// Built-in tars-hub bypass: no preview, no prompt. Preserves the
	// pre-federation flow that direct callers (cron, automated installs)
	// already depend on.
	if src.ID() == DefaultSourceID && !opts.DryRun {
		return inst.installTarsHub(ctx, src, entry)
	}

	preview, dl, err := inst.buildPreviewFromSource(ctx, ref, src, entry)
	if err != nil {
		return nil, err
	}
	if opts.OnPreview != nil {
		opts.OnPreview(preview)
	}
	if opts.DryRun {
		return &InstallResult{DryRunPreview: preview}, nil
	}

	if err := confirmExternalInstall(opts, preview); err != nil {
		return nil, err
	}

	// Materialize the bytes the preview was built from rather than
	// downloading again. A second download is not the content the user
	// approved: upstream may have moved, and converters stamp the import
	// time (imported_at, ATTRIBUTION.md) at second precision, so a
	// confirmation that took longer than a second never matched.
	result, err := inst.materializeSkill(ctx, src, entry, dl)
	if err != nil {
		return nil, err
	}
	result.DryRunPreview = preview
	return result, nil
}

// confirmExternalInstall returns nil when the install may proceed: --yes,
// or a Confirm callback that approved the preview. A declined preview is
// ErrInstallAborted.
func confirmExternalInstall(opts InstallOptions, preview *DryRunResult) error {
	if opts.Yes {
		return nil
	}
	if opts.Confirm == nil {
		return fmt.Errorf("skillhub: external-hub install requires confirmation; pass --yes, --dry-run, or supply a Confirm callback")
	}
	ok, err := opts.Confirm(preview)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInstallAborted
	}
	return nil
}

// installTarsHub is the original Install flow for the built-in source —
// no preview, no confirm, materialize directly. Kept separate so the
// external-hub path can stay focused.
func (inst *Installer) installTarsHub(ctx context.Context, src HubSource, entry *RegistryEntry) (*InstallResult, error) {
	dl, err := inst.downloadSkillFilesFromSource(ctx, src, entry)
	if err != nil {
		return nil, err
	}
	return inst.materializeSkill(ctx, src, entry, dl)
}

// materializeSkill runs the install sandbox over the downloaded files,
// writes them into the skill directory, and records the skill in the
// installed DB.
func (inst *Installer) materializeSkill(ctx context.Context, src HubSource, entry *RegistryEntry, dl *skillDownload) (*InstallResult, error) {
	if err := validatePackageName("skill", entry.Name); err != nil {
		return nil, err
	}
	sandboxReport, err := inst.runSkillInstallSandbox(ctx, entry, dl.files)
	if err != nil {
		return nil, err
	}
	skillDir := inst.skillDir(entry.Name)
	if err := materializePackageFiles(skillDir, dl.files); err != nil {
		return nil, err
	}
	if err := inst.addToDB(InstalledSkill{
		Name:     entry.Name,
		Version:  entry.Version,
		Source:   src.ID(),
		Dir:      skillDir,
		Path:     entry.Path,
		SHA256:   computeSHA256Hex(dl.files[skillManifest]),
		Upstream: dl.upstream,
	}); err != nil {
		return nil, err
	}
	result := &InstallResult{Sandbox: sandboxReport}
	if entry.RequiresPlugin != "" && !inst.isPluginInstalled(entry.RequiresPlugin) {
		result.RequiresPlugin = entry.RequiresPlugin
	}
	return result, nil
}

func sortedFilePaths(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for p := range files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ensureSources lazily initializes the source registry when an Installer was
// constructed without one (older tests, hand-built call sites). The default
// registry has just the built-in tars-hub source, backed by inst.Registry so
// the existing Registry pointer keeps controlling fetch URLs.
//
// If Sources already exists but its tars-hub source points at a stale
// Registry (legacy callers swap inst.Registry between calls), the entry is
// rebuilt so the source follows the current Registry pointer.
func (inst *Installer) ensureSources() *SourceRegistry {
	if inst.Sources != nil {
		inst.syncDefaultSource()
		return inst.Sources
	}
	reg := inst.Registry
	if reg == nil {
		reg = NewRegistry()
		inst.Registry = reg
	}
	sources := NewSourceRegistry()
	_ = sources.Register(&TarsHubSource{Registry: reg})
	inst.Sources = sources
	return sources
}

// syncDefaultSource refreshes the built-in tars-hub source to follow the
// current inst.Registry pointer. Legacy tests construct an Installer with
// Sources but then replace inst.Registry mid-test (e.g. swapping in a
// tampered httptest server); without this sync the tars-hub source would
// keep talking to the old server.
func (inst *Installer) syncDefaultSource() {
	if inst.Sources == nil || inst.Registry == nil {
		return
	}
	existing, ok := inst.Sources.Get(DefaultSourceID)
	if !ok {
		return
	}
	tarsHub, ok := existing.(*TarsHubSource)
	if !ok {
		return
	}
	if tarsHub.Registry == inst.Registry {
		return
	}
	tarsHub.Registry = inst.Registry
}

// resolveSkillSource picks the HubSource to use for a skill ref.
//
// If sourceID is non-empty, the source must be registered; otherwise an error
// is returned listing the known source IDs. With an empty sourceID, every
// registered source is queried by exact name and the single hit wins. Zero
// hits returns a not-found error; multiple hits return an ambiguity error
// that lists each source.
func (inst *Installer) resolveSkillSource(ctx context.Context, sourceID, bareName string) (HubSource, *RegistryEntry, error) {
	sources := inst.ensureSources()
	if sourceID != "" {
		return findSkillInSource(ctx, sources, sourceID, bareName)
	}
	return findSkillInAnySource(ctx, sources, bareName)
}

func findSkillInSource(ctx context.Context, sources *SourceRegistry, sourceID, bareName string) (HubSource, *RegistryEntry, error) {
	src, ok := sources.Get(sourceID)
	if !ok {
		return nil, nil, fmt.Errorf("hub source %q is not registered (known: %s)",
			sourceID, strings.Join(sources.IDs(), ", "))
	}
	entry, err := src.FindSkillByName(ctx, bareName)
	if err != nil {
		return nil, nil, err
	}
	return src, entry, nil
}

func findSkillInAnySource(ctx context.Context, sources *SourceRegistry, bareName string) (HubSource, *RegistryEntry, error) {
	type hit struct {
		src   HubSource
		entry *RegistryEntry
	}
	var hits []hit
	var firstErr error
	for _, src := range sources.List() {
		entry, err := src.FindSkillByName(ctx, bareName)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		hits = append(hits, hit{src: src, entry: entry})
	}
	switch len(hits) {
	case 0:
		if firstErr != nil {
			return nil, nil, firstErr
		}
		return nil, nil, fmt.Errorf("skill %q not found in any registered hub", bareName)
	case 1:
		return hits[0].src, hits[0].entry, nil
	default:
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.src.ID())
		}
		return nil, nil, fmt.Errorf("skill %q is ambiguous; available in: %s (use <source>:%s to pick one)",
			bareName, strings.Join(ids, ", "), bareName)
	}
}

// Uninstall removes an installed skill.
func (inst *Installer) Uninstall(name string) error {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	found := false
	var remaining []InstalledSkill
	for _, s := range db.Skills {
		if strings.ToLower(s.Name) == key {
			found = true
			// Keep the row when the files cannot be removed: a leftover
			// MCP directory would otherwise load as an unmanaged
			// workspace server and keep running.
			if err := os.RemoveAll(s.Dir); err != nil {
				return fmt.Errorf("remove %s: %w", s.Dir, err)
			}
			continue
		}
		remaining = append(remaining, s)
	}
	if !found {
		return fmt.Errorf("skill %q is not installed", name)
	}
	db.Skills = remaining
	return inst.saveDB(db)
}

// List returns the installed hub skills whose SKILL.md is still in the
// workspace. A skill directory deleted outside the hub drops out here, so
// the Hub tab offers to install it again instead of calling it installed.
func (inst *Installer) List() ([]InstalledSkill, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []InstalledSkill
	for _, row := range db.Skills {
		if skillFilesPresent(row) {
			out = append(out, row)
		}
	}
	return out, nil
}

func skillFilesPresent(row InstalledSkill) bool {
	info, err := os.Stat(filepath.Join(row.Dir, skillManifest))
	return err == nil && !info.IsDir()
}

// Update re-installs installed skills that have a newer version, routing
// each row to its recorded HubSource.
//
// External hubs publish no real versions, so their skills are downloaded
// and the source's own files compared with the checksum recorded at
// install (InstalledSkill.Upstream). A change is reported in Skipped, not
// applied: installing an external skill needs the user to review the
// converted files first (InstallOptions.Confirm), and an update has nobody
// to ask, so new upstream content waits for that same review through
// `tars skill install <source>:<name>`.
func (inst *Installer) Update(ctx context.Context) (UpdateResult, error) {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return UpdateResult{}, nil
		}
		return UpdateResult{}, err
	}
	sources := inst.ensureSources()
	var result UpdateResult
	for i, skill := range db.Skills {
		src, entry, skip := inst.resolveSkillUpdate(ctx, sources, skill)
		if skip != nil {
			result.Skipped = append(result.Skipped, *skip)
			continue
		}
		if reason := skillUpdateBlocker(skill); reason != "" {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: skill.Name, Reason: reason})
			continue
		}
		dl, err := inst.downloadSkillFilesFromSource(ctx, src, entry)
		if err != nil {
			// One broken package must not hold back the rest.
			result.Failed = append(result.Failed, UpdateDiagnostic{Name: skill.Name, Err: err})
			continue
		}
		if src.ID() != DefaultSourceID {
			result.Skipped = append(result.Skipped, externalSkillChange(skill, src.ID(), dl.upstream))
			continue
		}
		if err := inst.reinstallSkill(ctx, entry, skill.Dir, dl); err != nil {
			result.Failed = append(result.Failed, UpdateDiagnostic{Name: skill.Name, Err: err})
			continue
		}
		db.Skills[i].Version = entry.Version
		db.Skills[i].Path = entry.Path
		db.Skills[i].SHA256 = computeSHA256Hex(dl.files[skillManifest])
		result.Updated = append(result.Updated, skill.Name)
	}
	return result, errors.Join(updateFailuresError("skills", result.Failed), inst.saveUpdatedDB(db, result, "skills"))
}

// resolveSkillUpdate finds the source and current registry entry for an
// installed skill. A non-nil diagnostic means the skill is skipped: its
// source is gone, the lookup failed, or a tars-hub skill is up to date.
func (inst *Installer) resolveSkillUpdate(ctx context.Context, sources *SourceRegistry, skill InstalledSkill) (HubSource, *RegistryEntry, *UpdateDiagnostic) {
	sourceID := strings.TrimSpace(skill.Source)
	if sourceID == "" {
		sourceID = DefaultSourceID
	}
	src, ok := sources.Get(sourceID)
	if !ok {
		return nil, nil, &UpdateDiagnostic{
			Name:   skill.Name,
			Reason: fmt.Sprintf("source %q is no longer registered", sourceID),
		}
	}
	entry, err := findInstalledSkill(ctx, src, skill)
	if err != nil {
		return nil, nil, &UpdateDiagnostic{Name: skill.Name, Err: err}
	}
	if sourceID == DefaultSourceID && entry.Version == skill.Version {
		return nil, nil, &UpdateDiagnostic{Name: skill.Name, Reason: reasonUpToDate}
	}
	return src, entry, nil
}

// findInstalledSkill looks an installed skill up in its source: by the
// recorded directory when the source supports it, else by name. A skill
// whose directory now holds a skill of another name is an error, not an
// update — it would install under the old name with another skill's body.
func findInstalledSkill(ctx context.Context, src HubSource, skill InstalledSkill) (*RegistryEntry, error) {
	finder, ok := src.(SkillPathFinder)
	if !ok || strings.TrimSpace(skill.Path) == "" {
		return src.FindSkillByName(ctx, skill.Name)
	}
	entry, err := finder.FindSkillByPath(ctx, skill.Path)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(entry.Name, skill.Name) {
		return nil, fmt.Errorf("%s now holds skill %q, not %q; uninstall and install again", skill.Path, entry.Name, skill.Name)
	}
	return entry, nil
}

// skillUpdateBlocker says why an installed skill must not be replaced by
// an update, or "" when it may be. Rows written before the checksum was
// recorded have no SHA256 and count as unedited.
func skillUpdateBlocker(row InstalledSkill) string {
	raw, err := os.ReadFile(filepath.Join(row.Dir, skillManifest))
	if os.IsNotExist(err) {
		return "skill files were removed; install it again to restore it"
	}
	if err != nil {
		return "SKILL.md cannot be read: " + err.Error()
	}
	if row.SHA256 != "" && computeSHA256Hex(raw) != row.SHA256 {
		return "edited locally; uninstall and install again to replace your changes"
	}
	return ""
}

// externalSkillChange reports how an external skill's freshly downloaded
// files compare with the ones it was installed from (see Update).
func externalSkillChange(row InstalledSkill, sourceID, upstream string) UpdateDiagnostic {
	reinstall := fmt.Sprintf("tars skill install %s:%s", sourceID, row.Name)
	switch row.Upstream {
	case upstream:
		return UpdateDiagnostic{Name: row.Name, Reason: reasonUpToDate}
	case "":
		return UpdateDiagnostic{Name: row.Name, Reason: "installed before upstream changes were tracked; review it with `" + reinstall + "` to track updates"}
	default:
		return UpdateDiagnostic{Name: row.Name, Reason: "upstream changed; review and apply it with `" + reinstall + "`"}
	}
}

// reinstallSkill runs the install sandbox over a downloaded skill and
// overwrites the files in dir.
func (inst *Installer) reinstallSkill(ctx context.Context, entry *RegistryEntry, dir string, dl *skillDownload) error {
	if _, err := inst.runSkillInstallSandbox(ctx, entry, dl.files); err != nil {
		return err
	}
	return materializePackageFiles(dir, dl.files)
}

func (inst *Installer) skillDir(name string) string {
	return filepath.Join(inst.WorkspaceDir, hubSkillsDir, name)
}

func (inst *Installer) dbPath() string {
	return filepath.Join(inst.WorkspaceDir, installedDBFile)
}

func (inst *Installer) loadDB() (*InstalledDB, error) {
	data, err := os.ReadFile(inst.dbPath())
	if err != nil {
		return nil, err
	}
	var db InstalledDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parse %s: %w", installedDBFile, err)
	}
	// Backfill legacy rows that were written before the HubSource refactor:
	// an empty Source means the row came from the built-in tars-hub.
	for i := range db.Skills {
		if strings.TrimSpace(db.Skills[i].Source) == "" {
			db.Skills[i].Source = DefaultSourceID
		}
	}
	for i := range db.Plugins {
		if strings.TrimSpace(db.Plugins[i].Source) == "" {
			db.Plugins[i].Source = DefaultSourceID
		}
	}
	for i := range db.MCPs {
		if strings.TrimSpace(db.MCPs[i].Source) == "" {
			db.MCPs[i].Source = DefaultSourceID
		}
	}
	for i := range db.FocusTemplates {
		if strings.TrimSpace(db.FocusTemplates[i].Source) == "" {
			db.FocusTemplates[i].Source = DefaultSourceID
		}
	}
	return &db, nil
}

func (inst *Installer) saveDB(db *InstalledDB) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Write(inst.dbPath(), append(data, '\n'))
}

func (inst *Installer) saveUpdatedDB(db *InstalledDB, result UpdateResult, resource string) error {
	if len(result.Updated) == 0 {
		return nil
	}
	if err := inst.saveDB(db); err != nil {
		return fmt.Errorf("save installed %s after update: %w", resource, err)
	}
	return nil
}

func updateFailuresError(resource string, failed []UpdateDiagnostic) error {
	if len(failed) == 0 {
		return nil
	}
	parts := make([]string, 0, len(failed))
	for _, failure := range failed {
		detail := strings.TrimSpace(failure.Detail())
		if detail == "" {
			detail = "unknown error"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", failure.Name, detail))
	}
	return fmt.Errorf("update %s incomplete: %s", resource, strings.Join(parts, "; "))
}

func (inst *Installer) addToDB(skill InstalledSkill) error {
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
	key := strings.ToLower(skill.Name)
	for i, s := range db.Skills {
		if strings.ToLower(s.Name) == key {
			db.Skills[i] = skill
			return inst.saveDB(db)
		}
	}
	db.Skills = append(db.Skills, skill)
	return inst.saveDB(db)
}

// --- Plugin operations ---

// InstallPlugin downloads and installs a plugin from the registry.
func (inst *Installer) InstallPlugin(ctx context.Context, name string) (*InstallResult, error) {
	entry, err := inst.Registry.FindPluginByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := validatePackageName("plugin", entry.Name); err != nil {
		return nil, err
	}

	files, err := inst.downloadPluginFiles(ctx, entry)
	if err != nil {
		return nil, err
	}
	sandboxReport, err := inst.runPluginInstallSandbox(ctx, entry, files)
	if err != nil {
		return nil, err
	}
	pluginDir := inst.pluginDir(entry.Name)
	if err := materializePackageFiles(pluginDir, files); err != nil {
		return nil, err
	}

	if err := inst.addPluginToDB(InstalledPlugin{
		Name:    entry.Name,
		Version: entry.Version,
		Source:  "tars-hub",
		Dir:     pluginDir,
	}); err != nil {
		return nil, err
	}
	return &InstallResult{Sandbox: sandboxReport}, nil
}

// UninstallPlugin removes an installed plugin.
func (inst *Installer) UninstallPlugin(name string) error {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	found := false
	var remaining []InstalledPlugin
	for _, p := range db.Plugins {
		if strings.ToLower(p.Name) == key {
			found = true
			// Keep the row when the files cannot be removed: a leftover
			// MCP directory would otherwise load as an unmanaged
			// workspace server and keep running.
			if err := os.RemoveAll(p.Dir); err != nil {
				return fmt.Errorf("remove %s: %w", p.Dir, err)
			}
			continue
		}
		remaining = append(remaining, p)
	}
	if !found {
		return fmt.Errorf("plugin %q is not installed", name)
	}
	db.Plugins = remaining
	return inst.saveDB(db)
}

// ListPlugins returns all installed hub plugins.
func (inst *Installer) ListPlugins() ([]InstalledPlugin, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return db.Plugins, nil
}

// UpdatePlugins re-installs all installed plugins with the latest version.
func (inst *Installer) UpdatePlugins(ctx context.Context) (UpdateResult, error) {
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
	for i, plugin := range db.Plugins {
		entry, err := inst.Registry.FindPluginByName(ctx, plugin.Name)
		if err != nil {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: plugin.Name, Err: err})
			continue
		}
		if entry.Version == plugin.Version {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: plugin.Name, Reason: reasonUpToDate})
			continue
		}
		if err := inst.reinstallPlugin(ctx, entry, plugin.Dir); err != nil {
			result.Failed = append(result.Failed, UpdateDiagnostic{Name: plugin.Name, Err: err})
			continue
		}
		db.Plugins[i].Version = entry.Version
		result.Updated = append(result.Updated, plugin.Name)
	}
	return result, errors.Join(updateFailuresError("plugins", result.Failed), inst.saveUpdatedDB(db, result, "plugins"))
}

// reinstallPlugin downloads the plugin again, runs the install sandbox, and
// overwrites the files in dir.
func (inst *Installer) reinstallPlugin(ctx context.Context, entry *PluginEntry, dir string) error {
	files, err := inst.downloadPluginFiles(ctx, entry)
	if err != nil {
		return err
	}
	if _, err := inst.runPluginInstallSandbox(ctx, entry, files); err != nil {
		return err
	}
	return materializePackageFiles(dir, files)
}

func (inst *Installer) pluginDir(name string) string {
	return filepath.Join(inst.WorkspaceDir, hubPluginsDir, name)
}

func (inst *Installer) mcpDir(name string) string {
	return filepath.Join(inst.WorkspaceDir, hubMCPDir, name)
}

func (inst *Installer) isPluginInstalled(name string) bool {
	db, err := inst.loadDB()
	if err != nil {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	for _, p := range db.Plugins {
		if strings.ToLower(p.Name) == key {
			return true
		}
	}
	// Also check if the plugin directory exists in workspace/plugins (bundled plugins).
	pluginManifest := filepath.Join(inst.WorkspaceDir, hubPluginsDir, name, "tars.plugin.json")
	if _, err := os.Stat(pluginManifest); err == nil {
		return true
	}
	return false
}

func (inst *Installer) addPluginToDB(plugin InstalledPlugin) error {
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
	key := strings.ToLower(plugin.Name)
	for i, p := range db.Plugins {
		if strings.ToLower(p.Name) == key {
			db.Plugins[i] = plugin
			return inst.saveDB(db)
		}
	}
	db.Plugins = append(db.Plugins, plugin)
	return inst.saveDB(db)
}

func (inst *Installer) downloadSkillFiles(ctx context.Context, entry *RegistryEntry) (map[string][]byte, error) {
	if len(entry.Files) == 0 {
		content, err := inst.Registry.FetchFile(ctx, entry, skillManifest)
		if err != nil {
			return nil, fmt.Errorf("fetch legacy skill manifest for %q: %w", entry.Name, err)
		}
		return map[string][]byte{skillManifest: content}, nil
	}
	return inst.downloadVerifiedHubFiles(entry.Name, "skill", entry.Files, skillManifest, func(relPath string) ([]byte, error) {
		return inst.Registry.FetchFile(ctx, entry, relPath)
	})
}

// skillDownload is a downloaded skill, ready to materialize.
type skillDownload struct {
	// files maps relative paths to the bytes to write.
	files map[string][]byte
	// warnings are the adapter's notes from conversion (e.g. install
	// blocks skipped).
	warnings []string
	// upstream is the checksum of an external source's own files —
	// SKILL.md before conversion plus companions — and "" for tars-hub.
	// Converters stamp the import time into SKILL.md and ATTRIBUTION.md,
	// so the written files differ on every download and cannot tell
	// whether upstream changed.
	upstream string
}

// downloadSkillFilesFromSource routes skill download through the appropriate
// HubSource. Built-in tars-hub keeps the existing manifest-with-sha256 path;
// external hubs (CompanionFileLister implementers) discover files at fetch
// time and optionally apply a content converter and an ATTRIBUTION.md.
func (inst *Installer) downloadSkillFilesFromSource(ctx context.Context, src HubSource, entry *RegistryEntry) (*skillDownload, error) {
	if src == nil {
		return nil, fmt.Errorf("downloadSkillFilesFromSource: source is nil")
	}
	// tars-hub keeps the original path so existing manifests with sha256
	// verification stay in effect.
	if src.ID() == DefaultSourceID {
		files, err := inst.downloadSkillFiles(ctx, entry)
		if err != nil {
			return nil, err
		}
		return &skillDownload{files: files}, nil
	}
	return inst.downloadExternalSkillFiles(ctx, src, entry)
}

// downloadExternalSkillFiles handles the "no pre-declared manifest" path:
// fetch SKILL.md, optionally convert frontmatter, discover companion files,
// fetch each, and emit ATTRIBUTION.md from the source's LicenseFetcher.
func (inst *Installer) downloadExternalSkillFiles(ctx context.Context, src HubSource, entry *RegistryEntry) (*skillDownload, error) {
	rawManifest, err := src.FetchSkillContent(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("fetch %s SKILL.md for %q: %w", src.ID(), entry.Name, err)
	}
	manifest, warnings, err := convertExternalManifest(src, entry, rawManifest)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, 4)
	files[skillManifest] = rawManifest
	if err := fetchCompanionFiles(ctx, src, entry, files); err != nil {
		return nil, err
	}
	upstream := filesDigest(files)
	files[skillManifest] = manifest

	attribution, err := buildExternalAttribution(ctx, src, entry)
	if err != nil {
		return nil, err
	}
	if attribution != nil {
		files[AttributionFilename] = attribution
	}
	return &skillDownload{files: files, warnings: warnings, upstream: upstream}, nil
}

// filesDigest is a sha256 over every path and its bytes, in path order.
func filesDigest(files map[string][]byte) string {
	h := sha256.New()
	for _, p := range sortedFilePaths(files) {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", p, len(files[p]))
		_, _ = h.Write(files[p])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// convertExternalManifest rewrites SKILL.md into TARS frontmatter when the
// source has a converter, and passes it through unchanged otherwise.
func convertExternalManifest(src HubSource, entry *RegistryEntry, raw []byte) ([]byte, []string, error) {
	converter, ok := src.(SkillContentConverter)
	if !ok {
		return raw, nil, nil
	}
	converted, warnings, err := converter.ConvertSkillContent(entry, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("convert %s SKILL.md for %q: %w", src.ID(), entry.Name, err)
	}
	return converted, warnings, nil
}

// fetchCompanionFiles adds every companion file the source lists (other
// than SKILL.md itself) to files.
func fetchCompanionFiles(ctx context.Context, src HubSource, entry *RegistryEntry, files map[string][]byte) error {
	lister, ok := src.(CompanionFileLister)
	if !ok {
		return nil
	}
	paths, err := lister.ListCompanionFiles(ctx, entry)
	if err != nil {
		return fmt.Errorf("list %s companion files for %q: %w", src.ID(), entry.Name, err)
	}
	for _, rel := range paths {
		rel = strings.TrimSpace(rel)
		if rel == "" || rel == skillManifest {
			continue
		}
		body, err := src.FetchSkillFile(ctx, entry, rel)
		if err != nil {
			return fmt.Errorf("fetch %s companion %q for %q: %w", src.ID(), rel, entry.Name, err)
		}
		files[rel] = body
	}
	return nil
}

// buildExternalAttribution renders ATTRIBUTION.md from the source's
// license. It returns nil when the source has no LicenseFetcher.
func buildExternalAttribution(ctx context.Context, src HubSource, entry *RegistryEntry) ([]byte, error) {
	licenser, ok := src.(LicenseFetcher)
	if !ok {
		return nil, nil
	}
	body, label, err := licenser.FetchLicense(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("fetch %s license for %q: %w", src.ID(), entry.Name, err)
	}
	attribution, err := BuildAttribution(AttributionInput{
		SourceID:       src.ID(),
		OriginalName:   entry.Name,
		OriginalURL:    entry.Path,
		OriginalAuthor: entry.Author,
		LicenseLabel:   label,
		LicenseBody:    body,
	})
	if err != nil {
		return nil, fmt.Errorf("build %s attribution for %q: %w", src.ID(), entry.Name, err)
	}
	return attribution, nil
}

func (inst *Installer) downloadPluginFiles(ctx context.Context, entry *PluginEntry) (map[string][]byte, error) {
	return inst.downloadVerifiedHubFiles(entry.Name, "plugin", entry.Files, pluginManifest, func(relPath string) ([]byte, error) {
		return inst.Registry.FetchPluginFile(ctx, entry, relPath)
	})
}

func (inst *Installer) downloadVerifiedHubFiles(
	name string,
	label string,
	files RegistryFiles,
	requiredPath string,
	fetch func(relPath string) ([]byte, error),
) (map[string][]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("%s %q has no downloadable files", label, name)
	}
	downloaded := make(map[string][]byte, len(files))
	requiredFound := false
	for _, file := range files {
		relPath, err := cleanRegistryRelativePath(file.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid file path for %s %q: %w", label, name, err)
		}
		if strings.TrimSpace(file.SHA256) == "" {
			return nil, fmt.Errorf("%s %q file %q is missing sha256 checksum", label, name, relPath)
		}
		content, err := fetch(relPath)
		if err != nil {
			return nil, err
		}
		if err := verifyFileChecksum(content, file.SHA256); err != nil {
			return nil, fmt.Errorf("verify %q for %s %q: %w", relPath, label, name, err)
		}
		downloaded[relPath] = content
		if relPath == requiredPath {
			requiredFound = true
		}
	}
	if requiredPath != "" && !requiredFound {
		return nil, fmt.Errorf("%s %q manifest %q is not declared in registry files", label, name, requiredPath)
	}
	return downloaded, nil
}

// --- MCP operations ---

// InstallMCP downloads and installs an MCP package from the registry.
func (inst *Installer) InstallMCP(ctx context.Context, name string) (*InstallResult, error) {
	entry, err := inst.Registry.FindMCPByName(ctx, name)
	if err != nil {
		return nil, err
	}
	installed, sandboxReport, err := inst.installMCPEntry(ctx, entry)
	if err != nil {
		return nil, err
	}
	if err := inst.addMCPToDB(installed); err != nil {
		return nil, err
	}
	return &InstallResult{Sandbox: sandboxReport}, nil
}

// UninstallMCP removes an installed MCP package.
func (inst *Installer) UninstallMCP(name string) error {
	inst.dbMu.Lock()
	defer inst.dbMu.Unlock()
	db, err := inst.loadDB()
	if err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	found := false
	var remaining []InstalledMCP
	for _, mcp := range db.MCPs {
		if strings.ToLower(mcp.Name) == key {
			found = true
			// Keep the row when the files cannot be removed: a leftover
			// MCP directory would otherwise load as an unmanaged
			// workspace server and keep running.
			if err := os.RemoveAll(mcp.Dir); err != nil {
				return fmt.Errorf("remove %s: %w", mcp.Dir, err)
			}
			continue
		}
		remaining = append(remaining, mcp)
	}
	if !found {
		return fmt.Errorf("mcp server %q is not installed", name)
	}
	db.MCPs = remaining
	return inst.saveDB(db)
}

// ListMCPs returns all installed hub MCP packages.
func (inst *Installer) ListMCPs() ([]InstalledMCP, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return db.MCPs, nil
}

// UpdateMCPs re-installs all installed MCP packages with the latest version.
func (inst *Installer) UpdateMCPs(ctx context.Context) (UpdateResult, error) {
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
	for i, installed := range db.MCPs {
		entry, err := inst.Registry.FindMCPByName(ctx, installed.Name)
		if err != nil {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: installed.Name, Err: err})
			continue
		}
		if entry.Version == installed.Version {
			result.Skipped = append(result.Skipped, UpdateDiagnostic{Name: installed.Name, Reason: reasonUpToDate})
			continue
		}
		nextInstalled, _, err := inst.installMCPEntry(ctx, entry)
		if err != nil {
			result.Failed = append(result.Failed, UpdateDiagnostic{Name: installed.Name, Err: err})
			continue
		}
		db.MCPs[i] = nextInstalled
		result.Updated = append(result.Updated, installed.Name)
	}
	return result, errors.Join(updateFailuresError("MCP servers", result.Failed), inst.saveUpdatedDB(db, result, "MCP servers"))
}

func (inst *Installer) installMCPEntry(ctx context.Context, entry *MCPEntry) (InstalledMCP, SandboxReport, error) {
	if err := validatePackageName("mcp server", entry.Name); err != nil {
		return InstalledMCP{}, SandboxReport{}, err
	}
	manifestPath := strings.TrimSpace(entry.Manifest)
	if manifestPath == "" {
		manifestPath = defaultMCPManifest
	}
	cleanManifestPath, err := cleanRegistryRelativePath(manifestPath)
	if err != nil {
		return InstalledMCP{}, SandboxReport{}, fmt.Errorf("invalid manifest path for mcp server %q: %w", entry.Name, err)
	}
	files, err := inst.downloadMCPFiles(ctx, entry, cleanManifestPath)
	if err != nil {
		return InstalledMCP{}, SandboxReport{}, err
	}
	if _, ok := files[cleanManifestPath]; !ok {
		return InstalledMCP{}, SandboxReport{}, fmt.Errorf("mcp server %q manifest %q is missing", entry.Name, cleanManifestPath)
	}
	sandboxReport, err := inst.runMCPInstallSandbox(ctx, entry, files, cleanManifestPath)
	if err != nil {
		return InstalledMCP{}, SandboxReport{}, err
	}

	mcpDir := inst.mcpDir(entry.Name)
	if err := materializePackageFiles(mcpDir, files); err != nil {
		return InstalledMCP{}, SandboxReport{}, err
	}
	return InstalledMCP{
		Name:     entry.Name,
		Version:  entry.Version,
		Source:   "tars-hub",
		Dir:      mcpDir,
		Manifest: cleanManifestPath,
	}, sandboxReport, nil
}

func (inst *Installer) downloadMCPFiles(ctx context.Context, entry *MCPEntry, manifestPath string) (map[string][]byte, error) {
	if len(entry.Files) == 0 {
		return nil, fmt.Errorf("mcp server %q has no downloadable files", entry.Name)
	}
	files := make(map[string][]byte, len(entry.Files))
	manifestFound := false
	for _, file := range entry.Files {
		relPath, err := cleanRegistryRelativePath(file.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid file path for mcp server %q: %w", entry.Name, err)
		}
		content, err := inst.Registry.FetchMCPFile(ctx, entry, relPath)
		if err != nil {
			return nil, err
		}
		if err := verifyFileChecksum(content, file.SHA256); err != nil {
			return nil, fmt.Errorf("verify %q for mcp server %q: %w", relPath, entry.Name, err)
		}
		files[relPath] = content
		if relPath == manifestPath {
			manifestFound = true
		}
	}
	if !manifestFound {
		return nil, fmt.Errorf("mcp server %q manifest %q is not declared in registry files", entry.Name, manifestPath)
	}
	return files, nil
}

func (inst *Installer) addMCPToDB(mcp InstalledMCP) error {
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
	key := strings.ToLower(mcp.Name)
	for i, existing := range db.MCPs {
		if strings.ToLower(existing.Name) == key {
			db.MCPs[i] = mcp
			return inst.saveDB(db)
		}
	}
	db.MCPs = append(db.MCPs, mcp)
	return inst.saveDB(db)
}
