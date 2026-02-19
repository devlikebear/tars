package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

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

type localRuntimeState struct {
	notifications   *notificationCenter
	chatTrace       bool
	chatTraceFilter string
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
		Short: "Go TUI client for tarsd",
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
	cmd.Flags().StringVar(&opts.serverURL, "server-url", os.Getenv("TARS_SERVER_URL"), "tarsd server url")
	cmd.Flags().StringVar(&opts.sessionID, "session", "", "session id")
	cmd.Flags().StringVar(&opts.apiToken, "api-token", os.Getenv("TARS_API_TOKEN"), "api token")
	cmd.Flags().StringVar(&opts.adminToken, "admin-api-token", os.Getenv("TARS_ADMIN_API_TOKEN"), "admin api token")
	cmd.Flags().StringVar(&opts.message, "message", "", "send one message and exit")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "verbose status output")
	return cmd
}

func executeCommand(ctx context.Context, runtime runtimeClient, line, session string, stdout, stderr io.Writer) (bool, string, error) {
	return executeCommandWithState(ctx, runtime, line, session, stdout, stderr, nil)
}

func executeCommandWithState(ctx context.Context, runtime runtimeClient, line, session string, stdout, stderr io.Writer, state *localRuntimeState) (bool, string, error) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return true, session, nil
	}
	switch fields[0] {
	case "/help":
		fmt.Fprintln(stdout, helpText())
		return true, session, nil
	case "/session":
		fmt.Fprintf(stdout, "SYSTEM > session=%s\n", session)
		return true, session, nil
	case "/resume":
		if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
			sessions, err := runtime.listSessions(ctx)
			if err != nil {
				return true, session, err
			}
			if len(sessions) == 0 {
				return true, session, fmt.Errorf("no sessions available; use /new first")
			}
			fmt.Fprintln(stdout, "SYSTEM > resume targets")
			for i, s := range sessions {
				fmt.Fprintf(stdout, "%d. %s %s\n", i+1, s.ID, s.Title)
			}
			fmt.Fprintln(stdout, "SYSTEM > use /resume {number|id|latest}")
			return true, session, nil
		}
		arg := strings.TrimSpace(fields[1])
		next := ""
		if strings.EqualFold(arg, "latest") {
			sessions, err := runtime.listSessions(ctx)
			if err != nil {
				return true, session, err
			}
			if len(sessions) == 0 {
				return true, session, fmt.Errorf("no sessions available; use /new first")
			}
			next = strings.TrimSpace(sessions[0].ID)
		} else if n, err := strconv.Atoi(arg); err == nil {
			sessions, listErr := runtime.listSessions(ctx)
			if listErr != nil {
				return true, session, listErr
			}
			if len(sessions) == 0 {
				return true, session, fmt.Errorf("no sessions available; use /new first")
			}
			if n <= 0 || n > len(sessions) {
				return true, session, fmt.Errorf("resume target out of range: %d", n)
			}
			next = strings.TrimSpace(sessions[n-1].ID)
		} else {
			next = arg
		}
		if next == "" {
			return true, session, fmt.Errorf("resume target is empty")
		}
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
		scope := strings.TrimSpace(status.WorkspaceID)
		if scope == "" {
			scope = "default"
		}
		fmt.Fprintf(stdout, "SYSTEM > workspace=%s sessions=%d scope=%s", status.WorkspaceDir, status.SessionCount, scope)
		if strings.TrimSpace(status.WorkspaceID) != "" {
			fmt.Fprintf(stdout, " workspace_id=%s", strings.TrimSpace(status.WorkspaceID))
		}
		if strings.TrimSpace(status.AuthRole) != "" {
			fmt.Fprintf(stdout, " auth_role=%s", status.AuthRole)
		}
		fmt.Fprintln(stdout)
		return true, session, nil
	case "/whoami":
		identity, err := runtime.whoami(ctx)
		if err != nil {
			return true, session, err
		}
		role := strings.TrimSpace(identity.AuthRole)
		if role == "" {
			role = "anonymous"
		}
		scope := strings.TrimSpace(identity.WorkspaceID)
		if scope == "" {
			scope = "default"
		}
		mode := strings.TrimSpace(identity.AuthMode)
		if mode == "" {
			mode = "external-required"
		}
		fmt.Fprintf(stdout, "SYSTEM > authenticated=%t role=%s admin=%t workspace=%s mode=%s\n",
			identity.Authenticated, role, identity.IsAdmin, scope, mode)
		return true, session, nil
	case "/health":
		status, err := runtime.healthz(ctx)
		if err != nil {
			return true, session, err
		}
		fmt.Fprintf(stdout, "SYSTEM > ok=%t component=%s time=%s\n", status.OK, status.Component, status.Time)
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
		sort.SliceStable(agents, func(i, j int) bool {
			return strings.TrimSpace(agents[i].Name) < strings.TrimSpace(agents[j].Name)
		})
		if len(agents) == 0 {
			fmt.Fprintln(stdout, "SYSTEM > (no agents)")
			return true, session, nil
		}
		fmt.Fprintln(stdout, "SYSTEM > agents")
		detail := len(fields) > 1 && (strings.TrimSpace(fields[1]) == "--detail" || strings.TrimSpace(fields[1]) == "-d")
		if detail {
			fmt.Fprintln(stdout, "NAME         KIND     SOURCE      POLICY     ROUTING  ALLOW DENY RISK")
		}
		for _, a := range agents {
			if detail {
				risk := strings.TrimSpace(a.ToolsRiskMax)
				if risk == "" {
					risk = "-"
				}
				fmt.Fprintf(stdout, "%-12s %-8s %-11s %-10s %-8s %-5d %-4d %s\n",
					a.Name, a.Kind, a.Source, a.PolicyMode, a.SessionRoutingMode, a.ToolsAllowCount, a.ToolsDenyCount, risk)
				fmt.Fprintf(stdout, "  entry=%s allow=%d deny=%d risk_max=%s routing=%s",
					a.Entry, a.ToolsAllowCount, a.ToolsDenyCount, risk, a.SessionRoutingMode)
				if strings.TrimSpace(a.SessionFixedID) != "" {
					fmt.Fprintf(stdout, " fixed_session=%s", strings.TrimSpace(a.SessionFixedID))
				}
				fmt.Fprintln(stdout)
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
		sort.SliceStable(runs, func(i, j int) bool {
			left := strings.TrimSpace(runs[i].CreatedAt)
			right := strings.TrimSpace(runs[j].CreatedAt)
			if left == right {
				return strings.TrimSpace(runs[i].RunID) > strings.TrimSpace(runs[j].RunID)
			}
			if left == "" {
				return false
			}
			if right == "" {
				return true
			}
			return left > right
		})
		fmt.Fprintln(stdout, "SYSTEM > runs")
		fmt.Fprintln(stdout, "RUN_ID           STATUS      AGENT        SESSION          WORKSPACE       DIAG            BLOCKED")
		for _, r := range runs {
			workspace := strings.TrimSpace(r.WorkspaceID)
			if workspace == "" {
				workspace = "-"
			}
			diag := strings.TrimSpace(r.DiagnosticCode)
			if diag == "" {
				diag = "-"
			}
			blocked := strings.TrimSpace(r.PolicyBlockedTool)
			if blocked == "" {
				blocked = "-"
			}
			fmt.Fprintf(stdout, "%-16s %-11s %-12s %-16s %-15s %-15s %s diag=%s blocked=%s\n",
				r.RunID, r.Status, r.Agent, r.SessionID, workspace, diag, blocked, diag, blocked)
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
		fmt.Fprintf(stdout, "SYSTEM > run %s status=%s agent=%s session=%s", run.RunID, run.Status, run.Agent, run.SessionID)
		if strings.TrimSpace(run.WorkspaceID) != "" {
			fmt.Fprintf(stdout, " workspace=%s", run.WorkspaceID)
		}
		fmt.Fprintln(stdout)
		if strings.TrimSpace(run.Response) != "" {
			fmt.Fprintf(stdout, "response: %s\n", run.Response)
		}
		if strings.TrimSpace(run.Error) != "" {
			fmt.Fprintf(stdout, "error: %s\n", run.Error)
			if strings.TrimSpace(run.DiagnosticCode) != "" {
				fmt.Fprintf(stdout, "diagnostic: %s | %s\n", strings.TrimSpace(run.DiagnosticCode), strings.TrimSpace(run.DiagnosticReason))
			}
			if strings.TrimSpace(run.PolicyBlockedTool) != "" {
				fmt.Fprintf(stdout, "policy_blocked_tool=%s\n", strings.TrimSpace(run.PolicyBlockedTool))
			}
			if len(run.PolicyAllowedTools) > 0 {
				fmt.Fprintf(stdout, "policy_allowed=%s\n", strings.Join(run.PolicyAllowedTools, ","))
			}
			if len(run.PolicyDeniedTools) > 0 {
				fmt.Fprintf(stdout, "policy_denied=%s\n", strings.Join(run.PolicyDeniedTools, ","))
			}
			if strings.TrimSpace(run.PolicyRiskMax) != "" {
				fmt.Fprintf(stdout, "policy_risk_max=%s\n", strings.TrimSpace(run.PolicyRiskMax))
			}
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
		switch action {
		case "status":
			status, err := runtime.gatewayStatus(ctx)
			if err != nil {
				return true, session, err
			}
			scope := "default"
			fmt.Fprintf(stdout, "SYSTEM > gateway enabled=%t version=%d scope=%s runs_total=%d runs_active=%d agents=%d watch=%t persistence=%t runs_store=%t channels_store=%t restored_runs=%d restored_channels=%d reload_version=%d",
				status.Enabled,
				status.Version,
				scope,
				status.RunsTotal,
				status.RunsActive,
				status.AgentsCount,
				status.AgentsWatchEnabled,
				status.PersistenceEnabled,
				status.RunsPersistenceEnabled,
				status.ChannelsPersistenceEnabled,
				status.RunsRestored,
				status.ChannelsRestored,
				status.AgentsReloadVersion,
			)
			if strings.TrimSpace(status.LastRestoreError) != "" {
				fmt.Fprintf(stdout, " restore_error=%s", strings.TrimSpace(status.LastRestoreError))
			}
			fmt.Fprintln(stdout)
			return true, session, nil
		case "reload":
			status, err := runtime.gatewayReload(ctx)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > gateway enabled=%t version=%d\n", status.Enabled, status.Version)
			return true, session, nil
		case "restart":
			status, err := runtime.gatewayRestart(ctx)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > gateway enabled=%t version=%d\n", status.Enabled, status.Version)
			return true, session, nil
		case "summary":
			report, err := runtime.gatewayReportSummary(ctx)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > gateway summary runs_total=%d runs_active=%d channels_total=%d messages_total=%d archive=%t\n",
				report.RunsTotal, report.RunsActive, report.ChannelsTotal, report.MessagesTotal, report.ArchiveEnabled)
			return true, session, nil
		case "runs":
			limit := 50
			if len(fields) > 2 {
				n, err := parseOptionalLimit(fields[2], 50)
				if err != nil {
					return true, session, fmt.Errorf("usage: /gateway runs [limit]")
				}
				limit = n
			}
			report, err := runtime.gatewayReportRuns(ctx, limit)
			if err != nil {
				return true, session, err
			}
			if len(report.Runs) == 0 {
				fmt.Fprintln(stdout, "SYSTEM > (no gateway runs)")
				return true, session, nil
			}
			fmt.Fprintln(stdout, "SYSTEM > gateway runs")
			for _, run := range report.Runs {
				if strings.TrimSpace(run.WorkspaceID) != "" {
					fmt.Fprintf(stdout, "- %s status=%s agent=%s session=%s workspace=%s\n", run.RunID, run.Status, run.Agent, run.SessionID, run.WorkspaceID)
					continue
				}
				fmt.Fprintf(stdout, "- %s status=%s agent=%s session=%s\n", run.RunID, run.Status, run.Agent, run.SessionID)
			}
			return true, session, nil
		case "channels":
			limit := 50
			if len(fields) > 2 {
				n, err := parseOptionalLimit(fields[2], 50)
				if err != nil {
					return true, session, fmt.Errorf("usage: /gateway channels [limit]")
				}
				limit = n
			}
			report, err := runtime.gatewayReportChannels(ctx, limit)
			if err != nil {
				return true, session, err
			}
			if len(report.Messages) == 0 {
				fmt.Fprintln(stdout, "SYSTEM > (no channel messages)")
				return true, session, nil
			}
			fmt.Fprintln(stdout, "SYSTEM > gateway channel messages")
			for channelID, messages := range report.Messages {
				workspace := ""
				if len(messages) > 0 {
					workspace = strings.TrimSpace(messages[0].WorkspaceID)
				}
				if workspace != "" {
					fmt.Fprintf(stdout, "- %s messages=%d workspace=%s\n", channelID, len(messages), workspace)
					continue
				}
				fmt.Fprintf(stdout, "- %s messages=%d\n", channelID, len(messages))
			}
			return true, session, nil
		case "report":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /gateway report {summary|runs [limit]|channels [limit]}")
			}
			rewritten := strings.TrimSpace(strings.Replace(line, "/gateway report", "/gateway", 1))
			return executeCommandWithState(ctx, runtime, rewritten, session, stdout, stderr, state)
		default:
			return true, session, fmt.Errorf("usage: /gateway {status|reload|restart|summary|runs [limit]|channels [limit]}")
		}
	case "/browser":
		action := "status"
		if len(fields) > 1 {
			action = strings.TrimSpace(fields[1])
		}
		switch action {
		case "status":
			status, err := runtime.browserStatus(ctx)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > browser running=%t profile=%s driver=%s extension_connected=%t attached_tabs=%d\n",
				status.Running,
				strings.TrimSpace(status.Profile),
				strings.TrimSpace(status.Driver),
				status.ExtensionConnected,
				status.AttachedTabs,
			)
			return true, session, nil
		case "profiles":
			profiles, err := runtime.browserProfiles(ctx)
			if err != nil {
				return true, session, err
			}
			if len(profiles) == 0 {
				fmt.Fprintln(stdout, "SYSTEM > (no browser profiles)")
				return true, session, nil
			}
			fmt.Fprintln(stdout, "SYSTEM > browser profiles")
			for _, profile := range profiles {
				fmt.Fprintf(stdout, "- %s driver=%s default=%t running=%t extension_connected=%t\n",
					strings.TrimSpace(profile.Name),
					strings.TrimSpace(profile.Driver),
					profile.Default,
					profile.Running,
					profile.ExtensionConnected,
				)
			}
			return true, session, nil
		case "login":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /browser login {site_id} [--profile <name>]")
			}
			profile, err := parseProfileFlag(fields[3:])
			if err != nil {
				return true, session, err
			}
			result, err := runtime.browserLogin(ctx, fields[2], profile)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > browser login site=%s profile=%s mode=%s success=%t %s\n",
				strings.TrimSpace(result.SiteID),
				strings.TrimSpace(result.Profile),
				strings.TrimSpace(result.Mode),
				result.Success,
				strings.TrimSpace(result.Message),
			)
			return true, session, nil
		case "check":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /browser check {site_id} [--profile <name>]")
			}
			profile, err := parseProfileFlag(fields[3:])
			if err != nil {
				return true, session, err
			}
			result, err := runtime.browserCheck(ctx, fields[2], profile)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > browser check site=%s profile=%s checks=%d passed=%t %s\n",
				strings.TrimSpace(result.SiteID),
				strings.TrimSpace(result.Profile),
				result.CheckCount,
				result.Passed,
				strings.TrimSpace(result.Message),
			)
			return true, session, nil
		case "run":
			if len(fields) < 4 {
				return true, session, fmt.Errorf("usage: /browser run {site_id} {flow_action} [--profile <name>]")
			}
			profile, err := parseProfileFlag(fields[4:])
			if err != nil {
				return true, session, err
			}
			result, err := runtime.browserRun(ctx, fields[2], fields[3], profile)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > browser run site=%s action=%s profile=%s steps=%d success=%t %s\n",
				strings.TrimSpace(result.SiteID),
				strings.TrimSpace(result.Action),
				strings.TrimSpace(result.Profile),
				result.StepCount,
				result.Success,
				strings.TrimSpace(result.Message),
			)
			return true, session, nil
		default:
			return true, session, fmt.Errorf("usage: /browser {status|profiles|login|check|run}")
		}
	case "/vault":
		action := "status"
		if len(fields) > 1 {
			action = strings.TrimSpace(fields[1])
		}
		switch action {
		case "status":
			status, err := runtime.vaultStatus(ctx)
			if err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > vault enabled=%t ready=%t mode=%s addr=%s allowlist=%d",
				status.Enabled, status.Ready, strings.TrimSpace(status.AuthMode), strings.TrimSpace(status.Addr), status.AllowlistCount,
			)
			if strings.TrimSpace(status.LastError) != "" {
				fmt.Fprintf(stdout, " error=%s", strings.TrimSpace(status.LastError))
			}
			fmt.Fprintln(stdout)
			return true, session, nil
		default:
			return true, session, fmt.Errorf("usage: /vault {status}")
		}
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
	case "/notify":
		if state == nil || state.notifications == nil {
			return true, session, fmt.Errorf("notifications are not available in this context")
		}
		if len(fields) == 1 || strings.TrimSpace(fields[1]) == "list" {
			items := state.notifications.filtered()
			if len(items) == 0 {
				fmt.Fprintln(stdout, "SYSTEM > (no notifications)")
				return true, session, nil
			}
			fmt.Fprintf(stdout, "SYSTEM > notifications filter=%s\n", state.notifications.filterName())
			for i, item := range items {
				fmt.Fprintf(stdout, "%d. [%s/%s] %s (%s)\n", i+1, item.Category, item.Severity, item.Title, item.Timestamp)
			}
			return true, session, nil
		}
		sub := strings.TrimSpace(fields[1])
		switch sub {
		case "filter":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /notify filter {all|cron|heartbeat|error}")
			}
			if err := state.notifications.setFilter(fields[2]); err != nil {
				return true, session, err
			}
			fmt.Fprintf(stdout, "SYSTEM > notification filter: %s\n", state.notifications.filterName())
			return true, session, nil
		case "open":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /notify open {index}")
			}
			index, err := strconv.Atoi(strings.TrimSpace(fields[2]))
			if err != nil || index <= 0 {
				return true, session, fmt.Errorf("usage: /notify open {index}")
			}
			items := state.notifications.filtered()
			if index > len(items) {
				return true, session, fmt.Errorf("notification not found: %d", index)
			}
			item := items[index-1]
			fmt.Fprintf(stdout, "SYSTEM > [%s/%s] %s | %s | %s\n", item.Category, item.Severity, item.Title, item.Message, item.Timestamp)
			return true, session, nil
		case "clear":
			state.notifications.clear()
			fmt.Fprintln(stdout, "SYSTEM > notifications cleared")
			return true, session, nil
		default:
			return true, session, fmt.Errorf("usage: /notify {list|filter|open|clear}")
		}
	case "/trace":
		if state == nil {
			return true, session, fmt.Errorf("trace is only available in interactive mode")
		}
		if len(fields) == 1 {
			traceName := "off"
			if state.chatTrace {
				traceName = "on"
			}
			filter := strings.TrimSpace(state.chatTraceFilter)
			if filter == "" {
				filter = "all"
			}
			fmt.Fprintf(stdout, "SYSTEM > trace=%s filter=%s\n", traceName, filter)
			return true, session, nil
		}
		switch strings.ToLower(strings.TrimSpace(fields[1])) {
		case "on":
			state.chatTrace = true
			if strings.TrimSpace(state.chatTraceFilter) == "" {
				state.chatTraceFilter = "all"
			}
			fmt.Fprintf(stdout, "SYSTEM > trace=on filter=%s\n", state.chatTraceFilter)
			return true, session, nil
		case "off":
			state.chatTrace = false
			fmt.Fprintln(stdout, "SYSTEM > trace=off filter=all")
			return true, session, nil
		case "filter":
			if len(fields) < 3 {
				return true, session, fmt.Errorf("usage: /trace filter {all|llm|tool|error|system}")
			}
			filter := strings.ToLower(strings.TrimSpace(fields[2]))
			switch filter {
			case "all", "llm", "tool", "error", "system":
				state.chatTraceFilter = filter
				fmt.Fprintf(stdout, "SYSTEM > trace_filter=%s\n", state.chatTraceFilter)
				return true, session, nil
			default:
				return true, session, fmt.Errorf("usage: /trace filter {all|llm|tool|error|system}")
			}
		default:
			return true, session, fmt.Errorf("usage: /trace [on|off|filter {all|llm|tool|error|system}]")
		}
	default:
		return false, session, nil
	}
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
		return label
	}
	parts := []string{label}
	if toolCallID := strings.TrimSpace(evt.ToolCallID); toolCallID != "" {
		parts = append(parts, "id="+toolCallID)
	}
	if toolArgs := strings.TrimSpace(evt.ToolArgsPreview); toolArgs != "" {
		parts = append(parts, "args="+toolArgs)
	}
	if toolResult := strings.TrimSpace(evt.ToolResultPreview); toolResult != "" {
		parts = append(parts, "result="+toolResult)
	}
	return strings.Join(parts, " | ")
}

func formatRuntimeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	var apiErr *apiHTTPError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return message
	}
	hint := runtimeErrorHint(apiErr)
	if strings.TrimSpace(hint) == "" {
		return message
	}
	return message + "\nhint: " + hint
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
	case "workspace_id_required":
		return "workspace binding is missing on server; check tarsd role-to-workspace config"
	case "workspace_forbidden":
		return "your role is not allowed for the mapped workspace; ask admin to update workspace binding"
	case "unauthorized":
		if isAdminEndpointPath(endpointPath) {
			return "admin endpoint requires admin token; retry with --admin-api-token (or TARS_ADMIN_API_TOKEN)"
		}
		return "set --api-token (or TARS_API_TOKEN), then retry"
	case "forbidden":
		if isAdminEndpointPath(endpointPath) {
			return "this endpoint requires admin role; retry with --admin-api-token or ask admin"
		}
		return "your role/workspace is not allowed for this endpoint"
	}
	switch apiErr.Status {
	case http.StatusUnauthorized:
		return "verify API token and retry"
	case http.StatusForbidden:
		return "verify role/workspace permissions and retry"
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
