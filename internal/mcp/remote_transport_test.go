package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

func TestListTools_StreamableHTTP_UsesBearerAuthAndSessionHeader(t *testing.T) {
	t.Setenv("MCP_HTTP_TOKEN", "secret-token")
	var (
		mu          sync.Mutex
		sawInitAuth string
		sawToolAuth string
		sawSession  string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		switch req.Method {
		case "initialize":
			sawInitAuth = r.Header.Get("Authorization")
			w.Header().Set("Mcp-Session-Id", "sess-123")
			writeJSONResponse(t, w, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"capabilities":{}}`),
			})
		case "tools/list":
			sawToolAuth = r.Header.Get("Authorization")
			sawSession = r.Header.Get("Mcp-Session-Id")
			writeJSONResponse(t, w, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: json.RawMessage(`{
				  "tools":[
				    {"name":"read_file","description":"Read a file","inputSchema":{"type":"object"}}
				  ]
				}`),
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
	}))
	defer srv.Close()

	client := NewClient([]ServerConfig{{
		Name:         "remote-http",
		Transport:    "streamable_http",
		URL:          srv.URL,
		AuthMode:     "bearer",
		AuthTokenEnv: "MCP_HTTP_TOKEN",
	}})

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "read_file" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	if sawInitAuth != "Bearer secret-token" || sawToolAuth != "Bearer secret-token" {
		t.Fatalf("expected bearer auth header on both requests, got init=%q tools=%q", sawInitAuth, sawToolAuth)
	}
	if sawSession != "sess-123" {
		t.Fatalf("expected session header on tools/list, got %q", sawSession)
	}
}

func TestDoHTTPRPC_UsesSSEAcceptOnlyWhenRequested(t *testing.T) {
	var accepts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accepts = append(accepts, r.Header.Get("Accept"))
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSONResponse(t, w, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{}`),
		})
	}))
	defer srv.Close()

	client := NewClient(nil)
	ps := &pooledSession{
		server:     ServerConfig{Name: "remote-http", URL: srv.URL},
		httpClient: srv.Client(),
	}
	req := rpcRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}
	if _, err := client.doHTTPRPC(context.Background(), ps, srv.URL, req, true); err != nil {
		t.Fatalf("streamable http rpc: %v", err)
	}
	req.ID = 2
	if _, err := client.doHTTPRPC(context.Background(), ps, srv.URL, req, false); err != nil {
		t.Fatalf("plain http rpc: %v", err)
	}

	if len(accepts) != 2 {
		t.Fatalf("expected two requests, got accepts=%v", accepts)
	}
	if accepts[0] != "application/json, text/event-stream" {
		t.Fatalf("expected streamable request to accept SSE, got %q", accepts[0])
	}
	if accepts[1] != "application/json" {
		t.Fatalf("expected plain request to accept JSON only, got %q", accepts[1])
	}
}

func TestBuildTools_StreamableHTTPExecutesRemoteTool(t *testing.T) {
	var (
		mu        sync.Mutex
		acceptBy  = map[string]string{}
		callParam map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		acceptBy[req.Method] = r.Header.Get("Accept")
		if req.Method == "tools/call" {
			if params, ok := req.Params.(map[string]any); ok {
				callParam = params
			}
		}
		mu.Unlock()

		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-build-tools")
			writeJSONResponse(t, w, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"capabilities":{}}`),
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeJSONResponse(t, w, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: json.RawMessage(`{
				  "tools":[
				    {"name":"echo","description":"Echo a value","inputSchema":{"type":"object","properties":{"value":{"type":"string"}}}}
				  ]
				}`),
			})
		case "tools/call":
			writeJSONResponse(t, w, rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  json.RawMessage(`{"content":[{"type":"text","text":"called"}],"isError":false}`),
			})
		default:
			t.Fatalf("unexpected method: %s", req.Method)
		}
	}))
	defer srv.Close()

	client := NewClient([]ServerConfig{{
		Name:      "remote-http",
		Transport: "streamable_http",
		URL:       srv.URL,
	}})

	tools, err := client.BuildTools(context.Background())
	if err != nil {
		t.Fatalf("build tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "mcp.remote-http.echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{"value":"hi"}`))
	if err != nil {
		t.Fatalf("execute remote tool: %v", err)
	}
	if result.IsError || len(result.Content) != 1 || result.Content[0].Text != "called" {
		t.Fatalf("unexpected tool result: %+v", result)
	}

	mu.Lock()
	defer mu.Unlock()
	if acceptBy["notifications/initialized"] != "application/json" {
		t.Fatalf("expected notification to accept JSON only, got %q", acceptBy["notifications/initialized"])
	}
	for _, method := range []string{"initialize", "tools/list", "tools/call"} {
		if acceptBy[method] != "application/json, text/event-stream" {
			t.Fatalf("expected %s to accept SSE responses, got %q", method, acceptBy[method])
		}
	}
	if callParam["name"] != "echo" {
		t.Fatalf("expected tool call name echo, got params=%v", callParam)
	}
}

