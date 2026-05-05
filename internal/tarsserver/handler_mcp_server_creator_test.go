package tarsserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/rs/zerolog"
)

func TestMCPServerCreatorAPI_DraftAndSaveLocal(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	handler := newMCPServerCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil)

	reqBody := map[string]any{
		"name":        "safe-time",
		"description": "Expose safe local time utilities.",
		"language":    "python",
		"use_case":    "현재 시간을 읽어서 로컬 타임존 기준 문자열로 반환한다",
	}
	draftRec := postJSON(t, handler, "/v1/admin/mcp-servers/draft", reqBody)
	if draftRec.Code != http.StatusOK {
		t.Fatalf("expected draft 200, got %d body=%q", draftRec.Code, draftRec.Body.String())
	}

	var draft mcpServerCreatorDraftResponse
	if err := json.Unmarshal(draftRec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if draft.Name != "safe-time" || draft.Language != "python" {
		t.Fatalf("unexpected draft metadata: %+v", draft)
	}
	if len(draft.Tools) != 1 || draft.Tools[0].Name == "" {
		t.Fatalf("expected inferred tool signature, got %+v", draft.Tools)
	}
	if !mcpDraftContainsFile(draft.Files, "tars.mcp.json", `"command": "python3"`) {
		t.Fatalf("expected python manifest, files=%+v", draft.Files)
	}
	if !mcpDraftContainsFile(draft.Files, "server.py", `mcp.run(transport="stdio")`) {
		t.Fatalf("expected FastMCP stdio server, files=%+v", draft.Files)
	}

	saveRec := postJSON(t, handler, "/v1/admin/mcp-servers/save-local", draft)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected save 200, got %d body=%q", saveRec.Code, saveRec.Body.String())
	}
	var saved mcpServerCreatorSaveResponse
	if err := json.Unmarshal(saveRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if !saved.Saved || saved.Path == "" || len(saved.Files) == 0 {
		t.Fatalf("unexpected save response: %+v", saved)
	}
	manifestPath := filepath.Join(workspaceDir, "mcp-servers", "safe-time", "tars.mcp.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected saved manifest: %v", err)
	}
}

func TestMCPServerCreatorAPI_DraftsFromNaturalLanguageWithLLM(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	router, clients, err := llm.NewFakeRouter(llm.TierStandard, map[llm.Role]llm.Tier{
		llm.RoleAgentRuntimePlanner: llm.TierStandard,
	})
	if err != nil {
		t.Fatalf("fake router: %v", err)
	}
	clients[llm.TierStandard].ChatResponse = llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: `{
			"name": "hello-world",
			"description": "Respond to friendly hello requests.",
			"language": "node",
			"use_case": "When called, reply with a friendly Korean greeting.",
			"tools": [
				{"name": "say_hello", "description": "Return a friendly Korean greeting."}
			],
			"assistant_message": "Node MCP draft is ready for stdio validation."
		}`},
	}
	handler := newMCPServerCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil, router)

	draftRec := postJSON(t, handler, "/v1/admin/mcp-servers/draft", map[string]any{
		"prompt":   "hello world MCP 서버를 만들어줘. 호출하면 안녕이라고 응답하게 해줘.",
		"language": "node",
	})
	if draftRec.Code != http.StatusOK {
		t.Fatalf("expected draft 200, got %d body=%q", draftRec.Code, draftRec.Body.String())
	}

	var draft mcpServerCreatorDraftResponse
	if err := json.Unmarshal(draftRec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if clients[llm.TierStandard].ChatCalls != 1 {
		t.Fatalf("expected one LLM chat call, got %d", clients[llm.TierStandard].ChatCalls)
	}
	if draft.Name != "hello-world" || draft.Language != "node" || draft.DraftSource != "llm" {
		t.Fatalf("unexpected LLM draft metadata: %+v", draft)
	}
	if draft.AssistantMessage == "" {
		t.Fatalf("expected assistant message in LLM draft response")
	}
	if len(draft.Tools) != 1 || draft.Tools[0].Name != "say_hello" {
		t.Fatalf("expected LLM tool signature, got %+v", draft.Tools)
	}
	if !mcpDraftContainsFile(draft.Files, "server.mjs", "await server.connect(new StdioServerTransport())") {
		t.Fatalf("expected runnable Node stdio server, files=%+v", draft.Files)
	}
}

