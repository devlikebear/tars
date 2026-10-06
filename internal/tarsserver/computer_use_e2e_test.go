package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/config"
)

// The whole path the model's call takes, with only the two external parties
// replaced: a stub cua-driver binary replaying the 0.28.2 fixtures, and an
// httptest System One. Tool → engine → real subprocess → real HTTP client.
func TestComputerUseTool_EndToEndThroughStubDriverAndSystemOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub driver is a POSIX shell script")
	}
	fixtures, err := filepath.Abs(filepath.Join("..", "computeruse", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls.log")
	stub := filepath.Join(dir, "cua-driver")
	script := `#!/bin/sh
printf '%s %s\n' "$1" "$2" >> '` + calls + `'
case "$1" in
  list_windows) cat '` + fixtures + `/list_windows.json' ;;
  get_window_state) cat '` + fixtures + `/get_window_state_calculator.json' ;;
  *) printf '%s' '{"structuredContent":{"effect":"confirmed"}}' ;;
esac
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var states []string
	answers := []string{
		// e15 is the "2" key in the Calculator fixture.
		`{"answers":{"op":{"choice":"click","confidence":0.93},"target":{"choice":"e15","confidence":0.81},"input_key":{"choice":"none","confidence":0.9},"risky":{"noul":0.03},"done":{"noul":0.02}},"usage":{"input_tokens":700}}`,
		`{"answers":{"op":{"choice":"done","confidence":0.95},"target":{"choice":"none","confidence":0.9},"input_key":{"choice":"none","confidence":0.9},"risky":{"noul":0.01},"done":{"noul":0.97}},"usage":{"input_tokens":710}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			State string `json:"state"`
		}
		_ = json.Unmarshal(body, &req)
		states = append(states, req.State)
		i := len(states) - 1
		if i >= len(answers) {
			i = len(answers) - 1
		}
		_, _ = w.Write([]byte(answers[i]))
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.WorkspaceDir = t.TempDir()
	cfg.ToolsComputerUseEnabled = true
	cfg.ToolsComputerUseBackend = "jev"
	cfg.ToolsComputerUseCuaDriverPath = stub
	cfg.Jev.BaseURL = srv.URL

	var out computeruse.Result
	found := false
	for _, tl := range buildOptionalChatTools(cfg, nil, nil, nil) {
		if tl.Name != "computer_use" {
			continue
		}
		found = true
		res, err := tl.Execute(context.Background(), json.RawMessage(`{"goal":"press the key named by the input","inputs":{"secret":"hunter2-value"}}`))
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if res.IsError {
			t.Fatalf("error payload: %s", res.Content[0].Text)
		}
		if strings.Contains(res.Content[0].Text, "hunter2-value") {
			t.Fatalf("the input value reached the tool result: %s", res.Content[0].Text)
		}
		if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
			t.Fatal(err)
		}
	}
	if !found {
		t.Fatal("computer_use not registered")
	}

	if out.Status != computeruse.StatusDone || out.Steps != 2 || len(out.Trace) != 2 {
		t.Fatalf("result = %+v", out)
	}
	if out.Trace[0].Op != "click" || out.Trace[0].Target != "AXButton '2'" || out.Trace[0].Effect != "confirmed" {
		t.Fatalf("first step = %+v", out.Trace[0])
	}
	if out.Usage.JevInputTokens != 1410 {
		t.Fatalf("usage = %+v", out.Usage)
	}

	if len(states) != 2 {
		t.Fatalf("System One calls = %d, want 2", len(states))
	}
	for i, state := range states {
		if !strings.Contains(state, "INPUTS AVAILABLE: secret") {
			t.Errorf("state %d does not offer the input key:\n%s", i, state)
		}
		if strings.Contains(state, "hunter2-value") {
			t.Errorf("state %d carries the input value", i)
		}
	}
	if !strings.Contains(states[1], "click AXButton '2'") {
		t.Errorf("the second state does not recall the click:\n%s", states[1])
	}

	log, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	var tools []string
	for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
		tools = append(tools, strings.SplitN(line, " ", 2)[0])
	}
	// Ping, resolve the frontmost window, observe, act, observe again.
	want := "list_windows list_windows get_window_state click get_window_state"
	if got := strings.Join(tools, " "); got != want {
		t.Fatalf("driver calls = %q, want %q\n%s", got, want, log)
	}
	if !strings.Contains(string(log), `"element_token"`) {
		t.Fatalf("the click carried no element token:\n%s", log)
	}
}
