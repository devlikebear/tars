package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

func TestMCPToolName(t *testing.T) {
	got := MCPToolName("Filesystem Server", "read/file")
	if got != "mcp.filesystem_server.read_file" {
		t.Fatalf("unexpected tool name: %q", got)
	}
}

func TestRPCMessageRoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "tools/list",
		Params:  map[string]any{"cursor": ""},
	}
	if err := writeRPCMessage(buf, req); err != nil {
		t.Fatalf("write rpc message: %v", err)
	}

	data, err := readRPCMessage(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("read rpc message: %v", err)
	}
	var decoded rpcRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode rpc request: %v", err)
	}
	if decoded.ID != 7 || decoded.Method != "tools/list" {
		t.Fatalf("unexpected decoded request: %+v", decoded)
	}
}