func TestMCPServerCreatorAPI_DraftsFromNaturalLanguageHeuristicWithoutName(t *testing.T) {
	handler := newMCPServerCreatorAPIHandler(filepath.Join(t.TempDir(), "workspace"), zerolog.New(ioDiscard{}), nil)

	draftRec := postJSON(t, handler, "/v1/admin/mcp-servers/draft", map[string]any{
		"prompt":  "Expose a get time tool that returns the current local time as JSON.",
		"use_llm": false,
	})
	if draftRec.Code != http.StatusOK {
		t.Fatalf("expected draft 200, got %d body=%q", draftRec.Code, draftRec.Body.String())
	}

	var draft mcpServerCreatorDraftResponse
	if err := json.Unmarshal(draftRec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft: %v", err)
	}
	if draft.Name == "" || draft.Description == "" || draft.UseCase == "" {
		t.Fatalf("expected prompt-derived metadata, got %+v", draft)
	}
	if draft.DraftSource != "heuristic" {
		t.Fatalf("expected heuristic source, got %q", draft.DraftSource)
	}
	if len(draft.Tools) == 0 || draft.Tools[0].Name == "" {
		t.Fatalf("expected prompt-derived tool signature, got %+v", draft.Tools)
	}
}

func TestMCPServerCreatorAPI_RejectsUnsafeNamesAndPaths(t *testing.T) {
	handler := newMCPServerCreatorAPIHandler(filepath.Join(t.TempDir(), "workspace"), zerolog.New(ioDiscard{}), nil)

	badDraft := postJSON(t, handler, "/v1/admin/mcp-servers/draft", map[string]any{
		"name":        "../escape",
		"description": "bad",
		"language":    "node",
		"use_case":    "bad",
	})
	if badDraft.Code != http.StatusBadRequest {
		t.Fatalf("expected unsafe name to fail, got %d body=%q", badDraft.Code, badDraft.Body.String())
	}

	badSave := postJSON(t, handler, "/v1/admin/mcp-servers/save-local", mcpServerCreatorDraftResponse{
		Name: "safe-server",
		Files: []mcpServerCreatorFile{
			{Path: "tars.mcp.json", Content: "{}"},
			{Path: "../escape.py", Content: "print('bad')"},
		},
	})
	if badSave.Code != http.StatusBadRequest {
		t.Fatalf("expected unsafe file path to fail, got %d body=%q", badSave.Code, badSave.Body.String())
	}
}

func TestMCPServerCreatorAPI_TestBootstrapsNodeDependenciesBeforeStdio(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	handler := newMCPServerCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil)

	draft := mcpServerCreatorDraftResponse{
		Name:        "node-probe",
		Description: "Probe node MCP server.",
		Language:    "node",
		UseCase:     "echo probe",
		Tools: []mcpServerCreatorToolSpec{
			{Name: "echo", Description: "Echo request", InputSchema: defaultMCPInputSchema(), OutputSchema: defaultMCPOutputSchema()},
		},
		Files: []mcpServerCreatorFile{
			{Path: "package.json", Content: `{"type":"module","dependencies":{}}`},
			{Path: "tars.mcp.json", Content: `{"schema_version":1,"server":{"name":"node-probe","command":"node","args":["${MCP_DIR}/server.mjs"]}}`},
			{Path: "server.mjs", Content: nodeRawMCPProbeServer()},
		},
	}
	rec := postJSON(t, handler, "/v1/admin/mcp-servers/test", draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected test 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var result mcpServerCreatorTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected successful stdio validation, got %+v", result)
	}
	if len(result.ToolTrail) < 2 || result.ToolTrail[0].Tool != "npm_install" || result.ToolTrail[1].Tool != "mcp_stdio" {
		t.Fatalf("expected npm dependency bootstrap before stdio validation, got %+v", result.ToolTrail)
	}
}

