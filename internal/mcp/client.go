package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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
	mu          sync.RWMutex
	servers     []ServerConfig
	sessions    map[string]*pooledSession
	serverModes map[string]rpcMode
	timeout     time.Duration
	reqID       int64
}

type pooledSession struct {
	server      ServerConfig
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	reader      *bufio.Reader
	mode        rpcMode
	mu          sync.Mutex
	initialized bool
	closed      bool
}

type rpcMode int

const (
	rpcModeContentLength rpcMode = iota
	rpcModeJSONLine
)

func (m rpcMode) String() string {
	switch m {
	case rpcModeJSONLine:
		return "jsonline"
	default:
		return "content-length"
	}
}

func NewClient(servers []ServerConfig) *Client {
	client := &Client{
		timeout:     15 * time.Second,
		sessions:    map[string]*pooledSession{},
		serverModes: map[string]rpcMode{},
	}
	client.SetServers(servers)
	return client
}

func (c *Client) SetServers(servers []ServerConfig) {
	normalized := normalizeServers(servers)
	c.mu.Lock()
	defer c.mu.Unlock()

	nextNames := map[string]struct{}{}
	for _, server := range normalized {
		nextNames[server.Name] = struct{}{}
		inferred := inferRPCMode(server)
		if prev, ok := c.serverModes[server.Name]; ok && prev == rpcModeJSONLine && inferred == rpcModeContentLength {
			continue
		}
		c.serverModes[server.Name] = inferred
	}
	for name, sess := range c.sessions {
		if _, ok := nextNames[name]; ok {
			continue
		}
		sess.close()
		delete(c.sessions, name)
		delete(c.serverModes, name)
	}
	c.servers = normalized
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, sess := range c.sessions {
		sess.close()
		delete(c.sessions, name)
	}
}

func (c *Client) ListServers(ctx context.Context) ([]ServerStatus, error) {
	servers := c.serverSnapshot()
	if len(servers) == 0 {
		return []ServerStatus{}, nil
	}
	statuses := make([]ServerStatus, len(servers))
	var wg sync.WaitGroup
	for i, server := range servers {
		wg.Add(1)
		go func(index int, server ServerConfig) {
			defer wg.Done()
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
			statuses[index] = status
		}(i, server)
	}
	wg.Wait()
	return statuses, nil
}

func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	servers := c.serverSnapshot()
	if len(servers) == 0 {
		return []ToolInfo{}, nil
	}
	type toolResult struct {
		tools []ToolInfo
		err   error
	}
	results := make([]toolResult, len(servers))
	var wg sync.WaitGroup
	for i, server := range servers {
		wg.Add(1)
		go func(index int, server ServerConfig) {
			defer wg.Done()
			tools, err := c.listToolsForServer(ctx, server)
			results[index] = toolResult{tools: tools, err: err}
		}(i, server)
	}
	wg.Wait()

	out := make([]ToolInfo, 0)
	failed := 0
	for _, result := range results {
		if result.err != nil {
			failed++
			continue
		}
		out = append(out, result.tools...)
	}
	if len(out) == 0 && failed > 0 {
		return nil, fmt.Errorf("all configured mcp servers failed")
	}
	return out, nil
}

func (c *Client) BuildTools(ctx context.Context) ([]tool.Tool, error) {
	if len(c.serverSnapshot()) == 0 {
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
	abort  func()
	mode   rpcMode
}

func (s *pooledSession) close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_, _ = s.cmd.Process.Wait()
	}
}

