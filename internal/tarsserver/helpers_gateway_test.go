package tarsserver

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/gateway"
	"github.com/devlikebear/tarsncase/internal/llm"
	"github.com/rs/zerolog"
)

func TestBuildGatewayExecutors_CommandExecutor(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Config{
		WorkspaceDir: workspace,
		GatewayAgents: []config.GatewayAgent{
			{
				Name:    "worker",
				Command: "sh",
				Args:    []string{"-c", "pwd"},
				Enabled: true,
			},
		},
	}

	executors := buildGatewayExecutors(cfg, nil, zerolog.New(io.Discard))
	if len(executors) != 1 {
		t.Fatalf("expected 1 gateway executor, got %d", len(executors))
	}
	if executors[0].Info().Name != "worker" {
		t.Fatalf("unexpected executor info: %+v", executors[0].Info())
	}

	out, err := executors[0].Execute(context.Background(), gateway.ExecuteRequest{Prompt: "ignored"})
	if err != nil {
		t.Fatalf("executor execute: %v", err)
	}
	got := strings.TrimSpace(out)
	gotResolved, gotErr := filepath.EvalSymlinks(got)
	expectedResolved, expErr := filepath.EvalSymlinks(workspace)
	if gotErr == nil && expErr == nil {
		got = gotResolved
		workspace = expectedResolved
	}
	if got != workspace {
		t.Fatalf("expected executor workdir %q, got %q", workspace, got)
	}
}

func TestBuildGatewayExecutors_ResolveRelativeWorkingDir(t *testing.T) {
	workspace := t.TempDir()
	relativeDir := "nested"
	if err := os.MkdirAll(filepath.Join(workspace, relativeDir), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := config.Config{
		WorkspaceDir: workspace,
		GatewayAgents: []config.GatewayAgent{
			{
				Name:       "worker",
				Command:    "sh",
				Args:       []string{"-c", "pwd"},
				WorkingDir: relativeDir,
				Enabled:    true,
			},
		},
	}

	executors := buildGatewayExecutors(cfg, nil, zerolog.New(io.Discard))
	if len(executors) != 1 {
		t.Fatalf("expected 1 gateway executor, got %d", len(executors))
	}
	out, err := executors[0].Execute(context.Background(), gateway.ExecuteRequest{})
	if err != nil {
		t.Fatalf("executor execute: %v", err)
	}
	got := strings.TrimSpace(out)
	expected := filepath.Join(workspace, relativeDir)
	gotResolved, gotErr := filepath.EvalSymlinks(got)
	expectedResolved, expErr := filepath.EvalSymlinks(expected)
	if gotErr == nil && expErr == nil {
		got = gotResolved
		expected = expectedResolved
	}
	if got != expected {
		t.Fatalf("expected workdir %q, got %q", expected, got)
	}
}

func TestBuildGatewayExecutors_SkipDisabledAndInvalid(t *testing.T) {
	cfg := config.Config{
		WorkspaceDir: t.TempDir(),
		GatewayAgents: []config.GatewayAgent{
			{
				Name:    "disabled",
				Command: "sh",
				Args:    []string{"-c", "echo disabled"},
				Enabled: false,
			},
			{
				Name:    "invalid",
				Command: "",
				Enabled: true,
			},
			{
				Name:    "ok",
				Command: "sh",
				Args:    []string{"-c", "echo ok"},
				Enabled: true,
			},
		},
	}
	executors := buildGatewayExecutors(cfg, nil, zerolog.New(io.Discard))
	if len(executors) != 1 {
		t.Fatalf("expected only valid enabled executor, got %d", len(executors))
	}
	if executors[0].Info().Name != "ok" {
		t.Fatalf("unexpected executor name: %s", executors[0].Info().Name)
	}
}

func TestBuildGatewayExecutors_LoadWorkspaceMarkdownAgent(t *testing.T) {
	workspace := t.TempDir()
	agentFile := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentFile), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	raw := `---
name: researcher
description: Research-oriented worker
---
# Researcher
Find evidence first and answer with concise bullets.
`
	if err := os.WriteFile(agentFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	var capturedPrompt string
	var capturedLabel string
	runPrompt := func(_ context.Context, runLabel string, prompt string, _ []string) (string, error) {
		capturedLabel = runLabel
		capturedPrompt = prompt
		return "ok", nil
	}
	cfg := config.Config{WorkspaceDir: workspace}

	executors := buildGatewayExecutors(cfg, runPrompt, zerolog.New(io.Discard))
	if len(executors) != 1 {
		t.Fatalf("expected markdown agent executor, got %d", len(executors))
	}
	if executors[0].Info().Name != "researcher" {
		t.Fatalf("unexpected executor info: %+v", executors[0].Info())
	}
	if executors[0].Info().Source != "workspace" {
		t.Fatalf("expected workspace source, got %+v", executors[0].Info())
	}
	if !strings.Contains(executors[0].Info().Entry, "AGENT.md") {
		t.Fatalf("expected AGENT.md entry path, got %+v", executors[0].Info())
	}

	out, err := executors[0].Execute(context.Background(), gateway.ExecuteRequest{
		RunID:  "run_test",
		Prompt: "analyze TODO list",
	})
	if err != nil {
		t.Fatalf("executor execute: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected runPrompt output, got %q", out)
	}
	if !strings.Contains(capturedLabel, "spawn:run_test") {
		t.Fatalf("expected run label to include run id, got %q", capturedLabel)
	}
	if !strings.Contains(capturedLabel, "researcher") {
		t.Fatalf("expected run label to include agent name, got %q", capturedLabel)
	}
	if !strings.Contains(capturedPrompt, "Find evidence first") {
		t.Fatalf("expected prompt to include markdown agent instructions, got %q", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "analyze TODO list") {
		t.Fatalf("expected prompt to include user request, got %q", capturedPrompt)
	}
}

func TestBuildGatewayExecutors_ConfigAgentOverridesWorkspaceMarkdown(t *testing.T) {
	workspace := t.TempDir()
	agentFile := filepath.Join(workspace, "agents", "researcher", "AGENT.md")
	if err := os.MkdirAll(filepath.Dir(agentFile), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	if err := os.WriteFile(agentFile, []byte("name: researcher\n"), 0o644); err != nil {
		t.Fatalf("write agent file: %v", err)
	}

	runPromptCalls := 0
	runPrompt := func(_ context.Context, _ string, _ string, _ []string) (string, error) {
		runPromptCalls++
		return "prompt", nil
	}
	cfg := config.Config{
		WorkspaceDir: workspace,
		GatewayAgents: []config.GatewayAgent{
			{
				Name:    "researcher",
				Command: "sh",
				Args:    []string{"-c", "echo config"},
				Enabled: true,
			},
		},
	}

	executors := buildGatewayExecutors(cfg, runPrompt, zerolog.New(io.Discard))
	if len(executors) != 1 {
		t.Fatalf("expected one executor, got %d", len(executors))
	}
	out, err := executors[0].Execute(context.Background(), gateway.ExecuteRequest{Prompt: "hello"})
	if err != nil {
		t.Fatalf("executor execute: %v", err)
	}
	if strings.TrimSpace(out) != "config" {
		t.Fatalf("expected command executor output, got %q", out)
	}
	if runPromptCalls != 0 {
		t.Fatalf("expected markdown prompt executor not to run when config executor exists")
	}
}

func TestNewAgentPromptRunnerWithTools_InjectsAllowlistOnly(t *testing.T) {
	client := &captureToolsLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role:    "assistant",
					Content: "done",
				},
			},
		},
	}
	runner := newAgentPromptRunnerWithTools(t.TempDir(), client, 4, zerolog.New(io.Discard))
	if runner == nil {
		t.Fatal("expected prompt runner")
	}

	resp, err := runner(context.Background(), "spawn:test", "hello", []string{"shell_exec", "read_file", "unknown_tool", "read_file"})
	if err != nil {
		t.Fatalf("run prompt: %v", err)
	}
	if strings.TrimSpace(resp) != "done" {
		t.Fatalf("unexpected response: %q", resp)
	}
	if len(client.seenTools) == 0 {
		t.Fatal("expected tool schemas to be captured")
	}
	got := strings.Join(client.seenTools[0], ",")
	if got != "exec,read_file" {
		t.Fatalf("unexpected injected tool schemas: %s", got)
	}
}

