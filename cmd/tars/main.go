package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/cli"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithIO(args, os.Stdin, stdout, stderr)
}

func runWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = godotenv.Load(".env")
	logger := zerolog.New(stderr).With().Timestamp().Str("component", "tars").Logger()
	if hasVerboseFlag(args) {
		logger = logger.Level(zerolog.DebugLevel)
		logger.Debug().Msg("verbose logging enabled")
	}

	cmd := newRootCmd(stdin, stdout, stderr, logger)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		var ex *cli.ExitError
		if errors.As(err, &ex) {
			return ex.Code
		}
		logger.Error().Err(err).Msg("failed to parse command")
		if cli.IsFlagError(err) {
			return 2
		}
		return 1
	}
	return 0
}

func hasVerboseFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--verbose" {
			return true
		}
	}
	return false
}

func newRootCmd(stdin io.Reader, stdout, stderr io.Writer, logger zerolog.Logger) *cobra.Command {
	var verbose bool
	root := &cobra.Command{
		Use:           "tars",
		Short:         "CLI client for TARS",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose debug logging")
	root.AddCommand(newHeartbeatCmd(stdout, logger))
	root.AddCommand(newChatCmd(stdin, stdout, stderr, logger))

	return root
}

func newHeartbeatCmd(stdout io.Writer, logger zerolog.Logger) *cobra.Command {
	heartbeatCmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Heartbeat commands",
	}

	var serverURL string
	runOnceCmd := &cobra.Command{
		Use:   "run-once",
		Short: "Request one heartbeat run from tarsd",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runHeartbeatOnce(serverURL, stdout, logger)
		},
	}
	runOnceCmd.Flags().StringVar(&serverURL, "server-url", "http://127.0.0.1:8080", "tarsd API server URL")

	heartbeatCmd.AddCommand(runOnceCmd)
	return heartbeatCmd
}

func runHeartbeatOnce(serverURL string, stdout io.Writer, logger zerolog.Logger) error {
	base := strings.TrimRight(serverURL, "/")
	client := &http.Client{Timeout: 60 * time.Second}
	url := base + "/v1/heartbeat/run-once"

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	logger.Debug().Str("method", req.Method).Str("url", url).Msg("request tarsd")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request run-once endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read run-once response: %w", err)
	}
	logger.Debug().Int("status", resp.StatusCode).Int("bytes", len(body)).Msg("response from tarsd")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("run-once endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decode run-once response: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, parsed.Response)
	return nil
}

func newChatCmd(stdin io.Reader, stdout io.Writer, stderr io.Writer, logger zerolog.Logger) *cobra.Command {
	var serverURL string
	var sessionID string
	var message string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Send chat message or start interactive REPL",
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(message) != "" {
				return runChatMessage(serverURL, sessionID, message, stdout, stderr, logger)
			}
			return runChatREPL(serverURL, sessionID, stdin, stdout, stderr, logger)
		},
	}
	cmd.Flags().StringVar(&serverURL, "server-url", "http://127.0.0.1:8080", "tarsd API server URL")
	cmd.Flags().StringVar(&sessionID, "session", "", "session id")
	cmd.Flags().StringVarP(&message, "message", "m", "", "chat message")
	return cmd
}

func runChatMessage(serverURL, sessionID, message string, stdout io.Writer, statusOut io.Writer, logger zerolog.Logger) error {
	_, err := runChatMessageWithSession(serverURL, sessionID, message, stdout, statusOut, logger)
	return err
}

