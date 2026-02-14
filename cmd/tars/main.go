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
	_ = godotenv.Load(".env")
	logger := zerolog.New(stderr).With().Timestamp().Str("component", "tars").Logger()
	if hasVerboseFlag(args) {
		logger = logger.Level(zerolog.DebugLevel)
		logger.Debug().Msg("verbose logging enabled")
	}

	cmd := newRootCmd(stdout, stderr, logger)
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

func newRootCmd(stdout, stderr io.Writer, logger zerolog.Logger) *cobra.Command {
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
	root.AddCommand(newChatCmd(stdout, logger))

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

func newChatCmd(stdout io.Writer, logger zerolog.Logger) *cobra.Command {
	var serverURL string
	var sessionID string
	var message string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Send a chat message to tarsd",
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(message) == "" {
				return fmt.Errorf("--message (-m) is required")
			}
			return runChatMessage(serverURL, sessionID, message, stdout, logger)
		},
	}
	cmd.Flags().StringVar(&serverURL, "server-url", "http://127.0.0.1:8080", "tarsd API server URL")
	cmd.Flags().StringVar(&sessionID, "session", "", "session id")
	cmd.Flags().StringVarP(&message, "message", "m", "", "chat message")
	return cmd
}

func runChatMessage(serverURL, sessionID, message string, stdout io.Writer, logger zerolog.Logger) error {
	base := strings.TrimRight(serverURL, "/")
	client := &http.Client{Timeout: 5 * time.Minute}
	url := base + "/v1/chat"

	reqBody := map[string]string{
		"message": strings.TrimSpace(message),
	}
	if strings.TrimSpace(sessionID) != "" {
		reqBody["session_id"] = strings.TrimSpace(sessionID)
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encode chat request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, base+"/v1/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	logger.Debug().
		Str("method", req.Method).
		Str("url", url).
		Str("session_id", strings.TrimSpace(sessionID)).
		Int("message_len", len(strings.TrimSpace(message))).
		Msg("request tarsd chat api")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request chat endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		logger.Debug().Int("status", resp.StatusCode).Int("bytes", len(body)).Msg("chat api error response")
		return fmt.Errorf("chat endpoint status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
			Type  string `json:"type"`
			Text  string `json:"text"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			return fmt.Errorf("decode sse event: %w", err)
		}

		switch evt.Type {
		case "delta":
			logger.Debug().Int("delta_len", len(evt.Text)).Msg("chat delta")
			_, _ = fmt.Fprint(stdout, evt.Text)
		case "error":
			logger.Debug().Str("error", strings.TrimSpace(evt.Error)).Msg("chat stream error")
			return fmt.Errorf("chat api error: %s", strings.TrimSpace(evt.Error))
		case "done":
			logger.Debug().Msg("chat stream done")
			_, _ = fmt.Fprintln(stdout)
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read chat stream: %w", err)
	}

	_, _ = fmt.Fprintln(stdout)
	return nil
}
