package tarsserver

import (
	"context"
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

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
