package tarsserver

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/a2a"
	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/executionplane"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/workerprotocol"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestBuildWorkSchedulerConnectsConfiguredContainerToSharedRemoteLifecycle(t *testing.T) {
	t.Parallel()

	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "task.txt"), []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: true, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(t.TempDir()),
	})
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 71
	privateKey := ed25519.NewKeyFromSeed(seed)
	workerRoot := t.TempDir()
	workerConfigPath := filepath.Join(t.TempDir(), "worker.json")
	writePrivateJSON(t, workerConfigPath, workerprotocol.WorkerServiceConfig{
		SchemaVersion: 1, WorkerID: "container-worker", RootDir: workerRoot,
		StatePath: filepath.Join(workerRoot, "state.json"),
		PublicKey: base64.RawStdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)), MaxTokenTTLSeconds: 300,
		WireLimits: workerprotocol.WireLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20},
		Container: workerprotocol.ContainerWorkerConfig{
			RuntimePath: "/usr/bin/docker", Image: "worker@sha256:" + strings.Repeat("f", 64),
			Command: []string{"/usr/local/bin/tars-task-harness"}, CPUs: "1", PIDsLimit: 64,
		},
	})
	gatewayConfigPath := filepath.Join(t.TempDir(), "gateway.json")
	writePrivateJSON(t, gatewayConfigPath, workerprotocol.SchedulerGatewayConfig{
		SchemaVersion: 1, Adapter: "remote-container", WorkerID: "container-worker",
		Transport:       workerprotocol.SchedulerTransportInProcess,
		PrivateKey:      base64.RawStdEncoding.EncodeToString(privateKey),
		LeaseTTLSeconds: 120, TokenTTLSeconds: 60, SyncMode: workerprotocol.SyncModeDirectory,
		Policy:    workerprotocol.DefaultExecutionPolicy(),
		InProcess: workerprotocol.SchedulerInProcessConfig{WorkerConfigPath: workerConfigPath},
	})
	containerResponse, err := json.Marshal(workerprotocol.ContainerTaskResponse{
		Succeeded: true, Output: json.RawMessage(`{"summary":"same lifecycle"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &remoteContainerTestRunner{result: workerprotocol.ProcessResult{Stdout: append(containerResponse, '\n')}}
	dataDir := filepath.Join(t.TempDir(), "execution-data")
	cfg := config.Default()
	cfg.WorkspaceDir = workspaceDir
	cfg.WorkLedger.SchedulerEnabled = true
	cfg.WorkLedger.SchedulerPollMilliseconds = 5
	cfg.WorkLedger.SchedulerHeartbeatSeconds = 1
	cfg.WorkLedger.SchedulerLeaseSeconds = 5
	cfg.WorkLedger.SchedulerExecutionDataDir = dataDir
	cfg.WorkLedger.SchedulerRemoteWorkersEnabled = true
	cfg.WorkLedger.SchedulerRemoteWorkersGatewayConfigPath = gatewayConfigPath
	controller, err := buildWorkerControllerIfEnabled(cfg, ledger)
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := buildWorkSchedulerWithRemote(cfg, ledger, runtime, controller, runner, zerolog.Nop())
	if err != nil {
		t.Fatalf("build scheduler with remote container: %v", err)
	}
	t.Cleanup(scheduler.Close)
	work, err := scheduler.Submit(context.Background(), workscheduler.SubmitInput{
		WorkspaceID: "default", IdempotencyKey: "remote-container-lifecycle", Kind: "test", Source: "test",
		SourceID: "remote-container", Title: "Remote container", Objective: "prove shared lifecycle",
		Adapter: "remote-container", ActorID: "test",
		Steps: []workscheduler.StepSpec{{Key: "run", Title: "Run", Description: "execute once", Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || claimed != 1 {
		t.Fatalf("run remote scheduler claimed=%d err=%v", claimed, err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projection, err := scheduler.Wait(waitCtx, work.ID)
	if err != nil || projection.Work.State != workstore.WorkStateDone || projection.Attempts[0].Status != workstore.AttemptStatusSucceeded {
		t.Fatalf("remote container projection=%+v err=%v", projection, err)
	}
	if runner.calls != 1 || runner.spec.InheritEnv {
		t.Fatalf("container runner calls=%d spec=%+v", runner.calls, runner.spec)
	}
	if len(controller.Snapshot().Placements) != 1 {
		t.Fatalf("remote placements=%+v", controller.Snapshot().Placements)
	}
	for _, placement := range controller.Snapshot().Placements {
		if placement.State != workerprotocol.PlacementStateDestroyed || placement.WorkID != work.ID {
			t.Fatalf("remote placement=%+v", placement)
		}
	}
	runEntries, err := os.ReadDir(filepath.Join(dataDir, "remote-workers", "scheduler", "runs"))
	if err != nil || len(runEntries) != 0 {
		t.Fatalf("remote recovery journal after ledger commit entries=%d err=%v", len(runEntries), err)
	}
	wantEvents := map[workstore.EventType]bool{
		workstore.EventTypeWorkerPlacementCreated:   false,
		workstore.EventTypeWorkerExecutionStarted:   false,
		workstore.EventTypeWorkerArtifactsCollected: false,
		workstore.EventTypeWorkerPlacementDestroyed: false,
	}
	for _, event := range projection.Events {
		if _, ok := wantEvents[event.Type]; ok {
			wantEvents[event.Type] = true
		}
	}
	for eventType, found := range wantEvents {
		if !found {
			t.Fatalf("remote lifecycle missing Work Ledger event %s", eventType)
		}
	}
}

type remoteContainerTestRunner struct {
	calls  int
	spec   workerprotocol.ProcessSpec
	result workerprotocol.ProcessResult
}

func (runner *remoteContainerTestRunner) Run(_ context.Context, spec workerprotocol.ProcessSpec) (workerprotocol.ProcessResult, error) {
	runner.calls++
	runner.spec = spec
	return runner.result, nil
}

func writePrivateJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

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

func TestBuildA2AWorkExecutorDiscoversV1AndKeepsTokenInGateway(t *testing.T) {
	const token = "gateway-only-a2a-token"
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case a2a.AgentCardPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "name":"external-reviewer",
  "description":"Reviews durable work",
  "supportedInterfaces":[{"url":"` + server.URL + `/a2a","protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}],
  "version":"2026.8.0",
  "capabilities":{},
  "securitySchemes":{"bearer":{"httpAuthSecurityScheme":{"scheme":"bearer"}}},
  "securityRequirements":[{"schemes":{"bearer":{"list":[]}}}],
  "defaultInputModes":["text/plain"],
  "defaultOutputModes":["text/plain"],
  "skills":[{"id":"review","name":"Review","description":"Review work","tags":["review"]}]
}`))
		case "/a2a/message:send":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Fatalf("missing gateway authorization: %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", a2a.MediaType)
			_, _ = w.Write([]byte(`{"task":{"id":"remote-task","contextId":"remote-context","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"artifactId":"report","parts":[{"text":"approved"}]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	cfg := config.Default()
	cfg.WorkLedger.SchedulerA2AEnabled = true
	cfg.WorkLedger.SchedulerA2ADiscoveryURL = server.URL
	cfg.WorkLedger.SchedulerA2ABearerToken = token
	cfg.WorkLedger.SchedulerA2AAllowedHosts = []string{"127.0.0.1"}
	cfg.WorkLedger.SchedulerA2AAllowPrivateHosts = true
	cfg.WorkLedger.SchedulerA2APollMilliseconds = 1
	cfg.WorkLedger.SchedulerA2AMaxPollSeconds = 1
	executor, err := buildA2AWorkExecutor(context.Background(), cfg, ledger, server.Client())
	if err != nil {
		t.Fatalf("build A2A executor: %v", err)
	}
	if executor == nil || executor.Adapter() != a2a.AdapterName {
		t.Fatalf("unexpected A2A executor: %#v", executor)
	}
	execution := createA2ARuntimeExecution(t, ledger)
	result, err := executor.Execute(context.Background(), execution)
	if err != nil || !result.Succeeded {
		t.Fatalf("execute A2A work: result=%#v err=%v", result, err)
	}
	projection, err := ledger.GetWorkProjection(context.Background(), execution.Work.WorkspaceID, execution.Work.ID)
	if err != nil {
		t.Fatalf("get A2A work projection: %v", err)
	}
	raw, err := json.Marshal(projection.Events)
	if err != nil {
		t.Fatalf("marshal A2A events: %v", err)
	}
	if strings.Contains(string(raw), token) {
		t.Fatalf("work ledger leaked A2A token: %s", raw)
	}
}

func TestBuildA2AWorkExecutorRejectsMissingCredentialAndLeavesDisabledInstallUntouched(t *testing.T) {
	cfg := config.Default()
	if executor, err := buildA2AWorkExecutor(context.Background(), cfg, nil, nil); err != nil || executor != nil {
		t.Fatalf("disabled A2A executor=%#v err=%v", executor, err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "name":"secured","description":"secured agent",
  "supportedInterfaces":[{"url":"https://127.0.0.1/a2a","protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}],
  "version":"1.0.0","capabilities":{},
  "securityRequirements":[{"schemes":{"bearer":{"list":[]}}}],
  "defaultInputModes":["text/plain"],"defaultOutputModes":["text/plain"],
  "skills":[{"id":"run","name":"Run","description":"Run work","tags":["work"]}]
}`))
	}))
	t.Cleanup(server.Close)
	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	cfg.WorkLedger.SchedulerA2AEnabled = true
	cfg.WorkLedger.SchedulerA2ADiscoveryURL = server.URL
	cfg.WorkLedger.SchedulerA2AAllowPrivateHosts = true
	if _, err := buildA2AWorkExecutor(context.Background(), cfg, ledger, server.Client()); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("missing credential error=%v", err)
	}
}

