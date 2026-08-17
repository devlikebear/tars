package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The stream-json protocol is parsed by a pure function, so these tests feed it
// directly instead of driving a stubbed process. That keeps them fast and,
// unlike the claude-code-cli tests (which need a POSIX shell stub and are why
// internal/llm is excluded from the Windows job), runnable on every platform.
//
// Event shapes mirror the documented NDJSON from Antigravity CLI 1.1.13: a flat
// envelope with an `event` discriminator whose payload is nested under a key of
// the same name.

const agyConvID = "c3b66b04-872b-4fbe-a3a4-058a026ef20a"

func agyStream(lines ...string) string { return strings.Join(lines, "\n") }

func agyInitEvent() string {
	return `{"event":"init","conversation_id":"` + agyConvID +
		`","init":{"cwd":"/home/user/project","tools":["run_command","write_to_file"],"permission_mode":"request-review"}}`
}

func agyResultEvent(response string) string {
	return `{"event":"result","result":{"conversation_id":"` + agyConvID +
		`","status":"SUCCESS","response":"` + response +
		`","duration_seconds":6.88,"num_turns":1,"usage":{"input_tokens":10418,"output_tokens":589,"thinking_tokens":551,"cache_read_tokens":8113,"total_tokens":11007}}}`
}

func TestParseAntigravityCLIStream_HappyPath(t *testing.T) {
	stream := agyStream(
		agyInitEvent(),
		`{"event":"step_update","step_update":{"conversation_id":"`+agyConvID+`","step_index":0,"state":"DONE","step_type":"user_input"}}`,
		`{"event":"step_update","step_update":{"conversation_id":"`+agyConvID+`","step_index":3,"state":"DONE","step_type":"agent_response","text_delta":"Git rebase rewrites history.","duration_seconds":6.28}}`,
		`{"event":"step_update","step_update":{"conversation_id":"`+agyConvID+`","step_index":4,"state":"DONE","step_type":"checkpoint","duration_seconds":0.53}}`,
		agyResultEvent("Git rebase rewrites history."),
	)

	resp, err := parseAntigravityCLIStream(strings.NewReader(stream), ChatOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Message.Content != "Git rebase rewrites history." {
		t.Errorf("content = %q", resp.Message.Content)
	}
	if resp.Message.Role != "assistant" {
		t.Errorf("role = %q, want assistant", resp.Message.Role)
	}
	// The conversation id round-trips into --conversation, so unlike the
	// Gemini CLI this provider really can resume.
	if resp.SessionID != agyConvID {
		t.Errorf("session id = %q, want %q", resp.SessionID, agyConvID)
	}
	if resp.StopReason != "SUCCESS" {
		t.Errorf("stop reason = %q, want SUCCESS", resp.StopReason)
	}
	if resp.Turns != 1 {
		t.Errorf("turns = %d, want 1", resp.Turns)
	}
	if resp.Usage.InputTokens != 10418 || resp.Usage.OutputTokens != 589 || resp.Usage.CacheReadTokens != 8113 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	// The CLI reports no per-call cost.
	if resp.Usage.CostUSD != 0 {
		t.Errorf("cost = %v, want 0", resp.Usage.CostUSD)
	}
}

// text_delta fragments stream to OnDelta, but result.response is authoritative
// for the final content — otherwise a streaming caller and a non-streaming one
// would disagree, or the text would be counted twice.
func TestParseAntigravityCLIStream_ResultResponseWinsOverDeltas(t *testing.T) {
	stream := agyStream(
		agyInitEvent(),
		`{"event":"step_update","step_update":{"step_type":"agent_response","state":"ACTIVE","text_delta":"Git "}}`,
		`{"event":"step_update","step_update":{"step_type":"agent_response","state":"ACTIVE","text_delta":"rebase "}}`,
		`{"event":"step_update","step_update":{"step_type":"agent_response","state":"DONE","text_delta":"rewrites history."}}`,
		agyResultEvent("Git rebase rewrites history."),
	)

	var deltas []string
	resp, err := parseAntigravityCLIStream(
		strings.NewReader(stream),
		ChatOptions{OnDelta: func(text string) { deltas = append(deltas, text) }},
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Message.Content != "Git rebase rewrites history." {
		t.Errorf("content = %q, want the result response exactly once", resp.Message.Content)
	}
	want := []string{"Git ", "rebase ", "rewrites history."}
	if !reflect.DeepEqual(deltas, want) {
		t.Errorf("deltas = %#v, want %#v", deltas, want)
	}
}

// If the result carries no response text, the accumulated deltas are the only
// thing left to answer with.
func TestParseAntigravityCLIStream_FallsBackToDeltasWhenResponseEmpty(t *testing.T) {
	stream := agyStream(
		agyInitEvent(),
		`{"event":"step_update","step_update":{"step_type":"agent_response","state":"DONE","text_delta":"partial answer"}}`,
		`{"event":"result","result":{"conversation_id":"`+agyConvID+`","status":"SUCCESS","response":"","num_turns":1}}`,
	)

	resp, err := parseAntigravityCLIStream(strings.NewReader(stream), ChatOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Message.Content != "partial answer" {
		t.Errorf("content = %q, want the accumulated deltas", resp.Message.Content)
	}
}

func TestParseAntigravityCLIStream_ToolStepsAreObservationalOnly(t *testing.T) {
	stream := agyStream(
		agyInitEvent(),
		`{"event":"step_update","step_update":{"step_index":1,"state":"DONE","step_type":"tool","tool_name":"run_command",`+
			`"tool_info":{"name":"run_command","parameters":{"command":"go test ./..."},"output":"ok"}}}`,
		`{"event":"step_update","step_update":{"step_type":"agent_response","state":"DONE","text_delta":"tests pass"}}`,
		agyResultEvent("tests pass"),
	)

	resp, err := parseAntigravityCLIStream(strings.NewReader(stream), ChatOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The CLI already ran the tool. Re-dispatching it through TARS' registry
	// would repeat the work, so Message.ToolCalls must stay empty.
	if len(resp.Message.ToolCalls) != 0 {
		t.Errorf("Message.ToolCalls = %#v, want empty", resp.Message.ToolCalls)
	}
	if len(resp.ProviderExecutedTools) != 1 {
		t.Fatalf("ProviderExecutedTools = %#v, want 1 entry", resp.ProviderExecutedTools)
	}
	got := resp.ProviderExecutedTools[0]
	if got.Name != "run_command" {
		t.Errorf("tool name = %q", got.Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(got.Arguments), &args); err != nil {
		t.Fatalf("arguments %q is not JSON: %v", got.Arguments, err)
	}
	if args["command"] != "go test ./..." {
		t.Errorf("arguments = %v", args)
	}
}

// A tool step without tool_info (an ACTIVE step that has not resolved its
// arguments yet) is skipped rather than recorded as a blank call.
func TestParseAntigravityCLIStream_ToolStepWithoutInfoIsSkipped(t *testing.T) {
	stream := agyStream(
		agyInitEvent(),
		`{"event":"step_update","step_update":{"state":"ACTIVE","step_type":"tool","tool_name":"run_command"}}`,
		agyResultEvent("done"),
	)

	resp, err := parseAntigravityCLIStream(strings.NewReader(stream), ChatOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(resp.ProviderExecutedTools) != 0 {
		t.Errorf("ProviderExecutedTools = %#v, want empty", resp.ProviderExecutedTools)
	}
}

func TestParseAntigravityCLIStream_Errors(t *testing.T) {
	tests := []struct {
		name       string
		stream     string
		wantErrSub string
	}{
		{
			name: "a non-success status surfaces the error message",
			stream: agyStream(agyInitEvent(),
				`{"event":"result","result":{"status":"ERROR","error":{"type":"AuthError","message":"authentication required"}}}`),
			wantErrSub: "authentication required",
		},
		{
			name:       "a non-success status without an error object still fails",
			stream:     agyStream(agyInitEvent(), `{"event":"result","result":{"status":"CANCELLED"}}`),
			wantErrSub: `status "CANCELLED"`,
		},
		{
			name:       "a result with no terminal status fails closed",
			stream:     agyStream(agyInitEvent(), `{"event":"result","result":{"response":"looks complete"}}`),
			wantErrSub: "looks complete",
		},
		{
			// The process died mid-turn. Returning the partial deltas would
			// look like a complete answer.
			name: "a stream without a result event fails",
			stream: agyStream(agyInitEvent(),
				`{"event":"step_update","step_update":{"step_type":"agent_response","state":"ACTIVE","text_delta":"half an ans"}}`),
			wantErrSub: "without a result event",
		},
		{
			name:       "malformed json fails loudly",
			stream:     `{"event":"step_update",`,
			wantErrSub: "decode stream event",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseAntigravityCLIStream(strings.NewReader(tc.stream), ChatOptions{})
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErrSub)
			}
		})
	}
}

func TestParseAntigravityCLIStream_IgnoresBlankLinesAndUnknownEvents(t *testing.T) {
	stream := agyStream(
		``,
		agyInitEvent(),
		`   `,
		`{"event":"something_new_upstream_added","payload":{"a":1}}`,
		`{"event":"step_update","step_update":{"step_type":"subagent","state":"DONE"}}`,
		agyResultEvent("ok"),
		``,
	)

	resp, err := parseAntigravityCLIStream(strings.NewReader(stream), ChatOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp.Message.Content != "ok" {
		t.Errorf("content = %q, want ok", resp.Message.Content)
	}
}

func TestParseAntigravityCLIStream_AcceptsLargeToolOutputEvent(t *testing.T) {
	// Tool output is carried inside one NDJSON object. Keep this above the old
	// 1 MiB scanner limit to prevent a regression to bufio.Scanner defaults.
	toolOutput := strings.Repeat("x", 2*1024*1024)
	toolEvent, err := json.Marshal(map[string]any{
		"event": "step_update",
		"step_update": map[string]any{
			"state":     "DONE",
			"step_type": "tool",
			"tool_info": map[string]any{
				"name":       "run_command",
				"parameters": map[string]any{"command": "go test ./..."},
				"output":     toolOutput,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal tool event: %v", err)
	}

	stream := agyStream(agyInitEvent(), string(toolEvent), agyResultEvent("done"))
	resp, err := parseAntigravityCLIStream(strings.NewReader(stream), ChatOptions{})
	if err != nil {
		t.Fatalf("parse large event: %v", err)
	}
	if len(resp.ProviderExecutedTools) != 1 {
		t.Fatalf("ProviderExecutedTools = %#v, want 1 entry", resp.ProviderExecutedTools)
	}
}

func TestBuildAntigravityCLIArgs(t *testing.T) {
	tests := []struct {
		name string
		spec antigravityCLIArgSpec
		want []string
	}{
		{
			name: "minimal turn omits every optional flag",
			spec: antigravityCLIArgSpec{Prompt: "Say hello."},
			want: []string{"--output-format", "stream-json", "--print", "Say hello."},
		},
		{
			name: "model, mode, effort and timeout are threaded through",
			spec: antigravityCLIArgSpec{
				Model:           "gemini-3-pro",
				Prompt:          "Refactor it.",
				Mode:            "accept-edits",
				ReasoningEffort: "high",
				PrintTimeout:    90 * time.Second,
			},
			want: []string{
				"--output-format", "stream-json",
				"--print-timeout", "1m30s",
				"--model", "gemini-3-pro",
				"--mode", "accept-edits",
				"--effort", "high",
				"--print", "Refactor it.",
			},
		},
		{
			name: "a resumed turn passes --conversation",
			spec: antigravityCLIArgSpec{Prompt: "And again.", ConversationID: agyConvID},
			want: []string{
				"--output-format", "stream-json",
				"--conversation", agyConvID,
				"--print", "And again.",
			},
		},
		{
			name: "a json schema is passed verbatim",
			spec: antigravityCLIArgSpec{Prompt: "Extract it.", JSONSchema: `{"type":"object"}`},
			want: []string{
				"--output-format", "stream-json",
				"--json-schema", `{"type":"object"}`,
				"--print", "Extract it.",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildAntigravityCLIArgs(tc.spec)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("args =\n  %#v\nwant\n  %#v", got, tc.want)
			}
		})
	}
}

// The prompt is last and introduced by --print, so a prompt that looks like a
// flag cannot be parsed as one.
func TestBuildAntigravityCLIArgs_PromptCannotBeMistakenForAFlag(t *testing.T) {
	args := buildAntigravityCLIArgs(antigravityCLIArgSpec{
		Prompt: "--dangerously-skip-permissions --mode accept-edits",
	})
	if args[len(args)-2] != "--print" {
		t.Fatalf("prompt must be introduced by --print, got %#v", args)
	}
	if args[len(args)-1] != "--dangerously-skip-permissions --mode accept-edits" {
		t.Fatalf("prompt should be the final argument, got %#v", args)
	}
	for _, arg := range args[:len(args)-1] {
		if arg == "--dangerously-skip-permissions" || arg == "--mode" {
			t.Fatalf("prompt text leaked into argv: %#v", args)
		}
	}
}

// No env-var value may reach --dangerously-skip-permissions: auto-approving
// every tool call is not something a stray environment variable should do.
func TestResolveAntigravityCLIMode(t *testing.T) {
	tests := map[string]string{
		"accept-edits":                   "accept-edits",
		"ACCEPT-EDITS":                   "accept-edits",
		"plan":                           "plan",
		" plan ":                         "plan",
		"":                               "",
		"yolo":                           "",
		"dangerously-skip-permissions":   "",
		"--dangerously-skip-permissions": "",
		"acceptEdits":                    "",
		"nonsense":                       "",
	}
	for input, want := range tests {
		if got := resolveAntigravityCLIMode(input); got != want {
			t.Errorf("resolveAntigravityCLIMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveAntigravityCLIEffort(t *testing.T) {
	tests := map[string]string{
		"low":       "low",
		"minimal":   "low",
		"min":       "low",
		"none":      "low",
		"medium":    "medium",
		"med":       "medium",
		"high":      "high",
		"xhigh":     "high",
		"very-high": "high",
		"HIGH":      "high",
		"":          "",
		"nonsense":  "",
	}
	for input, want := range tests {
		if got := resolveAntigravityCLIEffort(input); got != want {
			t.Errorf("resolveAntigravityCLIEffort(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAntigravityCLIJSONSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`)
	if got := antigravityCLIJSONSchema(&ResponseFormat{Type: ResponseFormatJSONSchema, Schema: schema}); got != string(schema) {
		t.Errorf("json_schema = %q, want the schema verbatim", got)
	}
	// Only json_schema maps onto the flag; the CLI constrains the final result
	// only, so json_object has no schema to pass.
	for _, format := range []*ResponseFormat{
		nil,
		{Type: ResponseFormatText},
		{Type: ResponseFormatJSONObject},
		{Type: ResponseFormatJSONSchema},
	} {
		if got := antigravityCLIJSONSchema(format); got != "" {
			t.Errorf("format %+v produced %q, want empty", format, got)
		}
	}
}

func TestComposeAntigravityCLIPrompt_FoldsSystemMessagesIn(t *testing.T) {
	// The CLI has no system-prompt flag, so system content has to reach the
	// model through the prompt or it is silently lost.
	messages := []ChatMessage{
		{Role: "system", Content: "Answer only in Korean."},
		{Role: "user", Content: "Hello."},
		{Role: "assistant", Content: "안녕하세요."},
		{Role: "user", Content: "Again please."},
	}
	prompt := composeAntigravityCLIPrompt(
		collectAntigravityCLISystemPrompt(messages),
		buildAntigravityCLITranscript(messages),
	)
	for _, want := range []string{"Answer only in Korean.", "Hello.", "안녕하세요.", "Again please."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Index(prompt, "Answer only in Korean.") > strings.Index(prompt, "Hello.") {
		t.Errorf("system block should precede the transcript:\n%s", prompt)
	}
}

// The transcript must be empty when there is nothing to answer — that is the
// signal Chat uses to reject a turn. Folding the system block in first would
// mask it, since the preamble is never empty.
func TestBuildAntigravityCLITranscript_EmptyWithoutConversationalContent(t *testing.T) {
	for _, messages := range [][]ChatMessage{
		nil,
		{{Role: "system", Content: "You are helpful."}},
		{{Role: "system", Content: "a"}, {Role: "system", Content: "b"}},
	} {
		if got := buildAntigravityCLITranscript(messages); got != "" {
			t.Errorf("transcript for %#v = %q, want empty", messages, got)
		}
	}
	if got := buildAntigravityCLITranscript([]ChatMessage{{Role: "user", Content: "hi"}}); got == "" {
		t.Error("a user turn must produce a transcript")
	}
}

func TestComposeAntigravityCLIPrompt_BlankSystemBlockAddsNoSeparator(t *testing.T) {
	if got := composeAntigravityCLIPrompt("", "Again please."); got != "Again please." {
		t.Errorf("prompt = %q, want the bare transcript", got)
	}
	if got := composeAntigravityCLIPrompt("   ", "Again please."); got != "Again please." {
		t.Errorf("a blank system block must not add separators, got %q", got)
	}
}

func TestParseAntigravityCLITimeout(t *testing.T) {
	tests := map[string]time.Duration{
		"":         defaultAntigravityCLITimeout,
		"   ":      defaultAntigravityCLITimeout,
		"90s":      90 * time.Second,
		"2m":       2 * time.Minute,
		"120":      120 * time.Second,
		"0":        defaultAntigravityCLITimeout,
		"-30s":     defaultAntigravityCLITimeout,
		"nonsense": defaultAntigravityCLITimeout,
	}
	for input, want := range tests {
		if got := parseAntigravityCLITimeout(input); got != want {
			t.Errorf("parseAntigravityCLITimeout(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestAntigravityCLIEnv_DoesNotOverrideOperatorValues(t *testing.T) {
	base := []string{"PATH=/usr/bin", "NO_COLOR=0"}
	got := antigravityCLIEnv(base)

	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "NO_COLOR=0") || strings.Contains(joined, "NO_COLOR=1") {
		t.Errorf("an inherited NO_COLOR must win, got:\n%s", joined)
	}
	if len(base) != 2 {
		t.Errorf("base was mutated: %#v", base)
	}
}

func TestExtractAntigravityCLIUsage(t *testing.T) {
	usage := extractAntigravityCLIUsage(map[string]any{
		"input_tokens":      float64(10418),
		"output_tokens":     float64(589),
		"thinking_tokens":   float64(551),
		"cache_read_tokens": float64(8113),
		"total_tokens":      float64(11007),
	})
	if usage.InputTokens != 10418 || usage.OutputTokens != 589 || usage.CacheReadTokens != 8113 {
		t.Errorf("usage = %+v", usage)
	}
	// A missing or wrongly-shaped usage object must not panic.
	if got := extractAntigravityCLIUsage(nil); got != (Usage{}) {
		t.Errorf("nil usage = %+v, want zero value", got)
	}
	if got := extractAntigravityCLIUsage("not an object"); got != (Usage{}) {
		t.Errorf("string usage = %+v, want zero value", got)
	}
}

func TestIsAntigravityCLISuccess(t *testing.T) {
	for _, status := range []string{"SUCCESS", "success", " Success "} {
		if !isAntigravityCLISuccess(status) {
			t.Errorf("isAntigravityCLISuccess(%q) = false, want true", status)
		}
	}
	for _, status := range []string{"", "OK", "COMPLETED", "ERROR", "CANCELED", "INTERRUPTED", "INVALID", "WAITING", "RUNNING", "whatever"} {
		if isAntigravityCLISuccess(status) {
			t.Errorf("isAntigravityCLISuccess(%q) = true, want false", status)
		}
	}
}

func TestFindAntigravityCLIPath_HonoursTheEnvOverride(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "agy-stub"+exeSuffix())
	if err := os.WriteFile(stub, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	t.Setenv(antigravityCLIPathEnv, stub)
	got, err := FindAntigravityCLIPath()
	if err != nil {
		t.Fatalf("FindAntigravityCLIPath: %v", err)
	}
	if got != stub {
		t.Errorf("path = %q, want %q", got, stub)
	}

	t.Setenv(antigravityCLIPathEnv, filepath.Join(dir, "definitely-absent"+exeSuffix()))
	if _, err := FindAntigravityCLIPath(); err == nil {
		t.Fatal("an unresolvable override should error")
	} else if !strings.Contains(err.Error(), antigravityCLIProviderLabel) {
		t.Errorf("error should name the provider, got: %v", err)
	}
}

// An empty model is left unset rather than guessed: there is no stable short
// alias to hardcode and an invalid --model value fails the whole turn.
func TestNewAntigravityCLIClient_Defaults(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "agy-stub"+exeSuffix())
	if err := os.WriteFile(stub, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(antigravityCLIPathEnv, stub)

	client, err := NewAntigravityCLIClient("", "")
	if err != nil {
		t.Fatalf("NewAntigravityCLIClient: %v", err)
	}
	if client.workDir != "." {
		t.Errorf("workDir = %q, want \".\"", client.workDir)
	}
	if client.model != "" {
		t.Errorf("model = %q, want empty so the CLI picks its own default", client.model)
	}
}

// NewProvider must route "antigravity-cli" to this client without resolving any
// TARS-held credential — the CLI owns the Google login.
func TestNewProvider_RoutesAntigravityCLIWithoutACredential(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "agy-stub"+exeSuffix())
	if err := os.WriteFile(stub, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(antigravityCLIPathEnv, stub)
	// Deliberately no API key in the environment.
	t.Setenv("GEMINI_API_KEY", "")

	client, err := NewProvider(ProviderOptions{
		Provider: "antigravity-cli",
		WorkDir:  dir,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, ok := client.(*AntigravityCLIClient); !ok {
		t.Fatalf("client = %T, want *AntigravityCLIClient", client)
	}
}

func TestAntigravityCLIClient_ChatRejectsAnEmptyPrompt(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "agy-stub"+exeSuffix())
	if err := os.WriteFile(stub, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv(antigravityCLIPathEnv, stub)

	client, err := NewAntigravityCLIClient(dir, "")
	if err != nil {
		t.Fatalf("NewAntigravityCLIClient: %v", err)
	}
	// Only a system message: nothing for the model to answer, so the client
	// must fail before spawning the (non-executable) stub.
	if _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "system", Content: "You are helpful."},
	}, ChatOptions{}); err == nil {
		t.Fatal("expected an empty-prompt error")
	} else if !strings.Contains(err.Error(), "prompt is empty") {
		t.Errorf("error = %v, want an empty-prompt error", err)
	}
}

// Opt-in live coverage for the locally authenticated CLI. Normal CI skips it:
// it requires a user-owned Antigravity login and consumes account quota.
// Run with TARS_TEST_ANTIGRAVITY_LIVE=1 and optionally AGY_CLI_PATH=<path>.
func TestAntigravityCLIClient_LiveConversationResume(t *testing.T) {
	if os.Getenv("TARS_TEST_ANTIGRAVITY_LIVE") != "1" {
		t.Skip("set TARS_TEST_ANTIGRAVITY_LIVE=1 to exercise the authenticated CLI")
	}

	client, err := NewProvider(ProviderOptions{
		Provider: "antigravity-cli",
		WorkDir:  ".",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	firstMessages := []ChatMessage{
		{Role: "user", Content: "Remember the codeword ORCHID for the next turn. Reply with exactly: ACK"},
	}
	first, err := client.Chat(context.Background(), firstMessages, ChatOptions{})
	if err != nil {
		t.Fatalf("first Chat: %v", err)
	}
	if got := strings.TrimSpace(first.Message.Content); got != "ACK" {
		t.Fatalf("first response = %q, want ACK", got)
	}
	if strings.TrimSpace(first.SessionID) == "" {
		t.Fatal("first response did not expose a resumable conversation id")
	}

	secondMessages := append(firstMessages,
		ChatMessage{Role: "assistant", Content: first.Message.Content},
		ChatMessage{Role: "user", Content: "Reply with only the codeword I asked you to remember. No punctuation."},
	)
	second, err := client.Chat(context.Background(), secondMessages, ChatOptions{
		ResumeSessionID: first.SessionID,
	})
	if err != nil {
		t.Fatalf("resumed Chat: %v", err)
	}
	if got := strings.TrimSpace(second.Message.Content); got != "ORCHID" {
		t.Fatalf("resumed response = %q, want ORCHID", got)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("resumed session id = %q, want %q", second.SessionID, first.SessionID)
	}
}

// exeSuffix keeps the stub resolvable by exec.LookPath, which on Windows only
// accepts names carrying a PATHEXT extension.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
