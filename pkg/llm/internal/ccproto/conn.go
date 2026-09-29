// Package ccproto speaks the control protocol that Claude Code exposes when it
// runs with `--input-format stream-json --output-format stream-json`. It is the
// wire layer the Claude Agent SDKs are built on: both sides send
// control_request frames, answer the other side's with control_response, and
// may abandon one with control_cancel_request. Frames are newline-delimited
// JSON on the CLI's stdin and stdout, interleaved with ordinary stream events.
//
// The package owns only request correlation and framing. What a subtype means
// (can_use_tool, hook_callback, initialize, ...) belongs to the caller.
//
// Reference: the MIT-licensed claude-agent-sdk-python
// (_internal/query.py) and the @anthropic-ai/claude-agent-sdk type
// definitions, checked against Claude Code 2.1.283.
package ccproto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// ErrClosed is returned by Request once the Conn is closed.
var ErrClosed = errors.New("ccproto: connection closed")

// ResponseError is the CLI's error answer to one of our requests.
type ResponseError struct {
	Message string
}

func (e *ResponseError) Error() string { return "ccproto: " + e.Message }

// Handler answers one control request the CLI sent. Its result becomes the
// success payload; an error becomes an error response. ctx is cancelled when
// the CLI withdraws the request or the Conn closes, and the answer is then
// dropped. The caller ends a turn by closing the Conn, so a handler needs no
// other deadline.
type Handler func(ctx context.Context, subtype string, request json.RawMessage) (any, error)

// Conn multiplexes control requests in both directions over one CLI process.
// Dispatch must be fed every stdout line; the other methods are safe for
// concurrent use.
type Conn struct {
	handler Handler

	writeMu sync.Mutex
	w       io.Writer

	mu       sync.Mutex
	closed   bool
	nextID   uint64
	pending  map[string]chan reply
	inflight map[string]context.CancelFunc
	handlers sync.WaitGroup
}

type reply struct {
	payload json.RawMessage
	err     error
}

// NewConn writes frames to w, which is normally the CLI's stdin. handler may
// be nil, in which case every inbound request gets an error response.
func NewConn(w io.Writer, handler Handler) *Conn {
	return &Conn{
		handler:  handler,
		w:        w,
		pending:  map[string]chan reply{},
		inflight: map[string]context.CancelFunc{},
	}
}

// Request sends a control request and waits for the CLI's answer. request is
// the inner object and must carry its "subtype".
func (c *Conn) Request(ctx context.Context, request any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.nextID++
	id := "tars_req_" + strconv.FormatUint(c.nextID, 10)
	ch := make(chan reply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	forget := func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}
	if err := c.write(map[string]any{"type": "control_request", "request_id": id, "request": request}); err != nil {
		forget()
		return nil, err
	}
	select {
	case r := <-ch:
		return r.payload, r.err
	case <-ctx.Done():
		forget()
		return nil, ctx.Err()
	}
}

// WriteUserMessage sends one user turn.
func (c *Conn) WriteUserMessage(text string) error {
	return c.write(map[string]any{
		"type":               "user",
		"message":            map[string]any{"role": "user", "content": text},
		"parent_tool_use_id": nil,
	})
}

// frame is the union of the three control envelopes.
type frame struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	Response  *struct {
		Subtype   string          `json:"subtype"`
		RequestID string          `json:"request_id"`
		Response  json.RawMessage `json:"response"`
		Error     string          `json:"error"`
	} `json:"response"`
}

// Dispatch handles line if it is a control frame and reports whether it was
// one. Everything else (assistant, result, system, ...) is left to the caller.
func (c *Conn) Dispatch(line []byte) bool {
	var f frame
	if err := json.Unmarshal(line, &f); err != nil {
		return false
	}
	switch f.Type {
	case "control_request":
		c.serve(f.RequestID, f.Request)
	case "control_response":
		if f.Response != nil {
			c.resolve(f.Response.RequestID, f.Response.Subtype, f.Response.Response, f.Response.Error)
		}
	case "control_cancel_request":
		c.mu.Lock()
		cancel := c.inflight[f.RequestID]
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	default:
		return false
	}
	return true
}

func (c *Conn) resolve(id, subtype string, payload json.RawMessage, errText string) {
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch == nil {
		return
	}
	if subtype == "error" {
		if errText == "" {
			errText = "control request failed"
		}
		ch <- reply{err: &ResponseError{Message: errText}}
		return
	}
	ch <- reply{payload: payload}
}

func (c *Conn) serve(id string, request json.RawMessage) {
	var head struct {
		Subtype string `json:"subtype"`
	}
	_ = json.Unmarshal(request, &head)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.inflight[id] = cancel
	c.handlers.Add(1)
	c.mu.Unlock()

	// Handlers may block on a person (a permission prompt), so each runs on
	// its own goroutine and the stdout reader keeps draining.
	go func() {
		defer c.handlers.Done()
		defer func() {
			c.mu.Lock()
			delete(c.inflight, id)
			c.mu.Unlock()
			cancel()
		}()

		var (
			result any
			err    error
		)
		if c.handler == nil {
			err = fmt.Errorf("no handler for control request %q", head.Subtype)
		} else {
			result, err = c.handler(ctx, head.Subtype, request)
		}
		// A withdrawn request, or one outliving the Conn, gets no answer: the
		// CLI has already moved on.
		if ctx.Err() != nil {
			return
		}
		response := map[string]any{"subtype": "success", "request_id": id}
		if err != nil {
			response = map[string]any{"subtype": "error", "request_id": id, "error": err.Error()}
		} else {
			if result == nil {
				result = map[string]any{}
			}
			response["response"] = result
		}
		_ = c.write(map[string]any{"type": "control_response", "response": response})
	}()
}

func (c *Conn) write(v any) error {
	encoded, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ccproto: encode frame: %w", err)
	}
	encoded = append(encoded, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.w.Write(encoded); err != nil {
		return fmt.Errorf("ccproto: write frame: %w", err)
	}
	return nil
}

// Close fails pending requests with ErrClosed, cancels in-flight handlers,
// and waits for them to return. It does not close the writer.
func (c *Conn) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	pending := c.pending
	c.pending = map[string]chan reply{}
	for _, cancel := range c.inflight {
		cancel()
	}
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- reply{err: ErrClosed}
	}
	c.handlers.Wait()
}
