package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/initiative"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

var dayKST = time.FixedZone("KST", 9*3600)

func dayAt(h, m int) time.Time { return time.Date(2026, 11, 11, h, m, 0, 0, dayKST) }

func TestOpenWorkAndRecapFacts(t *testing.T) {
	now := dayAt(22, 10)
	earlier := dayAt(9, 0)
	yesterday := now.Add(-24 * time.Hour)
	sessions := []session.Session{
		{ID: "s1", Title: "Fix login", UpdatedAt: now.Add(-time.Hour)},
		{ID: "s2", Title: "Docs", UpdatedAt: now.Add(-2 * time.Hour), Goal: &session.SessionGoal{Description: "write the guide", Status: session.SessionGoalStatusActive}},
		{ID: "s3", Title: "", UpdatedAt: now, Goal: &session.SessionGoal{Description: "ship it", Status: session.SessionGoalStatusSatisfied, LastJudgedAt: &earlier}},
		{ID: "s4", Title: "Old goal", Goal: &session.SessionGoal{Description: "x", Status: session.SessionGoalStatusSatisfied, LastJudgedAt: &yesterday}},
		{ID: "h1", Title: "hidden", Hidden: true, Goal: &session.SessionGoal{Description: "y", Status: session.SessionGoalStatusActive}},
		{ID: "w1", Title: "worker", Kind: "worker"},
		{ID: "s5", Title: "Released", UpdatedAt: now},
		{ID: "s6", Title: "With pipeline and goal", UpdatedAt: now, Goal: &session.SessionGoal{Description: "z", Status: session.SessionGoalStatusActive}},
	}
	pipelines := []focuspipeline.Pipeline{
		{SessionID: "s1", Goal: "login", UpdatedAt: now.Add(-time.Hour)},
		{SessionID: "s5", Goal: "release", UpdatedAt: now, FinishedAt: &earlier},
		{SessionID: "w1", Goal: "worker pipeline", UpdatedAt: now},
		{SessionID: "s6", Goal: "pipeline goal", UpdatedAt: now.Add(-30 * time.Minute)},
	}
	open := openWork(sessions, pipelines)
	if len(open) != 3 || open[0].Href != "/console/focus/s6" || open[1].Href != "/console/focus/s1" || open[2].Href != "/console/chat/s2" {
		t.Fatalf("open = %+v", open)
	}
	if open[1].Title != "Fix login" || open[2].Title != "Docs" {
		t.Fatalf("titles = %+v", open)
	}

	failedAt, fineAt, oldAt := dayAt(3, 0), dayAt(4, 0), yesterday
	jobs := []cron.Job{
		{ID: "c1", Name: "backup", LastRunAt: &failedAt, LastRunError: "exit 1"},
		{ID: "c2", Name: "fine", LastRunAt: &fineAt},
		{ID: "c3", Name: "old", LastRunAt: &oldAt, LastRunError: "boom"},
	}
	src := initiativeDaySources{CronJobs: func() ([]cron.Job, error) { return jobs, nil }, Location: dayKST}
	// No store: only the cron failure.
	got := src.recap(now)
	if got == nil || len(got.Failed) != 1 || got.Failed[0].Title != "backup" || len(got.Done) != 0 {
		t.Fatalf("recap without sessions = %+v", got)
	}
	if (initiativeDaySources{}).recap(now) != nil {
		t.Fatal("nothing at all must be nil")
	}
	full := recapFacts(now, sessions, pipelines, jobs)
	if len(full.Done) != 2 || full.Done[0].Href != "/console/focus/s5" || full.Done[0].Title != "Released" ||
		full.Done[1].Href != "/console/chat/s3" || full.Done[1].Title != "ship it" {
		t.Fatalf("done = %+v", full.Done)
	}
	if len(full.Open) != 3 || len(full.Failed) != 1 {
		t.Fatalf("recap = %+v", full)
	}
	store := session.NewStore(t.TempDir())
	src.Store = store
	if facts := src.recap(now); facts == nil || len(facts.Failed) != 1 {
		t.Fatalf("recap with an empty store = %+v", facts)
	}
}

