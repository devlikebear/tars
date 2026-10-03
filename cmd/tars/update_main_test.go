package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/selfupdate"
)

type fakeUpdateServer struct {
	version    string
	versionErr error
	restartErr error
	restarted  string
}

func (f *fakeUpdateServer) Version(context.Context) (string, error) { return f.version, f.versionErr }
func (f *fakeUpdateServer) Restart(_ context.Context, want string) error {
	f.restarted = want
	return f.restartErr
}

// stubUpdate swaps the update command's dependencies for the test.
func stubUpdate(t *testing.T, status selfupdate.Status, srv *fakeUpdateServer) *[]selfupdate.Release {
	t.Helper()
	installed := &[]selfupdate.Release{}
	prevCheck, prevInstall, prevServer, prevExe, prevConfirm := updateCheck, updateInstall, updateServer, updateExePath, updateConfirm
	t.Cleanup(func() {
		updateCheck, updateInstall, updateServer, updateExePath, updateConfirm = prevCheck, prevInstall, prevServer, prevExe, prevConfirm
	})
	updateExePath = func() (string, error) { return "/opt/tars/bin/tars", nil }
	updateCheck = func(_ context.Context, opts selfupdate.Options) (selfupdate.Status, error) {
		if opts.ExePath != "/opt/tars/bin/tars" {
			t.Fatalf("check got exe %q", opts.ExePath)
		}
		return status, nil
	}
	updateInstall = func(_ context.Context, _ selfupdate.Options, rel selfupdate.Release) error {
		*installed = append(*installed, rel)
		return nil
	}
	updateServer = func(string, string) updateServerAPI { return srv }
	updateConfirm = func(io.Reader, io.Writer, string) (bool, error) {
		t.Fatal("asked for confirmation")
		return false, nil
	}
	return installed
}

var availableStatus = selfupdate.Status{Current: "1.2.3", Latest: "1.3.0", Available: true, Release: selfupdate.Release{Version: "1.3.0", ArchiveName: "tars_1.3.0_linux_amd64.tar.gz"}}

func runUpdateCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCommand(strings.NewReader(""), &out, io.Discard)
	cmd.SetArgs(append([]string{"update"}, args...))
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestUpdateCommandInstallsAndRestartsTheServer(t *testing.T) {
	srv := &fakeUpdateServer{version: "1.2.3"}
	installed := stubUpdate(t, availableStatus, srv)

	out, err := runUpdateCommand(t, "--yes", "--json")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	var got updateResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	want := updateResult{Current: "1.2.3", Latest: "1.3.0", Available: true, Updated: true, Restarted: true, Server: "restarted"}
	if got != want {
		t.Fatalf("result = %+v, want %+v", got, want)
	}
	if len(*installed) != 1 || srv.restarted != "1.3.0" {
		t.Fatalf("installed %v, restarted %q", *installed, srv.restarted)
	}
}

func TestUpdateCommandCheckOnlyInstallsNothing(t *testing.T) {
	installed := stubUpdate(t, availableStatus, &fakeUpdateServer{})
	out, err := runUpdateCommand(t, "--check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tars 1.3.0 is available (this is 1.2.3)") || len(*installed) != 0 {
		t.Fatalf("out=%q installed=%v", out, *installed)
	}

	stubUpdate(t, selfupdate.Status{Current: "1.3.0", Latest: "1.3.0"}, &fakeUpdateServer{})
	out, err = runUpdateCommand(t)
	if err != nil || !strings.Contains(out, "tars 1.3.0 is the latest release.") {
		t.Fatalf("up to date: out=%q err=%v", out, err)
	}
}

func TestUpdateCommandAsksFirstAndHonoursNo(t *testing.T) {
	installed := stubUpdate(t, availableStatus, &fakeUpdateServer{})
	var asked string
	updateConfirm = func(_ io.Reader, _ io.Writer, q string) (bool, error) {
		asked = q
		return false, nil
	}
	if _, err := runUpdateCommand(t); err != nil {
		t.Fatal(err)
	}
	if asked != "Update tars 1.2.3 to 1.3.0?" || len(*installed) != 0 {
		t.Fatalf("asked %q, installed %v", asked, *installed)
	}

	updateConfirm = func(io.Reader, io.Writer, string) (bool, error) { return false, errNotInteractive }
	if _, err := runUpdateCommand(t); !errors.Is(err, errNotInteractive) {
		t.Fatalf("non-interactive: err = %v", err)
	}
}

func TestUpdateCommandKeepsTheUpdateWhenTheServerCannotRestart(t *testing.T) {
	cases := map[string]struct {
		srv    *fakeUpdateServer
		args   []string
		server string
	}{
		"no server":    {srv: &fakeUpdateServer{versionErr: selfupdate.ErrServerNotRunning}, server: "not_running"},
		"no restart":   {srv: &fakeUpdateServer{version: "1.2.3"}, args: []string{"--no-restart"}, server: "skipped"},
		"unauthorized": {srv: &fakeUpdateServer{version: "1.2.3", restartErr: selfupdate.ErrRestartUnauthorized}, server: selfupdate.ErrRestartUnauthorized.Error()},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			installed := stubUpdate(t, availableStatus, tc.srv)
			out, err := runUpdateCommand(t, append([]string{"--yes", "--json"}, tc.args...)...)
			if err != nil {
				t.Fatalf("update: %v", err)
			}
			var got updateResult
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			if !got.Updated || got.Restarted || got.Server != tc.server || len(*installed) != 1 {
				t.Fatalf("result = %+v", got)
			}
		})
	}
}

func TestUpdateCommandSurfacesCheckErrors(t *testing.T) {
	stubUpdate(t, selfupdate.Status{}, &fakeUpdateServer{})
	updateCheck = func(context.Context, selfupdate.Options) (selfupdate.Status, error) {
		return selfupdate.Status{}, selfupdate.ErrHomebrew
	}
	if _, err := runUpdateCommand(t); !errors.Is(err, selfupdate.ErrHomebrew) {
		t.Fatalf("err = %v", err)
	}
}
