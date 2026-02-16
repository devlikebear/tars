package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExecBackgroundAndProcessLifecycle(t *testing.T) {
	root := t.TempDir()
	mgr := NewProcessManager()
	execTool := NewExecToolWithManager(root, mgr)
	procTool := NewProcessTool(mgr)

	res, err := execTool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 1","background":true}`))
	if err != nil {
		t.Fatalf("exec background: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected background start success, got %s", res.Text())
	}
	var start map[string]any
	if err := json.Unmarshal([]byte(res.Text()), &start); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	sessionID, _ := start["session_id"].(string)
	if strings.TrimSpace(sessionID) == "" {
		t.Fatalf("expected session_id, got %s", res.Text())
	}

	listRes, err := procTool.Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("process list: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("expected list success, got %s", listRes.Text())
	}
	if !strings.Contains(listRes.Text(), sessionID) {
		t.Fatalf("expected listed session id in %s", listRes.Text())
	}

	killRes, err := procTool.Execute(context.Background(), json.RawMessage(`{"action":"kill","session_id":"`+sessionID+`"}`))
	if err != nil {
		t.Fatalf("process kill: %v", err)
	}
	if killRes.IsError {
		t.Fatalf("expected kill success, got %s", killRes.Text())
	}

	removeRes, err := procTool.Execute(context.Background(), json.RawMessage(`{"action":"remove","session_id":"`+sessionID+`"}`))
	if err != nil {
		t.Fatalf("process remove: %v", err)
	}
	if removeRes.IsError {
		t.Fatalf("expected remove success, got %s", removeRes.Text())
	}
}

func TestExecBackgroundPollCompletes(t *testing.T) {
	root := t.TempDir()
	mgr := NewProcessManager()
	execTool := NewExecToolWithManager(root, mgr)
	procTool := NewProcessTool(mgr)

	res, err := execTool.Execute(context.Background(), json.RawMessage(`{"command":"echo hi","background":true}`))
	if err != nil {
		t.Fatalf("exec background: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected background start success, got %s", res.Text())
	}
	var start map[string]any
	if err := json.Unmarshal([]byte(res.Text()), &start); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	sessionID, _ := start["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("missing session id: %s", res.Text())
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		pollRes, err := procTool.Execute(context.Background(), json.RawMessage(`{"action":"poll","session_id":"`+sessionID+`"}`))
		if err != nil {
			t.Fatalf("process poll: %v", err)
		}
		if !pollRes.IsError && strings.Contains(pollRes.Text(), `"done":true`) {
			if !strings.Contains(pollRes.Text(), "hi") {
				t.Fatalf("expected stdout in poll response, got %s", pollRes.Text())
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("poll did not complete in time: %s", pollRes.Text())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
