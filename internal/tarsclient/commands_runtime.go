package tarsclient

import (
	"fmt"
	"strings"
)

func cmdRuntime(c commandContext) (bool, string, error) {
	switch c.fields[0] {
	case "/status":
		status, err := c.runtime.status(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > workspace=%s sessions=%d", status.WorkspaceDir, status.SessionCount)
		if strings.TrimSpace(status.AuthRole) != "" {
			fmt.Fprintf(c.stdout, " auth_role=%s", status.AuthRole)
		}
		fmt.Fprintln(c.stdout)
		return true, c.session, nil
	case "/whoami":
		identity, err := c.runtime.whoami(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		role := strings.TrimSpace(identity.AuthRole)
		if role == "" {
			role = "anonymous"
		}
		mode := strings.TrimSpace(identity.AuthMode)
		if mode == "" {
			mode = "external-required"
		}
		fmt.Fprintf(c.stdout, "SYSTEM > authenticated=%t role=%s admin=%t mode=%s\n",
			identity.Authenticated, role, identity.IsAdmin, mode)
		return true, c.session, nil
	case "/health":
		status, err := c.runtime.healthz(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > ok=%t component=%s time=%s\n", status.OK, status.Component, status.Time)
		return true, c.session, nil
	case "/heartbeat":
		result, err := c.runtime.heartbeatRunOnce(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if result.Skipped {
			fmt.Fprintf(c.stdout, "SYSTEM > skipped: %s\n", strings.TrimSpace(result.SkipReason))
			return true, c.session, nil
		}
		fmt.Fprintf(c.stdout, "SYSTEM > %s\n", strings.TrimSpace(result.Response))
		return true, c.session, nil
	case "/skills":
		skills, err := c.runtime.listSkills(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(skills) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no skills)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > skills")
		for _, s := range skills {
			fmt.Fprintf(c.stdout, "- %s invocable=%t source=%s\n", s.Name, s.UserInvocable, s.Source)
		}
		return true, c.session, nil
	case "/plugins":
		plugins, err := c.runtime.listPlugins(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		if len(plugins) == 0 {
			fmt.Fprintln(c.stdout, "SYSTEM > (no plugins)")
			return true, c.session, nil
		}
		fmt.Fprintln(c.stdout, "SYSTEM > plugins")
		for _, p := range plugins {
			fmt.Fprintf(c.stdout, "- %s source=%s version=%s\n", p.ID, p.Source, p.Version)
		}
		return true, c.session, nil
	case "/mcp":
		servers, err := c.runtime.listMCPServers(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		tools, err := c.runtime.listMCPTools(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > mcp servers=%d tools=%d\n", len(servers), len(tools))
		for _, s := range servers {
			fmt.Fprintf(c.stdout, "- %s connected=%t tools=%d\n", s.Name, s.Connected, s.ToolCount)
		}
		return true, c.session, nil
	case "/reload":
		result, err := c.runtime.reloadExtensions(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > reloaded=%t version=%d skills=%d plugins=%d mcp=%d gateway_refreshed=%t gateway_agents=%d\n",
			result.Reloaded, result.Version, result.Skills, result.Plugins, result.MCPCount, result.GatewayRefreshed, result.GatewayAgents)
		return true, c.session, nil
	default:
		return false, c.session, nil
	}
}
