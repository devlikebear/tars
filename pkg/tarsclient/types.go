package tarsclient

import (
	"fmt"
	"net/http"
	"strings"
)

// DefaultServerURL is the fallback API base URL when Config.ServerURL is empty.
const DefaultServerURL = "http://127.0.0.1:43180"

type Config struct {
	ServerURL     string
	APIToken      string
	AdminAPIToken string
	HTTPClient    *http.Client
}

type APIError struct {
	Method   string
	Endpoint string
	Status   int
	Code     string
	Message  string
	Body     string
}

func (e *APIError) Error() string {
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

type ChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

type ChatEvent struct {
	Type              string `json:"type"`
	Text              string `json:"text"`
	Error             string `json:"error"`
	SessionID         string `json:"session_id"`
	Message           string `json:"message"`
	Phase             string `json:"phase"`
	ToolName          string `json:"tool_name"`
	ToolCallID        string `json:"tool_call_id"`
	ToolArgsPreview   string `json:"tool_args_preview"`
	ToolResultPreview string `json:"tool_result_preview"`
	SkillName         string `json:"skill_name"`
	SkillReason       string `json:"skill_reason"`
}

type ChatResult struct {
	SessionID string
	Assistant string
}

type NotificationMessage struct {
	ID        int64  `json:"id,omitempty"`
	Type      string `json:"type"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	JobID     string `json:"job_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	OpenPath  string `json:"open_path,omitempty"`
}

type EventsHistoryInfo struct {
	Items []NotificationMessage `json:"items"`
	// UnreadCount is based on all retained notification history on server side,
	// not only on this paged Items slice.
	UnreadCount int   `json:"unread_count"`
	ReadCursor  int64 `json:"read_cursor"`
	LastID      int64 `json:"last_id"`
}

type EventsReadInfo struct {
	Acknowledged bool  `json:"acknowledged"`
	ReadCursor   int64 `json:"read_cursor"`
	UnreadCount  int   `json:"unread_count"`
}

type SessionSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type SessionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

type StatusInfo struct {
	WorkspaceDir  string `json:"workspace_dir"`
	SessionCount  int    `json:"session_count"`
	MainSessionID string `json:"main_session_id,omitempty"`
	AuthRole      string `json:"auth_role,omitempty"`
}

type ProviderInfo struct {
	ID                 string `json:"id"`
	SupportsLiveModels bool   `json:"supports_live_models"`
}

type ProvidersInfo struct {
	CurrentProvider string         `json:"current_provider"`
	CurrentModel    string         `json:"current_model"`
	AuthMode        string         `json:"auth_mode"`
	Providers       []ProviderInfo `json:"providers"`
}

type ModelsInfo struct {
	Provider     string   `json:"provider"`
	CurrentModel string   `json:"current_model"`
	Source       string   `json:"source"`
	Stale        bool     `json:"stale"`
	FetchedAt    string   `json:"fetched_at,omitempty"`
	ExpiresAt    string   `json:"expires_at,omitempty"`
	Models       []string `json:"models"`
	Warning      string   `json:"warning,omitempty"`
}

type WhoamiInfo struct {
	Authenticated bool   `json:"authenticated"`
	AuthRole      string `json:"auth_role,omitempty"`
	IsAdmin       bool   `json:"is_admin,omitempty"`
	AuthMode      string `json:"auth_mode,omitempty"`
}

type HealthInfo struct {
	OK        bool   `json:"ok"`
	Component string `json:"component,omitempty"`
	Time      string `json:"time,omitempty"`
}

type CompactRequest struct {
	SessionID        string `json:"session_id"`
	KeepRecent       int    `json:"keep_recent,omitempty"`
	KeepRecentTokens int    `json:"keep_recent_tokens,omitempty"`
	Instructions     string `json:"instructions,omitempty"`
}

type CompactInfo struct {
	Message string `json:"message"`
}

// PulseInfo summarizes the result of a pulse tick triggered via the
// /v1/pulse/run-once endpoint. Fields mirror the server-side
// pulse.TickOutcome with JSON field names aligned to that struct.
type PulseInfo struct {
	Skipped         bool   `json:"skipped,omitempty"`
	SkipReason      string `json:"skip_reason,omitempty"`
	DeciderInvoked  bool   `json:"decider_invoked,omitempty"`
	NotifyDelivered bool   `json:"notify_delivered,omitempty"`
	AutofixAttempt  string `json:"autofix_attempt,omitempty"`
	AutofixOK       bool   `json:"autofix_ok,omitempty"`
	Err             string `json:"err,omitempty"`
}

type SkillDef struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	UserInvocable bool   `json:"user_invocable"`
	Source        string `json:"source,omitempty"`
	RuntimePath   string `json:"runtime_path,omitempty"`
}

