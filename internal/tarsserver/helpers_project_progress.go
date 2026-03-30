package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/project"
	"github.com/rs/zerolog"
)

// newProjectProgressAfterHeartbeat returns a callback that advances active
// projects by one dispatch iteration after each heartbeat tick.
func newProjectProgressAfterHeartbeat(
	store *project.Store,
	runner project.TaskRunner,
	logger zerolog.Logger,
) func(ctx context.Context) error {
	if store == nil || runner == nil {
		return nil
	}
	return func(ctx context.Context) error {
		projects, err := store.List()
		if err != nil {
			return err
		}
		for _, p := range projects {
			if strings.TrimSpace(p.Status) == "archived" {
				continue
			}
			board, err := store.GetBoard(p.ID)
			if err != nil {
				logger.Debug().Err(err).Str("project_id", p.ID).Msg("skip project progress: board read failed")
				continue
			}
			if len(board.Tasks) == 0 {
				continue
			}
			// Dispatch todo tasks if any
			hasTodo := false
			hasReview := false
			for _, task := range board.Tasks {
				switch strings.TrimSpace(task.Status) {
				case "todo":
					hasTodo = true
				case "review":
					hasReview = true
				}
			}
			if !hasTodo && !hasReview {
				continue
			}
			orch := project.NewOrchestrator(store, runner)
			if hasTodo {
				report, err := orch.DispatchTodo(ctx, p.ID)
				if err != nil {
					logger.Debug().Err(err).Str("project_id", p.ID).Msg("project progress: dispatch todo failed")
				} else if len(report.Runs) > 0 {
					logger.Info().Str("project_id", p.ID).Int("dispatched", len(report.Runs)).Msg("project progress: dispatched todo tasks")
				}
			}
			if hasReview {
				report, err := orch.DispatchReview(ctx, p.ID)
				if err != nil {
					logger.Debug().Err(err).Str("project_id", p.ID).Msg("project progress: dispatch review failed")
				} else if len(report.Runs) > 0 {
					logger.Info().Str("project_id", p.ID).Int("dispatched", len(report.Runs)).Msg("project progress: dispatched review tasks")
				}
			}
		}
		return nil
	}
}
