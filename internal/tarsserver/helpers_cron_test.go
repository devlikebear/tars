package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/session"
	"github.com/rs/zerolog"
)

func TestResolveCronTargetSessionID_MainUsesConfiguredMainSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("create main session: %v", err)
	}
	otherSession, err := store.Create("other")
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	sessionID, explicit, err := resolveCronTargetSessionID(store, cron.Job{SessionTarget: "main"}, mainSession.ID)
	if err != nil {
		t.Fatalf("resolve main target: %v", err)
	}
	if explicit {
		t.Fatalf("main target should not be treated as explicit session id")
	}
	if sessionID != mainSession.ID {
		t.Fatalf("expected main session %q, got %q (latest=%q)", mainSession.ID, sessionID, otherSession.ID)
	}
}

func TestResolveCronTargetSessionID_CurrentUsesConfiguredMainSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}

	sessionID, explicit, err := resolveCronTargetSessionID(store, cron.Job{SessionTarget: "current"}, mainSession.ID)
	if err != nil {
		t.Fatalf("resolve current target: %v", err)
	}
	if explicit {
		t.Fatalf("current target should not be treated as explicit session id")
	}
	if sessionID != mainSession.ID {
		t.Fatalf("expected main session id %q, got %q", mainSession.ID, sessionID)
	}
}

func TestResolveCronTargetSessionID_ProjectUsesWorkerSessionWhenImplicit(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}

	sessionID, explicit, err := resolveCronTargetSessionID(store, cron.Job{ProjectID: "proj_demo"}, mainSession.ID)
	if err != nil {
		t.Fatalf("resolve worker target: %v", err)
	}
	if explicit {
		t.Fatalf("worker target should not be explicit")
	}
	if sessionID == "" || sessionID == mainSession.ID {
		t.Fatalf("expected hidden worker session distinct from main, got %q", sessionID)
	}

	sess, err := store.Get(sessionID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	if sess.Kind != "worker" || !sess.Hidden {
		t.Fatalf("unexpected worker session metadata: %+v", sess)
	}
}

func TestDeliverCronResult_WorkerSessionGetsRawAndMainGetsSummary(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	worker, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker session: %v", err)
	}

	now := time.Date(2026, 3, 7, 22, 0, 0, 0, time.UTC)
	job := cron.Job{
		ID:        "job_demo",
		Name:      "nightly writer",
		Prompt:    "write next chapter",
		Schedule:  "every:5m",
		ProjectID: "proj_demo",
	}
	if err := deliverCronResult(root, store, job, worker.ID, false, "drafted episode 2 and updated plot beats", now, zerolog.Nop()); err != nil {
		t.Fatalf("deliver cron result: %v", err)
	}

	workerMessages, err := session.ReadMessages(store.TranscriptPath(worker.ID))
	if err != nil {
		t.Fatalf("read worker transcript: %v", err)
	}
	if len(workerMessages) != 1 || workerMessages[0].Role != "system" || !containsAll(workerMessages[0].Content, "[CRON]", "response: drafted episode 2") {
		t.Fatalf("unexpected worker transcript: %+v", workerMessages)
	}

	mainMessages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(mainMessages) != 1 || mainMessages[0].Role != "system" || !containsAll(mainMessages[0].Content, "[CRON SUMMARY]", "project_id: proj_demo", "status: completed", "drafted episode 2") {
		t.Fatalf("unexpected main transcript: %+v", mainMessages)
	}
}

func TestCronJobRunner_HiddenWorkerDoesNotInjectTargetSessionContext(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	worker, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker session: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(worker.ID), session.Message{
		Role:      "system",
		Content:   "[CRON]\nresponse: contaminated prior raw output",
		Timestamp: time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append worker message: %v", err)
	}

	var seenPrompt string
	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, promptText string) (string, error) {
			seenPrompt = promptText
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		"",
		0,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:        "job_demo",
		Name:      "nightly writer",
		Prompt:    "write next chapter",
		Schedule:  "every:1m",
		ProjectID: "proj_demo",
	})
	if err != nil {
		t.Fatalf("run cron job: %v", err)
	}
	if strings.Contains(seenPrompt, "TARGET_SESSION_CONTEXT:") {
		t.Fatalf("did not expect hidden worker session context in prompt, got %q", seenPrompt)
	}
}

func TestCronJobRunner_IncludesDefaultTelegramChatContext(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}

	var seenPrompt string
	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, promptText string) (string, error) {
			seenPrompt = promptText
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		func(_ context.Context) (string, error) {
			return "8432508298", nil
		},
	)

	_, err = runner(context.Background(), cron.Job{
		ID:       "job_demo",
		Name:     "telegram notify",
		Prompt:   "1분 뒤에 이 채널로 테스트 알림 보내줘",
		Schedule: "at:2026-03-08T06:17:09Z",
	})
	if err != nil {
		t.Fatalf("run cron job: %v", err)
	}
	if !containsAll(seenPrompt,
		"CRON_TELEGRAM_CONTEXT:",
		"default_paired_chat_available: true",
		"telegram_send",
		"8432508298",
	) {
		t.Fatalf("expected telegram context in prompt, got %q", seenPrompt)
	}
}

