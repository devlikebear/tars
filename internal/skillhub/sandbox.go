package skillhub

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/skill"
)

const defaultSkillSmokeTimeout = 30 * time.Second

type SandboxCheckStatus string

const (
	SandboxCheckPassed SandboxCheckStatus = "passed"
	SandboxCheckFailed SandboxCheckStatus = "failed"
)

// SandboxCheck captures one validation step from a skill install sandbox.
type SandboxCheck struct {
	Name       string             `json:"name"`
	Command    string             `json:"command,omitempty"`
	Status     SandboxCheckStatus `json:"status"`
	Output     string             `json:"output,omitempty"`
	Error      string             `json:"error,omitempty"`
	DurationMS int64              `json:"duration_ms,omitempty"`
}

// SandboxReport describes the isolated workspace validation performed before install.
type SandboxReport struct {
	PackageType  string         `json:"package_type,omitempty"`
	PackageName  string         `json:"package_name,omitempty"`
	SkillName    string         `json:"skill_name"`
	WorkspaceDir string         `json:"workspace_dir,omitempty"`
	SkillDir     string         `json:"skill_dir,omitempty"`
	PluginDir    string         `json:"plugin_dir,omitempty"`
	MCPDir       string         `json:"mcp_dir,omitempty"`
	Passed       bool           `json:"passed"`
	Checks       []SandboxCheck `json:"checks"`
}

// SandboxError is returned when a package passes download verification but fails sandbox checks.
type SandboxError struct {
	Report SandboxReport
}

func (e *SandboxError) Error() string {
	if e == nil {
		return ""
	}
	packageType := strings.TrimSpace(e.Report.PackageType)
	if packageType == "" {
		packageType = "skill"
	}
	packageName := strings.TrimSpace(e.Report.PackageName)
	if packageName == "" {
		packageName = strings.TrimSpace(e.Report.SkillName)
	}
	for _, check := range e.Report.Checks {
		if check.Status == SandboxCheckFailed {
			detail := strings.TrimSpace(check.Error)
			if detail == "" {
				detail = strings.TrimSpace(check.Output)
			}
			if detail == "" {
				detail = "unknown failure"
			}
			return fmt.Sprintf("%s %q failed sandbox check %q: %s", packageType, packageName, check.Name, detail)
		}
	}
	return fmt.Sprintf("%s %q failed sandbox checks", packageType, packageName)
}

func (inst *Installer) runSkillInstallSandbox(ctx context.Context, entry *RegistryEntry, files map[string][]byte) (SandboxReport, error) {
	sandboxWorkspace, err := os.MkdirTemp("", "tars-skill-install-*")
	if err != nil {
		return SandboxReport{}, fmt.Errorf("create skill install sandbox: %w", err)
	}
	report := SandboxReport{
		PackageType:  "skill",
		PackageName:  entry.Name,
		SkillName:    entry.Name,
		WorkspaceDir: sandboxWorkspace,
		SkillDir:     filepath.Join(sandboxWorkspace, hubSkillsDir, entry.Name),
		Passed:       true,
	}
	defer func() {
		_ = os.RemoveAll(sandboxWorkspace)
	}()

	if err := materializePackageFiles(report.SkillDir, files); err != nil {
		return report, fmt.Errorf("materialize skill sandbox: %w", err)
	}

	meta, check := validateSkillSandboxManifest(entry.Name, files)
	report.Checks = append(report.Checks, check)
	if check.Status == SandboxCheckFailed {
		report.Passed = false
		return report, &SandboxError{Report: report}
	}

	for i, command := range meta.SmokeTests {
		check := runSkillSmokeCommand(ctx, report.WorkspaceDir, report.SkillDir, i+1, command)
		report.Checks = append(report.Checks, check)
		if check.Status == SandboxCheckFailed {
			report.Passed = false
			return report, &SandboxError{Report: report}
		}
	}

	return report, nil
}

