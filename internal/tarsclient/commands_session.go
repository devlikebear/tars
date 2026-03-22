package tarsclient

import (
	"fmt"
	"strings"
)

func singleMainSessionMessage() string {
	return "single-main-session mode is enabled. tars uses the main session automatically."
}

func cmdHelp(c commandContext) (bool, string, error) {
	advanced := len(c.fields) > 1 && strings.EqualFold(strings.TrimSpace(c.fields[1]), "advanced")
	fmt.Fprintln(c.stdout, helpText(advanced))
	return true, c.session, nil
}

func cmdSession(c commandContext) (bool, string, error) {
	switch c.fields[0] {
	case "/session":
		fmt.Fprintln(c.stdout, "SYSTEM > session=main")
		return true, c.session, nil
	case "/resume":
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	case "/new":
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	case "/sessions":
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	case "/history":
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	case "/export":
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	case "/search":
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	case "/compact":
		info, err := c.runtime.compact(c.ctx, compactRequest{
			SessionID:    "main",
			Instructions: parseCompactInstructions(c.line),
		})
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", strings.TrimSpace(info.Message))
		return true, c.session, nil
	default:
		return false, c.session, nil
	}
}

func parseCompactInstructions(line string) string {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < len("/compact") || !strings.HasPrefix(strings.ToLower(trimmed), "/compact") {
		return ""
	}
	rest := strings.TrimSpace(trimmed[len("/compact"):])
	if strings.HasPrefix(rest, ":") {
		rest = strings.TrimSpace(rest[1:])
	}
	return rest
}
