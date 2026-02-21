package tarsclient

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func cmdAgents(c commandContext) (bool, string, error) {
	switch c.fields[0] {
	case "/agents":
		agents, err := c.runtime.listAgents(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		sort.SliceStable(agents, func(i, j int) bool {
			return strings.TrimSpace(agents[i].Name) < strings.TrimSpace(agents[j].Name)
		})
		if len(agents) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no agents)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > agents")
		detail := len(c.fields) > 1 && (strings.TrimSpace(c.fields[1]) == "--detail" || strings.TrimSpace(c.fields[1]) == "-d")
		if detail {
			fmt.Fprintln(c.stdout, "NAME         KIND     SOURCE      POLICY     ROUTING  ALLOW DENY RISK")
		}
		for _, a := range agents {
			if detail {
				risk := strings.TrimSpace(a.ToolsRiskMax)
				if risk == "" {
					risk = "-"
				}
				fmt.Fprintf(c.stdout, "%-12s %-8s %-11s %-10s %-8s %-5d %-4d %s\n",
					a.Name, a.Kind, a.Source, a.PolicyMode, a.SessionRoutingMode, a.ToolsAllowCount, a.ToolsDenyCount, risk)
				fmt.Fprintf(c.stdout, "  entry=%s allow=%d deny=%d risk_max=%s routing=%s",
					a.Entry, a.ToolsAllowCount, a.ToolsDenyCount, risk, a.SessionRoutingMode)
				if strings.TrimSpace(a.SessionFixedID) != "" {
					fmt.Fprintf(c.stdout, " fixed_session=%s", strings.TrimSpace(a.SessionFixedID))
				}
				fmt.Fprintln(c.stdout)
				continue
			}
			fmt.Fprintf(c.stdout, "- %s kind=%s source=%s policy=%s\n", a.Name, a.Kind, a.Source, a.PolicyMode)
		}
		return true, c.session, nil
	case "/runs":
		limit := 30
		if len(c.fields) > 1 {
			n, err := parseOptionalLimit(c.fields[1], 30)
			if err != nil {
				return true, c.session, err
			}
			limit = n
		}
		runs, err := c.runtime.listRuns(c.ctx, limit)
		if err != nil {
			return true, c.session, err
		}
		if len(runs) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no runs)")
			return true, c.session, nil
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
		fmt.Fprintln(c.stdout, "SYSTEM > runs")
		fmt.Fprintln(c.stdout, "RUN_ID           STATUS      AGENT        SESSION")
		for _, r := range runs {
			diag := strings.TrimSpace(r.DiagnosticCode)
			if diag == "" {
				diag = "-"
			}
			blocked := strings.TrimSpace(r.PolicyBlockedTool)
			if blocked == "" {
				blocked = "-"
			}
			fmt.Fprintf(c.stdout, "%-16s %-11s %-12s %-16s diag=%s blocked=%s\n",
				r.RunID, r.Status, r.Agent, r.SessionID, diag, blocked)
		}
		return true, c.session, nil
	case "/run":
		if len(c.fields) < 2 {
			return true, c.session, fmt.Errorf("usage: /run {id}")
		}
		run, err := c.runtime.getRun(c.ctx, c.fields[1])
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > run %s status=%s agent=%s session=%s\n", run.RunID, run.Status, run.Agent, run.SessionID)
		if strings.TrimSpace(run.Response) != "" {
			fmt.Fprintf(c.stdout, "response: %s\n", run.Response)
		}
		if strings.TrimSpace(run.Error) != "" {
			fmt.Fprintf(c.stdout, "error: %s\n", run.Error)
			if strings.TrimSpace(run.DiagnosticCode) != "" {
				fmt.Fprintf(c.stdout, "diagnostic: %s | %s\n", strings.TrimSpace(run.DiagnosticCode), strings.TrimSpace(run.DiagnosticReason))
			}
			if strings.TrimSpace(run.PolicyBlockedTool) != "" {
				fmt.Fprintf(c.stdout, "policy_blocked_tool=%s\n", strings.TrimSpace(run.PolicyBlockedTool))
			}
			if len(run.PolicyAllowedTools) > 0 {
				fmt.Fprintf(c.stdout, "policy_allowed=%s\n", strings.Join(run.PolicyAllowedTools, ","))
			}
			if len(run.PolicyDeniedTools) > 0 {
				fmt.Fprintf(c.stdout, "policy_denied=%s\n", strings.Join(run.PolicyDeniedTools, ","))
			}
			if strings.TrimSpace(run.PolicyRiskMax) != "" {
				fmt.Fprintf(c.stdout, "policy_risk_max=%s\n", strings.TrimSpace(run.PolicyRiskMax))
			}
		}
		return true, c.session, nil
	case "/cancel-run":
		if len(c.fields) < 2 {
			return true, c.session, fmt.Errorf("usage: /cancel-run {id}")
		}
		run, err := c.runtime.cancelRun(c.ctx, c.fields[1])
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > canceled %s status=%s\n", run.RunID, run.Status)
		return true, c.session, nil
	case "/spawn":
		raw := strings.TrimSpace(strings.TrimPrefix(c.line, "/spawn"))
		sp, err := parseSpawnCommand(raw)
		if err != nil {
			return true, c.session, fmt.Errorf("usage: /spawn [--agent {name}] [--title {title}] [--session {id}] [--wait] {message}: %w", err)
		}
		run, err := c.runtime.spawnRun(c.ctx, agentSpawnRequest{SessionID: sp.SessionID, Title: sp.Title, Message: sp.Message, Agent: sp.Agent})
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > spawned run_id=%s status=%s\n", run.RunID, run.Status)
		if !sp.Wait {
			return true, c.session, nil
		}
		finalRun, err := waitRun(c.ctx, c.runtime, run.RunID, time.Second)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > run %s completed status=%s\n", finalRun.RunID, finalRun.Status)
		if strings.TrimSpace(finalRun.Response) != "" {
			fmt.Fprintf(c.stdout, "%s\n", finalRun.Response)
		}
		if strings.TrimSpace(finalRun.SessionID) != "" {
			fmt.Fprintf(c.stderr, "session=%s\n", finalRun.SessionID)
			return true, finalRun.SessionID, nil
		}
		return true, c.session, nil
	default:
		return false, c.session, nil
	}
}
