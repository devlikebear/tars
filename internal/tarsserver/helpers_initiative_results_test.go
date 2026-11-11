package tarsserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestInitiativeResultQueueReportsAndGivesBack(t *testing.T) {
	root := t.TempDir()
	var written []queuedResult
	path := filepath.Join(root, "results.json")
	q := newInitiativeResultQueue(path, func(_ context.Context, r queuedResult) error {
		written = append(written, r)
		return nil
	}, zerolog.Nop())
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if q.Pending(context.Background(), now) != nil {
		t.Fatal("empty queue must have no report")
	}
	for i, r := range []queuedResult{
		{Kind: "agent_run", Name: "digest", Status: "completed", Summary: "wrote\nthree notes", At: now.Add(-3 * time.Hour), NoteSessionID: "main", Note: "[RUN SUMMARY] old"},
		{Kind: "cron", Name: "backup", Status: "failed", Summary: "disk full", At: now.Add(-10 * time.Minute)},
		{Kind: "agent_run", Status: "completed", At: now.Add(-9 * time.Minute)},
		{Kind: "cron", Name: "a", Status: "completed", At: now.Add(-8 * time.Minute)},
		{Kind: "cron", Name: "b", Status: "completed", At: now.Add(-7 * time.Minute)},
	} {
		if err := q.Enqueue(r); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	subject := q.Pending(context.Background(), now)
	if len(written) != 1 || written[0].Note != "[RUN SUMMARY] old" {
		t.Fatalf("expired result fallback = %+v, want the old note written back", written)
	}
	if subject == nil {
		t.Fatal("no report for queued results")
	}
	lines := strings.Split(subject.Text, "\n")
	want := []string{`cron job "backup" failed: disk full`, "agent run completed", `cron job "a" completed`, "and 1 more"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("report lines = %q, want %q", lines, want)
	}
	if strings.Count(subject.ID, ",") != 3 {
		t.Fatalf("report ids = %q, want all four", subject.ID)
	}

	// A fresh queue reads the same file back.
	reread := newInitiativeResultQueue(path, nil, zerolog.Nop())
	if reread.Len() != 4 {
		t.Fatalf("reread len = %d", reread.Len())
	}
	q.Done(subject.ID)
	q.Done(" , ")
	if q.Len() != 0 {
		t.Fatalf("len after Done = %d", q.Len())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty queue file still there: %v", err)
	}

	if err := q.Enqueue(queuedResult{Kind: "agent_run", Status: "failed", At: now, NoteSessionID: "main", Note: "n2"}); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(queuedResult{Kind: "cron", Status: "failed", At: now}); err != nil {
		t.Fatal(err)
	}
	written = nil
	q.FlushAll(context.Background())
	if q.Len() != 0 || len(written) != 1 || written[0].Note != "n2" {
		t.Fatalf("flush wrote %+v, len %d", written, q.Len())
	}
}

func TestInitiativeResultQueueIgnoresCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	q := newInitiativeResultQueue(path, nil, zerolog.Nop())
	if q.Len() != 0 {
		t.Fatalf("corrupt queue len = %d", q.Len())
	}
}

func TestAgentRunReportable(t *testing.T) {
	run := func(mut func(*agentruntime.FinishedRun)) agentruntime.FinishedRun {
		f := agentruntime.FinishedRun{Run: agentruntime.Run{SessionID: "worker", Status: agentruntime.RunStatusCompleted}, NoteSessionID: "main"}
		mut(&f)
		return f
	}
	tests := []struct {
		name string
		f    agentruntime.FinishedRun
		want bool
	}{
		{"unawaited run with a main note", run(func(*agentruntime.FinishedRun) {}), true},
		{"unawaited run with no note", run(func(f *agentruntime.FinishedRun) { f.NoteSessionID = "" }), true},
		{"failed run", run(func(f *agentruntime.FinishedRun) { f.Run.Status = agentruntime.RunStatusFailed }), true},
		{"a turn waited for it", run(func(f *agentruntime.FinishedRun) { f.Awaited = true }), false},
		{"child step of a flow", run(func(f *agentruntime.FinishedRun) { f.Run.ParentRunID = "root" }), false},
		{"canceled", run(func(f *agentruntime.FinishedRun) { f.Run.Status = agentruntime.RunStatusCanceled }), false},
		{"note bound for another session", run(func(f *agentruntime.FinishedRun) { f.NoteSessionID = "other" }), false},
		{"ran in the main session itself", run(func(f *agentruntime.FinishedRun) { f.Run.SessionID = "main"; f.NoteSessionID = "" }), false},
	}
	for _, tt := range tests {
		if got := agentRunReportable(tt.f, "main"); got != tt.want {
			t.Fatalf("%s: agentRunReportable = %v, want %v", tt.name, got, tt.want)
		}
	}
	if agentRunReportable(run(func(*agentruntime.FinishedRun) {}), "") {
		t.Fatal("no main session must report nothing")
	}
}

