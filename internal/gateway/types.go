package gateway

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/session"
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
	ID                  string                   `json:"run_id"`
	WorkspaceID         string                   `json:"-"`
	SessionID           string                   `json:"session_id,omitempty"`
	SessionKind         string                   `json:"session_kind,omitempty"`
	Agent               string                   `json:"agent,omitempty"`
	Prompt              string                   `json:"prompt,omitempty"`
	ParentRunID         string                   `json:"parent_run_id,omitempty"`
	RootRunID           string                   `json:"root_run_id,omitempty"`
	ParentSessionID     string                   `json:"parent_session_id,omitempty"`
	Depth               int                      `json:"depth,omitempty"`
	Status              RunStatus                `json:"status"`
	Accepted            bool                     `json:"accepted"`
	Response            string                   `json:"response,omitempty"`
	Error               string                   `json:"error,omitempty"`
	DiagnosticCode      string                   `json:"diagnostic_code,omitempty"`
	DiagnosticReason    string                   `json:"diagnostic_reason,omitempty"`
	PolicyBlockedTool   string                   `json:"policy_blocked_tool,omitempty"`
	PolicyBlockedRule   string                   `json:"policy_blocked_rule,omitempty"`
	PolicyBlockedGroup  string                   `json:"policy_blocked_group,omitempty"`
	PolicyBlockedSource string                   `json:"policy_blocked_source,omitempty"`
	PolicyAllowedTools  []string                 `json:"policy_allowed_tools,omitempty"`
	PolicyDeniedTools   []string                 `json:"policy_denied_tools,omitempty"`
	PolicyRiskMax       string                   `json:"policy_risk_max,omitempty"`
	FlowID              string                   `json:"flow_id,omitempty"`
	StepID              string                   `json:"step_id,omitempty"`
	Tier                string                   `json:"tier,omitempty"`
	ConsensusMode       string                   `json:"consensus_mode,omitempty"`
	ConsensusVariants   []ConsensusVariantRecord `json:"consensus_variants,omitempty"`
	ConsensusCostUSD    float64                  `json:"consensus_cost_usd,omitempty"`
	ConsensusBudgetUSD  float64                  `json:"consensus_budget_usd,omitempty"`
	ProviderOverride    *ProviderOverride        `json:"provider_override,omitempty"`
	ResolvedAlias       string                   `json:"resolved_alias,omitempty"`
	ResolvedKind        string                   `json:"resolved_kind,omitempty"`
	ResolvedModel       string                   `json:"resolved_model,omitempty"`
	OverrideSource      string                   `json:"override_source,omitempty"`
	CreatedAt           string                   `json:"created_at"`
	StartedAt           string                   `json:"started_at,omitempty"`
	CompletedAt         string                   `json:"completed_at,omitempty"`
	UpdatedAt           string                   `json:"updated_at"`
}

type ProviderOverride struct {
	Alias string `json:"alias,omitempty" yaml:"alias,omitempty"`
	Model string `json:"model,omitempty" yaml:"model,omitempty"`
}

type ConsensusSpec struct {
	Strategy   string             `json:"strategy,omitempty"`
	Variants   []ProviderOverride `json:"variants,omitempty"`
	Aggregator *ProviderOverride  `json:"aggregator,omitempty"`
}

type ConsensusVariantRecord struct {
	VariantIdx int     `json:"variant_idx"`
	Alias      string  `json:"alias,omitempty"`
	Kind       string  `json:"kind,omitempty"`
	Model      string  `json:"model,omitempty"`
	Status     string  `json:"status,omitempty"`
	Response   string  `json:"response,omitempty"`
	Error      string  `json:"error,omitempty"`
	TokensIn   int     `json:"tokens_in,omitempty"`
	TokensOut  int     `json:"tokens_out,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`
	StartedAt  string  `json:"started_at,omitempty"`
	FinishedAt string  `json:"finished_at,omitempty"`
}

type RunEvent struct {
	Type            string  `json:"type"`
	RunID           string  `json:"run_id"`
	Timestamp       string  `json:"timestamp,omitempty"`
	Agent           string  `json:"agent,omitempty"`
	Status          string  `json:"status,omitempty"`
	Tier            string  `json:"tier,omitempty"`
	ResolvedAlias   string  `json:"resolved_alias,omitempty"`
	ResolvedKind    string  `json:"resolved_kind,omitempty"`
	ResolvedModel   string  `json:"resolved_model,omitempty"`
	Error           string  `json:"error,omitempty"`
	Message         string  `json:"message,omitempty"`
	Response        string  `json:"response,omitempty"`
	VariantCount    int     `json:"variant_count,omitempty"`
	VariantIdx      int     `json:"variant_idx,omitempty"`
	Alias           string  `json:"alias,omitempty"`
	Kind            string  `json:"kind,omitempty"`
	Model           string  `json:"model,omitempty"`
	Strategy        string  `json:"strategy,omitempty"`
	TokenBudget     int     `json:"token_budget,omitempty"`
	TokensIn        int     `json:"tokens_in,omitempty"`
	TokensOut       int     `json:"tokens_out,omitempty"`
	FinalTokens     int     `json:"final_tokens,omitempty"`
	CostUSDEstimate float64 `json:"cost_usd_estimate,omitempty"`
	CostUSDActual   float64 `json:"cost_usd_actual,omitempty"`
}