func TestCronJobRunner_RejectsPseudoToolContamination(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	worker, err := store.EnsureWorker("proj_demo")
	if err != nil {
		t.Fatalf("ensure worker session: %v", err)
	}

	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, _ string) (string, error) {
			return `{"command":"python3 -V","timeout_ms":1000}`, nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:        "job_demo",
		Name:      "nightly writer",
		Prompt:    "write next chapter",
		Schedule:  "every:1m",
		ProjectID: "proj_demo",
	})
	if err == nil {
		t.Fatal("expected pseudo-tool contamination error")
	}
	if !strings.Contains(err.Error(), "pseudo-tool contamination") {
		t.Fatalf("unexpected error: %v", err)
	}

	workerMessages, err := session.ReadMessages(store.TranscriptPath(worker.ID))
	if err != nil {
		t.Fatalf("read worker transcript: %v", err)
	}
	if len(workerMessages) != 0 {
		t.Fatalf("expected no worker transcript writes on contamination, got %+v", workerMessages)
	}

	mainMessages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(mainMessages) != 0 {
		t.Fatalf("expected no main transcript writes on contamination, got %+v", mainMessages)
	}
}

func TestPersistCronProjectArtifact_IncludesTelemetry(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 8, 1, 30, 0, 0, time.UTC)
	job := cron.Job{
		ID:        "job_demo",
		Name:      "nightly writer",
		Prompt:    "write next chapter",
		ProjectID: "proj_demo",
	}

	path, err := persistCronProjectArtifact(root, job, "drafted episode 2", now, cronRunTelemetry{
		PromptTokens:               120,
		SystemPromptTokens:         80,
		UserPromptTokens:           40,
		TargetSessionContextTokens: 0,
		TargetSessionContextUsed:   false,
		ToolCount:                  9,
		ResponseTokens:             22,
		ContaminationMarkers:       []string{"{\"command\":"},
	}, 0)
	if err != nil {
		t.Fatalf("persist artifact: %v", err)
	}

	expectedPath := filepath.Join(root, "projects", "proj_demo", "cron_runs", now.UTC().Format("20060102T150405Z")+"_job_demo.md")
	if path != expectedPath {
		t.Fatalf("expected artifact path %q, got %q", expectedPath, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	text := string(data)
	if !containsAll(text,
		"## Telemetry",
		"prompt_tokens: 120",
		"system_prompt_tokens: 80",
		"user_prompt_tokens: 40",
		"tool_count: 9",
		"response_tokens: 22",
		"contamination_markers: {\"command\":",
	) {
		t.Fatalf("artifact missing telemetry section:\n%s", text)
	}
}

func TestBuildCronNotificationEvent_UsesFriendlySummaryAndOpenPath(t *testing.T) {
	job := cron.Job{ID: "job_demo", Name: "novelist-1m", ProjectID: "project-134127"}
	evt := buildCronNotificationEvent(job, "info", "Cron completed", "# Result\n\n상태 문서를 갱신하고 1화 초안을 다듬었습니다.\n\n변경 파일:\n- `projects/project-134127/STATE.md`", "/tmp/cron.md", "sess-main")
	if evt.JobID != "job_demo" || evt.SessionID != "sess-main" {
		t.Fatalf("unexpected ids in notification: %+v", evt)
	}
	if evt.OpenPath != "/tmp/cron.md" {
		t.Fatalf("expected open path in notification, got %+v", evt)
	}
	if !containsAll(evt.Title, "Cron completed", "novelist-1m") {
		t.Fatalf("expected job name in title, got %q", evt.Title)
	}
	if !containsAll(evt.Message, "project-134127", "상태 문서를 갱신하고 1화 초안을 다듬었습니다") {
		t.Fatalf("expected friendly summary message, got %q", evt.Message)
	}
}

func TestTrimCronProjectArtifacts_KeepsNewestFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "proj_demo", "cron_runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cron_runs: %v", err)
	}
	names := []string{
		"20260308T010000Z_job_a.md",
		"20260308T010100Z_job_b.md",
		"20260308T010200Z_job_c.md",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := trimCronProjectArtifacts(dir, 2); err != nil {
		t.Fatalf("trim artifacts: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 artifacts left, got %d", len(entries))
	}
	got := entries[0].Name() + "," + entries[1].Name()
	if got != "20260308T010100Z_job_b.md,20260308T010200Z_job_c.md" {
		t.Fatalf("unexpected remaining artifacts: %s", got)
	}
}

func TestCronJobRunner_FailsWhenClaimedFileUpdateIsNotObserved(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	projectDir := filepath.Join(root, "projects", "proj_demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	statePath := filepath.Join(projectDir, "STATE.md")
	if err := os.WriteFile(statePath, []byte("phase: drafting\n"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, _ string) (string, error) {
			return "- `projects/proj_demo/TIMELINE_MAP.md` 추가\n- `STATE.md` 갱신", nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:        "job_demo",
		Name:      "nightly writer",
		Prompt:    "write next chapter",
		Schedule:  "every:1m",
		ProjectID: "proj_demo",
	})
	if err == nil {
		t.Fatal("expected claimed update verification error")
	}
	if !strings.Contains(err.Error(), "claimed file update not observed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