func TestDaySourcesReadStores(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	sess := plannedFocusSession(t, store)
	goal, err := store.Create("Docs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetGoal(goal.ID, &session.SessionGoal{Description: "write the guide"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(dayKST)
	day := localDayStart(now)
	logDir := filepath.Join(workspace, "memory")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	log := strings.Join([]string{
		`chat session=a user="hi" assistant="hello"`,
		`chat session=a user="more" assistant="ok"`,
		`chat session=b user="x" assistant="y"`,
		`cron job=backup id=c1 schedule=0 3 * * * response="done"`,
		`compaction flush: something`,
		``,
	}, "\n")
	if err := os.WriteFile(filepath.Join(logDir, day.AddDate(0, 0, -1).Format("2006-01-02")+".md"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	created := now.Add(-time.Hour)
	jobs := []cron.Job{
		{ID: "c1", Name: "evening", Enabled: true, Schedule: "every:30m", CreatedAt: created},
		{ID: "c2", Name: "tomorrow", Enabled: true, Schedule: "at:" + day.AddDate(0, 0, 2).UTC().Format(time.RFC3339), CreatedAt: created},
		{ID: "c3", Name: "", Enabled: true, Schedule: "at:" + now.Add(time.Minute).UTC().Format(time.RFC3339), CreatedAt: created},
	}
	src := initiativeDaySources{Store: store, CronJobs: func() ([]cron.Job, error) { return jobs, nil }, WorkspaceDir: workspace, Location: dayKST}
	facts := src.morning(now)
	if facts.Yesterday != (initiative.DayLog{ChatTurns: 3, Sessions: 2, CronRuns: 1}) {
		t.Fatalf("yesterday = %+v", facts.Yesterday)
	}
	if len(facts.Open) != 2 {
		t.Fatalf("open = %+v", facts.Open)
	}
	hrefs := facts.Open[0].Href + " " + facts.Open[1].Href
	if !strings.Contains(hrefs, "/console/focus/"+sess.ID) || !strings.Contains(hrefs, "/console/chat/"+goal.ID) {
		t.Fatalf("open hrefs = %q", hrefs)
	}
	// A run due in a minute is still today unless now is a minute from
	// midnight; the interval job always runs.
	names := []string{}
	for _, run := range facts.Cron {
		names = append(names, run.Name)
	}
	if len(facts.Cron) == 0 || facts.Cron[len(facts.Cron)-1].Name != "evening" || strings.Contains(strings.Join(names, ","), "tomorrow") {
		t.Fatalf("cron = %+v", facts.Cron)
	}
	if got := readDayLog("", day); got != (initiative.DayLog{}) {
		t.Fatalf("no workspace = %+v", got)
	}
	if got := readDayLog(workspace, day.AddDate(0, 0, -5)); got != (initiative.DayLog{}) {
		t.Fatalf("missing file = %+v", got)
	}
	if recap := src.recap(now); recap == nil || len(recap.Open) != 2 {
		t.Fatalf("recap = %+v", recap)
	}
}

func TestTodayCronRunsOrder(t *testing.T) {
	now := dayAt(8, 0)
	end := dayAt(0, 0).AddDate(0, 0, 1)
	jobs := []cron.Job{
		{ID: "late", Name: "digest", Enabled: true, Schedule: "0 18 * * *", Timezone: "Asia/Seoul"},
		{ID: "z", Name: "zeta", Enabled: true, Schedule: "every:1h"},
		{ID: "early", Name: "backup", Enabled: true, Schedule: "0 9 * * *", Timezone: "Asia/Seoul"},
		{ID: "a", Name: "alpha", Enabled: true, Schedule: "@every 5m"},
		{ID: "off", Name: "off", Schedule: "0 10 * * *"},
	}
	runs := todayCronRuns(jobs, now, end)
	var got []string
	for _, r := range runs {
		got = append(got, r.Name)
	}
	if strings.Join(got, ",") != "backup,digest,alpha,zeta" {
		t.Fatalf("order = %v", got)
	}
}

func daySpeaker(t *testing.T, morning func(time.Time) initiative.MorningFacts, brief *initiative.BriefStore) (*initiativeSpeaker, *session.Store, string, *[]notificationEvent) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	var events []notificationEvent
	return &initiativeSpeaker{
		mainSessionID: main.ID,
		store:         store,
		composer:      &initiative.SpeechComposer{Client: &speakStubClient{resp: speakTextResponse("Good morning.")}},
		location:      dayKST,
		notify:        func(_ context.Context, evt notificationEvent) { events = append(events, evt) },
		logger:        zerolog.Nop(),
		claim:         alwaysClaims,
		brief:         brief,
		morning:       morning,
	}, store, main.ID, &events
}

func TestInitiativeSpeakerMorningBrief(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	facts := func(time.Time) initiative.MorningFacts {
		return initiative.MorningFacts{
			Cron:      []initiative.CronRun{{Name: "digest", At: dayAt(18, 0)}},
			Yesterday: initiative.DayLog{ChatTurns: 2, Sessions: 1},
			Open: []initiative.WorkItem{
				{Title: "Fix login", Stage: "review", Href: "/console/focus/p1"},
				{Title: "Docs", Href: "/console/chat/d1"},
			},
		}
	}

	t.Run("greet: arrival lines first, then the morning lines without repeating a pipeline", func(t *testing.T) {
		speaker, _, _, events := daySpeaker(t, facts, arrivedBriefStore(t, now))
		out, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet, EntryID: "e1", Now: now})
		if err != nil || !out.Delivered || !out.Composed {
			t.Fatalf("outcome = %+v err %v", out, err)
		}
		brief := (*events)[0].Brief
		if len(brief) != 4 || brief[0].Kind != "waiting" || brief[1].Kind != "schedule" || brief[2].Kind != "yesterday" || brief[3].Kind != "open" {
			t.Fatalf("brief = %+v", brief)
		}
		if brief[3].Text["en"] != `In progress: "Docs"` || brief[3].Text["ko"] != `진행 중: "Docs"` || brief[3].Href != "/console/chat/d1" {
			t.Fatalf("open line = %+v", brief[3])
		}
		if brief[1].Text["en"] != `Cron today: "digest" at 18:00` || brief[2].Text["ko"] != "어제: 세션 1곳에서 대화 2번" {
			t.Fatalf("lines = %+v", brief)
		}
	})
	t.Run("follow_up: no open-work line and no arrival lines", func(t *testing.T) {
		speaker, store, mainID, events := daySpeaker(t, facts, arrivedBriefStore(t, now))
		req := initiative.SpeakRequest{Intent: initiative.IntentFollowUp, EntryID: "e2", Now: now, Subject: &initiative.Subject{Text: `focus "Fix login": stopped in review`}}
		if out, err := speaker.Speak(context.Background(), req); err != nil || !out.Delivered {
			t.Fatalf("outcome = %+v err %v", out, err)
		}
		brief := (*events)[0].Brief
		if len(brief) != 2 || brief[0].Kind != "schedule" || brief[1].Kind != "yesterday" {
			t.Fatalf("brief = %+v", brief)
		}
		msgs, _ := session.ReadMessages(store.TranscriptPath(mainID))
		if len(msgs) != 1 || strings.Contains(msgs[0].Content, "Cron today") {
			t.Fatalf("transcript = %+v, the brief stays out of it", msgs)
		}
	})
	t.Run("off: the greet carries only the arrival lines", func(t *testing.T) {
		speaker, _, _, events := daySpeaker(t, nil, nil)
		if _, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentGreet, EntryID: "e3", Now: now}); err != nil {
			t.Fatal(err)
		}
		if (*events)[0].Brief != nil {
			t.Fatalf("brief = %+v", (*events)[0].Brief)
		}
	})
}

