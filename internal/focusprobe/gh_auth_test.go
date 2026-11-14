package focusprobe

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGHAuthState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in gh is a POSIX shell script")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	signedIn := write("gh-ok", `[ "$1 $2" = "auth status" ] && [ "$GH_PROMPT_DISABLED" = 1 ] && exit 0; exit 3`)
	signedOut := write("gh-out", `echo "You are not logged into any GitHub hosts." >&2; exit 1`)

	for _, tc := range []struct{ bin, want string }{
		{signedIn, GHReady},
		{signedOut, GHSignedOut},
		{filepath.Join(dir, "no-such-gh"), GHMissing},
	} {
		if got := ghAuthState(context.Background(), tc.bin); got != tc.want {
			t.Errorf("%s: %s, want %s", filepath.Base(tc.bin), got, tc.want)
		}
	}

	t.Setenv(focusGHPathEnv, signedIn)
	if got := GHAuthState(context.Background()); got != GHReady {
		t.Errorf("TARS_FOCUS_GH_PATH: %s", got)
	}
}
