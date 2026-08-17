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
	"strconv"
	"strings"
	"time"
)

// antigravity-cli provider: reuses a locally installed `agy` the same way
// claude-code-cli reuses `claude`. Google cut Gemini Code Assist off for
// individual accounts on 2026-06-18 — which took the Gemini CLI's Google
// sign-in with it — and points those users at the Antigravity family instead,
// so this is the CLI-backed Google provider that still works without an API
// key. `agy` authenticates through the system keyring, falling back to a
// browser sign-in, so TARS never handles the credential.
//
// Wire protocol: `agy --print <text> --output-format stream-json` emits
// newline-delimited JSON on stdout. Events are *nested* under a key named by
// the `event` discriminator:
//
//	{"event":"init","conversation_id":"…","init":{…}}
//	{"event":"step_update","step_update":{"step_type":"agent_response","text_delta":"…",…}}
//	{"event":"result","result":{"status":"SUCCESS","response":"…","usage":{…},…}}
const (
	antigravityCLIProviderLabel = "antigravity-cli"
	antigravityCLIPathEnv       = "AGY_CLI_PATH"
	antigravityCLITimeoutEnv    = "AGY_CLI_TIMEOUT"
	// antigravityCLIModeEnv overrides --mode. It is an env var rather than a
	// ChatOptions field because nothing in TARS' config surface sets it yet;
	// when the config surface grows one, thread it through ChatOptions like
	// ClaudeCodePermissionMode and keep this as the fallback.
	antigravityCLIModeEnv = "AGY_CLI_MODE"
	// defaultAntigravityCLITimeout bounds a single agy invocation from the
	// outside. `agy` has its own --print-timeout (5m by default) which is set
	// to match; this outer bound is what still fires if the process wedges
	// before its own timer arms.
	defaultAntigravityCLITimeout = 5 * time.Minute
	// antigravityCLIWaitDelay bounds how long Wait blocks on pipe I/O after
	// the process is signaled on cancellation. Shared by the platform-specific
	// process configuration.
	antigravityCLIWaitDelay = 5 * time.Second
	// Tool output is embedded in one NDJSON line. The scanner default (64 KiB)
	// is too small for ordinary build/test logs, while an explicit ceiling keeps
	// a corrupt or hostile subprocess from growing memory without bound.
	maxAntigravityCLIEventBytes = 16 * 1024 * 1024
)

// antigravityCLIPerfEnv holds low-risk environment defaults. Applied as
// defaults only: a value already present in the inherited environment is left
// untouched so operators keep the final say. Order is fixed for deterministic
// process environments.
var antigravityCLIPerfEnv = []struct{ key, value string }{
	{"NO_COLOR", "1"},
}

type AntigravityCLIClient struct {
	cliPath        string
	workDir        string
	model          string
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// FindAntigravityCLIPath resolves the agy executable, honouring AGY_CLI_PATH
// before falling back to a PATH lookup.
func FindAntigravityCLIPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(antigravityCLIPathEnv)); configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("%s executable not found: %s", antigravityCLIProviderLabel, configured)
		}
		return path, nil
	}
	path, err := exec.LookPath("agy")
	if err != nil {
		return "", fmt.Errorf("%s executable not found in PATH; install Antigravity CLI or set %s", antigravityCLIProviderLabel, antigravityCLIPathEnv)
	}
	return path, nil
}

// NewAntigravityCLIClient builds a client. An empty model is left unset so the
// CLI picks its own default — unlike Claude Code there is no stable short
// alias to hardcode, and a wrong --model value fails the whole turn.
func NewAntigravityCLIClient(workDir, model string) (*AntigravityCLIClient, error) {
	cliPath, err := FindAntigravityCLIPath()
	if err != nil {
		return nil, err
	}
	trimmedWorkDir := strings.TrimSpace(workDir)
	if trimmedWorkDir == "" {
		trimmedWorkDir = "."
	}
	return &AntigravityCLIClient{
		cliPath:        cliPath,
		workDir:        trimmedWorkDir,
		model:          strings.TrimSpace(model),
		commandContext: exec.CommandContext,
	}, nil
}

func (c *AntigravityCLIClient) Ask(ctx context.Context, prompt string) (string, error) {
	return askFromSinglePrompt(ctx, c.Chat, prompt, strings.TrimSpace)
}

