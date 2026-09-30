package tarsserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type auditLog struct {
	mu      sync.Mutex
	entries []ops.AutomationAuditEntry
}

func (a *auditLog) record(entry ops.AutomationAuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, entry)
}

func (a *auditLog) results() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, e := range a.entries {
		out = append(out, e.Actor+":"+e.Result)
	}
	return out
}

func TestChatPermissionModeMappings(t *testing.T) {
	for mode, flag := range map[string]string{"manual": "default", "accept_edits": "acceptEdits", "plan": "plan", "auto": "auto", "bogus": ""} {
		if got := claudeCodePermissionFlag(mode); got != flag {
			t.Errorf("flag(%s) = %q, want %q", mode, got, flag)
		}
	}
	for flag, mode := range map[string]string{"default": "manual", "acceptEdits": "accept_edits", "plan": "plan", "auto": "auto", "dontAsk": "auto", "bypassPermissions": "auto", "": "auto"} {
		if got := chatPermissionModeFromClaudeFlag(flag); got != mode {
			t.Errorf("mode(%s) = %q, want %q", flag, got, mode)
		}
	}
	cases := []struct {
		mode, tool string
		want       chatGateVerdict
	}{
		{"manual", "list_dir", chatGateAllow},
		{"manual", "write_file", chatGateAsk},
		{"manual", "exec", chatGateAsk},
		{"accept_edits", "write_file", chatGateAllow},
		{"accept_edits", "edit_file", chatGateAllow},
		{"accept_edits", "apply_patch", chatGateAllow},
		{"accept_edits", "exec", chatGateAsk},
		{"plan", "write_file", chatGateRefuse},
		{"plan", "exec", chatGateRefuse},
		{"plan", "read_file", chatGateAllow},
		{"auto", "exec", chatGateAllow},
	}
	for _, c := range cases {
		if got := chatGateVerdictFor(c.mode, c.tool); got != c.want {
			t.Errorf("verdict(%s, %s) = %v, want %v", c.mode, c.tool, got, c.want)
		}
	}
	if gateModeFor(session.Session{PermissionMode: "plan"}) != "plan" || gateModeFor(session.Session{PermissionMode: "junk"}) != "manual" || gateModeFor(session.Session{}) != "manual" {
		t.Fatal("gateModeFor")
	}
}

func modeGate(mode string, audit *auditLog) gateHarness {
	sink := newEventSink()
	broker := newChatPermissionBroker()
	broker.audit = audit.record
	stream := newChatStreamWriter(sink, "s1", zerolog.New(io.Discard))
	return gateHarness{gate: newChatToolGate(broker, "s1", "/work", stream, mode), broker: broker, sink: sink}
}

func TestChatToolGateFollowsTheSessionMode(t *testing.T) {
	audit := &auditLog{}

	auto := modeGate("auto", audit)
	if r := wait(t, auto.authorize(context.Background(), "exec", `{"command":"make deploy"}`)); !r.decision.Allow {
		t.Fatalf("auto runs exec without asking: %+v", r)
	}
	auto.sink.none(t)

	edits := modeGate("accept_edits", audit)
	if r := wait(t, edits.authorize(context.Background(), "write_file", `{"path":"a.txt"}`)); !r.decision.Allow {
		t.Fatalf("accept_edits runs write_file: %+v", r)
	}
	edits.sink.none(t)
	pending := edits.authorize(context.Background(), "exec", `{"command":"go test ./..."}`)
	edits.answer(t, "deny")
	if outcome := edits.resolved(t); outcome != "denied" {
		t.Fatalf("outcome = %s", outcome)
	}
	if r := wait(t, pending); r.decision.Allow {
		t.Fatal("accept_edits still asks before exec")
	}

	plan := modeGate("plan", audit)
	r := wait(t, plan.authorize(context.Background(), "write_file", `{"path":"a.txt"}`))
	if r.decision.Allow || r.decision.Message != chatPlanModeMessage {
		t.Fatalf("plan refuses edits: %+v", r)
	}
	plan.sink.none(t)

	if got := strings.Join(audit.results(), ","); got != "console:denied,tars:refused_plan_mode" {
		t.Fatalf("audit = %s", got)
	}
	if e := audit.entries[1]; e.SessionID != "s1" || e.CWD != "/work" || e.Details["tool"] != "write_file" || e.Details["mode"] != "plan" || e.Action != "chat_tool_permission" {
		t.Fatalf("refusal entry = %+v", e)
	}

	if newChatToolGate(newChatPermissionBroker(), "s", "", nil, "nonsense").mode != "manual" {
		t.Fatal("an unknown mode asks")
	}
}