func (c *Client) listToolsForServer(ctx context.Context, server ServerConfig) ([]ToolInfo, error) {
	var tools []ToolInfo
	err := c.withPersistentSession(ctx, server, func(ctx context.Context, sess *session) error {
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
	err := c.withPersistentSession(ctx, server, func(ctx context.Context, sess *session) error {
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

func (c *Client) withPersistentSession(ctx context.Context, server ServerConfig, fn func(context.Context, *session) error) error {
	err := c.withPersistentSessionOnce(ctx, server, fn)
	if err == nil {
		return nil
	}
	switchedMode := false
	if c.currentMode(server.Name) == rpcModeContentLength && shouldFallbackToJSONLine(err) {
		c.setServerMode(server.Name, rpcModeJSONLine)
		switchedMode = true
	}
	c.dropSession(server.Name)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		if switchedMode {
			return c.withPersistentSessionOnce(ctx, server, fn)
		}
		return err
	}
	return c.withPersistentSessionOnce(ctx, server, fn)
}

func (c *Client) withPersistentSessionOnce(ctx context.Context, server ServerConfig, fn func(context.Context, *session) error) error {
	timeout := c.timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ps, err := c.getOrStartSession(server)
	if err != nil {
		return err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed {
		return fmt.Errorf("mcp session closed: %s", server.Name)
	}

	sess := &session{
		stdin:  ps.stdin,
		reader: ps.reader,
		abort:  ps.close,
		mode:   ps.mode,
	}
	if !ps.initialized {
		if err := c.initializeSession(callCtx, sess); err != nil {
			ps.close()
			return fmt.Errorf("initialize mcp session (%s): %w", sess.mode.String(), err)
		}
		ps.initialized = true
	}
	if err := fn(callCtx, sess); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "broken pipe") || strings.Contains(strings.ToLower(err.Error()), "eof") {
			ps.close()
		}
		return fmt.Errorf("mcp request failed (%s): %w", sess.mode.String(), err)
	}
	return nil
}

func (c *Client) getOrStartSession(server ServerConfig) (*pooledSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessions == nil {
		c.sessions = map[string]*pooledSession{}
	}
	if existing, ok := c.sessions[server.Name]; ok && !existing.closed {
		return existing, nil
	}

	cmd := exec.Command(server.Command, server.Args...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	if len(server.Env) > 0 {
		cmd.Env = append(os.Environ(), commandEnv(server.Env)...)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server %s: %w", server.Name, err)
	}
	ps := &pooledSession{
		server: server,
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
		mode:   rpcModeContentLength,
	}
	if mode, ok := c.serverModes[server.Name]; ok {
		ps.mode = mode
	}
	c.sessions[server.Name] = ps
	return ps, nil
}

func (c *Client) dropSession(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sess, ok := c.sessions[name]
	if !ok {
		return
	}
	sess.close()
	delete(c.sessions, name)
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

func inferRPCMode(server ServerConfig) rpcMode {
	if strings.TrimSpace(server.Command) == "mcp-server-sequential-thinking" {
		return rpcModeJSONLine
	}
	joinedArgs := strings.Join(server.Args, " ")
	if strings.Contains(joinedArgs, "@modelcontextprotocol/server-sequential-thinking") {
		return rpcModeJSONLine
	}
	return rpcModeContentLength
}

func shouldFallbackToJSONLine(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "missing content-length header") ||
		strings.Contains(msg, "invalid content-length header") ||
		strings.Contains(msg, "read rpc header line")
}

func (c *Client) currentMode(serverName string) rpcMode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	mode, ok := c.serverModes[serverName]
	if !ok {
		return rpcModeContentLength
	}
	return mode
}

func (c *Client) setServerMode(serverName string, mode rpcMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serverModes == nil {
		c.serverModes = map[string]rpcMode{}
	}
	c.serverModes[serverName] = mode
}

func (c *Client) serverSnapshot() []ServerConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ServerConfig, len(c.servers))
	copy(out, c.servers)
	return out
}

func normalizeServers(servers []ServerConfig) []ServerConfig {
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
	return copyServers
}

func (c *Client) initializeSession(ctx context.Context, sess *session) error {
	_, err := c.request(ctx, sess, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "tars",
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
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	var err error
	switch sess.mode {
	case rpcModeJSONLine:
		err = writeRPCJSONLine(sess.stdin, req)
	default:
		err = writeRPCMessage(sess.stdin, req)
	}
	if err != nil {
		return nil, err
	}
	for {
		type readResult struct {
			data []byte
			err  error
		}
		readCh := make(chan readResult, 1)
		go func() {
			var (
				data []byte
				err  error
			)
			switch sess.mode {
			case rpcModeJSONLine:
				data, err = readRPCJSONLine(sess.reader)
			default:
				data, err = readRPCMessage(sess.reader)
			}
			readCh <- readResult{data: data, err: err}
		}()

		var (
			data []byte
			err  error
		)
		select {
		case <-ctx.Done():
			if sess.abort != nil {
				sess.abort()
			}
			return nil, ctx.Err()
		case result := <-readCh:
			data = result.data
			err = result.err
		}
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
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	if sess.mode == rpcModeJSONLine {
		return writeRPCJSONLine(sess.stdin, req)
	}
	return writeRPCMessage(sess.stdin, req)
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

func writeRPCJSONLine(w io.Writer, req rpcRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal rpc request: %w", err)
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write rpc json line: %w", err)
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

func readRPCJSONLine(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read rpc json line: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		data := []byte(line)
		if !json.Valid(data) {
			continue
		}
		return data, nil
	}
}

func (c *Client) findServer(name string) (ServerConfig, bool) {
	for _, server := range c.serverSnapshot() {
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
