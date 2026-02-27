package tarsclient

import (
	"fmt"
	"strings"
)

func cmdSchedule(c commandContext) (bool, string, error) {
	if c.fields[0] != "/schedule" {
		return false, c.session, nil
	}
	if len(c.fields) < 2 {
		return true, c.session, fmt.Errorf("usage: /schedule {list|add|done|remove}")
	}
	sub := strings.ToLower(strings.TrimSpace(c.fields[1]))
	switch sub {
	case "list":
		items, err := c.runtime.listSchedules(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(items) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no schedules)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > schedules")
		for _, item := range items {
			fmt.Fprintf(c.stdout, "- %s title=%s schedule=%s status=%s\n", item.ID, item.Title, item.Schedule, item.Status)
		}
		return true, c.session, nil
	case "add":
		payload := strings.TrimSpace(strings.TrimPrefix(c.line, "/schedule add"))
		if payload == "" {
			return true, c.session, fmt.Errorf("usage: /schedule add {자연어}")
		}
		created, err := c.runtime.createSchedule(c.ctx, scheduleCreateRequest{Natural: payload})
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > schedule created id=%s schedule=%s\n", created.ID, created.Schedule)
		return true, c.session, nil
	case "done":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /schedule done {id}")
		}
		status := "completed"
		updated, err := c.runtime.updateSchedule(c.ctx, strings.TrimSpace(c.fields[2]), scheduleUpdateRequest{Status: &status})
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > schedule %s completed\n", updated.ID)
		return true, c.session, nil
	case "remove":
		if len(c.fields) < 3 {
			return true, c.session, fmt.Errorf("usage: /schedule remove {id}")
		}
		id := strings.TrimSpace(c.fields[2])
		if err := c.runtime.deleteSchedule(c.ctx, id); err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > schedule removed %s\n", id)
		return true, c.session, nil
	default:
		return true, c.session, fmt.Errorf("usage: /schedule {list|add|done|remove}")
	}
}
