package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/rs/zerolog"
)

// eventSink is an http.ResponseWriter that turns each SSE frame into an
// event on a channel, so a test can answer prompts while the gate waits.
type eventSink struct {
	header http.Header
	events chan map[string]any
}

func newEventSink() *eventSink {
	return &eventSink{header: http.Header{}, events: make(chan map[string]any, 32)}
}

func (s *eventSink) Header() http.Header { return s.header }
func (s *eventSink) WriteHeader(int)     {}
func (s *eventSink) Flush()              {}

func (s *eventSink) Write(p []byte) (int, error) {
	payload, ok := bytes.CutPrefix(bytes.TrimSpace(p), []byte("data: "))
	if ok {
		var event map[string]any
		if json.Unmarshal(payload, &event) == nil {
			s.events <- event
		}
	}
	return len(p), nil
}

func (s *eventSink) next(t *testing.T) map[string]any {
	t.Helper()
	select {
	case event := <-s.events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return nil
	}
}

func (s *eventSink) none(t *testing.T) {
	t.Helper()
	select {
	case event := <-s.events:
		t.Fatalf("unexpected event %v", event)
	default:
	}
}

type gateHarness struct {
	gate   *chatToolGate
	broker *chatPermissionBroker
	sink   *eventSink
}

func newGateHarness() gateHarness {
	sink := newEventSink()
	broker := newChatPermissionBroker()
	stream := newChatStreamWriter(sink, "s1", zerolog.New(io.Discard))
	return gateHarness{gate: newChatToolGate(broker, "s1", "", stream, ""), broker: broker, sink: sink}
}

type gateResult struct {
	decision agentloop.ToolDecision
	err      error
}

// authorize runs the gate in the background and returns its result channel.
func (h gateHarness) authorize(ctx context.Context, name, args string) <-chan gateResult {
	out := make(chan gateResult, 1)
	go func() {
		decision, err := h.gate.Authorize(ctx, agentloop.ToolCallRequest{ToolName: name, ToolCallID: "call_" + name, ToolArgs: args})
		out <- gateResult{decision, err}
	}()
	return out
}

// answer waits for the prompt, answers it, and returns the prompt event.
func (h gateHarness) answer(t *testing.T, decision string) map[string]any {
	t.Helper()
	prompt := h.sink.next(t)
	if prompt["type"] != "permission_request" {
		t.Fatalf("expected permission_request, got %v", prompt)
	}
	if err := h.broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: decision}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	return prompt
}

func (h gateHarness) resolved(t *testing.T) string {
	t.Helper()
	event := h.sink.next(t)
	if event["type"] != "permission_resolved" {
		t.Fatalf("expected permission_resolved, got %v", event)
	}
	return event["outcome"].(string)
}

func wait(t *testing.T, ch <-chan gateResult) gateResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("gate did not return")
		return gateResult{}
	}
}

func TestChatToolGateLetsReadOnlyToolsThrough(t *testing.T) {
	h := newGateHarness()
	r := wait(t, h.authorize(context.Background(), "list_dir", `{"path":"."}`))
	if r.err != nil || !r.decision.Allow {
		t.Fatalf("read-only tool: %+v", r)
	}
	h.sink.none(t)
}

func TestChatToolGateSessionRuleCoversTheSamePrefix(t *testing.T) {
	h := newGateHarness()
	first := h.authorize(context.Background(), "exec", `{"command":"git status --short"}`)
	prompt := h.answer(t, "allow_session")
	if prompt["tool_name"] != "exec" || prompt["tool_use_id"] != "call_exec" || prompt["session_rule"] != "exec(git status:*)" {
		t.Fatalf("prompt = %v", prompt)
	}
	if input, _ := prompt["input"].(map[string]any); input["command"] != "git status --short" {
		t.Fatalf("input = %v", prompt["input"])
	}
	if outcome := h.resolved(t); outcome != "allowed_session" {
		t.Fatalf("outcome = %q", outcome)
	}
	if r := wait(t, first); r.err != nil || !r.decision.Allow {
		t.Fatalf("first call: %+v", r)
	}

	// Same prefix: no prompt.
	if r := wait(t, h.authorize(context.Background(), "exec", `{"command":"git status"}`)); !r.decision.Allow {
		t.Fatalf("remembered prefix was asked again: %+v", r)
	}
	h.sink.none(t)

	// A different subcommand, or the prefix hiding a second command, asks.
	for _, command := range []string{"git push", "git status && rm -rf /"} {
		pending := h.authorize(context.Background(), "exec", `{"command":"`+command+`"}`)
		h.answer(t, "deny")
		h.resolved(t)
		if r := wait(t, pending); r.decision.Allow {
			t.Fatalf("%q ran under the git status rule", command)
		}
	}
}

func TestChatToolGateRulesAreScopedToTheSession(t *testing.T) {
	h := newGateHarness()
	pending := h.authorize(context.Background(), "write_file", `{"path":"a.txt"}`)
	if rule := h.answer(t, "allow_session")["session_rule"]; rule != "write_file" {
		t.Fatalf("session_rule = %v", rule)
	}
	h.resolved(t)
	wait(t, pending)

	if r := wait(t, h.authorize(context.Background(), "write_file", `{"path":"b.txt"}`)); !r.decision.Allow {
		t.Fatalf("write_file asked again: %+v", r)
	}
	h.sink.none(t)

	// Another tool is still asked about, and so is another session.
	pending = h.authorize(context.Background(), "edit_file", `{"path":"a.txt"}`)
	h.answer(t, "allow_once")
	h.resolved(t)
	wait(t, pending)

	other := newChatToolGate(h.broker, "s2", "", newChatStreamWriter(h.sink, "s2", zerolog.New(io.Discard)), "")
	out := make(chan gateResult, 1)
	go func() {
		d, err := other.Authorize(context.Background(), agentloop.ToolCallRequest{ToolName: "write_file", ToolArgs: `{"path":"c.txt"}`})
		out <- gateResult{d, err}
	}()
	prompt := h.sink.next(t)
	if prompt["type"] != "permission_request" || prompt["session_id"] != "s2" {
		t.Fatalf("another session's write_file was not asked about: %v", prompt)
	}
	_ = h.broker.answer(prompt["request_id"].(string), "s2", chatPermissionAnswer{Decision: "deny"})
	wait(t, out)
}

