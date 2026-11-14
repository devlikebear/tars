package tarsserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	pkgllm "github.com/devlikebear/tars/pkg/llm"
	"github.com/rs/zerolog"
)

// deviceScopeFixture is a workspace with one focus pipeline (session
// pipeline, Q&A session qa) and one ordinary chat session (other).
type deviceScopeFixture struct {
	root     string
	store    *session.Store
	pipeline string
	qa       string
	other    string
}

func newDeviceScopeFixture(t *testing.T) deviceScopeFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatal(err)
	}
	store := session.NewStore(root)
	f := deviceScopeFixture{root: root, store: store}
	for _, id := range []*string{&f.pipeline, &f.qa, &f.other} {
		sess, err := store.Create("s")
		if err != nil {
			t.Fatal(err)
		}
		*id = sess.ID
	}
	if err := focusStoreFor(store).Save(focuspipeline.Pipeline{SessionID: f.pipeline, QASessionID: f.qa}); err != nil {
		t.Fatal(err)
	}
	return f
}

// asRole serves h with role set as the auth middleware would; "" leaves
// the request as an admin or loopback caller sends it.
func asRole(h http.Handler, role string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role != "" {
			r = r.WithContext(serverauth.WithRoleContext(r.Context(), role))
		}
		h.ServeHTTP(w, r)
	})
}

func sendTo(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestFocusDeviceScopeAllows(t *testing.T) {
	f := newDeviceScopeFixture(t)
	scope := newFocusDeviceScope(f.store)
	for _, tc := range []struct {
		id           string
		pipelineOnly bool
		want         bool
	}{
		{f.pipeline, true, true},
		{f.pipeline, false, true},
		{f.qa, false, true},
		{f.qa, true, false},
		{f.other, false, false},
		{"", false, false},
		{"main", false, false},
		{"../escape", false, false},
	} {
		got, err := scope.allows(tc.id, tc.pipelineOnly)
		if err != nil || got != tc.want {
			t.Errorf("allows(%q, pipelineOnly=%v) = %v, %v; want %v", tc.id, tc.pipelineOnly, got, err, tc.want)
		}
	}
	if ok, err := newFocusDeviceScope(nil).allows(f.pipeline, false); ok || err != nil {
		t.Fatalf("no store reaches nothing: %v %v", ok, err)
	}
	if all, err := newFocusDeviceScope(nil).sessions(); err != nil || len(all) != 0 {
		t.Fatalf("no store lists nothing: %v %v", all, err)
	}
}

func TestFocusDeviceScopeUnreadablePipelines(t *testing.T) {
	f := newDeviceScopeFixture(t)
	// A pipeline file that cannot be read answers 500, not a guess.
	if err := os.WriteFile(filepath.Join(f.root, "sessions", f.other+".pipeline.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := asRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if newFocusDeviceScope(f.store).refuse(w, r, f.other, false) {
			return
		}
		w.WriteHeader(http.StatusTeapot)
	}), serverauth.RoleFocus)
	if rec := sendTo(t, h, http.MethodGet, "/", ""); rec.Code != http.StatusInternalServerError {
		t.Fatalf("unreadable pipeline: %d", rec.Code)
	}
	// The listing skips an unreadable file, so a Q&A lookup still answers.
	if ok, err := newFocusDeviceScope(f.store).allows(f.qa, false); !ok || err != nil {
		t.Fatalf("qa lookup past a broken file: %v %v", ok, err)
	}
}

