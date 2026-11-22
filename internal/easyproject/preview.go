package easyproject

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The preview (vibe-coder-identity.md §4, phase 3): the person's app,
// running on this computer, shown on the result screen. TARS runs the
// project's own development server on a free loopback port and stops it
// when nobody has looked for a while. Nothing here decides anything about
// the app; it only starts, watches and stops one process per project.

// PreviewState is where a project's preview is.
type PreviewState string

const (
	PreviewStopped  PreviewState = "stopped"
	PreviewStarting PreviewState = "starting"
	PreviewRunning  PreviewState = "running"
	PreviewFailed   PreviewState = "failed"
)

// ErrorPreviewFailed is a failed preview's code: the development server
// exited, or never answered. ErrorNodeMissing is the other one.
const ErrorPreviewFailed = "preview_failed"

// Preview is what the result screen reads.
type Preview struct {
	State PreviewState `json:"state"`
	// URL is set while the preview runs.
	URL string `json:"url,omitempty"`
	// ErrorCode is set when State is failed.
	ErrorCode string `json:"error_code,omitempty"`
}

// PreviewRunner builds the command that serves dir's app on port, bound to
// 127.0.0.1.
type PreviewRunner interface {
	// Ready answers ErrNodeMissing when no preview can run here.
	Ready() error
	Command(ctx context.Context, dir string, port int) (*exec.Cmd, error)
}

// NPMPreviewRunner runs the project's own `dev` script (Vite, from the
// scaffold) on a fixed host and port. The host is named, not left to Vite:
// it answered on localhost and not on 127.0.0.1 when measured, and the
// result screen and the checks must name the same address.
type NPMPreviewRunner struct{}

// Ready reports whether npm can be found.
func (NPMPreviewRunner) Ready() error {
	_, err := findNPM()
	return err
}

// Command is `npm run dev -- --host 127.0.0.1 --port <port> --strictPort`
// in dir, with npm's folder first on PATH.
func (NPMPreviewRunner) Command(ctx context.Context, dir string, port int) (*exec.Cmd, error) {
	bin, err := findNPM()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(bin, "npm"), "run", "dev", "--", // #nosec G204 -- fixed program and arguments
		"--host", "127.0.0.1", "--port", strconv.Itoa(port), "--strictPort")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "BROWSER=none")
	return cmd, nil
}

// PreviewPathEnv names a program to run instead of npm (e2e tests): it is
// called with the project folder and the port.
const PreviewPathEnv = "TARS_EASY_PREVIEW_PATH"

// ScriptPreviewRunner runs one program with the folder and port.
type ScriptPreviewRunner struct{ Path string }

// Ready reports whether the program exists.
func (s ScriptPreviewRunner) Ready() error {
	if !isFile(s.Path) {
		return ErrNodeMissing
	}
	return nil
}

// Command runs the program for dir and port.
func (s ScriptPreviewRunner) Command(ctx context.Context, dir string, port int) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, s.Path, dir, strconv.Itoa(port)) // #nosec G204 -- the program is the server operator's own setting
	cmd.Dir = dir
	return cmd, nil
}

// PreviewRunnerFromEnv is ScriptPreviewRunner when PreviewPathEnv is set,
// else NPMPreviewRunner.
func PreviewRunnerFromEnv() PreviewRunner {
	if path := strings.TrimSpace(os.Getenv(PreviewPathEnv)); path != "" {
		return ScriptPreviewRunner{Path: path}
	}
	return NPMPreviewRunner{}
}

// Defaults for Previews.
const (
	// DefaultPreviewIdle stops a preview nobody has read for this long: the
	// result screen reads its preview every half minute while it is open.
	DefaultPreviewIdle = 10 * time.Minute
	// DefaultPreviewStartTimeout bounds how long a preview may take to
	// answer its first request.
	DefaultPreviewStartTimeout = 90 * time.Second
	// previewStopGrace is how long a stopped preview has before it is killed.
	previewStopGrace = 3 * time.Second
	previewPoll      = 250 * time.Millisecond
	// previewLogLimit caps a preview's log file; its output is for the
	// server operator, never shown on an easy screen.
	previewLogLimit = 1 << 20
)

// Previews runs at most one preview per project.
type Previews struct {
	Runner PreviewRunner
	// LogDir holds one log file per project (<id>.log), outside the
	// project folder so the saved versions never carry it.
	LogDir       string
	Idle         time.Duration
	StartTimeout time.Duration
	Now          func() time.Time
	// FreePort picks the port; tests may fix it.
	FreePort func() (int, error)

	mu      sync.Mutex
	running map[string]*preview
	closed  bool
}

type preview struct {
	url      string
	state    PreviewState
	code     string
	lastSeen time.Time
	cancel   context.CancelFunc
	done     chan struct{}
	// stopping is set when TARS stopped it, so its exit is not a failure.
	stopping bool
}

func (ps *Previews) now() time.Time {
	if ps.Now != nil {
		return ps.Now()
	}
	return time.Now()
}

// Status is the project's preview, and counts as someone looking at it.
func (ps *Previews) Status(id string) Preview {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p := ps.running[id]
	if p == nil {
		return Preview{State: PreviewStopped}
	}
	p.lastSeen = ps.now()
	return p.view()
}

