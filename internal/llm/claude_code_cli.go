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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	claudeCodeCLIProviderLabel = "claude-code-cli"
	defaultClaudeCodeCLIModel  = "sonnet"
	claudeCodeCLIPathEnv       = "CLAUDE_CODE_CLI_PATH"
	claudeCodeCLITimeoutEnv    = "CLAUDE_CODE_CLI_TIMEOUT"
	// defaultClaudeCodeCLITimeout bounds a single claude invocation. Real
	// agentic turns can legitimately run for tens of seconds to minutes, so
	// the default is generous; operators tune it via CLAUDE_CODE_CLI_TIMEOUT.
	defaultClaudeCodeCLITimeout = 5 * time.Minute
)

// claudeCodeCLIPerfEnv holds low-risk environment toggles that cut Claude
// Code's per-invocation startup cost (autoupdater check, telemetry, error
// reporting, and other nonessential network traffic). Measured savings are
// roughly ~1s per call. These are applied as defaults only: a value already
// present in the inherited environment is left untouched so operators keep the
// final say. Order is fixed for deterministic process environments.
var claudeCodeCLIPerfEnv = []struct{ key, value string }{
	{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1"},
	{"DISABLE_AUTOUPDATER", "1"},
	{"DISABLE_TELEMETRY", "1"},
	{"DISABLE_ERROR_REPORTING", "1"},
}

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
	if pluginDir, cleanup, err := writeClaudeCodeSkillsPluginDir(opts.ClaudeCodeSkills); err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("skills plugin: %w", err))
	} else if pluginDir != "" {
		defer cleanup()
		args = append(args, "--plugin-dir", pluginDir)
	}
	if settingsPath, cleanup, err := writeClaudeCodeSettingsFile(opts.ClaudeCodePermissionDeny); err != nil {
		return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("settings: %w", err))
	} else if settingsPath != "" {
		defer cleanup()
		args = append(args, "--settings", settingsPath)
	}
	if systemPrompt := buildClaudeCodeCLISystemPrompt(messages); systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}
	args = append(args, prompt)

	// Bound the invocation so a hung or slow claude process fails predictably
	// instead of blocking until some upstream client gives up.
	timeout := parseClaudeCodeCLITimeout(os.Getenv(claudeCodeCLITimeoutEnv))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	env := claudeCodeCLIEnv(os.Environ())

	attempt := func() (ChatResponse, error) {
		cmd := exec.CommandContext(ctx, c.cliPath, args...)
		cmd.Dir = c.workDir
		cmd.Env = env
		// Run claude in its own process group and, on context cancellation,
		// kill the whole group. claude spawns descendants (e.g. stdio MCP
		// servers) that inherit the stdout pipe; killing only the direct
		// child leaves them holding the pipe open, so the stream read would
		// block past the deadline. WaitDelay bounds any residual pipe wait.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		cmd.WaitDelay = 5 * time.Second
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
		// A deadline kill surfaces as a process/stream error; report it as a
		// timeout so callers (and the retry policy) can tell it apart from a
		// transient crash.
		if ctx.Err() == context.DeadlineExceeded {
			return ChatResponse{}, newProviderError(claudeCodeCLIProviderLabel, "request", fmt.Errorf("cli timed out after %s", timeout))
		}
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

	return runClaudeCodeCLIWithRetry(ctx, attempt)
}

// runClaudeCodeCLIWithRetry runs attempt once and retries a single time on a
// transient failure. A failure is NOT transient — and so is not retried —
// when the context is done (a timeout or caller cancellation), to avoid
// doubling an already-long wait.
//
// Caveat: in streaming mode a retried attempt re-emits deltas. Retries only
// fire on a transient failure with the context still live, which is rare, so
// this is accepted in exchange for recovering from one-off process crashes.
func runClaudeCodeCLIWithRetry(ctx context.Context, attempt func() (ChatResponse, error)) (ChatResponse, error) {
	resp, err := attempt()
	if err == nil || ctx.Err() != nil {
		return resp, err
	}
	return attempt()
}

// claudeCodeCLIEnv returns base augmented with the claudeCodeCLIPerfEnv
// defaults for any key not already present in base. base is typically
// os.Environ(); existing values win so callers can override the toggles.
func claudeCodeCLIEnv(base []string) []string {
	present := make(map[string]struct{}, len(base))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i > 0 {
			present[kv[:i]] = struct{}{}
		}
	}
	out := append([]string(nil), base...)
	for _, def := range claudeCodeCLIPerfEnv {
		if _, ok := present[def.key]; ok {
			continue
		}
		out = append(out, def.key+"="+def.value)
	}
	return out
}

// parseClaudeCodeCLITimeout interprets a CLAUDE_CODE_CLI_TIMEOUT value. It
// accepts a Go duration string ("300s", "5m") or a bare integer treated as
// seconds. Empty, zero, negative, or unparseable input yields the default.
func parseClaudeCodeCLITimeout(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultClaudeCodeCLITimeout
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d > 0 {
			return d
		}
		return defaultClaudeCodeCLITimeout
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return defaultClaudeCodeCLITimeout
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
			Role: "assistant",
			// Claude Code self-executed any tool_use blocks inside its own
			// subprocess; surfacing them as Message.ToolCalls would make the
			// agent loop re-dispatch them through TARS' registry. The audit
			// trail lives on ProviderExecutedTools (below).
			Content:   content,
			ToolCalls: nil,
		},
		Usage:                 usage,
		StopReason:            stopReason,
		SessionID:             sessionID,
		ProviderExecutedTools: toolCalls,
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

