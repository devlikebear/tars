//go:build !darwin

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

func newPopupPresenter() (popupPresenter, error) {
	return nil, fmt.Errorf("assistant popup is unsupported on this platform")
}

func parsePromptDialogOutput(raw string) (popupResult, error) {
	_ = raw
	return popupResult{}, fmt.Errorf("assistant popup is unsupported on this platform")
}

func parseRecordingDialogOutput(raw string) (bool, error) {
	_ = raw
	return false, fmt.Errorf("assistant popup is unsupported on this platform")
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
