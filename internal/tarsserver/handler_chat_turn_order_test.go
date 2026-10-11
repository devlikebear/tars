package tarsserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// scriptedStreamClient plays a turn the way claude-code-cli streams one:
// text deltas and live tool reports in the order the CLI emitted them, then
// the response with every text block joined into one Content.
type scriptedStreamClient struct {
	steps []scriptedStreamStep
	resp  llm.ChatResponse
	err   error
}

type scriptedStreamStep struct {
	delta string
	tool  *llm.ProviderToolEvent
}

func (c *scriptedStreamClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *scriptedStreamClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	for _, step := range c.steps {
		if step.tool != nil {
			if opts.OnProviderTool != nil {
				opts.OnProviderTool(*step.tool)
			}
			continue
		}
		if opts.OnDelta != nil {
			opts.OnDelta(step.delta)
		}
	}
	return c.resp, c.err
}

type transcriptEntry struct {
	Role, Content, ToolCallID string
	Interim                   bool
}

func transcriptEntries(t *testing.T, store *session.Store) []transcriptEntry {
	t.Helper()
	sessions, err := store.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v, err %v", sessions, err)
	}
	messages, err := session.ReadMessages(store.TranscriptPath(sessions[0].ID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	out := make([]transcriptEntry, 0, len(messages))
	for _, m := range messages {
		entry := transcriptEntry{Role: m.Role, Interim: m.Interim, ToolCallID: m.ToolCallID}
		if m.Role != "tool" {
			entry.Content = m.Content
		}
		if m.Role == "user" {
			entry.Content = ""
		}
		out = append(out, entry)
	}
	return out
}

func assertTranscript(t *testing.T, got, want []transcriptEntry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("transcript =\n%+v\nwant\n%+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d = %+v, want %+v\nfull transcript: %+v", i, got[i], want[i], got)
		}
	}
}

func cliTurnSteps() []scriptedStreamStep {
	return []scriptedStreamStep{
		{delta: "Reading the file first."},
		{tool: &llm.ProviderToolEvent{Call: liveRead}},
		{tool: &llm.ProviderToolEvent{Call: liveRead, Finished: true, Result: "file contents"}},
		{delta: "\n\nNow the tests."},
		{tool: &llm.ProviderToolEvent{Call: liveBash}},
		{tool: &llm.ProviderToolEvent{Call: liveBash, Finished: true, Result: "ok"}},
		{delta: "\n\nAll green."},
	}
}

// A reopened claude-code-cli turn keeps the order it streamed in: each
// piece of text before the tools it went on to call, the reply last.
func TestChatAPI_CLITurnPersistsTextAndToolsInStreamOrder(t *testing.T) {
	handler, store := newLiveProviderToolHandler(t, &scriptedStreamClient{
		steps: cliTurnSteps(),
		resp: llm.ChatResponse{
			Message:               llm.ChatMessage{Role: "assistant", Content: "Reading the file first.\nNow the tests.\nAll green."},
			ProviderExecutedTools: []llm.ToolCall{liveRead, liveBash},
		},
	})
	postChat(t, handler)
	assertTranscript(t, transcriptEntries(t, store), []transcriptEntry{
		{Role: "user"},
		{Role: "assistant", Content: "Reading the file first.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_r"},
		{Role: "assistant", Content: "Now the tests.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_b"},
		{Role: "assistant", Content: "All green."},
	})
}

// A turn that ends on a tool call still closes with a (blank) reply message,
// so the tools stay attached to the turn when the history is replayed.
func TestChatAPI_CLITurnEndingOnToolKeepsOrder(t *testing.T) {
	steps := cliTurnSteps()
	handler, store := newLiveProviderToolHandler(t, &scriptedStreamClient{
		steps: steps[:len(steps)-1],
		resp: llm.ChatResponse{
			Message:               llm.ChatMessage{Role: "assistant", Content: "Reading the file first.\nNow the tests."},
			ProviderExecutedTools: []llm.ToolCall{liveRead, liveBash},
		},
	})
	postChat(t, handler)
	assertTranscript(t, transcriptEntries(t, store), []transcriptEntry{
		{Role: "user"},
		{Role: "assistant", Content: "Reading the file first.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_r"},
		{Role: "assistant", Content: "Now the tests.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_b"},
		{Role: "assistant", Content: ""},
	})
}

// A turn whose tools ran before any text keeps the old layout: tools, then
// the reply.
func TestChatAPI_TurnWithoutTextBeforeToolsKeepsOldLayout(t *testing.T) {
	handler, store := newLiveProviderToolHandler(t, &scriptedStreamClient{
		steps: []scriptedStreamStep{
			{tool: &llm.ProviderToolEvent{Call: liveRead}},
			{tool: &llm.ProviderToolEvent{Call: liveRead, Finished: true, Result: "file contents"}},
			{delta: "Read it."},
		},
		resp: llm.ChatResponse{
			Message:               llm.ChatMessage{Role: "assistant", Content: "Read it."},
			ProviderExecutedTools: []llm.ToolCall{liveRead},
		},
	})
	postChat(t, handler)
	assertTranscript(t, transcriptEntries(t, store), []transcriptEntry{
		{Role: "user"},
		{Role: "tool", ToolCallID: "toolu_r"},
		{Role: "assistant", Content: "Read it."},
	})
}

