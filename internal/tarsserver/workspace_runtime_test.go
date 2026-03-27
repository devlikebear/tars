package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/cron"
	"github.com/devlikebear/tars/internal/heartbeat"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestWorkspaceCronManager_TickRunsDueJobsSingleWorkspaceOnly(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	defaultStore := cron.NewStoreWithOptions(root, cron.StoreOptions{RunHistoryLimit: 20})
	if _, err := defaultStore.CreateWithOptions(cron.CreateInput{
		Name:      "default-job",
		Prompt:    "run-default",
		Schedule:  "every:1s",
		Enabled:   true,
		HasEnable: true,
	}); err != nil {
		t.Fatalf("create default job: %v", err)
	}

	resolver := newWorkspaceCronStoreResolver(root, 20, defaultStore)
	tenantStore, err := resolver.Resolve("team-a")
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	if _, err := tenantStore.CreateWithOptions(cron.CreateInput{
		Name:      "tenant-job",
		Prompt:    "run-tenant",
		Schedule:  "every:1s",
		Enabled:   true,
		HasEnable: true,
	}); err != nil {
		t.Fatalf("create tenant job: %v", err)
	}

	var mu sync.Mutex
	runCounts := map[string]int{}
	manager := newWorkspaceCronManager(
		resolver,
		func(ctx context.Context, _ cron.Job) (string, error) {
			workspaceID := normalizeWorkspaceID(serverauth.WorkspaceIDFromContext(ctx))
			mu.Lock()
			runCounts[workspaceID]++
			mu.Unlock()
			return "ok", nil
		},
		30*time.Second,
		func() time.Time { return time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC) },
		zerolog.Nop(),
	)
	if err := manager.Tick(context.Background()); err != nil {
		t.Fatalf("tick manager: %v", err)
	}

	if runCounts[defaultWorkspaceID] != 2 {
		t.Fatalf("expected default workspace run count 2 in single workspace mode, got %+v", runCounts)
	}
	if _, ok := runCounts["team-a"]; ok {
		t.Fatalf("did not expect team-a execution in single workspace mode, got %+v", runCounts)
	}
}

func TestWorkspaceHeartbeatRunner_AlwaysWritesDefaultWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("default heartbeat"), 0o644); err != nil {
		t.Fatalf("write default heartbeat: %v", err)
	}

	state := newHeartbeatWorkspaceState()
	now := func() time.Time { return time.Date(2026, 2, 20, 11, 0, 0, 0, time.UTC) }
	runner := newWorkspaceHeartbeatRunnerWithNotify(
		root,
		now,
		func(_ context.Context, _ string) (string, error) { return "heartbeat-ok", nil },
		func(_ string) heartbeat.Policy { return heartbeat.Policy{} },
		state,
		nil,
		nil,
	)

	ctx := serverauth.WithWorkspaceID(context.Background(), "team-a")
	if _, err := runner(ctx); err != nil {
		t.Fatalf("run heartbeat with non-default workspace context: %v", err)
	}

	defaultLogPath := filepath.Join(root, "memory", "2026-02-20.md")
	defaultLog, err := os.ReadFile(defaultLogPath)
	if err != nil {
		t.Fatalf("read default heartbeat log: %v", err)
	}
	if !strings.Contains(string(defaultLog), "heartbeat tick") {
		t.Fatalf("expected default heartbeat tick log, got %q", string(defaultLog))
	}

	defaultStatus := state.snapshot(defaultWorkspaceID, true, "", "", false)
	if strings.TrimSpace(defaultStatus.LastRunAt) == "" {
		t.Fatalf("expected default heartbeat state to record last run")
	}
	tenantStatus := state.snapshot("team-a", true, "", "", false)
	if strings.TrimSpace(tenantStatus.LastRunAt) != "" {
		t.Fatalf("did not expect team-a heartbeat state, got %+v", tenantStatus)
	}
}

func TestWorkspaceHeartbeatRunner_EnsuresMissingProjectAutopilotLoops(t *testing.T) {
	var ensured atomic.Int64
	runner := newWorkspaceHeartbeatRunnerWithNotify(
		t.TempDir(),
		func() time.Time { return time.Date(2026, 3, 14, 21, 0, 0, 0, time.UTC) },
		func(_ context.Context, _ string) (string, error) { return "heartbeat-ok", nil },
		func(_ string) heartbeat.Policy {
			return heartbeat.Policy{ShouldRun: func(context.Context, time.Time) (bool, string) { return false, "skip llm" }}
		},
		newHeartbeatWorkspaceState(),
		nil,
		func(context.Context) error {
			ensured.Add(1)
			return nil
		},
	)

	if _, err := runner(context.Background()); err != nil {
		t.Fatalf("run heartbeat: %v", err)
	}
	if ensured.Load() != 1 {
		t.Fatalf("expected heartbeat to ensure missing project autopilot loops once, got %d", ensured.Load())
	}
}

func TestStartBackgrounds_RestoresProjectAutopilotViaClosure(t *testing.T) {
	restored := make(chan struct{}, 1)
	runtime := &serveAPIRuntime{
		restoreProjectAutopilot: func() error {
			restored <- struct{}{}
			return nil
		},
	}

	if err := startBackgrounds(context.Background(), runtime, zerolog.Nop()); err != nil {
		t.Fatalf("start backgrounds: %v", err)
	}

	select {
	case <-restored:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected startup to restore project autopilot via closure")
	}
}

