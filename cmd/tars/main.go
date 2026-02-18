package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type options struct {
	serverURL   string
	sessionID   string
	apiToken    string
	workspaceID string
	message     string
	verbose     bool
}

func main() {
	if err := newRootCommand(os.Stdin, os.Stdout, os.Stderr).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "tars",
		Short: "Go TUI-lite client for tarsd",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client := chatClient{
				serverURL:   opts.serverURL,
				apiToken:    opts.apiToken,
				workspaceID: opts.workspaceID,
			}
			session := strings.TrimSpace(opts.sessionID)
			if strings.TrimSpace(opts.message) != "" {
				res, err := sendMessage(cmd.Context(), client, session, opts.message, opts.verbose, stdout, stderr)
				if err != nil {
					return err
				}
				if res.SessionID != "" {
					fmt.Fprintf(stderr, "session=%s\n", res.SessionID)
				}
				return nil
			}
			return runREPL(cmd.Context(), stdin, stdout, stderr, client, session, opts.verbose)
		},
	}
	cmd.Flags().StringVar(&opts.serverURL, "server-url", os.Getenv("TARS_SERVER_URL"), "tarsd server url")
	cmd.Flags().StringVar(&opts.sessionID, "session", "", "session id")
	cmd.Flags().StringVar(&opts.apiToken, "api-token", os.Getenv("TARS_API_TOKEN"), "api token")
	cmd.Flags().StringVar(&opts.workspaceID, "workspace-id", os.Getenv("TARS_WORKSPACE_ID"), "workspace id header")
	cmd.Flags().StringVar(&opts.message, "message", "", "send one message and exit")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "verbose status output")
	return cmd
}

func runREPL(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, client chatClient, session string, verbose bool) error {
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 8*1024), 2*1024*1024)
	for {
		fmt.Fprint(stdout, "You > ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch line {
		case "/exit", "/quit":
			return nil
		case "/new":
			session = ""
			fmt.Fprintln(stderr, "session reset")
			continue
		case "/session":
			fmt.Fprintf(stderr, "session=%s\n", session)
			continue
		}
		res, err := sendMessage(ctx, client, session, line, verbose, stdout, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			continue
		}
		session = res.SessionID
		if session != "" {
			fmt.Fprintf(stderr, "session=%s\n", session)
		}
	}
}

func sendMessage(ctx context.Context, client chatClient, session, message string, verbose bool, stdout, stderr io.Writer) (chatResult, error) {
	fmt.Fprint(stdout, "TARS > ")
	res, err := client.stream(ctx, chatRequest{Message: message, SessionID: session}, func(evt chatEvent) {
		if !verbose {
			return
		}
		label := strings.TrimSpace(evt.Message)
		if label == "" {
			label = strings.TrimSpace(evt.Phase)
		}
		if label != "" {
			fmt.Fprintf(stderr, "status: %s\n", label)
		}
	}, func(chunk string) {
		fmt.Fprint(stdout, chunk)
	})
	fmt.Fprintln(stdout)
	return res, err
}
