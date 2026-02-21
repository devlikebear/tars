package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/secrets"
	"github.com/devlikebear/tarsncase/internal/tarsapp"
	"github.com/spf13/cobra"
)

type options struct {
	serverURL  string
	sessionID  string
	apiToken   string
	adminToken string
	message    string
	verbose    bool
}

type serveOptions struct {
	configPath        string
	mode              string
	workspaceDir      string
	logFile           string
	verbose           bool
	runOnce           bool
	runLoop           bool
	serveAPI          bool
	apiAddr           string
	heartbeatInterval time.Duration
	maxHeartbeats     int
}

var serveRunner = runServeCommand

type localRuntimeState struct {
	notifications   *notificationCenter
	chatTrace       bool
	chatTraceFilter string
}

func main() {
	bootstrapClientEnv()
	if err := newRootCommand(os.Stdin, os.Stdout, os.Stderr).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func bootstrapClientEnv() {
	bootstrapClientEnvFiles(".env", ".env.secret")
}

func bootstrapClientEnvFiles(envPath, secretPath string) {
	tarsapp.LoadRuntimeEnvFiles(envPath, secretPath)
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "tars",
		Short: "Go TUI client for tars",
		RunE: func(cmd *cobra.Command, _ []string) error {
			chat := chatClient{
				serverURL: opts.serverURL,
				apiToken:  opts.apiToken,
			}
			runtime := runtimeClient{
				serverURL:     opts.serverURL,
				apiToken:      opts.apiToken,
				adminAPIToken: opts.adminToken,
			}
			session := strings.TrimSpace(opts.sessionID)
			if strings.TrimSpace(opts.message) != "" {
				res, err := sendMessage(cmd.Context(), chat, session, opts.message, opts.verbose, opts.verbose, stdout, stderr)
				if err != nil {
					return err
				}
				if res.SessionID != "" {
					fmt.Fprintf(stderr, "session=%s\n", res.SessionID)
				}
				return nil
			}
			return runTUI(cmd.Context(), stdin, stdout, chat, runtime, session, opts.verbose)
		},
	}
	cmd.Flags().StringVar(&opts.serverURL, "server-url", os.Getenv("TARS_SERVER_URL"), "tars server url")
	cmd.Flags().StringVar(&opts.sessionID, "session", "", "session id")
	cmd.Flags().StringVar(&opts.apiToken, "api-token", os.Getenv("TARS_API_TOKEN"), "api token")
	cmd.Flags().StringVar(&opts.adminToken, "admin-api-token", os.Getenv("TARS_ADMIN_API_TOKEN"), "admin api token")
	cmd.Flags().StringVar(&opts.message, "message", "", "send one message and exit")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "verbose status output")
	cmd.AddCommand(newServeCommand(stdout, stderr))
	return cmd
}

func newServeCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := serveOptions{
		serveAPI:          true,
		apiAddr:           "127.0.0.1:43180",
		heartbeatInterval: 30 * time.Minute,
	}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run TARS daemon server mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.runOnce && opts.runLoop {
				return fmt.Errorf("--run-once and --run-loop are mutually exclusive")
			}
			return serveRunner(cmd.Context(), normalizeServeOptions(opts), stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&opts.configPath, "config", "", "path to config file")
	cmd.Flags().StringVar(&opts.mode, "mode", "", "runtime mode override")
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", "", "workspace directory override")
	cmd.Flags().StringVar(&opts.logFile, "log-file", "", "append json logs to file")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "enable verbose debug logging")
	cmd.Flags().BoolVar(&opts.runOnce, "run-once", false, "run heartbeat once and exit")
	cmd.Flags().BoolVar(&opts.runLoop, "run-loop", false, "run heartbeat loop")
	cmd.Flags().BoolVar(&opts.serveAPI, "serve-api", true, "serve tars http api")
	cmd.Flags().StringVar(&opts.apiAddr, "api-addr", "127.0.0.1:43180", "http api listen address")
	cmd.Flags().DurationVar(&opts.heartbeatInterval, "heartbeat-interval", 30*time.Minute, "heartbeat interval (e.g. 30m, 5s)")
	cmd.Flags().IntVar(&opts.maxHeartbeats, "max-heartbeats", 0, "maximum heartbeat count in loop (0 means unlimited)")
	return cmd
}

func normalizeServeOptions(opts serveOptions) serveOptions {
	normalized := opts
	// Keep the previous behavior where run-once/run-loop mode does not start API serving.
	if normalized.runOnce || normalized.runLoop {
		normalized.serveAPI = false
	}
	return normalized
}

func runServeCommand(ctx context.Context, opts serveOptions, stdout, stderr io.Writer) error {
	return tarsapp.Serve(ctx, tarsapp.ServeOptions{
		ConfigPath:        strings.TrimSpace(opts.configPath),
		Mode:              strings.TrimSpace(opts.mode),
		WorkspaceDir:      strings.TrimSpace(opts.workspaceDir),
		LogFile:           strings.TrimSpace(opts.logFile),
		Verbose:           opts.verbose,
		RunOnce:           opts.runOnce,
		RunLoop:           opts.runLoop,
		ServeAPI:          opts.serveAPI,
		APIAddr:           strings.TrimSpace(opts.apiAddr),
		HeartbeatInterval: opts.heartbeatInterval,
		MaxHeartbeats:     opts.maxHeartbeats,
	}, stdout, stderr)
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
  /cron {list|get|runs|add|run|delete|enable|disable}
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
	default:
		return false
	}
}
