package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tarsncase/internal/config"
	"github.com/devlikebear/tarsncase/internal/tool"
)

type ServerConfig = config.MCPServer

type ServerStatus struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type ToolInfo struct {
	Server      string          `json:"server"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type Client struct {
	servers []ServerConfig
	timeout time.Duration
	reqID   int64
}

func NewClient(servers []ServerConfig) *Client {
	copyServers := make([]ServerConfig, 0, len(servers))
	for _, s := range servers {
		name := strings.TrimSpace(s.Name)
		command := strings.TrimSpace(s.Command)
		if name == "" || command == "" {
			continue
		}
		copied := ServerConfig{
			Name:    name,
			Command: command,
			Args:    append([]string(nil), s.Args...),
		}
		if len(s.Env) > 0 {
			copied.Env = make(map[string]string, len(s.Env))
			for k, v := range s.Env {
				copied.Env[k] = v
			}
		}
		copyServers = append(copyServers, copied)
	}
	return &Client{
		servers: copyServers,
		timeout: 15 * time.Second,
	}
}

func (c *Client) ListServers(ctx context.Context) ([]ServerStatus, error) {
	if c == nil || len(c.servers) == 0 {
		return []ServerStatus{}, nil
	}
	statuses := make([]ServerStatus, 0, len(c.servers))
	for _, server := range c.servers {
		tools, err := c.listToolsForServer(ctx, server)
		status := ServerStatus{
			Name:    server.Name,
			Command: server.Command,
		}
		if err != nil {
			status.Error = err.Error()
		} else {
			status.Connected = true
			status.ToolCount = len(tools)
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	if c == nil || len(c.servers) == 0 {
		return []ToolInfo{}, nil
	}
	var out []ToolInfo
	var failed int
	for _, server := range c.servers {
		tools, err := c.listToolsForServer(ctx, server)
		if err != nil {
			failed++
			continue
		}
		out = append(out, tools...)
	}
	if len(out) == 0 && failed > 0 {
		return nil, fmt.Errorf("all configured mcp servers failed")
	}
	return out, nil
}

func (c *Client) BuildTools(ctx context.Context) ([]tool.Tool, error) {
	if c == nil || len(c.servers) == 0 {
		return nil, nil
	}
	infos, err := c.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tool.Tool, 0, len(infos))
	for _, info := range infos {
		infoCopy := info
		out = append(out, tool.Tool{
			Name:        MCPToolName(infoCopy.Server, infoCopy.Name),
			Description: strings.TrimSpace(infoCopy.Description),
			Parameters:  cloneRawOrDefault(infoCopy.InputSchema),
			Execute: func(ctx context.Context, params json.RawMessage) (tool.Result, error) {
				args := map[string]any{}
				if trimmed := strings.TrimSpace(string(params)); trimmed != "" && trimmed != "null" {
					if err := json.Unmarshal(params, &args); err != nil {
						return tool.Result{
							Content: []tool.ContentBlock{{Type: "text", Text: fmt.Sprintf(`{"message":"invalid mcp tool args: %v"}`, err)}},
							IsError: true,
						}, nil
					}
				}
				result, err := c.callTool(ctx, infoCopy.Server, infoCopy.Name, args)
				if err != nil {
					return tool.Result{
						Content: []tool.ContentBlock{{Type: "text", Text: fmt.Sprintf(`{"message":"mcp tool call failed: %v"}`, err)}},
						IsError: true,
					}, nil
				}
				return result, nil
			},
		})
	}
	return out, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type session struct {
	stdin  io.WriteCloser
	reader *bufio.Reader
}

func (c *Client) listToolsForServer(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	var tools []ToolInfo
	err := c.withSession(ctx, server, func(ctx context.Context, sess *session) error {
		if err := c.initializeSession(ctx, sess); err != nil {
			return err
		}
		result, err := c.request(ctx, sess, "tools/list", map[string]any{})
		if err != nil {
			return err
		}
		var payload struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
		}
		if err := json.Unmarshal(result, &payload); err != nil {
			return fmt.Errorf("decode tools/list result: %w", err)
		}
		for _, t := range payload.Tools {
			if strings.TrimSpace(t.Name) == "" {
				continue
			}
			tools = append(tools, ToolInfo{
				Server:      server.Name,
				Name:        strings.TrimSpace(t.Name),
				Description: strings.TrimSpace(t.Description),
				InputSchema: cloneRawOrDefault(t.InputSchema),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func (c *Client) callTool(ctx context.Context, serverName, toolName string, args map[string]any) (tool.Result, error) {
	server, ok := c.findServer(serverName)
	if !ok {
		return tool.Result{}, fmt.Errorf("mcp server not found: %s", serverName)
	}

	var output tool.Result
	err := c.withSession(ctx, server, func(ctx context.Context, sess *session) error {
		if err := c.initializeSession(ctx, sess); err != nil {
			return err
		}
		result, err := c.request(ctx, sess, "tools/call", map[string]any{
			"name":      toolName,
			"arguments": args,
		})
		if err != nil {
			return err
		}
		var payload struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		}
		if err := json.Unmarshal(result, &payload); err != nil {
			text := strings.TrimSpace(string(result))
			if text == "" {
				text = "{}"
			}
			output = tool.Result{
				Content: []tool.ContentBlock{{Type: "text", Text: text}},
				IsError: false,
			}
			return nil
		}

		blocks := make([]tool.ContentBlock, 0, len(payload.Content))
		for _, c := range payload.Content {
			if c.Type == "" {
				c.Type = "text"
			}
			if c.Type != "text" {
				continue
			}
			blocks = append(blocks, tool.ContentBlock{Type: "text", Text: c.Text})
		}
		if len(blocks) == 0 {
			blocks = append(blocks, tool.ContentBlock{Type: "text", Text: "{}"})
		}
		output = tool.Result{
			Content: blocks,
			IsError: payload.IsError,
		}
		return nil
	})
	if err != nil {
		return tool.Result{}, err
	}
	return output, nil
}

func (c *Client) withSession(ctx context.Context, server ServerConfig, fn func(context.Context, *session) error) error {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, server.Command, server.Args...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	if len(server.Env) > 0 {
		cmd.Env = append(os.Environ(), commandEnv(server.Env)...)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mcp server %s: %w", server.Name, err)
	}

	runErr := fn(callCtx, &session{
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
	})
	_ = stdin.Close()
	_ = cmd.Wait()
	return runErr
}

func commandEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(extra))
	for k, v := range extra {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		pairs = append(pairs, key+"="+v)
	}
	return pairs
}

func (c *Client) initializeSession(ctx context.Context, sess *session) error {
	_, err := c.request(ctx, sess, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "tarsd",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}
	return c.notify(ctx, sess, "notifications/initialized", map[string]any{})
}

func (c *Client) request(ctx context.Context, sess *session, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.reqID, 1)
	if err := writeRPCMessage(sess.stdin, rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		data, err := readRPCMessage(sess.reader)
		if err != nil {
			return nil, err
		}
		var resp rpcResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp rpc error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(ctx context.Context, sess *session, method string, params any) error {
	_ = ctx
	return writeRPCMessage(sess.stdin, rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func writeRPCMessage(w io.Writer, req rpcRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal rpc request: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("write rpc header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write rpc body: %w", err)
	}
	return nil
}

func readRPCMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read rpc header line: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			raw := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
			n, err := strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid content-length header: %q", line)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing content-length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read rpc body: %w", err)
	}
	return body, nil
}

func (c *Client) findServer(name string) (ServerConfig, bool) {
	for _, server := range c.servers {
		if server.Name == name {
			return server, true
		}
	}
	return ServerConfig{}, false
}

func MCPToolName(serverName, toolName string) string {
	return "mcp." + sanitizeToken(serverName) + "." + sanitizeToken(toolName)
}

func sanitizeToken(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "unknown"
	}
	return out
}

func cloneRawOrDefault(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)
	}
	return append(json.RawMessage(nil), raw...)
}
