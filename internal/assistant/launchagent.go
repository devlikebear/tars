package assistant

import (
	"github.com/devlikebear/tars/internal/launchagent"
)

const DefaultLaunchAgentLabel = launchagent.DefaultAssistantLabel

type LaunchAgentConfig = launchagent.Config

func BuildLaunchAgentPlist(cfg LaunchAgentConfig) string {
	cfg.DefaultLabel = DefaultLaunchAgentLabel
	return launchagent.BuildPlist(cfg)
}

func DefaultLaunchAgentPath(label string) (string, error) {
	return launchagent.DefaultPath(label, DefaultLaunchAgentLabel)
}

func InstallLaunchAgent(path string, content string) error {
	return launchagent.Install(path, content)
}