func TestChatToolGateCompoundCommandGetsNoRule(t *testing.T) {
	h := newGateHarness()
	pending := h.authorize(context.Background(), "exec", `{"command":"make build && make test"}`)
	if rule := h.answer(t, "allow_session")["session_rule"]; rule != "" {
		t.Fatalf("session_rule = %v, want none", rule)
	}
	if outcome := h.resolved(t); outcome != "allowed" {
		t.Fatalf("outcome = %q, want allowed (once)", outcome)
	}
	wait(t, pending)

	// Nothing was remembered, so the same command asks again.
	pending = h.authorize(context.Background(), "exec", `{"command":"make build && make test"}`)
	h.answer(t, "deny")
	h.resolved(t)
	wait(t, pending)
}

func TestChatToolGateDeny(t *testing.T) {
	h := newGateHarness()
	pending := h.authorize(context.Background(), "apply_patch", `{"patch":"..."}`)
	h.answer(t, "deny")
	if outcome := h.resolved(t); outcome != "denied" {
		t.Fatalf("outcome = %q", outcome)
	}
	r := wait(t, pending)
	if r.err != nil || r.decision.Allow || strings.TrimSpace(r.decision.Message) == "" {
		t.Fatalf("deny: %+v", r)
	}
}

func TestChatToolGateWithdrawsOnCancel(t *testing.T) {
	h := newGateHarness()
	ctx, cancel := context.WithCancel(context.Background())
	pending := h.authorize(ctx, "exec", `{"command":"ls"}`)
	requestID := h.sink.next(t)["request_id"].(string)
	cancel()
	if outcome := h.resolved(t); outcome != "withdrawn" {
		t.Fatalf("outcome = %q", outcome)
	}
	if r := wait(t, pending); !errors.Is(r.err, context.Canceled) {
		t.Fatalf("err = %v", r.err)
	}
	if err := h.broker.answer(requestID, "s1", chatPermissionAnswer{Decision: "allow_once"}); !errors.Is(err, errChatPermissionNotFound) {
		t.Fatalf("answer after withdraw: %v", err)
	}
}

// nativeWriteClient stands in for a native provider that asks to write a
// file in the session's workspace, then answers.
func nativeWriteClient() *mockLLMClient {
	return &mockLLMClient{responses: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID:        "call_write",
			Name:      "write_file",
			Arguments: `{"path":"gated.txt","content":"hello"}`,
		}}}},
		{Message: llm.ChatMessage{Role: "assistant", Content: "done"}},
	}}
}

func TestChatAPI_NativeToolGateAsksBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		decision string
		outcome  string
		written  bool
	}{
		{"deny", "denied", false},
		{"allow_once", "allowed", true},
	} {
		t.Run(tc.decision, func(t *testing.T) {
			client := nativeWriteClient()
			srv := newPermissionTestServer(t, client)
			if outcome := answerNativeWrite(t, srv, tc.decision); outcome != tc.outcome {
				t.Fatalf("outcome = %v", outcome)
			}
			if written := gatedFileWritten(srv.root); written != tc.written {
				t.Fatalf("gated.txt written = %v, want %v", written, tc.written)
			}
			if client.callCount != 2 {
				t.Fatalf("the turn did not continue after the decision (llm calls = %d)", client.callCount)
			}
		})
	}
}

// answerNativeWrite runs a nativeWriteClient turn, answers its write_file
// prompt with decision, waits for the turn to finish, and returns the
// prompt's outcome.
func answerNativeWrite(t *testing.T, srv permissionTestServer, decision string) any {
	t.Helper()
	events := srv.startChat(t, true)
	prompt := waitForEvent(t, events, "permission_request")
	if prompt["tool_name"] != "write_file" || prompt["tool_use_id"] != "call_write" || prompt["session_rule"] != "write_file" {
		t.Fatalf("permission_request = %v", prompt)
	}
	if code := srv.answer(t, prompt["request_id"].(string), decision); code != http.StatusOK {
		t.Fatalf("answer: status %d", code)
	}
	outcome := waitForEvent(t, events, "permission_resolved")["outcome"]
	waitForEvent(t, events, "done")
	return outcome
}

func gatedFileWritten(root string) bool {
	written := false
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "gated.txt" {
			written = true
		}
		return nil
	})
	return written
}

// Without the console's opt-in the gate is absent and the tool runs as it
// always has.
func TestChatAPI_NativeToolGateNeedsInteractiveOptIn(t *testing.T) {
	client := nativeWriteClient()
	srv := newPermissionTestServer(t, client)
	events := srv.startChat(t, false)
	for {
		event, ok := <-events
		if !ok {
			t.Fatal("stream ended without done")
		}
		if event["type"] == "permission_request" {
			t.Fatalf("a non-interactive turn asked for permission: %v", event)
		}
		if event["type"] == "done" {
			return
		}
	}
}
