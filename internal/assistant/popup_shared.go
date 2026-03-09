package assistant

import (
	"context"
	"fmt"
	"strings"
)

type popupAction string

const (
	popupActionSend   popupAction = "send"
	popupActionMic    popupAction = "mic"
	popupActionCancel popupAction = "cancel"
)

type popupResult struct {
	Action popupAction
	Text   string
}

type popupPresenter interface {
	Prompt(ctx context.Context) (popupResult, error)
	WaitRecordingStop(ctx context.Context) (bool, error)
	ShowResult(ctx context.Context, result VoiceTurnResult) error
	ShowError(ctx context.Context, message string) error
}

func parsePromptDialogOutput(raw string) (popupResult, error) {
	button, err := parseDialogField(raw, "button returned:")
	if err != nil {
		return popupResult{}, err
	}
	text, _ := parseDialogField(raw, "text returned:")
	result := popupResult{Text: strings.TrimSpace(text)}
	switch strings.ToLower(strings.TrimSpace(button)) {
	case "send":
		result.Action = popupActionSend
	case "mic":
		result.Action = popupActionMic
	case "cancel":
		result.Action = popupActionCancel
	default:
		return popupResult{}, fmt.Errorf("unknown popup action: %s", strings.TrimSpace(button))
	}
	return result, nil
}

func parseRecordingDialogOutput(raw string) (bool, error) {
	button, err := parseDialogField(raw, "button returned:")
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(button)) {
	case "stop":
		return true, nil
	case "cancel":
		return false, nil
	default:
		return false, fmt.Errorf("unknown recording action: %s", strings.TrimSpace(button))
	}
}

func parseDialogField(raw string, prefix string) (string, error) {
	for _, part := range strings.Split(raw, ",") {
		item := strings.TrimSpace(part)
		if strings.HasPrefix(item, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(item, prefix)), nil
		}
	}
	return "", fmt.Errorf("missing dialog field: %s", strings.TrimSpace(prefix))
}

func popupPreviewText(raw string, maxLen int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if text == "" {
		return "(empty reply)"
	}
	if maxLen <= 0 || len([]rune(text)) <= maxLen {
		return text
	}
	runes := []rune(text)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
