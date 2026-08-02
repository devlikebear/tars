package tarsserver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestBuildWorkSchedulerHonorsRollbackAndValidatesLease(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	ledger, err := workstore.Open(context.Background(), filepath.Join(workspaceDir, "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open scheduler ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: true, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(workspaceDir),
	})
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	cfg := config.Default()
	cfg.WorkLedger.SchedulerEnabled = false
	scheduler, err := buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop())
	if err != nil || scheduler != nil {
		t.Fatalf("disabled scheduler=%v err=%v", scheduler, err)
	}

	cfg.WorkLedger.SchedulerEnabled = true
	cfg.WorkLedger.Enabled = false
	if scheduler, err = buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop()); err == nil || scheduler != nil {
		t.Fatalf("scheduler without ledger scheduler=%v err=%v", scheduler, err)
	}

	cfg.WorkLedger.Enabled = true
	disabledRuntime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: false, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(workspaceDir),
	})
	t.Cleanup(func() { _ = disabledRuntime.Close(context.Background()) })
	if scheduler, err = buildWorkSchedulerIfEnabled(cfg, ledger, disabledRuntime, zerolog.Nop()); err == nil || scheduler != nil {
		t.Fatalf("scheduler without agent runtime scheduler=%v err=%v", scheduler, err)
	}

	cfg.WorkLedger.SchedulerLeaseSeconds = 10
	cfg.WorkLedger.SchedulerHeartbeatSeconds = 10
	if scheduler, err = buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop()); err == nil || scheduler != nil {
		t.Fatalf("invalid lease scheduler=%v err=%v", scheduler, err)
	}

	cfg.WorkLedger.SchedulerHeartbeatSeconds = 3
	scheduler, err = buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop())
	if err != nil || scheduler == nil {
		t.Fatalf("enabled scheduler=%v err=%v", scheduler, err)
	}
	scheduler.Close()
}
