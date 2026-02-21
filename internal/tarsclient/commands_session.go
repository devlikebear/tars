package tarsclient

import (
	"fmt"
	"strconv"
	"strings"
)

func cmdHelp(c commandContext) (bool, string, error) {
	fmt.Fprintln(c.stdout, helpText())
	return true, c.session, nil
}

func cmdSession(c commandContext) (bool, string, error) {
	switch c.fields[0] {
	case "/session":
		fmt.Fprintf(c.stdout, "SYSTEM > session=%s\n", c.session)
		return true, c.session, nil
	case "/resume":
		if len(c.fields) < 2 || strings.TrimSpace(c.fields[1]) == "" {
			sessions, err := c.runtime.listSessions(c.ctx)
			if err != nil {
				return true, c.session, err
			}
			if len(sessions) == 0 {
				return true, c.session, fmt.Errorf("no sessions available; use /new first")
			}
			fmt.Fprintln(c.stdout, "SYSTEM > resume targets")
			for i, s := range sessions {
				fmt.Fprintf(c.stdout, "%d. %s %s\n", i+1, s.ID, s.Title)
			}
			fmt.Fprintln(c.stdout, "SYSTEM > use /resume {number|id|latest|main}")
			return true, c.session, nil
		}
		arg := strings.TrimSpace(c.fields[1])
		next := ""
		if strings.EqualFold(arg, "main") {
			status, err := c.runtime.status(c.ctx)
			if err != nil {
				return true, c.session, err
			}
			next = strings.TrimSpace(status.MainSessionID)
			if next == "" {
				return true, c.session, fmt.Errorf("main session is not configured")
			}
		} else if strings.EqualFold(arg, "latest") {
			sessions, err := c.runtime.listSessions(c.ctx)
			if err != nil {
				return true, c.session, err
			}
			if len(sessions) == 0 {
				return true, c.session, fmt.Errorf("no sessions available; use /new first")
			}
			next = strings.TrimSpace(sessions[0].ID)
		} else if n, err := strconv.Atoi(arg); err == nil {
			sessions, listErr := c.runtime.listSessions(c.ctx)
			if listErr != nil {
				return true, c.session, listErr
			}
			if len(sessions) == 0 {
				return true, c.session, fmt.Errorf("no sessions available; use /new first")
			}
			if n <= 0 || n > len(sessions) {
				return true, c.session, fmt.Errorf("resume target out of range: %d", n)
			}
			next = strings.TrimSpace(sessions[n-1].ID)
		} else {
			next = arg
		}
		if next == "" {
			return true, c.session, fmt.Errorf("resume target is empty")
		}
		fmt.Fprintf(c.stdout, "SYSTEM > resumed session=%s\n", next)
		fmt.Fprintf(c.stderr, "session=%s\n", next)
		return true, next, nil
	case "/new":
		title := strings.TrimSpace(strings.TrimPrefix(c.line, "/new"))
		if title == "" {
			title = "chat"
		}
		created, err := c.runtime.createSession(c.ctx, title)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > created session %s (%s)\n", created.ID, created.Title)
		fmt.Fprintf(c.stderr, "session=%s\n", created.ID)
		return true, created.ID, nil
	case "/sessions":
		sessions, err := c.runtime.listSessions(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(sessions) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no sessions)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > sessions")
		for _, s := range sessions {
			fmt.Fprintf(c.stdout, "- %s %s\n", s.ID, s.Title)
		}
		return true, c.session, nil
	case "/history":
		if strings.TrimSpace(c.session) == "" {
			return true, c.session, fmt.Errorf("history requires active session; use /new or --session")
		}
		messages, err := c.runtime.getHistory(c.ctx, c.session)
		if err != nil {
			return true, c.session, err
		}
		if len(messages) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no history)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > history")
		for _, m := range messages {
			content := strings.TrimSpace(m.Content)
			if len(content) > 120 {
				content = content[:117] + "..."
			}
			fmt.Fprintf(c.stdout, "- %s: %s\n", strings.TrimSpace(m.Role), content)
		}
		return true, c.session, nil
	case "/export":
		if strings.TrimSpace(c.session) == "" {
			return true, c.session, fmt.Errorf("export requires active session; use /new or --session")
		}
		markdown, err := c.runtime.exportSession(c.ctx, c.session)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprint(c.stdout, markdown)
		if !strings.HasSuffix(markdown, "\n") {
			fmt.Fprintln(c.stdout)
		}
		return true, c.session, nil
	case "/search":
		keyword := strings.TrimSpace(strings.TrimPrefix(c.line, "/search"))
		if keyword == "" {
			return true, c.session, fmt.Errorf("usage: /search {keyword}")
		}
		results, err := c.runtime.searchSessions(c.ctx, keyword)
		if err != nil {
			return true, c.session, err
		}
		if len(results) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no matched sessions)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > matched sessions")
		for _, s := range results {
			fmt.Fprintf(c.stdout, "- %s %s\n", s.ID, s.Title)
		}
		return true, c.session, nil
	case "/compact":
		if strings.TrimSpace(c.session) == "" {
			return true, c.session, fmt.Errorf("compact requires active session; use /new or --session")
		}
		result, err := c.runtime.compact(c.ctx, c.session)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", strings.TrimSpace(result.Message))
		return true, c.session, nil
	default:
		return false, c.session, nil
	}
}
