package toolbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/pkg/tools"
)

// maxRequestBytes bounds one JSON-RPC request body.
const maxRequestBytes = 8 << 20

// protocolVersions are the MCP revisions the bridge answers, newest first.
// A client asking for one of them gets it back; any other gets the newest.
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// ServeHTTP answers one MCP request for the turn its bearer token opens.
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	turn := b.turnFor(r)
	if turn == nil {
		// No WWW-Authenticate: there is no sign-in to send a client to.
		http.Error(w, "unknown or closed turn", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		// GET would open a server-to-client stream and DELETE would end a
		// session; the bridge has neither.
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "request body could not be read"}})
		return
	}
	requests, batch, err := decodeRequests(body)
	if err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: rpcParseError, Message: "invalid JSON-RPC message"}})
		return
	}
	// A call stops when the provider gives up on the request, and when the
	// turn itself ends.
	ctx, cancel := context.WithCancel(turn.Context)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()

	responses := make([]rpcResponse, 0, len(requests))
	for _, req := range requests {
		if resp, ok := turn.handle(ctx, req); ok {
			responses = append(responses, resp)
		}
	}
	if len(responses) == 0 {
		// Only notifications (or responses): nothing to answer.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if batch {
		writeRPC(w, responses)
		return
	}
	writeRPC(w, responses[0])
}

func decodeRequests(body []byte) ([]rpcRequest, bool, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, false, fmt.Errorf("empty body")
	}
	if trimmed[0] == '[' {
		var many []rpcRequest
		if err := json.Unmarshal(trimmed, &many); err != nil {
			return nil, true, err
		}
		if len(many) == 0 {
			return nil, true, fmt.Errorf("empty batch")
		}
		return many, true, nil
	}
	var one rpcRequest
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return nil, false, err
	}
	return []rpcRequest{one}, false, nil
}

func writeRPC(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

// handle answers one message. ok is false for a notification, which gets no
// answer.
func (t *openTurn) handle(ctx context.Context, req rpcRequest) (rpcResponse, bool) {
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return rpcResponse{}, false
	}
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	fail := func(code int, message string) (rpcResponse, bool) {
		resp.Error = &rpcError{Code: code, Message: message}
		return resp, true
	}
	switch req.Method {
	case "initialize":
		resp.Result = t.initialize(req.Params)
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": t.listTools()}
	case "tools/call":
		result, rpcErr := t.callTool(ctx, req.Params)
		if rpcErr != nil {
			return fail(rpcErr.Code, rpcErr.Message)
		}
		resp.Result = result
	case "":
		return fail(rpcInvalidRequest, "missing method")
	default:
		return fail(rpcMethodNotFound, "method not found: "+req.Method)
	}
	return resp, true
}

func (t *openTurn) initialize(params json.RawMessage) map[string]any {
	var in struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &in)
	version := protocolVersions[0]
	for _, known := range protocolVersions {
		if in.ProtocolVersion == known {
			version = known
		}
	}
	result := map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": ServerName, "version": "1"},
	}
	if text := strings.TrimSpace(t.Instructions); text != "" {
		result["instructions"] = text
	}
	return result
}

type toolDescriptor struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations map[string]any  `json:"annotations,omitempty"`
}

var emptyObjectSchema = json.RawMessage(`{"type":"object","properties":{}}`)

func (t *openTurn) listTools() []toolDescriptor {
	sorted := t.sortedTools()
	out := make([]toolDescriptor, 0, len(sorted))
	for _, tl := range sorted {
		schema := json.RawMessage(bytes.TrimSpace(tl.Parameters))
		if len(schema) == 0 || !json.Valid(schema) {
			schema = emptyObjectSchema
		}
		desc := toolDescriptor{Name: tl.Name, Description: tl.Description, InputSchema: schema}
		// A provider in a read-only mode (Claude Code's plan mode) lets a
		// tool through only when the server says it changes nothing.
		if tools.RecoveryPolicyForTool(tl).EffectClass == tools.ToolEffectReadOnly {
			desc.Annotations = map[string]any{"readOnlyHint": true}
		}
		out = append(out, desc)
	}
	return out
}

func (t *openTurn) callTool(ctx context.Context, params json.RawMessage) (map[string]any, *rpcError) {
	var in struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "invalid tools/call params"}
	}
	tl, ok := t.byName[strings.TrimSpace(in.Name)]
	if !ok {
		return nil, &rpcError{Code: rpcInvalidParams, Message: "unknown tool: " + in.Name}
	}
	args := json.RawMessage(bytes.TrimSpace(in.Arguments))
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage(`{}`)
	}

	t.calls.Lock()
	defer t.calls.Unlock()
	if t.Busy != nil {
		if done := t.Busy(); done != nil {
			defer done()
		}
	}
	text, isError := t.execute(ctx, tl, args)
	if t.Redact != nil {
		text = t.Redact(text)
	}
	if t.AfterCall != nil {
		t.AfterCall(ctx, Call{Name: tl.Name, Args: string(args), Result: text, IsError: isError})
	}
	// A tool's own failure is a result the model reads, not a protocol error.
	result := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	if isError {
		result["isError"] = true
	}
	return result, nil
}

func (t *openTurn) execute(ctx context.Context, tl tools.Tool, args json.RawMessage) (text string, isError bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			text, isError = fmt.Sprintf("tool %s failed: %v", tl.Name, recovered), true
		}
	}()
	result, err := tl.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("tool %s failed: %v", tl.Name, err), true
	}
	return result.Text(), result.IsError
}