type ResolvedProviderOverride struct {
	Alias string `json:"alias,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Model string `json:"model,omitempty"`
	Tier  string `json:"tier,omitempty"`
}

type PromptExecutionMetadata struct {
	ResolvedAlias  string
	ResolvedKind   string
	ResolvedModel  string
	OverrideSource string
}

type SpawnRequest struct {
	WorkspaceID      string
	SessionID        string
	Title            string
	Prompt           string
	Agent            string
	ParentRunID      string
	RootRunID        string
	ParentSessionID  string
	Depth            int
	SessionKind      string
	SessionHidden    bool
	FlowID           string
	StepID           string
	Tier             string
	Mode             string
	Consensus        *ConsensusSpec
	ProviderOverride *ProviderOverride
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

type NodeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type GatewayStatus struct {
	Enabled                    bool       `json:"enabled"`
	Version                    int64      `json:"version"`
	RunsTotal                  int        `json:"runs_total"`
	RunsActive                 int        `json:"runs_active"`
	AgentsCount                int        `json:"agents_count"`
	AgentsWatchEnabled         bool       `json:"agents_watch_enabled"`
	AgentsReloadVersion        int64      `json:"agents_reload_version"`
	AgentsLastReloadAt         string     `json:"agents_last_reload_at,omitempty"`
	ChannelsLocal              bool       `json:"channels_local_enabled"`
	ChannelsWebhook            bool       `json:"channels_webhook_enabled"`
	ChannelsTelegram           bool       `json:"channels_telegram_enabled"`
	PersistenceEnabled         bool       `json:"persistence_enabled"`
	RunsPersistenceEnabled     bool       `json:"runs_persistence_enabled"`
	ChannelsPersistenceEnabled bool       `json:"channels_persistence_enabled"`
	RestoreOnStartup           bool       `json:"restore_on_startup"`
	PersistenceDir             string     `json:"persistence_dir,omitempty"`
	RunsRestored               int        `json:"runs_restored"`
	ChannelsRestored           int        `json:"channels_restored"`
	LastPersistAt              string     `json:"last_persist_at,omitempty"`
	LastRestoreAt              string     `json:"last_restore_at,omitempty"`
	LastRestoreError           string     `json:"last_restore_error,omitempty"`
	LastReloadAt               string     `json:"last_reload_at,omitempty"`
	LastRestartAt              string     `json:"last_restart_at,omitempty"`
	Nodes                      []NodeInfo `json:"nodes"`
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
	GatewaySubagentsMaxThreads           int
	GatewaySubagentsMaxDepth             int
	GatewayConsensusEnabled              bool
	GatewayConsensusMaxFanout            int
	GatewayConsensusBudgetTokens         int
	GatewayConsensusBudgetUSD            float64
	GatewayConsensusTimeoutSeconds       int
	GatewayConsensusAllowedAliases       []string
	GatewayConsensusConcurrentRuns       int
	GatewayPersistenceDir                string
	GatewayRestoreOnStartup              bool
	GatewayReportSummaryEnabled          bool
	GatewayArchiveEnabled                bool
	GatewayArchiveDir                    string
	GatewayArchiveRetentionDays          int
	GatewayArchiveMaxFileBytes           int
	ResolveProviderOverride              func(tier string, override *ProviderOverride) (ResolvedProviderOverride, error)
	EstimateTokensCost                   func(provider, model string, inputTokens, outputTokens int) (float64, bool)
	Now                                  func() time.Time
}

type runState struct {
	run      Run
	req      SpawnRequest
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
	version             int64
	lastReload          time.Time
	lastRestart         time.Time
	runWG               sync.WaitGroup
	executionSem        *executionSemaphore
	runEvents           *runEventBroker
	stateVersion        uint64
	persistStore        snapshotStore
	lastPersistAt       time.Time
	lastRestoreAt       time.Time
	lastRestoreError    string
	runsRestored        int
	channelsRestored    int
	subagentPool        *weightedSemaphore
	consensusRuns       *weightedSemaphore
	consensusPool       *weightedSemaphore
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
