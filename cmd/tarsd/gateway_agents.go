package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/skill"
)

type workspaceGatewayAgent struct {
	Name        string
	Description string
	Prompt      string
	FilePath    string
}

func loadWorkspaceGatewayAgents(workspaceDir string) ([]workspaceGatewayAgent, error) {
	base := strings.TrimSpace(workspaceDir)
	if base == "" {
		return []workspaceGatewayAgent{}, nil
	}
	root := filepath.Join(base, "agents")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []workspaceGatewayAgent{}, nil
		}
		return nil, fmt.Errorf("stat agents dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return []workspaceGatewayAgent{}, nil
	}

	loaded := make([]workspaceGatewayAgent, 0)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Base(path), "AGENT.md") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		meta, body, err := skill.ParseFrontmatter(string(raw))
		if err != nil {
			return nil
		}
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
		}
		if !isValidGatewayAgentName(name) {
			return nil
		}
		prompt := strings.TrimSpace(body)
		if prompt == "" {
			return nil
		}
		description := strings.TrimSpace(meta.Description)
		if description == "" {
			description = inferGatewayAgentDescription(prompt)
		}
		if description == "" {
			description = "Workspace markdown sub-agent"
		}
		loaded = append(loaded, workspaceGatewayAgent{
			Name:        name,
			Description: description,
			Prompt:      prompt,
			FilePath:    path,
		})
		return nil
	})

	sort.Slice(loaded, func(i, j int) bool {
		left := strings.ToLower(loaded[i].FilePath)
		right := strings.ToLower(loaded[j].FilePath)
		return left < right
	})

	seen := map[string]struct{}{}
	out := make([]workspaceGatewayAgent, 0, len(loaded))
	for _, item := range loaded {
		key := strings.ToLower(strings.TrimSpace(item.Name))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out, nil
}

func isValidGatewayAgentName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}
	for _, ch := range trimmed {
		if (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' ||
			ch == '_' ||
			ch == '.' {
			continue
		}
		return false
	}
	return true
}

func inferGatewayAgentDescription(prompt string) string {
	lines := strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "#")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > 140 {
			return trimmed[:140] + "..."
		}
		return trimmed
	}
	return ""
}

func newWorkspacePromptExecutor(
	def workspaceGatewayAgent,
	runPrompt func(ctx context.Context, runLabel string, prompt string) (string, error),
) (gateway.AgentExecutor, error) {
	if runPrompt == nil {
		return nil, fmt.Errorf("run prompt is required")
	}
	name := strings.TrimSpace(def.Name)
	description := strings.TrimSpace(def.Description)
	if description == "" {
		description = "Workspace markdown sub-agent"
	}
	instructions := strings.TrimSpace(def.Prompt)
	return gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name:        name,
		Description: description,
		Source:      "workspace",
		Entry:       strings.TrimSpace(def.FilePath),
		RunPrompt: func(ctx context.Context, runLabel string, prompt string) (string, error) {
			label := strings.TrimSpace(runLabel)
			if label == "" {
				label = "spawn"
			}
			label += ":" + name
			return runPrompt(ctx, label, composeWorkspaceAgentPrompt(name, instructions, prompt))
		},
	})
}

func composeWorkspaceAgentPrompt(name, instructions, userPrompt string) string {
	task := strings.TrimSpace(userPrompt)
	profile := strings.TrimSpace(instructions)
	if profile == "" {
		return task
	}
	var b strings.Builder
	b.WriteString("Sub-agent profile: ")
	b.WriteString(strings.TrimSpace(name))
	b.WriteString("\n\n")
	b.WriteString(profile)
	if task != "" {
		b.WriteString("\n\nUser task:\n")
		b.WriteString(task)
	}
	return b.String()
}
