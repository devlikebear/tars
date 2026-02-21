package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

type chatEvent struct {
	Type              string `json:"type"`
	Text              string `json:"text"`
	Error             string `json:"error"`
	SessionID         string `json:"session_id"`
	Message           string `json:"message"`
	Phase             string `json:"phase"`
	ToolName          string `json:"tool_name"`
	ToolCallID        string `json:"tool_call_id"`
	ToolArgsPreview   string `json:"tool_args_preview"`
	ToolResultPreview string `json:"tool_result_preview"`
}

type chatResult struct {
	SessionID string
	Assistant string
}

type chatClient struct {
	serverURL  string
	apiToken   string
	httpClient *http.Client
}

func (c chatClient) stream(ctx context.Context, req chatRequest, onStatus func(chatEvent), onDelta func(string)) (chatResult, error) {
	if strings.TrimSpace(req.Message) == "" {
		return chatResult{}, fmt.Errorf("message is required")
	}
	endpoint, err := resolveURL(c.serverURL, "/v1/chat")
	if err != nil {
		return chatResult{}, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return chatResult{}, err
	}
	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return chatResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.apiToken); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return chatResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		code, message, ok := parseAPIErrorPayload(body)
		if ok {
			return chatResult{}, &apiHTTPError{
				Method:   http.MethodPost,
				Endpoint: endpoint,
				Status:   resp.StatusCode,
				Code:     code,
				Message:  message,
				Body:     strings.TrimSpace(string(body)),
			}
		}
		return chatResult{}, &apiHTTPError{
			Method:   http.MethodPost,
			Endpoint: endpoint,
			Status:   resp.StatusCode,
			Body:     strings.TrimSpace(string(body)),
		}
	}
	result := chatResult{SessionID: strings.TrimSpace(req.SessionID)}
	if err := scanSSELines(resp.Body, func(payload []byte) error {
		var evt chatEvent
		if err := json.Unmarshal(payload, &evt); err != nil {
			return fmt.Errorf("decode sse event: %w", err)
		}
		switch evt.Type {
		case "status":
			if onStatus != nil {
				onStatus(evt)
			}
		case "delta":
			result.Assistant += evt.Text
			if onDelta != nil && evt.Text != "" {
				onDelta(evt.Text)
			}
		case "error":
			if strings.TrimSpace(evt.Error) == "" {
				return fmt.Errorf("chat stream error")
			}
			return errors.New(strings.TrimSpace(evt.Error))
		case "done":
			if strings.TrimSpace(evt.SessionID) != "" {
				result.SessionID = strings.TrimSpace(evt.SessionID)
			}
		}
		return nil
	}); err != nil {
		return chatResult{}, err
	}
	return result, nil
}
