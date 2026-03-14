package tarsserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

type dashboardAutopilotStub struct {
	run project.AutopilotRun
	ok  bool
}

func (s dashboardAutopilotStub) Status(projectID string) (project.AutopilotRun, bool) {
	if !s.ok || strings.TrimSpace(projectID) == "" {
		return project.AutopilotRun{}, false
	}
	return s.run, true
}

func TestProjectDashboardHandler_RendersProjectOverviewAndActivity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	store := project.NewStore(root, nil)
	created, err := store.Create(project.CreateInput{
		Name:         "Dashboard Project",
		Objective:    "Ship a simple dashboard",
		Instructions: "Keep it server-rendered",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	phase := "executing"
	status := "active"
	nextAction := "Render project overview"
	if _, err := store.UpdateState(created.ID, project.ProjectStateUpdateInput{
		Phase:      &phase,
		Status:     &status,
		NextAction: &nextAction,
	}); err != nil {
		t.Fatalf("update state: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "pm",
		Kind:    "assignment",
		Status:  "queued",
		Message: "Assign dashboard build to dev-1",
	}); err != nil {
		t.Fatalf("append activity: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "agent",
		Agent:   "dev-1",
		Kind:    "task_status",
		Status:  "in_progress",
		Message: "Rendering dashboard page",
	}); err != nil {
		t.Fatalf("append second activity: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "agent",
		Agent:   "codex-cli",
		TaskID:  "task-1",
		Kind:    "agent_report",
		Status:  "completed",
		Message: "Implemented board rendering",
		Meta: map[string]string{
			"notes":   "Waiting for review",
			"run_id":  "run-task-1",
			"summary": "Implemented board rendering",
		},
	}); err != nil {
		t.Fatalf("append agent report: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "pm",
		Kind:    "blocker",
		Status:  "blocked",
		Message: "Waiting for GitHub metadata",
	}); err != nil {
		t.Fatalf("append blocker activity: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "pm",
		Kind:    "decision",
		Status:  "needed",
		Message: "Choose whether to continue without GitHub issue linkage",
	}); err != nil {
		t.Fatalf("append decision activity: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "pm",
		Kind:    "replan",
		Status:  "proposed",
		Message: "Split dashboard work into implementation and review slices",
	}); err != nil {
		t.Fatalf("append replan activity: %v", err)
	}
	if _, err := store.UpdateBoard(created.ID, project.BoardUpdateInput{
		Tasks: []project.BoardTask{
			{
				ID:               "task-1",
				Title:            "Build dashboard view",
				Status:           "in_progress",
				Assignee:         "dev-1",
				Issue:            "https://github.com/devlikebear/tars/issues/42",
				Branch:           "feat/dashboard-view",
				PR:               "https://github.com/devlikebear/tars/pull/42",
				ReviewApprovedBy: "review-bot",
				TestCommand:      "go test ./internal/tarsserver",
				BuildCommand:     "go test ./internal/project",
			},
			{
				ID:       "task-2",
				Title:    "Prepare review notes",
				Status:   "review",
				Assignee: "reviewer-1",
			},
		},
	}); err != nil {
		t.Fatalf("update board: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "system",
		TaskID:  "task-1",
		Kind:    project.ActivityKindTestStatus,
		Status:  "passed",
		Message: "Task tests passed",
	}); err != nil {
		t.Fatalf("append test status: %v", err)
	}
	if _, err := store.AppendActivity(created.ID, project.ActivityAppendInput{
		Source:  "system",
		TaskID:  "task-1",
		Kind:    project.ActivityKindBuildStatus,
		Status:  "passed",
		Message: "Task build passed",
	}); err != nil {
		t.Fatalf("append build status: %v", err)
	}

	handler := newProjectDashboardHandler(
		store,
		dashboardAutopilotStub{
			run: project.AutopilotRun{
				ProjectID:  created.ID,
				RunID:      "autopilot-1",
				Status:     project.AutopilotStatusBlocked,
				Message:    "Waiting for review decision",
				Iterations: 3,
				StartedAt:  "2026-03-14T00:00:00Z",
				UpdatedAt:  "2026-03-14T00:01:00Z",
			},
			ok: true,
		},
		newProjectDashboardBroker(),
		zerolog.New(io.Discard),
	)
	req := httptest.NewRequest(http.MethodGet, "/ui/projects/"+created.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content type, got %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Dashboard Project",
		"Ship a simple dashboard",
		"executing",
		"active",
		"Rendering dashboard page",
		"Assign dashboard build to dev-1",
		"Board",
		"Build dashboard view",
		"Prepare review notes",
		"GitHub Flow",
		"Autopilot",
		"Waiting for review decision",
		"autopilot-1",
		"Worker Reports",
		"Implemented board rendering",
		"Waiting for review",
		"Blockers",
		"Waiting for GitHub metadata",
		"Decisions",
		"Choose whether to continue without GitHub issue linkage",
		"Replans",
		"Split dashboard work into implementation and review slices",
		"https://github.com/devlikebear/tars/issues/42",
		"feat/dashboard-view",
		"https://github.com/devlikebear/tars/pull/42",
		"review-bot",
		"passed",
		"in_progress",
		"review",
		"todo",
		"0",
		"1 active",
		"/ui/projects/" + created.ID + "/stream",
		"autopilot-section",
		"board-section",
		"activity-section",
		"reports-section",
		"blockers-section",
		"decisions-section",
		"replans-section",
		"github-flow-section",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected dashboard body to contain %q, got %q", want, body)
		}
	}
}

func TestProjectDashboardHandler_ProjectNotFound(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	handler := newProjectDashboardHandler(project.NewStore(root, nil), nil, newProjectDashboardBroker(), zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/ui/projects/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestProjectDashboardHandler_ProjectStreamEmitsProjectEvents(t *testing.T) {
	broker := newProjectDashboardBroker()
	handler := newProjectDashboardHandler(nil, nil, broker, zerolog.New(io.Discard))

	req := httptest.NewRequest(http.MethodGet, "/ui/projects/demo/stream", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)

	broker.publish(newProjectDashboardEvent("demo", "activity"))
	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"project_id\":\"demo\"") {
		t.Fatalf("expected stream body to include project id, got %q", body)
	}
	if !strings.Contains(body, "\"kind\":\"activity\"") {
		t.Fatalf("expected stream body to include event kind, got %q", body)
	}
}
