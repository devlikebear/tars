package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// workDirEditingClient edits the turn's work directory the way a CLI
// provider does: inside the call, invisible to TARS's own tools.
type workDirEditingClient struct {
	edit func(dir string)
	err  error
}

func (c *workDirEditingClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *workDirEditingClient) Chat(_ context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if c.edit != nil && opts.WorkDir != "" {
		c.edit(opts.WorkDir)
	}
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	if opts.OnDelta != nil {
		opts.OnDelta("done")
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "done"}}, nil
}

type checkpointFixture struct {
	root       string
	project    string
	sessions   *session.Store
	sessionID  string
	store      *checkpoint.Store
	checkpoint http.Handler
}

func newCheckpointFixture(t *testing.T) checkpointFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	writeTestFile(t, filepath.Join(project, "base.txt"), "one\ntwo\n")
	store, err := checkpoint.Open(filepath.Join(root, "_shared", "checkpoints"), checkpoint.Options{})
	if err != nil {
		t.Skipf("checkpoints need git: %v", err)
	}
	sessions := session.NewStore(root)
	sess, err := sessions.Create("checkpoint session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := sessions.SetWorkDirs(sess.ID, []string{project}, project); err != nil {
		t.Fatalf("set work dirs: %v", err)
	}
	return checkpointFixture{
		root:       root,
		project:    project,
		sessions:   sessions,
		sessionID:  sess.ID,
		store:      store,
		checkpoint: newCheckpointAPIHandler(store, sessions, zerolog.New(io.Discard)),
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// chat runs one turn through the chat handler and returns its SSE events.
func (f checkpointFixture) chat(t *testing.T, client llm.Client, store *checkpoint.Store) []map[string]any {
	t.Helper()
	tooling := defaultChatToolingOptions()
	tooling.Checkpoints = store
	handler := newChatAPIHandlerWithRuntimeConfig(f.root, f.sessions, client, nil, zerolog.New(io.Discard), agent.DefaultMaxLoopIters, nil, "", tooling)
	raw, err := json.Marshal(map[string]any{"session_id": f.sessionID, "message": "edit the files"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status %d body=%q", rec.Code, rec.Body.String())
	}
	var events []map[string]any
	for _, block := range strings.Split(rec.Body.String(), "\n\n") {
		data, ok := strings.CutPrefix(strings.TrimSpace(block), "data: ")
		if !ok {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("decode event %q: %v", data, err)
		}
		events = append(events, event)
	}
	return events
}

func eventOfType(events []map[string]any, kind string) map[string]any {
	for _, e := range events {
		if e["type"] == kind {
			return e
		}
	}
	return nil
}

func (f checkpointFixture) get(t *testing.T, path string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	f.checkpoint.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if out != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v body=%q", path, err, rec.Body.String())
		}
	}
	return rec.Code
}

// A turn's edits are recorded whichever provider made them, announced on the
// stream under the user message's ID, and readable through the API.
func TestChatTurnRecordsCheckpoint(t *testing.T) {
	f := newCheckpointFixture(t)
	client := &workDirEditingClient{edit: func(dir string) {
		writeTestFile(t, filepath.Join(dir, "base.txt"), "one\n2\n")
		writeTestFile(t, filepath.Join(dir, "new.txt"), "fresh\n")
	}}
	events := f.chat(t, client, f.store)

	started := eventOfType(events, "turn_started")
	if started == nil {
		t.Fatalf("no turn_started event in %v", events)
	}
	turnID, _ := started["user_message_id"].(string)
	if turnID == "" {
		t.Fatalf("turn_started without a user message id: %v", started)
	}
	cp := eventOfType(events, "checkpoint")
	if cp == nil {
		t.Fatalf("no checkpoint event in %v", events)
	}
	if cp["user_message_id"] != turnID || cp["files"] != float64(2) || cp["additions"] != float64(2) || cp["deletions"] != float64(1) {
		t.Fatalf("checkpoint event = %v, want turn %s with 2 files +2 -1", cp, turnID)
	}
	if done := eventOfType(events, "done"); done == nil || done["user_message_id"] != turnID {
		t.Fatalf("done event = %v, want user_message_id %s", done, turnID)
	}

	history, err := session.ReadMessages(f.sessions.TranscriptPath(f.sessionID))
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if len(history) == 0 || history[0].Role != "user" || history[0].ID != turnID {
		t.Fatalf("the turn ID must be the stored user message's ID: %+v", history)
	}

	var list struct {
		SessionID string             `json:"session_id"`
		Turns     []checkpoint.Entry `json:"turns"`
	}
	if code := f.get(t, "/v1/admin/sessions/"+f.sessionID+"/checkpoints", &list); code != http.StatusOK {
		t.Fatalf("list status %d", code)
	}
	if list.SessionID != f.sessionID || len(list.Turns) != 1 || list.Turns[0].TurnID != turnID || list.Turns[0].Preview != "edit the files" {
		t.Fatalf("list = %+v", list)
	}

	var diff checkpoint.DiffResult
	if code := f.get(t, "/v1/admin/sessions/"+f.sessionID+"/checkpoints/"+turnID+"/diff", &diff); code != http.StatusOK {
		t.Fatalf("diff status %d", code)
	}
	if diff.Scope != checkpoint.ScopeTurn || len(diff.Files) != 2 {
		t.Fatalf("diff = %+v", diff)
	}
	var one checkpoint.DiffResult
	if code := f.get(t, "/v1/admin/sessions/"+f.sessionID+"/checkpoints/"+turnID+"/diff?scope=session&path=new.txt", &one); code != http.StatusOK {
		t.Fatalf("filtered diff status %d", code)
	}
	if one.Scope != checkpoint.ScopeSession || len(one.Files) != 1 || one.Files[0].Path != "new.txt" {
		t.Fatalf("filtered diff = %+v", one)
	}
}