type PluginRequires struct {
	Bins []string `json:"bins,omitempty"`
	Env  []string `json:"env,omitempty"`
}

type PluginPolicies struct {
	ToolsAllow []string `json:"tools_allow,omitempty"`
	ToolsDeny  []string `json:"tools_deny,omitempty"`
}

type PluginDef struct {
	SchemaVersion         int            `json:"schema_version,omitempty"`
	ID                    string         `json:"id"`
	Name                  string         `json:"name,omitempty"`
	Version               string         `json:"version,omitempty"`
	Description           string         `json:"description,omitempty"`
	Source                string         `json:"source,omitempty"`
	RootDir               string         `json:"root_dir,omitempty"`
	DefaultProjectProfile string         `json:"default_project_profile,omitempty"`
	SupportedOS           []string       `json:"supported_os,omitempty"`
	SupportedArch         []string       `json:"supported_arch,omitempty"`
	Requires              PluginRequires `json:"requires,omitempty"`
	Policies              PluginPolicies `json:"policies,omitempty"`
}

type MCPServerInfo struct {
	Name      string `json:"name"`
	Command   string `json:"command,omitempty"`
	URL       string `json:"url,omitempty"`
	Transport string `json:"transport,omitempty"`
	Source    string `json:"source,omitempty"`
	AuthMode  string `json:"auth_mode,omitempty"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

type MCPToolInfo struct {
	Server      string `json:"server"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ExtensionsReloadInfo struct {
	Reloaded         bool  `json:"reloaded"`
	Version          int64 `json:"version,omitempty"`
	Skills           int   `json:"skills,omitempty"`
	Plugins          int   `json:"plugins,omitempty"`
	MCPCount         int   `json:"mcp_count,omitempty"`
	GatewayRefreshed bool  `json:"gateway_refreshed,omitempty"`
	GatewayAgents    int   `json:"gateway_agents,omitempty"`
}

type CronJob struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	Schedule       string `json:"schedule"`
	Enabled        bool   `json:"enabled"`
	DeleteAfterRun bool   `json:"delete_after_run,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	SessionTarget  string `json:"session_target,omitempty"`
	WakeMode       string `json:"wake_mode,omitempty"`
	DeliveryMode   string `json:"delivery_mode,omitempty"`
	LastRunAt      string `json:"last_run_at,omitempty"`
	LastRunError   string `json:"last_run_error,omitempty"`
}

type CronRunRecord struct {
	JobID    string `json:"job_id"`
	RanAt    string `json:"ran_at"`
	Response string `json:"response,omitempty"`
	Error    string `json:"error,omitempty"`
}

type UsageLimits struct {
	DailyUSD   float64 `json:"daily_usd"`
	WeeklyUSD  float64 `json:"weekly_usd"`
	MonthlyUSD float64 `json:"monthly_usd"`
	Mode       string  `json:"mode"`
}

type UsageSummary struct {
	Period          string            `json:"period"`
	GroupBy         string            `json:"group_by"`
	TotalCalls      int               `json:"total_calls"`
	TotalCostUSD    float64           `json:"total_cost_usd"`
	TotalInput      int               `json:"total_input_tokens"`
	TotalOutput     int               `json:"total_output_tokens"`
	TotalCached     int               `json:"total_cached_tokens"`
	TotalCacheRead  int               `json:"total_cache_read_tokens"`
	TotalCacheWrite int               `json:"total_cache_write_tokens"`
	Rows            []UsageSummaryRow `json:"rows"`
}

type UsageSummaryRow struct {
	Key              string  `json:"key"`
	Calls            int     `json:"calls"`
	CostUSD          float64 `json:"cost_usd"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CachedTokens     int     `json:"cached_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
}

type UsageLimitStatus struct {
	Exceeded bool    `json:"exceeded"`
	Mode     string  `json:"mode"`
	Period   string  `json:"period,omitempty"`
	SpentUSD float64 `json:"spent_usd,omitempty"`
	LimitUSD float64 `json:"limit_usd,omitempty"`
}

type OpsStatus struct {
	Timestamp       string  `json:"timestamp"`
	DiskTotalBytes  uint64  `json:"disk_total_bytes"`
	DiskFreeBytes   uint64  `json:"disk_free_bytes"`
	DiskUsedPercent float64 `json:"disk_used_percent"`
	ProcessCount    int     `json:"process_count"`
}

type CleanupCandidate struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Reason    string `json:"reason,omitempty"`
}

type CleanupPlan struct {
	ApprovalID string             `json:"approval_id"`
	CreatedAt  string             `json:"created_at,omitempty"`
	TotalBytes int64              `json:"total_bytes"`
	Candidates []CleanupCandidate `json:"candidates"`
}

type Approval struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Status      string      `json:"status"`
	RequestedAt string      `json:"requested_at,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
	ReviewedAt  string      `json:"reviewed_at,omitempty"`
	Plan        CleanupPlan `json:"plan"`
	Note        string      `json:"note,omitempty"`
}

