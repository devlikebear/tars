package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestPlanSessionRetention(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	at := func(d time.Duration) time.Time { return now.Add(-d) }
	ptr := func(v time.Time) *time.Time { return &v }
	archived := func(id string, archivedAgo, updatedAgo time.Duration) session.Session {
		return session.Session{ID: id, UpdatedAt: at(updatedAgo), ArchivedAt: ptr(at(archivedAgo))}
	}
	sessions := []session.Session{
		{ID: "fresh", UpdatedAt: at(6 * day)},
		{ID: "idle", UpdatedAt: at(7 * day)},
		{ID: "idle-pinned", UpdatedAt: at(40 * day), PinnedAt: ptr(at(40 * day))},
		{ID: "idle-main", Kind: "main", UpdatedAt: at(40 * day)},
		{ID: "idle-default", UpdatedAt: at(40 * day)},
		{ID: "idle-in-use", UpdatedAt: at(40 * day)},
		{ID: "idle-subagent", Kind: "subagent", Hidden: true, UpdatedAt: at(9 * day)},
		archived("archived-recently", 10*day, 10*day),
		archived("archived-soon", 28*day, 28*day),
		archived("archived-due", 30*day, 30*day),
		archived("archived-but-used-since", 40*day, 2*day),
		{ID: "archived-pinned", UpdatedAt: at(40 * day), ArchivedAt: ptr(at(40 * day)), PinnedAt: ptr(at(40 * day))},
		{ID: "never-stamped"},
	}
	keep := func(s session.Session) bool { return s.ID == "idle-in-use" }
	ids := func(list []session.Session) string {
		out := make([]string, 0, len(list))
		for _, s := range list {
			out = append(out, s.ID)
		}
		return strings.Join(out, ",")
	}

	plan := planSessionRetention(sessions, sessionRetentionPolicyFromDays(7, 30), now, "idle-default", keep)
	if got := ids(plan.Archive); got != "idle,idle-subagent" {
		t.Fatalf("archive = %s", got)
	}
	if got := ids(plan.Delete); got != "archived-due" {
		t.Fatalf("delete = %s", got)
	}
	if got := ids(plan.Warn); got != "archived-soon" {
		t.Fatalf("warn = %s", got)
	}

	// Each step has its own switch.
	archiveOnly := planSessionRetention(sessions, sessionRetentionPolicyFromDays(7, 0), now, "idle-default", keep)
	if len(archiveOnly.Archive) != 2 || len(archiveOnly.Delete)+len(archiveOnly.Warn) != 0 {
		t.Fatalf("delete off: %+v", archiveOnly)
	}
	deleteOnly := planSessionRetention(sessions, sessionRetentionPolicyFromDays(0, 30), now, "idle-default", keep)
	if len(deleteOnly.Archive) != 0 || len(deleteOnly.Delete) != 1 {
		t.Fatalf("archive off: %+v", deleteOnly)
	}
	if sessionRetentionPolicyFromDays(0, -3).enabled() {
		t.Fatal("zero and negative days must turn retention off")
	}
}

