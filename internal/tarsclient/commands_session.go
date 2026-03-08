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
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", singleMainSessionMessage())
		return true, c.session, nil
	default:
		return false, c.session, nil
	}
}
