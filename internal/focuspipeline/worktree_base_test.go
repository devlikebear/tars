package focuspipeline

import "testing"

// TestSetWorktreeBase is tars dogfooding fix #1: a feature pipeline whose
// isolated worktree had to fall back to HEAD gets a notice card only when
// the fallback is surprising (a configured remote that could not be
// fetched or resolved) — a checkout with no "origin" remote at all is
// ordinary and stays silent on the deck.
func TestSetWorktreeBase(t *testing.T) {
	tests := []struct {
		name       string
		fallback   string
		wantNotice bool
	}{
		{"resolved as asked: no notice", "", false},
		{"no remote at all: no notice", "no_remote", false},
		{"remote exists but fetch failed: notice", "fetch_failed", true},
		{"remote exists but no default branch resolved: notice", "no_default_branch", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New("s1", "goal", t0)
			cardsBefore := len(p.Cards)
			p = SetWorktreeBase(p, tt.fallback, t0)
			found := false
			for _, c := range p.Cards[cardsBefore:] {
				if c.Title == NoticeWorktreeBaseFallback {
					found = true
				}
			}
			if found != tt.wantNotice {
				t.Fatalf("SetWorktreeBase(%q) notice = %v, want %v (cards %+v)", tt.fallback, found, tt.wantNotice, p.Cards[cardsBefore:])
			}
		})
	}
}
