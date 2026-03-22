package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/rs/zerolog"
)

func buildGatewayExecutors(
	cfg config.Config,
	runPrompt gatewayPromptRunner,
	logger zerolog.Logger,
) []gateway.AgentExecutor {
	out := make([]gateway.AgentExecutor, 0, len(cfg.GatewayAgents))
	registeredNames := map[string]struct{}{}
	for _, spec := range cfg.GatewayAgents {
		if !spec.Enabled {
			continue
		}
		name := strings.TrimSpace(spec.Name)
		command := strings.TrimSpace(os.ExpandEnv(spec.Command))
		if name == "" || command == "" {
			logger.Warn().
				Str("agent", name).
				Str("command", command).
				Msg("skipping invalid gateway agent executor config")
			continue
		}

		args := make([]string, 0, len(spec.Args))
		for _, arg := range spec.Args {
			args = append(args, os.ExpandEnv(arg))
		}

		env := map[string]string{}
		for key, value := range spec.Env {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			env[trimmed] = os.ExpandEnv(value)
		}

		workDir := strings.TrimSpace(os.ExpandEnv(spec.WorkingDir))
		if workDir == "" {
			workDir = strings.TrimSpace(cfg.WorkspaceDir)
		} else if !filepath.IsAbs(workDir) && strings.TrimSpace(cfg.WorkspaceDir) != "" {
			workDir = filepath.Join(cfg.WorkspaceDir, workDir)
		}

		timeout := time.Duration(spec.TimeoutSeconds) * time.Second
		executor, err := gateway.NewCommandExecutor(gateway.CommandExecutorOptions{
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Source:      "config",
			Command:     command,
			Args:        args,
			Env:         env,
			WorkDir:     workDir,
			Timeout:     timeout,
		})
		if err != nil {
			logger.Warn().Err(err).Str("agent", name).Msg("failed to build gateway agent executor")
			continue
		}
		out = append(out, executor)
		registeredNames[strings.ToLower(name)] = struct{}{}
	}

	if runPrompt != nil {
		if _, exists := registeredNames["explorer"]; !exists {
			executor, err := gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
				Name:        "explorer",
				Description: "Built-in read-only codebase exploration agent",
				Source:      "builtin",
				Entry:       "llm-loop:explorer",
				PolicyMode:  "allowlist",
				ToolsAllow: []string{
					"memory_get", "memory_search",
					"project_activity_get", "project_board_get", "project_brief_get", "project_get", "project_list", "project_state_get",
					"read", "read_file", "list_dir", "glob",
					"research_report", "usage_report",
				},
				RunPrompt: func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error) {
					label := strings.TrimSpace(runLabel)
					if label == "" {
						label = "spawn"
					}
					label += ":explorer"
					return runPrompt(ctx, label, prompt, allowedTools)
				},
			})
			if err != nil {
				logger.Warn().Err(err).Msg("failed to build built-in explorer executor")
			} else {
				out = append(out, executor)
				registeredNames["explorer"] = struct{}{}
			}
		}
	}

	if runPrompt == nil {
		return out
	}

	workspaceAgents, diagnostics, err := loadWorkspaceGatewayAgents(cfg.WorkspaceDir)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load workspace markdown agents")
		return out
	}
	for _, diag := range diagnostics {
		logger.Warn().Str("diagnostic", strings.TrimSpace(diag)).Msg("workspace gateway agent diagnostic")
	}
	for _, def := range workspaceAgents {
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if key == "" {
			continue
		}
		if _, exists := registeredNames[key]; exists {
			continue
		}
		executor, err := newWorkspacePromptExecutor(def, runPrompt)
		if err != nil {
			logger.Warn().Err(err).Str("agent", def.Name).Msg("failed to build workspace prompt executor")
			continue
		}
		out = append(out, executor)
		registeredNames[key] = struct{}{}
	}
	return out
}