func (p *preview) view() Preview {
	v := Preview{State: p.state, ErrorCode: p.code}
	if p.state == PreviewRunning {
		v.URL = p.url
	}
	return v
}

// Start starts the project's preview in dir unless one is starting or
// running, and answers its state. A failed one is started again.
func (ps *Previews) Start(id, dir string) Preview {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed {
		return Preview{State: PreviewStopped}
	}
	if p := ps.running[id]; p != nil && p.state != PreviewFailed {
		p.lastSeen = ps.now()
		return p.view()
	}
	if ps.running == nil {
		ps.running = map[string]*preview{}
	}
	p := &preview{state: PreviewStarting, lastSeen: ps.now(), done: make(chan struct{})}
	ps.running[id] = p
	if err := ps.launch(id, dir, p); err != nil {
		p.state, p.code = PreviewFailed, ErrorPreviewFailed
		if errors.Is(err, ErrNodeMissing) {
			p.code = ErrorNodeMissing
		}
		close(p.done)
	}
	return p.view()
}

// launch starts the process and the goroutines that watch it. ps.mu is held.
func (ps *Previews) launch(id, dir string, p *preview) error {
	if err := ps.Runner.Ready(); err != nil {
		return err
	}
	freePort := ps.FreePort
	if freePort == nil {
		freePort = FreeLoopbackPort
	}
	port, err := freePort()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := ps.Runner.Command(ctx, dir, port)
	if err != nil {
		cancel()
		return err
	}
	stopGroupOnCancel(cmd, previewStopGrace)
	cmd.WaitDelay = previewStopGrace * 2
	logFile := ps.openLog(id)
	if logFile != nil {
		cmd.Stdout, cmd.Stderr = logFile, logFile
	}
	if err := cmd.Start(); err != nil {
		cancel()
		if logFile != nil {
			_ = logFile.Close()
		}
		return err
	}
	p.url = fmt.Sprintf("http://127.0.0.1:%d/", port)
	p.cancel = cancel
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
		close(exited)
		ps.exited(id, p)
	}()
	go ps.waitReady(p, exited)
	return nil
}

func (ps *Previews) openLog(id string) *os.File {
	if ps.LogDir == "" || !validID(id) {
		return nil
	}
	if err := os.MkdirAll(ps.LogDir, 0o755); err != nil {
		return nil
	}
	path := filepath.Join(ps.LogDir, id+".log")
	if info, err := os.Stat(path); err == nil && info.Size() > previewLogLimit {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- the id is checked by validID
	if err != nil {
		return nil
	}
	return f
}

// waitReady marks the preview running once its address answers, or stops
// it as failed when it does not answer in time.
func (ps *Previews) waitReady(p *preview, exited <-chan struct{}) {
	timeout := ps.StartTimeout
	if timeout <= 0 {
		timeout = DefaultPreviewStartTimeout
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(previewPoll)
	defer tick.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		select {
		case <-exited:
			return
		case <-deadline.C:
			ps.mu.Lock()
			if p.state == PreviewStarting {
				p.state, p.code = PreviewFailed, ErrorPreviewFailed
				p.stopping = true
				p.cancel()
			}
			ps.mu.Unlock()
			return
		case <-tick.C:
			if answers(client, p.url) {
				ps.mu.Lock()
				if p.state == PreviewStarting {
					p.state = PreviewRunning
				}
				ps.mu.Unlock()
				return
			}
		}
	}
}

// answers reports whether anything answers HTTP at url.
func answers(client *http.Client, url string) bool {
	resp, err := client.Get(url) // #nosec G107 -- a loopback address this package chose
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// exited records a preview's process ending.
func (ps *Previews) exited(id string, p *preview) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	defer close(p.done)
	if p.stopping {
		if p.state != PreviewFailed && ps.running[id] == p {
			delete(ps.running, id)
		}
		return
	}
	// It ended by itself: the app's server crashed or never started.
	p.state, p.code = PreviewFailed, ErrorPreviewFailed
}

// Stop stops the project's preview and waits for it to end.
func (ps *Previews) Stop(id string) {
	ps.mu.Lock()
	p := ps.running[id]
	delete(ps.running, id)
	if p != nil && p.cancel != nil {
		p.stopping = true
		p.cancel()
	}
	ps.mu.Unlock()
	if p != nil {
		<-p.done
	}
}

// StopIdle stops the previews nobody has read for Idle, and drops failed
// ones nobody has read either. It answers how many it stopped.
func (ps *Previews) StopIdle() int {
	idle := ps.Idle
	if idle <= 0 {
		idle = DefaultPreviewIdle
	}
	ps.mu.Lock()
	var stale []string
	for id, p := range ps.running {
		if ps.now().Sub(p.lastSeen) >= idle {
			stale = append(stale, id)
		}
	}
	ps.mu.Unlock()
	for _, id := range stale {
		ps.Stop(id)
	}
	return len(stale)
}

// Close stops every preview; no new one starts after it.
func (ps *Previews) Close() {
	ps.mu.Lock()
	ps.closed = true
	ids := make([]string, 0, len(ps.running))
	for id := range ps.running {
		ids = append(ids, id)
	}
	ps.mu.Unlock()
	for _, id := range ids {
		ps.Stop(id)
	}
}

// FreeLoopbackPort asks the system for a free port on 127.0.0.1.
func FreeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