// A turn that edits nothing still gets a checkpoint, with nothing in it.
func TestChatTurnWithoutEditsRecordsEmptyCheckpoint(t *testing.T) {
	f := newCheckpointFixture(t)
	events := f.chat(t, &workDirEditingClient{}, f.store)
	cp := eventOfType(events, "checkpoint")
	if cp == nil || cp["files"] != float64(0) {
		t.Fatalf("checkpoint event = %v, want 0 files", cp)
	}
}

// A turn that fails after editing still records what it left on disk.
func TestFailedChatTurnStillRecordsCheckpoint(t *testing.T) {
	f := newCheckpointFixture(t)
	events := f.chat(t, &workDirEditingClient{
		edit: func(dir string) { writeTestFile(t, filepath.Join(dir, "half.txt"), "partial\n") },
		err:  errors.New("provider crashed"),
	}, f.store)
	if eventOfType(events, "error") == nil {
		t.Fatalf("no error event in %v", events)
	}
	if cp := eventOfType(events, "checkpoint"); cp == nil || cp["files"] != float64(1) {
		t.Fatalf("checkpoint event = %v, want 1 file", cp)
	}
}

// Without a store (no git) the turn runs as before and says nothing about
// checkpoints.
func TestChatTurnWithoutCheckpointStore(t *testing.T) {
	f := newCheckpointFixture(t)
	events := f.chat(t, &workDirEditingClient{}, nil)
	if eventOfType(events, "checkpoint") != nil {
		t.Fatalf("checkpoint event without a store: %v", events)
	}
	if eventOfType(events, "turn_started") == nil || eventOfType(events, "done") == nil {
		t.Fatalf("turn events missing: %v", events)
	}
}

func TestCheckpointAPIErrors(t *testing.T) {
	f := newCheckpointFixture(t)
	events := f.chat(t, &workDirEditingClient{}, f.store)
	turnID, _ := eventOfType(events, "turn_started")["user_message_id"].(string)
	base := "/v1/admin/sessions/" + f.sessionID + "/checkpoints/"

	cases := []struct {
		name string
		path string
		want int
	}{
		{"unknown session", "/v1/admin/sessions/nope/checkpoints", http.StatusNotFound},
		{"unknown turn", base + "nope/diff", http.StatusNotFound},
		{"bad scope", base + turnID + "/diff?scope=sideways", http.StatusBadRequest},
		{"escaping path", base + turnID + "/diff?path=../x", http.StatusBadRequest},
	}
	for _, tc := range cases {
		if code := f.get(t, tc.path, nil); code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, code, tc.want)
		}
	}

	rec := httptest.NewRecorder()
	f.checkpoint.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/sessions/"+f.sessionID+"/checkpoints", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", rec.Code)
	}

	off := newCheckpointAPIHandler(nil, f.sessions, zerolog.New(io.Discard))
	rec = httptest.NewRecorder()
	off.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/sessions/"+f.sessionID+"/checkpoints", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no store: status %d, want 503", rec.Code)
	}
}

// A skipped turn has no snapshots to diff; the API says so instead of failing.
func TestCheckpointAPISkippedTurn(t *testing.T) {
	f := newCheckpointFixture(t)
	f.store = mustOpenLimitedStore(t, f.root)
	f.checkpoint = newCheckpointAPIHandler(f.store, f.sessions, zerolog.New(io.Discard))
	events := f.chat(t, &workDirEditingClient{edit: func(dir string) {
		writeTestFile(t, filepath.Join(dir, "a.txt"), "a\n")
		writeTestFile(t, filepath.Join(dir, "b.txt"), "b\n")
	}}, f.store)
	cp := eventOfType(events, "checkpoint")
	if cp == nil || cp["skipped"] == "" {
		t.Fatalf("checkpoint event = %v, want a skip", cp)
	}
	turnID, _ := cp["user_message_id"].(string)
	if code := f.get(t, "/v1/admin/sessions/"+f.sessionID+"/checkpoints/"+turnID+"/diff", nil); code != http.StatusConflict {
		t.Fatalf("diff of a skipped turn: status %d, want 409", code)
	}
}

