package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
)

func TestA2AClientAndExecutorConstructionFailClosed(t *testing.T) {
	t.Parallel()

	invalidInterfaces := []AgentInterface{
		{URL: "https://agent.example.test", ProtocolBinding: "JSONRPC", ProtocolVersion: ProtocolVersion},
		{URL: "https://agent.example.test", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: "0.3"},
		{URL: "http://agent.example.test", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion},
		{URL: "https://user:secret@agent.example.test", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion},
		{URL: "https://agent.example.test?token=secret", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion},
		{URL: "https://agent.example.test/#fragment", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion},
	}
	for _, endpoint := range invalidInterfaces {
		if _, err := NewClient(endpoint, ClientOptions{}); err == nil {
			t.Errorf("NewClient() accepted unsafe endpoint %+v", endpoint)
		}
	}
	client, err := NewClient(AgentInterface{
		URL: "https://agent.example.test/a2a", ProtocolBinding: BindingHTTPJSON,
		ProtocolVersion: ProtocolVersion, Tenant: "tenant-a",
	}, ClientOptions{AllowedHosts: []string{"AGENT.EXAMPLE.TEST"}})
	if err != nil {
		t.Fatalf("NewClient(): %v", err)
	}
	if client.Interface().Tenant != "tenant-a" || client.maxRequestBytes != defaultMaxRequestBytes || client.maxResponseBytes != defaultMaxResponseBytes || client.httpClient.Timeout != 15*time.Second {
		t.Fatalf("client defaults=%+v", client)
	}
	if _, err := NewExecutor(ExecutorOptions{}); err == nil {
		t.Fatal("NewExecutor() accepted missing client and journal")
	}
	journal := &memoryJournal{}
	executor, err := NewExecutor(ExecutorOptions{Client: client, Journal: journal})
	if err != nil {
		t.Fatalf("NewExecutor(): %v", err)
	}
	if executor.Adapter() != AdapterName || executor.pollInterval != 2*time.Second || executor.maxPollDuration != 30*time.Minute || executor.maxPartBytes != defaultMaxArtifactPartBytes || len(executor.acceptedModes) != 2 {
		t.Fatalf("executor defaults=%+v", executor)
	}
	var nilExecutor *Executor
	if _, err := nilExecutor.Execute(context.Background(), workscheduler.Execution{}); err == nil {
		t.Fatal("nil executor executed")
	}
	if _, _, err := nilExecutor.Recover(context.Background(), workscheduler.Execution{}); err == nil {
		t.Fatal("nil executor recovered")
	}
	if err := nilExecutor.Cancel(context.Background(), workscheduler.Execution{}); err == nil {
		t.Fatal("nil executor canceled")
	}
}

