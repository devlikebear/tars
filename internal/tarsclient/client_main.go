package tarsclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/devlikebear/tarsncase/internal/secrets"
)

type Options struct {
	ServerURL  string
	SessionID  string
	APIToken   string
	AdminToken string
	Message    string
	Verbose    bool
}

type localRuntimeState struct {
	notifications   *notificationCenter
	chatTrace       bool
	chatTraceFilter string
}

func Run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, opts Options) error {
	chat := chatClient{
		serverURL: opts.ServerURL,
		apiToken:  opts.APIToken,
	}
	runtime := runtimeClient{
		serverURL:     opts.ServerURL,
		apiToken:      opts.APIToken,
		adminAPIToken: opts.AdminToken,
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
	return runTUI(ctx, stdin, stdout, chat, runtime, session, opts.Verbose)
}

func executeCommand(ctx context.Context, runtime runtimeClient, line, session string, stdout, stderr io.Writer) (bool, string, error) {
	return executeCommandWithState(ctx, runtime, line, session, stdout, stderr, nil)
}

func executeCommandWithState(ctx context.Context, runtime runtimeClient, line, session string, stdout, stderr io.Writer, state *localRuntimeState) (bool, string, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	return dispatchCommand(commandContext{
		ctx:     ctx,
		runtime: runtime,
		fields:  fields,
		line:    line,
		session: session,
		stdout:  stdout,
		stderr:  stderr,
		state:   state,
	})
}

func helpText() string {
	return strings.TrimSpace(`SYSTEM > commands
Session:
  /new [title]
  /session
  /resume [id|number|latest]
  /sessions
  /history
  /export
  /search {keyword}
  /compact

Runtime:
  /status
  /providers
  /models
  /model list
  /whoami
  /health
  /heartbeat
  /skills
  /plugins
  /mcp
  /reload
  /agents [--detail|-d]
  /runs [limit]
  /run {id}
  /cancel-run {id}
  /spawn [--agent ...] [--title ...] [--session ...] [--wait] {message}
  /gateway {status|reload|restart|summary|runs [limit]|channels [limit]}
  /browser {status|profiles|login|check|run}
  /vault {status}
  /channels
  /telegram {pairings|pairing approve {code}}
  /cron {list|get|runs|add|run|delete|enable|disable}
  /project {list|get|create|activate|archive}
  /usage {summary|limits|set-limits}
  /notify {list|filter|open|clear}

Chat:
  /trace [on|off|filter {all|llm|tool|error|system}]
  /quit`)
}

func sendMessage(ctx context.Context, client chatClient, session, message string, showStatus bool, verbose bool, stdout, stderr io.Writer) (chatResult, error) {
	fmt.Fprint(stdout, "TARS > ")
	res, err := client.stream(ctx, chatRequest{Message: message, SessionID: session}, func(evt chatEvent) {
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

func formatRuntimeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	var apiErr *apiHTTPError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return secrets.RedactText(message)
	}
	hint := runtimeErrorHint(apiErr)
	if strings.TrimSpace(hint) == "" {
		return secrets.RedactText(message)
	}
	return secrets.RedactText(message + "\nhint: " + hint)
}

func runtimeErrorHint(apiErr *apiHTTPError) string {
	if apiErr == nil {
		return ""
	}
	endpointPath := ""
	if parsed, err := url.Parse(strings.TrimSpace(apiErr.Endpoint)); err == nil {
		endpointPath = strings.TrimSpace(parsed.Path)
	}
	code := strings.ToLower(strings.TrimSpace(apiErr.Code))
	switch code {
	case "unauthorized":
		if isAdminEndpointPath(endpointPath) {
			return "admin endpoint requires admin token; retry with --admin-api-token (or TARS_ADMIN_API_TOKEN)"
		}
		return "set --api-token (or TARS_API_TOKEN), then retry"
	case "forbidden":
		if isAdminEndpointPath(endpointPath) {
			return "this endpoint requires admin role; retry with --admin-api-token or ask admin"
		}
		return "your role is not allowed for this endpoint"
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return "verify API token and retry"
	case http.StatusForbidden:
		return "verify role permissions and retry"
	default:
		return ""
	}
}

func isAdminEndpointPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	switch {
	case trimmed == "/v1/runtime/extensions/reload":
		return true
	case trimmed == "/v1/gateway/reload":
		return true
	case trimmed == "/v1/gateway/restart":
		return true
	case strings.HasPrefix(trimmed, "/v1/channels/webhook/inbound/"):
		return true
	case strings.HasPrefix(trimmed, "/v1/channels/telegram/webhook/"):
		return true
	case strings.HasPrefix(trimmed, "/v1/channels/telegram/pairings"):
		return true
	case trimmed == "/v1/usage/limits":
		return true
	default:
		return false
	}
}
