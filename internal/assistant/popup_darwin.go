//go:build darwin

package assistant

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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

func quoteAppleScript(raw string) string {
	return `"` + strings.ReplaceAll(raw, `"`, `\"`) + `"`
}
