package executionplane

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/llm"
)

func TestClaudeCodeWorkerNormalizesLifecycleTranscriptAndUsage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	canonicalRoot, err := canonicalDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	var seenMessages []llm.ChatMessage
	var seenOptions llm.ChatOptions
	worker, err := NewClaudeCodeWorker(ClaudeCodeWorkerOptions{
		Model: "sonnet", Timeout: time.Minute, MaxTurns: 12, MaxBudgetUSD: 2.5,
		Tools: []string{"Read", "Edit", "Bash"}, AllowedTools: []string{"Read(./**)", "Edit(./**)", "Bash(go test *)"},
		ClientFactory: func(gotRoot, gotModel string) (llm.Client, error) {
			if gotRoot != canonicalRoot || gotModel != "sonnet" {
				t.Fatalf("client root/model = %q/%q", gotRoot, gotModel)
			}
			return &fakeClaudeHarnessClient{chat: func(_ context.Context, messages []llm.ChatMessage, options llm.ChatOptions) (llm.ChatResponse, error) {
				seenMessages = append([]llm.ChatMessage(nil), messages...)
				seenOptions = options
				return llm.ChatResponse{
					Message: llm.ChatMessage{Role: "assistant", Content: "implemented safely"},
					Usage:   llm.Usage{InputTokens: 21, OutputTokens: 8, CostUSD: 0.42}, Turns: 3,
					StopReason: "end_turn", SessionID: "session-private",
					ProviderExecutedTools: []llm.ToolCall{{ID: "tool-1", Name: "Edit", Arguments: `{"file_path":"main.go"}`}},
				}, nil
			}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	execution := testExecution()
	execution.Work.Title = "Implement feature"
	execution.Work.Objective = "Make the requested change"
	execution.Work.ContractJSON = json.RawMessage(`{"secret":"contract-secret"}`)
	execution.Work.MetadataJSON = json.RawMessage(`{"secret":"metadata-secret"}`)
	execution.Claim.Step.Title = "Update source"
	execution.Claim.Step.Description = "Edit and test the code"
	result, err := worker.Execute(context.Background(), WorkerRequest{
		Execution:   execution,
		Environment: Environment{SchemaVersion: 1, ID: "worktree:attempt-1", Kind: "managed-worktree", RootDir: root},
	})
	if err != nil {
		t.Fatalf("execute worker: %v", err)
	}
	if !result.ExecutionResult.Succeeded || result.ExecutionResult.Usage.Iterations != 3 ||
		result.ExecutionResult.Usage.Tokens != 29 || result.ExecutionResult.Usage.CostUSD != 0.42 {
		t.Fatalf("execution result = %+v", result.ExecutionResult)
	}
	if len(seenMessages) != 2 || !strings.Contains(seenMessages[1].Content, "Edit and test the code") ||
		strings.Contains(seenMessages[1].Content, "contract-secret") || strings.Contains(seenMessages[1].Content, "metadata-secret") {
		t.Fatalf("bounded harness messages = %+v", seenMessages)
	}
	if seenOptions.ClaudeCodePermissionMode != "dontAsk" || seenOptions.ClaudeCodeHarness == nil ||
		!seenOptions.ClaudeCodeHarness.SafeMode || !seenOptions.ClaudeCodeHarness.StrictMCP || !seenOptions.ClaudeCodeHarness.DisableChrome || !seenOptions.ClaudeCodeHarness.IsolateEnvironment {
		t.Fatalf("harness options = %+v", seenOptions)
	}
	if len(result.Transcript) != 3 || result.Transcript[0].Type != "user" || result.Transcript[1].Type != "tool" || result.Transcript[2].Type != "assistant" {
		t.Fatalf("normalized transcript = %+v", result.Transcript)
	}
	var output map[string]any
	if err := json.Unmarshal(result.ExecutionResult.OutputJSON, &output); err != nil || output["summary"] != "implemented safely" || output["provider"] != "claude-code-cli" {
		t.Fatalf("output = %s err=%v", result.ExecutionResult.OutputJSON, err)
	}
	capabilities := worker.Capabilities()
	if capabilities.Resume || capabilities.Steering || !capabilities.Cancellation || !capabilities.ToolPolicy || !capabilities.Transcript || !capabilities.Cost {
		t.Fatalf("capabilities = %+v", capabilities)
	}
}

func TestClaudeCodeWorkerRejectsCredentialInjectionAndUnsupportedRecovery(t *testing.T) {
	t.Parallel()

	worker := mustClaudeCodeWorker(t, func(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
		t.Fatal("client must not run with injected credentials")
		return llm.ChatResponse{}, nil
	})
	request := WorkerRequest{
		Execution: testExecution(), Environment: Environment{Kind: "managed-worktree", RootDir: t.TempDir()},
		Credentials: CredentialGrant{ID: "long-lived", Values: map[string]string{"ANTHROPIC_API_KEY": "secret"}},
	}
	if _, err := worker.Execute(context.Background(), request); !errors.Is(err, ErrCredentialScope) {
		t.Fatalf("credential injection error = %v", err)
	}
	if _, found, err := worker.Recover(context.Background(), request, &WorkerCheckpoint{}); found || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("recover found=%v err=%v", found, err)
	}
}

func TestClaudeCodeWorkerRejectsUnboundedToolAuthority(t *testing.T) {
	t.Parallel()

	base := ClaudeCodeWorkerOptions{
		Model: "sonnet", Timeout: time.Minute, MaxTurns: 2, MaxBudgetUSD: 1,
		Tools: []string{"Read"}, AllowedTools: []string{"Read(./**)"},
	}
	for _, testCase := range []struct {
		name    string
		tools   []string
		allowed []string
	}{
		{name: "unscoped read", tools: []string{"Read"}, allowed: []string{"Read"}},
		{name: "unscoped bash", tools: []string{"Bash"}, allowed: []string{"Bash"}},
		{name: "network tool", tools: []string{"WebFetch"}, allowed: []string{"WebFetch(example.com)"}},
		{name: "rule injection", tools: []string{"Read"}, allowed: []string{"Read(./**),Bash"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			options := base
			options.Tools = testCase.tools
			options.AllowedTools = testCase.allowed
			if _, err := NewClaudeCodeWorker(options); err == nil {
				t.Fatalf("accepted tools=%v allowed=%v", testCase.tools, testCase.allowed)
			}
		})
	}
}

func TestClaudeCodeWorkerCancelStopsActiveInvocation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	worker := mustClaudeCodeWorker(t, func(ctx context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
		close(started)
		<-ctx.Done()
		return llm.ChatResponse{}, ctx.Err()
	})
	request := WorkerRequest{Execution: testExecution(), Environment: Environment{Kind: "managed-worktree", RootDir: t.TempDir()}}
	done := make(chan error, 1)
	go func() {
		_, err := worker.Execute(context.Background(), request)
		done <- err
	}()
	<-started
	if err := worker.Cancel(context.Background(), request); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("execute after cancel = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active invocation did not stop")
	}
}

