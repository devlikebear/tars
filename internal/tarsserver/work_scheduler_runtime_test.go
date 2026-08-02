package tarsserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/workscheduler"
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
	cfg.WorkLedger.SchedulerExecutionDataDir = filepath.Join(t.TempDir(), "execution-data")
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

func TestManagedWorkExecutionPlaneRunsLifecycleAndPreservesSource(t *testing.T) {
	workspaceDir := t.TempDir()
	if output, err := exec.Command("git", "-C", workspaceDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "source.txt"), []byte("source\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if output, err := exec.Command("git", "-C", workspaceDir, "add", "source.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", workspaceDir, "-c", "user.name=TARS Test", "-c", "user.email=tars@example.com", "commit", "-m", "initial").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}

	executionRoots := make(chan string, 1)
	promptExecutor, err := agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name: "worker", PolicyMode: "allowlist", ToolsAllow: []string{"read"},
		RunPrompt: func(ctx context.Context, _ string, _ string, _ []string, _ string, _ *agentruntime.ProviderOverride) (string, error) {
			root := agentruntime.ExecutionRootFromContext(ctx)
			executionRoots <- root
			if err := os.WriteFile(filepath.Join(root, "result.txt"), []byte("managed result\n"), 0o600); err != nil {
				return "", err
			}
			return "completed", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: true, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(t.TempDir()),
		Executors: []agentruntime.AgentExecutor{promptExecutor}, DefaultAgent: "worker",
	})
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open scheduler ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	dataDir := filepath.Join(t.TempDir(), "execution-data")
	cfg := config.Default()
	cfg.WorkspaceDir = workspaceDir
	cfg.WorkLedger.SchedulerEnabled = true
	cfg.WorkLedger.SchedulerExecutionEnvironment = "managed-worktree"
	cfg.WorkLedger.SchedulerExecutionDataDir = dataDir
	cfg.WorkLedger.SchedulerArtifactPaths = []string{"result.txt"}
	cfg.WorkLedger.SchedulerPollMilliseconds = 5
	cfg.WorkLedger.SchedulerHeartbeatSeconds = 1
	cfg.WorkLedger.SchedulerLeaseSeconds = 5
	scheduler, err := buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop())
	if err != nil {
		t.Fatalf("build managed scheduler: %v", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		scheduler.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("managed scheduler did not stop")
		}
	})

	contract := json.RawMessage(`{"schema_version":1,"flow_id":"flow-managed","agent":"worker","depth":0,"flow":{"flow_id":"flow-managed","agent":"worker","steps":[{"id":"stage","mode":"sequential","tasks":[{"id":"task","prompt":"write result"}]}]}}`)
	work, err := scheduler.Submit(context.Background(), workscheduler.SubmitInput{
		WorkspaceID: "default", IdempotencyKey: "managed-lifecycle", Kind: "subagent_flow",
		Source: "test", SourceID: "flow-managed", Title: "managed lifecycle", Objective: "verify managed lifecycle",
		ContractJSON: contract, Adapter: "agentruntime", ActorID: "test",
		Steps: []workscheduler.StepSpec{{Key: "task", Title: "task", Position: 1}},
	})
	if err != nil {
		t.Fatalf("submit managed work: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	projection, err := scheduler.Wait(waitCtx, work.ID)
	if err != nil {
		t.Fatalf("wait managed work: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || len(projection.Artifacts) != 1 || projection.Artifacts[0].Name != "result.txt" {
		t.Fatalf("managed projection = %#v", projection)
	}
	executionRoot := <-executionRoots
	if executionRoot == "" || executionRoot == workspaceDir {
		t.Fatalf("managed execution root = %q", executionRoot)
	}
	if _, err := os.Stat(executionRoot); !os.IsNotExist(err) {
		t.Fatalf("managed execution root was not cleaned up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "result.txt")); !os.IsNotExist(err) {
		t.Fatalf("managed worker mutated source workspace: %v", err)
	}
	artifactPath, err := filepathFromArtifactURI(projection.Artifacts[0].URI)
	if err != nil {
		t.Fatalf("artifact URI: %v", err)
	}
	if raw, err := os.ReadFile(artifactPath); err != nil || string(raw) != "managed result\n" {
		t.Fatalf("collected artifact = %q, %v", raw, err)
	}
	wantEvents := map[workstore.EventType]bool{
		workstore.EventTypeExecutionEnvironmentProvisioned: false,
		workstore.EventTypeExecutionWorkerStarted:          false,
		workstore.EventTypeExecutionEnvironmentSynced:      false,
		workstore.EventTypeExecutionArtifactsCollected:     false,
		workstore.EventTypeExecutionEnvironmentDestroyed:   false,
	}
	for _, event := range projection.Events {
		if _, ok := wantEvents[event.Type]; ok {
			wantEvents[event.Type] = true
		}
	}
	for eventType, found := range wantEvents {
		if !found {
			t.Fatalf("managed lifecycle missing event %s", eventType)
		}
	}
}

func filepathFromArtifactURI(raw string) (string, error) {
	const prefix = "file://"
	if !strings.HasPrefix(raw, prefix) {
		return "", os.ErrInvalid
	}
	return strings.TrimPrefix(raw, prefix), nil
}

func TestBuildNativeWorkExecutionPlaneSelectsHonestEnvironment(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	if output, err := exec.Command("git", "-C", workspaceDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", workspaceDir, "-c", "user.name=TARS Test", "-c", "user.email=tars@example.com", "commit", "--allow-empty", "-m", "initial").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}
	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open scheduler ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: true, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(t.TempDir()),
	})
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	cfg := config.Default()
	cfg.WorkspaceDir = workspaceDir
	cfg.WorkLedger.SchedulerExecutionDataDir = filepath.Join(t.TempDir(), "execution-data")
	local, err := buildNativeWorkExecutionPlane(cfg, ledger, runtime)
	if err != nil {
		t.Fatalf("build local execution plane: %v", err)
	}
	if descriptor := local.Descriptor(); descriptor.Adapter != "agentruntime" || descriptor.Provider != "local" || descriptor.Worker != "native-agentruntime" || !descriptor.Executor.Resume || descriptor.Environment.FilesystemIsolation {
		t.Fatalf("local execution descriptor = %#v", descriptor)
	}

	cfg.WorkLedger.SchedulerExecutionEnvironment = "managed-worktree"
	managed, err := buildNativeWorkExecutionPlane(cfg, ledger, runtime)
	if err != nil {
		t.Fatalf("build managed execution plane: %v", err)
	}
	if descriptor := managed.Descriptor(); descriptor.Provider != "managed-worktree" || !descriptor.Environment.FilesystemIsolation || !descriptor.Environment.Cleanup {
		t.Fatalf("managed execution descriptor = %#v", descriptor)
	}

	cfg.WorkLedger.SchedulerExecutionEnvironment = "container"
	if _, err := buildNativeWorkExecutionPlane(cfg, ledger, runtime); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported native container error = %v", err)
	}
	cfg.WorkLedger.SchedulerExecutionEnvironment = "local"
	cfg.WorkLedger.SchedulerExecutionDataDir = filepath.Join(workspaceDir, ".tars", "execution-plane")
	if _, err := buildNativeWorkExecutionPlane(cfg, ledger, runtime); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("overlapping execution data error = %v", err)
	}
}