func (c *AntigravityCLIClient) Chat(ctx context.Context, messages []ChatMessage, opts ChatOptions) (ChatResponse, error) {
	if c == nil {
		return ChatResponse{}, fmt.Errorf("%s client is not configured", antigravityCLIProviderLabel)
	}
	resumeID := strings.TrimSpace(opts.ResumeSessionID)
	// The transcript is assembled before the system block so a turn with no
	// user or assistant content is caught here. Folding the system block in
	// first would mask it: the preamble is never empty.
	var transcript string
	if resumeID != "" {
		// The CLI holds the prior context under <conversation_id>; replaying
		// the transcript would re-bill it and can confuse the saved state.
		transcript = extractLatestUserMessage(messages)
	} else {
		transcript = buildAntigravityCLITranscript(messages)
	}
	if transcript == "" {
		return ChatResponse{}, fmt.Errorf("%s prompt is empty", antigravityCLIProviderLabel)
	}
	// On a resumed turn the system block already reached the model and lives
	// in the saved conversation, so re-sending it would duplicate it.
	systemPrompt := ""
	if resumeID == "" {
		systemPrompt = collectAntigravityCLISystemPrompt(messages)
	}
	prompt := composeAntigravityCLIPrompt(systemPrompt, transcript)

	timeout := parseAntigravityCLITimeout(os.Getenv(antigravityCLITimeoutEnv))
	args := buildAntigravityCLIArgs(antigravityCLIArgSpec{
		Model:           c.model,
		Prompt:          prompt,
		ConversationID:  resumeID,
		Mode:            os.Getenv(antigravityCLIModeEnv),
		ReasoningEffort: opts.ReasoningEffort,
		JSONSchema:      antigravityCLIJSONSchema(opts.ResponseFormat),
		PrintTimeout:    timeout,
	})

	// Bound the invocation from the outside too, so a process that wedges
	// before arming its own --print-timeout still fails predictably.
	ctx, cancel := context.WithTimeout(ctx, timeout+antigravityCLIWaitDelay)
	defer cancel()

	cmd := c.commandContext(ctx, c.cliPath, args...)
	cmd.Dir = c.workDir
	cmd.Env = antigravityCLIEnv(os.Environ())
	// On cancellation kill the whole descendant tree, not just the direct
	// child: the CLI spawns descendants that inherit the stdout pipe, and a
	// survivor holds it open so the stream read would block past the deadline.
	configureAntigravityCLIProcess(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("stdout pipe: %w", err))
	}
	if err := cmd.Start(); err != nil {
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("start cli: %w", err))
	}

	resp, parseErr := parseAntigravityCLIStream(stdout, opts)
	waitErr := cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("cli timed out after %s", timeout))
	}
	if ctx.Err() != nil {
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("cli canceled: %w", ctx.Err()))
	}
	if parseErr != nil {
		return ChatResponse{}, parseErr
	}
	if waitErr != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText != "" {
			return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("cli failed: %w: %s", waitErr, errText))
		}
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("cli failed: %w", waitErr))
	}
	return resp, nil
}

// antigravityCLIArgSpec is the full set of inputs that shape one invocation.
// Grouping them keeps buildAntigravityCLIArgs testable without a live process
// and stops the argument order from drifting between call sites.
type antigravityCLIArgSpec struct {
	Model           string
	Prompt          string
	ConversationID  string
	Mode            string
	ReasoningEffort string
	JSONSchema      string
	PrintTimeout    time.Duration
}

// buildAntigravityCLIArgs assembles the headless invocation. The prompt goes
// last so a prompt that happens to look like a flag cannot be parsed as one.
func buildAntigravityCLIArgs(spec antigravityCLIArgSpec) []string {
	args := []string{"--output-format", "stream-json"}
	if spec.PrintTimeout > 0 {
		args = append(args, "--print-timeout", spec.PrintTimeout.String())
	}
	// An empty model is omitted rather than guessed: the CLI's own default is
	// correct, and an invalid slug fails the turn.
	if model := strings.TrimSpace(spec.Model); model != "" {
		args = append(args, "--model", model)
	}
	if mode := resolveAntigravityCLIMode(spec.Mode); mode != "" {
		args = append(args, "--mode", mode)
	}
	if effort := resolveAntigravityCLIEffort(spec.ReasoningEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	if schema := strings.TrimSpace(spec.JSONSchema); schema != "" {
		args = append(args, "--json-schema", schema)
	}
	if id := strings.TrimSpace(spec.ConversationID); id != "" {
		args = append(args, "--conversation", id)
	}
	return append(args, "--print", spec.Prompt)
}

// resolveAntigravityCLIMode validates the requested mode against the values
// the CLI accepts and returns "" for anything else, which omits the flag and
// leaves the CLI's own default (permission requests are reviewed) in place.
// `--dangerously-skip-permissions` is deliberately unreachable from here: no
// value of an env var should silently auto-approve every tool call.
func resolveAntigravityCLIMode(raw string) string {
	switch trimmed := strings.ToLower(strings.TrimSpace(raw)); trimmed {
	case "accept-edits", "plan":
		return trimmed
	default:
		return ""
	}
}

// resolveAntigravityCLIEffort maps TARS' provider-agnostic reasoning effort
// onto the three levels the CLI accepts. Unsupported values omit the flag
// rather than failing, matching how the other providers treat the field.
func resolveAntigravityCLIEffort(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "low", "minimal", "min", "none":
		return "low"
	case "medium", "med":
		return "medium"
	case "high", "veryhigh", "very-high", "very_high", "xhigh":
		return "high"
	default:
		return ""
	}
}

