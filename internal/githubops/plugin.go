// Package githubops provides the github-ops builtin plugin that wraps the
// `gh` CLI for issue/PR/worktree operations used by dogfooding automation.
package githubops

import (
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/tool"
)

const pluginID = "tars-github-ops"

type Plugin struct {
	workspaceDir string
}

func (p *Plugin) ID() string { return pluginID }

func (p *Plugin) Definition() plugin.Definition {
	return plugin.Definition{
		SchemaVersion: 3,
		ID:            pluginID,
		Name:          "GitHub Ops",
		Description:   "Wraps the `gh` CLI: search/create/comment issues, create draft PRs, manage external-repo worktrees.",
		Version:       "0.1.0",
		Source:        plugin.SourceBundled,
		Requires: plugin.Requires{
			Bins: []string{"gh", "git"},
		},
		ToolsProvider: &plugin.ToolsProvider{
			Type:  "go_plugin",
			Entry: "builtin:" + pluginID,
		},
	}
}

func (p *Plugin) Init(ctx plugin.PluginContext) error {
	p.workspaceDir = strings.TrimSpace(ctx.WorkspaceDir)
	return nil
}

func (p *Plugin) Close() error { return nil }

func (p *Plugin) HTTPHandlers() []plugin.HTTPHandlerEntry { return nil }

func (p *Plugin) managedReposRoot() string {
	if p.workspaceDir == "" {
		return ""
	}
	return filepath.Join(p.workspaceDir, "managed-repos")
}

func (p *Plugin) Tools() []tool.Tool {
	return []tool.Tool{
		newIssueSearchTool(defaultGHRunner),
		newIssueCreateTool(defaultGHRunner),
		newIssueCommentTool(defaultGHRunner),
		newPRCreateDraftTool(defaultGHRunner),
		newWorktreeSetupTool(defaultGitRunner, p.managedReposRoot),
		newWorktreeCleanupTool(defaultGitRunner, p.managedReposRoot),
	}
}