// One session through its whole life against a real store: idle, archived
// by the sweep, warned about, then deleted with its task file, pipeline and
// worktree hand-off — while the main session and a pipeline still being
// driven are left alone.
func TestSessionRetentionSweepArchivesThenDeletes(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "sessions"))
	mainSess, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.Create("an old task")
	if err != nil {
		t.Fatal(err)
	}
	driven, err := store.Create("still running to its goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTasks(old.ID, session.SessionTasks{Tasks: []session.Task{{ID: "t1", Title: "do it", Status: "pending"}}}); err != nil {
		t.Fatal(err)
	}
	focus := focusStoreFor(store)
	if err := focus.Save(focuspipeline.New(old.ID, "an old task", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := focus.Save(focuspipeline.StartGoal(focuspipeline.New(driven.ID, "goal", time.Now()), 0, time.Now())); err != nil {
		t.Fatal(err)
	}
	attachSessionDeleteHooks(store, focusPipelineCleanup(store, zerolog.Nop()))
	longAgo := time.Now().Add(-8 * 24 * time.Hour)
	for _, id := range []string{mainSess.ID, old.ID, driven.ID} {
		if err := store.Touch(id, longAgo); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	var notices []notificationEvent
	var audits []ops.AutomationAuditEntry
	var retired []string
	clock := time.Now()
	r := &sessionRetention{
		store:  store,
		policy: sessionRetentionPolicyFromDays(7, 30),
		retire: func(_ context.Context, s session.Session) { retired = append(retired, s.ID) },
		notify: func(_ context.Context, e notificationEvent) {
			mu.Lock()
			notices = append(notices, e)
			mu.Unlock()
		},
		audit:  func(e ops.AutomationAuditEntry) { audits = append(audits, e) },
		logger: zerolog.Nop(),
		now:    func() time.Time { return clock },
	}

	if got := r.sweep(context.Background()); got.Archived != 1 || got.Deleted != 0 {
		t.Fatalf("first sweep = %+v, want one archived", got)
	}
	for id, wantArchived := range map[string]bool{old.ID: true, mainSess.ID: false, driven.ID: false} {
		sess, err := store.Get(id)
		if err != nil || (sess.ArchivedAt != nil) != wantArchived {
			t.Fatalf("session %s archived = %v err = %v, want %v", id, sess.ArchivedAt != nil, err, wantArchived)
		}
	}
	if got := r.sweep(context.Background()); got.Archived+got.Deleted+got.Warned != 0 {
		t.Fatalf("a second sweep the same day = %+v, want nothing", got)
	}

	clock = clock.Add(28 * 24 * time.Hour)
	if got := r.sweep(context.Background()); got.Warned != 1 || got.Deleted != 0 {
		t.Fatalf("sweep two days before the deletion = %+v, want one warning", got)
	}
	if got := r.sweep(context.Background()); got.Warned != 0 {
		t.Fatalf("the warning repeated: %+v", got)
	}

	clock = clock.Add(3 * 24 * time.Hour)
	if got := r.sweep(context.Background()); got.Deleted != 1 {
		t.Fatalf("sweep after 31 days archived = %+v, want one deleted", got)
	}
	if _, err := store.Get(old.ID); err == nil {
		t.Fatal("the session is still in the index")
	}
	for _, name := range []string{old.ID + ".jsonl", old.ID + ".tasks.json", old.ID + ".pipeline.json"} {
		if _, err := os.Stat(filepath.Dir(store.TranscriptPath(name)) + "/" + name); !os.IsNotExist(err) {
			t.Fatalf("%s was left behind (err = %v)", name, err)
		}
	}
	if len(retired) != 1 || retired[0] != old.ID {
		t.Fatalf("worktree hand-off = %v, want the deleted session", retired)
	}
	if _, ok, _ := focus.Get(driven.ID); !ok {
		t.Fatal("the driven pipeline was removed")
	}

	steps := make([]string, 0, len(audits))
	for _, a := range audits {
		if a.Action != "session_retention" || a.SessionID != old.ID || a.Result != "ok" {
			t.Fatalf("audit = %+v", a)
		}
		steps = append(steps, a.Details["step"].(string))
	}
	if strings.Join(steps, ",") != "archive,delete" {
		t.Fatalf("audited steps = %v", steps)
	}
	mu.Lock()
	defer mu.Unlock()
	titles := make([]string, 0, len(notices))
	for _, n := range notices {
		titles = append(titles, n.Title)
	}
	if strings.Join(titles, "|") != "Session cleanup|Archived sessions will be deleted|Session cleanup" {
		t.Fatalf("notifications = %v", titles)
	}
}

// A session someone went back to after it was archived is not deleted.
func TestSessionRetentionKeepsAnArchivedSessionUsedSince(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "sessions"))
	sess, err := store.Create("revisited")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetArchived(sess.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(sess.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r := &sessionRetention{store: store, policy: sessionRetentionPolicyFromDays(7, 30), logger: zerolog.Nop(),
		now: func() time.Time { return time.Now().Add(60 * 24 * time.Hour) }}
	if got := r.sweep(context.Background()); got.Deleted != 0 {
		t.Fatalf("sweep = %+v, want nothing deleted", got)
	}
	if _, err := store.Get(sess.ID); err != nil {
		t.Fatalf("the session is gone: %v", err)
	}

	// Off means off, and a nil sweeper is a no-op.
	off := &sessionRetention{store: store, logger: zerolog.Nop()}
	if got := off.sweep(context.Background()); got != (sessionRetentionResult{}) {
		t.Fatalf("disabled sweep = %+v", got)
	}
	off.start(context.Background())()
	var none *sessionRetention
	none.start(context.Background())()
}
