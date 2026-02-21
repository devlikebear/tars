package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c runtimeClient) requestJSON(ctx context.Context, method, path string, body any, admin bool, out any) error {
	text, err := c.requestText(ctx, method, path, body, admin)
	if err != nil {
		return err
	}
	if out == nil || len(text) == 0 {
		return nil
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c runtimeClient) requestText(ctx context.Context, method, path string, body any, admin bool) (string, error) {
	endpoint, err := c.resolve(path)
	if err != nil {
		return "", err
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token := strings.TrimSpace(c.apiToken)
	if admin {
		token = strings.TrimSpace(c.adminAPIToken)
		if token == "" {
			token = strings.TrimSpace(c.apiToken)
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message, ok := parseAPIErrorPayload(text)
		if ok {
			return "", &apiHTTPError{
				Method:   method,
				Endpoint: endpoint,
				Status:   resp.StatusCode,
				Code:     code,
				Message:  message,
				Body:     strings.TrimSpace(string(text)),
			}
		}
		return "", &apiHTTPError{
			Method:   method,
			Endpoint: endpoint,
			Status:   resp.StatusCode,
			Body:     strings.TrimSpace(string(text)),
		}
	}
	return string(text), nil
}

func parseAPIErrorPayload(payload []byte) (code, message string, ok bool) {
	var body struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", "", false
	}
	code = strings.TrimSpace(body.Code)
	message = strings.TrimSpace(body.Error)
	if message == "" {
		message = strings.TrimSpace(body.Message)
	}
	if message == "" {
		message = code
	}
	if code == "" && message == "" {
		return "", "", false
	}
	return code, message, true
}

func (c runtimeClient) resolve(path string) (string, error) {
	return resolveURL(c.serverURL, path)
}