func TestMCPServerCreatorAPI_TestRunsStdioToolsListAndCall(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspace")
	handler := newMCPServerCreatorAPIHandler(workspaceDir, zerolog.New(ioDiscard{}), nil)

	script := `#!/usr/bin/env python3
import json
import sys

def read_message():
    headers = {}
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            return None
        line = line.decode().strip()
        if not line:
            break
        key, value = line.split(":", 1)
        headers[key.lower()] = value.strip()
    size = int(headers["content-length"])
    return json.loads(sys.stdin.buffer.read(size).decode())

def send(result, id):
    data = json.dumps({"jsonrpc":"2.0","id":id,"result":result}).encode()
    sys.stdout.buffer.write(b"Content-Length: " + str(len(data)).encode() + b"\r\n\r\n" + data)
    sys.stdout.buffer.flush()

while True:
    msg = read_message()
    if msg is None:
        break
    method = msg.get("method")
    if method == "notifications/initialized":
        continue
    if method == "initialize":
        send({"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"probe-mcp","version":"0.1.0"}}, msg["id"])
    elif method == "tools/list":
        send({"tools":[{"name":"echo","description":"Echo request","inputSchema":{"type":"object","properties":{"request":{"type":"string"}},"required":["request"]}}]}, msg["id"])
    elif method == "tools/call":
        send({"content":[{"type":"text","text":"mcp-call:" + msg["params"]["arguments"]["request"]}],"isError":False}, msg["id"])
`
	draft := mcpServerCreatorDraftResponse{
		Name:        "probe-mcp",
		Description: "Probe generated MCP server.",
		Language:    "python",
		UseCase:     "echo probe",
		Tools: []mcpServerCreatorToolSpec{
			{Name: "echo", Description: "Echo request", InputSchema: defaultMCPInputSchema(), OutputSchema: defaultMCPOutputSchema()},
		},
		Files: []mcpServerCreatorFile{
			{Path: "tars.mcp.json", Content: `{"schema_version":1,"server":{"name":"probe-mcp","command":"python3","args":["${MCP_DIR}/server.py"]}}`},
			{Path: "server.py", Content: script},
		},
	}
	rec := postJSON(t, handler, "/v1/admin/mcp-servers/test", draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected test 200, got %d body=%q", rec.Code, rec.Body.String())
	}
	var result mcpServerCreatorTestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected successful stdio validation, got %+v", result)
	}
	if !mcpCreatorContainsString(result.Tools, "echo") {
		t.Fatalf("expected tools/list result to include echo, got %+v", result.Tools)
	}
	if !strings.Contains(result.CallResult, "mcp-call:") {
		t.Fatalf("expected tools/call result, got %q", result.CallResult)
	}
	if result.SessionKind != "worker" || !result.Hidden {
		t.Fatalf("expected hidden worker sandbox metadata, got %+v", result)
	}
	if len(result.ProtocolSteps) < 2 || result.ProtocolSteps[0] != "tools/list" || result.ProtocolSteps[1] != "tools/call" {
		t.Fatalf("expected protocol steps, got %+v", result.ProtocolSteps)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "mcp-servers", "probe-mcp")); !os.IsNotExist(err) {
		t.Fatalf("sandbox test should not write into workspace/mcp-servers, err=%v", err)
	}
}

func mcpDraftContainsFile(files []mcpServerCreatorFile, path string, want string) bool {
	for _, file := range files {
		if file.Path == path && strings.Contains(file.Content, want) {
			return true
		}
	}
	return false
}

func mcpCreatorContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func nodeRawMCPProbeServer() string {
	return `#!/usr/bin/env node
import process from 'node:process'

let buffer = Buffer.alloc(0)

function send(id, result) {
  const body = Buffer.from(JSON.stringify({ jsonrpc: '2.0', id, result }))
  process.stdout.write('Content-Length: ' + body.length + '\r\n\r\n')
  process.stdout.write(body)
}

function handle(message) {
  if (message.method === 'notifications/initialized') return
  if (message.method === 'initialize') {
    send(message.id, { protocolVersion: '2024-11-05', capabilities: { tools: {} }, serverInfo: { name: 'node-probe', version: '0.1.0' } })
  } else if (message.method === 'tools/list') {
    send(message.id, { tools: [{ name: 'echo', description: 'Echo request', inputSchema: { type: 'object', properties: { request: { type: 'string' } }, required: ['request'] } }] })
  } else if (message.method === 'tools/call') {
    send(message.id, { content: [{ type: 'text', text: 'node-call:' + message.params.arguments.request }], isError: false })
  }
}

process.stdin.on('data', chunk => {
  buffer = Buffer.concat([buffer, chunk])
  while (true) {
    const split = buffer.indexOf('\r\n\r\n')
    if (split === -1) break
    const headers = buffer.slice(0, split).toString()
    const match = headers.match(/Content-Length:\s*(\d+)/i)
    if (!match) process.exit(1)
    const size = Number(match[1])
    const start = split + 4
    if (buffer.length < start + size) break
    const raw = buffer.slice(start, start + size).toString()
    buffer = buffer.slice(start + size)
    handle(JSON.parse(raw))
  }
})
`
}
