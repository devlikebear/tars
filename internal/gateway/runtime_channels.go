package gateway

import (
	"fmt"
	"strings"
	"time"
)

func (r *Runtime) MessageSend(channelID, threadID, text string) (ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return ChannelMessage{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.ChannelsLocalEnabled {
		return ChannelMessage{}, fmt.Errorf("local channels are disabled")
	}
	return r.appendChannelMessage(channelID, threadID, text, "outbound", "local", nil)
}

func (r *Runtime) MessageRead(channelID string, limit int) ([]ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return nil, fmt.Errorf("gateway runtime is disabled")
	}
	key := strings.TrimSpace(channelID)
	if key == "" {
		return nil, fmt.Errorf("channel_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	r.mu.RLock()
	items := append([]ChannelMessage(nil), r.channelMsgs[key]...)
	r.mu.RUnlock()
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (r *Runtime) ThreadReply(channelID, threadID, text string) (ChannelMessage, error) {
	if strings.TrimSpace(threadID) == "" {
		return ChannelMessage{}, fmt.Errorf("thread_id is required")
	}
	return r.MessageSend(channelID, threadID, text)
}

func (r *Runtime) InboundWebhook(channelID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return ChannelMessage{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.ChannelsWebhookEnabled {
		return ChannelMessage{}, fmt.Errorf("webhook channels are disabled")
	}
	return r.appendChannelMessage(channelID, threadID, text, "inbound", "webhook", payload)
}

func (r *Runtime) InboundTelegram(botID, threadID, text string, payload map[string]any) (ChannelMessage, error) {
	if r == nil || !r.opts.Enabled {
		return ChannelMessage{}, fmt.Errorf("gateway runtime is disabled")
	}
	if !r.opts.ChannelsTelegramEnabled {
		return ChannelMessage{}, fmt.Errorf("telegram channels are disabled")
	}
	channelID := strings.TrimSpace(botID)
	if channelID == "" {
		channelID = "telegram"
	}
	return r.appendChannelMessage(channelID, threadID, text, "inbound", "telegram", payload)
}

func (r *Runtime) appendChannelMessage(channelID, threadID, text, direction, source string, payload map[string]any) (ChannelMessage, error) {
	key := strings.TrimSpace(channelID)
	if key == "" {
		return ChannelMessage{}, fmt.Errorf("channel_id is required")
	}
	body := strings.TrimSpace(text)
	if body == "" {
		return ChannelMessage{}, fmt.Errorf("text is required")
	}
	now := r.nowFn().UTC()
	msg := ChannelMessage{
		ID:        fmt.Sprintf("msg_%d", r.messageSeq.Add(1)),
		ChannelID: key,
		ThreadID:  strings.TrimSpace(threadID),
		Direction: strings.TrimSpace(direction),
		Source:    strings.TrimSpace(source),
		Text:      body,
		Timestamp: now.Format(time.RFC3339),
	}
	if len(payload) > 0 {
		msg.Payload = payload
	}
	r.mu.Lock()
	r.channelMsgs[key] = append(r.channelMsgs[key], msg)
	if max := r.opts.GatewayChannelsMaxMessagesPerChannel; max > 0 && len(r.channelMsgs[key]) > max {
		r.channelMsgs[key] = r.channelMsgs[key][len(r.channelMsgs[key])-max:]
	}
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return msg, nil
}