func mustOpenLimitedStore(t *testing.T, root string) *checkpoint.Store {
	t.Helper()
	limits := checkpoint.DefaultLimits
	limits.MaxFiles = 1
	store, err := checkpoint.Open(filepath.Join(root, "_shared", "limited"), checkpoint.Options{Limits: limits})
	if err != nil {
		t.Skipf("checkpoints need git: %v", err)
	}
	return store
}

// Deleting a session drops its checkpoints; so does the startup sweep for
// sessions deleted while nothing was listening.
func TestCheckpointCleanupFollowsSessions(t *testing.T) {
	f := newCheckpointFixture(t)
	f.chat(t, &workDirEditingClient{}, f.store)

	gone, err := f.sessions.Create("deleted while offline")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.SetWorkDirs(gone.ID, []string{f.project}, f.project); err != nil {
		t.Fatal(err)
	}
	offline := f
	offline.sessionID = gone.ID
	offline.chat(t, &workDirEditingClient{}, f.store)
	if err := f.sessions.Delete(gone.ID); err != nil {
		t.Fatal(err)
	}

	attachCheckpointCleanup(f.store, f.sessions, zerolog.New(io.Discard))
	waitForTurns(t, f.store, gone.ID, 0)
	if turns, err := f.store.List(f.sessionID); err != nil || len(turns) != 1 {
		t.Fatalf("the sweep must keep a live session's checkpoints: %v %v", turns, err)
	}

	if err := f.sessions.Delete(f.sessionID); err != nil {
		t.Fatal(err)
	}
	waitForTurns(t, f.store, f.sessionID, 0)
}

func waitForTurns(t *testing.T, store *checkpoint.Store, sessionID string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		turns, err := store.List(sessionID)
		if err == nil && len(turns) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %s: %d turns (err %v), want %d", sessionID, len(turns), err, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The TARS workspace itself holds transcripts, memory, and logs that change
// on every turn. A session pointed at it gets no checkpoint.
func TestChatTurnInWorkspaceRootIsNotCheckpointed(t *testing.T) {
	f := newCheckpointFixture(t)
	if err := f.sessions.SetWorkDirs(f.sessionID, []string{f.root}, f.root); err != nil {
		t.Fatal(err)
	}
	events := f.chat(t, &workDirEditingClient{}, f.store)
	if cp := eventOfType(events, "checkpoint"); cp != nil {
		t.Fatalf("checkpoint for a workspace-root turn: %v", cp)
	}
	if turns, err := f.store.List(f.sessionID); err != nil || len(turns) != 0 {
		t.Fatalf("turns = %v %v, want none", turns, err)
	}
}

// A session with no folder of its own works in its artifacts directory,
// which is recorded like any other.
func TestChatTurnInArtifactsDirIsCheckpointed(t *testing.T) {
	f := newCheckpointFixture(t)
	bare, err := f.sessions.Create("no folder")
	if err != nil {
		t.Fatal(err)
	}
	f.sessionID = bare.ID
	events := f.chat(t, &workDirEditingClient{edit: func(dir string) {
		writeTestFile(t, filepath.Join(dir, "report.md"), "# report\n")
	}}, f.store)
	if cp := eventOfType(events, "checkpoint"); cp == nil || cp["files"] != float64(1) {
		t.Fatalf("checkpoint event = %v, want 1 file", cp)
	}
}

func TestSameDir(t *testing.T) {
	dir := t.TempDir()
	if !sameDir(dir, dir+string(filepath.Separator)) || !sameDir(dir, filepath.Join(dir, "x", "..")) {
		t.Fatal("equivalent spellings of one directory differ")
	}
	if sameDir(dir, filepath.Join(dir, "x")) || sameDir("", dir) || sameDir(dir, " ") {
		t.Fatal("different or empty directories match")
	}
	target := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err == nil {
		if !sameDir(link, target) {
			t.Fatal("a directory and a symlink to it differ")
		}
	} else {
		t.Logf("symlink unavailable, skipping the link case: %v", err)
	}
	upper := filepath.Join(dir, "Work")
	lower := filepath.Join(dir, "work")
	saved := foldPathCase
	t.Cleanup(func() { foldPathCase = saved })
	foldPathCase = true
	if !sameDir(upper, lower) {
		t.Fatal("case differs on a case-insensitive volume")
	}
	foldPathCase = false
	if sameDir(upper, lower) {
		t.Fatal("case ignored on a case-sensitive volume")
	}
}
