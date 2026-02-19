package main

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

func TestWorkspaceCronManager_TickRunsDueJobsAcrossWorkspaces(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	defaultStore := cron.NewStoreWithOptions(root, cron.StoreOptions{RunHistoryLimit: 20})
	defaultJob, err := defaultStore.CreateWithOptions(cron.CreateInput{
		Name:      "default-job",
		Prompt:    "run-default",
		Schedule:  "every:1s",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create default job: %v", err)
	}

	resolver := newWorkspaceCronStoreResolver(root, 20, defaultStore)
	tenantStore, err := resolver.Resolve("team-a")
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	tenantJob, err := tenantStore.CreateWithOptions(cron.CreateInput{
		Name:      "tenant-job",
		Prompt:    "run-tenant",
		Schedule:  "every:1s",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
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

	if runCounts[defaultWorkspaceID] != 1 {
		t.Fatalf("expected default workspace run count 1, got %+v", runCounts)
	}
	if runCounts["team-a"] != 1 {
		t.Fatalf("expected team-a workspace run count 1, got %+v", runCounts)
	}

	updatedDefault, err := defaultStore.Get(defaultJob.ID)
	if err != nil {
		t.Fatalf("get default job: %v", err)
	}
	if updatedDefault.LastRunAt == nil {
		t.Fatalf("expected default job last_run_at to be recorded")
	}
	updatedTenant, err := tenantStore.Get(tenantJob.ID)
	if err != nil {
		t.Fatalf("get tenant job: %v", err)
	}
	if updatedTenant.LastRunAt == nil {
		t.Fatalf("expected tenant job last_run_at to be recorded")
	}
}

func TestWorkspaceHeartbeatRunner_UsesWorkspaceFromContext(t *testing.T) {
	root := t.TempDir()
	if err := memory.EnsureWorkspace(root); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	tenantDir := resolveWorkspaceDir(root, "team-a")
	if err := memory.EnsureWorkspace(tenantDir); err != nil {
		t.Fatalf("ensure tenant workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "HEARTBEAT.md"), []byte("default heartbeat"), 0o644); err != nil {
		t.Fatalf("write default heartbeat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tenantDir, "HEARTBEAT.md"), []byte("tenant heartbeat"), 0o644); err != nil {
		t.Fatalf("write tenant heartbeat: %v", err)
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
		t.Fatalf("run heartbeat in team-a: %v", err)
	}

	tenantLogPath := filepath.Join(tenantDir, "memory", "2026-02-20.md")
	tenantLog, err := os.ReadFile(tenantLogPath)
	if err != nil {
		t.Fatalf("read tenant heartbeat log: %v", err)
	}
	if !strings.Contains(string(tenantLog), "heartbeat tick") {
		t.Fatalf("expected tenant heartbeat tick log, got %q", string(tenantLog))
	}

	defaultLogPath := filepath.Join(root, "memory", "2026-02-20.md")
	if _, err := os.Stat(defaultLogPath); err == nil {
		defaultLog, readErr := os.ReadFile(defaultLogPath)
		if readErr != nil {
			t.Fatalf("read default heartbeat log: %v", readErr)
		}
		if strings.Contains(string(defaultLog), "heartbeat tick") {
			t.Fatalf("did not expect default workspace heartbeat tick, got %q", string(defaultLog))
		}
	}

	tenantStatus := state.snapshot("team-a", true, "", "", false)
	if strings.TrimSpace(tenantStatus.LastRunAt) == "" {
		t.Fatalf("expected tenant heartbeat state to record last run")
	}
	defaultStatus := state.snapshot(defaultWorkspaceID, true, "", "", false)
	if strings.TrimSpace(defaultStatus.LastRunAt) != "" {
		t.Fatalf("expected default heartbeat state to remain empty, got %+v", defaultStatus)
	}
}