// A native provider's text before a tool call used to be streamed and then
// lost: only the last iteration's reply was saved. It is saved in place now.
func TestChatAPI_NativeTurnPersistsIntermediateText(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := session.NewStore(root)
	client := &mockLLMClient{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", Content: "Writing the note.", ToolCalls: []llm.ToolCall{{
				ID:        "call_write",
				Name:      "write_file",
				Arguments: `{"path":"notes/a.txt","content":"one\n"}`,
			}}}},
			{Message: llm.ChatMessage{Role: "assistant", Content: "Wrote it."}},
		},
	}
	handler := newChatAPIHandler(root, store, client, zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"message":"write a note"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tars-Debug-Auth-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	assertTranscript(t, transcriptEntries(t, store), []transcriptEntry{
		{Role: "user"},
		{Role: "assistant", Content: "Writing the note.", Interim: true},
		{Role: "tool", ToolCallID: "call_write"},
		{Role: "assistant", Content: "Wrote it."},
	})
}

// The model is handed back the same shape of turn as before: one assistant
// message carrying every tool call, followed by the tool results. Text said
// between the tools is folded into that message ahead of the reply.
func TestBuildLLMMessageHistory_FoldsInterimTextIntoReply(t *testing.T) {
	history := []session.Message{
		{Role: "user", Content: "fix the tests"},
		{Role: "assistant", Content: "Reading the file first.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_r", ToolName: "Read", ToolArgs: `{}`, Content: "file contents"},
		{Role: "assistant", Content: "Now the tests.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_b", ToolName: "Bash", ToolArgs: `{}`, Content: "ok"},
		{Role: "assistant", Content: "All green."},
		{Role: "user", Content: "thanks"},
	}
	got := buildLLMMessageHistory(history)
	if len(got) != 5 {
		t.Fatalf("messages = %+v, want user, assistant, 2 tools, user", got)
	}
	reply := got[1]
	if reply.Role != "assistant" || reply.Content != "Reading the file first.\n\nNow the tests.\n\nAll green." {
		t.Fatalf("reply = %+v", reply)
	}
	if len(reply.ToolCalls) != 2 || reply.ToolCalls[0].ID != "toolu_r" || reply.ToolCalls[1].ID != "toolu_b" {
		t.Fatalf("tool calls = %+v", reply.ToolCalls)
	}
	if got[2].ToolCallID != "toolu_r" || got[3].ToolCallID != "toolu_b" || got[4].Content != "thanks" {
		t.Fatalf("messages = %+v", got)
	}
}

// Interim text of a turn that never got its reply (a failed turn) does not
// leak into the next turn's reply.
func TestBuildLLMMessageHistory_DropsInterimTextOfUnfinishedTurn(t *testing.T) {
	history := []session.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "Starting.", Interim: true},
		{Role: "tool", ToolCallID: "toolu_r", ToolName: "Read", Content: "(no result)"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "Done."},
	}
	got := buildLLMMessageHistory(history)
	if len(got) != 3 || got[2].Content != "Done." || len(got[2].ToolCalls) != 0 {
		t.Fatalf("messages = %+v", got)
	}
}

// A reply the provider did not stream is never replaced by streamed text
// that is not it: such a turn keeps the old layout.
func TestChatTurnMessages_UnstreamedReplyKeepsOldLayout(t *testing.T) {
	records := []ToolCallRecord{{ToolName: "Read", ToolCallID: "toolu_r", textBefore: "Reading."}}
	resp := llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "The reply."}}
	got := chatTurnMessages(resp, records, "", time.Now())
	if len(got) != 2 || got[0].Role != "tool" || got[1].Content != "The reply." || got[1].Interim {
		t.Fatalf("messages = %+v", got)
	}
}

// A nil collector (other callers of the agent loop) records nothing.
func TestChatTurnText_NilIsInert(t *testing.T) {
	var text *chatTurnText
	text.write("ignored")
	if got := text.take(); got != "" {
		t.Fatalf("take = %q", got)
	}
	live := newChatTurnText()
	live.write("a")
	live.write("b")
	if got := live.take(); got != "ab" {
		t.Fatalf("take = %q", got)
	}
	if got := live.take(); got != "" {
		t.Fatalf("second take = %q", got)
	}
}

// TestBuildLLMMessageHistory_ConsecutiveAssistantMessagesSurvive checks
// tars#1220's requirement that an initiative-authored assistant message
// followed later by a normal turn's assistant reply, with no tool calls
// and no user message between them (the user has not replied yet), comes
// through as two separate assistant ChatMessages rather than erroring or
// silently merging — buildLLMMessageHistory has no special-casing for
// repeated roles, so two assistant entries in the transcript become two
// assistant entries in the LLM history, same as it already does for a
// user message with nothing between it and the next.
func TestBuildLLMMessageHistory_ConsecutiveAssistantMessagesSurvive(t *testing.T) {
	history := []session.Message{
		{Role: "assistant", Content: "Welcome back!", Initiative: &session.MessageInitiative{Intent: "greet"}},
		{Role: "user", Content: "thanks"},
		{Role: "assistant", Content: "You're welcome."},
	}
	got := buildLLMMessageHistory(history)
	if len(got) != 3 {
		t.Fatalf("messages = %+v, want 3", got)
	}
	if got[0].Role != "assistant" || got[0].Content != "Welcome back!" {
		t.Fatalf("first message = %+v", got[0])
	}
	if got[1].Role != "user" || got[1].Content != "thanks" {
		t.Fatalf("second message = %+v", got[1])
	}
	if got[2].Role != "assistant" || got[2].Content != "You're welcome." {
		t.Fatalf("third message = %+v", got[2])
	}
}
