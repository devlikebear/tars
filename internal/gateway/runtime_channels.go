package gateway

import (
	"fmt"
	"strings"
	"time"
)

func (r *Runtime) MessageSend(channelID, threadID, text string) (ChannelMessage, error) {
	return r.MessageSendByWorkspace(defaultWorkspaceID, channelID, threadID, text)
}

func (r *Runtime) MessageSendByWorkspace(workspaceID, channelID, threadID, text string) (ChannelMessage, error) {
	if err := r.requireEnabled(); err != nil {
		return ChannelMessage{}, err
	}
	if !r.opts.ChannelsLocalEnabled {
		return ChannelMessage{}, fmt.Errorf("local channels are disabled")
	}
	return r.appendChannelMessage(workspaceID, channelID, threadID, text, "outbound", "local", nil)
}

func (r *Runtime) MessageRead(channelID string, limit int) ([]ChannelMessage, error) {
	return r.MessageReadByWorkspace(defaultWorkspaceID, channelID, limit)
}

func (r *Runtime) MessageReadByWorkspace(workspaceID, channelID string, limit int) ([]ChannelMessage, error) {
	if err := r.requireEnabled(); err != nil {
		return nil, err
	}
	key := strings.TrimSpace(channelID)
	if key == "" {
		return nil, fmt.Errorf("channel_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	internalKey := workspaceChannelKey(workspaceID, key)
	r.mu.RLock()
	items := append([]ChannelMessage(nil), r.channelMsgs[internalKey]...)
	r.mu.RUnlock()
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (r *Runtime) ThreadReply(channelID, threadID, text string) (ChannelMessage, error) {
	return r.ThreadReplyByWorkspace(defaultWorkspaceID, channelID, threadID, text)
}

func (r *Runtime) ThreadReplyByWorkspace(workspaceID, channelID, threadID, text string) (ChannelMessage, error) {
	if strings.TrimSpace(threadID) == "" {
		return ChannelMessage{}, fmt.Errorf("thread_id is required")
	}
	return r.MessageSendByWorkspace(workspaceID, channelID, threadID, text)
}

func (r *Runtime) InboundWebhook(channelID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	return r.InboundWebhookByWorkspace(defaultWorkspaceID, channelID, threadID, text, payload)
}

func (r *Runtime) InboundWebhookByWorkspace(workspaceID, channelID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if err := r.requireEnabled(); err != nil {
		return ChannelMessage{}, err
	}
	if !r.opts.ChannelsWebhookEnabled {
		return ChannelMessage{}, fmt.Errorf("webhook channels are disabled")
	}
	return r.appendChannelMessage(workspaceID, channelID, threadID, text, "inbound", "webhook", payload)
}

func (r *Runtime) InboundTelegram(botID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	return r.InboundTelegramByWorkspace(defaultWorkspaceID, botID, threadID, text, payload)
}

func (r *Runtime) InboundTelegramByWorkspace(workspaceID, botID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if err := r.requireEnabled(); err != nil {
		return ChannelMessage{}, err
	}
	if !r.opts.ChannelsTelegramEnabled {
		return ChannelMessage{}, fmt.Errorf("telegram channels are disabled")
	}
	channelID := strings.TrimSpace(botID)
	if channelID == "" {
		channelID = "telegram"
	}
	return r.appendChannelMessage(workspaceID, channelID, threadID, text, "inbound", "telegram", payload)
}

func (r *Runtime) OutboundTelegram(botID, chatID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	return r.OutboundTelegramByWorkspace(defaultWorkspaceID, botID, chatID, threadID, text, payload)
}

func (r *Runtime) OutboundTelegramByWorkspace(workspaceID, botID, chatID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if err := r.requireEnabled(); err != nil {
		return ChannelMessage{}, err
	}
	if !r.opts.ChannelsTelegramEnabled {
		return ChannelMessage{}, fmt.Errorf("telegram channels are disabled")
	}
	channelID := strings.TrimSpace(chatID)
	if channelID == "" {
		channelID = strings.TrimSpace(botID)
	}
	if channelID == "" {
		channelID = "telegram"
	}
	return r.appendChannelMessage(workspaceID, channelID, threadID, text, "outbound", "telegram", payload)
}

func (r *Runtime) appendChannelMessage(workspaceID, channelID, threadID, text, direction, source string, payload map[string]any) (ChannelMessage, error) {
	key := strings.TrimSpace(channelID)
	if key == "" {
		return ChannelMessage{}, fmt.Errorf("channel_id is required")
	}
	body := strings.TrimSpace(text)
	if body == "" {
		return ChannelMessage{}, fmt.Errorf("text is required")
	}
	normalizedWorkspaceID := normalizeWorkspaceID(workspaceID)
	internalKey := workspaceChannelKey(normalizedWorkspaceID, key)
	now := r.nowFn().UTC()
	msg := ChannelMessage{
		ID:          fmt.Sprintf("msg_%d", r.messageSeq.Add(1)),
		WorkspaceID: normalizedWorkspaceID,
		ChannelID:   key,
		ThreadID:    strings.TrimSpace(threadID),
		Direction:   strings.TrimSpace(direction),
		Source:      strings.TrimSpace(source),
		Text:        body,
		Timestamp:   now.Format(time.RFC3339),
	}
	if len(payload) > 0 {
		msg.Payload = payload
	}
	r.mu.Lock()
	r.channelMsgs[internalKey] = append(r.channelMsgs[internalKey], msg)
	if max := r.opts.GatewayChannelsMaxMessagesPerChannel; max > 0 && len(r.channelMsgs[internalKey]) > max {
		r.channelMsgs[internalKey] = r.channelMsgs[internalKey][len(r.channelMsgs[internalKey])-max:]
	}
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return msg, nil
}

func workspaceChannelKey(workspaceID, channelID string) string {
	return normalizeWorkspaceID(workspaceID) + ":" + strings.TrimSpace(channelID)
}
