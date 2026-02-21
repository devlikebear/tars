package tarsclient

import (
	"fmt"
	"strconv"
	"strings"
)

func cmdCron(c commandContext) (bool, string, error) {
	if c.fields[0] != "/cron" {
		return false, c.session, nil
	}
	if len(c.fields) == 1 || strings.TrimSpace(c.fields[1]) == "list" {
		jobs, err := c.runtime.listCronJobs(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(jobs) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no cron jobs)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > cron jobs")
		for _, job := range jobs {
			fmt.Fprintf(c.stdout, "- %s name=%s schedule=%s enabled=%t\n", job.ID, job.Name, job.Schedule, job.Enabled)
		}
		return true, c.session, nil
	}
	sub := strings.TrimSpace(c.fields[1])
	switch sub {
	case "add":
		if len(c.fields) < 4 {
			return true, c.session, fmt.Errorf("usage: /cron add {schedule} {prompt}")
		}
		schedule := strings.TrimSpace(c.fields[2])
		prompt := strings.TrimSpace(strings.TrimPrefix(c.line, "/cron add "+schedule))
		if schedule == "" || prompt == "" {
			return true, c.session, fmt.Errorf("usage: /cron add {schedule} {prompt}")
		}
		job, err := c.runtime.createCronJob(c.ctx, schedule, prompt)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > created cron job %s schedule=%s\n", job.ID, job.Schedule)
		return true, c.session, nil
	case "run":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /cron run {job_id}")
		}
		response, err := c.runtime.runCronJob(c.ctx, c.fields[2])
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", response)
		return true, c.session, nil
	case "get":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /cron get {job_id}")
		}
		jobID := strings.TrimSpace(c.fields[2])
		job, err := c.runtime.getCronJob(c.ctx, jobID)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > cron job %s\n", job.ID)
		fmt.Fprintf(c.stdout, "name=%s schedule=%s enabled=%t delete_after_run=%t\n", cronValueOrDash(job.Name), cronValueOrDash(job.Schedule), job.Enabled, job.DeleteAfterRun)
		fmt.Fprintf(c.stdout, "session_target=%s wake_mode=%s delivery_mode=%s\n", cronValueOrDash(job.SessionTarget), cronValueOrDash(job.WakeMode), cronValueOrDash(job.DeliveryMode))
		if strings.TrimSpace(job.LastRunAt) != "" {
			fmt.Fprintf(c.stdout, "last_run_at=%s\n", strings.TrimSpace(job.LastRunAt))
		}
		if strings.TrimSpace(job.LastRunError) != "" {
			fmt.Fprintf(c.stdout, "last_run_error=%s\n", cronLogText(job.LastRunError))
		}
		fmt.Fprintln(c.stdout, "prompt:")
		fmt.Fprintln(c.stdout, cronPromptText(job.Prompt))

		runs, err := c.runtime.listCronRuns(c.ctx, jobID, 10)
		if err != nil {
			return true, c.session, err
		}
		if len(runs) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no cron run logs)")
			return true, c.session, nil
		}
		fmt.Fprintf(c.stdout, "SYSTEM > cron run logs (latest %d)\n", len(runs))
		for _, run := range runs {
			ranAt := cronValueOrDash(run.RanAt)
			if strings.TrimSpace(run.Error) != "" {
				fmt.Fprintf(c.stdout, "- %s error=%s\n", ranAt, cronLogText(run.Error))
				continue
			}
			fmt.Fprintf(c.stdout, "- %s response=%s\n", ranAt, cronLogText(run.Response))
		}
		return true, c.session, nil
	case "runs":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /cron runs {job_id} [limit]")
		}
		limit := 20
		if len(c.fields) > 3 {
			n, err := parseOptionalLimit(c.fields[3], 20)
			if err != nil {
				return true, c.session, fmt.Errorf("usage: /cron runs {job_id} [limit]")
			}
			limit = n
		}
		runs, err := c.runtime.listCronRuns(c.ctx, c.fields[2], limit)
		if err != nil {
			return true, c.session, err
		}
		if len(runs) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no cron runs)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > cron runs")
		for _, run := range runs {
			if strings.TrimSpace(run.Error) != "" {
				fmt.Fprintf(c.stdout, "- %s error=%s\n", cronValueOrDash(run.RanAt), cronLogText(run.Error))
				continue
			}
			fmt.Fprintf(c.stdout, "- %s response=%s\n", cronValueOrDash(run.RanAt), cronLogText(run.Response))
		}
		return true, c.session, nil
	case "delete":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /cron delete {job_id}")
		}
		if err := c.runtime.deleteCronJob(c.ctx, c.fields[2]); err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > deleted cron job %s\n", strings.TrimSpace(c.fields[2]))
		return true, c.session, nil
	case "enable", "disable":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /cron %s {job_id}", sub)
		}
		enabled := sub == "enable"
		job, err := c.runtime.updateCronJobEnabled(c.ctx, c.fields[2], enabled)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > %s enabled=%t\n", job.ID, job.Enabled)
		return true, c.session, nil
	default:
		return true, c.session, fmt.Errorf("usage: /cron {list|get|runs|add|run|delete|enable|disable}")
	}
}

