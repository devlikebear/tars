package gateway

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tarsncase/internal/browser"
	"github.com/devlikebear/tarsncase/internal/session"
)

type RunStatus string

const (
	RunStatusAccepted  RunStatus = "accepted"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
)

type Run struct {
	ID                 string    `json:"run_id"`
	WorkspaceID        string    `json:"-"`
	SessionID          string    `json:"session_id,omitempty"`
	Agent              string    `json:"agent,omitempty"`
	Prompt             string    `json:"prompt,omitempty"`
	Status             RunStatus `json:"status"`
	Accepted           bool      `json:"accepted"`
	Response           string    `json:"response,omitempty"`
	Error              string    `json:"error,omitempty"`
	DiagnosticCode     string    `json:"diagnostic_code,omitempty"`
	DiagnosticReason   string    `json:"diagnostic_reason,omitempty"`
	PolicyBlockedTool  string    `json:"policy_blocked_tool,omitempty"`
	PolicyAllowedTools []string  `json:"policy_allowed_tools,omitempty"`
	PolicyDeniedTools  []string  `json:"policy_denied_tools,omitempty"`
	PolicyRiskMax      string    `json:"policy_risk_max,omitempty"`
	CreatedAt          string    `json:"created_at"`
	StartedAt          string    `json:"started_at,omitempty"`
	CompletedAt        string    `json:"completed_at,omitempty"`
	UpdatedAt          string    `json:"updated_at"`
}

type SpawnRequest struct {
	WorkspaceID string
	SessionID   string
	Title       string
	Prompt      string
	Agent       string
}

type ChannelMessage struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"-"`
	ChannelID   string         `json:"channel_id"`
	ThreadID    string         `json:"thread_id,omitempty"`
	Direction   string         `json:"direction"`
	Source      string         `json:"source"`
	Text        string         `json:"text"`
	Payload     map[string]any `json:"payload,omitempty"`
	Timestamp   string         `json:"timestamp"`
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
	Driver             string `json:"driver"`
	Default            bool   `json:"default"`
	Running            bool   `json:"running"`
	ExtensionConnected bool   `json:"extension_connected,omitempty"`
}

type NodeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type GatewayStatus struct {
	Enabled                    bool         `json:"enabled"`
	Version                    int64        `json:"version"`
	RunsTotal                  int          `json:"runs_total"`
	RunsActive                 int          `json:"runs_active"`
	AgentsCount                int          `json:"agents_count"`
	AgentsWatchEnabled         bool         `json:"agents_watch_enabled"`
	AgentsReloadVersion        int64        `json:"agents_reload_version"`
	AgentsLastReloadAt         string       `json:"agents_last_reload_at,omitempty"`
	ChannelsLocal              bool         `json:"channels_local_enabled"`
	ChannelsWebhook            bool         `json:"channels_webhook_enabled"`
	ChannelsTelegram           bool         `json:"channels_telegram_enabled"`
	PersistenceEnabled         bool         `json:"persistence_enabled"`
	RunsPersistenceEnabled     bool         `json:"runs_persistence_enabled"`
	ChannelsPersistenceEnabled bool         `json:"channels_persistence_enabled"`
	RestoreOnStartup           bool         `json:"restore_on_startup"`
	PersistenceDir             string       `json:"persistence_dir,omitempty"`
	RunsRestored               int          `json:"runs_restored"`
	ChannelsRestored           int          `json:"channels_restored"`
	LastPersistAt              string       `json:"last_persist_at,omitempty"`
	LastRestoreAt              string       `json:"last_restore_at,omitempty"`
	LastRestoreError           string       `json:"last_restore_error,omitempty"`
	LastReloadAt               string       `json:"last_reload_at,omitempty"`
	LastRestartAt              string       `json:"last_restart_at,omitempty"`
	Browser                    BrowserState `json:"browser"`
	Nodes                      []NodeInfo   `json:"nodes"`
}

type ReportSummary struct {
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

type ReportRuns struct {
	GeneratedAt    string `json:"generated_at"`
	ArchiveEnabled bool   `json:"archive_enabled"`
	Count          int    `json:"count"`
	Runs           []Run  `json:"runs"`
}

type ReportChannels struct {
	GeneratedAt    string                      `json:"generated_at"`
	ArchiveEnabled bool                        `json:"archive_enabled"`
	Count          int                         `json:"count"`
	Messages       map[string][]ChannelMessage `json:"messages"`
}

type RuntimeOptions struct {
	Enabled                              bool
	WorkspaceDir                         string
	SessionStore                         *session.Store
	SessionStoreForWorkspace             func(workspaceID string) *session.Store
	RunPrompt                            func(ctx context.Context, runLabel string, prompt string) (string, error)
	Executors                            []AgentExecutor
	DefaultAgent                         string
	GatewayAgentsWatchEnabled            bool
	ChannelsLocalEnabled                 bool
	ChannelsWebhookEnabled               bool
	ChannelsTelegramEnabled              bool
	GatewayPersistenceEnabled            bool
	GatewayRunsPersistenceEnabled        bool
	GatewayChannelsPersistenceEnabled    bool
	GatewayRunsMaxRecords                int
	GatewayChannelsMaxMessagesPerChannel int
	GatewayPersistenceDir                string
	GatewayRestoreOnStartup              bool
	GatewayReportSummaryEnabled          bool
	GatewayArchiveEnabled                bool
	GatewayArchiveDir                    string
	GatewayArchiveRetentionDays          int
	GatewayArchiveMaxFileBytes           int
	BrowserDefaultProfile                string
	BrowserManagedHeadless               bool
	BrowserManagedExecutablePath         string
	BrowserManagedUserDataDir            string
	BrowserSiteFlowsDir                  string
	BrowserAutoLoginSiteAllowlist        []string
	BrowserVaultReader                   browser.SecretReader
	BrowserService                       *browser.Service
	Now                                  func() time.Time
}

type runState struct {
	run      Run
	executor AgentExecutor
	cancel   context.CancelFunc
	done     chan struct{}
	closed   bool
}

type Runtime struct {
	opts RuntimeOptions

	nowFn func() time.Time

	mu                  sync.RWMutex
	runs                map[string]*runState
	runOrder            []string
	closed              bool
	runSeq              atomic.Uint64
	messageSeq          atomic.Uint64
	channelMsgs         map[string][]ChannelMessage
	executors           map[string]AgentExecutor
	defaultAgent        string
	agentsWatchEnabled  bool
	agentsReloadVersion int64
	agentsLastReload    time.Time
	browser             BrowserState
	version             int64
	lastReload          time.Time
	lastRestart         time.Time
	runWG               sync.WaitGroup
	stateVersion        uint64
	persistStore        snapshotStore
	lastPersistAt       time.Time
	lastRestoreAt       time.Time
	lastRestoreError    string
	runsRestored        int
	channelsRestored    int
	browserService      *browser.Service
}

const DefaultWorkspaceID = "default"

const defaultWorkspaceID = DefaultWorkspaceID

func NormalizeWorkspaceID(workspaceID string) string {
	trimmed := strings.TrimSpace(workspaceID)
	if trimmed == "" {
		return DefaultWorkspaceID
	}
	return trimmed
}

func normalizeWorkspaceID(workspaceID string) string {
	return NormalizeWorkspaceID(workspaceID)
}