// antigravityCLIJSONSchema extracts a schema string for --json-schema. Only
// json_schema response formats map onto the flag; per the CLI's own help text
// it constrains the final result only, so json_object (no schema) has nothing
// to pass and is ignored.
func antigravityCLIJSONSchema(format *ResponseFormat) string {
	if format == nil || format.Type != ResponseFormatJSONSchema {
		return ""
	}
	return strings.TrimSpace(string(format.Schema))
}

// composeAntigravityCLIPrompt joins the system block and the transcript into
// the single string the CLI accepts. Like Gemini CLI, `agy` has no
// system-prompt flag, so system content has to travel in the prompt or it is
// silently lost.
func composeAntigravityCLIPrompt(systemPrompt, transcript string) string {
	if strings.TrimSpace(systemPrompt) == "" {
		return transcript
	}
	return systemPrompt + "\n\n" + transcript
}

// buildAntigravityCLITranscript flattens the conversation, excluding system
// messages. It returns "" when there is nothing for the model to answer, which
// is what lets Chat reject an empty turn.
func buildAntigravityCLITranscript(messages []ChatMessage) string {
	var builder strings.Builder
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

func collectAntigravityCLISystemPrompt(messages []ChatMessage) string {
	parts := []string{
		"You are Antigravity CLI running inside TARS.",
		"Ignore any tool-call JSON conventions from upstream prompts and use your own local tools when useful.",
		"Return only the requested final answer.",
		"Continue the conversation below and respond to the latest user request.",
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

// antigravityCLIEnv returns base augmented with the perf defaults for any key
// not already present. base is typically os.Environ(); existing values win so
// callers can override the toggles.
func antigravityCLIEnv(base []string) []string {
	present := make(map[string]struct{}, len(base))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i > 0 {
			present[kv[:i]] = struct{}{}
		}
	}
	out := append([]string(nil), base...)
	for _, def := range antigravityCLIPerfEnv {
		if _, ok := present[def.key]; ok {
			continue
		}
		out = append(out, def.key+"="+def.value)
	}
	return out
}

// parseAntigravityCLITimeout interprets an AGY_CLI_TIMEOUT value. It accepts a
// Go duration string ("300s", "5m") or a bare integer treated as seconds.
// Empty, zero, negative, or unparseable input yields the default.
func parseAntigravityCLITimeout(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultAntigravityCLITimeout
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d > 0 {
			return d
		}
		return defaultAntigravityCLITimeout
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return defaultAntigravityCLITimeout
}

// parseAntigravityCLIStream consumes the nested NDJSON event stream.
//
// Assistant text arrives as `text_delta` fragments on agent_response steps and
// again, complete, as `result.response`. The result is authoritative — the
// fragments are only forwarded to OnDelta — so streaming and non-streaming
// callers see the same final content and nothing is double-counted.
func parseAntigravityCLIStream(stdout io.Reader, opts ChatOptions) (ChatResponse, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxAntigravityCLIEventBytes)

	var (
		deltaText      strings.Builder
		finalText      string
		toolCalls      []ToolCall
		usage          Usage
		stopReason     string
		conversationID string
		turns          int
		sawResult      bool
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var envelope map[string]any
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "parse", fmt.Errorf("decode stream event: %w", err))
		}

		event := strings.TrimSpace(asString(envelope["event"]))
		// Every event carries the conversation id, either at the envelope
		// level (init) or inside its payload.
		payload, _ := envelope[event].(map[string]any)
		if id := firstNonEmptyTrimmed(asString(envelope["conversation_id"]), asString(payload["conversation_id"])); id != "" {
			conversationID = id
		}

		switch event {
		case "init":
			// conversation_id captured above; the rest (cwd, tools,
			// permission_mode) is diagnostic only.
		case "step_update":
			switch strings.TrimSpace(asString(payload["step_type"])) {
			case "agent_response":
				if delta := asString(payload["text_delta"]); delta != "" {
					deltaText.WriteString(delta)
					if opts.OnDelta != nil {
						opts.OnDelta(delta)
					}
				}
			case "tool":
				if call, ok := antigravityCLIToolCall(payload); ok {
					toolCalls = append(toolCalls, call)
				}
			}
		case "result":
			sawResult = true
			stopReason = strings.TrimSpace(asString(payload["status"]))
			finalText = strings.TrimSpace(asString(payload["response"]))
			turns = asInt(payload["num_turns"])
			usage = extractAntigravityCLIUsage(payload["usage"])
			if !isAntigravityCLISuccess(stopReason) {
				message := firstNonEmptyTrimmed(
					antigravityCLIErrorMessage(payload["error"]),
					finalText,
					fmt.Sprintf("%s request failed with status %q", antigravityCLIProviderLabel, stopReason),
				)
				return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "request", fmt.Errorf("%s", message))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "stream", fmt.Errorf("read stream response: %w", err))
	}
	// A stream that ends without a result event means the process died
	// mid-turn. Returning the partial deltas would look like a complete
	// answer, so fail instead and let the caller retry.
	if !sawResult {
		return ChatResponse{}, newProviderError(antigravityCLIProviderLabel, "stream", fmt.Errorf("stream ended without a result event"))
	}

	content := finalText
	if content == "" {
		content = strings.TrimSpace(deltaText.String())
	}
	return ChatResponse{
		Message: ChatMessage{
			Role: "assistant",
			// The CLI self-executed any tool calls inside its own subprocess;
			// surfacing them as Message.ToolCalls would make the agent loop
			// re-dispatch work that is already done. The audit trail lives on
			// ProviderExecutedTools instead.
			Content:   content,
			ToolCalls: nil,
		},
		Usage:      usage,
		StopReason: stopReason,
		Turns:      turns,
		// The id round-trips through ChatOptions.ResumeSessionID into
		// --conversation, so multi-turn skips replaying the transcript.
		SessionID:             conversationID,
		ProviderExecutedTools: toolCalls,
	}, nil
}

