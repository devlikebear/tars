// Package llm is a compatibility shim.
//
// The provider and routing implementation moved to pkg/llm so that the public
// API owns its own types and renders on pkg.go.dev; see #928. This package
// aliases that surface so no in-repo call site had to change in the same
// commit as the move.
//
// New in-repo code should import github.com/devlikebear/tars/pkg/llm directly.
package llm

import pkgllm "github.com/devlikebear/tars/pkg/llm"

type (
	AnthropicClient          = pkgllm.AnthropicClient
	AntigravityCLIClient     = pkgllm.AntigravityCLIClient
	Capability               = pkgllm.Capability
	ChatMessage              = pkgllm.ChatMessage
	ChatOptions              = pkgllm.ChatOptions
	ChatResponse             = pkgllm.ChatResponse
	ClaudeCodeCLIClient      = pkgllm.ClaudeCodeCLIClient
	ClaudeCodeHarnessOptions = pkgllm.ClaudeCodeHarnessOptions
	ClaudeCodeMCPServer      = pkgllm.ClaudeCodeMCPServer
	ClaudeCodeSkill          = pkgllm.ClaudeCodeSkill
	Client                   = pkgllm.Client
	ClientConfig             = pkgllm.ClientConfig
	CodexRateLimitSnapshot   = pkgllm.CodexRateLimitSnapshot
	CodexRateLimitSource     = pkgllm.CodexRateLimitSource
	CodexRateLimitWindow     = pkgllm.CodexRateLimitWindow
	ContentBlock             = pkgllm.ContentBlock
	FakeClient               = pkgllm.FakeClient
	GeminiNativeClient       = pkgllm.GeminiNativeClient
	ModelFetcher             = pkgllm.ModelFetcher
	OpenAICodexClient        = pkgllm.OpenAICodexClient
	OpenAICompatibleClient   = pkgllm.OpenAICompatibleClient
	ProviderError            = pkgllm.ProviderError
	ProviderOptions          = pkgllm.ProviderOptions
	ReasoningBlock           = pkgllm.ReasoningBlock
	ResponseFormat           = pkgllm.ResponseFormat
	ResponseFormatType       = pkgllm.ResponseFormatType
	Role                     = pkgllm.Role
	Router                   = pkgllm.Router
	RouterConfig             = pkgllm.RouterConfig
	SelectionMetadata        = pkgllm.SelectionMetadata
	Tier                     = pkgllm.Tier
	TierEntry                = pkgllm.TierEntry
	TierRecommendation       = pkgllm.TierRecommendation
	TierResolution           = pkgllm.TierResolution
	ToolCall                 = pkgllm.ToolCall
	ToolChoice               = pkgllm.ToolChoice
	ToolChoiceMode           = pkgllm.ToolChoiceMode
	ToolFunctionSchema       = pkgllm.ToolFunctionSchema
	ToolSchema               = pkgllm.ToolSchema
	Usage                    = pkgllm.Usage
)

const (
	CapCacheUsageReporting   = pkgllm.CapCacheUsageReporting
	CapJSONSchemaResponse    = pkgllm.CapJSONSchemaResponse
	CapMultimodalInput       = pkgllm.CapMultimodalInput
	CapPromptCaching         = pkgllm.CapPromptCaching
	CapReasoningEffort       = pkgllm.CapReasoningEffort
	CapServiceTier           = pkgllm.CapServiceTier
	CapSessionResume         = pkgllm.CapSessionResume
	CapStreaming             = pkgllm.CapStreaming
	CapThinkingRoundTrip     = pkgllm.CapThinkingRoundTrip
	CapToolCalling           = pkgllm.CapToolCalling
	ReasoningBlockRedacted   = pkgllm.ReasoningBlockRedacted
	ReasoningBlockThinking   = pkgllm.ReasoningBlockThinking
	ResponseFormatJSONObject = pkgllm.ResponseFormatJSONObject
	ResponseFormatJSONSchema = pkgllm.ResponseFormatJSONSchema
	ResponseFormatText       = pkgllm.ResponseFormatText
	RoleAgentRuntimeDefault  = pkgllm.RoleAgentRuntimeDefault
	RoleAgentRuntimePlanner  = pkgllm.RoleAgentRuntimePlanner
	RoleChatMain             = pkgllm.RoleChatMain
	RoleContextCompactor     = pkgllm.RoleContextCompactor
	RoleCritic               = pkgllm.RoleCritic
	RoleGoalJudge            = pkgllm.RoleGoalJudge
	RoleMemoryHook           = pkgllm.RoleMemoryHook
	RolePulseDecider         = pkgllm.RolePulseDecider
	RoleReflectionKB         = pkgllm.RoleReflectionKB
	RoleReflectionMemory     = pkgllm.RoleReflectionMemory
	RoleSessionCleanup       = pkgllm.RoleSessionCleanup
	TierHeavy                = pkgllm.TierHeavy
	TierLight                = pkgllm.TierLight
	TierStandard             = pkgllm.TierStandard
	ToolChoiceModeAuto       = pkgllm.ToolChoiceModeAuto
	ToolChoiceModeNone       = pkgllm.ToolChoiceModeNone
	ToolChoiceModeRequired   = pkgllm.ToolChoiceModeRequired
	ToolChoiceModeSpecific   = pkgllm.ToolChoiceModeSpecific
)

var (
	AllCapabilities              = pkgllm.AllCapabilities
	AllRoles                     = pkgllm.AllRoles
	AllTiers                     = pkgllm.AllTiers
	CapabilitiesFor              = pkgllm.CapabilitiesFor
	DeclaredProviderKinds        = pkgllm.DeclaredProviderKinds
	DefaultClientConfig          = pkgllm.DefaultClientConfig
	FindAntigravityCLIPath       = pkgllm.FindAntigravityCLIPath
	FindClaudeCodeCLIPath        = pkgllm.FindClaudeCodeCLIPath
	NewAnthropicClient           = pkgllm.NewAnthropicClient
	NewAntigravityCLIClient      = pkgllm.NewAntigravityCLIClient
	NewClaudeCodeCLIClient       = pkgllm.NewClaudeCodeCLIClient
	NewFakeRouter                = pkgllm.NewFakeRouter
	NewGeminiClient              = pkgllm.NewGeminiClient
	NewGeminiNativeClient        = pkgllm.NewGeminiNativeClient
	NewModelFetcher              = pkgllm.NewModelFetcher
	NewOpenAIClient              = pkgllm.NewOpenAIClient
	NewOpenAICodexClient         = pkgllm.NewOpenAICodexClient
	NewProvider                  = pkgllm.NewProvider
	NewRouter                    = pkgllm.NewRouter
	ParseRole                    = pkgllm.ParseRole
	ParseRoleOrKeep              = pkgllm.ParseRoleOrKeep
	ParseTier                    = pkgllm.ParseTier
	ParseTierOrKeep              = pkgllm.ParseTierOrKeep
	RecommendTierForTask         = pkgllm.RecommendTierForTask
	SelectionMetadataFromContext = pkgllm.SelectionMetadataFromContext
	SupportsCapability           = pkgllm.SupportsCapability
	ToolChoiceAuto               = pkgllm.ToolChoiceAuto
	ToolChoiceNone               = pkgllm.ToolChoiceNone
	ToolChoiceRequired           = pkgllm.ToolChoiceRequired
	ToolChoiceSpecific           = pkgllm.ToolChoiceSpecific
	WithSelectionMetadata        = pkgllm.WithSelectionMetadata
)
