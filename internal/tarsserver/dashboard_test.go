package tarsserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

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

	handler := newProjectDashboardHandler(store, zerolog.New(io.Discard))
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
	handler := newProjectDashboardHandler(project.NewStore(root, nil), zerolog.New(io.Discard))
	req := httptest.NewRequest(http.MethodGet, "/ui/projects/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%q", rec.Code, rec.Body.String())
	}
}
