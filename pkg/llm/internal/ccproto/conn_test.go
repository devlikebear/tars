package ccproto

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// wire captures what the Conn writes to the CLI's stdin, one frame per line.
type wire struct {
	mu     sync.Mutex
	frames []map[string]any
	added  chan struct{}
}

func newWire() *wire { return &wire{added: make(chan struct{}, 64)} }

func (w *wire) Write(p []byte) (int, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	for scanner.Scan() {
		var frame map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return 0, err
		}
		w.mu.Lock()
		w.frames = append(w.frames, frame)
		w.mu.Unlock()
		w.added <- struct{}{}
	}
	return len(p), nil
}

// next waits for the next frame written after the ones already consumed.
func (w *wire) next(t *testing.T, seen *int) map[string]any {
	t.Helper()
	for {
		w.mu.Lock()
		if *seen < len(w.frames) {
			frame := w.frames[*seen]
			*seen++
			w.mu.Unlock()
			return frame
		}
		w.mu.Unlock()
		select {
		case <-w.added:
		case <-time.After(2 * time.Second):
			t.Fatalf("no frame written after %d frames", *seen)
		}
	}
}

func TestConnRequestMatchesResponseByID(t *testing.T) {
	w := newWire()
	conn := NewConn(context.Background(), w, nil)
	defer conn.Close()

	type result struct {
		raw json.RawMessage
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := conn.Request(context.Background(), map[string]any{"subtype": "initialize"})
		done <- result{raw, err}
	}()

	seen := 0
	frame := w.next(t, &seen)
	if frame["type"] != "control_request" {
		t.Fatalf("type = %v, want control_request", frame["type"])
	}
	id, _ := frame["request_id"].(string)
	if id == "" {
		t.Fatal("request_id is empty")
	}
	if sub := frame["request"].(map[string]any)["subtype"]; sub != "initialize" {
		t.Fatalf("subtype = %v", sub)
	}

	// A response for someone else's request is not ours to consume.
	if !conn.Dispatch([]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"other","response":{}}}`)) {
		t.Fatal("control_response must be consumed even when unmatched")
	}
	conn.Dispatch([]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"` + id + `","response":{"models":[]}}}`))

	got := <-done
	if got.err != nil {
		t.Fatalf("request: %v", got.err)
	}
	if string(got.raw) != `{"models":[]}` {
		t.Fatalf("response = %s", got.raw)
	}
}

func TestConnRequestSurfacesErrorResponse(t *testing.T) {
	w := newWire()
	conn := NewConn(context.Background(), w, nil)
	defer conn.Close()

	done := make(chan error, 1)
	go func() {
		_, err := conn.Request(context.Background(), map[string]any{"subtype": "set_model"})
		done <- err
	}()
	seen := 0
	id := w.next(t, &seen)["request_id"].(string)
	conn.Dispatch([]byte(`{"type":"control_response","response":{"subtype":"error","request_id":"` + id + `","error":"unknown model"}}`))

	err := <-done
	var respErr *ResponseError
	if !errors.As(err, &respErr) || respErr.Message != "unknown model" {
		t.Fatalf("err = %v, want a ResponseError with the CLI's text", err)
	}
}

func TestConnRequestFailsWhenClosed(t *testing.T) {
	w := newWire()
	conn := NewConn(context.Background(), w, nil)

	done := make(chan error, 1)
	go func() {
		_, err := conn.Request(context.Background(), map[string]any{"subtype": "interrupt"})
		done <- err
	}()
	seen := 0
	w.next(t, &seen)
	conn.Close()

	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}
	if _, err := conn.Request(context.Background(), map[string]any{"subtype": "interrupt"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("request after close: err = %v, want ErrClosed", err)
	}
}

func TestConnAnswersInboundRequestWithHandlerResult(t *testing.T) {
	w := newWire()
	var gotSubtype string
	var gotRequest map[string]any
	handler := func(_ context.Context, subtype string, request json.RawMessage) (any, error) {
		gotSubtype = subtype
		_ = json.Unmarshal(request, &gotRequest)
		return map[string]any{"behavior": "deny", "message": "no"}, nil
	}
	conn := NewConn(context.Background(), w, handler)
	defer conn.Close()

	if !conn.Dispatch([]byte(`{"type":"control_request","request_id":"cli_1","request":{"subtype":"can_use_tool","tool_name":"Bash"}}`)) {
		t.Fatal("control_request must be consumed")
	}
	seen := 0
	frame := w.next(t, &seen)
	resp := frame["response"].(map[string]any)
	if frame["type"] != "control_response" || resp["subtype"] != "success" || resp["request_id"] != "cli_1" {
		t.Fatalf("frame = %v", frame)
	}
	if body := resp["response"].(map[string]any); body["behavior"] != "deny" {
		t.Fatalf("response body = %v", body)
	}
	if gotSubtype != "can_use_tool" || gotRequest["tool_name"] != "Bash" {
		t.Fatalf("handler saw subtype=%q request=%v", gotSubtype, gotRequest)
	}
}

