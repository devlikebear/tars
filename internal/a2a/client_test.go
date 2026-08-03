package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClientSendPollAndCancelUsesA2AV1WithoutLeakingToken(t *testing.T) {
	const secret = "a2a-secret-token"
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("A2A-Version"); got != ProtocolVersion {
			t.Fatalf("A2A-Version = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", MediaType)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/a2a/message:send":
			var request SendMessageRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode send request: %v", err)
			}
			if request.Message.Role != RoleUser || request.Message.MessageID != "msg-1" || !request.Configuration.ReturnImmediately {
				t.Fatalf("unexpected request: %#v", request)
			}
			_, _ = w.Write([]byte(`{"task":{"id":"task-1","contextId":"ctx-1","status":{"state":"TASK_STATE_WORKING"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/a2a/tasks/task-1":
			polls.Add(1)
			_, _ = w.Write([]byte(`{"id":"task-1","contextId":"ctx-1","status":{"state":"TASK_STATE_COMPLETED"},"artifacts":[{"artifactId":"artifact-1","parts":[{"text":"done"}]}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/a2a/tasks/task-1:cancel":
			_, _ = w.Write([]byte(`{"id":"task-1","contextId":"ctx-1","status":{"state":"TASK_STATE_CANCELED"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(AgentInterface{
		URL: server.URL + "/a2a", ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion,
	}, ClientOptions{
		HTTPClient:        server.Client(),
		AllowLoopbackHTTP: true,
		TokenProvider: TokenProviderFunc(func(context.Context) (string, error) {
			return secret, nil
		}),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	response, err := client.SendMessage(context.Background(), SendMessageRequest{
		Message:       Message{MessageID: "msg-1", Role: RoleUser, Parts: []Part{NewTextPart("run task")}},
		Configuration: SendMessageConfiguration{AcceptedOutputModes: []string{"text/plain"}, ReturnImmediately: true},
	})
	if err != nil || response.Task == nil || response.Task.ID != "task-1" {
		t.Fatalf("send message: response=%#v err=%v", response, err)
	}
	task, err := client.GetTask(context.Background(), "task-1")
	if err != nil || task.Status.State != TaskStateCompleted || polls.Load() != 1 {
		t.Fatalf("get task: task=%#v polls=%d err=%v", task, polls.Load(), err)
	}
	canceled, err := client.CancelTask(context.Background(), "task-1")
	if err != nil || canceled.Status.State != TaskStateCanceled {
		t.Fatalf("cancel task: task=%#v err=%v", canceled, err)
	}

	raw, err := json.Marshal(struct {
		Client *Client `json:"client"`
	}{Client: client})
	if err != nil {
		t.Fatalf("marshal client: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("client serialization leaked token: %s", raw)
	}
}

func TestClientRejectsUnsafePartsInvalidResponsesAndBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", MediaType)
		_, _ = w.Write([]byte(strings.Repeat("x", 513)))
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(AgentInterface{
		URL: server.URL, ProtocolBinding: BindingHTTPJSON, ProtocolVersion: ProtocolVersion,
	}, ClientOptions{HTTPClient: server.Client(), AllowLoopbackHTTP: true, MaxResponseBytes: 512})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{MessageID: "msg-url", Role: RoleUser, Parts: []Part{{URL: "https://files.example.test/secret"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "url and raw parts") {
		t.Fatalf("expected unsafe part rejection, got %v", err)
	}

	_, err = client.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{MessageID: "msg-bound", Role: RoleUser, Parts: []Part{NewTextPart("bounded")}},
	})
	if err == nil || !strings.Contains(err.Error(), "response limit") {
		t.Fatalf("expected response bound rejection, got %v", err)
	}
}

func TestSanitizeTaskQuarantinesRemoteFilesAndKeepsTextAndData(t *testing.T) {
	data := json.RawMessage(`{"score":1}`)
	text := "approved"
	task := Task{
		ID: "task-1", ContextID: "context-1", Status: TaskStatus{State: TaskStateCompleted},
		Artifacts: []Artifact{{ArtifactID: "artifact-1", Parts: []Part{
			{Text: &text, MediaType: "text/plain"},
			{Data: data, MediaType: "application/json"},
			{URL: "https://files.example.test/result?token=must-not-persist", Filename: "remote.txt", MediaType: "text/plain"},
			{Raw: "c2VjcmV0", Filename: "raw.txt", MediaType: "text/plain"},
		}}},
	}

	output := SanitizeTask(task, 1024)
	if len(output.Artifacts) != 1 || len(output.Artifacts[0].Parts) != 2 {
		t.Fatalf("unexpected accepted artifacts: %#v", output.Artifacts)
	}
	if len(output.Quarantined) != 2 {
		t.Fatalf("unexpected quarantine report: %#v", output.Quarantined)
	}
	raw, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if strings.Contains(string(raw), "must-not-persist") || strings.Contains(string(raw), "c2VjcmV0") {
		t.Fatalf("quarantine output persisted remote content: %s", raw)
	}
}