func (inst *Installer) runPluginInstallSandbox(ctx context.Context, entry *PluginEntry, files map[string][]byte) (SandboxReport, error) {
	sandboxWorkspace, err := os.MkdirTemp("", "tars-plugin-install-*")
	if err != nil {
		return SandboxReport{}, fmt.Errorf("create plugin install sandbox: %w", err)
	}
	report := SandboxReport{
		PackageType:  "plugin",
		PackageName:  entry.Name,
		WorkspaceDir: sandboxWorkspace,
		PluginDir:    filepath.Join(sandboxWorkspace, hubPluginsDir, entry.Name),
		Passed:       true,
	}
	defer func() {
		_ = os.RemoveAll(sandboxWorkspace)
	}()

	if err := materializePackageFiles(report.PluginDir, files); err != nil {
		return report, fmt.Errorf("materialize plugin sandbox: %w", err)
	}

	definition, check := validatePluginSandboxManifest(entry.Name, filepath.Join(sandboxWorkspace, hubPluginsDir))
	report.Checks = append(report.Checks, check)
	if check.Status == SandboxCheckFailed {
		report.Passed = false
		return report, &SandboxError{Report: report}
	}

	report.Checks = append(report.Checks, SandboxCheck{
		Name:   "plugin_mcp_gating",
		Status: SandboxCheckPassed,
		Output: fmt.Sprintf("%d plugin-declared MCP servers remain subject to runtime gating", len(definition.MCPServers)),
	})
	return report, nil
}

func validatePluginSandboxManifest(entryName string, pluginsRoot string) (plugin.Definition, SandboxCheck) {
	snapshot, err := plugin.Load(plugin.LoadOptions{
		Sources: []plugin.SourceDir{{Source: plugin.SourceWorkspace, Dir: pluginsRoot}},
	})
	if err != nil {
		return plugin.Definition{}, SandboxCheck{
			Name:   "plugin_manifest",
			Status: SandboxCheckFailed,
			Error:  err.Error(),
		}
	}
	if len(snapshot.Diagnostics) > 0 {
		messages := make([]string, 0, len(snapshot.Diagnostics))
		for _, diagnostic := range snapshot.Diagnostics {
			if strings.TrimSpace(diagnostic.Path) != "" {
				messages = append(messages, diagnostic.Path+": "+diagnostic.Message)
			} else {
				messages = append(messages, diagnostic.Message)
			}
		}
		return plugin.Definition{}, SandboxCheck{
			Name:   "plugin_manifest",
			Status: SandboxCheckFailed,
			Error:  strings.Join(messages, "; "),
		}
	}
	for _, definition := range snapshot.Plugins {
		if strings.EqualFold(definition.ID, entryName) {
			return definition, SandboxCheck{
				Name:   "plugin_manifest",
				Status: SandboxCheckPassed,
				Output: fmt.Sprintf("%s parsed", pluginManifest),
			}
		}
	}
	return plugin.Definition{}, SandboxCheck{
		Name:   "plugin_manifest",
		Status: SandboxCheckFailed,
		Error:  fmt.Sprintf("plugin manifest id does not match registry name %q", entryName),
	}
}

func (inst *Installer) runMCPInstallSandbox(ctx context.Context, entry *MCPEntry, files map[string][]byte, manifestPath string) (SandboxReport, error) {
	sandboxWorkspace, err := os.MkdirTemp("", "tars-mcp-install-*")
	if err != nil {
		return SandboxReport{}, fmt.Errorf("create mcp install sandbox: %w", err)
	}
	report := SandboxReport{
		PackageType:  "mcp",
		PackageName:  entry.Name,
		WorkspaceDir: sandboxWorkspace,
		MCPDir:       filepath.Join(sandboxWorkspace, hubMCPDir, entry.Name),
		Passed:       true,
	}
	defer func() {
		_ = os.RemoveAll(sandboxWorkspace)
	}()

	if err := materializePackageFiles(report.MCPDir, files); err != nil {
		return report, fmt.Errorf("materialize mcp sandbox: %w", err)
	}
	manifest, check := validateMCPSandboxManifest(entry.Name, files, manifestPath)
	report.Checks = append(report.Checks, check)
	if check.Status == SandboxCheckFailed {
		report.Passed = false
		return report, &SandboxError{Report: report}
	}
	server := expandMCPServer(manifest.Server, report.MCPDir)
	check = validateMCPSandboxServer(server)
	report.Checks = append(report.Checks, check)
	if check.Status == SandboxCheckFailed {
		report.Passed = false
		return report, &SandboxError{Report: report}
	}
	return report, nil
}

