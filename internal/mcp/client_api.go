package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/devlikebear/tarsncase/internal/tool"
)

func (c *Client) ListServers(ctx context.Context) ([]ServerStatus, error) {
	if err := c.validateServerCommands(); err != nil {
		return nil, err
	}
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
	if err := c.validateServerCommands(); err != nil {
		return nil, err
	}
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
	if err := c.validateServerCommands(); err != nil {
		return nil, err
	}
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