// writeClaudeCodeSettingsFile materializes the smallest possible Claude Code
// settings document — exactly `{"permissions":{"deny":[...]}}` — into a temp
// file and returns its path plus a cleanup function. Returns an empty path
// when there is no usable deny rule so the caller skips the --settings flag.
//
// This is intentionally NOT a generic settings passthrough. The function
// accepts only a deny list and emits a fixed two-key shape; there is no code
// path here that can write `env`, `hooks`, `apiKeyHelper`, `model`, or any
// other key. That makes the credential / arbitrary-binary threat model a
// schema-level guarantee rather than a runtime filter: even adversarial
// session-override input can only ever add more deny rules (tightening what
// Claude Code's self-executed tools may do), never widen authority.
//
// Entries are trimmed; blank entries are dropped and duplicates are removed
// while preserving first-seen order so the emitted document is stable.
func writeClaudeCodeSettingsFile(deny []string) (string, func(), error) {
	seen := make(map[string]struct{}, len(deny))
	rules := make([]string, 0, len(deny))
	for _, d := range deny {
		rule := strings.TrimSpace(d)
		if rule == "" {
			continue
		}
		if _, dup := seen[rule]; dup {
			continue
		}
		seen[rule] = struct{}{}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return "", func() {}, nil
	}
	payload := map[string]any{"permissions": map[string]any{"deny": rules}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", func() {}, fmt.Errorf("encode settings: %w", err)
	}
	f, err := os.CreateTemp("", "tars-claude-settings-*.json")
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

// claudeCodeSkillDirName sanitizes a TARS skill name into a Claude Code
// plugin skill directory name: lowercase, only [a-z0-9-], collapsed dashes,
// trimmed. Returns "" when nothing usable remains so the caller can skip the
// entry rather than emit a malformed directory.
func claudeCodeSkillDirName(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// writeClaudeCodeSkillsPluginDir materializes the supplied skills into a
// temporary Claude Code plugin directory and returns its path plus a cleanup
// function. Returns an empty path when there are no usable skills so the
// caller skips the --plugin-dir flag.
//
// Layout (verified against claude 2.1.142):
//
//	<tmp>/.claude-plugin/plugin.json     {"name":"tars-skills",...}
//	<tmp>/skills/<dir>/SKILL.md          frontmatter(name,description) + body
//
// Claude Code loads this as a session-only plugin; the skills surface as
// `tars-skills:<name>` in its slash-command / skill registry. Names that
// sanitize to empty, or collide after sanitization, are skipped (first writer
// wins) so we never emit a broken plugin.
func writeClaudeCodeSkillsPluginDir(skills []ClaudeCodeSkill) (string, func(), error) {
	type prepared struct {
		dir         string
		name        string
		description string
		content     string
	}
	var entries []prepared
	seen := map[string]struct{}{}
	for _, sk := range skills {
		name := strings.TrimSpace(sk.Name)
		if name == "" {
			continue
		}
		dir := claudeCodeSkillDirName(name)
		if dir == "" {
			continue
		}
		if _, dup := seen[dir]; dup {
			continue
		}
		seen[dir] = struct{}{}
		entries = append(entries, prepared{
			dir:         dir,
			name:        name,
			description: strings.TrimSpace(sk.Description),
			content:     sk.Content,
		})
	}
	if len(entries) == 0 {
		return "", func() {}, nil
	}

	root, err := os.MkdirTemp("", "tars-claude-skills-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }

	manifestDir := filepath.Join(root, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("mkdir manifest: %w", err)
	}
	manifest := map[string]any{
		"name":        "tars-skills",
		"version":     "0.0.1",
		"description": "TARS session skill catalog (ephemeral, regenerated per call)",
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), manifestBytes, 0o644); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("write manifest: %w", err)
	}

	for _, e := range entries {
		skillDir := filepath.Join(root, "skills", e.dir)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("mkdir skill %q: %w", e.dir, err)
		}
		var doc strings.Builder
		doc.WriteString("---\n")
		doc.WriteString("name: ")
		doc.WriteString(e.name)
		doc.WriteString("\n")
		if e.description != "" {
			doc.WriteString("description: ")
			// Keep description single-line; YAML scalar safety for the few
			// chars that would break a bare scalar.
			doc.WriteString(strings.ReplaceAll(e.description, "\n", " "))
			doc.WriteString("\n")
		}
		doc.WriteString("---\n\n")
		doc.WriteString(e.content)
		if !strings.HasSuffix(e.content, "\n") {
			doc.WriteString("\n")
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(doc.String()), 0o644); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("write skill %q: %w", e.dir, err)
		}
	}
	return root, cleanup, nil
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
