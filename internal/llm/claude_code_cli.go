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
	return askFromSinglePrompt(ctx, c.Chat, prompt, strings.TrimSpace)
}

func (c *ClaudeCodeCLIClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	if c == nil {
		return ChatResponse{}, fmt.Errorf("%s client is not configured", claudeCodeCLIProviderLabel)
	}
	resumeID := strings.TrimSpace(opts.ResumeSessionID)
	var prompt string
	if resumeID != "" {
		// Claude Code already holds the prior context under <session_id>;
		// passing the full transcript again would double up tokens and may
		// confuse the saved state. Send only the latest user turn.
		prompt = extractLatestUserMessage(messages)
	} else {
		prompt = buildClaudeCodeCLIPrompt(messages)
	}
	if prompt == "" {
		return ChatResponse{}, fmt.Errorf("%s prompt is empty", claudeCodeCLIProviderLabel)
	}

	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", resolveClaudeCodePermissionMode(opts.ClaudeCodePermissionMode),
		"--model", c.model,
		"--add-dir", c.workDir,
	}
	if resumeID != "" {
		// --resume requires session-persistence to be enabled so Claude Code
		// can actually load the saved transcript from disk.
		args = append(args, "--resume", resumeID)
	} else {
		args = append(args, "--no-session-persistence")
	}
	if mcpPath, cleanup, err := writeClaudeCodeMCPConfigFile(opts.ClaudeCodeMCPServers); err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("mcp config: %w", err))
	} else if mcpPath != "" {
		defer cleanup()
		args = append(args, "--mcp-config", mcpPath)
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
		toolCalls     []ToolCall
		resultText    string
		usage         Usage
		stopReason    string
		sessionID     string
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

		if sid := strings.TrimSpace(asString(payload["session_id"])); sid != "" {
			sessionID = sid
		}

		switch strings.TrimSpace(asString(payload["type"])) {
		case "system":
			// session_id already captured above; nothing else to do for init.
		case "assistant":
			text, calls := extractClaudeCodeAssistantBlocks(payload)
			if text != "" {
				if assistantText.Len() > 0 {
					assistantText.WriteString("\n")
				}
				assistantText.WriteString(text)
				if opts.OnDelta != nil {
					opts.OnDelta(text)
				}
			}
			toolCalls = append(toolCalls, calls...)
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
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		},
		Usage:      usage,
		StopReason: stopReason,
		SessionID:  sessionID,
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

// resolveClaudeCodePermissionMode validates the caller-supplied permission
// mode against Claude Code's recognized values and falls back to "auto" for
// empty or unknown input. Keeping the fallback inside the provider means the
// provider stays usable even if a caller wires through a value Claude Code
// adds (or removes) in a future release: invalid input degrades gracefully
// instead of failing the whole turn.
func resolveClaudeCodePermissionMode(raw string) string {
	switch strings.TrimSpace(raw) {
	case "acceptEdits", "plan", "bypassPermissions", "auto":
		return strings.TrimSpace(raw)
	default:
		return "auto"
	}
}

// writeClaudeCodeMCPConfigFile materializes a Claude Code-shaped MCP config
// (`{"mcpServers": {name: {...}}}`) into a temp file and returns its path
// plus a cleanup function. Returns an empty path when servers is empty —
// callers should skip the --mcp-config flag in that case.
//
// stdio servers: type/command/args/env
// remote servers: type/url/headers ("http" or "sse" transport)
//
// Entries with empty Name are silently skipped; this avoids producing an
// invalid JSON object with an empty key.
func writeClaudeCodeMCPConfigFile(servers []ClaudeCodeMCPServer) (string, func(), error) {
	entries := make(map[string]map[string]any, len(servers))
	for _, srv := range servers {
		name := strings.TrimSpace(srv.Name)
		if name == "" {
			continue
		}
		entry := map[string]any{}
		transport := strings.ToLower(strings.TrimSpace(srv.Transport))
		switch transport {
		case "http", "sse":
			entry["type"] = transport
			if url := strings.TrimSpace(srv.URL); url != "" {
				entry["url"] = url
			}
			if len(srv.Headers) > 0 {
				entry["headers"] = srv.Headers
			}
		default:
			entry["type"] = "stdio"
			if cmd := strings.TrimSpace(srv.Command); cmd != "" {
				entry["command"] = cmd
			}
			if len(srv.Args) > 0 {
				entry["args"] = srv.Args
			}
			if len(srv.Env) > 0 {
				entry["env"] = srv.Env
			}
		}
		entries[name] = entry
	}
	if len(entries) == 0 {
		return "", func() {}, nil
	}
	payload := map[string]any{"mcpServers": entries}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", func() {}, fmt.Errorf("encode mcp config: %w", err)
	}
	f, err := os.CreateTemp("", "tars-claude-mcp-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp file: %w", err)
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.Write(encoded); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close temp file: %w", err)
	}
	return path, cleanup, nil
}

// extractLatestUserMessage returns the trimmed Content of the final user
// message in the slice. Used in resume mode to avoid re-sending history that
// the upstream session already has.
func extractLatestUserMessage(messages []ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(strings.ToLower(messages[i].Role)) != "user" {
			continue
		}
		if trimmed := strings.TrimSpace(messages[i].Content); trimmed != "" {
			return trimmed
		}
	}
	return ""
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

// extractClaudeCodeAssistantBlocks splits an assistant stream-json event into
// its concatenated text and any tool_use blocks. tool_use input is preserved
// as a JSON-encoded argument string so TARS callers can route it the same way
// they handle ToolCall.Arguments from other providers.
func extractClaudeCodeAssistantBlocks(payload map[string]any) (string, []ToolCall) {
	message, ok := payload["message"].(map[string]any)
	if !ok {
		return "", nil
	}
	blocks, ok := message["content"].([]any)
	if !ok {
		return "", nil
	}
	var (
		text  strings.Builder
		calls []ToolCall
	)
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch strings.TrimSpace(asString(block["type"])) {
		case "text":
			text.WriteString(asString(block["text"]))
		case "tool_use":
			call := ToolCall{
				ID:   strings.TrimSpace(asString(block["id"])),
				Name: strings.TrimSpace(asString(block["name"])),
			}
			if input, ok := block["input"]; ok && input != nil {
				if encoded, err := json.Marshal(input); err == nil {
					call.Arguments = string(encoded)
				}
			}
			if call.Name != "" {
				calls = append(calls, call)
			}
		}
	}
	return text.String(), calls
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