func TestInitiativeSpeakerEveningRecap(t *testing.T) {
	now := dayAt(22, 10)
	recap := &initiative.RecapFacts{
		Done:   []initiative.WorkItem{{Title: "Ship docs", Href: "/console/focus/a"}},
		Failed: []initiative.WorkItem{{Title: "backup", Href: "/console/cron"}},
	}
	for _, tt := range []struct {
		name   string
		recent []initiative.UserMessage
		first  string
	}{
		{"english", []initiative.UserMessage{{Text: "thanks"}}, "Here's today's wrap-up."},
		{"korean", []initiative.UserMessage{{Text: "고마워"}}, "오늘 하루 정리예요."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stub := &speakStubClient{resp: speakTextResponse("never used")}
			speaker, store, mainID, events := daySpeaker(t, nil, nil)
			speaker.composer = &initiative.SpeechComposer{Client: stub}
			out, err := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentEveningRecap, EntryID: "r1", Now: now, SendsText: true, RecentUser: tt.recent, Recap: recap})
			if err != nil || !out.Delivered || out.Composed {
				t.Fatalf("outcome = %+v err %v, want delivered without composing", out, err)
			}
			if stub.messages != nil {
				t.Fatalf("composer called with %d messages", len(stub.messages))
			}
			msgs, _ := session.ReadMessages(store.TranscriptPath(mainID))
			if len(msgs) != 1 || !strings.HasPrefix(msgs[0].Content, tt.first+"\n") || strings.Count(msgs[0].Content, "\n") != 2 {
				t.Fatalf("transcript = %+v", msgs)
			}
			if msgs[0].Initiative == nil || msgs[0].Initiative.Intent != string(initiative.IntentEveningRecap) {
				t.Fatalf("initiative metadata = %+v", msgs[0].Initiative)
			}
			evt := (*events)[0]
			if evt.Message != tt.first || len(evt.Brief) != 2 || evt.Brief[1].Severity != "block" || evt.Brief[0].Text["ko"] != `오늘 끝낸 일: "Ship docs"` {
				t.Fatalf("event = %+v", evt)
			}
		})
	}
	t.Run("no facts says nothing", func(t *testing.T) {
		speaker, _, _, _ := daySpeaker(t, nil, nil)
		out, _ := speaker.Speak(context.Background(), initiative.SpeakRequest{Intent: initiative.IntentEveningRecap, EntryID: "r2", Now: now})
		if out.Delivered || out.Reason != "speak_unavailable" {
			t.Fatalf("outcome = %+v", out)
		}
	})
}

