package apihandlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

type focusReadsFixture struct {
	sessions *session.Store
	pipeline session.Session
	qa       session.Session
	plain    session.Session
	handler  http.Handler
}

func newFocusReadsFixture(t *testing.T, withCheckpoints bool) focusReadsFixture {
	t.Helper()
	dir := t.TempDir()
	sessions := session.NewStore(dir)
	pipe, err := sessions.Create("focus task")
	if err != nil {
		t.Fatal(err)
	}
	qa, err := sessions.Create("focus task Q&A")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := sessions.Create("a chat")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for _, m := range []struct {
		id, role, text string
	}{{pipe.ID, "user", "build it"}, {qa.ID, "user", "why this file?"}, {plain.ID, "user", "private"}} {
		if err := session.AppendMessage(sessions.TranscriptPath(m.id), session.Message{Role: m.role, Content: m.text, Timestamp: at}); err != nil {
			t.Fatal(err)
		}
	}
	p := focuspipeline.New(pipe.ID, "build it", at)
	p.QASessionID = qa.ID
	if err := focuspipeline.NewStore(filepath.Join(dir, "sessions")).Save(p); err != nil {
		t.Fatal(err)
	}
	var store *checkpoint.Store
	if withCheckpoints {
		store, err = checkpoint.Open(filepath.Join(dir, "_shared", "checkpoints"), checkpoint.Options{})
		if err != nil {
			t.Fatal(err)
		}
	}
	return focusReadsFixture{sessions: sessions, pipeline: pipe, qa: qa, plain: plain, handler: NewFocusReadsHandler(sessions, store, zerolog.Nop())}
}

func (f focusReadsFixture) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestFocusReadsSessionAndHistory(t *testing.T) {
	f := newFocusReadsFixture(t, true)
	base := "/v1/focus/pipelines/" + f.pipeline.ID

	rec := f.get(t, base+"/session")
	var sess session.Session
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &sess) != nil || sess.ID != f.pipeline.ID || sess.Title != "focus task" {
		t.Fatalf("session: %d %s", rec.Code, rec.Body.String())
	}

	for _, tc := range []struct{ query, want string }{{"", "build it"}, {"?session=qa", "why this file?"}} {
		rec := f.get(t, base+"/history"+tc.query)
		var messages []session.Message
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &messages) != nil || len(messages) != 1 || messages[0].Content != tc.want {
			t.Fatalf("history %q: %d %s", tc.query, rec.Code, rec.Body.String())
		}
	}
	if rec := f.get(t, base+"/history?session="+f.plain.ID); rec.Code != http.StatusBadRequest {
		t.Fatalf("another session by id: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFocusReadsReachOnlyPipelines(t *testing.T) {
	f := newFocusReadsFixture(t, true)
	// A chat with no pipeline, the Q&A session addressed directly, a
	// pipeline whose session is gone, and a malformed id are all unknown.
	if err := focuspipeline.NewStore(filepath.Join(f.sessions.WorkspaceDir(), "sessions")).Save(focuspipeline.New("ghost", "g", time.Now())); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{f.plain.ID, f.qa.ID, "ghost", "..%2F.."} {
		for _, suffix := range []string{"/session", "/history", "/checkpoints", "/checkpoints/t1/diff"} {
			if rec := f.get(t, "/v1/focus/pipelines/"+id+suffix); rec.Code != http.StatusNotFound {
				t.Fatalf("%s%s: %d %s", id, suffix, rec.Code, rec.Body.String())
			}
		}
	}
	// Reads only.
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/focus/pipelines/"+f.pipeline.ID+"/history", strings.NewReader("{}")))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post: %d", rec.Code)
	}
}

func TestFocusReadsCheckpoints(t *testing.T) {
	f := newFocusReadsFixture(t, true)
	base := "/v1/focus/pipelines/" + f.pipeline.ID
	rec := f.get(t, base+"/checkpoints")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"turns":[]`) || !strings.Contains(rec.Body.String(), `"reverts":[]`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.get(t, base+"/checkpoints/nope/diff?scope=turn"); rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown turn: %d %s", rec.Code, rec.Body.String())
	}

	// No checkpoint store (it failed to open): an empty list, no diffs.
	none := newFocusReadsFixture(t, false)
	base = "/v1/focus/pipelines/" + none.pipeline.ID
	if rec := none.get(t, base+"/checkpoints"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"turns":[]`) {
		t.Fatalf("no store list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := none.get(t, base+"/checkpoints/t1/diff"); rec.Code != http.StatusNotFound {
		t.Fatalf("no store diff: %d", rec.Code)
	}
}

func TestFocusReadsQAHistoryBeforeAndAfterItsSession(t *testing.T) {
	f := newFocusReadsFixture(t, false)
	pipelines := focuspipeline.NewStore(filepath.Join(f.sessions.WorkspaceDir(), "sessions"))
	p, _, err := pipelines.Get(f.pipeline.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, qa := range map[string]string{"no question yet": "", "session gone": "deleted-qa"} {
		p.QASessionID = qa
		if err := pipelines.Save(p); err != nil {
			t.Fatal(err)
		}
		rec := f.get(t, "/v1/focus/pipelines/"+f.pipeline.ID+"/history?session=qa")
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestFocusReadsCheckpointErrors(t *testing.T) {
	h := &focusReads{logger: zerolog.Nop()}
	for _, tc := range []struct {
		err  error
		code int
	}{
		{checkpoint.ErrNotFound, http.StatusNotFound},
		{checkpoint.ErrSkipped, http.StatusConflict},
		{checkpoint.ErrInvalid, http.StatusBadRequest},
		{errors.New("disk"), http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		h.writeCheckpointError(rec, tc.err)
		if rec.Code != tc.code {
			t.Errorf("%v: %d, want %d", tc.err, rec.Code, tc.code)
		}
		if tc.code == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "disk") {
			t.Errorf("an internal error's text stays on the desk: %s", rec.Body.String())
		}
	}
}

func TestFocusReadsWithoutSessions(t *testing.T) {
	rec := httptest.NewRecorder()
	NewFocusReadsHandler(nil, nil, zerolog.Nop()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/focus/pipelines/x/session", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no sessions: %d", rec.Code)
	}
}
