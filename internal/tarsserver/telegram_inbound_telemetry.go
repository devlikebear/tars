package tarsserver

import "strings"

func (h *telegramInboundHandler) recordInbound(updateID, userID int64, chatID, threadID, text, sessionID, policy string, extraPayload map[string]any) {
	if h == nil || h.runtime == nil {
		return
	}
	payload := map[string]any{
		"provider":  "telegram",
		"update_id": updateID,
		"user_id":   userID,
		"chat_id":   strings.TrimSpace(chatID),
	}
	if strings.TrimSpace(policy) != "" {
		payload["dm_policy"] = strings.TrimSpace(policy)
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["session_id"] = strings.TrimSpace(sessionID)
	}
	for key, value := range extraPayload {
		payload[strings.TrimSpace(key)] = value
	}
	_, err := h.runtime.InboundTelegram("telegram", strings.TrimSpace(threadID), strings.TrimSpace(text), payload)
	if err != nil {
		h.logger.Debug().Err(err).Msg("telegram inbound gateway record failed")
	}
}

func (h *telegramInboundHandler) recordOutbound(chatID, threadID, text, sessionID string, extraPayload map[string]any) {
	if h == nil || h.runtime == nil {
		return
	}
	payload := map[string]any{
		"provider": "telegram",
	}
	if strings.TrimSpace(sessionID) != "" {
		payload["session_id"] = strings.TrimSpace(sessionID)
	}
	for key, value := range extraPayload {
		payload[strings.TrimSpace(key)] = value
	}
	_, err := h.runtime.OutboundTelegram("telegram", strings.TrimSpace(chatID), strings.TrimSpace(threadID), strings.TrimSpace(text), payload)
	if err != nil {
		h.logger.Debug().Err(err).Msg("telegram outbound gateway record failed")
	}
}
