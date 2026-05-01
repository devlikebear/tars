package skillhub

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	SkillName    string         `json:"skill_name"`
	WorkspaceDir string         `json:"workspace_dir,omitempty"`
	SkillDir     string         `json:"skill_dir,omitempty"`
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
	for _, check := range e.Report.Checks {
		if check.Status == SandboxCheckFailed {
			detail := strings.TrimSpace(check.Error)
			if detail == "" {
				detail = strings.TrimSpace(check.Output)
			}
			if detail == "" {
				detail = "unknown failure"
			}
			return fmt.Sprintf("skill %q failed sandbox check %q: %s", e.Report.SkillName, check.Name, detail)
		}
	}
	return fmt.Sprintf("skill %q failed sandbox checks", e.Report.SkillName)
}

func (inst *Installer) runSkillInstallSandbox(ctx context.Context, entry *RegistryEntry, files map[string][]byte) (SandboxReport, error) {
	sandboxWorkspace, err := os.MkdirTemp("", "tars-skill-install-*")
	if err != nil {
		return SandboxReport{}, fmt.Errorf("create skill install sandbox: %w", err)
	}
	report := SandboxReport{
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
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
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
