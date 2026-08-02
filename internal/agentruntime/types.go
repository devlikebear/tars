package agentruntime

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/internal/workstore"
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
	ID          string `json:"run_id"`
	WorkspaceID string `json:"-"`
	SessionID   string `json:"session_id,omitempty"`
	// TaskID, when set, names the session.Task this run was spawned to work
	// on. Read-only metadata that lets UI consumers correlate live run state
	// with the task that triggered it. Forwarded from SpawnRequest.TaskID at
	// spawn time and never mutated thereafter.
	TaskID                    string                   `json:"task_id,omitempty"`
	WorkID                    string                   `json:"work_id,omitempty"`
	SessionKind               string                   `json:"session_kind,omitempty"`
	Agent                     string                   `json:"agent,omitempty"`
	Prompt                    string                   `json:"prompt,omitempty"`
	ParentRunID               string                   `json:"parent_run_id,omitempty"`
	RootRunID                 string                   `json:"root_run_id,omitempty"`
	ParentSessionID           string                   `json:"parent_session_id,omitempty"`
	Depth                     int                      `json:"depth,omitempty"`
	RestartedFromRunID        string                   `json:"restarted_from_run_id,omitempty"`
	RestartedFromCheckpointID string                   `json:"restarted_from_checkpoint_id,omitempty"`
	RestartAttempt            int                      `json:"restart_attempt,omitempty"`
	RestartReason             string                   `json:"restart_reason,omitempty"`
	RecoveryMode              RecoveryMode             `json:"recovery_mode,omitempty"`
	Status                    RunStatus                `json:"status"`
	Accepted                  bool                     `json:"accepted"`
	Response                  string                   `json:"response,omitempty"`
	Error                     string                   `json:"error,omitempty"`
	DiagnosticCode            string                   `json:"diagnostic_code,omitempty"`
	DiagnosticReason          string                   `json:"diagnostic_reason,omitempty"`
	PolicyBlockedTool         string                   `json:"policy_blocked_tool,omitempty"`
	PolicyBlockedRule         string                   `json:"policy_blocked_rule,omitempty"`
	PolicyBlockedGroup        string                   `json:"policy_blocked_group,omitempty"`
	PolicyBlockedSource       string                   `json:"policy_blocked_source,omitempty"`
	PolicyAllowedTools        []string                 `json:"policy_allowed_tools,omitempty"`
	PolicyDeniedTools         []string                 `json:"policy_denied_tools,omitempty"`
	PolicyRiskMax             string                   `json:"policy_risk_max,omitempty"`
	FlowID                    string                   `json:"flow_id,omitempty"`
	StepID                    string                   `json:"step_id,omitempty"`
	Tier                      string                   `json:"tier,omitempty"`
	ConsensusMode             string                   `json:"consensus_mode,omitempty"`
	ConsensusVariants         []ConsensusVariantRecord `json:"consensus_variants,omitempty"`
	ConsensusCostUSD          float64                  `json:"consensus_cost_usd,omitempty"`
	ConsensusBudgetUSD        float64                  `json:"consensus_budget_usd,omitempty"`
	FileAttention             []FileAttentionSummary   `json:"file_attention,omitempty"`
	FileOpsTotal              int                      `json:"file_ops_total,omitempty"`
	DiffTimeline              []DiffTimelineEntry      `json:"diff_timeline,omitempty"`
	Checkpoints               []RunCheckpoint          `json:"checkpoints,omitempty"`
	ToolRequests              []ToolRequestRecord      `json:"tool_requests,omitempty"`
	ToolResults               []ToolResultRecord       `json:"tool_results,omitempty"`
	EffectReceipts            []EffectReceipt          `json:"effect_receipts,omitempty"`
	LatestContinuation        *CheckpointContinuation  `json:"latest_continuation,omitempty"`
	ProviderOverride          *ProviderOverride        `json:"provider_override,omitempty"`
	ResolvedAlias             string                   `json:"resolved_alias,omitempty"`
	ResolvedKind              string                   `json:"resolved_kind,omitempty"`
	ResolvedModel             string                   `json:"resolved_model,omitempty"`
	OverrideSource            string                   `json:"override_source,omitempty"`
	CreatedAt                 string                   `json:"created_at"`
	StartedAt                 string                   `json:"started_at,omitempty"`
	CompletedAt               string                   `json:"completed_at,omitempty"`
	UpdatedAt                 string                   `json:"updated_at"`
}

