package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const defaultCodexCLIModel = "gpt-5.3-codex"

type CodexCLIClient struct {
	command string
	model   string
}

func NewCodexCLIClient(model string) (*CodexCLIClient, error) {
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
	}, nil
}

func (c *CodexCLIClient) Ask(ctx context.Context, prompt string) (string, error) {
	outFile, err := os.CreateTemp("", "tars-codex-output-*.txt")
	if err != nil {
		return "", fmt.Errorf("create codex output file: %w", err)
	}
	outPath := outFile.Name()
	if err := outFile.Close(); err != nil {
		return "", fmt.Errorf("close codex output file: %w", err)
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
		return "", fmt.Errorf("codex exec failed: %w: %s", err, msg)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		return "", fmt.Errorf("read codex output file: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("codex exec returned empty response")
	}
	return text, nil
}
