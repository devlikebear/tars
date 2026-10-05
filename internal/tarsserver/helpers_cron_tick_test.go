package tarsserver

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/rs/zerolog"
)

func TestWorkspaceCronManagerTickRunsDueJob(t *testing.T) {
	store := cron.NewStore(t.TempDir())
	if _, err := store.CreateWithOptions(cron.CreateInput{
		Name:      "due-job",
		Prompt:    "run due",
		Schedule:  "every:1s",
		Enabled:   true,
		HasEnable: true,
	}); err != nil {
		t.Fatalf("create job: %v", err)
	}

	var prompts []string
	manager := newWorkspaceCronManager(
		newWorkspaceCronStoreResolver("", 0, store),
		func(_ context.Context, job cron.Job) (string, error) {
			prompts = append(prompts, job.Prompt)
			return "ok", nil
		},
		time.Second,
		func() time.Time { return time.Date(2026, 2, 15, 19, 0, 0, 0, time.UTC) },
		zerolog.New(io.Discard),
	)

	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(prompts) != 1 || prompts[0] != "run due" {
		t.Fatalf("expected the due job to run once, got %+v", prompts)
	}
}
