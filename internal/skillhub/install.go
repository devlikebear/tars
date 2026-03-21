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
	installedDBFile = "skillhub.json"
)

// InstalledDB tracks installed hub skills.
type InstalledDB struct {
	Skills []InstalledSkill `json:"skills"`
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

// Install downloads and installs a skill from the registry.
func (inst *Installer) Install(ctx context.Context, name string) error {
	entry, err := inst.Registry.FindByName(ctx, name)
	if err != nil {
		return err
	}

	content, err := inst.Registry.FetchSkillContent(ctx, entry)
	if err != nil {
		return err
	}

	skillDir := inst.skillDir(entry.Name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, content, 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
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

	return inst.addToDB(InstalledSkill{
		Name:    entry.Name,
		Version: entry.Version,
		Source:  "tars-hub",
		Dir:     skillDir,
	})
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
