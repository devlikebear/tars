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
	case "/help":
		fmt.Fprintln(stdout, "SYSTEM > commands: /help /session /resume {id} /new [title] /sessions /history /export /search {keyword} /status /compact /heartbeat /skills /plugins /mcp /reload /agents [--detail] /runs [limit] /run {id} /cancel-run {id} /spawn [...] /gateway {status|reload|restart} /channels /cron {list|get|runs|add|run|delete|enable|disable} /quit")
		return true, session, nil
	case "/session":
		fmt.Fprintf(stdout, "SYSTEM > session=%s\n", session)
		return true, session, nil
	case "/resume":
		if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
			return true, session, fmt.Errorf("usage: /resume {session_id}")
		}
		next := strings.TrimSpace(fields[1])
		fmt.Fprintf(stdout, "SYSTEM > resumed session=%s\n", next)
		fmt.Fprintf(stderr, "session=%s\n", next)
		return true, next, nil
	case "/new":
		title := strings.TrimSpace(strings.TrimPrefix(line, "/new"))
		if title == "" {
			title = "chat"
		}
		created, err := runtime.createSession(ctx, title)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > created session %s (%s)\n", created.ID, created.Title)
		fmt.Fprintf(stderr, "session=%s\n", created.ID)
		return true, created.ID, nil
	case "/sessions":
		sessions, err := runtime.listSessions(ctx)
		if err != nil {
			return true, session, err
		}
		if len(sessions) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no sessions)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > sessions")
		for _, s := range sessions {
			fmt.Fprintf(stdout, "- %s %s\n", s.ID, s.Title)
		}
		return true, session, nil
	case "/history":
		if strings.TrimSpace(session) == "" {
			return true, session, fmt.Errorf("history requires active session; use /new or --session")
		}
		messages, err := runtime.getHistory(ctx, session)
		if err != nil {
			return true, session, err
		}
		if len(messages) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no history)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > history")
		for _, m := range messages {
			content := strings.TrimSpace(m.Content)
			if len(content) > 120 {
				content = content[:117] + "..."
			}
			fmt.Fprintf(stdout, "- %s: %s\n", strings.TrimSpace(m.Role), content)
		}
		return true, session, nil
	case "/export":
		if strings.TrimSpace(session) == "" {
			return true, session, fmt.Errorf("export requires active session; use /new or --session")
		}
		markdown, err := runtime.exportSession(ctx, session)
		if err != nil {
			return true, session, err
		}
		fmt.Fprint(stdout, markdown)
		if !strings.HasSuffix(markdown, "\n") {
			fmt.Fprintln(stdout)
		}
		return true, session, nil
	case "/search":
		keyword := strings.TrimSpace(strings.TrimPrefix(line, "/search"))
		if keyword == "" {
			return true, session, fmt.Errorf("usage: /search {keyword}")
		}
		results, err := runtime.searchSessions(ctx, keyword)
		if err != nil {
			return true, session, err
		}
		if len(results) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no matched sessions)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > matched sessions")
		for _, s := range results {
			fmt.Fprintf(stdout, "- %s %s\n", s.ID, s.Title)
		}
		return true, session, nil
	case "/status":
		status, err := runtime.status(ctx)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > workspace=%s sessions=%d", status.WorkspaceDir, status.SessionCount)
		if strings.TrimSpace(status.WorkspaceID) != "" {
			fmt.Fprintf(stdout, " workspace_id=%s", status.WorkspaceID)
		}
		if strings.TrimSpace(status.AuthRole) != "" {
			fmt.Fprintf(stdout, " auth_role=%s", status.AuthRole)
		}
		fmt.Fprintln(stdout)
		return true, session, nil
	case "/compact":
		if strings.TrimSpace(session) == "" {
			return true, session, fmt.Errorf("compact requires active session; use /new or --session")
		}
		result, err := runtime.compact(ctx, session)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > %s\n", strings.TrimSpace(result.Message))
		return true, session, nil
	case "/heartbeat":
		result, err := runtime.heartbeatRunOnce(ctx)
		if err != nil {
			return true, session, err
		}
		if result.Skipped {
			fmt.Fprintf(stdout, "SYSTEM > skipped: %s\n", strings.TrimSpace(result.SkipReason))
			return true, session, nil
		}
		fmt.Fprintf(stdout, "SYSTEM > %s\n", strings.TrimSpace(result.Response))
		return true, session, nil
	case "/skills":
		skills, err := runtime.listSkills(ctx)
		if err != nil {
			return true, session, err
		}
		if len(skills) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no skills)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > skills")
		for _, s := range skills {
			fmt.Fprintf(stdout, "- %s invocable=%t source=%s\n", s.Name, s.UserInvocable, s.Source)
		}
		return true, session, nil
	case "/plugins":
		plugins, err := runtime.listPlugins(ctx)
		if err != nil {
			return true, session, err
		}
		if len(plugins) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no plugins)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > plugins")
		for _, p := range plugins {
			fmt.Fprintf(stdout, "- %s source=%s version=%s\n", p.ID, p.Source, p.Version)
		}
		return true, session, nil
	case "/mcp":
		servers, err := runtime.listMCPServers(ctx)
		if err != nil {
			return true, session, err
		}
		tools, err := runtime.listMCPTools(ctx)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > mcp servers=%d tools=%d\n", len(servers), len(tools))
		for _, s := range servers {
			fmt.Fprintf(stdout, "- %s connected=%t tools=%d\n", s.Name, s.Connected, s.ToolCount)
		}
		return true, session, nil
	case "/reload":
		result, err := runtime.reloadExtensions(ctx)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > reloaded=%t version=%d skills=%d plugins=%d mcp=%d gateway_refreshed=%t gateway_agents=%d\n",
			result.Reloaded, result.Version, result.Skills, result.Plugins, result.MCPCount, result.GatewayRefreshed, result.GatewayAgents)
		return true, session, nil
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
		detail := len(fields) > 1 && strings.TrimSpace(fields[1]) == "--detail"
		for _, a := range agents {
			if detail {
				fmt.Fprintf(stdout, "- %s kind=%s source=%s entry=%s policy=%s allow=%d routing=%s fixed_session=%s\n",
					a.Name, a.Kind, a.Source, a.Entry, a.PolicyMode, a.ToolsAllowCount, a.SessionRoutingMode, a.SessionFixedID)
				continue
			}
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
	case "/channels":
		status, err := runtime.gatewayStatus(ctx)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > channels_local=%t channels_webhook=%t channels_telegram=%t\n",
			status.ChannelsLocalEnabled,
			status.ChannelsWebhookEnabled,
			status.ChannelsTelegramEnabled,
		)
		return true, session, nil
	case "/cron":
		if len(fields) == 1 || strings.TrimSpace(fields[1]) == "list" {
			jobs, err := runtime.listCronJobs(ctx)
			if err != nil {
				return true, session, err
			}
			if len(jobs) == 0 {
				fmt.Fprintln(stdout, "SYSTEM > (no cron jobs)")
				return true, session, nil
			}
			fmt.Fprintln(stdout, "SYSTEM > cron jobs")
			for _, job := range jobs {
				fmt.Fprintf(stdout, "- %s name=%s schedule=%s enabled=%t\n", job.ID, job.Name, job.Schedule, job.Enabled)
			}
			return true, session, nil
		}
		sub := strings.TrimSpace(fields[1])
		switch sub {
		case "add":
			if len(fields) < 4 {
				return true, session, fmt.Errorf("usage: /cron add {schedule} {prompt}")
			}
			schedule := strings.TrimSpace(fields[2])
			prompt := strings.TrimSpace(strings.TrimPrefix(line, "/cron add "+schedule))
			if schedule == "" || prompt == "" {
				return true, session, fmt.Errorf("usage: /cron add {schedule} {prompt}")
			}
			job, err := runtime.createCronJob(ctx, schedule, prompt)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > created cron job %s schedule=%s\n", job.ID, job.Schedule)
			return true, session, nil
		case "run":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /cron run {job_id}")
			}
			response, err := runtime.runCronJob(ctx, fields[2])
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > %s\n", response)
			return true, session, nil
		case "get":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /cron get {job_id}")
			}
			job, err := runtime.getCronJob(ctx, fields[2])
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > %s name=%s schedule=%s enabled=%t\n", job.ID, job.Name, job.Schedule, job.Enabled)
			return true, session, nil
		case "runs":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /cron runs {job_id} [limit]")
			}
			limit := 20
			if len(fields) > 3 {
				n, err := parseOptionalLimit(fields[3], 20)
				if err != nil {
					return true, session, fmt.Errorf("usage: /cron runs {job_id} [limit]")
				}
				limit = n
			}
			runs, err := runtime.listCronRuns(ctx, fields[2], limit)
			if err != nil {
				return true, session, err
			}
			if len(runs) == 0 {
				fmt.Fprintln(stdout, "SYSTEM > (no cron runs)")
				return true, session, nil
			}
			fmt.Fprintln(stdout, "SYSTEM > cron runs")
			for _, run := range runs {
				if strings.TrimSpace(run.Error) != "" {
					fmt.Fprintf(stdout, "- %s error=%s\n", run.RanAt, run.Error)
					continue
				}
				fmt.Fprintf(stdout, "- %s response=%s\n", run.RanAt, strings.TrimSpace(run.Response))
			}
			return true, session, nil
		case "delete":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /cron delete {job_id}")
			}
			if err := runtime.deleteCronJob(ctx, fields[2]); err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > deleted cron job %s\n", strings.TrimSpace(fields[2]))
			return true, session, nil
		case "enable", "disable":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /cron %s {job_id}", sub)
			}
			enabled := sub == "enable"
			job, err := runtime.updateCronJobEnabled(ctx, fields[2], enabled)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > %s enabled=%t\n", job.ID, job.Enabled)
			return true, session, nil
		default:
			return true, session, fmt.Errorf("usage: /cron {list|get|runs|add|run|delete|enable|disable}")
		}
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