type planResult struct {
	decision llm.ClaudeCodePermissionDecision
	err      error
}

func runPlanHandler(t *testing.T, broker *chatPermissionBroker, sink *eventSink, modes *chatModeSwitch) <-chan planResult {
	t.Helper()
	handler := newChatPermissionHandler(broker, "s1", "/work", newChatStreamWriter(sink, "s1", zerolog.New(io.Discard)), modes)
	out := make(chan planResult, 1)
	go func() {
		d, err := handler(context.Background(), llm.ClaudeCodePermissionRequest{ToolName: "ExitPlanMode", Input: json.RawMessage(`{"plan":"1. Edit a.txt\n2. Run tests"}`)})
		out <- planResult{d, err}
	}()
	return out
}

func TestPlanApprovalSwitchesTheMode(t *testing.T) {
	audit := &auditLog{}
	broker := newChatPermissionBroker()
	broker.audit = audit.record
	broker.always = newChatAlwaysRuleStore(t.TempDir())
	sink := newEventSink()
	var saved string
	done := runPlanHandler(t, broker, sink, &chatModeSwitch{set: func(mode string) error { saved = mode; return nil }})

	prompt := sink.next(t)
	if prompt["type"] != "permission_request" || prompt["tool_name"] != "ExitPlanMode" || prompt["session_rule"] != "" || prompt["always_dir"] != "" {
		t.Fatalf("prompt = %v", prompt)
	}
	if !strings.Contains(prompt["input"].(map[string]any)["plan"].(string), "Run tests") {
		t.Fatalf("the plan rides in the input: %v", prompt)
	}
	if err := broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "allow_once", Mode: "auto"}); err != nil {
		t.Fatal(err)
	}
	if outcome := sink.next(t)["outcome"]; outcome != "plan_approved" {
		t.Fatalf("outcome = %v", outcome)
	}
	if event := sink.next(t); event["type"] != "permission_mode" || event["mode"] != "auto" {
		t.Fatalf("mode event = %v", event)
	}
	r := <-done
	if r.err != nil || !r.decision.Allow || len(r.decision.UpdatedPermissions) != 1 {
		t.Fatalf("decision = %+v %v", r.decision, r.err)
	}
	var update map[string]string
	_ = json.Unmarshal(r.decision.UpdatedPermissions[0], &update)
	if update["type"] != "setMode" || update["mode"] != "auto" || update["destination"] != "session" {
		t.Fatalf("update = %v", update)
	}
	if saved != "auto" {
		t.Fatalf("session mode = %q", saved)
	}
	if got := strings.Join(audit.results(), ","); got != "console:plan_approved" {
		t.Fatalf("audit = %s", got)
	}
}

func TestPlanApprovalDefaultsToAcceptEditsAndCanBeRejected(t *testing.T) {
	broker := newChatPermissionBroker()
	sink := newEventSink()
	done := runPlanHandler(t, broker, sink, nil)
	prompt := sink.next(t)
	_ = broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "allow_once"})
	sink.next(t)
	if event := sink.next(t); event["mode"] != "accept_edits" {
		t.Fatalf("default mode = %v", event)
	}
	var update map[string]string
	r := <-done
	_ = json.Unmarshal(r.decision.UpdatedPermissions[0], &update)
	if update["mode"] != "acceptEdits" {
		t.Fatalf("update = %v", update)
	}

	done = runPlanHandler(t, broker, sink, nil)
	prompt = sink.next(t)
	_ = broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "deny", Message: "also update the docs"})
	if outcome := sink.next(t)["outcome"]; outcome != "plan_rejected" {
		t.Fatalf("outcome = %v", outcome)
	}
	r = <-done
	if r.decision.Allow || !strings.Contains(r.decision.Message, "Keep planning") || !strings.Contains(r.decision.Message, "also update the docs") {
		t.Fatalf("rejection = %+v", r.decision)
	}

	done = runPlanHandler(t, broker, sink, nil)
	prompt = sink.next(t)
	_ = broker.answer(prompt["request_id"].(string), "s1", chatPermissionAnswer{Decision: "deny"})
	sink.next(t)
	if r = <-done; r.decision.Message != chatPlanKeepPlanningMessage {
		t.Fatalf("bare rejection = %q", r.decision.Message)
	}
}

