package tarsserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/session"
	"github.com/rs/zerolog"
)

func TestAgentRunsAPIHandler_GatewayRunEventsEndpointStreamsRunLifecycle(t *testing.T) {
	store := session.NewStore(t.TempDir())
	release := make(chan struct{})
	executor, err := gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name:        "worker",
		Description: "worker",
		PolicyMode:  "allowlist",
		ToolsAllow:  []string{"read_file"},
		RunPrompt: func(_ context.Context, _ string, _ string, _ []string, _ string, _ *gateway.ProviderOverride) (string, error) {
			<-release
			return "done", nil
		},
	})
	if err != nil {
		t.Fatalf("new prompt executor: %v", err)
	}
	runtime := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		Executors:    []gateway.AgentExecutor{executor},
		DefaultAgent: "worker",
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if closeErr := runtime.Close(ctx); closeErr != nil {
			t.Fatalf("close gateway runtime: %v", closeErr)
		}
	})
	h := newAgentRunsAPIHandler(runtime, zerolog.New(io.Discard))
	run, err := runtime.Spawn(context.Background(), gateway.SpawnRequest{Prompt: "hello", Agent: "worker"})
	if err != nil {
		t.Fatalf("spawn run: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/v1/gateway/runs/"+run.ID+"/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		cancel()
		<-done
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"run_finished"`) {
		t.Fatalf("expected run_finished event in stream, got %q", body)
	}
}
