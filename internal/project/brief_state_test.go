package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/session"
)

func TestStoreUpdateBriefAndFinalize(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })
	sessionStore := session.NewStore(root)
	sess, err := sessionStore.Create("brief session")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	title := "Orbit Hearts"
	goal := "Write a 24-episode space opera serial."
	kind := "serial"
	premise := "Two rival navigators discover a map hidden in a dead star."
	brief, err := store.UpdateBrief(sess.ID, BriefUpdateInput{
		Title:         &title,
		Goal:          &goal,
		Kind:          &kind,
		Premise:       &premise,
		OpenQuestions: []string{"Who betrays the crew in arc one?", "What is the cost of the map?"},
	})
	if err != nil {
		t.Fatalf("update brief: %v", err)
	}
	if brief.Status != "collecting" {
		t.Fatalf("expected collecting status, got %q", brief.Status)
	}

	ready := "ready"
	brief, err = store.UpdateBrief(sess.ID, BriefUpdateInput{Status: &ready})
	if err != nil {
		t.Fatalf("mark brief ready: %v", err)
	}
	if brief.Status != "ready" {
		t.Fatalf("expected ready status, got %q", brief.Status)
	}

	created, finalized, err := store.FinalizeBrief(sess.ID, sessionStore)
	if err != nil {
		t.Fatalf("finalize brief: %v", err)
	}
	if finalized.Status != "finalized" {
		t.Fatalf("expected finalized brief status, got %q", finalized.Status)
	}
	if created.Objective != goal {
		t.Fatalf("expected project objective %q, got %q", goal, created.Objective)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", created.ID, "STATE.md")); err != nil {
		t.Fatalf("expected STATE.md to be created: %v", err)
	}
	state, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get project state after finalize: %v", err)
	}
	if state.Goal != goal || state.Phase != "planning" || state.Status != "active" {
		t.Fatalf("expected finalized brief to initialize planning state, got %+v", state)
	}
	if state.NextAction != "Who betrays the crew in arc one?" {
		t.Fatalf("expected open question to seed next action, got %+v", state)
	}
	if got := strings.Join(state.RemainingTasks, ","); got != "Who betrays the crew in arc one?,What is the cost of the map?" {
		t.Fatalf("expected open questions to seed remaining tasks, got %+v", state.RemainingTasks)
	}
	for _, name := range []string{"STORY_BIBLE.md", "CHARACTERS.md", "PLOT.md"} {
		if _, err := os.Stat(filepath.Join(root, "projects", created.ID, name)); err != nil {
			t.Fatalf("expected %s to be created: %v", name, err)
		}
	}
	refreshed, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("get project after finalize: %v", err)
	}
	if refreshed.SessionID == "" {
		t.Fatal("expected project to have a dedicated session_id after finalize")
	}
	projSess, err := sessionStore.Get(refreshed.SessionID)
	if err != nil {
		t.Fatalf("get project session: %v", err)
	}
	if projSess.ProjectID != created.ID {
		t.Fatalf("expected session project_id %q, got %q", created.ID, projSess.ProjectID)
	}
}

func TestStoreStateRoundtripAndNormalization(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })
	created, err := store.Create(CreateInput{Name: "Ops Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	goal := "Keep the operations backlog moving."
	phase := "executing"
	status := "paused"
	nextAction := "Review the last status update."
	state, err := store.UpdateState(created.ID, ProjectStateUpdateInput{
		Goal:           &goal,
		Phase:          &phase,
		Status:         &status,
		NextAction:     &nextAction,
		RemainingTasks: []string{"Summarize blockers", "Define next checkpoint"},
	})
	if err != nil {
		t.Fatalf("update state: %v", err)
	}
	if state.Phase != "executing" || state.Status != "paused" {
		t.Fatalf("unexpected normalized state: %+v", state)
	}

	raw, err := os.ReadFile(filepath.Join(root, "projects", created.ID, "STATE.md"))
	if err != nil {
		t.Fatalf("read state document: %v", err)
	}
	if !strings.Contains(string(raw), "next_action: \"Review the last status update.\"") {
		t.Fatalf("expected next_action in state document, got %q", string(raw))
	}

	loaded, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if got := strings.Join(loaded.RemainingTasks, ","); got != "Summarize blockers,Define next checkpoint" {
		t.Fatalf("unexpected remaining_tasks: %q", got)
	}

	activity, err := store.ListActivity(created.ID, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(activity) < 2 {
		t.Fatalf("expected state activity to be recorded, got %d items", len(activity))
	}
	if activity[0].Kind != ActivityKindStateChanged {
		t.Fatalf("expected newest activity kind %q, got %+v", ActivityKindStateChanged, activity[0])
	}
	if activity[0].Status != "paused" {
		t.Fatalf("expected state activity status paused, got %+v", activity[0])
	}
}

func TestStoreStatePreservesDraftingPhase(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })
	created, err := store.Create(CreateInput{Name: "Serial Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	phase := "drafting"
	state, err := store.UpdateState(created.ID, ProjectStateUpdateInput{Phase: &phase})
	if err != nil {
		t.Fatalf("update state: %v", err)
	}
	if state.Phase != "drafting" {
		t.Fatalf("expected drafting phase, got %+v", state)
	}

	loaded, err := store.GetState(created.ID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if loaded.Phase != "drafting" {
		t.Fatalf("expected persisted drafting phase, got %+v", loaded)
	}
}
