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
		return true, c.session, fmt.Errorf("usage: /project {list|get|create|activate|archive}")
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
	default:
		return true, c.session, fmt.Errorf("usage: /project {list|get|create|activate|archive}")
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
