package toolbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/tools"
)

type ctxKey struct{}

func textResult(text string) tools.Result {
	return tools.Result{Content: []tools.ContentBlock{{Type: "text", Text: text}}}
}

func testTools() []tools.Tool {
	return []tools.Tool{
		{
			Name:        "memory_search",
			Description: "search memory",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			Execute: func(ctx context.Context, params json.RawMessage) (tools.Result, error) {
				var in struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal(params, &in); err != nil {
					return tools.Result{}, err
				}
				session, _ := ctx.Value(ctxKey{}).(string)
				return textResult("hit for " + in.Query + " in " + session), nil
			},
		},
		{
			Name: "cron_create",
			Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{Content: []tools.ContentBlock{{Type: "text", Text: "bad schedule"}}, IsError: true}, nil
			},
		},
		{
			Name: "broken",
			Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
				return tools.Result{}, errors.New("boom")
			},
		},
		{
			Name:    "panics",
			Execute: func(context.Context, json.RawMessage) (tools.Result, error) { panic("nil map") },
		},
		{
			Name:    "leaky",
			Execute: func(context.Context, json.RawMessage) (tools.Result, error) { return textResult("token=hunter2"), nil },
		},
	}
}

func openTestTurn(t *testing.T, turn Turn) (*Bridge, Endpoint, func()) {
	t.Helper()
	bridge := New()
	t.Cleanup(func() { _ = bridge.Close() })
	if turn.Tools == nil {
		turn.Tools = testTools()
	}
	if turn.Context == nil {
		turn.Context = context.WithValue(context.Background(), ctxKey{}, "session-1")
	}
	endpoint, done, err := bridge.Open(turn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return bridge, endpoint, done
}

func post(t *testing.T, endpoint Endpoint, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, endpoint.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func call(t *testing.T, endpoint Endpoint, body string) rpcReply {
	t.Helper()
	status, data := post(t, endpoint, endpoint.Token, body)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, data)
	}
	var reply rpcReply
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return reply
}

type callResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func callTool(t *testing.T, endpoint Endpoint, name, args string) callResult {
	t.Helper()
	reply := call(t, endpoint, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)
	if reply.Error != nil {
		t.Fatalf("rpc error: %+v", reply.Error)
	}
	var result callResult
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("content = %+v", result.Content)
	}
	return result
}

func TestInitializeEchoesKnownVersionAndSendsInstructions(t *testing.T) {
	_, endpoint, _ := openTestTurn(t, Turn{Instructions: "  use these  "})
	for asked, want := range map[string]string{
		"2025-03-26": "2025-03-26",
		"2099-01-01": protocolVersions[0],
		"":           protocolVersions[0],
	} {
		reply := call(t, endpoint, `{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"`+asked+`"}}`)
		var result struct {
			ProtocolVersion string `json:"protocolVersion"`
			Capabilities    struct {
				Tools map[string]any `json:"tools"`
			} `json:"capabilities"`
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
			Instructions string `json:"instructions"`
		}
		if err := json.Unmarshal(reply.Result, &result); err != nil {
			t.Fatal(err)
		}
		if result.ProtocolVersion != want {
			t.Errorf("asked %q: version = %q, want %q", asked, result.ProtocolVersion, want)
		}
		if result.Capabilities.Tools == nil || result.ServerInfo.Name != ServerName || result.Instructions != "use these" {
			t.Errorf("asked %q: result = %+v", asked, result)
		}
		if string(reply.ID) != `"a"` {
			t.Errorf("id = %s", reply.ID)
		}
	}
}

