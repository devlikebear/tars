package tarsserver

import (
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/tool"
)

// execSandboxFromConfig turns tools.exec.sandbox into the exec tool's
// sandbox, or nil when it is off (#1310).
//
// On top of the configured paths it hides TARS's own config — it holds the
// provider API keys — and keeps the always-allow permission rules read-only:
// a sandboxed command that could add a rule there would have the next CLI
// turn run it outside the sandbox.
func execSandboxFromConfig(cfg config.Config) *tool.ExecSandbox {
	if !cfg.ToolsExecSandboxEnabled {
		return nil
	}
	hidden := append([]string(nil), cfg.ToolsExecSandboxDenyReadPaths...)
	hidden = append(hidden, filepath.Join(config.TarsHomeDir(), "config"))
	if path := strings.TrimSpace(config.ResolveConfigPath("")); path != "" && path != config.DefaultConfigFilename {
		hidden = append(hidden, path)
	}
	readOnly := append([]string(nil), cfg.ToolsExecSandboxReadOnlyPaths...)
	if workspace := strings.TrimSpace(cfg.WorkspaceDir); workspace != "" {
		hidden = append(hidden, filepath.Join(workspace, "config"))
		readOnly = append(readOnly, filepath.Join(workspace, "_shared", "permissions"))
	}
	return &tool.ExecSandbox{
		Network:       cfg.ToolsExecSandboxNetwork,
		WritablePaths: append([]string(nil), cfg.ToolsExecSandboxWritablePaths...),
		ReadOnlyPaths: readOnly,
		DenyReadPaths: hidden,
	}
}
