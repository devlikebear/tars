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
	SessionID    string
	Prompt       string
	AllowedTools []string
}

type AgentInfo struct {
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	Enabled         bool     `json:"enabled"`
	Kind            string   `json:"kind,omitempty"`
	Source          string   `json:"source,omitempty"`
	Entry           string   `json:"entry,omitempty"`
	PolicyMode      string   `json:"policy_mode"`
	ToolsAllow      []string `json:"tools_allow,omitempty"`
	ToolsAllowCount int      `json:"tools_allow_count"`
}

type AgentExecutor interface {
	Info() AgentInfo
	Execute(ctx context.Context, req ExecuteRequest) (string, error)
}

type PromptExecutor struct {
	name        string
	description string
	kind        string
	source      string
	entry       string
	policyMode  string
	toolsAllow  []string
	runPrompt   func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error)
}

type PromptExecutorOptions struct {
	Name        string
	Description string
	Source      string
	Entry       string
	PolicyMode  string
	ToolsAllow  []string
	RunPrompt   func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error)
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
	if policyMode == "allowlist" && len(toolsAllow) == 0 {
		return nil, fmt.Errorf("allowlist policy requires at least one allowed tool")
	}
	return &PromptExecutor{
		name:        trimmed,
		description: description,
		kind:        "prompt",
		source:      source,
		entry:       strings.TrimSpace(opts.Entry),
		policyMode:  policyMode,
		toolsAllow:  toolsAllow,
		runPrompt:   opts.RunPrompt,
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
		Name:            e.name,
		Description:     e.description,
		Enabled:         true,
		Kind:            e.kind,
		Source:          e.source,
		Entry:           e.entry,
		PolicyMode:      normalizePolicyMode(e.policyMode),
		ToolsAllow:      append([]string(nil), e.toolsAllow...),
		ToolsAllowCount: len(e.toolsAllow),
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
	if v := strings.TrimSpace(req.RunID); v != "" {
		env = append(env, "TARS_RUN_ID="+v)
	}
	if v := strings.TrimSpace(req.SessionID); v != "" {
		env = append(env, "TARS_SESSION_ID="+v)
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

func sanitizeToolsAllow(raw []string) []string {
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
