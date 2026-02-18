package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type options struct {
	serverURL   string
	sessionID   string
	apiToken    string
	adminToken  string
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
			chat := chatClient{
				serverURL:   opts.serverURL,
				apiToken:    opts.apiToken,
				workspaceID: opts.workspaceID,
			}
			runtime := runtimeClient{
				serverURL:     opts.serverURL,
				apiToken:      opts.apiToken,
				adminAPIToken: opts.adminToken,
				workspaceID:   opts.workspaceID,
			}
			session := strings.TrimSpace(opts.sessionID)
			if strings.TrimSpace(opts.message) != "" {
				res, err := sendMessage(cmd.Context(), chat, session, opts.message, opts.verbose, stdout, stderr)
				if err != nil {
					return err
				}
				if res.SessionID != "" {
					fmt.Fprintf(stderr, "session=%s\n", res.SessionID)
				}
				return nil
			}
			return runREPL(cmd.Context(), stdin, stdout, stderr, chat, runtime, session, opts.verbose)
		},
	}
	cmd.Flags().StringVar(&opts.serverURL, "server-url", os.Getenv("TARS_SERVER_URL"), "tarsd server url")
	cmd.Flags().StringVar(&opts.sessionID, "session", "", "session id")
	cmd.Flags().StringVar(&opts.apiToken, "api-token", os.Getenv("TARS_API_TOKEN"), "api token")
	cmd.Flags().StringVar(&opts.adminToken, "admin-api-token", os.Getenv("TARS_ADMIN_API_TOKEN"), "admin api token")
	cmd.Flags().StringVar(&opts.workspaceID, "workspace-id", os.Getenv("TARS_WORKSPACE_ID"), "workspace id header")
	cmd.Flags().StringVar(&opts.message, "message", "", "send one message and exit")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "verbose status output")
	return cmd
}

func runREPL(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, chat chatClient, runtime runtimeClient, session string, verbose bool) error {
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
		if strings.HasPrefix(line, "/") {
			handled, nextSession, err := executeCommand(ctx, runtime, line, session, stdout, stderr)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				continue
			}
			if handled {
				session = nextSession
				continue
			}
		}
		res, err := sendMessage(ctx, chat, session, line, verbose, stdout, stderr)
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

func executeCommand(ctx context.Context, runtime runtimeClient, line, session string, stdout, stderr io.Writer) (bool, string, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return true, session, nil
	}
	switch fields[0] {
	case "/agents":
		agents, err := runtime.listAgents(ctx)
		if err != nil {
			return true, session, err
		}
		if len(agents) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no agents)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > agents")
		for _, a := range agents {
			fmt.Fprintf(stdout, "- %s kind=%s source=%s policy=%s\n", a.Name, a.Kind, a.Source, a.PolicyMode)
		}
		return true, session, nil
	case "/runs":
		limit := 30
		if len(fields) > 1 {
			n, err := parseOptionalLimit(fields[1], 30)
			if err != nil {
				return true, session, err
			}
			limit = n
		}
		runs, err := runtime.listRuns(ctx, limit)
		if err != nil {
			return true, session, err
		}
		if len(runs) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no runs)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > runs")
		for _, r := range runs {
			fmt.Fprintf(stdout, "- %s status=%s agent=%s session=%s\n", r.RunID, r.Status, r.Agent, r.SessionID)
		}
		return true, session, nil
	case "/run":
		if len(fields) < 2 {
			return true, session, fmt.Errorf("usage: /run {id}")
		}
		run, err := runtime.getRun(ctx, fields[1])
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > run %s status=%s agent=%s session=%s\n", run.RunID, run.Status, run.Agent, run.SessionID)
		if strings.TrimSpace(run.Response) != "" {
			fmt.Fprintf(stdout, "response: %s\n", run.Response)
		}
		if strings.TrimSpace(run.Error) != "" {
			fmt.Fprintf(stdout, "error: %s\n", run.Error)
		}
		return true, session, nil
	case "/cancel-run":
		if len(fields) < 2 {
			return true, session, fmt.Errorf("usage: /cancel-run {id}")
		}
		run, err := runtime.cancelRun(ctx, fields[1])
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > canceled %s status=%s\n", run.RunID, run.Status)
		return true, session, nil
	case "/spawn":
		raw := strings.TrimSpace(strings.TrimPrefix(line, "/spawn"))
		sp, err := parseSpawnCommand(raw)
		if err != nil {
			return true, session, fmt.Errorf("usage: /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}: %w", err)
		}
		run, err := runtime.spawnRun(ctx, spawnRequest{SessionID: sp.SessionID, Title: sp.Title, Message: sp.Message, Agent: sp.Agent})
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > spawned run_id=%s status=%s\n", run.RunID, run.Status)
		if !sp.Wait {
			return true, session, nil
		}
		finalRun, err := waitRun(ctx, runtime, run.RunID, time.Second)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > run %s completed status=%s\n", finalRun.RunID, finalRun.Status)
		if strings.TrimSpace(finalRun.Response) != "" {
			fmt.Fprintf(stdout, "%s\n", finalRun.Response)
		}
		if strings.TrimSpace(finalRun.SessionID) != "" {
			fmt.Fprintf(stderr, "session=%s\n", finalRun.SessionID)
			return true, finalRun.SessionID, nil
		}
		return true, session, nil
	case "/gateway":
		action := "status"
		if len(fields) > 1 {
			action = strings.TrimSpace(fields[1])
		}
		var (
			status gatewayStatus
			err    error
		)
		switch action {
		case "status":
			status, err = runtime.gatewayStatus(ctx)
		case "reload":
			status, err = runtime.gatewayReload(ctx)
		case "restart":
			status, err = runtime.gatewayRestart(ctx)
		default:
			return true, session, fmt.Errorf("usage: /gateway {status|reload|restart}")
		}
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > gateway enabled=%t version=%d\n", status.Enabled, status.Version)
		return true, session, nil
	default:
		return false, session, nil
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
