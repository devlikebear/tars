//go:build !windows

package tarsserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

// stubGH writes a POSIX shell script standing in for gh and returns its path.
func stubGH(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFocusGHProbeStub(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "view.json")
	if err := os.WriteFile(fixture, []byte(ghViewFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		script  string
		want    string
		errPart string
	}{
		{"found", `[ "$1 $2 $3 $4" = "pr view 42 --json" ] || exit 9
[ "$GH_PROMPT_DISABLED" = "1" ] || exit 8
pwd > "$(dirname "$0")/cwd"
cat "` + fixture + `"`, focuspipeline.ProbeFound, ""},
		{"no PR on the branch", `echo 'no pull requests found for branch "x"' >&2; exit 1`, focuspipeline.ProbeNone, ""},
		{"not logged in", `echo 'To get started with GitHub CLI, please run:  gh auth login' >&2; exit 4`, focuspipeline.ProbeUnavailable, "gh auth login"},
		{"malformed output", `echo '{nope'`, focuspipeline.ProbeUnavailable, "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := stubGH(t, tt.script)
			dir := t.TempDir()
			got := runFocusGH(t.Context(), dir, bin, 42)
			if got.Status != tt.want || !strings.Contains(got.Error, tt.errPart) {
				t.Fatalf("probe = %+v", got)
			}
			if tt.want == focuspipeline.ProbeFound {
				cwd, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "cwd"))
				resolved, _ := filepath.EvalSymlinks(dir)
				if got := strings.TrimSpace(string(cwd)); got != dir && got != resolved {
					t.Fatalf("ran in %q, want %q", got, dir)
				}
			}
		})
	}
}

func TestFocusGHProbeTimeout(t *testing.T) {
	old := focusGHTimeout
	focusGHTimeout = 200 * time.Millisecond
	t.Cleanup(func() { focusGHTimeout = old })
	bin := stubGH(t, "sleep 5 & wait")
	start := time.Now()
	got := runFocusGH(t.Context(), t.TempDir(), bin, 0)
	if got.Status != focuspipeline.ProbeUnavailable || !strings.Contains(got.Error, "timed out") {
		t.Fatalf("probe = %+v", got)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("probe took %v: the process group was not killed", elapsed)
	}
}

func TestFocusGHProbeByBranchHasNoNumber(t *testing.T) {
	bin := stubGH(t, `[ "$1 $2 $3" = "pr view --json" ] || { echo "args: $*" >&2; exit 9; }
echo '{"number":5,"state":"OPEN"}'`)
	if got := runFocusGH(t.Context(), t.TempDir(), bin, 0); got.Status != focuspipeline.ProbeFound || got.Number != 5 {
		t.Fatalf("probe = %+v", got)
	}
}
