package focuspipeline

import "time"

// NoticeWorktreeBaseFallback is the notice card title left on a pipeline
// whose isolated worktree could not branch from the remote's default
// branch (tars dogfooding fix #1) and fell back to the checkout's HEAD —
// which may carry unrelated local commits (a feature branch the checkout
// happened to be on). The review diff and PR this pipeline produces follow
// whatever base actually landed, so the developer needs to know when it
// was not the one asked for.
const NoticeWorktreeBaseFallback = "worktree base fell back to HEAD"

// baseFallbackNoRemote is sessionworktree.BaseFallbackNoRemote's value,
// duplicated as a literal so this package does not import sessionworktree
// (fallback arrives as a plain string).
const baseFallbackNoRemote = "no_remote"

// SetWorktreeBase records a feature pipeline's isolated worktree's actual
// base, read right after creation (focus_pipeline.go): a non-empty
// fallback reason (sessionworktree.BaseFallback*, passed through as a
// plain string) means the remote's default branch could not be resolved.
// A checkout with no "origin" remote at all (baseFallbackNoRemote) is
// ordinary — a scratch repository, or one never pushed — so it stays
// silent here (the creation response's worktree_base still reports it,
// for a caller that cares); any other fallback (the remote exists but
// could not be fetched or resolved) is surprising enough for a notice
// card, so a stale or unrelated baseline is visible instead of discovered
// later in review. An empty fallback (the base resolved as asked, or the
// pipeline did not ask for one) is a no-op.
func SetWorktreeBase(p Pipeline, fallback string, now time.Time) Pipeline {
	if fallback == "" || fallback == baseFallbackNoRemote {
		return p
	}
	p.addCard(CardNotice, 0, NoticeWorktreeBaseFallback, map[string]string{"reason": fallback}, now)
	return p
}
