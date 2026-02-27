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
	"/status":     cmdRuntime,
	"/providers":  cmdRuntime,
	"/models":     cmdRuntime,
	"/model":      cmdRuntime,
	"/whoami":     cmdRuntime,
	"/health":     cmdRuntime,
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
	"/cron":       cmdCron,
	"/project":    cmdProject,
	"/usage":      cmdUsage,
	"/ops":        cmdOps,
	"/approve":    cmdApprove,
	"/schedule":   cmdSchedule,
	"/notify":     cmdNotify,
	"/trace":      cmdTrace,
}

func dispatchCommand(c commandContext) (bool, string, error) {
	if len(c.fields) == 0 {
		return true, c.session, nil
	}
	handler, ok := commandHandlers[strings.TrimSpace(c.fields[0])]
	if !ok {
		return false, c.session, nil
	}
	return handler(c)
}