func TestInitiativeAgentResultRouter(t *testing.T) {
	q := newInitiativeResultQueue(filepath.Join(t.TempDir(), "results.json"), nil, zerolog.Nop())
	if initiativeAgentResultRouter(nil, "main", time.Now) != nil {
		t.Fatal("nil queue must give no router")
	}
	router := initiativeAgentResultRouter(q, "main", time.Now)
	if router(agentruntime.FinishedRun{Run: agentruntime.Run{SessionID: "w", Status: agentruntime.RunStatusCompleted}, Awaited: true}) {
		t.Fatal("router took an awaited run")
	}
	if !router(agentruntime.FinishedRun{Run: agentruntime.Run{SessionID: "w", Agent: "scout", Status: agentruntime.RunStatusFailed, Error: "boom"}, NoteSessionID: "main", Note: "note"}) {
		t.Fatal("router declined a reportable run")
	}
	subject := q.Pending(context.Background(), time.Now())
	if subject == nil || subject.Text != `agent run "scout" failed: boom` {
		t.Fatalf("subject = %+v", subject)
	}
}

func TestCronResultReportable(t *testing.T) {
	job := func(mode, sessionID string) cron.Job {
		return cron.Job{Name: "j", DeliveryMode: mode, SessionID: sessionID}
	}
	tests := []struct {
		name      string
		job       cron.Job
		succeeded bool
		want      bool
	}{
		{"daily_log success has no session note", job("daily_log", ""), true, true},
		{"default mode is daily_log", job("", ""), true, true},
		{"session success already wrote [CRON]", job("session", ""), true, false},
		{"both success already wrote [CRON]", job("both", ""), true, false},
		{"none is never reported", job("none", ""), false, false},
		{"failure writes no note", job("session", ""), false, true},
		{"bound to its own session", job("daily_log", "s1"), false, false},
		{"reminders belong elsewhere", reminderJob(t, "r", "stretch"), true, false},
	}
	for _, tt := range tests {
		if got := cronResultReportable(tt.job, tt.succeeded); got != tt.want {
			t.Fatalf("%s: cronResultReportable = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestWithCronResultReport(t *testing.T) {
	q := newInitiativeResultQueue(filepath.Join(t.TempDir(), "results.json"), nil, zerolog.Nop())
	if withCronResultReport(nil, q, time.Now) != nil {
		t.Fatal("nil runner must stay nil")
	}
	runErr := errors.New("prompt failed")
	runner := withCronResultReport(func(_ context.Context, job cron.Job) (string, error) {
		if job.Name == "bad" {
			return "", runErr
		}
		return "## Title\nall green", nil
	}, q, time.Now)
	if resp, err := runner(context.Background(), cron.Job{Name: "ok", DeliveryMode: "daily_log"}); err != nil || resp == "" {
		t.Fatalf("runner = %q %v", resp, err)
	}
	if _, err := runner(context.Background(), cron.Job{Name: "bad", DeliveryMode: "session"}); !errors.Is(err, runErr) {
		t.Fatalf("runner error = %v", err)
	}
	if _, err := runner(context.Background(), cron.Job{Name: "noted", DeliveryMode: "session"}); err != nil {
		t.Fatal(err)
	}
	subject := q.Pending(context.Background(), time.Now())
	if subject == nil || subject.Text != "cron job \"ok\" completed: all green\ncron job \"bad\" failed: prompt failed" {
		t.Fatalf("subject = %+v", subject)
	}
}

func TestFollowUpSubject(t *testing.T) {
	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, loc)
	yesterday := now.Add(-12 * time.Hour)
	sessions := []session.Session{
		{ID: "s-goal", Title: "Docs", UpdatedAt: yesterday, Goal: &session.SessionGoal{Description: "finish the\nguide", Status: session.SessionGoalStatusActive}},
		{ID: "s-today", Title: "Today", UpdatedAt: now.Add(-time.Hour), Goal: &session.SessionGoal{Description: "x", Status: session.SessionGoalStatusActive}},
		{ID: "s-old", Title: "Old", UpdatedAt: now.Add(-5 * 24 * time.Hour), Goal: &session.SessionGoal{Description: "x", Status: session.SessionGoalStatusActive}},
		{ID: "s-done", Title: "Done", UpdatedAt: yesterday, Goal: &session.SessionGoal{Description: "x", Status: session.SessionGoalStatusSatisfied}},
		{ID: "s-hidden", Hidden: true, UpdatedAt: yesterday, Goal: &session.SessionGoal{Description: "x", Status: session.SessionGoalStatusActive}},
		{ID: "s-focus", Title: "Pay", UpdatedAt: yesterday, Goal: &session.SessionGoal{Description: "dup", Status: session.SessionGoalStatusActive}},
		{ID: "s-finished", UpdatedAt: yesterday},
		{ID: "s-blocked", UpdatedAt: yesterday},
	}
	finished := yesterday
	pipelines := []focuspipeline.Pipeline{
		{SessionID: "s-focus", Goal: "결제 리팩터링", Current: focuspipeline.StagePRReview, Stages: []focuspipeline.Stage{{ID: focuspipeline.StagePRReview, Status: focuspipeline.StatusActive}}, PR: &focuspipeline.PRInfo{Number: 12}, UpdatedAt: yesterday.Add(time.Hour)},
		{SessionID: "s-finished", Goal: "shipped", FinishedAt: &finished, UpdatedAt: yesterday},
		{SessionID: "s-blocked", Goal: "build it", Current: focuspipeline.StageBuild, Stages: []focuspipeline.Stage{{ID: focuspipeline.StageBuild, Status: focuspipeline.StatusBlocked}}, UpdatedAt: yesterday.Add(-time.Hour)},
		{SessionID: "s-unknown", Goal: "orphan", UpdatedAt: yesterday},
	}
	got := followUpSubject(now, loc, sessions, pipelines)
	if got == nil {
		t.Fatal("no follow-up")
	}
	want := `focus "결제 리팩터링": pull request #12 waiting on checks or review` + "\n" +
		`session "Docs" goal: finish the guide` + "\n" +
		`focus "build it": stopped while building (blocked)`
	if got.Text != want {
		t.Fatalf("follow-up =\n%s\nwant\n%s", got.Text, want)
	}
	if !got.At.Equal(yesterday.Add(time.Hour)) || got.ID != "" {
		t.Fatalf("subject = %+v", got)
	}
	if followUpSubject(now, loc, sessions[1:2], nil) != nil {
		t.Fatal("work touched today is not a follow-up")
	}
	many := []session.Session{}
	for i := 0; i < 5; i++ {
		many = append(many, session.Session{ID: string(rune('a' + i)), UpdatedAt: yesterday.Add(time.Duration(i) * time.Minute), Goal: &session.SessionGoal{Description: "g", Status: session.SessionGoalStatusActive}})
	}
	if got := followUpSubject(now, nil, many, nil); got == nil || !strings.HasSuffix(got.Text, "and 2 more") || !strings.Contains(got.Text, `session "session" goal: g`) {
		t.Fatalf("capped follow-up = %+v", got)
	}
}

func TestFollowUpStage(t *testing.T) {
	stage := func(kind focuspipeline.StageID, pr *focuspipeline.PRInfo) focuspipeline.Pipeline {
		return focuspipeline.Pipeline{Current: kind, Stages: []focuspipeline.Stage{{ID: kind, Status: focuspipeline.StatusActive}}, PR: pr}
	}
	for kind, want := range map[focuspipeline.StageID]string{
		focuspipeline.StagePlan:     "stopped at planning",
		focuspipeline.StageReview:   "stopped in review",
		focuspipeline.StagePR:       "ready to open the pull request",
		focuspipeline.StagePRReview: "the pull request waiting on checks or review",
		"other":                     "unfinished",
	} {
		if got := followUpStage(stage(kind, nil)); got != want {
			t.Fatalf("stage %s = %q, want %q", kind, got, want)
		}
	}
	if got := followUpStage(stage(focuspipeline.StageMerge, &focuspipeline.PRInfo{Number: 7})); got != "pull request #7 waiting to merge" {
		t.Fatalf("merge = %q", got)
	}
}

func TestInitiativeFollowUpSourceReadsSessions(t *testing.T) {
	if initiativeFollowUpSource(nil, time.UTC) != nil {
		t.Fatal("nil store must give no source")
	}
	store := session.NewStore(t.TempDir())
	sess, err := store.Create("Refactor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetGoal(sess.ID, &session.SessionGoal{Description: "split the handler", Status: session.SessionGoalStatusActive}); err != nil {
		t.Fatal(err)
	}
	source := initiativeFollowUpSource(store, time.UTC)
	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	got := source(tomorrow)
	if got == nil || got.Text != `session "Refactor" goal: split the handler` {
		t.Fatalf("follow-up = %+v", got)
	}
}

func TestSessionObserverReadsResultAndFollowUpOnArrival(t *testing.T) {
	now := time.Now()
	calls := 0
	deps := sessionObserverDeps{
		Result:   func(context.Context, time.Time) *initiative.Subject { return &initiative.Subject{ID: "r"} },
		FollowUp: func(time.Time) *initiative.Subject { calls++; return &initiative.Subject{Text: "f"} },
		Presence: func() initiative.PresenceState {
			return initiative.PresenceState{LastAt: now, ArrivedAt: now.Add(-time.Minute)}
		},
	}
	obs, err := newSessionInitiativeObserver(deps).Observe(context.Background(), now)
	if err != nil || obs.Result == nil || obs.FollowUp == nil {
		t.Fatalf("obs = %+v err=%v", obs, err)
	}
	deps.Presence = func() initiative.PresenceState {
		return initiative.PresenceState{LastAt: now, ArrivedAt: now.Add(-time.Hour)}
	}
	obs, _ = newSessionInitiativeObserver(deps).Observe(context.Background(), now)
	if obs.FollowUp != nil || calls != 1 {
		t.Fatalf("follow-up read away from an arrival: %+v calls=%d", obs.FollowUp, calls)
	}
}

func TestInitiativeSpeakerShareResultMarksQueueDone(t *testing.T) {
	speaker, store, mainID, events := subjectSpeaker(t, nil)
	speaker.results = newInitiativeResultQueue(filepath.Join(t.TempDir(), "results.json"), nil, zerolog.Nop())
	if err := speaker.results.Enqueue(queuedResult{Kind: "cron", Name: "backup", Status: "failed", Summary: "disk full", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	subject := speaker.results.Pending(context.Background(), time.Now())
	outcome, err := speaker.Speak(context.Background(), initiative.SpeakRequest{
		Intent: initiative.IntentShareResult, EntryID: "e1", Now: time.Now(), Subject: subject, Channel: initiative.ChannelConsole,
	})
	if err != nil || !outcome.Delivered || outcome.Composed {
		t.Fatalf("outcome = %+v err=%v", outcome, err)
	}
	msgs, _ := session.ReadMessages(store.TranscriptPath(mainID))
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, `cron job "backup" failed: disk full`) || msgs[0].Initiative.Intent != "share_result" {
		t.Fatalf("message = %+v", msgs)
	}
	if speaker.results.Len() != 0 {
		t.Fatal("a delivered report must leave the queue")
	}
	if len(*events) != 1 || (*events)[0].Expression != "happy" {
		t.Fatalf("events = %+v", *events)
	}
	if initiativeSpeakExpression(initiative.IntentFollowUp) != "greeting" {
		t.Fatal("follow_up should greet")
	}
}

func TestBuildInitiativeRuntimeWiresShareResultAndFollowUp(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	results := newInitiativeResultQueue(filepath.Join(root, "results.json"), nil, zerolog.Nop())
	cfg := config.Config{}
	cfg.Initiative = config.InitiativeConfig{Enabled: true, Mode: "live", ShareResult: true, FollowUp: true}
	setup := buildInitiativeRuntime(initiativeSetupInputs{
		Config: cfg, WorkspaceDir: root, SessionStore: store, MainSessionID: main.ID, Results: results, Logger: zerolog.Nop(),
	})
	if !setup.ShareResultActive || setup.Speaker == nil || setup.Speaker.results != results {
		t.Fatalf("setup = %+v speaker=%+v", setup, setup.Speaker)
	}
	snap := setup.Runtime.Snapshot()
	if !snap.ShareResult.Enabled || !snap.FollowUp.Enabled {
		t.Fatalf("snapshot = %+v / %+v", snap.ShareResult, snap.FollowUp)
	}

	cfg.Initiative.Mode = "shadow"
	setup = buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: root, SessionStore: store, MainSessionID: main.ID, Results: results, Logger: zerolog.Nop()})
	if setup.ShareResultActive || setup.Speaker != nil || !setup.Runtime.Snapshot().FollowUp.Enabled {
		t.Fatalf("shadow: share_result must stay off, follow_up stays recorded: %+v", setup)
	}

	cfg.Initiative = config.InitiativeConfig{Enabled: true, Mode: "live"}
	setup = buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: root, SessionStore: store, MainSessionID: main.ID, Results: results, Logger: zerolog.Nop()})
	if setup.ShareResultActive || setup.Runtime.Snapshot().ShareResult.Enabled {
		t.Fatalf("off by default: %+v", setup)
	}
}
