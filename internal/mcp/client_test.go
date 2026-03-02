package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
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

func TestRPCJSONLineRoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      9,
		Method:  "initialize",
		Params:  map[string]any{"protocolVersion": "2024-11-05"},
	}
	if err := writeRPCJSONLine(buf, req); err != nil {
		t.Fatalf("write rpc json line: %v", err)
	}

	data, err := readRPCJSONLine(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("read rpc json line: %v", err)
	}
	var decoded rpcRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode rpc request: %v", err)
	}
	if decoded.ID != 9 || decoded.Method != "initialize" {
		t.Fatalf("unexpected decoded request: %+v", decoded)
	}
}

func TestInferRPCMode_SequentialThinkingUsesJSONLine(t *testing.T) {
	mode := inferRPCMode(ServerConfig{
		Name:    "sequential-thinking",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
	})
	if mode != rpcModeJSONLine {
		t.Fatalf("expected json line mode, got %v", mode)
	}
}

func TestShouldFallbackToJSONLine(t *testing.T) {
	cases := []error{
		context.DeadlineExceeded,
		errors.New("missing content-length header"),
		errors.New("read rpc header line: EOF"),
	}
	for _, tc := range cases {
		if !shouldFallbackToJSONLine(tc) {
			t.Fatalf("expected fallback=true for %v", tc)
		}
	}
	if shouldFallbackToJSONLine(errors.New("permission denied")) {
		t.Fatalf("expected fallback=false for unrelated error")
	}
}

func TestSetServers_InferSequentialThinkingMode(t *testing.T) {
	client := NewClient(nil)
	client.SetServers([]ServerConfig{
		{
			Name:    "sequential-thinking",
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
		},
	})
	if got := client.currentMode("sequential-thinking"); got != rpcModeJSONLine {
		t.Fatalf("expected json line mode, got %v", got)
	}
}

func TestValidateServerCommands_AllowlistMatch(t *testing.T) {
	client := NewClient([]ServerConfig{
		{
			Name:    "filesystem",
			Command: "/usr/local/bin/npx",
		},
	})
	client.SetCommandAllowlist([]string{"npx"})
	if err := client.validateServerCommands(); err != nil {
		t.Fatalf("expected allowlisted command to pass, got %v", err)
	}
}

func TestValidateServerCommands_AllowlistMismatch(t *testing.T) {
	client := NewClient([]ServerConfig{
		{
			Name:    "filesystem",
			Command: "npx",
		},
	})
	client.SetCommandAllowlist([]string{"uvx"})
	err := client.validateServerCommands()
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "mcp_command_allowlist_json") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestValidateServerCommands_EmptyAllowlistBlocksCommands(t *testing.T) {
	client := NewClient([]ServerConfig{
		{
			Name:    "filesystem",
			Command: "npx",
		},
	})
	client.SetCommandAllowlist(nil)
	if err := client.validateServerCommands(); err == nil {
		t.Fatalf("expected empty allowlist to block command")
	}
}

type testWriteCloser struct {
	io.Writer
}

func (w testWriteCloser) Close() error {
	return nil
}

func TestRequestTimeoutAbortsBlockingRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	aborted := false
	sess := &session{
		stdin:  testWriteCloser{Writer: io.Discard},
		reader: bufio.NewReader(reader),
		abort: func() {
			aborted = true
			_ = reader.Close()
			_ = writer.Close()
		},
	}

	client := &Client{}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	_, err := client.request(ctx, sess, "tools/list", map[string]any{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !aborted {
		t.Fatalf("expected abort callback to be called on timeout")
	}
}
