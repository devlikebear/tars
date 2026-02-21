package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type telegramSendRequest struct {
	BotID     string
	ChatID    string
	Text      string
	ThreadID  string
	ParseMode string
}

type telegramSendResult struct {
	MessageID int64  `json:"message_id"`
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
}

type telegramSender interface {
	Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error)
}

type telegramSendFunc func(ctx context.Context, req telegramSendRequest) (telegramSendResult, error)

func (f telegramSendFunc) Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
	if f == nil {
		return telegramSendResult{}, fmt.Errorf("telegram sender is not configured")
	}
	return f(ctx, req)
}

type telegramHTTPSender struct {
	botToken string
	baseURL  string
	client   *http.Client
}

func newTelegramSender(botToken string) telegramSender {
	trimmed := strings.TrimSpace(botToken)
	if trimmed == "" {
		return nil
	}
	return &telegramHTTPSender{
		botToken: trimmed,
		baseURL:  "https://api.telegram.org",
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (s *telegramHTTPSender) Send(ctx context.Context, req telegramSendRequest) (telegramSendResult, error) {
	if s == nil || strings.TrimSpace(s.botToken) == "" {
		return telegramSendResult{}, fmt.Errorf("telegram sender is not configured")
	}
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		return telegramSendResult{}, fmt.Errorf("chat_id is required")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return telegramSendResult{}, fmt.Errorf("text is required")
	}
	requestBody := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode := strings.TrimSpace(req.ParseMode); parseMode != "" {
		requestBody["parse_mode"] = parseMode
	}
	if threadID := strings.TrimSpace(req.ThreadID); threadID != "" {
		if parsed, err := strconv.ParseInt(threadID, 10, 64); err == nil {
			requestBody["message_thread_id"] = parsed
		} else {
			requestBody["message_thread_id"] = threadID
		}
	}

	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return telegramSendResult{}, fmt.Errorf("encode telegram request: %w", err)
	}
	endpoint := strings.TrimRight(s.baseURL, "/") + "/bot" + s.botToken + "/sendMessage"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return telegramSendResult{}, fmt.Errorf("build telegram request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return telegramSendResult{}, fmt.Errorf("telegram request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return telegramSendResult{}, fmt.Errorf("telegram api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int64  `json:"message_id"`
			Text      string `json:"text"`
			Chat      struct {
				ID json.Number `json:"id"`
			} `json:"chat"`
		} `json:"result"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		return telegramSendResult{}, fmt.Errorf("decode telegram response: %w", err)
	}
	if !parsed.OK {
		description := strings.TrimSpace(parsed.Description)
		if description == "" {
			description = "telegram api returned ok=false"
		}
		return telegramSendResult{}, errors.New(description)
	}
	resultChatID := strings.TrimSpace(parsed.Result.Chat.ID.String())
	if resultChatID == "" {
		resultChatID = chatID
	}
	resultText := strings.TrimSpace(parsed.Result.Text)
	if resultText == "" {
		resultText = text
	}
	return telegramSendResult{
		MessageID: parsed.Result.MessageID,
		ChatID:    resultChatID,
		Text:      resultText,
	}, nil
}
