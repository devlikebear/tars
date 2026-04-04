package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/project"
	"github.com/devlikebear/tars/internal/session"
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

func TestResolveCronTargetSessionID_EmptyTargetReturnsEmpty(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}

	sessionID, explicit, err := resolveCronTargetSessionID(store, cron.Job{ProjectID: "proj_demo"}, mainSession.ID)
	if err != nil {
		t.Fatalf("resolve empty target: %v", err)
	}
	if explicit {
		t.Fatalf("empty target should not be explicit")
	}
	if sessionID != "" {
		t.Fatalf("expected empty session id for implicit project target, got %q", sessionID)
	}
}

func TestDeliverCronResult_DeliversToTargetSession(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}

	now := time.Date(2026, 3, 7, 22, 0, 0, 0, time.UTC)
	job := cron.Job{
		ID:            "job_demo",
		Name:          "nightly writer",
		Prompt:        "write next chapter",
		Schedule:      "every:5m",
		ProjectID:     "proj_demo",
		SessionTarget: "main",
	}
	if err := deliverCronResult(root, store, job, mainSession.ID, false, "drafted episode 2 and updated plot beats", now, zerolog.Nop()); err != nil {
		t.Fatalf("deliver cron result: %v", err)
	}

	messages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read target transcript: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != "system" || !containsAll(messages[0].Content, "[CRON]", "response: drafted episode 2") {
		t.Fatalf("unexpected target transcript: %+v", messages)
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
	var seenAllowedTools []string
	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, promptText string, allowedTools []string) (string, error) {
			seenPrompt = promptText
			seenAllowedTools = append([]string(nil), allowedTools...)
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
	if len(seenAllowedTools) != 0 {
		t.Fatalf("did not expect project tool allowlist for project without tool policy, got %+v", seenAllowedTools)
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
		func(_ context.Context, _ string, promptText string, _ []string) (string, error) {
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

func TestCronJobRunner_RequiresFinalizedBriefForAutonomousProjectWork(t *testing.T) {
	root := t.TempDir()
	projectStore := project.NewStore(root, nil)
	goal := "write thriller"
	status := "collecting"
	if _, err := projectStore.UpdateBrief("brief-1", project.BriefUpdateInput{
		Goal:   &goal,
		Status: &status,
	}); err != nil {
		t.Fatalf("update brief: %v", err)
	}

	called := false
	runner := newCronJobRunnerWithNotify(
		root,
		session.NewStore(root),
		func(ctx context.Context, runLabel string, promptText string, _ []string) (string, error) {
			called = true
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		"",
		0,
		nil,
	)

	_, err := runner(context.Background(), cron.Job{
		ID:       "job-1",
		Name:     "writer",
		Prompt:   "현재 활성 세션의 소설 프로젝트 brief_id=brief-1 를 이어서 진행하라.",
		Schedule: "every:5m",
	})
	if err == nil {
		t.Fatalf("expected brief validation error")
	}
	if !strings.Contains(err.Error(), "brief brief-1 is not finalized") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatalf("expected runPrompt not to be called")
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
		func(_ context.Context, _ string, _ string, _ []string) (string, error) {
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

func TestCronJobRunner_EmitsErrorNotificationOnContamination(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}

	events := make([]notificationEvent, 0, 1)
	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, _ string, _ []string) (string, error) {
			return `{"command":"python3 -V","timeout_ms":1000}`, nil
		},
		zerolog.Nop(),
		func(_ context.Context, evt notificationEvent) {
			events = append(events, evt)
		},
		mainSession.ID,
		2,
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
	if len(events) != 1 {
		t.Fatalf("expected one notification event, got %+v", events)
	}
	if events[0].Severity != "error" {
		t.Fatalf("expected error severity, got %+v", events[0])
	}
	if !containsAll(events[0].Title, "Cron failed", "nightly writer") {
		t.Fatalf("unexpected notification title: %+v", events[0])
	}
	if strings.TrimSpace(events[0].OpenPath) == "" {
		t.Fatalf("expected contamination failure to persist an artifact path, got %+v", events[0])
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
	if evt.JobID != "job_demo" || evt.ProjectID != "project-134127" || evt.SessionID != "sess-main" {
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
		func(_ context.Context, _ string, _ string, _ []string) (string, error) {
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

func TestCronJobRunner_ProjectToolPolicyAddsShellToolsToAllowlist(t *testing.T) {
	root := t.TempDir()
	projectStore := project.NewStore(root, nil)
	item, err := projectStore.Create(project.CreateInput{
		Name: "Ops Demo",
		Type: "operations",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	updated, err := projectStore.Update(item.ID, project.UpdateInput{
		ToolsAllowGroups:   []string{"files", "shell"},
		ToolsAllowPatterns: []string{"^project_"},
	})
	if err != nil {
		t.Fatalf("update project policy: %v", err)
	}

	var seenAllowedTools []string
	runner := newCronJobRunnerWithNotify(
		root,
		session.NewStore(root),
		func(_ context.Context, _ string, _ string, allowedTools []string) (string, error) {
			seenAllowedTools = append([]string(nil), allowedTools...)
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		"",
		0,
		nil,
	)

	if _, err := runner(context.Background(), cron.Job{
		ID:        "job_demo",
		Name:      "triage logs",
		Prompt:    "inspect logs",
		Schedule:  "every:5m",
		ProjectID: updated.ID,
	}); err != nil {
		t.Fatalf("run cron job: %v", err)
	}

	if !slices.Contains(seenAllowedTools, "exec") {
		t.Fatalf("expected shell allowlist to include exec, got %+v", seenAllowedTools)
	}
	if !slices.Contains(seenAllowedTools, "read_file") {
		t.Fatalf("expected files allowlist to include read_file, got %+v", seenAllowedTools)
	}
	if !slices.Contains(seenAllowedTools, "project") {
		t.Fatalf("expected project tool allowlist to retain project, got %+v", seenAllowedTools)
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