type CleanupApplyResult struct {
	ApprovalID   string   `json:"approval_id"`
	DeletedCount int      `json:"deleted_count"`
	DeletedBytes int64    `json:"deleted_bytes"`
	Errors       []string `json:"errors,omitempty"`
}

type ScheduleItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt,omitempty"`
	Natural   string `json:"natural,omitempty"`
	Schedule  string `json:"schedule"`
	Status    string `json:"status"`
	CronJobID string `json:"cron_job_id,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type ScheduleCreateRequest struct {
	Natural  string `json:"natural"`
	Title    string `json:"title,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type ScheduleUpdateRequest struct {
	Title    *string `json:"title,omitempty"`
	Prompt   *string `json:"prompt,omitempty"`
	Schedule *string `json:"schedule,omitempty"`
	Status   *string `json:"status,omitempty"`
	Timezone *string `json:"timezone,omitempty"`
}

type AgentDescriptor struct {
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

type AgentRun struct {
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

type GatewayStatus struct {
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

type GatewayReportSummary struct {
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

type GatewayReportRuns struct {
	GeneratedAt    string     `json:"generated_at"`
	ArchiveEnabled bool       `json:"archive_enabled"`
	Count          int        `json:"count"`
	Runs           []AgentRun `json:"runs"`
}

type ChannelReportMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Source    string `json:"source"`
	Direction string `json:"direction"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
}

type GatewayReportChannels struct {
	GeneratedAt    string                            `json:"generated_at"`
	ArchiveEnabled bool                              `json:"archive_enabled"`
	Count          int                               `json:"count"`
	Messages       map[string][]ChannelReportMessage `json:"messages"`
}

type BrowserState struct {
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

type BrowserProfile struct {
	Name               string `json:"name"`
	Driver             string `json:"driver,omitempty"`
	Default            bool   `json:"default,omitempty"`
	Running            bool   `json:"running,omitempty"`
	ExtensionConnected bool   `json:"extension_connected,omitempty"`
}

type BrowserLoginResult struct {
	SiteID  string `json:"site_id"`
	Profile string `json:"profile,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type BrowserCheckResult struct {
	SiteID     string `json:"site_id"`
	Profile    string `json:"profile,omitempty"`
	CheckCount int    `json:"check_count"`
	Passed     bool   `json:"passed"`
	Message    string `json:"message,omitempty"`
}

type BrowserRunResult struct {
	SiteID    string `json:"site_id"`
	Profile   string `json:"profile,omitempty"`
	Action    string `json:"action,omitempty"`
	StepCount int    `json:"step_count"`
	Success   bool   `json:"success"`
	Message   string `json:"message,omitempty"`
}

type VaultStatusInfo struct {
	Enabled        bool   `json:"enabled"`
	Ready          bool   `json:"ready,omitempty"`
	AuthMode       string `json:"auth_mode,omitempty"`
	Addr           string `json:"addr,omitempty"`
	Namespace      string `json:"namespace,omitempty"`
	AllowlistCount int    `json:"allowlist_count,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

type TelegramPairingPending struct {
	Code      string `json:"code"`
	UserID    int64  `json:"user_id"`
	ChatID    string `json:"chat_id"`
	Username  string `json:"username,omitempty"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

type TelegramPairingAllowed struct {
	UserID     int64  `json:"user_id"`
	ChatID     string `json:"chat_id"`
	Username   string `json:"username,omitempty"`
	ApprovedAt string `json:"approved_at"`
}

type TelegramPairingsInfo struct {
	DMPolicy       string                   `json:"dm_policy"`
	PollingEnabled bool                     `json:"polling_enabled"`
	Pending        []TelegramPairingPending `json:"pending"`
	Allowed        []TelegramPairingAllowed `json:"allowed"`
}

// SpawnRequest is the API payload for POST /v1/agent/runs.
// CLI parsing types (for example, spawnCommand) stay in internal/tarsclient.
type SpawnRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Message   string `json:"message"`
	Agent     string `json:"agent,omitempty"`
}