func runChatMessageWithSession(serverURL, sessionID, message string, stdout io.Writer, statusOut io.Writer, logger zerolog.Logger) (string, error) {
	base := strings.TrimRight(serverURL, "/")
	client := &http.Client{Timeout: 5 * time.Minute}
	url := base + "/v1/chat"
	currentSessionID := strings.TrimSpace(sessionID)

	reqBody := map[string]string{
		"message": strings.TrimSpace(message),
	}
	if currentSessionID != "" {
		reqBody["session_id"] = currentSessionID
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, base+"/v1/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	logger.Debug().
		Str("method", req.Method).
		Str("url", url).
		Str("session_id", currentSessionID).
		Int("message_len", len(strings.TrimSpace(message))).
		Msg("request tarsd chat api")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request chat endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		logger.Debug().Int("status", resp.StatusCode).Int("bytes", len(body)).Msg("chat api error response")
		return "", fmt.Errorf("chat endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	logger.Debug().Int("status", resp.StatusCode).Msg("chat stream connected")

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if payload == "" {
			continue
		}

		var evt struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Error     string `json:"error"`
			SessionID string `json:"session_id"`
			Phase     string `json:"phase"`
			Message   string `json:"message"`
			ToolName  string `json:"tool_name"`
		}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			return "", fmt.Errorf("decode sse event: %w", err)
		}

		switch evt.Type {
		case "delta":
			logger.Debug().Int("delta_len", len(evt.Text)).Msg("chat delta")
			_, _ = fmt.Fprint(stdout, evt.Text)
		case "status":
			statusMessage := strings.TrimSpace(evt.Message)
			if statusMessage == "" {
				statusMessage = strings.TrimSpace(evt.Phase)
			}
			if strings.TrimSpace(evt.ToolName) != "" {
				statusMessage = fmt.Sprintf("%s (%s)", statusMessage, strings.TrimSpace(evt.ToolName))
			}
			if statusMessage != "" {
				logger.Debug().Str("phase", strings.TrimSpace(evt.Phase)).Str("message", statusMessage).Msg("chat status")
				_, _ = fmt.Fprintf(statusOut, "[status] %s\n", statusMessage)
			}
		case "error":
			logger.Debug().Str("error", strings.TrimSpace(evt.Error)).Msg("chat stream error")
			return "", fmt.Errorf("chat api error: %s", strings.TrimSpace(evt.Error))
		case "done":
			logger.Debug().Msg("chat stream done")
			_, _ = fmt.Fprintln(stdout)
			if strings.TrimSpace(evt.SessionID) != "" {
				currentSessionID = strings.TrimSpace(evt.SessionID)
			}
			return currentSessionID, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read chat stream: %w", err)
	}

	_, _ = fmt.Fprintln(stdout)
	return currentSessionID, nil
}

func runChatREPL(serverURL, sessionID string, stdin io.Reader, stdout io.Writer, statusOut io.Writer, logger zerolog.Logger) error {
	currentSessionID := strings.TrimSpace(sessionID)
	scanner := bufio.NewScanner(stdin)

	_, _ = fmt.Fprintln(stdout, "Entering chat REPL. Type /help for commands.")
	for {
		_, _ = fmt.Fprint(stdout, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read repl input: %w", err)
			}
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "/exit" || line == "/quit" {
			return nil
		}
		if strings.HasPrefix(line, "/") {
			handled, nextSessionID, err := handleREPLCommand(serverURL, currentSessionID, line, scanner, stdout, logger)
			if err != nil {
				_, _ = fmt.Fprintf(stdout, "error: %v\n", err)
				continue
			}
			if strings.TrimSpace(nextSessionID) != "" {
				currentSessionID = strings.TrimSpace(nextSessionID)
			}
			if handled {
				continue
			}
		}

		nextSessionID, err := runChatMessageWithSession(serverURL, currentSessionID, line, stdout, statusOut, logger)
		if err != nil {
			return err
		}
		if strings.TrimSpace(nextSessionID) != "" {
			currentSessionID = strings.TrimSpace(nextSessionID)
		}
	}
}

func handleREPLCommand(serverURL, currentSessionID, line string, scanner *bufio.Scanner, stdout io.Writer, logger zerolog.Logger) (bool, string, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return true, "", nil
	}

	switch fields[0] {
	case "/help":
		_, _ = fmt.Fprintln(stdout, "Commands: /sessions, /new [title], /resume {id}, /history, /status, /compact, /quit")
		return true, "", nil
	case "/sessions":
		return true, "", printSessions(serverURL, stdout, logger)
	case "/new":
		title := strings.TrimSpace(strings.TrimPrefix(line, "/new"))
		if title == "" {
			title = "chat"
		}
		id, err := createSession(serverURL, title, logger)
		if err != nil {
			return true, "", err
		}
		_, _ = fmt.Fprintf(stdout, "active session: %s\n", id)
		return true, id, nil
	case "/resume":
		if len(fields) >= 2 {
			id := strings.TrimSpace(fields[1])
			if err := ensureSessionExists(serverURL, id, logger); err != nil {
				return true, "", err
			}
			_, _ = fmt.Fprintf(stdout, "resumed session: %s\n", id)
			return true, id, nil
		}

		sessions, err := fetchSessions(serverURL, logger)
		if err != nil {
			return true, "", err
		}
		if len(sessions) == 0 {
			_, _ = fmt.Fprintln(stdout, "(no sessions)")
			return true, "", nil
		}
		_, _ = fmt.Fprintln(stdout, "Select session:")
		for i, s := range sessions {
			_, _ = fmt.Fprintf(stdout, "%d) %s\t%s\n", i+1, s.ID, s.Title)
		}
		_, _ = fmt.Fprint(stdout, "number> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return true, "", fmt.Errorf("read resume selection: %w", err)
			}
			return true, "", fmt.Errorf("input closed while selecting session")
		}
		choice := strings.TrimSpace(scanner.Text())
		idx, err := strconv.Atoi(choice)
		if err != nil || idx < 1 || idx > len(sessions) {
			_, _ = fmt.Fprintln(stdout, "invalid selection")
			return true, "", nil
		}
		id := sessions[idx-1].ID
		_, _ = fmt.Fprintf(stdout, "resumed session: %s\n", id)
		return true, id, nil
	case "/history":
		if strings.TrimSpace(currentSessionID) == "" {
			return true, "", fmt.Errorf("no active session. use /new or /resume {session_id}")
		}
		return true, "", printHistory(serverURL, currentSessionID, stdout, logger)
	case "/status":
		return true, "", printStatus(serverURL, stdout, logger)
	case "/compact":
		return true, "", runCompact(serverURL, stdout, logger)
	default:
		_, _ = fmt.Fprintf(stdout, "unknown command: %s\n", fields[0])
		return true, "", nil
	}
}

type sessionSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type sessionHistoryItem struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func fetchSessions(serverURL string, logger zerolog.Logger) ([]sessionSummary, error) {
	base := strings.TrimRight(serverURL, "/")
	url := base + "/v1/sessions"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	logger.Debug().Str("method", req.Method).Str("url", url).Msg("request tarsd sessions api")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request sessions endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sessions response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sessions endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var sessions []sessionSummary
	if err := json.Unmarshal(body, &sessions); err != nil {
		return nil, fmt.Errorf("decode sessions response: %w", err)
	}
	return sessions, nil
}

func printSessions(serverURL string, stdout io.Writer, logger zerolog.Logger) error {
	sessions, err := fetchSessions(serverURL, logger)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		_, _ = fmt.Fprintln(stdout, "(no sessions)")
		return nil
	}
	for _, s := range sessions {
		_, _ = fmt.Fprintf(stdout, "%s\t%s\n", s.ID, s.Title)
	}
	return nil
}

func createSession(serverURL, title string, logger zerolog.Logger) (string, error) {
	base := strings.TrimRight(serverURL, "/")
	url := base + "/v1/sessions"
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return "", fmt.Errorf("encode create session request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	logger.Debug().Str("method", req.Method).Str("url", url).Str("title", title).Msg("request tarsd create session api")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("request create session endpoint: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read create session response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create session endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var created sessionSummary
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", fmt.Errorf("decode create session response: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("create session response missing id")
	}
	return strings.TrimSpace(created.ID), nil
}

func ensureSessionExists(serverURL, sessionID string, logger zerolog.Logger) error {
	base := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/v1/sessions/%s", base, strings.TrimSpace(sessionID))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	logger.Debug().Str("method", req.Method).Str("url", url).Msg("request tarsd get session api")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("request session endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read session response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("session endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func printHistory(serverURL, sessionID string, stdout io.Writer, logger zerolog.Logger) error {
	base := strings.TrimRight(serverURL, "/")
	url := fmt.Sprintf("%s/v1/sessions/%s/history", base, strings.TrimSpace(sessionID))
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	logger.Debug().Str("method", req.Method).Str("url", url).Msg("request tarsd history api")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("request history endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read history response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("history endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var messages []sessionHistoryItem
	if err := json.Unmarshal(body, &messages); err != nil {
		return fmt.Errorf("decode history response: %w", err)
	}
	if len(messages) == 0 {
		_, _ = fmt.Fprintln(stdout, "(no history)")
		return nil
	}
	for _, m := range messages {
		_, _ = fmt.Fprintf(stdout, "%s [%s] %s\n", m.Timestamp.Format(time.RFC3339), m.Role, m.Content)
	}
	return nil
}

func printStatus(serverURL string, stdout io.Writer, logger zerolog.Logger) error {
	base := strings.TrimRight(serverURL, "/")
	url := base + "/v1/status"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	logger.Debug().Str("method", req.Method).Str("url", url).Msg("request tarsd status api")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("request status endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read status response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		WorkspaceDir string `json:"workspace_dir"`
		SessionCount int    `json:"session_count"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decode status response: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "workspace=%s sessions=%d\n", parsed.WorkspaceDir, parsed.SessionCount)
	return nil
}

func runCompact(serverURL string, stdout io.Writer, logger zerolog.Logger) error {
	base := strings.TrimRight(serverURL, "/")
	url := base + "/v1/compact"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	logger.Debug().Str("method", req.Method).Str("url", url).Msg("request tarsd compact api")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("request compact endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read compact response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("compact endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decode compact response: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, strings.TrimSpace(parsed.Message))
	return nil
}
