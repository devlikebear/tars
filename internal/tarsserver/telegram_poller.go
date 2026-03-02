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

	"github.com/rs/zerolog"
)

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message,omitempty"`
}

type telegramMessage struct {
	MessageID       int64             `json:"message_id"`
	Text            string            `json:"text"`
	Caption         string            `json:"caption,omitempty"`
	Photo           []telegramPhoto   `json:"photo,omitempty"`
	Document        *telegramDocument `json:"document,omitempty"`
	Voice           *telegramVoice    `json:"voice,omitempty"`
	MessageThreadID int64             `json:"message_thread_id,omitempty"`
	Chat            telegramChat      `json:"chat"`
	From            telegramUser      `json:"from"`
	Date            int64             `json:"date,omitempty"`
}

type telegramPhoto struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

type telegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

type telegramVoice struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type telegramChat struct {
	ID    json.Number `json:"id"`
	Type  string      `json:"type"`
	Title string      `json:"title,omitempty"`
}

type telegramUser struct {
	ID        json.Number `json:"id"`
	Username  string      `json:"username,omitempty"`
	FirstName string      `json:"first_name,omitempty"`
	LastName  string      `json:"last_name,omitempty"`
}

func (c telegramChat) IDString() string {
	return strings.TrimSpace(c.ID.String())
}

func (u telegramUser) IDInt64() int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(u.ID.String()), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func (u telegramUser) DisplayName() string {
	if name := strings.TrimSpace(u.Username); name != "" {
		return name
	}
	full := strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
	if full != "" {
		return full
	}
	return strings.TrimSpace(u.ID.String())
}

type telegramUpdatePoller struct {
	botToken string
	baseURL  string
	client   *http.Client
	logger   zerolog.Logger
	onUpdate func(context.Context, telegramUpdate)
	readLast func() int64
	saveLast func(int64) error
}

func newTelegramUpdatePoller(botToken string, logger zerolog.Logger, onUpdate func(context.Context, telegramUpdate)) *telegramUpdatePoller {
	trimmedToken := strings.TrimSpace(botToken)
	if trimmedToken == "" || onUpdate == nil {
		return nil
	}
	return &telegramUpdatePoller{
		botToken: trimmedToken,
		baseURL:  "https://api.telegram.org",
		client: &http.Client{
			Timeout: telegramPollTimeout + 5*time.Second,
		},
		logger:   logger,
		onUpdate: onUpdate,
	}
}

func (p *telegramUpdatePoller) withOffsetStore(readLast func() int64, saveLast func(int64) error) *telegramUpdatePoller {
	if p == nil {
		return nil
	}
	p.readLast = readLast
	p.saveLast = saveLast
	return p
}

func (p *telegramUpdatePoller) Run(ctx context.Context) {
	if p == nil {
		return
	}
	offset := int64(0)
	if p.readLast != nil {
		if last := p.readLast(); last > 0 {
			offset = last + 1
		}
	}
	backoff := 500 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		updates, err := p.fetchUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.logger.Debug().Err(err).Msg("telegram polling failed")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Second {
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
			continue
		}
		backoff = 500 * time.Millisecond
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			// Keep update handling ordered in a single polling loop.
			p.onUpdate(ctx, update)
			if p.saveLast != nil && update.UpdateID > 0 {
				if err := p.saveLast(update.UpdateID); err != nil {
					p.logger.Debug().Err(err).Int64("update_id", update.UpdateID).Msg("persist telegram update offset failed")
				}
			}
		}
	}
}

func (p *telegramUpdatePoller) fetchUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	if p == nil {
		return nil, fmt.Errorf("telegram poller is not configured")
	}
	reqBody := map[string]any{
		"timeout":         int(telegramPollTimeout / time.Second),
		"allowed_updates": []string{"message"},
	}
	if offset > 0 {
		reqBody["offset"] = offset
	}
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("encode telegram getUpdates request: %w", err)
	}
	endpoint := strings.TrimRight(p.baseURL, "/") + "/bot" + p.botToken + "/getUpdates"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("build telegram getUpdates request: %s", sanitizeTelegramError(err, p.botToken))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram getUpdates failed: %s", sanitizeTelegramError(err, p.botToken))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram getUpdates status %d: %s", resp.StatusCode, sanitizeTelegramLogText(strings.TrimSpace(string(body)), p.botToken))
	}
	var payload struct {
		OK          bool             `json:"ok"`
		Description string           `json:"description"`
		Result      []telegramUpdate `json:"result"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode telegram getUpdates response: %w", err)
	}
	if !payload.OK {
		description := strings.TrimSpace(payload.Description)
		if description == "" {
			description = "telegram getUpdates returned ok=false"
		}
		return nil, errors.New(sanitizeTelegramLogText(description, p.botToken))
	}
	if payload.Result == nil {
		return []telegramUpdate{}, nil
	}
	return payload.Result, nil
}