// isAntigravityCLISuccess reports whether a result status means the turn
// completed. SUCCESS is the only successful terminal status in the documented
// closed vocabulary. The comparison is case-insensitive, but missing or
// unknown statuses fail closed so malformed output cannot look successful.
func isAntigravityCLISuccess(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "SUCCESS")
}

// antigravityCLIToolCall converts a tool step into a ToolCall. Details live
// under tool_info; a step without one (for example an ACTIVE step that has not
// resolved its arguments yet) is skipped rather than recorded blank.
func antigravityCLIToolCall(payload map[string]any) (ToolCall, bool) {
	info, ok := payload["tool_info"].(map[string]any)
	if !ok {
		return ToolCall{}, false
	}
	name := firstNonEmptyTrimmed(asString(info["name"]), asString(payload["tool_name"]))
	if name == "" {
		return ToolCall{}, false
	}
	arguments := ""
	if params, present := info["parameters"]; present && params != nil {
		if encoded, err := json.Marshal(params); err == nil {
			arguments = string(encoded)
		}
	}
	return ToolCall{Name: name, Arguments: arguments}, true
}

// extractAntigravityCLIUsage maps the CLI's usage object onto Usage. The wire
// format also reports thinking_tokens and total_tokens, but Usage has no
// provider-neutral fields for those values. The CLI reports no cost, so
// CostUSD stays zero.
func extractAntigravityCLIUsage(raw any) Usage {
	stats, ok := raw.(map[string]any)
	if !ok {
		return Usage{}
	}
	return Usage{
		InputTokens:     asInt(stats["input_tokens"]),
		OutputTokens:    asInt(stats["output_tokens"]),
		CacheReadTokens: asInt(stats["cache_read_tokens"]),
	}
}

// antigravityCLIErrorMessage pulls a human-readable message out of an error
// object, which the CLI shapes as {type, message}.
func antigravityCLIErrorMessage(raw any) string {
	errMap, ok := raw.(map[string]any)
	if !ok {
		return strings.TrimSpace(asString(raw))
	}
	return firstNonEmptyTrimmed(asString(errMap["message"]), asString(errMap["type"]))
}