type RunCheckpoint struct {
	SchemaVersion            int                     `json:"schema_version"`
	ID                       string                  `json:"checkpoint_id"`
	RunID                    string                  `json:"run_id,omitempty"`
	Format                   CheckpointFormat        `json:"format"`
	Capability               CheckpointCapability    `json:"capability"`
	Resumable                bool                    `json:"resumable"`
	ResumeReason             string                  `json:"resume_reason"`
	RecoveryModes            []RecoveryMode          `json:"recovery_modes"`
	RecoveryApprovalRequired bool                    `json:"recovery_approval_required,omitempty"`
	RecoveryApprovalReason   string                  `json:"recovery_approval_reason,omitempty"`
	NextAction               string                  `json:"next_action,omitempty"`
	StateRefs                []CheckpointReference   `json:"state_refs,omitempty"`
	ToolRequestRefs          []CheckpointReference   `json:"tool_request_refs,omitempty"`
	ToolResultRefs           []CheckpointReference   `json:"tool_result_refs,omitempty"`
	EffectReceiptRefs        []CheckpointReference   `json:"effect_receipt_refs,omitempty"`
	WorkspaceSnapshotRefs    []CheckpointReference   `json:"workspace_snapshot_refs,omitempty"`
	EnvironmentSnapshotRefs  []CheckpointReference   `json:"environment_snapshot_refs,omitempty"`
	Continuation             *CheckpointContinuation `json:"continuation,omitempty"`
	Kind                     string                  `json:"kind"`
	Label                    string                  `json:"label,omitempty"`
	Status                   RunStatus               `json:"status,omitempty"`
	Agent                    string                  `json:"agent,omitempty"`
	Prompt                   string                  `json:"prompt,omitempty"`
	Tier                     string                  `json:"tier,omitempty"`
	ProviderOverride         *ProviderOverride       `json:"provider_override,omitempty"`
	AllowedTools             []string                `json:"allowed_tools,omitempty"`
	Error                    string                  `json:"error,omitempty"`
	CreatedAt                string                  `json:"created_at"`
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
	ToolName        string  `json:"tool_name,omitempty"`
	ToolCallID      string  `json:"tool_call_id,omitempty"`
	Path            string  `json:"path,omitempty"`
	Action          string  `json:"action,omitempty"`
	ToolIsError     bool    `json:"tool_is_error,omitempty"`
	CheckpointID    string  `json:"checkpoint_id,omitempty"`
	CheckpointKind  string  `json:"checkpoint_kind,omitempty"`
}

type FileAttentionSummary struct {
	Path      string `json:"path"`
	Total     int    `json:"total"`
	Reads     int    `json:"reads,omitempty"`
	Edits     int    `json:"edits,omitempty"`
	Lists     int    `json:"lists,omitempty"`
	Writes    int    `json:"writes,omitempty"`
	FirstAt   string `json:"first_at,omitempty"`
	LastAt    string `json:"last_at,omitempty"`
	Sparkline []int  `json:"sparkline,omitempty"`
}

