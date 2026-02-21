package main

import (
	"fmt"
	"net/http"
	"strings"
)

type apiHTTPError struct {
	Method   string
	Endpoint string
	Status   int
	Code     string
	Message  string
	Body     string
}

func (e *apiHTTPError) Error() string {
	if e == nil {
		return ""
	}
	method := strings.TrimSpace(e.Method)
	if method == "" {
		method = http.MethodGet
	}
	endpoint := strings.TrimSpace(e.Endpoint)
	status := e.Status
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = strings.TrimSpace(e.Body)
	}
	if message == "" {
		message = http.StatusText(status)
	}
	code := strings.TrimSpace(e.Code)
	if code != "" {
		return fmt.Sprintf("%s %s status %d [%s]: %s", method, endpoint, status, code, message)
	}
	return fmt.Sprintf("%s %s status %d: %s", method, endpoint, status, message)
}

type sessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type sessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

type statusInfo struct {
	WorkspaceDir string `json:"workspace_dir"`
	SessionCount int    `json:"session_count"`
	AuthRole     string `json:"auth_role,omitempty"`
}

type whoamiInfo struct {
	Authenticated bool   `json:"authenticated"`
	AuthRole      string `json:"auth_role,omitempty"`
	IsAdmin       bool   `json:"is_admin,omitempty"`
	AuthMode      string `json:"auth_mode,omitempty"`
}

type healthInfo struct {
	OK        bool   `json:"ok"`
	Component string `json:"component,omitempty"`
	Time      string `json:"time,omitempty"`
}

type compactInfo struct {
	Message string `json:"message"`
}

type heartbeatInfo struct {
	Response     string `json:"response"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skip_reason,omitempty"`
	Acknowledged bool   `json:"acknowledged"`
	Logged       bool   `json:"logged"`
}

type skillDef struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	UserInvocable bool   `json:"user_invocable"`
	Source        string `json:"source,omitempty"`
	RuntimePath   string `json:"runtime_path,omitempty"`
}

type pluginDef struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
	RootDir string `json:"root_dir,omitempty"`
}

type mcpServerInfo struct {
	Name      string `json:"name"`
	Command   string `json:"command,omitempty"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type mcpToolInfo struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type extensionsReloadInfo struct {
	Reloaded         bool  `json:"reloaded"`
	Version          int64 `json:"version,omitempty"`
	Skills           int   `json:"skills,omitempty"`
	Plugins          int   `json:"plugins,omitempty"`
	MCPCount         int   `json:"mcp_count,omitempty"`
	GatewayRefreshed bool  `json:"gateway_refreshed,omitempty"`
	GatewayAgents    int   `json:"gateway_agents,omitempty"`
}

type cronJob struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	Schedule       string `json:"schedule"`
	Enabled        bool   `json:"enabled"`
	DeleteAfterRun bool   `json:"delete_after_run,omitempty"`
	SessionTarget  string `json:"session_target,omitempty"`
	WakeMode       string `json:"wake_mode,omitempty"`
	DeliveryMode   string `json:"delivery_mode,omitempty"`
	LastRunAt      string `json:"last_run_at,omitempty"`
	LastRunError   string `json:"last_run_error,omitempty"`
}

type cronRunRecord struct {
	JobID    string `json:"job_id"`
	RanAt    string `json:"ran_at"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}
