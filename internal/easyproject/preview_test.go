package easyproject

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The preview tests run this test binary as the "development server": the
// helper below serves, exits or hangs as the environment says. It works on
// every platform, unlike a shell script.
const previewHelperEnv = "TARS_EASY_PREVIEW_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(previewHelperEnv) {
	case "":
		os.Exit(m.Run())
	case "serve":
		port := os.Args[len(os.Args)-1]
		_ = http.ListenAndServe("127.0.0.1:"+port, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "hello from the app")
		}))
		os.Exit(1)
	case "exit":
		os.Exit(3)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

type helperRunner struct {
	mode  string
	ready error
}

func (h helperRunner) Ready() error { return h.ready }

func (h helperRunner) Command(ctx context.Context, dir string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$", strconv.Itoa(port)) // #nosec G204 -- the test binary
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), previewHelperEnv+"="+h.mode)
	return cmd, nil
}

func waitPreview(t *testing.T, ps *Previews, id string, want PreviewState) Preview {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		p := ps.Status(id)
		if p.State == want {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("preview state = %+v, want %s", p, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

const testProjectID = "0123456789abcdef"

func TestPreviewRunsAnswersAndStops(t *testing.T) {
	logDir := t.TempDir()
	ps := &Previews{Runner: helperRunner{mode: "serve"}, LogDir: logDir}
	defer ps.Close()

	if got := ps.Status(testProjectID); got.State != PreviewStopped {
		t.Fatalf("before start: %+v", got)
	}
	first := ps.Start(testProjectID, t.TempDir())
	if first.State != PreviewStarting || first.URL != "" {
		t.Fatalf("start = %+v, want starting without an address", first)
	}
	running := waitPreview(t, ps, testProjectID, PreviewRunning)
	resp, err := http.Get(running.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "hello from the app" {
		t.Fatalf("body = %q", body)
	}
	// Starting again while it runs keeps the same server.
	if again := ps.Start(testProjectID, t.TempDir()); again.URL != running.URL {
		t.Fatalf("second start = %+v, want the running %s", again, running.URL)
	}
	if _, err := os.Stat(filepath.Join(logDir, testProjectID+".log")); err != nil {
		t.Fatalf("log file: %v", err)
	}

	ps.Stop(testProjectID)
	if got := ps.Status(testProjectID); got.State != PreviewStopped {
		t.Fatalf("after stop: %+v", got)
	}
	if _, err := (&http.Client{Timeout: time.Second}).Get(running.URL); err == nil {
		t.Fatal("the stopped preview still answers")
	}
}

func TestPreviewThatExitsOrNeverAnswersFails(t *testing.T) {
	ps := &Previews{Runner: helperRunner{mode: "exit"}}
	defer ps.Close()
	ps.Start(testProjectID, t.TempDir())
	if got := waitPreview(t, ps, testProjectID, PreviewFailed); got.ErrorCode != ErrorPreviewFailed {
		t.Fatalf("exit: %+v", got)
	}

	hang := &Previews{Runner: helperRunner{mode: "hang"}, StartTimeout: 300 * time.Millisecond}
	defer hang.Close()
	hang.Start(testProjectID, t.TempDir())
	if got := waitPreview(t, hang, testProjectID, PreviewFailed); got.ErrorCode != ErrorPreviewFailed {
		t.Fatalf("hang: %+v", got)
	}
	// A failed preview is started again.
	hang.Runner = helperRunner{mode: "serve"}
	hang.StartTimeout = 0
	if got := hang.Start(testProjectID, t.TempDir()); got.State != PreviewStarting {
		t.Fatalf("restart: %+v", got)
	}
	waitPreview(t, hang, testProjectID, PreviewRunning)
}

func TestPreviewWithoutNode(t *testing.T) {
	ps := &Previews{Runner: helperRunner{mode: "serve", ready: ErrNodeMissing}}
	got := ps.Start(testProjectID, t.TempDir())
	if got.State != PreviewFailed || got.ErrorCode != ErrorNodeMissing {
		t.Fatalf("got %+v", got)
	}
}

func TestPreviewStopsWhenNobodyLooks(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	advance := func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() }

	ps := &Previews{Runner: helperRunner{mode: "serve"}, Now: clock, Idle: 10 * time.Minute}
	defer ps.Close()
	ps.Start(testProjectID, t.TempDir())
	waitPreview(t, ps, testProjectID, PreviewRunning)

	advance(9 * time.Minute)
	ps.Status(testProjectID) // someone looked
	advance(9 * time.Minute)
	if n := ps.StopIdle(); n != 0 {
		t.Fatalf("stopped %d while someone looked 9 minutes ago", n)
	}
	advance(2 * time.Minute)
	if n := ps.StopIdle(); n != 1 {
		t.Fatalf("stopped %d, want 1", n)
	}
	if got := ps.Status(testProjectID); got.State != PreviewStopped {
		t.Fatalf("after idle: %+v", got)
	}
}

func TestPreviewsCloseStopsAllAndRefusesNew(t *testing.T) {
	ps := &Previews{Runner: helperRunner{mode: "serve"}}
	ps.Start(testProjectID, t.TempDir())
	running := waitPreview(t, ps, testProjectID, PreviewRunning)
	ps.Close()
	if _, err := (&http.Client{Timeout: time.Second}).Get(running.URL); err == nil {
		t.Fatal("a preview still answers after Close")
	}
	if got := ps.Start(testProjectID, t.TempDir()); got.State != PreviewStopped {
		t.Fatalf("start after close: %+v", got)
	}
}

func TestNPMPreviewRunnerNamesTheHostAndPort(t *testing.T) {
	bin, _ := fakeNode(t, "exit 0", "exit 0")
	if err := (NPMPreviewRunner{}).Ready(); err != nil {
		t.Fatal(err)
	}
	cmd, err := (NPMPreviewRunner{}).Command(context.Background(), "/p", 5173)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(bin, "npm"), "run", "dev", "--", "--host", "127.0.0.1", "--port", "5173", "--strictPort"}
	if strings.Join(cmd.Args, " ") != strings.Join(want, " ") || cmd.Dir != "/p" {
		t.Fatalf("command = %v in %s", cmd.Args, cmd.Dir)
	}
	t.Setenv("PATH", t.TempDir())
	if err := (NPMPreviewRunner{}).Ready(); !errors.Is(err, ErrNodeMissing) {
		t.Fatalf("without npm: %v", err)
	}
}

func TestPreviewRunnerFromEnv(t *testing.T) {
	t.Setenv(PreviewPathEnv, "")
	if _, ok := PreviewRunnerFromEnv().(NPMPreviewRunner); !ok {
		t.Fatal("want the npm runner by default")
	}
	script := filepath.Join(t.TempDir(), "preview")
	t.Setenv(PreviewPathEnv, script)
	runner := PreviewRunnerFromEnv()
	if runner.Ready() == nil {
		t.Fatal("a missing program is not ready")
	}
	if err := os.WriteFile(script, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runner.Ready(); err != nil {
		t.Fatal(err)
	}
	cmd, err := runner.Command(context.Background(), "/p", 4321)
	if err != nil || cmd.Args[1] != "/p" || cmd.Args[2] != "4321" {
		t.Fatalf("command = %v, %v", cmd.Args, err)
	}
}