type DiffTimelineSummary struct {
	Files     int `json:"files"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

type DiffFileChange struct {
	Path            string `json:"path"`
	Status          string `json:"status"`
	Additions       int    `json:"additions,omitempty"`
	Deletions       int    `json:"deletions,omitempty"`
	Patch           string `json:"patch,omitempty"`
	GitInspectorURL string `json:"git_inspector_url,omitempty"`
}

type DiffTimelineEntry struct {
	ID              string              `json:"id"`
	RunID           string              `json:"run_id"`
	SessionID       string              `json:"session_id,omitempty"`
	SessionKind     string              `json:"session_kind,omitempty"`
	Agent           string              `json:"agent,omitempty"`
	Prompt          string              `json:"prompt,omitempty"`
	ParentRunID     string              `json:"parent_run_id,omitempty"`
	RootRunID       string              `json:"root_run_id,omitempty"`
	FlowID          string              `json:"flow_id,omitempty"`
	StepID          string              `json:"step_id,omitempty"`
	StartedAt       string              `json:"started_at,omitempty"`
	CompletedAt     string              `json:"completed_at,omitempty"`
	RepoRoot        string              `json:"repo_root,omitempty"`
	GitInspectorURL string              `json:"git_inspector_url,omitempty"`
	Summary         DiffTimelineSummary `json:"summary"`
	Files           []DiffFileChange    `json:"files,omitempty"`
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
	WorkspaceID               string
	WorkID                    string
	SessionID                 string
	TaskID                    string
	Title                     string
	Prompt                    string
	SystemPromptAppend        string
	Agent                     string
	ParentRunID               string
	RootRunID                 string
	ParentSessionID           string
	Depth                     int
	SessionKind               string
	SessionHidden             bool
	FlowID                    string
	StepID                    string
	Tier                      string
	Mode                      string
	Consensus                 *ConsensusSpec
	ProviderOverride          *ProviderOverride
	RestartedFromRunID        string
	RestartedFromCheckpointID string
	RestartAttempt            int
	RestartReason             string
	RecoveryMode              RecoveryMode
	RecoveryPlan              *RecoveryExecutionPlan
}

type RestartRequest struct {
	WorkspaceID           string
	RunID                 string
	CheckpointID          string
	Agent                 string
	Tier                  string
	ProviderOverride      *ProviderOverride
	PromptAdjustment      string
	Title                 string
	Mode                  RecoveryMode
	ConfirmUnsafeRecovery bool
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

type AgentRuntimeStatus struct {
	Enabled                    bool   `json:"enabled"`
	Version                    int64  `json:"version"`
	RunsTotal                  int    `json:"runs_total"`
	RunsActive                 int    `json:"runs_active"`
	AgentsCount                int    `json:"agents_count"`
	AgentsWatchEnabled         bool   `json:"agents_watch_enabled"`
	AgentsReloadVersion        int64  `json:"agents_reload_version"`
	AgentsLastReloadAt         string `json:"agents_last_reload_at,omitempty"`
	ChannelsLocal              bool   `json:"channels_local_enabled"`
	ChannelsWebhook            bool   `json:"channels_webhook_enabled"`
	ChannelsTelegram           bool   `json:"channels_telegram_enabled"`
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
	LastReloadAt               string `json:"last_reload_at,omitempty"`
	LastRestartAt              string `json:"last_restart_at,omitempty"`
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
	Enabled                                   bool
	WorkspaceDir                              string
	SessionStore                              *session.Store
	SessionStoreForWorkspace                  func(workspaceID string) *session.Store
	RunPrompt                                 func(ctx context.Context, runLabel string, prompt string) (string, error)
	RunPromptCheckpointSupport                ExecutorCheckpointSupport
	Executors                                 []AgentExecutor
	DefaultAgent                              string
	AgentRuntimeAgentsWatchEnabled            bool
	ChannelsLocalEnabled                      bool
	ChannelsWebhookEnabled                    bool
	ChannelsTelegramEnabled                   bool
	AgentRuntimePersistenceEnabled            bool
	AgentRuntimeRunsPersistenceEnabled        bool
	AgentRuntimeChannelsPersistenceEnabled    bool
	AgentRuntimeRunsMaxRecords                int
	AgentRuntimeChannelsMaxMessagesPerChannel int
	AgentRuntimeSubagentsMaxThreads           int
	AgentRuntimeSubagentsMaxDepth             int
	AgentRuntimeConsensusEnabled              bool
	AgentRuntimeConsensusMaxFanout            int
	AgentRuntimeConsensusBudgetTokens         int
	AgentRuntimeConsensusBudgetUSD            float64
	AgentRuntimeConsensusTimeoutSeconds       int
	AgentRuntimeConsensusAllowedAliases       []string
	AgentRuntimeConsensusConcurrentRuns       int
	AgentRuntimePersistenceDir                string
	AgentRuntimeRestoreOnStartup              bool
	AgentRuntimeReportSummaryEnabled          bool
	AgentRuntimeArchiveEnabled                bool
	AgentRuntimeArchiveDir                    string
	AgentRuntimeArchiveRetentionDays          int
	AgentRuntimeArchiveMaxFileBytes           int
	ResolveProviderOverride                   func(tier string, override *ProviderOverride) (ResolvedProviderOverride, error)
	EstimateTokensCost                        func(provider, model string, inputTokens, outputTokens int) (float64, bool)
	UsageTracker                              *usage.Tracker
	// OnRunsSnapshot observes a read-only copy of the current run slice after
	// state changes. It is independent of file persistence so a durable control
	// plane can mirror runs even when legacy runs.json storage is disabled.
	OnRunsSnapshot func(runs []Run)
	Now            func() time.Time
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
	persistMu           sync.Mutex
	persistStore        snapshotStore
	lastPersistAt       time.Time
	lastRestoreAt       time.Time
	lastRestoreError    string
	runsRestored        int
	channelsRestored    int
	subagentPool        *weightedSemaphore
	consensusRuns       *weightedSemaphore
	consensusPool       *weightedSemaphore
	effectReceiptStore  *workstore.Store
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