func TestListTools_WebSocket_UsesOAuthAuthHeader(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-secret")
	upgrader := websocket.Upgrader{}
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req rpcRequest
			if err := json.Unmarshal(data, &req); err != nil {
				t.Fatalf("decode websocket request: %v", err)
			}
			var result json.RawMessage
			switch req.Method {
			case "initialize":
				result = json.RawMessage(`{"capabilities":{}}`)
			case "notifications/initialized":
				continue
			case "tools/list":
				result = json.RawMessage(`{
				  "tools":[
				    {"name":"search","description":"Search remote docs","inputSchema":{"type":"object"}}
				  ]
				}`)
			default:
				t.Fatalf("unexpected websocket method: %s", req.Method)
			}
			if err := conn.WriteJSON(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
				t.Fatalf("write websocket response: %v", err)
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient([]ServerConfig{{
		Name:          "remote-ws",
		Transport:     "websocket",
		URL:           wsURL,
		AuthMode:      "oauth",
		OAuthProvider: "claude-code",
	}})

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools over websocket: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "search" {
		t.Fatalf("unexpected websocket tools: %+v", tools)
	}
	if sawAuth != "Bearer oauth-secret" {
		t.Fatalf("expected oauth bearer header, got %q", sawAuth)
	}
}

func TestListTools_LegacySSE_UsesEndpointStream(t *testing.T) {
	t.Setenv("MCP_SSE_TOKEN", "sse-secret")
	var (
		streamMu   sync.Mutex
		streamW    http.ResponseWriter
		streamF    http.Flusher
		getAuth    string
		postAuth   string
		streamOpen = make(chan struct{})
		openOnce   sync.Once
		done       = make(chan struct{})
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sse":
			getAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("expected flusher")
			}
			streamMu.Lock()
			streamW = w
			streamF = flusher
			streamMu.Unlock()
			fmt.Fprint(w, "event: endpoint\ndata: /messages\n\n")
			flusher.Flush()
			openOnce.Do(func() { close(streamOpen) })
			<-done
		case "/messages":
			<-streamOpen
			postAuth = r.Header.Get("Authorization")
			var req rpcRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode legacy sse request: %v", err)
			}
			if req.Method == "notifications/initialized" {
				w.WriteHeader(http.StatusAccepted)
				return
			}
			var result json.RawMessage
			switch req.Method {
			case "initialize":
				result = json.RawMessage(`{"capabilities":{}}`)
			case "tools/list":
				result = json.RawMessage(`{
				  "tools":[
				    {"name":"legacy_read","description":"Read through sse","inputSchema":{"type":"object"}}
				  ]
				}`)
			default:
				t.Fatalf("unexpected legacy sse method: %s", req.Method)
			}
			payload, err := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
			if err != nil {
				t.Fatalf("marshal legacy sse response: %v", err)
			}
			streamMu.Lock()
			if streamW == nil || streamF == nil {
				streamMu.Unlock()
				t.Fatalf("legacy sse stream writer not ready")
			}
			fmt.Fprintf(streamW, "event: message\ndata: %s\n\n", payload)
			streamF.Flush()
			streamMu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer close(done)

	client := NewClient([]ServerConfig{{
		Name:         "legacy-sse",
		Transport:    "sse",
		URL:          srv.URL + "/sse",
		AuthMode:     "bearer",
		AuthTokenEnv: "MCP_SSE_TOKEN",
	}})

	server := client.serverSnapshot()[0]
	tools, err := client.listToolsForServer(context.Background(), server)
	if err != nil {
		t.Fatalf("list tools over legacy sse: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "legacy_read" {
		t.Fatalf("unexpected legacy sse tools: %+v", tools)
	}
	if getAuth != "Bearer sse-secret" || postAuth != "Bearer sse-secret" {
		t.Fatalf("expected bearer auth on GET/POST, got get=%q post=%q", getAuth, postAuth)
	}
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
