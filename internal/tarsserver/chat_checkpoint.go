package tarsserver

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/checkpoint"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

// checkpointEndTimeout bounds the end snapshot. It runs after the turn,
// including a cancelled one, so it cannot use the turn's own context.
const checkpointEndTimeout = 2 * time.Minute

// beginChatCheckpoint snapshots the turn's work tree before the model runs,
// so every provider's edits — a CLI provider's included — can be diffed
// afterwards. It never fails the turn; nil means no checkpoint.
func beginChatCheckpoint(ctx context.Context, deps chatHandlerDeps, state chatRunState, preview string) *checkpoint.Turn {
	store := deps.tooling.Checkpoints
	if store == nil || state.turnID == "" || strings.TrimSpace(state.cwd) == "" {
		return nil
	}
	// A session without a folder of its own works in the TARS workspace,
	// whose transcripts, memory, and logs change on every turn. That is
	// TARS's data, not the user's project: leave it out.
	if sameDir(state.cwd, state.requestWorkspaceDir) {
		return nil
	}
	turn, err := store.BeginTurn(ctx, state.sessionID, state.turnID, state.cwd, preview)
	if err != nil {
		deps.logger.Warn().Err(err).Str("session_id", state.sessionID).Msg("checkpoint: begin turn failed")
		return nil
	}
	return turn
}

// endChatCheckpoint records the turn's end snapshot and tells the console
// what changed. Call it on every way out of the turn: success, error, and
// cancellation all leave edits on disk, so the snapshot outlives ctx's
// cancellation.
func endChatCheckpoint(ctx context.Context, turn *checkpoint.Turn, stream *chatStreamWriter, logger zerolog.Logger, sessionID string) {
	if turn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkpointEndTimeout)
	defer cancel()
	entry, err := turn.End(ctx)
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("checkpoint: end turn failed")
	}
	if entry.Skipped != "" {
		logger.Debug().Str("session_id", sessionID).Str("reason", entry.Skipped).Str("detail", entry.SkipDetail).Msg("checkpoint skipped")
	}
	stream.checkpoint(entry)
}

// attachCheckpointCleanup drops a session's checkpoints when the session is
// deleted, and sweeps out checkpoints of sessions that are already gone.
func attachCheckpointCleanup(store *checkpoint.Store, sessions *session.Store, logger zerolog.Logger) {
	if store == nil || sessions == nil {
		return
	}
	sessions.SetDeleteHook(func(sessionID string) {
		// The hook runs under the session index lock; git work does not.
		go func() {
			if err := store.DropSession(context.Background(), sessionID); err != nil {
				logger.Warn().Err(err).Str("session_id", sessionID).Msg("checkpoint: drop deleted session failed")
			}
		}()
	})
	go func() {
		all, err := sessions.ListAll()
		if err != nil {
			logger.Warn().Err(err).Msg("checkpoint: list sessions for sweep failed")
			return
		}
		alive := make(map[string]bool, len(all))
		for _, s := range all {
			alive[s.ID] = true
		}
		// A session created after the listing is alive too: only a definite
		// "not found" drops its checkpoints.
		isAlive := func(id string) bool {
			if alive[id] {
				return true
			}
			_, err := sessions.Get(id)
			return err == nil || !isSessionNotFound(err)
		}
		if err := store.SweepOrphans(context.Background(), isAlive); err != nil {
			logger.Warn().Err(err).Msg("checkpoint: sweep failed")
		}
	}()
}

// sameDir reports whether two paths name the same directory, ignoring case
// where the file system does.
func sameDir(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(absA, absB)
	}
	return absA == absB
}

// isSessionNotFound matches the session store's not-found errors, which are
// the sentinel or, from older paths, the same message.
func isSessionNotFound(err error) bool {
	return errors.Is(err, session.ErrSessionNotFound) || strings.Contains(err.Error(), "session not found")
}

// openCheckpointStore opens the turn checkpoint store under the workspace.
// Checkpoints are an aid, not a requirement: without git, chat still works and
// the checkpoint API answers 503.
func openCheckpointStore(workspaceDir string, logger zerolog.Logger) *checkpoint.Store {
	store, err := checkpoint.Open(filepath.Join(workspaceDir, "_shared", "checkpoints"), checkpoint.Options{})
	if err != nil {
		logger.Warn().Err(err).Msg("checkpoint: store unavailable; turn changes will not be recorded")
		return nil
	}
	return store
}
