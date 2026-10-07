package serverupdate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/devlikebear/tars/desktop/internal/activity"
	"github.com/devlikebear/tars/desktop/internal/server"
)

type call struct {
	bin  string
	args []string
	env  []string
}

func recorder(stdout, stderr string, err error) (Run, *[]call) {
	calls := &[]call{}
	return func(_ context.Context, bin string, args, env []string) ([]byte, []byte, error) {
		*calls = append(*calls, call{bin, args, env})
		return []byte(stdout), []byte(stderr), err
	}, calls
}

func TestCheckAndApplyRunTarsUpdate(t *testing.T) {
	run, calls := recorder("some log line\n"+`{"current":"1.2.3","latest":"1.3.0","available":true}`+"\n", "", nil)
	u := Updater{Bin: `C:\TARS\tars.exe`, Cfg: server.Config{URL: "http://127.0.0.1:43180", Token: "user", AdminToken: "admin"}, Run: run}

	got, err := u.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := (Result{Current: "1.2.3", Latest: "1.3.0", Available: true}); got != want {
		t.Fatalf("check = %+v", got)
	}
	if _, err := u.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{`C:\TARS\tars.exe`, []string{"update", "--json", "--check", "--server-url", "http://127.0.0.1:43180"}, []string{"TARS_ADMIN_API_TOKEN=admin"}},
		{`C:\TARS\tars.exe`, []string{"update", "--json", "--yes", "--server-url", "http://127.0.0.1:43180"}, []string{"TARS_ADMIN_API_TOKEN=admin"}},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %#v", *calls)
	}
	for _, c := range *calls {
		if strings.Contains(strings.Join(c.args, " "), "admin") {
			t.Fatal("the admin token must not be on the command line")
		}
	}
}

func TestRunErrors(t *testing.T) {
	run, _ := recorder("", "Error: this tars is managed by Homebrew; update it with: brew upgrade devlikebear/tap/tars\nmore", errors.New("exit status 1"))
	_, err := Updater{Bin: "tars", Run: run}.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "managed by Homebrew") || strings.Contains(err.Error(), "more") {
		t.Fatalf("err = %v", err)
	}

	// A tars from before `tars update` prints help, not JSON.
	run, _ = recorder("Web console and automation CLI for tars\n", "", nil)
	if _, err := (Updater{Bin: "tars", Run: run}).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "older than `tars update`") {
		t.Fatalf("old tars: err = %v", err)
	}

	run, _ = recorder("{not json}\n", "", nil)
	if _, err := (Updater{Bin: "tars", Run: run}).Check(context.Background()); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func TestDecideOnlyRestartsAnIdleServer(t *testing.T) {
	available := Result{Available: true}
	busy := []activity.Snapshot{
		{Running: []activity.Running{{SessionID: "s1"}}},
		{Pending: []activity.Approval{{RequestID: "r1"}}},
		{Queued: []activity.QueuedApproval{{ApprovalID: "a1"}}},
	}
	for i, snap := range busy {
		if got := Decide(available, snap, nil); got != WaitForIdle {
			t.Fatalf("busy %d: %v", i, got)
		}
	}
	if got := Decide(available, activity.Snapshot{}, errors.New("401")); got != WaitForIdle {
		t.Fatalf("unknown activity must wait, got %v", got)
	}
	if got := Decide(available, activity.Snapshot{}, nil); got != ApplyNow {
		t.Fatalf("idle: %v", got)
	}
	if got := Decide(Result{}, busy[0], nil); got != UpToDate {
		t.Fatalf("up to date: %v", got)
	}
	if Next(WaitForIdle) != RetryBusyEvery || Next(ApplyNow) != CheckEvery || Next(UpToDate) != CheckEvery {
		t.Fatal("Next misjudged a step")
	}
	if !Enabled("windows") || !Enabled("darwin") || Enabled("linux") {
		t.Fatal("Enabled is Windows and macOS")
	}
}
