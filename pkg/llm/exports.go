package llm

import internal "github.com/devlikebear/tars/internal/llm"

type ContentBlock = internal.ContentBlock
type ChatMessage = internal.ChatMessage
type ToolCall = internal.ToolCall
type ToolFunctionSchema = internal.ToolFunctionSchema
type ToolSchema = internal.ToolSchema
type Usage = internal.Usage
type ToolChoiceMode = internal.ToolChoiceMode

const (
	ToolChoiceModeAuto     = internal.ToolChoiceModeAuto
	ToolChoiceModeNone     = internal.ToolChoiceModeNone
	ToolChoiceModeRequired = internal.ToolChoiceModeRequired
	ToolChoiceModeSpecific = internal.ToolChoiceModeSpecific
)

type ToolChoice = internal.ToolChoice
type ResponseFormatType = internal.ResponseFormatType

const (
	ResponseFormatText       = internal.ResponseFormatText
	ResponseFormatJSONObject = internal.ResponseFormatJSONObject
	ResponseFormatJSONSchema = internal.ResponseFormatJSONSchema
)

type ResponseFormat = internal.ResponseFormat
type ChatOptions = internal.ChatOptions
type ClaudeCodeSkill = internal.ClaudeCodeSkill
type ClaudeCodeMCPServer = internal.ClaudeCodeMCPServer
type ChatResponse = internal.ChatResponse
type ClientConfig = internal.ClientConfig
type Client = internal.Client
type ProviderOptions = internal.ProviderOptions
type ProviderError = internal.ProviderError
type ClaudeCodeCLIClient = internal.ClaudeCodeCLIClient
type AntigravityCLIClient = internal.AntigravityCLIClient
type ModelFetcher = internal.ModelFetcher
type Role = internal.Role

const (
	RoleChatMain            = internal.RoleChatMain
	RoleContextCompactor    = internal.RoleContextCompactor
	RoleMemoryHook          = internal.RoleMemoryHook
	RoleReflectionMemory    = internal.RoleReflectionMemory
	RoleReflectionKB        = internal.RoleReflectionKB
	RolePulseDecider        = internal.RolePulseDecider
	RoleSessionCleanup      = internal.RoleSessionCleanup
	RoleAgentRuntimeDefault = internal.RoleAgentRuntimeDefault
	RoleAgentRuntimePlanner = internal.RoleAgentRuntimePlanner
	RoleGoalJudge           = internal.RoleGoalJudge
	RoleCritic              = internal.RoleCritic
)

type Tier = internal.Tier

const (
	TierHeavy    = internal.TierHeavy
	TierStandard = internal.TierStandard
	TierLight    = internal.TierLight
)

type TierResolution = internal.TierResolution
type Router = internal.Router
type TierEntry = internal.TierEntry
type RouterConfig = internal.RouterConfig
type TierRecommendation = internal.TierRecommendation
type CodexRateLimitWindow = internal.CodexRateLimitWindow
type CodexRateLimitSnapshot = internal.CodexRateLimitSnapshot
type CodexRateLimitSource = internal.CodexRateLimitSource

func ToolChoiceAuto() *ToolChoice { return internal.ToolChoiceAuto() }

func ToolChoiceNone() *ToolChoice { return internal.ToolChoiceNone() }

func ToolChoiceRequired() *ToolChoice { return internal.ToolChoiceRequired() }

func ToolChoiceSpecific(name string) *ToolChoice { return internal.ToolChoiceSpecific(name) }

func DefaultClientConfig() ClientConfig { return internal.DefaultClientConfig() }

func NewProvider(opts ProviderOptions) (Client, error) { return internal.NewProvider(opts) }

func NewModelFetcher() ModelFetcher { return internal.NewModelFetcher() }

func AllRoles() []Role { return internal.AllRoles() }

func ParseRole(raw string) (Role, bool) { return internal.ParseRole(raw) }

func AllTiers() []Tier { return internal.AllTiers() }

func ParseTier(raw string) (Tier, error) { return internal.ParseTier(raw) }

func NewRouter(cfg RouterConfig) (Router, error) { return internal.NewRouter(cfg) }

func RecommendTierForTask(message string) TierRecommendation {
	return internal.RecommendTierForTask(message)
}

func FindClaudeCodeCLIPath() (string, error) { return internal.FindClaudeCodeCLIPath() }

func NewClaudeCodeCLIClient(workDir, model string) (*ClaudeCodeCLIClient, error) {
	return internal.NewClaudeCodeCLIClient(workDir, model)
}

func FindAntigravityCLIPath() (string, error) { return internal.FindAntigravityCLIPath() }

func NewAntigravityCLIClient(workDir, model string) (*AntigravityCLIClient, error) {
	return internal.NewAntigravityCLIClient(workDir, model)
}