func TestConnAnswersHandlerErrorAndMissingHandlerWithErrorResponse(t *testing.T) {
	for name, handler := range map[string]Handler{
		"handler error": func(context.Context, string, json.RawMessage) (any, error) { return nil, errors.New("boom") },
		"no handler":    nil,
	} {
		t.Run(name, func(t *testing.T) {
			w := newWire()
			conn := NewConn(context.Background(), w, handler)
			defer conn.Close()

			conn.Dispatch([]byte(`{"type":"control_request","request_id":"cli_2","request":{"subtype":"hook_callback"}}`))
			seen := 0
			resp := w.next(t, &seen)["response"].(map[string]any)
			if resp["subtype"] != "error" || resp["request_id"] != "cli_2" || resp["error"] == "" {
				t.Fatalf("response = %v", resp)
			}
		})
	}
}

func TestConnCancelRequestCancelsHandlerWithoutResponding(t *testing.T) {
	w := newWire()
	started := make(chan struct{})
	finished := make(chan struct{})
	handler := func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return map[string]any{"behavior": "allow"}, nil
	}
	conn := NewConn(context.Background(), w, handler)
	defer conn.Close()

	conn.Dispatch([]byte(`{"type":"control_request","request_id":"cli_3","request":{"subtype":"can_use_tool"}}`))
	<-started
	if !conn.Dispatch([]byte(`{"type":"control_cancel_request","request_id":"cli_3"}`)) {
		t.Fatal("control_cancel_request must be consumed")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("handler context was not cancelled")
	}
	conn.Close()

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.frames) != 0 {
		t.Fatalf("a cancelled request must not be answered, wrote %v", w.frames)
	}
}

func TestConnCloseCancelsInFlightHandlers(t *testing.T) {
	w := newWire()
	started := make(chan struct{})
	handler := func(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	conn := NewConn(context.Background(), w, handler)
	conn.Dispatch([]byte(`{"type":"control_request","request_id":"cli_4","request":{"subtype":"can_use_tool"}}`))
	<-started

	closed := make(chan struct{})
	go func() { conn.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return while a handler was blocked")
	}
}

func TestConnDispatchLeavesOtherFramesToCaller(t *testing.T) {
	conn := NewConn(context.Background(), io.Discard, nil)
	defer conn.Close()
	for _, line := range []string{
		`{"type":"assistant","message":{}}`,
		`{"type":"result","subtype":"success"}`,
		`{"type":"system","subtype":"init"}`,
		`not json`,
	} {
		if conn.Dispatch([]byte(line)) {
			t.Fatalf("Dispatch consumed %s", line)
		}
	}
}

func TestConnWriteUserMessage(t *testing.T) {
	w := newWire()
	conn := NewConn(context.Background(), w, nil)
	defer conn.Close()

	if err := conn.WriteUserMessage("hello"); err != nil {
		t.Fatalf("write: %v", err)
	}
	seen := 0
	frame := w.next(t, &seen)
	msg := frame["message"].(map[string]any)
	if frame["type"] != "user" || msg["role"] != "user" || msg["content"] != "hello" {
		t.Fatalf("frame = %v", frame)
	}
	if v, ok := frame["parent_tool_use_id"]; !ok || v != nil {
		t.Fatalf("parent_tool_use_id must be present and null, got %v", frame)
	}
}

func TestConnRequestGivesUpWhenContextEnds(t *testing.T) {
	w := newWire()
	conn := NewConn(context.Background(), w, nil)
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := conn.Request(ctx, map[string]any{"subtype": "interrupt"})
		done <- err
	}()
	seen := 0
	id := w.next(t, &seen)["request_id"].(string)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	// A late answer for the abandoned request is still consumed quietly.
	if !conn.Dispatch([]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"` + id + `","response":{}}}`)) {
		t.Fatal("late control_response must be consumed")
	}
}

type brokenPipe struct{}

func (brokenPipe) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestConnRequestReportsWriteFailure(t *testing.T) {
	conn := NewConn(context.Background(), brokenPipe{}, nil)
	defer conn.Close()

	_, err := conn.Request(context.Background(), map[string]any{"subtype": "initialize"})
	if err == nil || !strings.Contains(err.Error(), "broken pipe") {
		t.Fatalf("err = %v, want the write failure", err)
	}
	if err := conn.WriteUserMessage("hi"); err == nil {
		t.Fatal("WriteUserMessage must report the write failure")
	}
}

func TestResponseErrorMessage(t *testing.T) {
	err := &ResponseError{Message: "bad hooks"}
	if err.Error() != "ccproto: bad hooks" {
		t.Fatalf("Error() = %q", err.Error())
	}
}