func TestA2AMessageAndTaskContractsRejectAmbiguousPayloads(t *testing.T) {
	t.Parallel()

	text := "text"
	messageCases := []struct {
		name string
		call func() error
	}{
		{name: "blank id", call: func() error { return validateOutboundMessage(Message{Role: RoleUser, Parts: []Part{NewTextPart("x")}}) }},
		{name: "no parts", call: func() error { return validateOutboundMessage(Message{MessageID: "m1", Role: RoleUser}) }},
		{name: "wrong outbound role", call: func() error {
			return validateOutboundMessage(Message{MessageID: "m1", Role: RoleAgent, Parts: []Part{NewTextPart("x")}})
		}},
		{name: "wrong inbound role", call: func() error {
			return validateInboundMessage(Message{MessageID: "m1", Role: RoleUser, Parts: []Part{NewTextPart("x")}})
		}},
		{name: "empty part", call: func() error {
			return validateOutboundMessage(Message{MessageID: "m1", Role: RoleUser, Parts: []Part{{}}})
		}},
		{name: "ambiguous part", call: func() error {
			return validateOutboundMessage(Message{MessageID: "m1", Role: RoleUser, Parts: []Part{{Text: &text, Data: json.RawMessage(`{}`)}}})
		}},
		{name: "invalid data", call: func() error {
			return validateOutboundMessage(Message{MessageID: "m1", Role: RoleUser, Parts: []Part{{Data: json.RawMessage(`{`)}}})
		}},
		{name: "raw outbound", call: func() error {
			return validateOutboundMessage(Message{MessageID: "m1", Role: RoleUser, Parts: []Part{{Raw: "secret"}}})
		}},
	}
	for _, tc := range messageCases {
		if err := tc.call(); err == nil {
			t.Errorf("%s payload was accepted", tc.name)
		}
	}
	if err := validateInboundMessage(Message{MessageID: "m1", Role: RoleAgent, Parts: []Part{NewTextPart("ok")}}); err != nil {
		t.Fatalf("valid inbound message: %v", err)
	}

	taskCases := []Task{
		{Status: TaskStatus{State: TaskStateWorking}},
		{ID: "task-1", ContextID: "../escape", Status: TaskStatus{State: TaskStateWorking}},
		{ID: "task-1", Status: TaskStatus{State: TaskStateUnspecified}},
		{ID: "task-1", Status: TaskStatus{State: TaskStateCompleted}, Artifacts: []Artifact{{ArtifactID: "", Parts: []Part{NewTextPart("x")}}}},
		{ID: "task-1", Status: TaskStatus{State: TaskStateCompleted}, Artifacts: []Artifact{{ArtifactID: "a1"}}},
		{ID: "task-1", Status: TaskStatus{State: TaskStateCompleted}, Artifacts: []Artifact{{ArtifactID: "a1", Parts: []Part{{}}}}},
	}
	for _, task := range taskCases {
		if err := validateTask(task); err == nil {
			t.Errorf("invalid task was accepted: %+v", task)
		}
	}
	for _, state := range []TaskState{TaskStateSubmitted, TaskStateWorking, TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateInputRequired, TaskStateRejected, TaskStateAuthRequired} {
		if err := validateTask(Task{ID: "task-1", Status: TaskStatus{State: state}}); err != nil {
			t.Errorf("state %s rejected: %v", state, err)
		}
	}
}

