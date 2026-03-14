package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreateSeedsKanbanBoard(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	store := NewStore(root, func() time.Time { return now })

	created, err := store.Create(CreateInput{Name: "Kanban Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "projects", created.ID, "KANBAN.md")); err != nil {
		t.Fatalf("expected KANBAN.md to be created: %v", err)
	}

	board, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if board.ProjectID != created.ID {
		t.Fatalf("expected project id %q, got %q", created.ID, board.ProjectID)
	}
	if got := len(board.Columns); got != 4 {
		t.Fatalf("expected 4 default columns, got %d", got)
	}
}

func TestStoreBoardRoundtrip(t *testing.T) {
	root := t.TempDir()
	times := []time.Time{
		time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 14, 12, 5, 0, 0, time.UTC),
	}
	nowIndex := 0
	store := NewStore(root, func() time.Time {
		current := times[nowIndex]
		if nowIndex < len(times)-1 {
			nowIndex++
		}
		return current
	})

	created, err := store.Create(CreateInput{Name: "Kanban Project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	updated, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Build activity feed",
				Status:         "in_progress",
				Assignee:       "dev-1",
				Role:           "developer",
				WorkerKind:     WorkerKindCodexCLI,
				ReviewRequired: true,
				TestCommand:    "go test ./internal/project",
				BuildCommand:   "go test ./internal/tarsserver",
			},
		},
	})
	if err != nil {
		t.Fatalf("update board: %v", err)
	}
	if updated.UpdatedAt != times[1].UTC().Format(time.RFC3339) {
		t.Fatalf("unexpected updated_at: %q", updated.UpdatedAt)
	}
	if len(updated.Tasks) != 1 || updated.Tasks[0].Status != "in_progress" {
		t.Fatalf("unexpected updated tasks: %+v", updated.Tasks)
	}

	loaded, err := store.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("get board: %v", err)
	}
	if len(loaded.Tasks) != 1 || loaded.Tasks[0].Assignee != "dev-1" {
		t.Fatalf("unexpected loaded tasks: %+v", loaded.Tasks)
	}
	if loaded.Tasks[0].WorkerKind != WorkerKindCodexCLI {
		t.Fatalf("expected worker kind %q, got %+v", WorkerKindCodexCLI, loaded.Tasks[0])
	}

	second, err := store.UpdateBoard(created.ID, BoardUpdateInput{
		Tasks: []BoardTask{
			{
				ID:             "task-1",
				Title:          "Build activity feed",
				Status:         "review",
				Assignee:       "dev-1",
				Role:           "developer",
				ReviewRequired: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("update board second time: %v", err)
	}
	if second.Tasks[0].Status != "review" {
		t.Fatalf("expected review status, got %+v", second.Tasks[0])
	}

	activity, err := store.ListActivity(created.ID, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(activity) < 3 {
		t.Fatalf("expected board activities, got %d items", len(activity))
	}
	if activity[0].Kind != ActivityKindBoardTaskUpdated {
		t.Fatalf("expected newest board update activity, got %+v", activity[0])
	}
	if activity[1].Kind != ActivityKindBoardTaskCreated {
		t.Fatalf("expected board task create activity next, got %+v", activity[1])
	}
}
