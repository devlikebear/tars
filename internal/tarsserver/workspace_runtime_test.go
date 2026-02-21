package tarsserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
	"github.com/devlikebear/tarsncase/internal/heartbeat"
	"github.com/devlikebear/tarsncase/internal/memory"
	"github.com/devlikebear/tarsncase/internal/serverauth"
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
