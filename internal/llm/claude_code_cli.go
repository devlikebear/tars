package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	claudeCodeCLIProviderLabel = "claude-code-cli"
	defaultClaudeCodeCLIModel  = "sonnet"
	claudeCodeCLIPathEnv       = "CLAUDE_CODE_CLI_PATH"
)

type ClaudeCodeCLIClient struct {
	cliPath string
	workDir string
	model   string
}

func FindClaudeCodeCLIPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(claudeCodeCLIPathEnv)); configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("%s executable not found: %s", claudeCodeCLIProviderLabel, configured)
		}
		return path, nil
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("%s executable not found in PATH; install Claude Code or set %s", claudeCodeCLIProviderLabel, claudeCodeCLIPathEnv)
	}
	return path, nil
}

func NewClaudeCodeCLIClient(workDir, model string) (*ClaudeCodeCLIClient, error) {
	cliPath, err := FindClaudeCodeCLIPath()
	if err != nil {
		return nil, err
	}
	trimmedWorkDir := strings.TrimSpace(workDir)
	if trimmedWorkDir == "" {
		trimmedWorkDir = "."
	}
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		trimmedModel = defaultClaudeCodeCLIModel
	}
	return &ClaudeCodeCLIClient{
		cliPath: cliPath,
		workDir: trimmedWorkDir,
		model:   trimmedModel,
	}, nil
}

func (c *ClaudeCodeCLIClient) Ask(ctx context.Context, prompt string) (string, error) {
	resp, err := c.Chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, ChatOptions{})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Message.Content), nil
}

func (c *ClaudeCodeCLIClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	if c == nil {
		return ChatResponse{}, fmt.Errorf("%s client is not configured", claudeCodeCLIProviderLabel)
	}
	prompt := buildClaudeCodeCLIPrompt(messages)
	if prompt == "" {
		return ChatResponse{}, fmt.Errorf("%s prompt is empty", claudeCodeCLIProviderLabel)
	}

	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "auto",
		"--model", c.model,
		"--add-dir", c.workDir,
		"--no-session-persistence",
	}
	if systemPrompt := buildClaudeCodeCLISystemPrompt(messages); systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, c.cliPath, args...)
	cmd.Dir = c.workDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("stdout pipe: %w", err))
	}
	if err := cmd.Start(); err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("start cli: %w", err))
	}

	resp, parseErr := parseClaudeCodeCLIStream(stdout, opts)
	waitErr := cmd.Wait()
	if parseErr != nil {
		return ChatResponse{}, parseErr
	}
	if waitErr != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("cli failed: %w: %s", waitErr, errText))
		}
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("cli failed: %w", waitErr))
	}
	return resp, nil
}

func parseClaudeCodeCLIStream(stdout io.Reader, opts ChatOptions) (ChatResponse, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		assistantText strings.Builder
		resultText    string
		usage         Usage
		stopReason    string
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "parse", fmt.Errorf("decode stream event: %w", err))
		}

		switch strings.TrimSpace(asString(payload["type"])) {
		case "assistant":
			text := extractClaudeCodeAssistantText(payload)
			if text == "" {
				continue
			}
			if assistantText.Len() > 0 {
				assistantText.WriteString("\n")
			}
			assistantText.WriteString(text)
			if opts.OnDelta != nil {
				opts.OnDelta(text)
			}
		case "result":
			stopReason = strings.TrimSpace(asString(payload["stop_reason"]))
			usage = extractClaudeCodeUsage(payload["usage"])
			resultText = strings.TrimSpace(asString(payload["result"]))
			if asBool(payload["is_error"]) {
				errText := firstNonEmptyTrimmed(resultText, fmt.Sprintf("%s request failed", claudeCodeCLIProviderLabel))
				return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("%s", errText))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "stream", fmt.Errorf("read stream response: %w", err))
	}

	content := strings.TrimSpace(assistantText.String())
	if content == "" {
		content = resultText
	}
	return ChatResponse{
		Message: ChatMessage{
			Role:    "assistant",
			Content: content,
		},
		Usage:      usage,
		StopReason: stopReason,
	}, nil
}

func buildClaudeCodeCLISystemPrompt(messages []ChatMessage) string {
	parts := []string{
		"You are Claude Code running inside TARS.",
		"Ignore any tool-call JSON conventions from upstream prompts and use Claude Code's own local tools when useful.",
		"Return the final answer as plain text.",
	}
	for _, msg := range messages {
		if strings.TrimSpace(strings.ToLower(msg.Role)) != "system" {
			continue
		}
		if trimmed := strings.TrimSpace(msg.Content); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildClaudeCodeCLIPrompt(messages []ChatMessage) string {
	var builder strings.Builder
	builder.WriteString("Continue the conversation below and respond to the latest user request.\n\n")
	for _, msg := range messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role == "" || role == "system" {
			continue
		}
		builder.WriteString(strings.ToUpper(role))
		builder.WriteString(":\n")
		if text := strings.TrimSpace(msg.Content); text != "" {
			builder.WriteString(text)
			builder.WriteString("\n")
		}
		if len(msg.ToolCalls) > 0 {
			builder.WriteString("Tool calls:\n")
			for _, call := range msg.ToolCalls {
				builder.WriteString("- ")
				builder.WriteString(strings.TrimSpace(call.Name))
				if args := strings.TrimSpace(call.Arguments); args != "" {
					builder.WriteString(" ")
					builder.WriteString(args)
				}
				builder.WriteString("\n")
			}
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func extractClaudeCodeAssistantText(payload map[string]any) string {
	message, ok := payload["message"].(map[string]any)
	if !ok {
		return ""
	}
	blocks, ok := message["content"].([]any)
	if !ok {
		return ""
	}
	var builder strings.Builder
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(asString(block["type"])) != "text" {
			continue
		}
		builder.WriteString(asString(block["text"]))
	}
	return builder.String()
}

func extractClaudeCodeUsage(raw any) Usage {
	usageMap, ok := raw.(map[string]any)
	if !ok {
		return Usage{}
	}
	return Usage{
		InputTokens:      asInt(usageMap["input_tokens"], usageMap["inputTokens"]),
		OutputTokens:     asInt(usageMap["output_tokens"], usageMap["outputTokens"]),
		CachedTokens:     asInt(usageMap["cached_tokens"], usageMap["cachedTokens"]),
		CacheReadTokens:  asInt(usageMap["cache_read_input_tokens"], usageMap["cacheReadInputTokens"], usageMap["cache_read_tokens"], usageMap["cacheReadTokens"]),
		CacheWriteTokens: asInt(usageMap["cache_creation_input_tokens"], usageMap["cacheCreationInputTokens"], usageMap["cache_write_tokens"], usageMap["cacheWriteTokens"]),
	}
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func asBool(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func asInt(values ...any) int {
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			return typed
		case int32:
			return int(typed)
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case json.Number:
			if n, err := typed.Int64(); err == nil {
				return int(n)
			}
		}
	}
	return 0
}
