package gateway

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type ExecuteRequest struct {
	RunID        string
	WorkspaceID  string
	SessionID    string
	ProjectID    string
	Prompt       string
	AllowedTools []string
}

type AgentInfo struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	Enabled            bool     `json:"enabled"`
	Kind               string   `json:"kind,omitempty"`
	Source             string   `json:"source,omitempty"`
	Entry              string   `json:"entry,omitempty"`
	PolicyMode         string   `json:"policy_mode"`
	ToolsAllow         []string `json:"tools_allow,omitempty"`
	ToolsAllowCount    int      `json:"tools_allow_count"`
	ToolsDeny          []string `json:"tools_deny,omitempty"`
	ToolsDenyCount     int      `json:"tools_deny_count"`
	ToolsRiskMax       string   `json:"tools_risk_max,omitempty"`
	ToolsAllowGroups   []string `json:"tools_allow_groups,omitempty"`
	ToolsAllowPatterns []string `json:"tools_allow_patterns,omitempty"`
	SessionRoutingMode string   `json:"session_routing_mode,omitempty"`
	SessionFixedID     string   `json:"session_fixed_id,omitempty"`
}

type AgentExecutor interface {
	Info() AgentInfo
	Execute(ctx context.Context, req ExecuteRequest) (string, error)
}

type PromptExecutor struct {
	name               string
	description        string
	kind               string
	source             string
	entry              string
	policyMode         string
	toolsAllow         []string
	toolsDeny          []string
	toolsRiskMax       string
	toolsAllowGroups   []string
	toolsAllowPatterns []string
	sessionRoutingMode string
	sessionFixedID     string
	runPrompt          func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error)
}

type PromptExecutorOptions struct {
	Name               string
	Description        string
	Source             string
	Entry              string
	PolicyMode         string
	ToolsAllow         []string
	ToolsDeny          []string
	ToolsRiskMax       string
	ToolsAllowGroups   []string
	ToolsAllowPatterns []string
	SessionRoutingMode string
	SessionFixedID     string
	RunPrompt          func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error)
}

func NewPromptExecutorWithOptions(opts PromptExecutorOptions) (*PromptExecutor, error) {
	trimmed := strings.TrimSpace(opts.Name)
	if trimmed == "" {
		return nil, fmt.Errorf("executor name is required")
	}
	if opts.RunPrompt == nil {
		return nil, fmt.Errorf("run prompt is required")
	}
	description := strings.TrimSpace(opts.Description)
	if description == "" {
		description = "Prompt-based in-process executor"
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "prompt"
	}
	policyMode := normalizePolicyMode(opts.PolicyMode)
	toolsAllow := sanitizeToolsAllow(opts.ToolsAllow)
	toolsDeny := sanitizeToolsAllow(opts.ToolsDeny)
	toolsRiskMax := normalizeToolRiskMax(opts.ToolsRiskMax)
	toolsAllowGroups := sanitizeStringList(opts.ToolsAllowGroups)
	toolsAllowPatterns := sanitizeStringList(opts.ToolsAllowPatterns)
	sessionRoutingMode := normalizeSessionRoutingMode(opts.SessionRoutingMode)
	sessionFixedID := strings.TrimSpace(opts.SessionFixedID)
	if policyMode == "allowlist" && len(toolsAllow) == 0 {
		return nil, fmt.Errorf("allowlist policy requires at least one allowed tool")
	}
	if sessionRoutingMode == "fixed" && sessionFixedID == "" {
		return nil, fmt.Errorf("fixed session routing requires session_fixed_id")
	}
	return &PromptExecutor{
		name:               trimmed,
		description:        description,
		kind:               "prompt",
		source:             source,
		entry:              strings.TrimSpace(opts.Entry),
		policyMode:         policyMode,
		toolsAllow:         toolsAllow,
		toolsDeny:          toolsDeny,
		toolsRiskMax:       toolsRiskMax,
		toolsAllowGroups:   toolsAllowGroups,
		toolsAllowPatterns: toolsAllowPatterns,
		sessionRoutingMode: sessionRoutingMode,
		sessionFixedID:     sessionFixedID,
		runPrompt:          opts.RunPrompt,
	}, nil
}

func NewPromptExecutor(name, description string, runPrompt func(ctx context.Context, runLabel string, prompt string) (string, error)) (*PromptExecutor, error) {
	return NewPromptExecutorWithOptions(PromptExecutorOptions{
		Name:        name,
		Description: description,
		RunPrompt: func(ctx context.Context, runLabel string, prompt string, _ []string) (string, error) {
			return runPrompt(ctx, runLabel, prompt)
		},
	})
}