func createA2ARuntimeExecution(t *testing.T, ledger *workstore.Store) workscheduler.Execution {
	t.Helper()
	ctx := context.Background()
	work, err := ledger.CreateWork(ctx, workstore.CreateWorkInput{
		WorkspaceID: "default", IdempotencyKey: "a2a-runtime-work", Kind: "external-agent",
		Title: "Review", Objective: "Review durable work", InitialState: workstore.WorkStateRunning, ActorID: "test",
	})
	if err != nil {
		t.Fatalf("create A2A work: %v", err)
	}
	step, err := ledger.CreateStep(ctx, workstore.CreateStepInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, IdempotencyKey: "a2a-runtime-step",
		Title: "Review", State: workstore.WorkStateRunning, ActorID: "test",
	})
	if err != nil {
		t.Fatalf("create A2A step: %v", err)
	}
	attempt, err := ledger.CreateAttempt(ctx, workstore.CreateAttemptInput{
		WorkspaceID: work.WorkspaceID, WorkID: work.ID, StepID: step.ID, IdempotencyKey: "a2a-runtime-attempt",
		Number: 1, Adapter: a2a.AdapterName, Status: workstore.AttemptStatusRunning, ActorID: "test",
	})
	if err != nil {
		t.Fatalf("create A2A attempt: %v", err)
	}
	return workscheduler.Execution{Work: work, Claim: workstore.StepClaim{Step: step, Attempt: attempt}}
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

