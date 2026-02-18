package sentinel

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Supervisor struct {
	opts Options

	nowFn      func() time.Time
	httpClient *http.Client
	eventStore eventStore

	mu                  sync.RWMutex
	started             bool
	closed              bool
	state               SupervisionState
	paused              bool
	targetCmd           *exec.Cmd
	targetExitCh        <-chan processExit
	targetPID           int
	targetStartedAt     time.Time
	targetLastExitAt    time.Time
	targetLastExitCode  int
	hasLastExitCode     bool
	healthOK            bool
	healthLastOKAt      time.Time
	healthLastError     string
	restartAttempt      int
	cooldownUntil       time.Time
	lastRestartAt       time.Time
	nextStartAt         time.Time
	lastProbeAt         time.Time
	lastProbeDuration   time.Duration
	startGraceUntil     time.Time
	probeFailures       int
	manualRestart       bool
	restartOnExitReason string
	events              []Event
	eventSeq            int64
	eventsRestored      int
	lastEventPersistAt  time.Time
	lastEventRestoreAt  time.Time
	lastEventRestoreErr string

	loopCancel context.CancelFunc
	wg         sync.WaitGroup
}

func NewSupervisor(opts Options) *Supervisor {
	if !opts.Enabled {
		opts.Enabled = true
	}
	if opts.ProbeInterval <= 0 {
		opts.ProbeInterval = 5 * time.Second
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = time.Second
	}
	if opts.ProbeFailThreshold <= 0 {
		opts.ProbeFailThreshold = 3
	}
	if opts.RestartMaxAttempts <= 0 {
		opts.RestartMaxAttempts = 3
	}
	if opts.RestartBackoff <= 0 {
		opts.RestartBackoff = time.Second
	}
	if opts.RestartBackoffMax <= 0 {
		opts.RestartBackoffMax = 10 * time.Second
	}
	if opts.RestartCooldown <= 0 {
		opts.RestartCooldown = time.Minute
	}
	if opts.EventBufferSize <= 0 {
		opts.EventBufferSize = 200
	}
	if opts.EventStoreMaxRecords <= 0 {
		opts.EventStoreMaxRecords = 5000
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: opts.ProbeTimeout}
	}
	state := StateStopped
	if opts.Autostart {
		state = StateStarting
	}
	s := &Supervisor{
		opts:       opts,
		nowFn:      nowFn,
		httpClient: client,
		eventStore: newEventStore(opts.EventStorePath, opts.EventStoreMaxRecords),
		state:      state,
		events:     make([]Event, 0, opts.EventBufferSize),
	}
	s.restoreEvents()
	return s
}

func (s *Supervisor) restoreEvents() {
	if s == nil || !s.opts.EventPersistenceEnabled || !s.eventStore.enabled() {
		return
	}
	events, err := s.eventStore.read()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEventRestoreAt = s.nowFn().UTC()
	if err != nil {
		s.lastEventRestoreErr = strings.TrimSpace(err.Error())
		return
	}
	s.lastEventRestoreErr = ""
	if len(events) > s.opts.EventBufferSize {
		events = events[len(events)-s.opts.EventBufferSize:]
	}
	s.events = append([]Event(nil), events...)
	var maxID int64
	for _, evt := range s.events {
		if evt.ID > maxID {
			maxID = evt.ID
		}
	}
	s.eventSeq = maxID
	s.eventsRestored = len(s.events)
}

func (s *Supervisor) Start(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("supervisor is nil")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("supervisor already started")
	}
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("supervisor is closed")
	}
	s.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	s.loopCancel = cancel
	if s.opts.Autostart {
		s.nextStartAt = s.nowFn()
		s.state = StateStarting
	} else {
		s.state = StateStopped
	}
	s.appendEventLocked("info", EventStart, "sentinel supervisor started", nil)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.runLoop(loopCtx)
	return nil
}

