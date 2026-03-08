package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/gateway"
	"github.com/rs/zerolog"
)

func newChannelsAPIHandler(runtime *gateway.Runtime, logger zerolog.Logger) http.Handler {
	return newChannelsAPIHandlerWithTelegramSender(runtime, nil, logger)
}

func newChannelsAPIHandlerWithTelegramSender(runtime *gateway.Runtime, sender telegramSender, logger zerolog.Logger) http.Handler {
	return newChannelsAPIHandlerWithTelegramPairings(runtime, sender, nil, "pairing", false, logger)
}

func newChannelsAPIHandlerWithTelegramPairings(
	runtime *gateway.Runtime,
	sender telegramSender,
	pairings *telegramPairingStore,
	dmPolicy string,
	pollingEnabled bool,
	logger zerolog.Logger,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/channels/webhook/inbound/", func(w http.ResponseWriter, r *http.Request) {
		handleWebhookInbound(w, r, runtime, logger)
	})
	mux.HandleFunc("/v1/channels/telegram/webhook/", func(w http.ResponseWriter, r *http.Request) {
		handleTelegramInbound(w, r, runtime, logger)
	})
	mux.HandleFunc("/v1/channels/telegram/send", func(w http.ResponseWriter, r *http.Request) {
		handleTelegramSend(w, r, runtime, sender, logger)
	})
	mux.HandleFunc("/v1/channels/telegram/pairings", func(w http.ResponseWriter, r *http.Request) {
		handleTelegramPairingsList(w, r, pairings, dmPolicy, pollingEnabled)
	})
	mux.HandleFunc("/v1/channels/telegram/pairings/", func(w http.ResponseWriter, r *http.Request) {
		handleTelegramPairingsAction(w, r, pairings)
	})
	return mux
}

func handleWebhookInbound(w http.ResponseWriter, r *http.Request, runtime *gateway.Runtime, logger zerolog.Logger) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
		return
	}
	channelID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/channels/webhook/inbound/"))
	if channelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "channel_id is required"})
		return
	}
	payload, ok := decodeMapBody(w, r, false)
	if !ok {
		return
	}
	threadID := strings.TrimSpace(asString(payload["thread_id"]))
	msg, err := runtime.InboundWebhook(channelID, threadID, extractInboundText(payload), payload)
	if err != nil {
		logger.Error().Err(err).Str("channel_id", channelID).Msg("webhook inbound failed")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func handleTelegramInbound(w http.ResponseWriter, r *http.Request, runtime *gateway.Runtime, logger zerolog.Logger) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
		return
	}
	botID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/channels/telegram/webhook/"))
	if botID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bot_id is required"})
		return
	}
	payload, ok := decodeMapBody(w, r, false)
	if !ok {
		return
	}
	threadID := strings.TrimSpace(asString(payload["thread_id"]))
	msg, err := runtime.InboundTelegram(botID, threadID, extractInboundText(payload), payload)
	if err != nil {
		logger.Error().Err(err).Str("bot_id", botID).Msg("telegram inbound failed")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func handleTelegramSend(
	w http.ResponseWriter,
	r *http.Request,
	runtime *gateway.Runtime,
	sender telegramSender,
	logger zerolog.Logger,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gateway runtime is not configured"})
		return
	}
	if sender == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram sender is not configured"})
		return
	}
	payload, ok := decodeMapBody(w, r, true)
	if !ok {
		return
	}
	chatID := strings.TrimSpace(asTelegramString(payload["chat_id"]))
	if chatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "chat_id is required"})
		return
	}
	text := strings.TrimSpace(asTelegramString(payload["text"]))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	threadID := strings.TrimSpace(asTelegramString(payload["thread_id"]))
	parseMode := strings.TrimSpace(asTelegramString(payload["parse_mode"]))
	botID := strings.TrimSpace(asTelegramString(payload["bot_id"]))

	sendCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	sendResult, err := sender.Send(sendCtx, telegramSendRequest{
		BotID:     botID,
		ChatID:    chatID,
		Text:      text,
		ThreadID:  threadID,
		ParseMode: parseMode,
	})
	if err != nil {
		logger.Error().Err(err).Str("chat_id", chatID).Msg("telegram send failed")
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "telegram send failed: " + strings.TrimSpace(err.Error())})
		return
	}

	recordPayload := map[string]any{"provider": "telegram"}
	if botID != "" {
		recordPayload["bot_id"] = botID
	}
	if parseMode != "" {
		recordPayload["parse_mode"] = parseMode
	}
	if sendResult.MessageID > 0 {
		recordPayload["message_id"] = sendResult.MessageID
	}
	if sendResult.ChatID != "" {
		recordPayload["provider_chat_id"] = sendResult.ChatID
	}
	if sendResult.Text != "" {
		recordPayload["provider_text"] = sendResult.Text
	}
	msg, err := runtime.OutboundTelegram(botID, chatID, threadID, text, recordPayload)
	if err != nil {
		logger.Error().Err(err).Str("chat_id", chatID).Msg("telegram outbound record failed")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, msg)
}

func handleTelegramPairingsList(
	w http.ResponseWriter,
	r *http.Request,
	pairings *telegramPairingStore,
	dmPolicy string,
	pollingEnabled bool,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if pairings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram pairing store is not configured"})
		return
	}
	writeJSON(w, http.StatusOK, pairings.snapshot(dmPolicy, pollingEnabled))
}

func handleTelegramPairingsAction(w http.ResponseWriter, r *http.Request, pairings *telegramPairingStore) {
	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/channels/telegram/pairings/"))
	if path == "" || path != "approve" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if pairings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telegram pairing store is not configured"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	allowed, err := pairings.approve(req.Code)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approved": allowed})
}

func decodeMapBody(w http.ResponseWriter, r *http.Request, useNumber bool) (map[string]any, bool) {
	payload := map[string]any{}
	decoder := json.NewDecoder(r.Body)
	if useNumber {
		decoder.UseNumber()
	}
	if err := decoder.Decode(&payload); err != nil && (useNumber || err != io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return nil, false
	}
	return payload, true
}

func extractInboundText(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if v := strings.TrimSpace(asString(payload["text"])); v != "" {
		return v
	}
	if msg, ok := payload["message"].(map[string]any); ok {
		if v := strings.TrimSpace(asString(msg["text"])); v != "" {
			return v
		}
	}
	return ""
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func asTelegramString(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return strings.TrimSpace(value.String())
	case float64:
		if value == math.Trunc(value) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
