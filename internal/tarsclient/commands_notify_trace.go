package tarsclient

import (
	"fmt"
	"strconv"
	"strings"
)

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
		maxID := int64(0)
		for _, item := range items {
			if item.ID > maxID {
				maxID = item.ID
			}
		}
		if maxID > 0 {
			readInfo, err := c.runtime.markEventsRead(c.ctx, maxID)
			if err != nil {
				return true, c.session, err
			}
			c.state.notifications.setReadCursor(readInfo.ReadCursor)
		}
		return true, c.session, nil
	}
	sub := strings.TrimSpace(c.fields[1])
	switch sub {
	case "filter":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /notify filter {all|cron|pulse|reflection|error}")
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
