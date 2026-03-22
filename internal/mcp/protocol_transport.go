package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/devlikebear/tars/internal/config"
)

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
	pooled    *pooledSession
	stdin     io.WriteCloser
	reader    *bufio.Reader
	abort     func()
	mode      rpcMode
	transport string
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
	switch sess.transport {
	case config.MCPTransportStreamableHTTP:
		return c.requestStreamableHTTP(ctx, sess, req)
	case config.MCPTransportSSE:
		return c.requestLegacySSE(ctx, sess, req)
	case config.MCPTransportWebSocket:
		return c.requestWebSocket(ctx, sess, req)
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
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	switch sess.transport {
	case config.MCPTransportStreamableHTTP:
		return c.notifyStreamableHTTP(ctx, sess, req)
	case config.MCPTransportSSE:
		return c.notifyLegacySSE(ctx, sess, req)
	case config.MCPTransportWebSocket:
		return c.notifyWebSocket(ctx, sess, req)
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
