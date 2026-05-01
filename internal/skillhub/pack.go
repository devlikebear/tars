package skillhub

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	packActionInstall          = "install"
	packActionUpdate           = "update"
	packActionAlreadyInstalled = "already_installed"
)

// PlanPackInstall resolves a pack into the ordered packages that would be installed.
func (inst *Installer) PlanPackInstall(ctx context.Context, name string) (PackInstallPlan, error) {
	pack, err := inst.Registry.FindPackByName(ctx, name)
	if err != nil {
		return PackInstallPlan{}, err
	}

	db, err := inst.loadDB()
	if err != nil {
		if os.IsNotExist(err) {
			db = &InstalledDB{}
		} else {
			return PackInstallPlan{}, err
		}
	}

	plan := PackInstallPlan{Pack: *pack}
	seen := map[string]struct{}{}
	for _, dep := range pack.Plugins {
		entry, err := inst.Registry.FindPluginByName(ctx, dep)
		if err != nil {
			return PackInstallPlan{}, fmt.Errorf("pack %q references plugin %q: %w", pack.Name, dep, err)
		}
		currentVersion, installed := installedPluginVersion(db, entry.Name)
		item := PackInstallItem{
			Type:        "plugin",
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			Action:      packInstallAction(currentVersion, installed, entry.Version),
		}
		appendPackItem(&plan, seen, item)
	}
	for _, dep := range pack.MCPServers {
		entry, err := inst.Registry.FindMCPByName(ctx, dep)
		if err != nil {
			return PackInstallPlan{}, fmt.Errorf("pack %q references mcp server %q: %w", pack.Name, dep, err)
		}
		currentVersion, installed := installedMCPVersion(db, entry.Name)
		item := PackInstallItem{
			Type:        "mcp",
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			Action:      packInstallAction(currentVersion, installed, entry.Version),
		}
		appendPackItem(&plan, seen, item)
	}
	for _, dep := range pack.Skills {
		entry, err := inst.Registry.FindByName(ctx, dep)
		if err != nil {
			return PackInstallPlan{}, fmt.Errorf("pack %q references skill %q: %w", pack.Name, dep, err)
		}
		currentVersion, installed := installedSkillVersion(db, entry.Name)
		item := PackInstallItem{
			Type:        "skill",
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			Action:      packInstallAction(currentVersion, installed, entry.Version),
		}
		appendPackItem(&plan, seen, item)
	}
	if len(plan.Items) == 0 {
		return PackInstallPlan{}, fmt.Errorf("pack %q has no packages", pack.Name)
	}
	return plan, nil
}

// InstallPack installs every package in a pack using the standard sandboxed installers.
func (inst *Installer) InstallPack(ctx context.Context, name string) (*PackInstallResult, error) {
	plan, err := inst.PlanPackInstall(ctx, name)
	if err != nil {
		return nil, err
	}
	result := &PackInstallResult{Plan: plan}
	for _, item := range plan.Items {
		if item.Action == packActionAlreadyInstalled {
			result.Skipped = append(result.Skipped, item)
			continue
		}
		installResult, err := inst.installPackItem(ctx, item)
		if err != nil {
			return result, fmt.Errorf("install %s %q from pack %q: %w", item.Type, item.Name, plan.Pack.Name, err)
		}
		result.Installed = append(result.Installed, item)
		if installResult != nil {
			result.SandboxReports = append(result.SandboxReports, installResult.Sandbox)
		}
	}
	return result, nil
}

func (inst *Installer) installPackItem(ctx context.Context, item PackInstallItem) (*InstallResult, error) {
	switch item.Type {
	case "skill":
		return inst.Install(ctx, item.Name)
	case "plugin":
		return inst.InstallPlugin(ctx, item.Name)
	case "mcp":
		return inst.InstallMCP(ctx, item.Name)
	default:
		return nil, fmt.Errorf("unknown pack item type %q", item.Type)
	}
}

func appendPackItem(plan *PackInstallPlan, seen map[string]struct{}, item PackInstallItem) {
	key := strings.ToLower(strings.TrimSpace(item.Type)) + ":" + strings.ToLower(strings.TrimSpace(item.Name))
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	plan.Items = append(plan.Items, item)
}

func packInstallAction(currentVersion string, ok bool, nextVersion string) string {
	if !ok {
		return packActionInstall
	}
	if currentVersion == nextVersion {
		return packActionAlreadyInstalled
	}
	return packActionUpdate
}

func installedSkillVersion(db *InstalledDB, name string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, item := range db.Skills {
		if strings.ToLower(item.Name) == key {
			return item.Version, true
		}
	}
	return "", false
}

func installedPluginVersion(db *InstalledDB, name string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, item := range db.Plugins {
		if strings.ToLower(item.Name) == key {
			return item.Version, true
		}
	}
	return "", false
}

func installedMCPVersion(db *InstalledDB, name string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, item := range db.MCPs {
		if strings.ToLower(item.Name) == key {
			return item.Version, true
		}
	}
	return "", false
}
