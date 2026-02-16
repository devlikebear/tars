package tool

import (
	"bytes"
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

type ProcessSnapshot struct {
	SessionID  string `json:"session_id"`
	Command    string `json:"command"`
	Running    bool   `json:"running"`
	Done       bool   `json:"done"`
	ExitCode   int    `json:"exit_code,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	Message    string `json:"message,omitempty"`
}

type managedProcess struct {
	sessionID string
	command   string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	stdin     io.WriteCloser
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	startedAt time.Time
	endedAt   time.Time
	done      bool
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
	if timeoutMS > maxExecTimeoutMS {
		timeoutMS = maxExecTimeoutMS
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
		startedAt: time.Now().UTC(),
		exitCode:  -1,
	}
	cmd.Stdout = &mp.stdout
	cmd.Stderr = &mp.stderr

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
		defer mp.mu.Unlock()
		mp.done = true
		mp.endedAt = time.Now().UTC()
		mp.runErr = err
		if err == nil {
			mp.exitCode = 0
			return
		}
		if ex, ok := err.(*exec.ExitError); ok {
			mp.exitCode = ex.ExitCode()
			return
		}
		mp.exitCode = -1
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