func (s *Supervisor) runLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.controlTickInterval())
	defer ticker.Stop()
	for {
		exitCh := s.currentExitCh()
		select {
		case <-ctx.Done():
			s.shutdownProcess()
			return
		case ex, ok := <-exitCh:
			if ok {
				s.handleProcessExit(ex)
			}
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Supervisor) controlTickInterval() time.Duration {
	if s == nil {
		return 50 * time.Millisecond
	}
	d := s.opts.ProbeInterval / 2
	if d < 10*time.Millisecond {
		d = 10 * time.Millisecond
	}
	if d > 200*time.Millisecond {
		d = 200 * time.Millisecond
	}
	return d
}

func (s *Supervisor) currentExitCh() <-chan processExit {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.targetExitCh
}

func (s *Supervisor) tick() {
	if s == nil {
		return
	}
	now := s.nowFn()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if s.paused {
		s.state = StatePaused
		s.mu.Unlock()
		return
	}
	if s.state == StateCooldown {
		if now.Before(s.cooldownUntil) {
			s.mu.Unlock()
			return
		}
		s.cooldownUntil = time.Time{}
		s.restartAttempt = 0
		s.state = StateStopped
		s.appendEventLocked("info", EventCooldownExit, "cooldown finished", nil)
	}
	if s.targetPID == 0 {
		if s.nextStartAt.IsZero() && s.opts.Autostart {
			s.nextStartAt = now
		}
		if !s.nextStartAt.IsZero() && !now.Before(s.nextStartAt) {
			s.mu.Unlock()
			s.startProcess("scheduled")
			return
		}
		s.mu.Unlock()
		return
	}
	s.state = StateRunning
	if strings.TrimSpace(s.opts.ProbeURL) == "" {
		s.mu.Unlock()
		return
	}
	if !s.lastProbeAt.IsZero() && now.Sub(s.lastProbeAt) < s.opts.ProbeInterval {
		s.mu.Unlock()
		return
	}
	s.lastProbeAt = now
	s.mu.Unlock()

	startedAt := s.nowFn()
	err := probeHealth(s.httpClient, s.opts.ProbeURL, s.opts.ProbeTimeout)
	s.handleProbeResult(err, s.nowFn().Sub(startedAt))
}

func (s *Supervisor) startProcess(reason string) {
	now := s.nowFn().UTC()
	cmd, pid, exitCh, err := startTargetProcess(s.opts)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.paused {
		if cmd != nil {
			_ = stopTargetProcess(cmd)
		}
		return
	}
	if s.targetPID != 0 {
		if cmd != nil {
			_ = stopTargetProcess(cmd)
		}
		return
	}
	if err != nil {
		s.state = StateError
		s.healthOK = false
		s.healthLastError = err.Error()
		s.appendEventLocked("error", EventError, "failed to start target", map[string]any{"error": err.Error()})
		s.scheduleRestartLocked(now, "start failure")
		return
	}
	s.targetCmd = cmd
	s.targetExitCh = exitCh
	s.targetPID = pid
	s.targetStartedAt = now
	s.restartOnExitReason = ""
	s.nextStartAt = time.Time{}
	s.state = StateRunning
	s.probeFailures = 0
	s.lastProbeDuration = 0
	s.lastProbeAt = time.Time{}
	s.startGraceUntil = now.Add(s.opts.ProbeStartGrace)
	s.appendEventLocked("info", EventStart, "target started", map[string]any{"pid": pid, "reason": reason})
}

func (s *Supervisor) handleProbeResult(err error, duration time.Duration) {
	now := s.nowFn().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.targetPID == 0 || s.paused {
		return
	}
	s.lastProbeDuration = duration
	if err == nil {
		s.probeFailures = 0
		s.healthLastError = ""
		s.healthLastOKAt = now
		if !s.healthOK {
			s.appendEventLocked("info", EventHealthOK, "health probe recovered", nil)
		}
		s.healthOK = true
		return
	}
	if !s.startGraceUntil.IsZero() && now.Before(s.startGraceUntil) {
		s.healthOK = false
		s.healthLastError = err.Error()
		s.probeFailures = 0
		return
	}
	s.healthOK = false
	s.healthLastError = err.Error()
	s.probeFailures++
	if s.probeFailures < s.opts.ProbeFailThreshold {
		return
	}
	s.appendEventLocked("warn", EventHealthFail, "health probe threshold exceeded", map[string]any{
		"error":      err.Error(),
		"fail_count": s.probeFailures,
	})
	s.probeFailures = 0
	if s.targetCmd != nil {
		s.restartOnExitReason = "health probe failure"
		_ = stopTargetProcess(s.targetCmd)
	}
}

func (s *Supervisor) handleProcessExit(ex processExit) {
	now := s.nowFn().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.targetPID == 0 {
		return
	}
	if ex.pid != 0 && s.targetPID != ex.pid {
		return
	}
	s.targetPID = 0
	s.targetCmd = nil
	s.targetExitCh = nil
	s.targetLastExitAt = now
	s.targetLastExitCode = ex.exitCode
	s.hasLastExitCode = true
	s.state = StateStopped
	s.startGraceUntil = time.Time{}
	meta := map[string]any{"exit_code": ex.exitCode}
	if ex.err != nil {
		meta["error"] = ex.err.Error()
	}
	s.appendEventLocked("info", EventExit, "target exited", meta)
	if s.closed || s.paused {
		return
	}
	if s.manualRestart {
		s.manualRestart = false
		s.state = StateStarting
		s.nextStartAt = now
		return
	}
	if reason := strings.TrimSpace(s.restartOnExitReason); reason != "" {
		s.restartOnExitReason = ""
		s.scheduleRestartLocked(now, reason)
		return
	}
	s.scheduleRestartLocked(now, "target exit")
}

func (s *Supervisor) scheduleRestartLocked(now time.Time, reason string) {
	s.restartAttempt++
	if s.restartAttempt > s.opts.RestartMaxAttempts {
		s.state = StateCooldown
		s.cooldownUntil = now.Add(s.opts.RestartCooldown)
		s.nextStartAt = time.Time{}
		s.appendEventLocked("warn", EventCooldownEnter, "restart attempts exceeded; entering cooldown", map[string]any{
			"restart_attempt": s.restartAttempt,
			"max_attempts":    s.opts.RestartMaxAttempts,
			"reason":          reason,
		})
		return
	}
	backoff := computeBackoff(s.restartAttempt, s.opts.RestartBackoff, s.opts.RestartBackoffMax)
	s.lastRestartAt = now
	s.nextStartAt = now.Add(backoff)
	s.state = StateStarting
	s.appendEventLocked("warn", EventRestart, "scheduled target restart", map[string]any{
		"restart_attempt": s.restartAttempt,
		"backoff_ms":      backoff.Milliseconds(),
		"reason":          reason,
	})
}

func (s *Supervisor) Restart(_ context.Context) (Status, error) {
	if s == nil {
		return Status{}, fmt.Errorf("supervisor is nil")
	}
	now := s.nowFn().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Status{}, fmt.Errorf("supervisor is closed")
	}
	s.manualRestart = true
	s.restartAttempt = 0
	s.cooldownUntil = time.Time{}
	s.lastRestartAt = now
	s.nextStartAt = now
	s.restartOnExitReason = ""
	s.state = StateStarting
	pid := s.targetPID
	cmd := s.targetCmd
	s.appendEventLocked("info", EventRestart, "manual restart requested", map[string]any{"pid": pid})
	s.mu.Unlock()

	if cmd != nil {
		_ = stopTargetProcess(cmd)
	}
	if pid == 0 {
		s.startProcess("manual")
	}
	return s.Status(), nil
}