func TestBuildInitiativeRuntimeWiresDayBrief(t *testing.T) {
	workspace := t.TempDir()
	store := session.NewStore(workspace)
	if _, err := store.EnsureMain(); err != nil {
		t.Fatal(err)
	}
	failedAt := dayAt(3, 0)
	cfg := config.Config{}
	cfg.Initiative = config.InitiativeConfig{Enabled: true, Mode: "live", MorningBrief: true, EveningRecap: true, EveningRecapWindow: "22:00-23:00", Timezone: "Asia/Seoul"}
	setup := buildInitiativeRuntime(initiativeSetupInputs{
		Config: cfg, WorkspaceDir: workspace, SessionStore: store, Logger: zerolog.Nop(),
		CronJobs: func() ([]cron.Job, error) {
			return []cron.Job{{ID: "c1", Name: "backup", LastRunAt: &failedAt, LastRunError: "exit 1"}}, nil
		},
		Now: func() time.Time { return dayAt(22, 10) },
	})
	if setup.Speaker == nil || setup.Speaker.morning == nil {
		t.Fatalf("speaker = %+v, want one with the morning brief", setup.Speaker)
	}
	snap := setup.Runtime.Snapshot()
	if !snap.MorningBrief || !snap.EveningRecap.Enabled || snap.EveningRecap.Window != "22:00-23:00" {
		t.Fatalf("snapshot = %+v / %+v", snap.MorningBrief, snap.EveningRecap)
	}
	if e := setup.Runtime.RunOnceAt(context.Background(), dayAt(22, 10)); !e.Signals.RecapDue {
		t.Fatalf("entry in the window = %+v, want a recap due", e)
	}

	cfg.Initiative.Mode = "shadow"
	shadow := buildInitiativeRuntime(initiativeSetupInputs{Config: cfg, WorkspaceDir: t.TempDir(), SessionStore: store, Logger: zerolog.Nop(),
		CronJobs: func() ([]cron.Job, error) {
			return []cron.Job{{ID: "c1", Name: "backup", LastRunAt: &failedAt, LastRunError: "exit 1"}}, nil
		}})
	if shadow.Runtime.Snapshot().MorningBrief {
		t.Fatal("shadow mode has no morning brief")
	}
	if e := shadow.Runtime.RunOnceAt(context.Background(), dayAt(15, 0)); e.Signals.RecapDue {
		t.Fatalf("entry outside the window = %+v", e)
	}
}