func TestConfiguredClaudeCodeHarnessRunsDurableManagedLifecycle(t *testing.T) {
	workspaceDir := t.TempDir()
	if output, err := exec.Command("git", "-C", workspaceDir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "source.txt"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", workspaceDir, "add", "source.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", workspaceDir, "-c", "user.name=TARS Test", "-c", "user.email=tars@example.com", "commit", "-m", "initial").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, output)
	}

	stubDir := t.TempDir()
	argsPath := filepath.Join(stubDir, "args.txt")
	envPath := filepath.Join(stubDir, "env.txt")
	stubPath := filepath.Join(stubDir, "claude")
	stub := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + shellQuoteForTest(argsPath) + "\n" +
		"env > " + shellQuoteForTest(envPath) + "\n" +
		"printf 'generated by harness\\n' > generated.txt\n" +
		`printf '%s\n' '{"type":"assistant","message":{"model":"sonnet","content":[{"type":"text","text":"implemented generated.txt"},{"type":"tool_use","id":"tool-1","name":"Write","input":{"file_path":"generated.txt"}}]}}'` + "\n" +
		`printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"num_turns":2,"session_id":"harness-session","stop_reason":"end_turn","total_cost_usd":0.25,"usage":{"input_tokens":10,"output_tokens":4},"result":"implemented generated.txt"}'` + "\n"
	if err := os.WriteFile(stubPath, []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_CLI_PATH", stubPath)
	t.Setenv("ANTHROPIC_API_KEY", "must-not-reach-worker")

	harnessConfigPath := filepath.Join(t.TempDir(), "claude-code.json")
	writePrivateJSON(t, harnessConfigPath, executionplane.ClaudeCodeHarnessConfig{
		SchemaVersion: 1, Adapter: "claude-code", Model: "sonnet", TimeoutSeconds: 60,
		MaxTurns: 4, MaxBudgetUSD: 1,
		Tools:        []string{"Read", "Edit", "Write", "Glob", "Grep"},
		AllowedTools: []string{"Read(./**)", "Edit(./**)", "Write(./**)", "Glob(./**)", "Grep(./**)"},
	})
	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	runtime := agentruntime.NewRuntime(agentruntime.RuntimeOptions{
		Enabled: true, WorkspaceDir: workspaceDir, SessionStore: session.NewStore(t.TempDir()),
	})
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	dataDir := filepath.Join(t.TempDir(), "execution-data")
	cfg := config.Default()
	cfg.WorkspaceDir = workspaceDir
	cfg.WorkLedger.SchedulerEnabled = true
	cfg.WorkLedger.SchedulerExecutionDataDir = dataDir
	cfg.WorkLedger.SchedulerExternalHarnessConfigPath = harnessConfigPath
	cfg.WorkLedger.SchedulerPollMilliseconds = 5
	cfg.WorkLedger.SchedulerHeartbeatSeconds = 1
	cfg.WorkLedger.SchedulerLeaseSeconds = 5
	scheduler, err := buildWorkSchedulerIfEnabled(cfg, ledger, runtime, zerolog.Nop())
	if err != nil {
		t.Fatalf("build scheduler: %v", err)
	}
	t.Cleanup(scheduler.Close)
	work, err := scheduler.Submit(context.Background(), workscheduler.SubmitInput{
		WorkspaceID: "default", IdempotencyKey: "claude-code-harness", Kind: "coding", Source: "test",
		SourceID: "claude-code", Title: "Create generated file", Objective: "Implement a generated source artifact",
		Adapter: "claude-code", ActorID: "test",
		ContractJSON: json.RawMessage(`{"private":"must-not-be-sent"}`),
		MetadataJSON: json.RawMessage(`{"private":"must-not-be-sent"}`),
		Steps:        []workscheduler.StepSpec{{Key: "implement", Title: "Implement", Description: "Create generated.txt", Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := scheduler.RunOnce(context.Background()); err != nil || claimed != 1 {
		t.Fatalf("run once claimed=%d err=%v", claimed, err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projection, err := scheduler.Wait(waitCtx, work.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if projection.Work.State != workstore.WorkStateDone || len(projection.Attempts) != 1 || len(projection.Artifacts) != 2 {
		t.Fatalf("projection = %+v", projection)
	}
	if projection.Attempts[0].Status != workstore.AttemptStatusSucceeded || !strings.Contains(string(projection.Attempts[0].OutputJSON), "implemented generated.txt") {
		t.Fatalf("attempt = %+v", projection.Attempts[0])
	}
	if len(projection.Schedules) != 1 ||
		projection.Schedules[0].ConsumedIterations != 2 ||
		projection.Schedules[0].ConsumedTokens != 14 ||
		projection.Schedules[0].ConsumedCostUSD != 0.25 {
		t.Fatalf("schedule usage = %+v", projection.Schedules)
	}
	artifactBodies := map[string]string{}
	for _, artifact := range projection.Artifacts {
		path, err := filepathFromArtifactURI(artifact.URI)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		artifactBodies[artifact.Name] = string(raw)
	}
	if !strings.Contains(artifactBodies["changes.patch"], "generated.txt") || !strings.Contains(artifactBodies["changes.patch"], "generated by harness") {
		t.Fatalf("changes.patch = %s", artifactBodies["changes.patch"])
	}
	if !strings.Contains(artifactBodies["transcript.jsonl"], "Create generated.txt") || strings.Contains(artifactBodies["transcript.jsonl"], "must-not-be-sent") {
		t.Fatalf("transcript.jsonl = %s", artifactBodies["transcript.jsonl"])
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "generated.txt")); !os.IsNotExist(err) {
		t.Fatalf("source workspace was modified: %v", err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--safe-mode", "--strict-mcp-config", "--no-chrome", "--max-turns", "--max-budget-usd"} {
		if !strings.Contains(string(args), flag) {
			t.Fatalf("harness args missing %s:\n%s", flag, args)
		}
	}
	environment, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(environment), "ANTHROPIC_API_KEY=") || strings.Contains(string(environment), "must-not-reach-worker") {
		t.Fatalf("credential environment reached worker:\n%s", environment)
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
			t.Fatalf("missing lifecycle event %s", eventType)
		}
	}
}

func TestConfiguredClaudeCodeHarnessIsDisabledByDefaultAndRejectsWorkspaceConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.WorkspaceDir = t.TempDir()
	cfg.WorkLedger.SchedulerExecutionDataDir = filepath.Join(t.TempDir(), "execution-data")
	if executor, err := buildConfiguredClaudeCodeWorkExecutor(cfg, nil); err != nil || executor != nil {
		t.Fatalf("disabled harness executor=%v err=%v", executor, err)
	}
	path := filepath.Join(cfg.WorkspaceDir, "claude-code.json")
	writePrivateJSON(t, path, executionplane.ClaudeCodeHarnessConfig{
		SchemaVersion: 1, Adapter: "claude-code", Model: "sonnet", TimeoutSeconds: 60,
		MaxTurns: 2, MaxBudgetUSD: 1, Tools: []string{"Read"}, AllowedTools: []string{"Read(./**)"},
	})
	cfg.WorkLedger.SchedulerExternalHarnessConfigPath = path
	if executor, err := buildConfiguredClaudeCodeWorkExecutor(cfg, nil); err == nil || executor != nil || !strings.Contains(err.Error(), "outside the workspace") {
		t.Fatalf("workspace config executor=%v err=%v", executor, err)
	}
}

func shellQuoteForTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
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
