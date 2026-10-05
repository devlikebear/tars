package tarsserver

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
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
	cfg.WorkspaceDir = workspaceDir
	cfg.WorkLedger.SchedulerEnabled = false
	scheduler, err := buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop())
	if err != nil || scheduler != nil {
		t.Fatalf("disabled scheduler=%v err=%v", scheduler, err)
	}

	cfg.WorkLedger.SchedulerEnabled = true
	cfg.WorkLedger.Enabled = false
	if scheduler, err = buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop()); err != nil || scheduler != nil {
		t.Fatalf("a server without the ledger should start without the scheduler, scheduler=%v err=%v", scheduler, err)
	}

	cfg.WorkLedger.Enabled = true
	disabledRuntime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: false, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(workspaceDir),
	})
	t.Cleanup(func() { _ = disabledRuntime.Close(context.Background()) })
	if scheduler, err = buildWorkSchedulerIfEnabled(cfg, ledger, disabledRuntime, zerolog.Nop()); err != nil || scheduler != nil {
		t.Fatalf("a server without the agent runtime should start without the scheduler, scheduler=%v err=%v", scheduler, err)
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

func TestRunUsageLookupReadsTheRunsCallsFromTheUsageLog(t *testing.T) {
	if runUsageLookup() != nil || runUsageLookup(nil) != nil {
		t.Fatal("expected no lookup without a tracker")
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tracker, err := usage.NewTracker(t.TempDir(), usage.TrackerOptions{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	for _, entry := range []usage.Entry{
		{Timestamp: now, Provider: "p", Model: "m", InputTokens: 4000, OutputTokens: 99, EstimatedCostUSD: 0.2, Source: "agent_run", RunID: "run_7:explorer"},
		{Timestamp: now, Provider: "p", Model: "m", InputTokens: 1, OutputTokens: 1, EstimatedCostUSD: 9, Source: "agent_run", RunID: "run_70:explorer"},
	} {
		if err := tracker.Record(entry); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	lookup := runUsageLookup(tracker)
	if tokens, cost := lookup(agentruntime.Run{ID: "run_7"}); tokens != 4099 || cost != 0.2 {
		t.Fatalf("run_7 usage = %d tokens, %v USD", tokens, cost)
	}
	if tokens, cost := lookup(agentruntime.Run{}); tokens != 0 || cost != 0 {
		t.Fatalf("a run without an id has no usage, got %d / %v", tokens, cost)
	}
}