func validateMCPSandboxManifest(entryName string, files map[string][]byte, manifestPath string) (MCPManifest, SandboxCheck) {
	data, ok := files[manifestPath]
	if !ok {
		return MCPManifest{}, SandboxCheck{
			Name:   "mcp_manifest",
			Status: SandboxCheckFailed,
			Error:  fmt.Sprintf("%s is missing", manifestPath),
		}
	}
	manifest, err := parseMCPManifest(data, entryName)
	if err != nil {
		return MCPManifest{}, SandboxCheck{
			Name:   "mcp_manifest",
			Status: SandboxCheckFailed,
			Error:  err.Error(),
		}
	}
	if !strings.EqualFold(manifest.Server.Name, entryName) {
		return manifest, SandboxCheck{
			Name:   "mcp_manifest",
			Status: SandboxCheckFailed,
			Error:  fmt.Sprintf("mcp server name %q does not match registry name %q", manifest.Server.Name, entryName),
		}
	}
	return manifest, SandboxCheck{
		Name:   "mcp_manifest",
		Status: SandboxCheckPassed,
		Output: fmt.Sprintf("%s parsed", manifestPath),
	}
}

func validateMCPSandboxServer(server config.MCPServer) SandboxCheck {
	if config.MCPServerIsRemote(server) {
		return SandboxCheck{
			Name:   "mcp_remote_smoke",
			Status: SandboxCheckPassed,
			Output: "remote endpoint declared; network dial skipped during install sandbox",
		}
	}
	if strings.TrimSpace(server.Command) == "" {
		return SandboxCheck{
			Name:   "mcp_stdio_smoke",
			Status: SandboxCheckFailed,
			Error:  "mcp server command is required",
		}
	}
	if _, err := exec.LookPath(server.Command); err != nil {
		return SandboxCheck{
			Name:    "mcp_stdio_smoke",
			Command: server.Command,
			Status:  SandboxCheckFailed,
			Error:   err.Error(),
		}
	}
	return SandboxCheck{
		Name:    "mcp_stdio_smoke",
		Command: server.Command,
		Status:  SandboxCheckPassed,
		Output:  "stdio command is available",
	}
}

func validateSkillSandboxManifest(entryName string, files map[string][]byte) (skill.Frontmatter, SandboxCheck) {
	data, ok := files[skillManifest]
	if !ok {
		return skill.Frontmatter{}, SandboxCheck{
			Name:   "manifest",
			Status: SandboxCheckFailed,
			Error:  fmt.Sprintf("%s is missing", skillManifest),
		}
	}
	meta, _, err := skill.ParseFrontmatter(string(data))
	if err != nil {
		return skill.Frontmatter{}, SandboxCheck{
			Name:   "manifest",
			Status: SandboxCheckFailed,
			Error:  err.Error(),
		}
	}
	if strings.TrimSpace(meta.Name) != "" && !strings.EqualFold(meta.Name, entryName) {
		return meta, SandboxCheck{
			Name:   "manifest",
			Status: SandboxCheckFailed,
			Error:  fmt.Sprintf("frontmatter name %q does not match registry name %q", meta.Name, entryName),
		}
	}
	return meta, SandboxCheck{
		Name:   "manifest",
		Status: SandboxCheckPassed,
		Output: fmt.Sprintf("%s parsed", skillManifest),
	}
}

func runSkillSmokeCommand(ctx context.Context, workspaceDir string, skillDir string, index int, command string) SandboxCheck {
	command = strings.TrimSpace(command)
	check := SandboxCheck{
		Name:    fmt.Sprintf("smoke_%d", index),
		Command: command,
	}
	if command == "" {
		check.Status = SandboxCheckFailed
		check.Error = "smoke command is empty"
		return check
	}

	start := time.Now()
	cmdCtx, cancel := context.WithTimeout(ctx, defaultSkillSmokeTimeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "/bin/sh", "-c", command)
	cmd.Dir = skillDir
	cmd.Env = append(os.Environ(),
		"TARS_SANDBOX=1",
		"TARS_WORKSPACE_DIR="+workspaceDir,
		"TARS_SKILL_DIR="+skillDir,
	)
	output, err := cmd.CombinedOutput()
	check.DurationMS = time.Since(start).Milliseconds()
	check.Output = truncateSandboxOutput(string(output))
	if cmdCtx.Err() == context.DeadlineExceeded {
		check.Status = SandboxCheckFailed
		check.Error = fmt.Sprintf("timed out after %s", defaultSkillSmokeTimeout)
		return check
	}
	if err != nil {
		check.Status = SandboxCheckFailed
		check.Error = err.Error()
		return check
	}
	check.Status = SandboxCheckPassed
	return check
}

func truncateSandboxOutput(output string) string {
	const maxOutput = 4096
	output = strings.TrimSpace(output)
	if len(output) <= maxOutput {
		return output
	}
	return output[:maxOutput] + "\n... output truncated ..."
}