func TestClaudeCodeWorkerBoundsTimeoutAndRecordsCrash(t *testing.T) {
	t.Parallel()

	t.Run("timeout", func(t *testing.T) {
		worker, err := NewClaudeCodeWorker(ClaudeCodeWorkerOptions{
			Model: "sonnet", Timeout: 20 * time.Millisecond, MaxTurns: 2, MaxBudgetUSD: 1,
			Tools: []string{"Read"}, AllowedTools: []string{"Read(./**)"},
			ClientFactory: func(string, string) (llm.Client, error) {
				return &fakeClaudeHarnessClient{chat: func(ctx context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
					<-ctx.Done()
					return llm.ChatResponse{}, ctx.Err()
				}}, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		request := WorkerRequest{Execution: testExecution(), Environment: Environment{Kind: "managed-worktree", RootDir: t.TempDir()}}
		if _, err := worker.Execute(context.Background(), request); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("crash", func(t *testing.T) {
		worker := mustClaudeCodeWorker(t, func(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
			return llm.ChatResponse{}, errors.New("subprocess crashed")
		})
		request := WorkerRequest{Execution: testExecution(), Environment: Environment{Kind: "managed-worktree", RootDir: t.TempDir()}}
		result, err := worker.Execute(context.Background(), request)
		if err == nil || !strings.Contains(err.Error(), "subprocess crashed") {
			t.Fatalf("crash error = %v", err)
		}
		if len(result.Transcript) != 2 || result.Transcript[1].Type != "error" || !strings.Contains(result.Transcript[1].Text, "subprocess crashed") {
			t.Fatalf("crash transcript = %+v", result.Transcript)
		}
	})
}

func mustClaudeCodeWorker(t *testing.T, chat func(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error)) *ClaudeCodeWorker {
	t.Helper()
	worker, err := NewClaudeCodeWorker(ClaudeCodeWorkerOptions{
		Model: "sonnet", Timeout: time.Minute, MaxTurns: 4, MaxBudgetUSD: 1,
		Tools: []string{"Read"}, AllowedTools: []string{"Read(./**)"},
		ClientFactory: func(string, string) (llm.Client, error) { return &fakeClaudeHarnessClient{chat: chat}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

type fakeClaudeHarnessClient struct {
	chat func(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error)
}

func (client *fakeClaudeHarnessClient) Ask(context.Context, string) (string, error) {
	return "", errors.New("unexpected Ask")
}

func (client *fakeClaudeHarnessClient) Chat(ctx context.Context, messages []llm.ChatMessage, options llm.ChatOptions) (llm.ChatResponse, error) {
	return client.chat(ctx, messages, options)
}
