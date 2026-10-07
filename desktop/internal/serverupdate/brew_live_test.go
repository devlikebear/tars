//go:build integration

package serverupdate

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/desktop/internal/server"
)

// TestBrewLiveCheck asks the machine's own Homebrew whether tars is
// outdated. It only checks (brew update, brew outdated): nothing is
// installed. Skipped without a Homebrew-installed tars.
//
//	cd desktop && go test -tags integration ./internal/serverupdate -run TestBrewLiveCheck -v
func TestBrewLiveCheck(t *testing.T) {
	tars, err := exec.LookPath("tars")
	if err != nil {
		t.Skip("no tars on PATH")
	}
	resolved, err := filepath.EvalSymlinks(tars)
	if err != nil || !ManagedByHomebrew(resolved) {
		t.Skipf("tars at %s is not a Homebrew install", tars)
	}
	run := func(ctx context.Context, bin string, args, env []string) ([]byte, []byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(), env...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.Bytes(), stderr.Bytes(), err
	}
	findBrew := func() string {
		path, _ := exec.LookPath("brew")
		return path
	}
	src, err := For("darwin", tars, resolved, findBrew, server.Config{URL: server.DefaultURL}, run)
	if err != nil {
		t.Fatal(err)
	}
	brew, ok := src.(Brew)
	if !ok {
		t.Fatalf("source = %T", src)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	res, err := brew.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("check: %+v, launchd runs the server: %v", res, brew.serviceLoaded(ctx))
	if res.Available && (res.Latest == "" || res.Current == "") {
		t.Fatalf("an available update without versions: %+v", res)
	}
}
