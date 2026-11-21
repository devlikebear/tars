package tarsserver

import (
	"context"
	"time"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/focuspipeline"
)

// saveVersionTimeout bounds the git commands of one saved version.
const saveVersionTimeout = time.Minute

// saveVersionAfterFinish saves the finished work of a pipeline that asked
// for it (an easy-mode project: no pull request, so finishing is where the
// work becomes a version) and records the commit, or a fixed error code, on
// the pipeline. A stopped or blocked pipeline never gets here: it is not
// Finished. Called after the store's lock, when no turn runs.
func (d *focusDriver) saveVersionAfterFinish(sessionID string, p focuspipeline.Pipeline) {
	if !focuspipeline.SaveDue(p) {
		return
	}
	log := d.logger.With().Str("session_id", sessionID).Logger()
	saved := focuspipeline.SavedVersion{}
	dir := d.sessionDir(sessionID)
	ctx, cancel := context.WithTimeout(context.Background(), saveVersionTimeout)
	defer cancel()
	commit, err := easyproject.SaveVersion(ctx, dir, focuspipeline.SaveMessage(p))
	if dir == "" || err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("focus: save the finished version failed")
		saved.Error = focuspipeline.SaveFailed
	} else {
		saved.Commit = commit
		log.Info().Str("commit", commit).Msg("focus: saved the finished version")
	}
	if _, _, err := focusStoreFor(d.sessions).Update(sessionID, func(p focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		return focuspipeline.RecordSaved(p, saved, d.now()), nil
	}); err != nil {
		log.Warn().Err(err).Msg("focus: record the saved version")
	}
}
