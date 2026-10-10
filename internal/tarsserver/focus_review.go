package tarsserver

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/rs/zerolog"
)

// The review loop's git facts (docs/decisions/focus-mode.md §5, P3): the
// commit a pipeline's review diff starts from, and the diff excerpt each
// finding card shows. Git runs outside focuspipeline.Store.Update's lock.

// focusRecordBaseCommit stores HEAD of the session's folder as the
// pipeline's base commit the first time a focus turn runs, before any
// change. A folder outside git records nothing.
func focusRecordBaseCommit(sessions *session.Store, sessionID string, p focuspipeline.Pipeline, logger zerolog.Logger) focuspipeline.Pipeline {
	if p.BaseCommit != "" || !p.Active() {
		return p
	}
	dir, err := sessions.GetCurrentDir(sessionID)
	if err != nil || strings.TrimSpace(dir) == "" {
		return p
	}
	head, err := focusprobe.RunGit(context.Background(), focusprobe.GitTimeout, dir, nil, "rev-parse", "--verify", "HEAD")
	if err != nil || head == "" {
		return p
	}
	updated, _, err := focusStoreFor(sessions).Update(sessionID, func(cur focuspipeline.Pipeline) (focuspipeline.Pipeline, error) {
		if cur.BaseCommit == "" {
			cur.BaseCommit = head
		}
		return cur, nil
	})
	if err != nil {
		logger.Warn().Err(err).Str("session_id", sessionID).Msg("focus: record base commit failed")
		p.BaseCommit = head
		return p
	}
	return updated
}

// focusEnrichFindings adds diff excerpts to a review turn's findings. It
// reads the pipeline and the session folder before the update that applies
// the turn, so git never runs under the pipeline lock.
func focusEnrichFindings(sessions *session.Store, sessionID string, blocks focuspipeline.Blocks) focuspipeline.Blocks {
	if len(blocks.Findings) == 0 {
		return blocks
	}
	p, ok, err := focusStoreFor(sessions).Get(sessionID)
	if err != nil || !ok || p.CurrentKind() != focuspipeline.StageReview {
		return blocks
	}
	dir, err := sessions.GetCurrentDir(sessionID)
	if err != nil {
		return blocks
	}
	blocks.Findings = focusprobe.FindingExcerpts(context.Background(), dir, p.BaseCommit, blocks.Findings)
	return blocks
}
