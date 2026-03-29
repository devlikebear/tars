package tool

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/session"
)

func newGatewayRuntimeForToolTests(t *testing.T) *gateway.Runtime {
	t.Helper()
	store := session.NewStore(t.TempDir())
	rt := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:                 true,
		WorkspaceDir:            t.TempDir(),
		SessionStore:            store,
		ChannelsLocalEnabled:    true,
		ChannelsWebhookEnabled:  true,
		ChannelsTelegramEnabled: true,
		RunPrompt: func(_ context.Context, _ string, prompt string) (string, error) {
			return "ok: " + prompt, nil
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rt.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})
	return rt
}

func TestSessionsSpawnAndRunsTools(t *testing.T) {
	rt := newGatewayRuntimeForToolTests(t)
	spawn := NewSessionsSpawnTool(rt)
	runs := NewSessionsRunsTool(rt)

	spawnRes, err := spawn.Execute(context.Background(), json.RawMessage(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("spawn execute: %v", err)
	}
	if spawnRes.IsError {
		t.Fatalf("spawn expected success: %s", spawnRes.Text())
	}

	listRes, err := runs.Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("runs list execute: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("runs list expected success: %s", listRes.Text())
	}
	var listPayload struct {
		Count int `json:"count"`
		Runs  []struct {
			ID string `json:"run_id"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(listRes.Text()), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if listPayload.Count == 0 || len(listPayload.Runs) == 0 {
		t.Fatalf("expected runs in list payload: %s", listRes.Text())
	}

	runID := listPayload.Runs[0].ID
	getRes, err := runs.Execute(context.Background(), json.RawMessage(`{"action":"get","run_id":"`+runID+`"}`))
	if err != nil {
		t.Fatalf("runs get execute: %v", err)
	}
	if getRes.IsError {
		t.Fatalf("runs get expected success: %s", getRes.Text())
	}
}

func TestSessionsSendTool(t *testing.T) {
	rt := newGatewayRuntimeForToolTests(t)
	send := NewSessionsSendTool(rt)
	res, err := send.Execute(context.Background(), json.RawMessage(`{"message":"hello","timeout_ms":5000}`))
	if err != nil {
		t.Fatalf("sessions_send execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("sessions_send expected success: %s", res.Text())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Text()), &payload); err != nil {
		t.Fatalf("decode send payload: %v", err)
	}
	if payload["status"] != string(gateway.RunStatusCompleted) {
		t.Fatalf("expected completed status, got %+v", payload)
	}
}

func TestMessageNodesGatewayTools(t *testing.T) {
	rt := newGatewayRuntimeForToolTests(t)

	message := NewMessageTool(rt, true)
	msgRes, err := message.Execute(context.Background(), json.RawMessage(`{"action":"send","channel_id":"general","text":"hello"}`))
	if err != nil {
		t.Fatalf("message send execute: %v", err)
	}
	if msgRes.IsError {
		t.Fatalf("message send expected success: %s", msgRes.Text())
	}
	readRes, err := message.Execute(context.Background(), json.RawMessage(`{"action":"read","channel_id":"general"}`))
	if err != nil {
		t.Fatalf("message read execute: %v", err)
	}
	if readRes.IsError {
		t.Fatalf("message read expected success: %s", readRes.Text())
	}

	nodes := NewNodesTool(rt, true)
	nodeRes, err := nodes.Execute(context.Background(), json.RawMessage(`{"action":"invoke","name":"clock.now"}`))
	if err != nil {
		t.Fatalf("nodes invoke execute: %v", err)
	}
	if nodeRes.IsError {
		t.Fatalf("nodes invoke expected success: %s", nodeRes.Text())
	}

	gatewayTool := NewGatewayTool(rt, true)
	statusRes, err := gatewayTool.Execute(context.Background(), json.RawMessage(`{"action":"status"}`))
	if err != nil {
		t.Fatalf("gateway status execute: %v", err)
	}
	if statusRes.IsError {
		t.Fatalf("gateway status expected success: %s", statusRes.Text())
	}
}

func TestSessionsRunsCancel(t *testing.T) {
	store := session.NewStore(t.TempDir())
	rt := gateway.NewRuntime(gateway.RuntimeOptions{
		Enabled:      true,
		SessionStore: store,
		RunPrompt: func(ctx context.Context, _ string, _ string) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(2 * time.Second):
				return "done", nil
			}
		},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := rt.Close(ctx); err != nil {
			t.Fatalf("close gateway runtime: %v", err)
		}
	})
	spawn := NewSessionsSpawnTool(rt)
	runs := NewSessionsRunsTool(rt)
	spawnRes, err := spawn.Execute(context.Background(), json.RawMessage(`{"message":"long"}`))
	if err != nil {
		t.Fatalf("spawn execute: %v", err)
	}
	if spawnRes.IsError {
		t.Fatalf("spawn expected success: %s", spawnRes.Text())
	}
	var spawnPayload struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(spawnRes.Text()), &spawnPayload); err != nil {
		t.Fatalf("decode spawn payload: %v", err)
	}
	cancelRes, err := runs.Execute(context.Background(), json.RawMessage(`{"action":"cancel","run_id":"`+spawnPayload.RunID+`"}`))
	if err != nil {
		t.Fatalf("runs cancel execute: %v", err)
	}
	if cancelRes.IsError {
		t.Fatalf("runs cancel expected success: %s", cancelRes.Text())
	}
}

func TestSessionsRunsTool_WorkspaceScoped(t *testing.T) {
	rt := newGatewayRuntimeForToolTests(t)
	spawn := NewSessionsSpawnTool(rt)
	runs := NewSessionsRunsTool(rt)

	ctxA := serverauth.WithWorkspaceID(context.Background(), "ws-a")
	ctxB := serverauth.WithWorkspaceID(context.Background(), "ws-b")

	resA, err := spawn.Execute(ctxA, json.RawMessage(`{"message":"from-a"}`))
	if err != nil {
		t.Fatalf("spawn ws-a execute: %v", err)
	}
	if resA.IsError {
		t.Fatalf("spawn ws-a expected success: %s", resA.Text())
	}

	resB, err := spawn.Execute(ctxB, json.RawMessage(`{"message":"from-b"}`))
	if err != nil {
		t.Fatalf("spawn ws-b execute: %v", err)
	}
	if resB.IsError {
		t.Fatalf("spawn ws-b expected success: %s", resB.Text())
	}

	listA, err := runs.Execute(ctxA, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("runs list ws-a execute: %v", err)
	}
	if listA.IsError {
		t.Fatalf("runs list ws-a expected success: %s", listA.Text())
	}
	var payloadA struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(listA.Text()), &payloadA); err != nil {
		t.Fatalf("decode ws-a list payload: %v", err)
	}
	if payloadA.Count != 1 {
		t.Fatalf("expected ws-a list count=1, got %d payload=%s", payloadA.Count, listA.Text())
	}
}