func (e *PromptExecutor) Info() AgentInfo {
	if e == nil {
		return AgentInfo{}
	}
	return AgentInfo{
		Name:               e.name,
		Description:        e.description,
		Enabled:            true,
		Kind:               e.kind,
		Source:             e.source,
		Entry:              e.entry,
		PolicyMode:         normalizePolicyMode(e.policyMode),
		ToolsAllow:         append([]string(nil), e.toolsAllow...),
		ToolsAllowCount:    len(e.toolsAllow),
		ToolsDeny:          append([]string(nil), e.toolsDeny...),
		ToolsDenyCount:     len(e.toolsDeny),
		ToolsRiskMax:       normalizeToolRiskMax(e.toolsRiskMax),
		ToolsAllowGroups:   append([]string(nil), e.toolsAllowGroups...),
		ToolsAllowPatterns: append([]string(nil), e.toolsAllowPatterns...),
		SessionRoutingMode: normalizeSessionRoutingMode(e.sessionRoutingMode),
		SessionFixedID:     strings.TrimSpace(e.sessionFixedID),
	}
}

func (e *PromptExecutor) Execute(ctx context.Context, req ExecuteRequest) (string, error) {
	if e == nil || e.runPrompt == nil {
		return "", fmt.Errorf("executor is not configured")
	}
	runLabel := "spawn"
	if strings.TrimSpace(req.RunID) != "" {
		runLabel = "spawn:" + strings.TrimSpace(req.RunID)
	}
	allowed := sanitizeToolsAllow(req.AllowedTools)
	if len(allowed) == 0 {
		allowed = append([]string(nil), e.toolsAllow...)
	}
	return e.runPrompt(ctx, runLabel, strings.TrimSpace(req.Prompt), allowed)
}

type CommandExecutorOptions struct {
	Name        string
	Description string
	Source      string
	Entry       string
	Command     string
	Args        []string
	Env         map[string]string
	WorkDir     string
	Timeout     time.Duration
}

type CommandExecutor struct {
	name        string
	description string
	kind        string
	source      string
	entry       string
	command     string
	args        []string
	env         map[string]string
	workDir     string
	timeout     time.Duration
}

func NewCommandExecutor(opts CommandExecutorOptions) (*CommandExecutor, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("executor name is required")
	}
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		return nil, fmt.Errorf("executor command is required")
	}
	description := strings.TrimSpace(opts.Description)
	if description == "" {
		description = "External command-based gateway agent executor"
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "command"
	}
	entry := strings.TrimSpace(opts.Entry)
	if entry == "" {
		entry = command
		if len(opts.Args) > 0 {
			entry = command + " " + strings.Join(opts.Args, " ")
		}
	}
	envCopy := map[string]string{}
	for key, value := range opts.Env {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		envCopy[k] = value
	}
	return &CommandExecutor{
		name:        name,
		description: description,
		kind:        "command",
		source:      source,
		entry:       entry,
		command:     command,
		args:        append([]string(nil), opts.Args...),
		env:         envCopy,
		workDir:     strings.TrimSpace(opts.WorkDir),
		timeout:     opts.Timeout,
	}, nil
}

func (e *CommandExecutor) Info() AgentInfo {
	if e == nil {
		return AgentInfo{}
	}
	return AgentInfo{
		Name:            e.name,
		Description:     e.description,
		Enabled:         true,
		Kind:            e.kind,
		Source:          e.source,
		Entry:           e.entry,
		PolicyMode:      "full",
		ToolsAllowCount: 0,
		ToolsDenyCount:  0,
	}
}

func (e *CommandExecutor) Execute(ctx context.Context, req ExecuteRequest) (string, error) {
	if e == nil || strings.TrimSpace(e.command) == "" {
		return "", fmt.Errorf("executor is not configured")
	}
	runCtx := ctx
	cancel := func() {}
	if e.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, e.timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, e.command, e.args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	env := append([]string{}, os.Environ()...)
	for key, value := range e.env {
		env = append(env, key+"="+value)
	}
	if v, ok := sanitizeMetadataEnvValue(req.RunID); ok {
		env = append(env, "TARS_RUN_ID="+v)
	}
	if v, ok := sanitizeMetadataEnvValue(req.SessionID); ok {
		env = append(env, "TARS_SESSION_ID="+v)
	}
	if v, ok := sanitizeMetadataEnvValue(req.WorkspaceID); ok {
		env = append(env, "TARS_WORKSPACE_ID="+v)
	}
	cmd.Env = env
	cmd.Stdin = strings.NewReader(req.Prompt)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return "", fmt.Errorf("command executor %q failed: %w: %s", e.name, err, errText)
		}
		return "", fmt.Errorf("command executor %q failed: %w", e.name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func normalizePolicyMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "allowlist" {
		return mode
	}
	return "full"
}

func sanitizeMetadataEnvValue(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return trimmed, true
}

func sanitizeToolsAllow(raw []string) []string {
	return sanitizeStringList(raw)
}

func sanitizeStringList(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func normalizeSessionRoutingMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", "caller":
		return "caller"
	case "new", "fixed":
		return mode
	default:
		return "caller"
	}
}

func normalizeToolRiskMax(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "low", "medium", "high":
		return value
	default:
		return ""
	}
}
