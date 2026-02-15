package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	zlog "github.com/rs/zerolog/log"
)

const defaultCodexCLIModel = "gpt-5.3-codex"

type CodexCLIClient struct {
	command string
	model   string
	config  ClientConfig
}

func NewCodexCLIClient(model string) (*CodexCLIClient, error) {
	return newCodexCLIClientWithConfig(model, DefaultClientConfig())
}

func newCodexCLIClientWithConfig(model string, config ClientConfig) (*CodexCLIClient, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("codex-cli model is required")
	}

	command := strings.TrimSpace(os.Getenv("CODEX_CLI_BIN"))
	if command == "" {
		command = "codex"
	}

	return &CodexCLIClient{
		command: command,
		model:   model,
		config:  config,
	}, nil
}

func (c *CodexCLIClient) Ask(ctx context.Context, prompt string) (string, error) {
	start := time.Now()
	outFile, err := os.CreateTemp("", "tars-codex-output-*.txt")
	if err != nil {
		return "", newProviderError("codex-cli", "exec", fmt.Errorf("create codex output file: %w", err))
	}
	outPath := outFile.Name()
	if err := outFile.Close(); err != nil {
		return "", newProviderError("codex-cli", "exec", fmt.Errorf("close codex output file: %w", err))
	}
	defer os.Remove(outPath)

	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--color", "never",
		"--sandbox", "read-only",
		"--output-last-message", outPath,
		"--model", c.model,
		"-",
	}
	zlog.Debug().
		Str("provider", "codex-cli").
		Str("command", c.command).
		Str("model", c.model).
		Int("prompt_len", len(prompt)).
		Str("prompt_preview", truncateForLog(strings.TrimSpace(prompt), 240)).
		Msg("llm request start")
	cmd := exec.CommandContext(ctx, c.command, args...)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = "no additional error output"
		}
		perr := newProviderError("codex-cli", "exec", fmt.Errorf("codex exec failed: %w: %s", err, msg))
		perr.Message = msg
		return "", perr
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		return "", newProviderError("codex-cli", "exec", fmt.Errorf("read codex output file: %w", err))
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", newProviderError("codex-cli", "parse", fmt.Errorf("codex exec returned empty response"))
	}
	zlog.Debug().
		Str("provider", "codex-cli").
		Int("assistant_len", len(text)).
		Str("assistant_preview", truncateForLog(text, 240)).
		Dur("latency", time.Since(start)).
		Msg("llm response complete")
	return text, nil
}

func (c *CodexCLIClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	if len(opts.Tools) > 0 || strings.TrimSpace(opts.ToolChoice) != "" {
		zlog.Warn().
			Str("provider", "codex-cli").
			Int("tool_count", len(opts.Tools)).
			Str("tool_choice", strings.TrimSpace(opts.ToolChoice)).
			Msg("tool-calls unsupported by codex-cli provider")
		return ChatResponse{}, newProviderError(
			"codex-cli",
			"request",
			fmt.Errorf("tool calls are not supported by codex-cli provider"),
		)
	}
	zlog.Debug().Str("provider", "codex-cli").Int("message_count", len(messages)).Msg("llm chat prepare prompt")

	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}
	prompt := strings.Join(lines, "\n")

	resp, err := c.Ask(ctx, prompt)
	if err != nil {
		return ChatResponse{}, err
	}

	return ChatResponse{
		Message: ChatMessage{
			Role:    "assistant",
			Content: resp,
		},
		Usage:      Usage{},
		StopReason: "",
	}, nil
}
