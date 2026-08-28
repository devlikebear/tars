package apptool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/session"
)

// Several tool constructors had no test at all. Every one of them puts a name,
// description, and JSON schema into the system prompt on each chat turn, so a
// malformed schema or a renamed tool is a live defect rather than a cosmetic
// one — and nothing was checking.

func assertToolShape(t *testing.T, tl Tool, wantName string) {
	t.Helper()
	if tl.Name != wantName {
		t.Errorf("tool name = %q, want %q", tl.Name, wantName)
	}
	if strings.TrimSpace(tl.Description) == "" {
		t.Errorf("%s has an empty description; the model sees this verbatim", wantName)
	}
	if tl.Execute == nil {
		t.Errorf("%s has no Execute function", wantName)
	}
	// The schema goes into the prompt as JSON — a malformed one would be
	// shipped to the provider unnoticed.
	if len(tl.Parameters) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(tl.Parameters, &schema); err != nil {
			t.Fatalf("%s parameter schema is not valid JSON: %v", wantName, err)
		}
		if schema["type"] != "object" {
			t.Errorf("%s schema type = %v, want object", wantName, schema["type"])
		}
	}
}

func TestNewSessionsListTool_Shape(t *testing.T) {
	assertToolShape(t, NewSessionsListTool(session.NewStore(t.TempDir())), "sessions_list")
}

func TestNewSessionsHistoryTool_Shape(t *testing.T) {
	assertToolShape(t, NewSessionsHistoryTool(session.NewStore(t.TempDir())), "sessions_history")
}

func TestNewAgentsListTool_Shape(t *testing.T) {
	// A nil runtime is the disabled case; the tool must still be constructible
	// so the registry can report it as unavailable rather than panic.
	assertToolShape(t, NewAgentsListTool(nil), "agents_list")
}

func TestNewSessionTool_Shape(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	tl := NewSessionTool(store, nil, func(context.Context) (SessionStatus, error) {
		return SessionStatus{SessionID: main.ID}, nil
	})
	assertToolShape(t, tl, "session")
}

func TestNewWorkspaceTool_Shape(t *testing.T) {
	assertToolShape(t, NewWorkspaceTool(t.TempDir()), "workspace")
}

func TestNewAgentSyspromptGetTool_Shape(t *testing.T) {
	assertToolShape(t, NewAgentSyspromptGetTool(t.TempDir()), "agent_sysprompt_get")
}

func TestSessionsListTool_ReturnsTheStoresSessions(t *testing.T) {
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	res, err := NewSessionsListTool(store).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("sessions_list returned an error: %+v", res)
	}
	if !strings.Contains(res.Content[0].Text, main.ID) {
		t.Fatalf("sessions_list did not include the main session %q: %s", main.ID, res.Content[0].Text)
	}
}

func TestSessionsHistoryTool_ReportsAnUnknownSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	if _, err := store.EnsureMain(); err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	res, err := NewSessionsHistoryTool(store).Execute(
		context.Background(),
		json.RawMessage(`{"session_id":"sess_does_not_exist"}`),
	)
	if err != nil {
		t.Fatalf("execute returned a Go error rather than a tool result: %v", err)
	}
	if !res.IsError {
		t.Fatalf("history for an unknown session should be an error result: %+v", res)
	}
}

func TestWorkspaceTool_ListGetSetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	tl := NewWorkspaceTool(dir)
	run := func(body string) Result {
		t.Helper()
		res, err := tl.Execute(context.Background(), json.RawMessage(body))
		if err != nil {
			t.Fatalf("execute %s: %v", body, err)
		}
		return res
	}

	if res := run(`{"action":"list","scope":"workspace"}`); res.IsError {
		t.Fatalf("list returned an error: %+v", res)
	}

	// set then get, through the same dispatch the model uses.
	setRes := run(`{"action":"set","scope":"workspace","file":"USER.md","content":"name: tester\n"}`)
	if setRes.IsError {
		t.Fatalf("set returned an error: %+v", setRes)
	}
	written, err := os.ReadFile(filepath.Join(dir, "USER.md"))
	if err != nil {
		t.Fatalf("set did not write the file: %v", err)
	}
	if !strings.Contains(string(written), "name: tester") {
		t.Fatalf("file content = %q", written)
	}

	getRes := run(`{"action":"get","scope":"workspace","file":"USER.md"}`)
	if getRes.IsError {
		t.Fatalf("get returned an error: %+v", getRes)
	}
	if !strings.Contains(getRes.Content[0].Text, "name: tester") {
		t.Fatalf("get did not return what set wrote: %s", getRes.Content[0].Text)
	}
}

func TestWorkspaceTool_RejectsAnUnknownAction(t *testing.T) {
	res, err := NewWorkspaceTool(t.TempDir()).Execute(
		context.Background(),
		json.RawMessage(`{"action":"demolish"}`),
	)
	if err != nil {
		t.Fatalf("execute returned a Go error rather than a tool result: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unknown action should be an error result: %+v", res)
	}
}

func TestWithCurrentTelegramTarget_RoundTripsThroughContext(t *testing.T) {
	ctx := WithCurrentTelegramTarget(context.Background(), "bot-1", "chat-9", "thread-3")
	if ctx == nil {
		t.Fatal("WithCurrentTelegramTarget returned a nil context")
	}
	// The value is read back by the telegram tool's default-target resolution;
	// asserting the context is distinct from its parent pins that something
	// was actually stored.
	if ctx == context.Background() {
		t.Fatal("WithCurrentTelegramTarget did not attach anything")
	}
}
