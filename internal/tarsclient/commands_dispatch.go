package tarsclient

import (
	"context"
	"io"
	"strings"
)

type commandContext struct {
	ctx     context.Context
	runtime runtimeClient
	fields  []string
	line    string
	session string
	stdout  io.Writer
	stderr  io.Writer
	state   *localRuntimeState
}

type commandHandler func(commandContext) (bool, string, error)

var commandHandlers = map[string]commandHandler{
	"/help":       cmdHelp,
	"/session":    cmdSession,
	"/resume":     cmdSession,
	"/new":        cmdSession,
	"/sessions":   cmdSession,
	"/history":    cmdSession,
	"/export":     cmdSession,
	"/search":     cmdSession,
	"/compact":    cmdSession,
	"/providers":  cmdRuntime,
	"/models":     cmdRuntime,
	"/model":      cmdRuntime,
	"/whoami":     cmdRuntime,
	"/heartbeat":  cmdRuntime,
	"/skills":     cmdRuntime,
	"/plugins":    cmdRuntime,
	"/mcp":        cmdRuntime,
	"/reload":     cmdRuntime,
	"/agents":     cmdAgents,
	"/runs":       cmdAgents,
	"/run":        cmdAgents,
	"/cancel-run": cmdAgents,
	"/spawn":      cmdAgents,
	"/gateway":    cmdGateway,
	"/browser":    cmdBrowser,
	"/vault":      cmdVault,
	"/channels":   cmdChannels,
	"/telegram":   cmdTelegram,
	"/usage":      cmdUsage,
	"/ops":        cmdOps,
	"/schedule":   cmdSchedule,
	"/notify":     cmdNotify,
	"/trace":      cmdTrace,
}

var legacyTUICommandHints = map[string]string{
	"/status":  "Use `tars status` or open the web console at /console.",
	"/health":  "Use `tars health` or open the web console at /console.",
	"/project": "Use `tars project ...` or manage projects in the web console.",
	"/cron":    "Use `tars cron ...` or manage cron jobs in the web console.",
	"/approve": "Use `tars approve ...` or review approvals in the web console.",
}

func dispatchCommand(c commandContext) (bool, string, error) {
	if len(c.fields) == 0 {
		return true, c.session, nil
	}
	command := strings.TrimSpace(c.fields[0])
	if hint, ok := legacyTUICommandHints[command]; ok {
		_, err := io.WriteString(c.stdout, "SYSTEM > legacy TUI no longer handles "+command+". "+hint+"\n")
		return true, c.session, err
	}
	handler, ok := commandHandlers[command]
	if !ok {
		return false, c.session, nil
	}
	return handler(c)
}
