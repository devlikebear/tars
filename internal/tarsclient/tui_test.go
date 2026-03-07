package tarsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIsExitCommand(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{in: "exit", want: true},
		{in: "quit", want: true},
		{in: "/exit", want: true},
		{in: "/quit", want: true},
		{in: "  quit  ", want: true},
		{in: "hello", want: false},
		{in: "/quit now", want: false},
	}
	for _, tc := range cases {
		if got := isExitCommand(tc.in); got != tc.want {
			t.Fatalf("isExitCommand(%q)=%t, want %t", tc.in, got, tc.want)
		}
	}
}

func TestClassifyStatusCategory(t *testing.T) {
	cases := []struct {
		name string
		evt  chatEvent
		want string
	}{
		{
			name: "llm phase",
			evt:  chatEvent{Type: "status", Phase: "before_llm"},
			want: "llm",
		},
		{
			name: "tool phase",
			evt:  chatEvent{Type: "status", Phase: "before_tool_call"},
			want: "tool",
		},
		{
			name: "error phase",
			evt:  chatEvent{Type: "status", Phase: "error"},
			want: "error",
		},
		{
			name: "system fallback",
			evt:  chatEvent{Type: "status", Phase: "stream_open"},
			want: "system",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := classifyStatusCategory(tc.evt)
			if got != tc.want {
				t.Fatalf("classifyStatusCategory() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShouldRenderTraceEvent(t *testing.T) {
	state := localRuntimeState{
		chatTrace:       true,
		chatTraceFilter: "tool",
	}
	if !shouldRenderTraceEvent(state, chatEvent{Type: "status", Phase: "before_tool_call"}) {
		t.Fatal("expected tool event to be rendered for tool filter")
	}
	if shouldRenderTraceEvent(state, chatEvent{Type: "status", Phase: "before_llm"}) {
		t.Fatal("expected llm event to be filtered out for tool filter")
	}
	state.chatTrace = false
	if shouldRenderTraceEvent(state, chatEvent{Type: "status", Phase: "before_tool_call"}) {
		t.Fatal("expected no render when trace is off")
	}
}

func TestCompleteCommandInput(t *testing.T) {
	next, changed := completeCommandInput("/ga")
	if !changed {
		t.Fatal("expected completion for /ga")
	}
	if next != "/gateway " {
		t.Fatalf("expected /gateway completion, got %q", next)
	}

	next, changed = completeCommandInput("/trace filter t")
	if !changed {
		t.Fatal("expected completion for /trace filter t")
	}
	if next != "/trace filter tool " {
		t.Fatalf("expected /trace filter tool completion, got %q", next)
	}

	next, changed = completeCommandInput("/bro")
	if !changed {
		t.Fatal("expected completion for /bro")
	}
	if next != "/browser " {
		t.Fatalf("expected /browser completion, got %q", next)
	}
}

func TestModelEscClearsInputAndCancelsInflight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := newTarsAppModel(ctx, cancel, chatClient{}, runtimeClient{}, "", false)
	model.input.SetValue("hello")

	canceled := false
	model.inflight = true
	model.inflightCancel = func() {
		canceled = true
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, ok := updated.(*tarsAppModel)
	if !ok {
		t.Fatalf("expected *tarsAppModel, got %T", updated)
	}
	if next.input.Value() != "" {
		t.Fatalf("expected input cleared, got %q", next.input.Value())
	}
	if !canceled {
		t.Fatal("expected inflight cancel to be called")
	}
	if !next.inflightCanceled {
		t.Fatal("expected inflightCanceled flag on esc")
	}
}

func TestModelHistoryNavigation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := newTarsAppModel(ctx, cancel, chatClient{}, runtimeClient{}, "", false)
	model.pushHistory("first")
	model.pushHistory("second")

	model.historyPrev()
	if got := model.input.Value(); got != "second" {
		t.Fatalf("expected second on first historyPrev, got %q", got)
	}
	model.historyPrev()
	if got := model.input.Value(); got != "first" {
		t.Fatalf("expected first on second historyPrev, got %q", got)
	}
	model.historyNext()
	if got := model.input.Value(); got != "second" {
		t.Fatalf("expected second on historyNext, got %q", got)
	}
}

func TestHandleSubmit_ExitAliases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := newTarsAppModel(ctx, cancel, chatClient{}, runtimeClient{}, "", false)

	updated, cmd := model.handleSubmit("exit")
	if cmd == nil {
		t.Fatal("expected quit command for exit alias")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
	next, ok := updated.(*tarsAppModel)
	if !ok {
		t.Fatalf("expected *tarsAppModel, got %T", updated)
	}
	select {
	case <-next.ctx.Done():
	default:
		t.Fatal("expected model context canceled on exit alias")
	}
}

func TestFormatChatLine_HelpStylingPreservesText(t *testing.T) {
	section := formatChatLine("Session:", "Session:")
	if !strings.Contains(section, "Session:") {
		t.Fatalf("expected section text preserved, got %q", section)
	}

	cmd := formatChatLine("  /status", "  /status")
	if !strings.Contains(cmd, "/status") {
		t.Fatalf("expected command text preserved, got %q", cmd)
	}

	header := formatChatLine("SYSTEM > commands", "SYSTEM > commands")
	if !strings.Contains(header, "SYSTEM > commands") {
		t.Fatalf("expected header text preserved, got %q", header)
	}
}

func TestTUI_MainSession_BootstrapOnInit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"workspace_dir":   "/tmp/ws",
				"session_count":   1,
				"main_session_id": "sess-main",
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/events/history"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items":        []map[string]any{},
				"unread_count": 0,
				"read_cursor":  0,
				"last_id":      0,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := newTarsAppModel(ctx, cancel, chatClient{serverURL: server.URL}, runtimeClient{serverURL: server.URL}, "", false)
	_ = model.Init()

	deadline := time.After(2 * time.Second)
	for strings.TrimSpace(model.sessionID) == "" {
		select {
		case msg := <-model.asyncCh:
			updated, _ := model.Update(msg)
			next, ok := updated.(*tarsAppModel)
			if !ok {
				t.Fatalf("expected *tarsAppModel, got %T", updated)
			}
			model = next
		case <-deadline:
			t.Fatalf("timeout waiting for main session bootstrap")
		}
	}
	if model.sessionID != "sess-main" {
		t.Fatalf("expected bootstrapped session sess-main, got %q", model.sessionID)
	}
}
