package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type workspaceGatewayAgent struct {
	Name        string
	Description string
	Prompt      string
	FilePath    string
	PolicyMode  string
	ToolsAllow  []string
}

type workspaceGatewayAgentFrontmatter struct {
	Name             string
	Description      string
	ToolsAllow       []string
	ToolsAllowExists bool
}

func loadWorkspaceGatewayAgents(workspaceDir string) ([]workspaceGatewayAgent, []string, error) {
	base := strings.TrimSpace(workspaceDir)
	if base == "" {
		return []workspaceGatewayAgent{}, []string{}, nil
	}
	root := filepath.Join(base, "agents")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []workspaceGatewayAgent{}, []string{}, nil
		}
		return nil, nil, fmt.Errorf("stat agents dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return []workspaceGatewayAgent{}, []string{}, nil
	}

	knownTools := knownGatewayPromptTools(base)
	loaded := make([]workspaceGatewayAgent, 0)
	diagnostics := make([]string, 0)
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
		meta, body, err := parseWorkspaceGatewayAgentDocument(string(raw))
		if err != nil {
			diagnostics = append(diagnostics, fmt.Sprintf("skip %s: invalid frontmatter: %v", path, err))
			return nil
		}
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = strings.TrimSpace(filepath.Base(filepath.Dir(path)))
		}
		if !isValidGatewayAgentName(name) {
			diagnostics = append(diagnostics, fmt.Sprintf("skip %s: invalid agent name %q", path, name))
			return nil
		}
		prompt := strings.TrimSpace(body)
		if prompt == "" {
			diagnostics = append(diagnostics, fmt.Sprintf("skip %s: empty prompt body", path))
			return nil
		}
		description := strings.TrimSpace(meta.Description)
		if description == "" {
			description = inferGatewayAgentDescription(prompt)
		}
		if description == "" {
			description = "Workspace markdown sub-agent"
		}
		policyMode := "full"
		toolsAllow := []string{}
		if meta.ToolsAllowExists {
			policyMode = "allowlist"
			normalized, unknown := normalizeGatewayToolsAllow(meta.ToolsAllow, knownTools)
			if len(unknown) > 0 {
				diagnostics = append(
					diagnostics,
					fmt.Sprintf("agent %s tools_allow ignored unknown tools: %s", name, strings.Join(unknown, ", ")),
				)
			}
			if len(normalized) == 0 {
				diagnostics = append(diagnostics, fmt.Sprintf("skip agent %s: tools_allow has no valid tools", name))
				return nil
			}
			toolsAllow = normalized
		}
		loaded = append(loaded, workspaceGatewayAgent{
			Name:        name,
			Description: description,
			Prompt:      prompt,
			FilePath:    path,
			PolicyMode:  policyMode,
			ToolsAllow:  toolsAllow,
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
			diagnostics = append(diagnostics, fmt.Sprintf("skip duplicate agent name: %s", item.Name))
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out, diagnostics, nil
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
	runPrompt func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error),
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
		PolicyMode:  normalizeGatewayPolicyMode(def.PolicyMode),
		ToolsAllow:  append([]string(nil), def.ToolsAllow...),
		RunPrompt: func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error) {
			label := strings.TrimSpace(runLabel)
			if label == "" {
				label = "spawn"
			}
			label += ":" + name
			return runPrompt(ctx, label, composeWorkspaceAgentPrompt(name, instructions, prompt), allowedTools)
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

func normalizeGatewayPolicyMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "full"
	}
	if mode == "allowlist" {
		return mode
	}
	return "full"
}

func knownGatewayPromptTools(workspaceDir string) map[string]struct{} {
	out := map[string]struct{}{}
	registry := newBaseToolRegistry(workspaceDir)
	for _, schema := range registry.Schemas() {
		name := tool.CanonicalToolName(schema.Function.Name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func normalizeGatewayToolsAllow(raw []string, known map[string]struct{}) ([]string, []string) {
	normalized := make([]string, 0, len(raw))
	unknown := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		canonical := tool.CanonicalToolName(trimmed)
		if canonical == "" {
			continue
		}
		if _, ok := known[canonical]; !ok {
			unknown = append(unknown, trimmed)
			continue
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}
	sort.Strings(normalized)
	sort.Strings(unknown)
	return normalized, unknown
}

func parseWorkspaceGatewayAgentDocument(raw string) (workspaceGatewayAgentFrontmatter, string, error) {
	metaBlock, body, hasFrontmatter, err := splitYAMLFrontmatter(raw)
	if err != nil {
		return workspaceGatewayAgentFrontmatter{}, "", err
	}
	if !hasFrontmatter {
		return workspaceGatewayAgentFrontmatter{}, body, nil
	}
	meta, err := parseWorkspaceGatewayAgentFrontmatter(metaBlock)
	if err != nil {
		return workspaceGatewayAgentFrontmatter{}, "", err
	}
	return meta, body, nil
}

func splitYAMLFrontmatter(raw string) (meta string, body string, hasFrontmatter bool, err error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", raw, false, nil
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
			return rest[:end], "", true, nil
		}
		return "", "", false, fmt.Errorf("unterminated frontmatter block")
	}
	return rest[:end], rest[end+len("\n---\n"):], true, nil
}

func parseWorkspaceGatewayAgentFrontmatter(raw string) (workspaceGatewayAgentFrontmatter, error) {
	meta := workspaceGatewayAgentFrontmatter{}
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			return workspaceGatewayAgentFrontmatter{}, fmt.Errorf("unexpected list item: %q", line)
		}
		keyRaw, valueRaw, ok := strings.Cut(line, ":")
		if !ok {
			return workspaceGatewayAgentFrontmatter{}, fmt.Errorf("invalid frontmatter line: %q", lines[i])
		}
		key := strings.ToLower(strings.TrimSpace(keyRaw))
		value := strings.TrimSpace(valueRaw)

		switch key {
		case "name":
			meta.Name = trimYAMLScalar(value)
		case "description":
			meta.Description = trimYAMLScalar(value)
		case "tools_allow", "tools-allow":
			meta.ToolsAllowExists = true
			if value == "" {
				for i+1 < len(lines) {
					next := strings.TrimSpace(lines[i+1])
					if next == "" || strings.HasPrefix(next, "#") {
						i++
						continue
					}
					if !strings.HasPrefix(next, "-") {
						break
					}
					item := strings.TrimSpace(strings.TrimPrefix(next, "-"))
					item = trimYAMLScalar(item)
					if item != "" {
						meta.ToolsAllow = append(meta.ToolsAllow, item)
					}
					i++
				}
				continue
			}
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				meta.ToolsAllow = append(meta.ToolsAllow, parseInlineYAMLList(value)...)
				continue
			}
			item := trimYAMLScalar(value)
			if item != "" {
				meta.ToolsAllow = append(meta.ToolsAllow, item)
			}
		}
	}
	return meta, nil
}

func parseInlineYAMLList(raw string) []string {
	inner := strings.TrimSpace(raw)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	if strings.TrimSpace(inner) == "" {
		return []string{}
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := trimYAMLScalar(part)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func trimYAMLScalar(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') ||
			(trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') {
			trimmed = trimmed[1 : len(trimmed)-1]
		}
	}
	return strings.TrimSpace(trimmed)
}