func TestNewAgentPromptRunnerWithTools_HardBlocksDisallowedToolCall(t *testing.T) {
	client := &captureToolsLLMClient{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "tool_1", Name: "exec", Arguments: `{"command":"pwd"}`},
					},
				},
			},
		},
	}
	runner := newAgentPromptRunnerWithTools(t.TempDir(), client, 4, zerolog.New(io.Discard))
	if runner == nil {
		t.Fatal("expected prompt runner")
	}

	_, err := runner(context.Background(), "spawn:test", "hello", []string{"read_file"})
	if err == nil {
		t.Fatal("expected hard block error for disallowed tool call")
	}
	if !strings.Contains(err.Error(), "tool not injected for this request: exec") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.seenTools) == 0 || strings.Join(client.seenTools[0], ",") != "read_file" {
		t.Fatalf("expected only read_file injected, got %+v", client.seenTools)
	}
}

type captureToolsLLMClient struct {
	responses []llm.ChatResponse
	seenTools [][]string
	callCount int
}

func (c *captureToolsLLMClient) Ask(context.Context, string) (string, error) {
	if len(c.responses) == 0 {
		return "", nil
	}
	return c.responses[0].Message.Content, nil
}

func (c *captureToolsLLMClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	names := make([]string, 0, len(opts.Tools))
	for _, schema := range opts.Tools {
		names = append(names, strings.TrimSpace(schema.Function.Name))
	}
	c.seenTools = append(c.seenTools, names)

	if len(c.responses) == 0 {
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: ""}}, nil
	}
	idx := c.callCount
	if idx >= len(c.responses) {
		idx = len(c.responses) - 1
	}
	c.callCount++
	return c.responses[idx], nil
}
