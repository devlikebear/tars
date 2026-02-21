package config

import (
	"encoding/json"
	"strconv"
	"strings"
)

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseBool(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func parseMCPServersJSON(raw string, fallback []MCPServer) []MCPServer {
	var parsed []MCPServer
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]MCPServer, 0, len(parsed))
	for _, server := range parsed {
		name := strings.TrimSpace(server.Name)
		command := strings.TrimSpace(server.Command)
		if name == "" || command == "" {
			continue
		}
		s := MCPServer{
			Name:    name,
			Command: command,
			Args:    append([]string(nil), server.Args...),
		}
		if len(server.Env) > 0 {
			s.Env = make(map[string]string, len(server.Env))
			for k, v := range server.Env {
				s.Env[k] = v
			}
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func parseJSONStringList(raw string, fallback []string) []string {
	var parsed []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]string, 0, len(parsed))
	for _, item := range parsed {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func parseGatewayAgentsJSON(raw string, fallback []GatewayAgent) []GatewayAgent {
	type rawGatewayAgent struct {
		Name           string            `json:"name"`
		Description    string            `json:"description,omitempty"`
		Command        string            `json:"command"`
		Args           []string          `json:"args,omitempty"`
		Env            map[string]string `json:"env,omitempty"`
		WorkingDir     string            `json:"working_dir,omitempty"`
		TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
		Enabled        *bool             `json:"enabled,omitempty"`
	}
	var parsed []rawGatewayAgent
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]GatewayAgent, 0, len(parsed))
	for _, agent := range parsed {
		name := strings.TrimSpace(agent.Name)
		command := strings.TrimSpace(agent.Command)
		if name == "" || command == "" {
			continue
		}
		item := GatewayAgent{
			Name:           name,
			Description:    strings.TrimSpace(agent.Description),
			Command:        command,
			Args:           append([]string(nil), agent.Args...),
			WorkingDir:     strings.TrimSpace(agent.WorkingDir),
			TimeoutSeconds: agent.TimeoutSeconds,
			Enabled:        true,
		}
		if agent.Enabled != nil {
			item.Enabled = *agent.Enabled
		}
		if len(agent.Env) > 0 {
			item.Env = make(map[string]string, len(agent.Env))
			for k, v := range agent.Env {
				item.Env[k] = v
			}
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