func TestA2AClientHTTPFailureBoundaries(t *testing.T) {
	t.Parallel()

	validMessage := SendMessageRequest{Message: Message{MessageID: "message-1", Role: RoleUser, Parts: []Part{NewTextPart("run")}}}
	var nilClient *Client
	if err := nilClient.doJSON(context.Background(), http.MethodGet, "/tasks/task-1", nil, &Task{}); err == nil {
		t.Fatal("nil client issued request")
	}

	tests := []struct {
		name     string
		handler  http.HandlerFunc
		options  func(*ClientOptions)
		wantAuth bool
	}{
		{name: "token provider error", handler: validTaskResponse, options: func(options *ClientOptions) {
			options.TokenProvider = TokenProviderFunc(func(context.Context) (string, error) { return "", errors.New("vault unavailable") })
		}, wantAuth: true},
		{name: "blank token", handler: validTaskResponse, options: func(options *ClientOptions) {
			options.TokenProvider = TokenProviderFunc(func(context.Context) (string, error) { return " ", nil })
		}, wantAuth: true},
		{name: "header injection", handler: validTaskResponse, options: func(options *ClientOptions) {
			options.TokenProvider = TokenProviderFunc(func(context.Context) (string, error) { return "token\r\nX-Evil: 1", nil })
		}, wantAuth: true},
		{name: "http error", handler: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusBadGateway) }},
		{name: "wrong media type", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`{}`))
		}},
		{name: "malformed json", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", MediaType)
			_, _ = w.Write([]byte(`{`))
		}},
		{name: "ambiguous response", handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", MediaType)
			_, _ = w.Write([]byte(`{}`))
		}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()
			options := ClientOptions{HTTPClient: server.Client(), AllowLoopbackHTTP: true}
			if tc.options != nil {
				tc.options(&options)
			}
			client, err := NewClient(AgentInterface{URL: server.URL, ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion}, options)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.SendMessage(context.Background(), validMessage)
			if err == nil {
				t.Fatal("failure response was accepted")
			}
			if tc.wantAuth && !errors.Is(err, ErrAuthentication) {
				t.Fatalf("error=%v want ErrAuthentication", err)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(validTaskResponse))
	defer server.Close()
	client, err := NewClient(AgentInterface{URL: server.URL, ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion}, ClientOptions{HTTPClient: server.Client(), AllowLoopbackHTTP: true, MaxRequestBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendMessage(context.Background(), validMessage); !errors.Is(err, ErrRequestLimit) {
		t.Fatalf("oversized request error=%v", err)
	}
	if _, err := client.GetTask(context.Background(), "../escape"); err == nil {
		t.Fatal("unsafe get task ID was accepted")
	}
	if _, err := client.CancelTask(context.Background(), ""); err == nil {
		t.Fatal("blank cancel task ID was accepted")
	}
}

func TestA2ACompletionMapsAllOperatorAndTerminalStates(t *testing.T) {
	t.Parallel()

	executor := &Executor{client: &Client{}, journal: &memoryJournal{}, maxPartBytes: 1024}
	execution := testExternalExecution()
	cases := []struct {
		state     TaskState
		succeeded bool
		message   string
	}{
		{TaskStateCompleted, true, ""},
		{TaskStateInputRequired, false, "operator input"},
		{TaskStateAuthRequired, false, "operator authentication"},
		{TaskStateFailed, false, "failed"},
		{TaskStateCanceled, false, "canceled"},
		{TaskStateRejected, false, "rejected"},
		{TaskStateWorking, false, "unsupported"},
	}
	for _, tc := range cases {
		result, err := executor.completeTask(context.Background(), execution, Task{ID: "task-1", Status: TaskStatus{State: tc.state}}, 3)
		if err != nil || result.Succeeded != tc.succeeded || result.Usage.Iterations != 3 || !strings.Contains(result.Error, tc.message) {
			t.Errorf("state=%s result=%+v err=%v", tc.state, result, err)
		}
	}

	prompt := executionPrompt(execution)
	if !strings.Contains(prompt, "Work: Review the patch") || !strings.Contains(prompt, "Instructions: Run checks") || strings.Contains(prompt, "contract-secret") || strings.Contains(prompt, "metadata-secret") {
		t.Fatalf("bounded execution prompt=%q", prompt)
	}
	if got := messageIDForAttempt(" attempt-1 "); got != messageIDForAttempt("attempt-1") || !strings.HasPrefix(got, "tars-") {
		t.Fatalf("message ID=%q", got)
	}

	missing := &memoryJournal{}
	executor.journal = missing
	if err := executor.Cancel(context.Background(), execution); err != nil {
		t.Fatalf("cancel without external reference: %v", err)
	}
	if _, found, err := executor.Recover(context.Background(), execution); err != nil || found {
		t.Fatalf("recover missing reference found=%v err=%v", found, err)
	}
}

func TestA2AEndpointAndBoundedReadHelpers(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLoopbackName(value) {
			t.Errorf("%q not detected as loopback", value)
		}
	}
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "0.0.0.0", "169.254.1.1", "224.0.0.1"} {
		if !isPrivateAddress(net.ParseIP(value)) {
			t.Errorf("%q not detected as private", value)
		}
	}
	if !sameHost(" Agent.Example.Test ", "agent.example.test") {
		t.Fatal("host equality was not case-insensitive")
	}
	parsed, _ := url.Parse("https://agent.example.test")
	if err := validateEndpointSyntax(parsed, EndpointPolicy{AllowedHosts: []string{"other.example.test"}}); err == nil {
		t.Fatal("non-allowlisted host was accepted")
	}
	if err := validateJSONContentType("application/json; charset=utf-8", true); err != nil {
		t.Fatalf("discovery application/json rejected: %v", err)
	}
	if err := validateJSONContentType("application/json", false); err == nil {
		t.Fatal("non-A2A API content type was accepted")
	}
	if _, err := readBounded(strings.NewReader("x"), 0); !errors.Is(err, ErrResponseLimit) {
		t.Fatalf("zero bound error=%v", err)
	}
	if _, err := readBounded(errorReader{}, 10); err == nil {
		t.Fatal("reader failure was ignored")
	}
	client := noRedirectClient(&http.Client{})
	if client.Timeout != 15*time.Second || client.CheckRedirect == nil {
		t.Fatalf("safe HTTP client=%+v", client)
	}
}

func validTaskResponse(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", MediaType)
	_, _ = w.Write([]byte(`{"task":{"id":"task-1","status":{"state":"TASK_STATE_WORKING"}}}`))
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
