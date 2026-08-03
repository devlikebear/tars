package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestExecutorRunsExternalTaskAndQuarantinesFiles(t *testing.T) {
	var sends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", MediaType)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/a2a/message:send":
			sends.Add(1)
			body := new(bytes.Buffer)
			_, _ = body.ReadFrom(r.Body)
			if strings.Contains(body.String(), "contract-secret") || strings.Contains(body.String(), "metadata-secret") {
				t.Fatalf("executor forwarded ledger-only data: %s", body.String())
			}
			if !strings.Contains(body.String(), "Review the patch") || !strings.Contains(body.String(), "Check behavior") {
				t.Fatalf("executor omitted task instructions: %s", body.String())
			}
			_, _ = w.Write([]byte(`{"task":{"id":"remote-1","contextId":"context-1","status":{"state":"TASK_STATE_WORKING"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/a2a/tasks/remote-1":
			_, _ = w.Write([]byte(`{
  "id":"remote-1",
  "contextId":"context-1",
  "status":{"state":"TASK_STATE_COMPLETED"},
  "artifacts":[{"artifactId":"report","parts":[
    {"text":"approved","mediaType":"text/plain"},
    {"url":"https://files.example.test/result?token=secret","filename":"result.txt","mediaType":"text/plain"}
  ]}]
}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	journal := &memoryJournal{}
	executor := newTestExecutor(t, server, journal)
	execution := testExternalExecution()
	result, err := executor.Execute(context.Background(), execution)
	if err != nil {
		t.Fatalf("execute external task: %v", err)
	}
	if !result.Succeeded || sends.Load() != 1 {
		t.Fatalf("unexpected result: %#v sends=%d", result, sends.Load())
	}
	var output TaskOutput
	if err := json.Unmarshal(result.OutputJSON, &output); err != nil {
		t.Fatalf("decode result output: %v", err)
	}
	if output.TaskID != "remote-1" || output.State != TaskStateCompleted || len(output.Quarantined) != 1 {
		t.Fatalf("unexpected output: %#v", output)
	}
	if strings.Contains(string(result.OutputJSON), "token=secret") {
		t.Fatalf("result persisted quarantined URL: %s", result.OutputJSON)
	}

	records := journal.snapshot()
	if len(records) != 4 || records[0].Kind != EventTaskSubmitted ||
		records[1].State != TaskStateWorking || records[2].State != TaskStateCompleted ||
		records[3].Kind != EventArtifactQuarantined {
		t.Fatalf("unexpected journal events: %#v", records)
	}
}

func TestExecutorMapsInterruptedTaskToHumanReviewFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", MediaType)
		_, _ = w.Write([]byte(`{"task":{"id":"remote-input","status":{"state":"TASK_STATE_INPUT_REQUIRED","message":{"messageId":"question-1","role":"ROLE_AGENT","parts":[{"text":"Which environment?"}]}}}}`))
	}))
	t.Cleanup(server.Close)

	executor := newTestExecutor(t, server, &memoryJournal{})
	result, err := executor.Execute(context.Background(), testExternalExecution())
	if err != nil {
		t.Fatalf("execute interrupted task: %v", err)
	}
	if result.Succeeded || !strings.Contains(result.Error, "operator input") {
		t.Fatalf("interrupted task should require review: %#v", result)
	}
}

func TestExecutorRecoversAndCancelsDurableExternalTaskWithoutResending(t *testing.T) {
	var sends atomic.Int32
	var cancels atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", MediaType)
		switch {
		case r.URL.Path == "/a2a/message:send":
			sends.Add(1)
			http.Error(w, "must not resend", http.StatusConflict)
		case r.Method == http.MethodGet && r.URL.Path == "/a2a/tasks/durable-1":
			_, _ = w.Write([]byte(`{"id":"durable-1","contextId":"ctx","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"artifactId":"answer","parts":[{"text":"recovered"}]}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/a2a/tasks/durable-1:cancel":
			cancels.Add(1)
			_, _ = w.Write([]byte(`{"id":"durable-1","contextId":"ctx","status":{"state":"TASK_STATE_CANCELED"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	execution := testExternalExecution()
	journal := &memoryJournal{reference: TaskReference{TaskID: "durable-1", ContextID: "ctx"}, found: true}
	executor := newTestExecutor(t, server, journal)
	result, found, err := executor.Recover(context.Background(), execution)
	if err != nil || !found || !result.Succeeded || sends.Load() != 0 {
		t.Fatalf("recover result=%#v found=%v sends=%d err=%v", result, found, sends.Load(), err)
	}
	if err := executor.Cancel(context.Background(), execution); err != nil {
		t.Fatalf("cancel recovered task: %v", err)
	}
	if cancels.Load() != 1 {
		t.Fatalf("cancel calls=%d", cancels.Load())
	}
}

func newTestExecutor(t *testing.T, server *httptest.Server, journal Journal) *Executor {
	t.Helper()
	client, err := NewClient(AgentInterface{
		URL: server.URL + "/a2a", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion,
	}, ClientOptions{HTTPClient: server.Client(), AllowLoopbackHTTP: true})
	if err != nil {
		t.Fatalf("new test client: %v", err)
	}
	executor, err := NewExecutor(ExecutorOptions{
		Client: client, Journal: journal, PollInterval: time.Millisecond, MaxPollDuration: time.Second,
	})
	if err != nil {
		t.Fatalf("new test executor: %v", err)
	}
	return executor
}

func testExternalExecution() workscheduler.Execution {
	return workscheduler.Execution{
		Work: workstore.Work{
			ID: "work-1", WorkspaceID: "workspace-1", Title: "Review the patch",
			Objective: "Check behavior", ContractJSON: json.RawMessage(`{"secret":"contract-secret"}`),
			MetadataJSON: json.RawMessage(`{"secret":"metadata-secret"}`),
		},
		Claim: workstore.StepClaim{
			Step:    workstore.Step{ID: "step-1", WorkID: "work-1", WorkspaceID: "workspace-1", Title: "Review", Description: "Run checks"},
			Attempt: workstore.Attempt{ID: "attempt-1"},
		},
	}
}

type memoryJournal struct {
	mu        sync.Mutex
	records   []ExternalEvent
	reference TaskReference
	found     bool
}

func (journal *memoryJournal) Record(_ context.Context, _ workscheduler.Execution, event ExternalEvent) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	journal.records = append(journal.records, event)
	if event.Kind == EventTaskSubmitted {
		journal.reference = TaskReference{TaskID: event.TaskID, ContextID: event.ContextID}
		journal.found = true
	}
	return nil
}

func (journal *memoryJournal) Lookup(_ context.Context, _ workscheduler.Execution) (TaskReference, bool, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.reference, journal.found, nil
}

func (journal *memoryJournal) snapshot() []ExternalEvent {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return append([]ExternalEvent(nil), journal.records...)
}