func TestFocusDeviceChatRoutes(t *testing.T) {
	f := newDeviceScopeFixture(t)
	chat := newChatAPIHandlerWithRuntimeConfig(f.root, f.store, &permissionAskingClient{}, nil, zerolog.New(io.Discard), 0, nil, "", defaultChatToolingOptions())
	phone := asRole(chat, serverauth.RoleFocus)

	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"turn in another session", http.MethodPost, "/v1/chat", `{"session_id":"` + f.other + `","message":"hi"}`, http.StatusForbidden},
		{"turn in the main session", http.MethodPost, "/v1/chat", `{"message":"hi"}`, http.StatusForbidden},
		{"turn in the Q&A session", http.MethodPost, "/v1/chat", `{"session_id":"` + f.qa + `","message":"hi"}`, http.StatusForbidden},
		{"turn pinning a tier", http.MethodPost, "/v1/chat", `{"session_id":"` + f.pipeline + `","message":"hi","tier_recommendation":{"chosen_tier":"light","accepted":true,"pin":true}}`, http.StatusForbidden},
		{"feed of another session", http.MethodGet, "/v1/chat/stream?session_id=" + f.other, "", http.StatusForbidden},
		{"feed of the pipeline session", http.MethodGet, "/v1/chat/stream?session_id=" + f.pipeline, "", http.StatusNoContent},
		{"feed of the Q&A session", http.MethodGet, "/v1/chat/stream?session_id=" + f.qa, "", http.StatusNoContent},
		{"cancel in another session", http.MethodPost, "/v1/chat/cancel?session_id=" + f.other, "", http.StatusForbidden},
		{"cancel in the pipeline session", http.MethodPost, "/v1/chat/cancel?session_id=" + f.pipeline, "", http.StatusNotFound},
		{"answer in another session", http.MethodPost, "/v1/chat/permissions/r1", `{"session_id":"` + f.other + `","decision":"allow_once"}`, http.StatusForbidden},
		{"answer always", http.MethodPost, "/v1/chat/permissions/r1", `{"session_id":"` + f.pipeline + `","decision":"allow_always"}`, http.StatusForbidden},
		{"answer with a mode", http.MethodPost, "/v1/chat/permissions/r1", `{"session_id":"` + f.pipeline + `","decision":"allow_once","mode":"auto"}`, http.StatusForbidden},
		{"answer once", http.MethodPost, "/v1/chat/permissions/r1", `{"session_id":"` + f.pipeline + `","decision":"allow_once"}`, http.StatusNotFound},
	} {
		if rec := sendTo(t, phone, tc.method, tc.path, tc.body); rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}

	// Every other caller is held to nothing new.
	desk := asRole(chat, "")
	if rec := sendTo(t, desk, http.MethodGet, "/v1/chat/stream?session_id="+f.other, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("desk feed: %d", rec.Code)
	}
	if rec := sendTo(t, desk, http.MethodPost, "/v1/chat/permissions/r1", `{"session_id":"`+f.other+`","decision":"allow_always"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("desk answer: %d", rec.Code)
	}
}

func TestFocusDeviceTurnInItsPipelineSession(t *testing.T) {
	f := newDeviceScopeFixture(t)
	chat := newChatAPIHandlerWithRuntimeConfig(f.root, f.store, &pkgllm.FakeClient{Label: "phone"}, nil, zerolog.New(io.Discard), 0, nil, "", defaultChatToolingOptions())
	rec := sendTo(t, asRole(chat, serverauth.RoleFocus), http.MethodPost, "/v1/chat", `{"session_id":"`+f.pipeline+`","message":"from the train"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "phone") {
		t.Fatalf("phone turn: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusDeviceChatActivity(t *testing.T) {
	f := newDeviceScopeFixture(t)
	activity := newChatActivity(f.store, nil)
	for _, id := range []string{f.pipeline, f.qa, f.other} {
		activity.observe(id, map[string]any{"type": "permission_request", "request_id": "r-" + id, "tool_name": "Bash"})
		activity.begin(id)
	}
	activity.queued = func() ([]chatQueuedApproval, error) {
		return []chatQueuedApproval{{ApprovalID: "a1", SessionID: f.pipeline}, {ApprovalID: "a2", SessionID: f.other}}, nil
	}
	read := func(role string) chatActivitySnapshot {
		t.Helper()
		rec := sendTo(t, asRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handleChatActivity(w, r, activity)
		}), role), http.MethodGet, "/v1/chat/activity", "")
		var snap chatActivitySnapshot
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &snap) != nil {
			t.Fatalf("%q: %d %s", role, rec.Code, rec.Body.String())
		}
		return snap
	}
	phone := read(serverauth.RoleFocus)
	if len(phone.Running) != 2 || len(phone.Pending) != 2 || len(phone.Queued) != 1 || phone.Queued[0].ApprovalID != "a1" {
		t.Fatalf("phone sees %+v", phone)
	}
	for _, p := range phone.Pending {
		if p.SessionID == f.other {
			t.Fatalf("phone saw another session's question: %+v", p)
		}
	}
	if desk := read(""); len(desk.Running) != 3 || len(desk.Pending) != 3 || len(desk.Queued) != 2 {
		t.Fatalf("desk sees %+v", desk)
	}

	// A workspace path that is a file makes the pipeline listing fail.
	notADir := filepath.Join(t.TempDir(), "workspace")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := newChatActivity(session.NewStore(notADir), nil)
	rec := sendTo(t, asRole(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleChatActivity(w, r, broken)
	}), serverauth.RoleFocus), http.MethodGet, "/v1/chat/activity", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unreadable pipelines: %d", rec.Code)
	}
}

func TestFocusDeviceOpsApprovals(t *testing.T) {
	f := newDeviceScopeFixture(t)
	manager := ops.NewManager(f.root, ops.Options{HomeDir: t.TempDir()})
	ask := func(sessionID string) string {
		t.Helper()
		a, err := manager.CreateToolPermissionApproval(ops.ToolPermissionRequest{SessionID: sessionID, Source: "focus", ToolName: "exec"})
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	inPipeline, inQA, inOther := ask(f.pipeline), ask(f.qa), ask(f.other)

	// A cleanup approval acts on the desk, outside any session.
	path := filepath.Join(f.root, "ops", "approvals.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var items []ops.Approval
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	items = append(items, ops.Approval{ID: "cleanup-1", Type: "cleanup", Status: ops.ApprovalStatusPending})
	raw, _ = json.Marshal(items)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	h := newOpsAPIHandler(manager, zerolog.New(io.Discard), nil, f.store)
	phone := asRole(h, serverauth.RoleFocus)
	for _, tc := range []struct {
		name, path string
		want       int
	}{
		{"another session", "/v1/ops/approvals/" + inOther + "/approve", http.StatusForbidden},
		{"cleanup", "/v1/ops/approvals/cleanup-1/approve", http.StatusForbidden},
		{"unknown", "/v1/ops/approvals/nope/approve", http.StatusNotFound},
		{"pipeline session", "/v1/ops/approvals/" + inPipeline + "/approve", http.StatusOK},
		{"Q&A session", "/v1/ops/approvals/" + inQA + "/reject", http.StatusOK},
	} {
		if rec := sendTo(t, phone, http.MethodPost, tc.path, ""); rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
	if got, err := manager.GetApproval(inOther); err != nil || got.Status != ops.ApprovalStatusPending {
		t.Fatalf("a refused answer must leave the question open: %+v %v", got, err)
	}
	// The desk still answers anything.
	if rec := sendTo(t, asRole(h, ""), http.MethodPost, "/v1/ops/approvals/"+inOther+"/reject", ""); rec.Code != http.StatusOK {
		t.Fatalf("desk: %d %s", rec.Code, rec.Body.String())
	}
}
