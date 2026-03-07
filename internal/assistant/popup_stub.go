//go:build !darwin

package assistant

import (
	"context"
	"fmt"
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
