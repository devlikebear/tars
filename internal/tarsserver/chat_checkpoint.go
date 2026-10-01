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

// attachSessionDeleteHooks installs hooks as the session store's one delete
// hook, run in order. Nil hooks are skipped.
func attachSessionDeleteHooks(sessions *session.Store, hooks ...func(sessionID string)) {
	if sessions == nil {
		return
	}
	live := make([]func(string), 0, len(hooks))
	for _, h := range hooks {
		if h != nil {
			live = append(live, h)
		}
	}
	if len(live) == 0 {
		return
	}
	sessions.SetDeleteHook(func(sessionID string) {
		for _, h := range live {
			h(sessionID)
		}
	})
}

// checkpointCleanup returns the delete hook that drops a deleted session's
// checkpoints, and sweeps out checkpoints of sessions that are already gone.
// It returns nil without a checkpoint store.
func checkpointCleanup(store *checkpoint.Store, sessions *session.Store, logger zerolog.Logger) func(string) {
	if store == nil || sessions == nil {
		return nil
	}
	hook := func(sessionID string) {
		// The hook runs under the session index lock; git work does not.
		go func() {
			if err := store.DropSession(context.Background(), sessionID); err != nil {
				logger.Warn().Err(err).Str("session_id", sessionID).Msg("checkpoint: drop deleted session failed")
			}
		}()
	}
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
	return hook
}

// foldPathCase is set where volumes are case-insensitive by default.
var foldPathCase = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// sameDir reports whether two paths name the same directory. Symlinks are
// resolved first: the session store saves work directories resolved, while
// the workspace path keeps the spelling it was configured with, so the two
// differ whenever the workspace sits under a link (macOS's /var and /tmp, a
// relocated ~/.tars). A path that cannot be resolved is compared as written.
func sameDir(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	ca, cb := resolvedDir(a), resolvedDir(b)
	return ca == cb || (foldPathCase && strings.EqualFold(ca, cb))
}

func resolvedDir(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
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
