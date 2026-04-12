package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/research"
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

	sessionID, explicit, err := resolveCronTargetSessionID(store, cron.Job{}, mainSession.ID)
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

func TestResolveCronTargetSessionID_SessionBindingDefaultsToBoundSession(t *testing.T) {
	store := session.NewStore(t.TempDir())
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	boundSession, err := store.Create("monitor")
	if err != nil {
		t.Fatalf("create bound session: %v", err)
	}

	sessionID, explicit, err := resolveCronTargetSessionID(store, cron.Job{SessionID: boundSession.ID}, mainSession.ID)
	if err != nil {
		t.Fatalf("resolve bound session target: %v", err)
	}
	if explicit {
		t.Fatalf("session binding default target should not be treated as explicit")
	}
	if sessionID != boundSession.ID {
		t.Fatalf("expected bound session id %q, got %q", boundSession.ID, sessionID)
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
		SessionTarget: "main",
	}
	if err := deliverCronResult(context.Background(), root, store, job, mainSession.ID, false, "drafted episode 2 and updated plot beats", now, zerolog.Nop(), nil); err != nil {
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

func TestCronJobRunner_GlobalReminderDeliversToMainSessionAndTelegram(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	payload, err := cron.MergePayloadMeta(nil, cron.PayloadMeta{
		TaskType:        "reminder",
		ReminderMessage: "테스트 알림",
		TelegramChatID:  "101",
	})
	if err != nil {
		t.Fatalf("build reminder payload: %v", err)
	}

	sent := make([]string, 0, 1)
	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			t.Fatal("expected reminder cron to bypass llm runner")
			return "", nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		nil,
		func(_ context.Context, job cron.Job, reminderText string) error {
			if job.ID != "job_reminder" {
				t.Fatalf("unexpected reminder job: %+v", job)
			}
			sent = append(sent, reminderText)
			return nil
		},
	)

	response, err := runner(context.Background(), cron.Job{
		ID:            "job_reminder",
		Name:          "테스트 알림",
		Prompt:        "다음 알림을 보내기: 테스트 알림",
		Schedule:      "at:2026-03-08T06:17:09Z",
		SessionTarget: "main",
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("run reminder cron job: %v", err)
	}
	if response != "테스트 알림" {
		t.Fatalf("expected reminder response text, got %q", response)
	}
	if len(sent) != 1 || sent[0] != "테스트 알림" {
		t.Fatalf("expected telegram reminder send, got %+v", sent)
	}

	messages, err := session.ReadMessages(store.TranscriptPath(mainSession.ID))
	if err != nil {
		t.Fatalf("read main transcript: %v", err)
	}
	if len(messages) != 1 || !containsAll(messages[0].Content, "[REMINDER]", "message: 테스트 알림") {
		t.Fatalf("unexpected main reminder transcript: %+v", messages)
	}
}

func TestCronJobRunner_SessionReminderStaysInBoundSession(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	boundSession, err := store.Create("monitor")
	if err != nil {
		t.Fatalf("create bound session: %v", err)
	}
	payload, err := cron.MergePayloadMeta(nil, cron.PayloadMeta{
		TaskType:        "reminder",
		ReminderMessage: "세션 알림",
		TelegramChatID:  "101",
	})
	if err != nil {
		t.Fatalf("build reminder payload: %v", err)
	}

	externalSendCount := 0
	runner := newCronJobRunnerWithNotify(
		root,
		store,
		func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			t.Fatal("expected session reminder cron to bypass llm runner")
			return "", nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		nil,
		func(_ context.Context, _ cron.Job, _ string) error {
			externalSendCount++
			return nil
		},
	)

	_, err = runner(context.Background(), cron.Job{
		ID:        "job_session_reminder",
		Name:      "세션 알림",
		Prompt:    "다음 알림을 보내기: 세션 알림",
		Schedule:  "at:2026-03-08T06:17:09Z",
		SessionID: boundSession.ID,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("run session reminder cron job: %v", err)
	}
	if externalSendCount != 0 {
		t.Fatalf("expected session reminder to avoid external channel sends, got %d", externalSendCount)
	}

	messages, err := session.ReadMessages(store.TranscriptPath(boundSession.ID))
	if err != nil {
		t.Fatalf("read bound transcript: %v", err)
	}
	if len(messages) != 1 || !containsAll(messages[0].Content, "[REMINDER]", "message: 세션 알림") {
		t.Fatalf("unexpected bound reminder transcript: %+v", messages)
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
		func(_ context.Context, _ string, promptText string, allowedTools []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			seenPrompt = promptText
			seenAllowedTools = append([]string(nil), allowedTools...)
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		"",
		0,
		nil,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:       "job_demo",
		Name:     "nightly writer",
		Prompt:   "write next chapter",
		Schedule: "every:1m",
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
		func(_ context.Context, _ string, promptText string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
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
		nil,
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

func TestCronJobRunner_NoProjectPrerequisiteValidationAfterRemoval(t *testing.T) {
	root := t.TempDir()

	called := false
	runner := newCronJobRunnerWithNotify(
		root,
		session.NewStore(root),
		func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			called = true
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		"",
		0,
		nil,
		nil,
	)

	// After project removal, no brief validation occurs
	_, err := runner(context.Background(), cron.Job{
		ID:       "job-1",
		Name:     "writer",
		Prompt:   "현재 활성 세션의 소설 프로젝트 brief_id=brief-1 를 이어서 진행하라.",
		Schedule: "every:5m",
	})
	if err != nil {
		t.Fatalf("expected no error without brief validation, got %v", err)
	}
	if !called {
		t.Fatalf("expected runPrompt to be called")
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
		func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			return `{"command":"python3 -V","timeout_ms":1000}`, nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		nil,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:       "job_demo",
		Name:     "nightly writer",
		Prompt:   "write next chapter",
		Schedule: "every:1m",
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
		func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			return `{"command":"python3 -V","timeout_ms":1000}`, nil
		},
		zerolog.Nop(),
		func(_ context.Context, evt notificationEvent) {
			events = append(events, evt)
		},
		mainSession.ID,
		2,
		nil,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:       "job_demo",
		Name:     "nightly writer",
		Prompt:   "write next chapter",
		Schedule: "every:1m",
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
	// OpenPath is empty after project artifact persistence was removed
}

func TestPersistCronProjectArtifact_AppendsSessionArtifactLog(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 8, 1, 30, 0, 0, time.UTC)
	job := cron.Job{ID: "job_demo", Name: "nightly writer", Prompt: "write next chapter", SessionID: "sess-main"}
	path, err := persistCronProjectArtifact(root, job, "drafted episode 2", nil, now, cronRunTelemetry{ToolCount: 2}, 0)
	if err != nil {
		t.Fatalf("persist artifact: %v", err)
	}
	wantPath := filepath.Join(root, "artifacts", "sess-main", "cronjob-log.jsonl")
	if path != wantPath {
		t.Fatalf("expected session cron log path %q, got %q", wantPath, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session cron log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one session cron log line, got %d", len(lines))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("decode session cron log entry: %v", err)
	}
	if entry["job_id"] != "job_demo" || entry["session_id"] != "sess-main" {
		t.Fatalf("unexpected session cron log entry: %+v", entry)
	}
	if entry["status"] != "completed" {
		t.Fatalf("expected completed session cron log status, got %+v", entry)
	}
}

func TestPersistCronProjectArtifact_AppendsGlobalArtifactLog(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 8, 1, 35, 0, 0, time.UTC)
	job := cron.Job{ID: "job_global", Name: "nightly writer", Prompt: "write next chapter"}
	path, err := persistCronProjectArtifact(root, job, "", errors.New("boom"), now, cronRunTelemetry{}, 0)
	if err != nil {
		t.Fatalf("persist global artifact: %v", err)
	}
	wantPath := filepath.Join(root, "artifacts", "_global", "cronjob-log.jsonl")
	if path != wantPath {
		t.Fatalf("expected global cron log path %q, got %q", wantPath, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read global cron log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one global cron log line, got %d", len(lines))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("decode global cron log entry: %v", err)
	}
	if entry["scope"] != "global" || entry["status"] != "error" {
		t.Fatalf("unexpected global cron log entry: %+v", entry)
	}
}

func TestCronPromptRunner_UsesBoundSessionContext(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(root)
	mainSession, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main session: %v", err)
	}
	boundSession, err := store.Create("monitor")
	if err != nil {
		t.Fatalf("create bound session: %v", err)
	}
	if err := store.SetToolConfig(boundSession.ID, &session.SessionToolConfig{
		ToolsCustom:  true,
		ToolsEnabled: []string{"read_file"},
	}); err != nil {
		t.Fatalf("set bound session tool config: %v", err)
	}
	if err := store.SetPromptOverride(boundSession.ID, "SESSION OVERRIDE"); err != nil {
		t.Fatalf("set prompt override: %v", err)
	}
	if err := session.AppendMessage(store.TranscriptPath(boundSession.ID), session.Message{
		Role:      "assistant",
		Content:   "previous monitoring summary",
		Timestamp: time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append bound session message: %v", err)
	}

	client := &mockLLMClient{
		response: llm.ChatResponse{
			Message: llm.ChatMessage{Role: "assistant", Content: "ok"},
		},
		disableDelta: true,
	}

	tooling := buildChatToolingOptions(nil, nil, nil, defaultChatToolingOptions().Compaction, "standard", true, memory.SemanticConfig{}, 1, nil)
	tooling.OpsManager = ops.NewManager(root, ops.Options{})
	tooling.ResearchService = research.NewService(root, research.Options{})

	deps := chatHandlerDeps{
		workspaceDir:  root,
		store:         store,
		client:        client,
		logger:        zerolog.Nop(),
		maxIters:      1,
		mainSessionID: mainSession.ID,
		tooling:       tooling,
	}

	fallbackCalled := false
	runner := newCronPromptRunnerWithSessionContext(func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
		fallbackCalled = true
		return "fallback", nil
	}, deps)

	ctx := withCronExecutionContext(context.Background(), cronExecutionContext{SessionID: boundSession.ID})
	result, err := runner(ctx, "cron:job_demo", "check the website", nil, "", nil)
	if err != nil {
		t.Fatalf("run bound cron prompt: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected bound cron prompt result ok, got %q", result)
	}
	if fallbackCalled {
		t.Fatal("expected bound cron prompt to bypass fallback runner")
	}
	if client.callCount != 1 {
		t.Fatalf("expected one llm call, got %d", client.callCount)
	}
	if len(client.seenMessages) != 1 || len(client.seenMessages[0]) < 3 {
		t.Fatalf("expected system/history/user messages, got %+v", client.seenMessages)
	}
	systemPrompt := client.seenMessages[0][0].Content
	if !strings.Contains(systemPrompt, "SESSION OVERRIDE") {
		t.Fatalf("expected session prompt override in system prompt, got %q", systemPrompt)
	}
	userMessage := client.seenMessages[0][len(client.seenMessages[0])-1].Content
	if userMessage != "check the website" {
		t.Fatalf("expected cron prompt as user message, got %q", userMessage)
	}
	if len(client.seenTools) != 1 || len(client.seenTools[0]) != 1 || client.seenTools[0][0] != "read_file" {
		t.Fatalf("expected bound session tool filter to apply, got %+v", client.seenTools)
	}
}

func TestBuildCronNotificationEvent_UsesFriendlySummaryAndOpenPath(t *testing.T) {
	job := cron.Job{ID: "job_demo", Name: "novelist-1m"}
	evt := buildCronNotificationEvent(job, "info", "Cron completed", "# Result\n\n상태 문서를 갱신하고 1화 초안을 다듬었습니다.\n\n변경 파일:\n- STATE.md", "/tmp/cron.md", "sess-main")
	if evt.JobID != "job_demo" || evt.SessionID != "sess-main" {
		t.Fatalf("unexpected ids in notification: %+v", evt)
	}
	if evt.OpenPath != "/tmp/cron.md" {
		t.Fatalf("expected open path in notification, got %+v", evt)
	}
	if !containsAll(evt.Title, "Cron completed", "novelist-1m") {
		t.Fatalf("expected job name in title, got %q", evt.Title)
	}
	if !strings.Contains(evt.Message, "상태 문서를 갱신하고 1화 초안을 다듬었습니다") {
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
		func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			return "- `projects/proj_demo/TIMELINE_MAP.md` 추가\n- `STATE.md` 갱신", nil
		},
		zerolog.Nop(),
		nil,
		mainSession.ID,
		0,
		nil,
		nil,
	)

	_, err = runner(context.Background(), cron.Job{
		ID:       "job_demo",
		Name:     "nightly writer",
		Prompt:   "write next chapter",
		Schedule: "every:1m",
	})
	if err == nil {
		t.Fatal("expected claimed update verification error")
	}
	if !strings.Contains(err.Error(), "claimed file update not observed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCronJobRunner_NoProjectToolPolicyAfterRemoval(t *testing.T) {
	root := t.TempDir()

	var seenAllowedTools []string
	runner := newCronJobRunnerWithNotify(
		root,
		session.NewStore(root),
		func(_ context.Context, _ string, _ string, allowedTools []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			seenAllowedTools = append([]string(nil), allowedTools...)
			return "ok", nil
		},
		zerolog.Nop(),
		nil,
		"",
		0,
		nil,
		nil,
	)

	if _, err := runner(context.Background(), cron.Job{
		ID:       "job_demo",
		Name:     "triage logs",
		Prompt:   "inspect logs",
		Schedule: "every:5m",
	}); err != nil {
		t.Fatalf("run cron job: %v", err)
	}

	// After project removal, no tool policy filtering occurs — allowed tools should be nil
	if seenAllowedTools != nil {
		t.Fatalf("expected nil allowed tools without project policy, got %+v", seenAllowedTools)
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
