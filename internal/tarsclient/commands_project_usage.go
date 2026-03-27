package tarsclient

import (
	"fmt"
	"strconv"
	"strings"
)

func cmdProject(c commandContext) (bool, string, error) {
	if c.fields[0] != "/project" {
		return false, c.session, nil
	}
	if len(c.fields) < 2 {
		return true, c.session, fmt.Errorf("usage: /project {list|get|create|activate|archive|board|activity|dispatch|autopilot}")
	}
	switch strings.ToLower(strings.TrimSpace(c.fields[1])) {
	case "list":
		items, err := c.runtime.listProjects(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(items) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no projects)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > projects")
		for _, item := range items {
			fmt.Fprintf(c.stdout, "- %s name=%s type=%s status=%s\n",
				strings.TrimSpace(item.ID),
				strings.TrimSpace(item.Name),
				strings.TrimSpace(item.Type),
				strings.TrimSpace(item.Status),
			)
		}
		return true, c.session, nil
	case "get":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /project get {project_id}")
		}
		item, err := c.runtime.getProject(c.ctx, c.fields[2])
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project %s\n", strings.TrimSpace(item.ID))
		fmt.Fprintf(c.stdout, "name=%s type=%s status=%s\n", strings.TrimSpace(item.Name), strings.TrimSpace(item.Type), strings.TrimSpace(item.Status))
		if strings.TrimSpace(item.GitRepo) != "" {
			fmt.Fprintf(c.stdout, "git_repo=%s\n", strings.TrimSpace(item.GitRepo))
		}
		if strings.TrimSpace(item.Objective) != "" {
			fmt.Fprintf(c.stdout, "objective=%s\n", strings.TrimSpace(item.Objective))
		}
		if body := strings.TrimSpace(item.Body); body != "" {
			fmt.Fprintln(c.stdout, body)
		}
		return true, c.session, nil
	case "create":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /project create {name} [type]")
		}
		req := projectCreateRequest{
			Name: c.fields[2],
		}
		if len(c.fields) >= 4 {
			req.Type = strings.TrimSpace(c.fields[3])
		}
		item, err := c.runtime.createProject(c.ctx, req)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project created id=%s name=%s type=%s\n", strings.TrimSpace(item.ID), strings.TrimSpace(item.Name), strings.TrimSpace(item.Type))
		return true, c.session, nil
	case "activate":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /project activate {project_id} [session_id]")
		}
		sessionID := c.session
		if len(c.fields) >= 4 {
			sessionID = strings.TrimSpace(c.fields[3])
		}
		if err := c.runtime.activateProject(c.ctx, c.fields[2], sessionID); err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project activated project_id=%s session_id=%s\n", strings.TrimSpace(c.fields[2]), strings.TrimSpace(sessionID))
		return true, c.session, nil
	case "archive", "delete":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /project archive {project_id}")
		}
		if err := c.runtime.deleteProject(c.ctx, c.fields[2]); err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project archived %s\n", strings.TrimSpace(c.fields[2]))
		return true, c.session, nil
	case "board":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /project board {project_id}")
		}
		item, err := c.runtime.getProjectBoard(c.ctx, c.fields[2])
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project board %s columns=%s updated=%s\n",
			strings.TrimSpace(item.ProjectID),
			strings.Join(item.Columns, ","),
			strings.TrimSpace(item.UpdatedAt),
		)
		if len(item.Tasks) == 0 {
			fmt.Fprintln(c.stdout, "(no board tasks)")
			return true, c.session, nil
		}
		for _, task := range item.Tasks {
			fmt.Fprintf(c.stdout, "- %s status=%s assignee=%s role=%s title=%s\n",
				strings.TrimSpace(task.ID),
				strings.TrimSpace(task.Status),
				strings.TrimSpace(task.Assignee),
				strings.TrimSpace(task.Role),
				strings.TrimSpace(task.Title),
			)
		}
		return true, c.session, nil
	case "activity":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /project activity {project_id} [limit]")
		}
		limit := 10
		if len(c.fields) >= 4 {
			value, err := strconv.Atoi(strings.TrimSpace(c.fields[3]))
			if err != nil || value < 0 {
				return true, c.session, fmt.Errorf("limit must be a non-negative integer")
			}
			limit = value
		}
		items, err := c.runtime.listProjectActivity(c.ctx, c.fields[2], limit)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project activity %s count=%d\n", strings.TrimSpace(c.fields[2]), len(items))
		if len(items) == 0 {
			fmt.Fprintln(c.stdout, "(no recent activity)")
			return true, c.session, nil
		}
		for _, item := range items {
			fmt.Fprintf(c.stdout, "- %s source=%s kind=%s status=%s task=%s agent=%s message=%s\n",
				strings.TrimSpace(item.Timestamp),
				strings.TrimSpace(item.Source),
				strings.TrimSpace(item.Kind),
				strings.TrimSpace(item.Status),
				strings.TrimSpace(item.TaskID),
				strings.TrimSpace(item.Agent),
				strings.TrimSpace(item.Message),
			)
		}
		return true, c.session, nil
	case "dispatch":
		if len(c.fields) < 4 {
			return true, c.session, fmt.Errorf("usage: /project dispatch {project_id} {todo|review}")
		}
		stage := strings.ToLower(strings.TrimSpace(c.fields[3]))
		if stage != "todo" && stage != "review" {
			return true, c.session, fmt.Errorf("stage must be todo|review")
		}
		report, err := c.runtime.dispatchProject(c.ctx, c.fields[2], stage)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > project dispatch %s stage=%s runs=%d\n",
			strings.TrimSpace(report.ProjectID),
			stage,
			len(report.Runs),
		)
		for _, run := range report.Runs {
			fmt.Fprintf(c.stdout, "- %s task=%s agent=%s worker=%s status=%s\n",
				strings.TrimSpace(run.ID),
				strings.TrimSpace(run.TaskID),
				strings.TrimSpace(run.Agent),
				strings.TrimSpace(run.WorkerKind),
				strings.TrimSpace(run.Status),
			)
		}
		return true, c.session, nil
	case "autopilot":
		if len(c.fields) < 4 {
			return true, c.session, fmt.Errorf("usage: /project autopilot {start|advance|status} {project_id}")
		}
		action := strings.ToLower(strings.TrimSpace(c.fields[2]))
		projectID := strings.TrimSpace(c.fields[3])
		switch action {
		case "start":
			item, err := c.runtime.startProjectAutopilot(c.ctx, projectID)
			if err != nil {
				return true, c.session, err
			}
			fmt.Fprintf(c.stdout, "SYSTEM > autopilot started project_id=%s run_id=%s status=%s iterations=%d\n",
				strings.TrimSpace(item.ProjectID),
				strings.TrimSpace(item.RunID),
				strings.TrimSpace(item.Status),
				item.Iterations,
			)
			return true, c.session, nil
		case "advance":
			item, err := c.runtime.advanceProjectAutopilot(c.ctx, projectID)
			if err != nil {
				return true, c.session, err
			}
			fmt.Fprintf(c.stdout, "SYSTEM > project autopilot advance %s phase=%s status=%s run_status=%s next_action=%s message=%s\n",
				strings.TrimSpace(item.ProjectID),
				strings.TrimSpace(item.Name),
				strings.TrimSpace(item.Status),
				strings.TrimSpace(item.RunStatus),
				strings.TrimSpace(item.NextAction),
				strings.TrimSpace(item.Message),
			)
			return true, c.session, nil
		case "status":
			item, err := c.runtime.getProjectAutopilot(c.ctx, projectID)
			if err != nil {
				return true, c.session, err
			}
			fmt.Fprintf(c.stdout, "SYSTEM > project autopilot %s run_id=%s status=%s iterations=%d phase=%s phase_status=%s next_action=%s message=%s\n",
				strings.TrimSpace(item.ProjectID),
				strings.TrimSpace(item.RunID),
				strings.TrimSpace(item.Status),
				item.Iterations,
				strings.TrimSpace(item.Phase),
				strings.TrimSpace(item.PhaseStatus),
				strings.TrimSpace(item.NextAction),
				strings.TrimSpace(item.Message),
			)
			return true, c.session, nil
		default:
			return true, c.session, fmt.Errorf("usage: /project autopilot {start|advance|status} {project_id}")
		}
	default:
		return true, c.session, fmt.Errorf("usage: /project {list|get|create|activate|archive|board|activity|dispatch|autopilot}")
	}
}

