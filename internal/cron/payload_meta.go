package cron

import (
	"bytes"
	"encoding/json"
	"strings"
)

const payloadMetaKey = "_tars_cron"

type PayloadMeta struct {
	TaskType          string `json:"task_type,omitempty"`
	ReminderMessage   string `json:"reminder_message,omitempty"`
	SourceSessionKind string `json:"source_session_kind,omitempty"`
	TelegramChatID    string `json:"telegram_chat_id,omitempty"`
	TelegramThreadID  string `json:"telegram_thread_id,omitempty"`
	TelegramBotID     string `json:"telegram_bot_id,omitempty"`
}

func MergePayloadMeta(raw json.RawMessage, meta PayloadMeta) (json.RawMessage, error) {
	payload, err := normalizePayload(raw)
	if err != nil {
		return nil, err
	}
	meta = normalizePayloadMeta(meta)
	if isEmptyPayloadMeta(meta) {
		return payload, nil
	}

	base := map[string]any{}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &base); err != nil {
			return nil, err
		}
	}
	base[payloadMetaKey] = map[string]any{
		"task_type":           meta.TaskType,
		"reminder_message":    meta.ReminderMessage,
		"source_session_kind": meta.SourceSessionKind,
		"telegram_chat_id":    meta.TelegramChatID,
		"telegram_thread_id":  meta.TelegramThreadID,
		"telegram_bot_id":     meta.TelegramBotID,
	}

	buf := &bytes.Buffer{}
	encoded, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if err := json.Compact(buf, encoded); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

func ExtractPayloadMeta(raw json.RawMessage) (PayloadMeta, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return PayloadMeta{}, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return PayloadMeta{}, false
	}
	metaRaw, ok := obj[payloadMetaKey]
	if !ok || len(metaRaw) == 0 {
		return PayloadMeta{}, false
	}
	var meta PayloadMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return PayloadMeta{}, false
	}
	meta = normalizePayloadMeta(meta)
	if isEmptyPayloadMeta(meta) {
		return PayloadMeta{}, false
	}
	return meta, true
}

func normalizePayloadMeta(meta PayloadMeta) PayloadMeta {
	meta.TaskType = strings.TrimSpace(strings.ToLower(meta.TaskType))
	meta.ReminderMessage = strings.TrimSpace(meta.ReminderMessage)
	meta.SourceSessionKind = strings.TrimSpace(strings.ToLower(meta.SourceSessionKind))
	meta.TelegramChatID = strings.TrimSpace(meta.TelegramChatID)
	meta.TelegramThreadID = strings.TrimSpace(meta.TelegramThreadID)
	meta.TelegramBotID = strings.TrimSpace(meta.TelegramBotID)
	return meta
}

func isEmptyPayloadMeta(meta PayloadMeta) bool {
	return meta.TaskType == "" &&
		meta.ReminderMessage == "" &&
		meta.SourceSessionKind == "" &&
		meta.TelegramChatID == "" &&
		meta.TelegramThreadID == "" &&
		meta.TelegramBotID == ""
}