func TestToolsListIsSortedWithSchemasAndReadOnlyHint(t *testing.T) {
	_, endpoint, _ := openTestTurn(t, Turn{})
	reply := call(t, endpoint, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var result struct {
		Tools []toolDescriptor `json:"tools"`
	}
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatal(err)
	}
	var names []string
	byName := map[string]toolDescriptor{}
	for _, tl := range result.Tools {
		names = append(names, tl.Name)
		byName[tl.Name] = tl
	}
	if got := strings.Join(names, ","); got != "broken,cron_create,leaky,memory_search,panics" {
		t.Fatalf("names = %s", got)
	}
	if !bytes.Contains(byName["memory_search"].InputSchema, []byte(`"query"`)) {
		t.Errorf("memory_search schema = %s", byName["memory_search"].InputSchema)
	}
	if byName["memory_search"].Annotations["readOnlyHint"] != true {
		t.Errorf("memory_search should be marked read-only: %+v", byName["memory_search"].Annotations)
	}
	if byName["cron_create"].Annotations != nil {
		t.Errorf("cron_create must not be marked read-only: %+v", byName["cron_create"].Annotations)
	}
	// A tool without parameters still needs an object schema.
	if string(byName["cron_create"].InputSchema) != string(emptyObjectSchema) {
		t.Errorf("cron_create schema = %s", byName["cron_create"].InputSchema)
	}
}

func TestToolsCallRunsWithTheTurnContext(t *testing.T) {
	var seen []Call
	busy, released := 0, 0
	_, endpoint, _ := openTestTurn(t, Turn{
		AfterCall: func(_ context.Context, c Call) { seen = append(seen, c) },
		Busy: func() func() {
			busy++
			return func() { released++ }
		},
	})
	result := callTool(t, endpoint, "memory_search", `{"query":"tea"}`)
	if result.IsError || result.Content[0].Text != "hit for tea in session-1" {
		t.Fatalf("result = %+v", result)
	}
	if len(seen) != 1 || seen[0].Name != "memory_search" || seen[0].Args != `{"query":"tea"}` || seen[0].Result != "hit for tea in session-1" || seen[0].IsError {
		t.Fatalf("AfterCall saw %+v", seen)
	}
	if busy != 1 || released != 1 {
		t.Fatalf("busy = %d, released = %d", busy, released)
	}
}

func TestToolFailuresAreResultsNotProtocolErrors(t *testing.T) {
	_, endpoint, _ := openTestTurn(t, Turn{})
	for name, want := range map[string]string{
		"cron_create": "bad schedule",
		"broken":      "tool broken failed: boom",
		"panics":      "tool panics failed: nil map",
	} {
		// No arguments at all is an empty object to the tool.
		reply := call(t, endpoint, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"`+name+`"}}`)
		var result callResult
		if reply.Error != nil || json.Unmarshal(reply.Result, &result) != nil {
			t.Fatalf("%s: reply = %+v", name, reply)
		}
		if !result.IsError || result.Content[0].Text != want {
			t.Errorf("%s: result = %+v", name, result)
		}
	}
}

func TestResultIsRedactedBeforeItLeaves(t *testing.T) {
	var seen Call
	_, endpoint, _ := openTestTurn(t, Turn{
		Redact:    func(s string) string { return strings.ReplaceAll(s, "hunter2", "[redacted]") },
		AfterCall: func(_ context.Context, c Call) { seen = c },
	})
	result := callTool(t, endpoint, "leaky", `{}`)
	if result.Content[0].Text != "token=[redacted]" || seen.Result != "token=[redacted]" {
		t.Fatalf("result = %q, AfterCall = %q", result.Content[0].Text, seen.Result)
	}
}

func TestUnknownToolAndMethodAreProtocolErrors(t *testing.T) {
	_, endpoint, _ := openTestTurn(t, Turn{})
	reply := call(t, endpoint, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec","arguments":{}}}`)
	if reply.Error == nil || reply.Error.Code != rpcInvalidParams {
		t.Fatalf("unknown tool: %+v", reply)
	}
	reply = call(t, endpoint, `{"jsonrpc":"2.0","id":4,"method":"resources/list"}`)
	if reply.Error == nil || reply.Error.Code != rpcMethodNotFound {
		t.Fatalf("unknown method: %+v", reply)
	}
	reply = call(t, endpoint, `{"jsonrpc":"2.0","id":5,"method":"ping"}`)
	if reply.Error != nil || string(reply.Result) != `{}` {
		t.Fatalf("ping: %+v", reply)
	}
	status, data := post(t, endpoint, endpoint.Token, `{not json`)
	var parse rpcReply
	if status != http.StatusOK || json.Unmarshal(data, &parse) != nil || parse.Error == nil || parse.Error.Code != rpcParseError {
		t.Fatalf("bad json: %d %s", status, data)
	}
}

