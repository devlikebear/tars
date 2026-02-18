package sentinel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SENTINEL_HELPER") != "1" {
		return
	}
	args := os.Args
	idx := slices.Index(args, "--")
	if idx < 0 || idx+1 >= len(args) {
		os.Exit(2)
	}
	mode := strings.TrimSpace(args[idx+1])
	switch mode {
	case "fail":
		os.Exit(1)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		os.Exit(3)
	}
}

func helperProcess(mode string) (string, []string, map[string]string) {
	return os.Args[0], []string{"-test.run=TestHelperProcess", "--", mode}, map[string]string{
		"GO_WANT_SENTINEL_HELPER": "1",
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met: %s", msg)
}

func closeSupervisor(t *testing.T, s *Supervisor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("close supervisor: %v", err)
	}
}

func TestSupervisor_EntersCooldownAfterMaxAttempts(t *testing.T) {
	cmd, args, env := helperProcess("fail")
	s := NewSupervisor(Options{
		TargetCommand:      cmd,
		TargetArgs:         args,
		TargetEnv:          env,
		Autostart:          true,
		ProbeInterval:      50 * time.Millisecond,
		ProbeTimeout:       20 * time.Millisecond,
		ProbeFailThreshold: 3,
		RestartMaxAttempts: 3,
		RestartBackoff:     10 * time.Millisecond,
		RestartBackoffMax:  20 * time.Millisecond,
		RestartCooldown:    150 * time.Millisecond,
		EventBufferSize:    64,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	defer closeSupervisor(t, s)

	waitForCondition(t, 3*time.Second, func() bool {
		st := s.Status()
		return st.SupervisionState == StateCooldown
	}, "supervisor enters cooldown")

	st := s.Status()
	if st.RestartMaxAttempts != 3 {
		t.Fatalf("expected restart_max_attempts=3, got %d", st.RestartMaxAttempts)
	}
	if strings.TrimSpace(st.CooldownUntil) == "" {
		t.Fatalf("expected cooldown_until to be set, status=%+v", st)
	}
	events := s.Events(128)
	foundCooldown := false
	for _, evt := range events {
		if evt.Type == EventCooldownEnter {
			foundCooldown = true
			break
		}
	}
	if !foundCooldown {
		t.Fatalf("expected cooldown_enter event, got %+v", events)
	}
}

func TestSupervisor_HealthFailureTriggersRestart(t *testing.T) {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer probe.Close()

	cmd, args, env := helperProcess("sleep")
	s := NewSupervisor(Options{
		TargetCommand:      cmd,
		TargetArgs:         args,
		TargetEnv:          env,
		Autostart:          true,
		ProbeURL:           probe.URL,
		ProbeInterval:      25 * time.Millisecond,
		ProbeTimeout:       200 * time.Millisecond,
		ProbeFailThreshold: 2,
		RestartMaxAttempts: 3,
		RestartBackoff:     10 * time.Millisecond,
		RestartBackoffMax:  20 * time.Millisecond,
		RestartCooldown:    300 * time.Millisecond,
		EventBufferSize:    128,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	defer closeSupervisor(t, s)

	waitForCondition(t, 2*time.Second, func() bool {
		return s.Status().TargetPID > 0
	}, "target process started")
	initialPID := s.Status().TargetPID

	waitForCondition(t, 4*time.Second, func() bool {
		st := s.Status()
		return strings.TrimSpace(st.LastRestartAt) != "" && st.TargetPID != 0 && st.TargetPID != initialPID
	}, "health failure triggered restart")

	events := s.Events(200)
	foundHealthFail := false
	foundRestart := false
	for _, evt := range events {
		if evt.Type == EventHealthFail {
			foundHealthFail = true
		}
		if evt.Type == EventRestart {
			foundRestart = true
		}
	}
	if !foundHealthFail {
		t.Fatalf("expected health_fail event, events=%+v", events)
	}
	if !foundRestart {
		t.Fatalf("expected restart event, events=%+v", events)
	}
}

func TestSupervisor_PauseAndResume(t *testing.T) {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer probe.Close()

	cmd, args, env := helperProcess("sleep")
	s := NewSupervisor(Options{
		TargetCommand:      cmd,
		TargetArgs:         args,
		TargetEnv:          env,
		Autostart:          true,
		ProbeURL:           probe.URL,
		ProbeInterval:      30 * time.Millisecond,
		ProbeTimeout:       200 * time.Millisecond,
		ProbeFailThreshold: 3,
		RestartMaxAttempts: 3,
		RestartBackoff:     10 * time.Millisecond,
		RestartBackoffMax:  20 * time.Millisecond,
		RestartCooldown:    200 * time.Millisecond,
		EventBufferSize:    64,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	defer closeSupervisor(t, s)

	waitForCondition(t, 2*time.Second, func() bool {
		st := s.Status()
		return st.TargetPID > 0 && st.SupervisionState == StateRunning
	}, "target running before pause")

	paused := s.Pause()
	if paused.SupervisionState != StatePaused {
		t.Fatalf("expected paused state, got %+v", paused)
	}
	if paused.TargetPID == 0 {
		t.Fatalf("expected target pid while paused, got %+v", paused)
	}

	resumed := s.Resume()
	if resumed.SupervisionState != StateRunning && resumed.SupervisionState != StateStarting {
		t.Fatalf("unexpected resume status: %+v", resumed)
	}
	waitForCondition(t, 2*time.Second, func() bool {
		st := s.Status()
		return st.SupervisionState == StateRunning && st.TargetPID > 0
	}, "target running after resume")
}

func TestSupervisor_StartupGraceDefersProbeFailureRestart(t *testing.T) {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(15 * time.Millisecond)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer probe.Close()

	cmd, args, env := helperProcess("sleep")
	s := NewSupervisor(Options{
		TargetCommand:      cmd,
		TargetArgs:         args,
		TargetEnv:          env,
		Autostart:          true,
		ProbeURL:           probe.URL,
		ProbeInterval:      30 * time.Millisecond,
		ProbeTimeout:       300 * time.Millisecond,
		ProbeFailThreshold: 2,
		ProbeStartGrace:    250 * time.Millisecond,
		RestartMaxAttempts: 3,
		RestartBackoff:     10 * time.Millisecond,
		RestartBackoffMax:  20 * time.Millisecond,
		RestartCooldown:    300 * time.Millisecond,
		EventBufferSize:    128,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	defer closeSupervisor(t, s)

	waitForCondition(t, 2*time.Second, func() bool {
		return s.Status().TargetPID > 0
	}, "target process started")
	initialPID := s.Status().TargetPID

	time.Sleep(120 * time.Millisecond)
	duringGrace := s.Status()
	if duringGrace.LastRestartAt != "" {
		t.Fatalf("expected no restart during grace window, got %+v", duringGrace)
	}
	if strings.TrimSpace(duringGrace.StartGraceUntil) == "" {
		t.Fatalf("expected start_grace_until in status, got %+v", duringGrace)
	}

	waitForCondition(t, 4*time.Second, func() bool {
		st := s.Status()
		return strings.TrimSpace(st.LastRestartAt) != "" && st.TargetPID != 0 && st.TargetPID != initialPID
	}, "probe failure restart after grace window")
	waitForCondition(t, 2*time.Second, func() bool {
		return s.Status().LastProbeDurationMS > 0
	}, "probe duration telemetry populated")

	after := s.Status()
	if after.LastProbeDurationMS <= 0 {
		t.Fatalf("expected probe duration telemetry, got %+v", after)
	}
}

func TestSupervisor_EventPersistenceRestore(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	s1 := NewSupervisor(Options{
		Autostart:               false,
		EventBufferSize:         32,
		EventPersistenceEnabled: true,
		EventStorePath:          eventsPath,
		EventStoreMaxRecords:    100,
		ProbeInterval:           30 * time.Millisecond,
		ProbeTimeout:            50 * time.Millisecond,
		ProbeFailThreshold:      2,
		RestartMaxAttempts:      2,
		RestartBackoff:          10 * time.Millisecond,
		RestartBackoffMax:       20 * time.Millisecond,
		RestartCooldown:         100 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s1.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	_ = s1.Pause()
	_ = s1.Resume()
	closeSupervisor(t, s1)

	s2 := NewSupervisor(Options{
		Autostart:               false,
		EventBufferSize:         32,
		EventPersistenceEnabled: true,
		EventStorePath:          eventsPath,
		EventStoreMaxRecords:    100,
		ProbeInterval:           30 * time.Millisecond,
		ProbeTimeout:            50 * time.Millisecond,
		ProbeFailThreshold:      2,
		RestartMaxAttempts:      2,
		RestartBackoff:          10 * time.Millisecond,
		RestartBackoffMax:       20 * time.Millisecond,
		RestartCooldown:         100 * time.Millisecond,
	})
	status := s2.Status()
	if !status.EventPersistenceEnabled {
		t.Fatalf("expected event persistence enabled in status")
	}
	if status.EventsRestored <= 0 {
		t.Fatalf("expected events restored > 0, got %+v", status)
	}
	if strings.TrimSpace(status.LastEventRestoreAt) == "" {
		t.Fatalf("expected last_event_restore_at, got %+v", status)
	}
	if len(s2.Events(10)) == 0 {
		t.Fatalf("expected restored events in ring buffer")
	}
}
