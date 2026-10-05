package tarsclient

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/devlikebear/tars/internal/secrets"
)

type Options struct {
	ServerURL string
	SessionID string
	APIToken  string
	Message   string
	Verbose   bool
}

func Run(ctx context.Context, _ io.Reader, stdout, stderr io.Writer, opts Options) error {
	chat := chatClient{
		serverURL: opts.ServerURL,
		apiToken:  opts.APIToken,
	}
	session := strings.TrimSpace(opts.SessionID)
	if strings.TrimSpace(opts.Message) != "" {
		res, err := sendMessage(ctx, chat, session, opts.Message, opts.Verbose, opts.Verbose, stdout, stderr)
		if err != nil {
			return err
		}
		if res.SessionID != "" {
			fmt.Fprintf(stderr, "session=%s\n", res.SessionID)
		}
		return nil
	}
	return fmt.Errorf("interactive terminal UI has been removed; use the web console at /console or one-shot CLI commands")
}

func sendMessage(ctx context.Context, client chatClient, session, message string, showStatus bool, verbose bool, stdout, stderr io.Writer) (chatResult, error) {
	fmt.Fprint(stdout, "TARS > ")
	res, err := client.stream(ctx, chatRequest{Message: message, SessionID: session}, func(evt chatEvent) {
		// Goal events are printed even without --verbose: a judge error or an
		// exhausted budget is why an unattended run stopped.
		if evt.Type == "goal_event" {
			_, _ = fmt.Fprintf(stderr, "goal: %s\n", secrets.RedactText(formatGoalEvent(evt)))
			return
		}
		if !showStatus {
			return
		}
		label := formatChatStatusEvent(evt, verbose)
		if strings.TrimSpace(label) != "" {
			fmt.Fprintf(stderr, "status: %s\n", strings.TrimSpace(label))
		}
	}, func(chunk string) {
		fmt.Fprint(stdout, chunk)
	})
	fmt.Fprintln(stdout)
	return res, err
}

func formatChatStatusEvent(evt chatEvent, verbose bool) string {
	label := strings.TrimSpace(evt.Message)
	if label == "" {
		label = strings.TrimSpace(evt.Phase)
	}
	if label == "" {
		return ""
	}
	toolName := strings.TrimSpace(evt.ToolName)
	if toolName != "" {
		switch strings.TrimSpace(evt.Phase) {
		case "before_tool_call", "after_tool_call", "error":
			label = fmt.Sprintf("%s (%s)", label, toolName)
		default:
			if verbose {
				label = fmt.Sprintf("%s tool=%s", label, toolName)
			}
		}
	}
	if !verbose {
		return secrets.RedactText(label)
	}
	parts := []string{secrets.RedactText(label)}
	if toolCallID := strings.TrimSpace(evt.ToolCallID); toolCallID != "" {
		parts = append(parts, "id="+secrets.RedactText(toolCallID))
	}
	if toolArgs := strings.TrimSpace(evt.ToolArgsPreview); toolArgs != "" {
		parts = append(parts, "args="+secrets.RedactText(toolArgs))
	}
	if toolResult := strings.TrimSpace(evt.ToolResultPreview); toolResult != "" {
		parts = append(parts, "result="+secrets.RedactText(toolResult))
	}
	return secrets.RedactText(strings.Join(parts, " | "))
}

func formatGoalEvent(evt chatEvent) string {
	phase := strings.TrimSpace(evt.Phase)
	if reason := strings.TrimSpace(evt.Reason); reason != "" {
		return phase + " — " + reason
	}
	return phase
}