func TestChatPermissionAuditRecordsDecisionsAndWithdrawals(t *testing.T) {
	audit := &auditLog{}
	h := modeGate("manual", audit)
	pending := h.authorize(context.Background(), "exec", `{"command":"ls -la"}`)
	h.answer(t, "allow_session")
	h.resolved(t)
	wait(t, pending)

	ctx, cancel := context.WithCancel(context.Background())
	pending = h.authorize(ctx, "write_file", `{"path":"b"}`)
	h.sink.next(t)
	cancel()
	if outcome := h.resolved(t); outcome != "withdrawn" {
		t.Fatalf("outcome = %s", outcome)
	}
	wait(t, pending)
	if got := strings.Join(audit.results(), ","); got != "console:allowed_session,tars:withdrawn" {
		t.Fatalf("audit = %s", got)
	}
	if audit.entries[0].Details["rule"] != "exec(ls:*)" || audit.entries[0].Details["request_id"] == nil {
		t.Fatalf("entry = %+v", audit.entries[0])
	}
	var nilBroker *chatPermissionBroker
	nilBroker.record("x", "allowed", chatPermissionAudit{})
	if auditTo(nil) != nil {
		t.Fatal("no ops manager, no audit")
	}
}

func newModeServer(t *testing.T, configFlag string) (*httptest.Server, *session.Store, string, *auditLog) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	sess, err := store.Create("modes")
	if err != nil {
		t.Fatal(err)
	}
	audit := &auditLog{}
	mux := http.NewServeMux()
	mux.Handle("/v1/admin/sessions/{id}/permission-mode", newPermissionModeHandler(store, chatPermissionModeResolver{configFlag: configFlag}, audit.record))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store, sess.ID, audit
}

func modeRequest(t *testing.T, method, url, body string) (int, chatPermissionModeView) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var view chatPermissionModeView
	_ = json.NewDecoder(resp.Body).Decode(&view)
	return resp.StatusCode, view
}

func TestPermissionModeEndpoint(t *testing.T) {
	srv, store, id, audit := newModeServer(t, "acceptEdits")
	url := srv.URL + "/v1/admin/sessions/" + id + "/permission-mode"

	code, view := modeRequest(t, http.MethodGet, url, "")
	if code != 200 || view.Mode != "" || view.Effective != "manual" || view.ClaudeCodeEffective != "accept_edits" || view.Source != "config" || view.ClaudeCodeFlag != "acceptEdits" || len(view.Modes) != 4 {
		t.Fatalf("inherited = %d %+v", code, view)
	}
	code, view = modeRequest(t, http.MethodPut, url, `{"mode":"plan"}`)
	if code != 200 || view.Mode != "plan" || view.Effective != "plan" || view.ClaudeCodeEffective != "plan" || view.Source != "session" || view.ClaudeCodeFlag != "plan" {
		t.Fatalf("set = %d %+v", code, view)
	}
	if sess, _ := store.Get(id); sess.PermissionMode != "plan" {
		t.Fatalf("stored = %q", sess.PermissionMode)
	}
	if code, _ = modeRequest(t, http.MethodPut, url, `{"mode":"yolo"}`); code != http.StatusBadRequest {
		t.Fatalf("bad mode = %d", code)
	}
	code, view = modeRequest(t, http.MethodPut, url, `{"mode":""}`)
	if code != 200 || view.Mode != "" || view.Source != "config" {
		t.Fatalf("cleared = %d %+v", code, view)
	}
	if code, _ = modeRequest(t, http.MethodDelete, url, ""); code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE = %d", code)
	}
	if code, _ = modeRequest(t, http.MethodGet, srv.URL+"/v1/admin/sessions/nope/permission-mode", ""); code != http.StatusNotFound {
		t.Fatalf("missing session = %d", code)
	}
	if got := strings.Join(audit.results(), ","); got != "console:changed,console:changed" {
		t.Fatalf("audit = %s", got)
	}
	if audit.entries[0].Details["from"] != "" || audit.entries[0].Details["to"] != "plan" || audit.entries[1].Details["from"] != "plan" {
		t.Fatalf("entry = %+v", audit.entries[0])
	}
}

func TestPermissionAnswerAcceptsAPlanMode(t *testing.T) {
	broker := newChatPermissionBroker()
	id, answers, done := broker.open("s1")
	defer done()
	post := func(body string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/permissions/"+id, strings.NewReader(body))
		handleChatPermissionAnswer(rec, req, broker)
		return rec.Code
	}
	if code := post(`{"session_id":"s1","decision":"allow_once","mode":"plan"}`); code != http.StatusBadRequest {
		t.Fatalf("plan is not a mode to continue in: %d", code)
	}
	if code := post(`{"session_id":"s1","decision":"allow_once","mode":"auto"}`); code != http.StatusOK {
		t.Fatalf("auto = %d", code)
	}
	if a := <-answers; a.Mode != "auto" {
		t.Fatalf("answer = %+v", a)
	}
}