func (s *Supervisor) Pause() Status {
	if s == nil {
		return Status{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.statusLocked()
	}
	s.paused = true
	s.state = StatePaused
	s.appendEventLocked("info", EventPause, "supervision paused", nil)
	return s.statusLocked()
}

func (s *Supervisor) Resume() Status {
	if s == nil {
		return Status{}
	}
	now := s.nowFn().UTC()
	s.mu.Lock()
	if s.closed {
		defer s.mu.Unlock()
		return s.statusLocked()
	}
	s.paused = false
	s.lastProbeAt = time.Time{}
	if s.targetPID > 0 {
		s.state = StateRunning
	} else {
		s.state = StateStarting
		s.nextStartAt = now
	}
	s.appendEventLocked("info", EventResume, "supervision resumed", nil)
	status := s.statusLocked()
	s.mu.Unlock()
	if status.TargetPID == 0 {
		s.startProcess("resume")
	}
	return status
}

func (s *Supervisor) Status() Status {
	if s == nil {
		return Status{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statusLocked()
}

func (s *Supervisor) statusLocked() Status {
	status := Status{
		Enabled:                 s.opts.Enabled,
		SupervisionState:        s.state,
		Target:                  TargetSummary{Command: s.opts.TargetCommand, Args: append([]string(nil), s.opts.TargetArgs...), Cwd: s.opts.TargetWorkingDir},
		TargetPID:               s.targetPID,
		HealthOK:                s.healthOK,
		HealthLastError:         s.healthLastError,
		RestartAttempt:          s.restartAttempt,
		RestartMaxAttempts:      s.opts.RestartMaxAttempts,
		ConsecutiveFailures:     s.probeFailures,
		LastProbeDurationMS:     durationMillisCeil(s.lastProbeDuration),
		EventPersistenceEnabled: s.opts.EventPersistenceEnabled,
		EventsRestored:          s.eventsRestored,
		LastEventRestoreError:   strings.TrimSpace(s.lastEventRestoreErr),
		EventCount:              len(s.events),
	}
	if !s.targetStartedAt.IsZero() {
		status.TargetStartedAt = s.targetStartedAt.Format(time.RFC3339)
	}
	if !s.targetLastExitAt.IsZero() {
		status.TargetLastExitAt = s.targetLastExitAt.Format(time.RFC3339)
	}
	if s.hasLastExitCode {
		exitCode := s.targetLastExitCode
		status.TargetLastExitCode = &exitCode
	}
	if !s.healthLastOKAt.IsZero() {
		status.HealthLastOKAt = s.healthLastOKAt.Format(time.RFC3339)
	}
	if !s.cooldownUntil.IsZero() {
		status.CooldownUntil = s.cooldownUntil.Format(time.RFC3339)
	}
	if !s.lastRestartAt.IsZero() {
		status.LastRestartAt = s.lastRestartAt.Format(time.RFC3339)
	}
	if !s.startGraceUntil.IsZero() {
		status.StartGraceUntil = s.startGraceUntil.Format(time.RFC3339)
	}
	if !s.lastEventPersistAt.IsZero() {
		status.LastEventPersistAt = s.lastEventPersistAt.UTC().Format(time.RFC3339)
	}
	if !s.lastEventRestoreAt.IsZero() {
		status.LastEventRestoreAt = s.lastEventRestoreAt.UTC().Format(time.RFC3339)
	}
	return status
}

func (s *Supervisor) Events(limit int) []Event {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.events) == 0 {
		return []Event{}
	}
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	start := len(s.events) - limit
	out := make([]Event, 0, limit)
	for _, evt := range s.events[start:] {
		copyEvt := evt
		if len(evt.Meta) > 0 {
			copyEvt.Meta = make(map[string]any, len(evt.Meta))
			for key, value := range evt.Meta {
				copyEvt.Meta[key] = value
			}
		}
		out = append(out, copyEvt)
	}
	return out
}

func (s *Supervisor) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancel := s.loopCancel
	cmd := s.targetCmd
	s.state = StateStopped
	s.appendEventLocked("info", EventExit, "sentinel supervisor stopping", nil)
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if cmd != nil {
		_ = stopTargetProcess(cmd)
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Supervisor) shutdownProcess() {
	s.mu.Lock()
	cmd := s.targetCmd
	s.mu.Unlock()
	if cmd != nil {
		_ = stopTargetProcess(cmd)
	}
}

func (s *Supervisor) appendEventLocked(level string, eventType EventType, message string, meta map[string]any) {
	s.eventSeq++
	evt := Event{
		ID:      s.eventSeq,
		Time:    s.nowFn().UTC().Format(time.RFC3339),
		Level:   strings.TrimSpace(level),
		Type:    eventType,
		Message: strings.TrimSpace(message),
	}
	if len(meta) > 0 {
		evt.Meta = make(map[string]any, len(meta))
		for key, value := range meta {
			evt.Meta[key] = value
		}
	}
	s.events = append(s.events, evt)
	if len(s.events) > s.opts.EventBufferSize {
		s.events = s.events[len(s.events)-s.opts.EventBufferSize:]
	}
	if s.opts.EventPersistenceEnabled && s.eventStore.enabled() {
		if err := s.eventStore.write(s.events); err == nil {
			s.lastEventPersistAt = s.nowFn().UTC()
		}
	}
}

func durationMillisCeil(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms <= 0 {
		return 1
	}
	return ms
}
