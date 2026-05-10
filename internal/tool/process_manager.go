package tool

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// processBufferCap caps the in-memory rolling tail kept per stream. The
// snapshot helper trims further to maxExecOutputBytes when serializing,
// so this is purely an upper bound to prevent runaway memory from
// long-running watchers (e.g. `gh pr checks --watch`).
const processBufferCap = 64 * 1024

// defaultProcessMaxTimeoutMS is the per-process upper bound when the
// caller doesn't override it. Background processes are typically
// long-running watchers (CI, builds), so this is much larger than the
// synchronous exec cap. Wait() callers can still pass shorter timeouts.
const defaultProcessMaxTimeoutMS = 1800000 // 30 minutes

// tailBuffer is an io.Writer that keeps the most recent `cap` bytes
// written to it, dropping the oldest data when the buffer would exceed
// `cap`. It is safe for concurrent use because cmd.Stdout/Stderr can be
// written from a goroutine while a Wait() caller reads via String().
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newTailBuffer(capacity int) *tailBuffer {
	if capacity <= 0 {
		capacity = processBufferCap
	}
	return &tailBuffer{cap: capacity}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(p) >= t.cap {
		t.buf = append(t.buf[:0], p[len(p)-t.cap:]...)
		return len(p), nil
	}
	if len(t.buf)+len(p) > t.cap {
		drop := len(t.buf) + len(p) - t.cap
		t.buf = t.buf[drop:]
	}
	t.buf = append(t.buf, p...)
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

type ProcessSnapshot struct {
	SessionID   string `json:"session_id"`
	Command     string `json:"command"`
	Running     bool   `json:"running"`
	Done        bool   `json:"done"`
	ExitCode    int    `json:"exit_code,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Stdout      string `json:"stdout,omitempty"`
	Stderr      string `json:"stderr,omitempty"`
	Message     string `json:"message,omitempty"`
	// WaitTimedOut signals that a process wait call returned before the
	// process exited because the wait_timeout_ms elapsed.
	WaitTimedOut bool `json:"wait_timed_out,omitempty"`
}

type managedProcess struct {
	sessionID string
	command   string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	stdin     io.WriteCloser
	stdout    *tailBuffer
	stderr    *tailBuffer
	startedAt time.Time
	endedAt   time.Time
	done      bool
	doneCh    chan struct{}
	exitCode  int
	runErr    error
	mu        sync.RWMutex
}

type ProcessManager struct {
	mu       sync.RWMutex
	sessions map[string]*managedProcess
	counter  atomic.Uint64
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{sessions: map[string]*managedProcess{}}
}

func (m *ProcessManager) Start(ctx context.Context, workspaceDir, commandLine string, timeoutMS int) (ProcessSnapshot, error) {
	if m == nil {
		return ProcessSnapshot{}, fmt.Errorf("process manager is not configured")
	}
	fields := strings.Fields(strings.TrimSpace(commandLine))
	if len(fields) == 0 {
		return ProcessSnapshot{}, fmt.Errorf("command is required")
	}
	if timeoutMS < minExecTimeoutMS {
		timeoutMS = minExecTimeoutMS
	}
	if timeoutMS > defaultProcessMaxTimeoutMS {
		timeoutMS = defaultProcessMaxTimeoutMS
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	cmd := exec.CommandContext(runCtx, fields[0], fields[1:]...)
	cmd.Dir = workspaceDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return ProcessSnapshot{}, fmt.Errorf("create stdin pipe: %w", err)
	}
	mp := &managedProcess{
		sessionID: fmt.Sprintf("proc_%d", m.counter.Add(1)),
		command:   commandLine,
		cmd:       cmd,
		cancel:    cancel,
		stdin:     stdin,
		stdout:    newTailBuffer(processBufferCap),
		stderr:    newTailBuffer(processBufferCap),
		startedAt: time.Now().UTC(),
		exitCode:  -1,
		doneCh:    make(chan struct{}),
	}
	cmd.Stdout = mp.stdout
	cmd.Stderr = mp.stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return ProcessSnapshot{}, fmt.Errorf("start process: %w", err)
	}
	m.mu.Lock()
	m.sessions[mp.sessionID] = mp
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		mp.mu.Lock()
		mp.done = true
		mp.endedAt = time.Now().UTC()
		mp.runErr = err
		if err == nil {
			mp.exitCode = 0
		} else if ex, ok := err.(*exec.ExitError); ok {
			mp.exitCode = ex.ExitCode()
		} else {
			mp.exitCode = -1
		}
		close(mp.doneCh)
		mp.mu.Unlock()
	}()

	return m.snapshot(mp, true), nil
}

func (m *ProcessManager) List() []ProcessSnapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ProcessSnapshot, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.snapshot(m.sessions[id], false))
	}
	return out
}

func (m *ProcessManager) Poll(sessionID string) (ProcessSnapshot, error) {
	mp, err := m.get(sessionID)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	return m.snapshot(mp, true), nil
}

func (m *ProcessManager) Log(sessionID string) (ProcessSnapshot, error) {
	mp, err := m.get(sessionID)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	return m.snapshot(mp, true), nil
}

// Wait blocks until the named process finishes, the per-call timeout
// elapses, or ctx is cancelled. The returned bool reports whether the
// wait timed out (process is still running). This is the cheap
// alternative to having the LLM repeatedly call Poll, which would burn
// agent loop iterations for every check.
func (m *ProcessManager) Wait(ctx context.Context, sessionID string, timeoutMS int) (ProcessSnapshot, bool, error) {
	mp, err := m.get(sessionID)
	if err != nil {
		return ProcessSnapshot{}, false, err
	}
	if timeoutMS <= 0 {
		timeoutMS = defaultProcessMaxTimeoutMS
	}
	if timeoutMS > defaultProcessMaxTimeoutMS {
		timeoutMS = defaultProcessMaxTimeoutMS
	}
	if timeoutMS < minExecTimeoutMS {
		timeoutMS = minExecTimeoutMS
	}
	timer := time.NewTimer(time.Duration(timeoutMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-mp.doneCh:
		return m.snapshot(mp, true), false, nil
	case <-timer.C:
		snap := m.snapshot(mp, true)
		snap.WaitTimedOut = true
		return snap, true, nil
	case <-ctx.Done():
		return m.snapshot(mp, true), false, ctx.Err()
	}
}

func (m *ProcessManager) Write(sessionID string, chars string) (ProcessSnapshot, error) {
	mp, err := m.get(sessionID)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	mp.mu.Lock()
	if mp.done {
		mp.mu.Unlock()
		return m.snapshot(mp, true), fmt.Errorf("process already completed")
	}
	if mp.stdin == nil {
		mp.mu.Unlock()
		return m.snapshot(mp, true), fmt.Errorf("stdin is not available")
	}
	if _, err := io.WriteString(mp.stdin, chars); err != nil {
		mp.mu.Unlock()
		return m.snapshot(mp, true), fmt.Errorf("write stdin failed: %w", err)
	}
	mp.mu.Unlock()
	return m.snapshot(mp, true), nil
}

func (m *ProcessManager) Kill(sessionID string) (ProcessSnapshot, error) {
	mp, err := m.get(sessionID)
	if err != nil {
		return ProcessSnapshot{}, err
	}
	mp.mu.Lock()
	if mp.done {
		mp.mu.Unlock()
		return m.snapshot(mp, true), nil
	}
	if mp.cancel != nil {
		mp.cancel()
	}
	if mp.cmd != nil && mp.cmd.Process != nil {
		if err := mp.cmd.Process.Kill(); err != nil {
			mp.mu.Unlock()
			return m.snapshot(mp, true), fmt.Errorf("kill process failed: %w", err)
		}
	}
	mp.mu.Unlock()
	return m.snapshot(mp, true), nil
}

func (m *ProcessManager) Remove(sessionID string) error {
	if m == nil {
		return fmt.Errorf("process manager is not configured")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(m.sessions, sessionID)
	return nil
}

func (m *ProcessManager) ClearDone() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, mp := range m.sessions {
		mp.mu.RLock()
		done := mp.done
		mp.mu.RUnlock()
		if done {
			delete(m.sessions, id)
			removed++
		}
	}
	return removed
}

func (m *ProcessManager) get(sessionID string) (*managedProcess, error) {
	if m == nil {
		return nil, fmt.Errorf("process manager is not configured")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	mp, ok := m.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", strings.TrimSpace(sessionID))
	}
	return mp, nil
}

func (m *ProcessManager) snapshot(mp *managedProcess, withOutput bool) ProcessSnapshot {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	snap := ProcessSnapshot{
		SessionID: mp.sessionID,
		Command:   mp.command,
		Running:   !mp.done,
		Done:      mp.done,
		ExitCode:  mp.exitCode,
		StartedAt: mp.startedAt.Format(time.RFC3339),
	}
	if !mp.endedAt.IsZero() {
		snap.CompletedAt = mp.endedAt.Format(time.RFC3339)
	}
	if withOutput {
		snap.Stdout = trimOutput(mp.stdout.String(), maxExecOutputBytes)
		snap.Stderr = trimOutput(mp.stderr.String(), maxExecOutputBytes)
	}
	if mp.runErr != nil {
		snap.Message = mp.runErr.Error()
	}
	return snap
}
