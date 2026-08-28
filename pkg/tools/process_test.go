package tools

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

func TestProcessTool_WaitCompletes(t *testing.T) {
	root := t.TempDir()
	mgr := NewProcessManager()
	execTool := NewExecToolWithManager(root, mgr)
	procTool := NewProcessTool(mgr)

	res, err := execTool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 1","background":true,"timeout_ms":10000}`))
	if err != nil {
		t.Fatalf("exec background: %v", err)
	}
	var start map[string]any
	_ = json.Unmarshal([]byte(res.Text()), &start)
	sessionID := start["session_id"].(string)

	startTime := time.Now()
	waitRes, err := procTool.Execute(context.Background(), json.RawMessage(`{"action":"wait","session_id":"`+sessionID+`","timeout_ms":5000}`))
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	elapsed := time.Since(startTime)
	if waitRes.IsError {
		t.Fatalf("expected wait success, got %s", waitRes.Text())
	}
	if !strings.Contains(waitRes.Text(), `"done":true`) {
		t.Fatalf("expected done=true after wait, got %s", waitRes.Text())
	}
	if strings.Contains(waitRes.Text(), `"wait_timed_out":true`) {
		t.Fatalf("did not expect wait_timed_out, got %s", waitRes.Text())
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected wait to return promptly when process exits, elapsed=%s", elapsed)
	}
}

func TestProcessTool_WaitReportsTimeout(t *testing.T) {
	root := t.TempDir()
	mgr := NewProcessManager()
	execTool := NewExecToolWithManager(root, mgr)
	procTool := NewProcessTool(mgr)

	res, err := execTool.Execute(context.Background(), json.RawMessage(`{"command":"sleep 5","background":true,"timeout_ms":10000}`))
	if err != nil {
		t.Fatalf("exec background: %v", err)
	}
	var start map[string]any
	_ = json.Unmarshal([]byte(res.Text()), &start)
	sessionID := start["session_id"].(string)
	defer func() {
		_, _ = procTool.Execute(context.Background(), json.RawMessage(`{"action":"kill","session_id":"`+sessionID+`"}`))
	}()

	waitRes, err := procTool.Execute(context.Background(), json.RawMessage(`{"action":"wait","session_id":"`+sessionID+`","timeout_ms":300}`))
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !strings.Contains(waitRes.Text(), `"wait_timed_out":true`) {
		t.Fatalf("expected wait_timed_out=true, got %s", waitRes.Text())
	}
	if !strings.Contains(waitRes.Text(), `"running":true`) {
		t.Fatalf("expected running=true while waiting, got %s", waitRes.Text())
	}
}

func TestTailBuffer_KeepsTailWithinCap(t *testing.T) {
	tb := newTailBuffer(8)
	if _, err := tb.Write([]byte("abcd")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := tb.String(); got != "abcd" {
		t.Fatalf("expected abcd, got %q", got)
	}
	// Append past cap; oldest should be dropped.
	if _, err := tb.Write([]byte("efghij")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := tb.String(); got != "cdefghij" {
		t.Fatalf("expected cdefghij (8 trailing bytes), got %q (len=%d)", got, len(got))
	}
	// Single write larger than cap → truncate to last cap bytes.
	if _, err := tb.Write([]byte("0123456789ABCDEF")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := tb.String(); got != "89ABCDEF" {
		t.Fatalf("expected last 8 bytes of long write, got %q", got)
	}
}