func TestNotificationsGetNoAnswerAndBatchesAnswerRequestsOnly(t *testing.T) {
	_, endpoint, _ := openTestTurn(t, Turn{})
	status, data := post(t, endpoint, endpoint.Token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if status != http.StatusAccepted || len(bytes.TrimSpace(data)) != 0 {
		t.Fatalf("notification: %d %q", status, data)
	}
	status, data = post(t, endpoint, endpoint.Token, `[{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":9,"method":"ping"}]`)
	var replies []rpcReply
	if status != http.StatusOK || json.Unmarshal(data, &replies) != nil || len(replies) != 1 || string(replies[0].ID) != "9" {
		t.Fatalf("batch: %d %s", status, data)
	}
}

func TestOnlyALiveTokenIsServed(t *testing.T) {
	bridge, endpoint, done := openTestTurn(t, Turn{})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	for name, token := range map[string]string{"none": "", "wrong": strings.Repeat("0", 64), "prefix": endpoint.Token[:32]} {
		if status, _ := post(t, endpoint, token, body); status != http.StatusUnauthorized {
			t.Errorf("%s token: status %d", name, status)
		}
	}
	// A second turn has its own token and its own tools.
	other, otherDone, err := bridge.Open(Turn{Tools: testTools()[:1]})
	if err != nil {
		t.Fatal(err)
	}
	defer otherDone()
	if other.Token == endpoint.Token || other.URL != endpoint.URL {
		t.Fatalf("second turn endpoint = %+v, first = %+v", other, endpoint)
	}
	reply := call(t, other, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"cron_create"}}`)
	if reply.Error == nil {
		t.Fatalf("the second turn reached a tool it was not given: %+v", reply)
	}

	done()
	if status, _ := post(t, endpoint, endpoint.Token, body); status != http.StatusUnauthorized {
		t.Fatalf("closed turn still answers: %d", status)
	}
	if status, _ := post(t, other, other.Token, body); status != http.StatusOK {
		t.Fatalf("closing one turn closed another: %d", status)
	}
}

func TestOnlyPostOnTheMCPPath(t *testing.T) {
	_, endpoint, _ := openTestTurn(t, Turn{})
	req, _ := http.NewRequest(http.MethodGet, endpoint.URL, nil)
	req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != http.MethodPost {
		t.Fatalf("GET: %d Allow=%q", resp.StatusCode, resp.Header.Get("Allow"))
	}
	elsewhere := endpoint
	elsewhere.URL = strings.TrimSuffix(endpoint.URL, "/mcp") + "/other"
	if status, _ := post(t, elsewhere, endpoint.Token, `{}`); status != http.StatusNotFound {
		t.Fatalf("other path: %d", status)
	}
}

func TestACallStopsWhenTheTurnEnds(t *testing.T) {
	turnCtx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	_, endpoint, _ := openTestTurn(t, Turn{
		Context: turnCtx,
		Tools: []tools.Tool{{
			Name: "waits",
			Execute: func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
				close(started)
				<-ctx.Done()
				return tools.Result{}, ctx.Err()
			},
		}},
	})
	go func() {
		<-started
		cancel()
	}()
	result := callTool(t, endpoint, "waits", `{}`)
	if !result.IsError || !strings.Contains(result.Content[0].Text, "context canceled") {
		t.Fatalf("result = %+v", result)
	}
}

func TestOpenRefusesNothingToServeAndAClosedBridge(t *testing.T) {
	bridge := New()
	if _, _, err := bridge.Open(Turn{}); err == nil {
		t.Fatal("Open with no tools should fail")
	}
	if _, _, err := bridge.Open(Turn{Tools: []tools.Tool{{Name: "no_execute"}}}); err == nil {
		t.Fatal("Open with no callable tool should fail")
	}
	var missing *Bridge
	if _, _, err := missing.Open(Turn{Tools: testTools()}); err == nil {
		t.Fatal("a nil bridge should fail to open")
	}
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := bridge.Open(Turn{Tools: testTools()}); err == nil {
		t.Fatal("Open after Close should fail")
	}
}