func TestSerializedSupervisorRunner_SerializesConcurrentRuns(t *testing.T) {
	var active atomic.Int64
	var maxActive atomic.Int64

	runner := newSerializedSupervisorRunner(serializedSupervisorOptions[int]{
		nowFn:   func() time.Time { return time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC) },
		timeout: 200 * time.Millisecond,
		run: func(ctx context.Context, _ time.Time) (int, error) {
			current := active.Add(1)
			for {
				seen := maxActive.Load()
				if current <= seen || maxActive.CompareAndSwap(seen, current) {
					break
				}
			}
			select {
			case <-ctx.Done():
				active.Add(-1)
				return 0, ctx.Err()
			case <-time.After(25 * time.Millisecond):
			}
			active.Add(-1)
			return 1, nil
		},
	})

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := runner(context.Background()); err != nil {
				t.Errorf("runner error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := maxActive.Load(); got != 1 {
		t.Fatalf("expected serialized supervisor execution, got max active %d", got)
	}
}

func TestWorkspaceWatchdogRunner_FlagsExplicitSessionTargetAndContaminatedTranscript(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	sessionStore := session.NewStore(root)
	legacySession, err := sessionStore.Create("legacy worker")
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}
	if err := session.AppendMessage(sessionStore.TranscriptPath(legacySession.ID), session.Message{
		Role:      "assistant",
		Content:   `assistant to=functions.exec {"command":"echo hi"} Need maybe call read_file first`,
		Timestamp: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("append contaminated message: %v", err)
	}

	cronStore := cron.NewStoreWithOptions(root, cron.StoreOptions{RunHistoryLimit: 20})
	if _, err := cronStore.CreateWithOptions(cron.CreateInput{
		Name:          "novelist",
		Prompt:        "continue writing",
		Schedule:      "every:1m",
		Enabled:       true,
		HasEnable:     true,
		ProjectID:     "project-1",
		SessionTarget: legacySession.ID,
	}); err != nil {
		t.Fatalf("create cron job: %v", err)
	}

	state := newWatchdogWorkspaceState()
	var events []notificationEvent
	runner := newWorkspaceWatchdogRunnerWithNotify(
		root,
		newWorkspaceCronStoreResolver(root, 20, cronStore),
		func() time.Time { return time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC) },
		state,
		func(_ context.Context, evt notificationEvent) {
			events = append(events, evt)
		},
	)

	result, err := runner(context.Background())
	if err != nil {
		t.Fatalf("run watchdog: %v", err)
	}
	if result.Healthy {
		t.Fatalf("expected unhealthy watchdog result, got %+v", result)
	}
	if !containsWatchdogKind(result.Findings, "explicit_session_target") {
		t.Fatalf("expected explicit_session_target finding, got %+v", result.Findings)
	}
	if !containsWatchdogKind(result.Findings, "contaminated_transcript") {
		t.Fatalf("expected contaminated_transcript finding, got %+v", result.Findings)
	}
	if len(events) != 1 || events[0].Category != "watchdog" || events[0].Severity != "warn" {
		t.Fatalf("expected watchdog warn notification, got %+v", events)
	}
	status := state.snapshot(defaultWorkspaceID)
	if status.Healthy {
		t.Fatalf("expected recorded watchdog state to be unhealthy, got %+v", status)
	}
}

func TestWorkspaceWatchdogRunner_FlagsStaleProjectProgress(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	projectDir := filepath.Join(root, "projects", "project-1")
	if err := os.MkdirAll(filepath.Join(projectDir, "cron_runs"), 0o755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}
	statePath := filepath.Join(projectDir, "STATE.md")
	if err := os.WriteFile(statePath, []byte("phase: drafting\n"), 0o644); err != nil {
		t.Fatalf("write state doc: %v", err)
	}
	docTime := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(statePath, docTime, docTime); err != nil {
		t.Fatalf("touch state doc: %v", err)
	}
	artifactPath := filepath.Join(projectDir, "cron_runs", "20260308T001500Z_job.md")
	if err := os.WriteFile(artifactPath, []byte("ran"), 0o644); err != nil {
		t.Fatalf("write cron artifact: %v", err)
	}
	artifactTime := time.Date(2026, 3, 8, 0, 15, 0, 0, time.UTC)
	if err := os.Chtimes(artifactPath, artifactTime, artifactTime); err != nil {
		t.Fatalf("touch cron artifact: %v", err)
	}

	cronStore := cron.NewStoreWithOptions(root, cron.StoreOptions{RunHistoryLimit: 20})
	job, err := cronStore.CreateWithOptions(cron.CreateInput{
		Name:      "novelist",
		Prompt:    "continue writing",
		Schedule:  "every:1m",
		Enabled:   true,
		HasEnable: true,
		ProjectID: "project-1",
	})
	if err != nil {
		t.Fatalf("create cron job: %v", err)
	}
	ranAt := artifactTime
	if _, err := cronStore.MarkRunResult(job.ID, ranAt, "no progress", nil); err != nil {
		t.Fatalf("mark run result: %v", err)
	}

	runner := newWorkspaceWatchdogRunnerWithNotify(
		root,
		newWorkspaceCronStoreResolver(root, 20, cronStore),
		func() time.Time { return time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC) },
		newWatchdogWorkspaceState(),
		nil,
	)

	result, err := runner(context.Background())
	if err != nil {
		t.Fatalf("run watchdog: %v", err)
	}
	if !containsWatchdogKind(result.Findings, "stale_project_progress") {
		t.Fatalf("expected stale_project_progress finding, got %+v", result.Findings)
	}
}

func containsWatchdogKind(findings []watchdogFinding, kind string) bool {
	for _, finding := range findings {
		if strings.EqualFold(strings.TrimSpace(finding.Kind), strings.TrimSpace(kind)) {
			return true
		}
	}
	return false
}
