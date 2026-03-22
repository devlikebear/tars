package skillhub

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	hubSkillsDir    = "skills"
	hubPluginsDir   = "plugins"
	installedDBFile = "skillhub.json"
)

// InstalledDB tracks installed hub skills and plugins.
type InstalledDB struct {
	Skills  []InstalledSkill  `json:"skills"`
	Plugins []InstalledPlugin `json:"plugins,omitempty"`
	MCPs    []InstalledMCP    `json:"mcps,omitempty"`
}

// Installer handles installing and managing hub skills.
type Installer struct {
	WorkspaceDir string
	Registry     *Registry
}

// NewInstaller creates an installer for the given workspace.
func NewInstaller(workspaceDir string) *Installer {
	return &Installer{
		WorkspaceDir: workspaceDir,
		Registry:     NewRegistry(),
	}
}

// InstallResult contains the result of a skill installation.
type InstallResult struct {
	RequiresPlugin string // non-empty if the skill depends on a plugin
}

// Install downloads and installs a skill from the registry.
func (inst *Installer) Install(ctx context.Context, name string) (*InstallResult, error) {
	entry, err := inst.Registry.FindByName(ctx, name)
	if err != nil {
		return nil, err
	}

	content, err := inst.Registry.FetchSkillContent(ctx, entry)
	if err != nil {
		return nil, err
	}

	skillDir := inst.skillDir(entry.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill dir: %w", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, content, 0o644); err != nil {
		return nil, fmt.Errorf("write skill file: %w", err)
	}

	// Download companion files listed in the registry entry.
	for _, relPath := range entry.Files {
		fileContent, err := inst.Registry.FetchFile(ctx, entry, relPath)
		if err != nil {
			continue // best-effort: skip files that fail to download
		}
		dst := filepath.Join(skillDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(dst, fileContent, 0o644); err != nil {
			continue
		}
	}

	if err := inst.addToDB(InstalledSkill{
		Name:    entry.Name,
		Version: entry.Version,
		Source:  "tars-hub",
		Dir:     skillDir,
	}); err != nil {
		return nil, err
	}

	result := &InstallResult{}
	if entry.RequiresPlugin != "" {
		// Check if the required plugin is already installed.
		if !inst.isPluginInstalled(entry.RequiresPlugin) {
			result.RequiresPlugin = entry.RequiresPlugin
		}
	}
	return result, nil
}

// Uninstall removes an installed skill.
func (inst *Installer) Uninstall(name string) error {
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
			_ = os.RemoveAll(s.Dir)
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

// List returns all installed hub skills.
func (inst *Installer) List() ([]InstalledSkill, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return db.Skills, nil
}

// Update re-installs all installed skills with the latest version.
func (inst *Installer) Update(ctx context.Context) ([]string, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var updated []string
	for i, skill := range db.Skills {
		entry, err := inst.Registry.FindByName(ctx, skill.Name)
		if err != nil {
			continue
		}
		if entry.Version == skill.Version {
			continue
		}
		content, err := inst.Registry.FetchSkillContent(ctx, entry)
		if err != nil {
			continue
		}
		skillFile := filepath.Join(skill.Dir, "SKILL.md")
		if err := os.WriteFile(skillFile, content, 0o644); err != nil {
			continue
		}
		for _, relPath := range entry.Files {
			fileContent, fetchErr := inst.Registry.FetchFile(ctx, entry, relPath)
			if fetchErr != nil {
				continue
			}
			dst := filepath.Join(skill.Dir, filepath.FromSlash(relPath))
			_ = os.MkdirAll(filepath.Dir(dst), 0o755)
			_ = os.WriteFile(dst, fileContent, 0o644)
		}
		db.Skills[i].Version = entry.Version
		updated = append(updated, skill.Name)
	}
	if len(updated) > 0 {
		_ = inst.saveDB(db)
	}
	return updated, nil
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
	return &db, nil
}

func (inst *Installer) saveDB(db *InstalledDB) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(inst.dbPath(), append(data, '\n'), 0o644)
}

func (inst *Installer) addToDB(skill InstalledSkill) error {
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
func (inst *Installer) InstallPlugin(ctx context.Context, name string) error {
	entry, err := inst.Registry.FindPluginByName(ctx, name)
	if err != nil {
		return err
	}

	pluginDir := inst.pluginDir(entry.Name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return fmt.Errorf("create plugin dir: %w", err)
	}

	// Download all declared files.
	for _, relPath := range entry.Files {
		content, err := inst.Registry.FetchPluginFile(ctx, entry, relPath)
		if err != nil {
			continue // best-effort
		}
		dst := filepath.Join(pluginDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(dst, content, 0o644); err != nil {
			continue
		}
	}

	return inst.addPluginToDB(InstalledPlugin{
		Name:    entry.Name,
		Version: entry.Version,
		Source:  "tars-hub",
		Dir:     pluginDir,
	})
}

// UninstallPlugin removes an installed plugin.
func (inst *Installer) UninstallPlugin(name string) error {
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
			_ = os.RemoveAll(p.Dir)
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
func (inst *Installer) UpdatePlugins(ctx context.Context) ([]string, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var updated []string
	for i, plugin := range db.Plugins {
		entry, err := inst.Registry.FindPluginByName(ctx, plugin.Name)
		if err != nil {
			continue
		}
		if entry.Version == plugin.Version {
			continue
		}
		for _, relPath := range entry.Files {
			content, fetchErr := inst.Registry.FetchPluginFile(ctx, entry, relPath)
			if fetchErr != nil {
				continue
			}
			dst := filepath.Join(plugin.Dir, filepath.FromSlash(relPath))
			_ = os.MkdirAll(filepath.Dir(dst), 0o755)
			_ = os.WriteFile(dst, content, 0o644)
		}
		db.Plugins[i].Version = entry.Version
		updated = append(updated, plugin.Name)
	}
	if len(updated) > 0 {
		_ = inst.saveDB(db)
	}
	return updated, nil
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

// --- MCP operations ---

// InstallMCP downloads and installs an MCP package from the registry.
func (inst *Installer) InstallMCP(ctx context.Context, name string) error {
	entry, err := inst.Registry.FindMCPByName(ctx, name)
	if err != nil {
		return err
	}
	installed, err := inst.installMCPEntry(ctx, entry)
	if err != nil {
		return err
	}
	return inst.addMCPToDB(installed)
}

// UninstallMCP removes an installed MCP package.
func (inst *Installer) UninstallMCP(name string) error {
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
			_ = os.RemoveAll(mcp.Dir)
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
func (inst *Installer) UpdateMCPs(ctx context.Context) ([]string, error) {
	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var updated []string
	for i, installed := range db.MCPs {
		entry, err := inst.Registry.FindMCPByName(ctx, installed.Name)
		if err != nil {
			continue
		}
		if entry.Version == installed.Version {
			continue
		}
		nextInstalled, err := inst.installMCPEntry(ctx, entry)
		if err != nil {
			continue
		}
		db.MCPs[i] = nextInstalled
		updated = append(updated, installed.Name)
	}
	if len(updated) > 0 {
		_ = inst.saveDB(db)
	}
	return updated, nil
}

func (inst *Installer) installMCPEntry(ctx context.Context, entry *MCPEntry) (InstalledMCP, error) {
	manifestPath := strings.TrimSpace(entry.Manifest)
	if manifestPath == "" {
		manifestPath = defaultMCPManifest
	}
	cleanManifestPath, err := cleanRegistryRelativePath(manifestPath)
	if err != nil {
		return InstalledMCP{}, fmt.Errorf("invalid manifest path for mcp server %q: %w", entry.Name, err)
	}
	files, err := inst.downloadMCPFiles(ctx, entry, cleanManifestPath)
	if err != nil {
		return InstalledMCP{}, err
	}
	manifestData, ok := files[cleanManifestPath]
	if !ok {
		return InstalledMCP{}, fmt.Errorf("mcp server %q manifest %q is missing", entry.Name, cleanManifestPath)
	}
	if _, err := parseMCPManifest(manifestData, entry.Name); err != nil {
		return InstalledMCP{}, err
	}

	mcpDir := inst.mcpDir(entry.Name)
	if err := materializePackageFiles(mcpDir, files); err != nil {
		return InstalledMCP{}, err
	}
	return InstalledMCP{
		Name:     entry.Name,
		Version:  entry.Version,
		Source:   "tars-hub",
		Dir:      mcpDir,
		Manifest: cleanManifestPath,
	}, nil
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