func cmdUsage(c commandContext) (bool, string, error) {
	if c.fields[0] != "/usage" {
		return false, c.session, nil
	}
	if len(c.fields) < 2 {
		return true, c.session, fmt.Errorf("usage: /usage {summary|limits|set-limits}")
	}
	switch strings.ToLower(strings.TrimSpace(c.fields[1])) {
	case "summary":
		period := "today"
		groupBy := "provider"
		if len(c.fields) >= 3 {
			period = strings.TrimSpace(c.fields[2])
		}
		if len(c.fields) >= 4 {
			groupBy = strings.TrimSpace(c.fields[3])
		}
		summary, limits, limitStatus, err := c.runtime.usageSummary(c.ctx, period, groupBy)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > usage period=%s group_by=%s calls=%d cost_usd=%.6f\n",
			summary.Period, summary.GroupBy, summary.TotalCalls, summary.TotalCostUSD)
		fmt.Fprintf(c.stdout, "limits daily=%.2f weekly=%.2f monthly=%.2f mode=%s\n",
			limits.DailyUSD, limits.WeeklyUSD, limits.MonthlyUSD, limits.Mode)
		if limitStatus.Exceeded {
			fmt.Fprintf(c.stdout, "limit_status=exceeded period=%s spent=%.6f limit=%.6f mode=%s\n",
				limitStatus.Period, limitStatus.SpentUSD, limitStatus.LimitUSD, limitStatus.Mode)
		}
		for _, row := range summary.Rows {
			fmt.Fprintf(c.stdout, "- %s calls=%d cost_usd=%.6f in=%d out=%d\n",
				strings.TrimSpace(row.Key), row.Calls, row.CostUSD, row.InputTokens, row.OutputTokens)
		}
		return true, c.session, nil
	case "limits":
		limits, err := c.runtime.usageLimits(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > usage limits daily=%.2f weekly=%.2f monthly=%.2f mode=%s\n",
			limits.DailyUSD, limits.WeeklyUSD, limits.MonthlyUSD, limits.Mode)
		return true, c.session, nil
	case "set-limits":
		if len(c.fields) < 6 {
			return true, c.session, fmt.Errorf("usage: /usage set-limits {daily_usd} {weekly_usd} {monthly_usd} {soft|hard}")
		}
		daily, err := strconv.ParseFloat(strings.TrimSpace(c.fields[2]), 64)
		if err != nil {
			return true, c.session, fmt.Errorf("daily_usd must be a number")
		}
		weekly, err := strconv.ParseFloat(strings.TrimSpace(c.fields[3]), 64)
		if err != nil {
			return true, c.session, fmt.Errorf("weekly_usd must be a number")
		}
		monthly, err := strconv.ParseFloat(strings.TrimSpace(c.fields[4]), 64)
		if err != nil {
			return true, c.session, fmt.Errorf("monthly_usd must be a number")
		}
		mode := strings.TrimSpace(strings.ToLower(c.fields[5]))
		if mode != "soft" && mode != "hard" {
			return true, c.session, fmt.Errorf("mode must be soft|hard")
		}
		updated, err := c.runtime.updateUsageLimits(c.ctx, usageLimits{
			DailyUSD:   daily,
			WeeklyUSD:  weekly,
			MonthlyUSD: monthly,
			Mode:       mode,
		})
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > usage limits updated daily=%.2f weekly=%.2f monthly=%.2f mode=%s\n",
			updated.DailyUSD, updated.WeeklyUSD, updated.MonthlyUSD, updated.Mode)
		return true, c.session, nil
	default:
		return true, c.session, fmt.Errorf("usage: /usage {summary|limits|set-limits}")
	}
}
