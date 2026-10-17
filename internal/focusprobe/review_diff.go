package focusprobe

import (
	"context"
	"strings"
	"time"
)

// The diff the second opinion review reads (docs/decisions/focus-mode.md
// §5.1): the server builds it, so the second reviewer looks at exactly the
// change the stage's own review was asked about.

const (
	// ReviewDiffMaxBytes caps the diff put in a second opinion's prompt.
	ReviewDiffMaxBytes = 60 << 10
	// reviewDiffTimeout bounds each git call: a whole-tree diff takes
	// longer than the per-file probes GitTimeout is sized for.
	reviewDiffTimeout = 20 * time.Second
	// reviewUntrackedMax caps the untracked files listed.
	reviewUntrackedMax = 200
)

// ReviewDiff is the change a review reads, as git reports it.
type ReviewDiff struct {
	// Base is the commit the diff starts from ("HEAD" when the pipeline
	// recorded none).
	Base string
	// Diff is `git diff <base>`: committed and uncommitted changes to
	// tracked files, cut to ReviewDiffMaxBytes at a line end when
	// Truncated.
	Diff      string
	Truncated bool
	// Stat is `git diff --stat <base>`, set only when Diff was cut, so the
	// reviewer still sees every changed file.
	Stat string
	// Untracked lists the files git does not track yet (ignored ones left
	// out), at most reviewUntrackedMax; their content is not in Diff.
	Untracked []string
}

// Empty reports whether there is no change to review.
func (d ReviewDiff) Empty() bool {
	return strings.TrimSpace(d.Diff) == "" && len(d.Untracked) == 0
}

// ReviewDiffOf reads the review diff of dir's repository since base. ok is
// false when dir is not in a git repository or git fails.
func ReviewDiffOf(ctx context.Context, dir, base string) (ReviewDiff, bool) {
	if strings.TrimSpace(dir) == "" {
		return ReviewDiff{}, false
	}
	top, err := RunGit(ctx, GitTimeout, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil || top == "" {
		return ReviewDiff{}, false
	}
	// Only a commit id recorded by the server reaches git as an argument.
	if base = strings.TrimSpace(base); !IsGitObjectID(base) {
		base = "HEAD"
	}
	out := ReviewDiff{Base: base}
	diff, err := RunGit(ctx, reviewDiffTimeout, top, nil, "diff", "--no-color", "--no-ext-diff", base, "--")
	if err != nil {
		return ReviewDiff{}, false
	}
	if len(diff) > ReviewDiffMaxBytes {
		cut := strings.LastIndexByte(diff[:ReviewDiffMaxBytes], '\n')
		if cut <= 0 {
			cut = ReviewDiffMaxBytes
		}
		out.Diff = strings.ToValidUTF8(diff[:cut], "")
		out.Truncated = true
		out.Stat, _ = RunGit(ctx, reviewDiffTimeout, top, nil, "diff", "--no-color", "--stat=120", base, "--")
	} else {
		out.Diff = diff
	}
	untracked, err := RunGit(ctx, reviewDiffTimeout, top, nil, "ls-files", "--others", "--exclude-standard")
	if err == nil && untracked != "" {
		for _, f := range strings.Split(untracked, "\n") {
			if f = strings.TrimSpace(f); f != "" && len(out.Untracked) < reviewUntrackedMax {
				out.Untracked = append(out.Untracked, f)
			}
		}
	}
	return out, true
}
