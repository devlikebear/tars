package main

type agentDescriptor struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	Enabled            bool     `json:"enabled,omitempty"`
	Kind               string   `json:"kind,omitempty"`
	Source             string   `json:"source,omitempty"`
	Entry              string   `json:"entry,omitempty"`
	Default            bool     `json:"default,omitempty"`
	PolicyMode         string   `json:"policy_mode,omitempty"`
	ToolsAllow         []string `json:"tools_allow,omitempty"`
	ToolsAllowCount    int      `json:"tools_allow_count,omitempty"`
	ToolsDeny          []string `json:"tools_deny,omitempty"`
	ToolsDenyCount     int      `json:"tools_deny_count,omitempty"`
	ToolsRiskMax       string   `json:"tools_risk_max,omitempty"`
	ToolsAllowGroups   []string `json:"tools_allow_groups,omitempty"`
	ToolsAllowPatterns []string `json:"tools_allow_patterns,omitempty"`
	SessionRoutingMode string   `json:"session_routing_mode,omitempty"`
	SessionFixedID     string   `json:"session_fixed_id,omitempty"`
}

type agentRun struct {
	RunID              string   `json:"run_id"`
	SessionID          string   `json:"session_id,omitempty"`
	Agent              string   `json:"agent,omitempty"`
	Status             string   `json:"status"`
	Accepted           bool     `json:"accepted"`
	Response           string   `json:"response,omitempty"`
	Error              string   `json:"error,omitempty"`
	DiagnosticCode     string   `json:"diagnostic_code,omitempty"`
	DiagnosticReason   string   `json:"diagnostic_reason,omitempty"`
	PolicyBlockedTool  string   `json:"policy_blocked_tool,omitempty"`
	PolicyAllowedTools []string `json:"policy_allowed_tools,omitempty"`
	PolicyDeniedTools  []string `json:"policy_denied_tools,omitempty"`
	PolicyRiskMax      string   `json:"policy_risk_max,omitempty"`
	CreatedAt          string   `json:"created_at,omitempty"`
	StartedAt          string   `json:"started_at,omitempty"`
	CompletedAt        string   `json:"completed_at,omitempty"`
}

type gatewayStatus struct {
	Enabled                    bool   `json:"enabled"`
	Version                    int64  `json:"version"`
	RunsTotal                  int    `json:"runs_total"`
	RunsActive                 int    `json:"runs_active"`
	AgentsCount                int    `json:"agents_count"`
	AgentsWatchEnabled         bool   `json:"agents_watch_enabled"`
	AgentsReloadVersion        int64  `json:"agents_reload_version"`
	ChannelsLocalEnabled       bool   `json:"channels_local_enabled"`
	ChannelsWebhookEnabled     bool   `json:"channels_webhook_enabled"`
	ChannelsTelegramEnabled    bool   `json:"channels_telegram_enabled"`
	PersistenceEnabled         bool   `json:"persistence_enabled"`
	RunsPersistenceEnabled     bool   `json:"runs_persistence_enabled"`
	ChannelsPersistenceEnabled bool   `json:"channels_persistence_enabled"`
	RestoreOnStartup           bool   `json:"restore_on_startup"`
	PersistenceDir             string `json:"persistence_dir,omitempty"`
	RunsRestored               int    `json:"runs_restored"`
	ChannelsRestored           int    `json:"channels_restored"`
	LastPersistAt              string `json:"last_persist_at,omitempty"`
	LastRestoreAt              string `json:"last_restore_at,omitempty"`
	LastRestoreError           string `json:"last_restore_error,omitempty"`
	AgentsLastReloadAt         string `json:"agents_last_reload_at,omitempty"`
}

type gatewayReportSummary struct {
	GeneratedAt      string         `json:"generated_at"`
	SummaryEnabled   bool           `json:"summary_enabled"`
	ArchiveEnabled   bool           `json:"archive_enabled"`
	RunsTotal        int            `json:"runs_total"`
	RunsActive       int            `json:"runs_active"`
	RunsByStatus     map[string]int `json:"runs_by_status"`
	ChannelsTotal    int            `json:"channels_total"`
	MessagesTotal    int            `json:"messages_total"`
	MessagesBySource map[string]int `json:"messages_by_source"`
}

type gatewayReportRuns struct {
	GeneratedAt    string     `json:"generated_at"`
	ArchiveEnabled bool       `json:"archive_enabled"`
	Count          int        `json:"count"`
	Runs           []agentRun `json:"runs"`
}

type channelReportMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Source    string `json:"source"`
	Direction string `json:"direction"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type gatewayReportChannels struct {
	GeneratedAt    string                            `json:"generated_at"`
	ArchiveEnabled bool                              `json:"archive_enabled"`
	Count          int                               `json:"count"`
	Messages       map[string][]channelReportMessage `json:"messages"`
}

type browserState struct {
	Running            bool   `json:"running"`
	Profile            string `json:"profile,omitempty"`
	Driver             string `json:"driver,omitempty"`
	CurrentURL         string `json:"current_url,omitempty"`
	LastSnapshot       string `json:"last_snapshot,omitempty"`
	LastAction         string `json:"last_action,omitempty"`
	LastScreenshot     string `json:"last_screenshot,omitempty"`
	ExtensionConnected bool   `json:"extension_connected,omitempty"`
	AttachedTabs       int    `json:"attached_tabs,omitempty"`
	LastError          string `json:"last_error,omitempty"`
}

type browserProfile struct {
	Name               string `json:"name"`
	Driver             string `json:"driver,omitempty"`
	Default            bool   `json:"default,omitempty"`
	Running            bool   `json:"running,omitempty"`
	ExtensionConnected bool   `json:"extension_connected,omitempty"`
}

type browserLoginResult struct {
	SiteID  string `json:"site_id"`
	Profile string `json:"profile,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type browserCheckResult struct {
	SiteID     string `json:"site_id"`
	Profile    string `json:"profile,omitempty"`
	CheckCount int    `json:"check_count"`
	Passed     bool   `json:"passed"`
	Message    string `json:"message,omitempty"`
}

type browserRunResult struct {
	SiteID    string `json:"site_id"`
	Profile   string `json:"profile,omitempty"`
	Action    string `json:"action,omitempty"`
	StepCount int    `json:"step_count"`
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
}

type vaultStatusInfo struct {
	Enabled        bool   `json:"enabled"`
	Ready          bool   `json:"ready,omitempty"`
	AuthMode       string `json:"auth_mode,omitempty"`
	Addr           string `json:"addr,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	AllowlistCount int    `json:"allowlist_count,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

type spawnRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Message   string `json:"message"`
	Agent     string `json:"agent,omitempty"`
}

type spawnCommand struct {
	SessionID string
	Title     string
	Agent     string
	Wait      bool
	Message   string
}