func cronPromptText(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return "-"
	}
	return text
}

func cronLogText(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return "-"
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n", "\\n")
	return text
}

func cronValueOrDash(v string) string {
	text := strings.TrimSpace(v)
	if text == "" {
		return "-"
	}
	return text
}

func cmdNotify(c commandContext) (bool, string, error) {
	if c.fields[0] != "/notify" {
		return false, c.session, nil
	}
	if c.state == nil || c.state.notifications == nil {
		return true, c.session, fmt.Errorf("notifications are not available in this context")
	}
	if len(c.fields) == 1 || strings.TrimSpace(c.fields[1]) == "list" {
		items := c.state.notifications.filtered()
		if len(items) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no notifications)")
			return true, c.session, nil
		}
		fmt.Fprintf(c.stdout, "SYSTEM > notifications filter=%s\n", c.state.notifications.filterName())
		for i, item := range items {
			fmt.Fprintf(c.stdout, "%d. [%s/%s] %s (%s)\n", i+1, item.Category, item.Severity, item.Title, item.Timestamp)
		}
		return true, c.session, nil
	}
	sub := strings.TrimSpace(c.fields[1])
	switch sub {
	case "filter":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /notify filter {all|cron|heartbeat|error}")
		}
		if err := c.state.notifications.setFilter(c.fields[2]); err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > notification filter: %s\n", c.state.notifications.filterName())
		return true, c.session, nil
	case "open":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /notify open {index}")
		}
		index, err := strconv.Atoi(strings.TrimSpace(c.fields[2]))
		if err != nil || index <= 0 {
			return true, c.session, fmt.Errorf("usage: /notify open {index}")
		}
		items := c.state.notifications.filtered()
		if index > len(items) {
			return true, c.session, fmt.Errorf("notification not found: %d", index)
		}
		item := items[index-1]
		fmt.Fprintf(c.stdout, "SYSTEM > [%s/%s] %s | %s | %s\n", item.Category, item.Severity, item.Title, item.Message, item.Timestamp)
		return true, c.session, nil
	case "clear":
		c.state.notifications.clear()
		fmt.Fprintln(c.stdout, "SYSTEM > notifications cleared")
		return true, c.session, nil
	default:
		return true, c.session, fmt.Errorf("usage: /notify {list|filter|open|clear}")
	}
}

func cmdTrace(c commandContext) (bool, string, error) {
	if c.fields[0] != "/trace" {
		return false, c.session, nil
	}
	if c.state == nil {
		return true, c.session, fmt.Errorf("trace is only available in interactive mode")
	}
	if len(c.fields) == 1 {
		traceName := "off"
		if c.state.chatTrace {
			traceName = "on"
		}
		filter := strings.TrimSpace(c.state.chatTraceFilter)
		if filter == "" {
			filter = "all"
		}
		fmt.Fprintf(c.stdout, "SYSTEM > trace=%s filter=%s\n", traceName, filter)
		return true, c.session, nil
	}
	switch strings.ToLower(strings.TrimSpace(c.fields[1])) {
	case "on":
		c.state.chatTrace = true
		if strings.TrimSpace(c.state.chatTraceFilter) == "" {
			c.state.chatTraceFilter = "all"
		}
		fmt.Fprintf(c.stdout, "SYSTEM > trace=on filter=%s\n", c.state.chatTraceFilter)
		return true, c.session, nil
	case "off":
		c.state.chatTrace = false
		fmt.Fprintln(c.stdout, "SYSTEM > trace=off filter=all")
		return true, c.session, nil
	case "filter":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /trace filter {all|llm|tool|error|system}")
		}
		filter := strings.ToLower(strings.TrimSpace(c.fields[2]))
		switch filter {
		case "all", "llm", "tool", "error", "system":
			c.state.chatTraceFilter = filter
			fmt.Fprintf(c.stdout, "SYSTEM > trace_filter=%s\n", c.state.chatTraceFilter)
			return true, c.session, nil
		default:
			return true, c.session, fmt.Errorf("usage: /trace filter {all|llm|tool|error|system}")
		}
	default:
		return true, c.session, fmt.Errorf("usage: /trace [on|off|filter {all|llm|tool|error|system}]")
	}
}
