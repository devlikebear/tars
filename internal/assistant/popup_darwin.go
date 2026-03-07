//go:build darwin

package assistant

import (
	"context"
	"fmt"
	"os/exec"
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

type appleScriptRunner func(ctx context.Context, script string) (string, error)

type appleScriptPopup struct {
	run appleScriptRunner
}

func newPopupPresenter() (popupPresenter, error) {
	return appleScriptPopup{run: runAppleScript}, nil
}

func (p appleScriptPopup) Prompt(ctx context.Context) (popupResult, error) {
	raw, err := p.run(ctx, `tell application "System Events"
display dialog "Ask TARS" default answer "" buttons {"Cancel", "Mic", "Send"} default button "Send"
end tell`)
	if err != nil {
		return popupResult{}, err
	}
	return parsePromptDialogOutput(raw)
}

func (p appleScriptPopup) WaitRecordingStop(ctx context.Context) (bool, error) {
	raw, err := p.run(ctx, `tell application "System Events"
display dialog "Recording... press Stop to send audio." buttons {"Cancel", "Stop"} default button "Stop"
end tell`)
	if err != nil {
		return false, err
	}
	return parseRecordingDialogOutput(raw)
}

func (p appleScriptPopup) ShowResult(ctx context.Context, result VoiceTurnResult) error {
	title := quoteAppleScript("TARS replied")
	reply := quoteAppleScript(popupPreviewText(result.AssistantReply, 500))
	transcript := quoteAppleScript(popupPreviewText(result.Transcript, 180))
	raw, err := p.run(ctx, fmt.Sprintf(`tell application "System Events"
display dialog %s with title %s buttons {"OK"} default button "OK"
end tell`, quoteAppleScript("You: "+transcript+"\n\nTARS: "+reply), title))
	if err != nil {
		return err
	}
	_ = raw
	return nil
}

func (p appleScriptPopup) ShowError(ctx context.Context, message string) error {
	raw, err := p.run(ctx, fmt.Sprintf(`tell application "System Events"
display alert %s message %s as critical buttons {"OK"} default button "OK"
end tell`, quoteAppleScript("TARS assistant error"), quoteAppleScript(popupPreviewText(message, 700))))
	if err != nil {
		return err
	}
	_ = raw
	return nil
}

func runAppleScript(ctx context.Context, script string) (string, error) {
	cmd := exec.CommandContext(ctx, "osascript", "-e", strings.TrimSpace(script))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("osascript failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
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

func quoteAppleScript(raw string) string {
	return `"` + strings.ReplaceAll(raw, `"`, `\"`) + `"`
}
