package assistant

import (
	"context"
	"fmt"
	"strings"
)

func RunTextTurn(ctx context.Context, deps VoiceTurnDeps, text string) (VoiceTurnResult, error) {
	message := strings.TrimSpace(text)
	if message == "" {
		return VoiceTurnResult{}, fmt.Errorf("empty transcript")
	}
	return executeChatTurn(ctx, deps, message)
}

func executeChatTurn(ctx context.Context, deps VoiceTurnDeps, message string) (VoiceTurnResult, error) {
	if deps.ChatClient == nil {
		return VoiceTurnResult{}, fmt.Errorf("chat client is required")
	}
	reply, nextSession, err := deps.ChatClient.Chat(ctx, strings.TrimSpace(message), strings.TrimSpace(deps.SessionID))
	if err != nil {
		return VoiceTurnResult{}, err
	}
	result := VoiceTurnResult{
		Transcript:     strings.TrimSpace(message),
		AssistantReply: strings.TrimSpace(reply),
		SessionID:      strings.TrimSpace(nextSession),
	}
	if result.SessionID == "" {
		result.SessionID = strings.TrimSpace(deps.SessionID)
	}
	if deps.Speaker != nil && strings.TrimSpace(result.AssistantReply) != "" {
		if err := deps.Speaker.Speak(ctx, result.AssistantReply); err != nil {
			result.TTSError = err.Error()
		}
	}
	return result, nil
}
