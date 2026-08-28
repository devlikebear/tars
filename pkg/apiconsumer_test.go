// Package pkg_test is a compile-only consumer of every public package.
//
// The API snapshot in docs/public-api-surface.txt catches identifiers that
// appear or disappear. It cannot catch a *signature* change — swapping an
// argument's type, or adding a parameter — because the identifier's name is
// unchanged. This file does: it calls the main entry points the way an
// external consumer would, so a signature change fails the build here.
//
// It deliberately avoids the network and the filesystem. Anything that would
// need credentials is referenced rather than called; the goal is that the
// compiler type-checks these call sites, not that they run.
package pkg_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/devlikebear/tars/pkg/agentloop"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/devlikebear/tars/pkg/mcp"
	"github.com/devlikebear/tars/pkg/memory"
	"github.com/devlikebear/tars/pkg/session"
	"github.com/devlikebear/tars/pkg/skill"
	"github.com/devlikebear/tars/pkg/tarsclient"
	"github.com/devlikebear/tars/pkg/tools"
)

// pinResponseShape exists for its parameter list. Calling it type-checks each
// ChatResponse field a consumer reads, so changing one of their types is a
// compile error here rather than a surprise downstream.
func pinResponseShape(
	content string,
	toolCalls []llm.ToolCall,
	reasoning []llm.ReasoningBlock,
	inputTokens int,
	cacheReadTokens int,
	stopReason string,
	sessionID string,
) {
}

// TestLLMSurfaceCompiles pins the shapes an agent author touches on every
// turn: build a request, read a response, route by tier.
func TestLLMSurfaceCompiles(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hello", ContentBlocks: []llm.ContentBlock{
			{Type: "text", Text: "hello"},
		}},
	}
	opts := llm.ChatOptions{
		Tools: []llm.ToolSchema{{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        "echo",
				Description: "echo back",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		}},
		ToolChoice:      llm.ToolChoiceAuto(),
		ReasoningEffort: "medium",
		ResponseFormat:  &llm.ResponseFormat{Type: llm.ResponseFormatText},
	}

	client := &llm.FakeClient{}
	resp, err := client.Chat(context.Background(), messages, opts)
	if err != nil {
		t.Fatalf("fake chat: %v", err)
	}
	// Pin the response field types through a typed call rather than typed
	// declarations: the parameter list does the same job, and a change to any
	// of these field types stops compiling here.
	pinResponseShape(
		resp.Message.Content,
		resp.Message.ToolCalls,
		resp.Message.ReasoningBlocks,
		resp.Usage.InputTokens,
		resp.Usage.CacheReadTokens,
		resp.StopReason,
		resp.SessionID,
	)

	// Routing.
	router, err := llm.NewRouter(llm.RouterConfig{
		Tiers: map[llm.Tier]llm.TierEntry{
			llm.TierHeavy:    {Client: &llm.FakeClient{}, Provider: "anthropic", Model: "m"},
			llm.TierStandard: {Client: &llm.FakeClient{}, Provider: "anthropic", Model: "m"},
			llm.TierLight:    {Client: &llm.FakeClient{}, Provider: "anthropic", Model: "m"},
		},
		DefaultTier: llm.TierStandard,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	if _, _, err := router.ClientFor(llm.RoleChatMain); err != nil {
		t.Fatalf("client for role: %v", err)
	}
	if _, _, err := router.ClientForTier(llm.TierHeavy); err != nil {
		t.Fatalf("client for tier: %v", err)
	}

	// Capability declarations are part of the surface too.
	if _, declared := llm.CapabilitiesFor("anthropic"); !declared {
		t.Error("anthropic declares no capabilities")
	}
	_ = llm.SupportsCapability("anthropic", llm.CapToolCalling)
	_ = llm.AllCapabilities()
	_ = llm.AllRoles()
	_ = llm.AllTiers()
	_ = llm.RecommendTierForTask("write a compiler")
	_ = llm.DefaultClientConfig()
}

// pinConstructorSignatures type-checks the public constructors without
// invoking them, which matters because most of them would need a credential.
func pinConstructorSignatures(
	newProvider func(llm.ProviderOptions) (llm.Client, error),
	newAnthropic func(string, string, string, int) (*llm.AnthropicClient, error),
	newClaudeCode func(string, string) (*llm.ClaudeCodeCLIClient, error),
	newAntigravity func(string, string) (*llm.AntigravityCLIClient, error),
	findClaudeCode func() (string, error),
	newModelFetcher func() llm.ModelFetcher,
) {
}

// TestLLMConstructorSignaturesCompile references the constructors without
// calling them. examples/min-agent uses a scripted client, so without this
// nothing in the repo would type-check NewProvider's real signature.
func TestLLMConstructorSignaturesCompile(t *testing.T) {
	// Same trick: the parameter types are the assertion.
	pinConstructorSignatures(
		llm.NewProvider,
		llm.NewAnthropicClient,
		llm.NewClaudeCodeCLIClient,
		llm.NewAntigravityCLIClient,
		llm.FindClaudeCodeCLIPath,
		llm.NewModelFetcher,
	)
	// Constructing the options struct pins its field names and types.
	_ = llm.ProviderOptions{
		Provider:        "anthropic",
		Model:           "claude-haiku-4-5",
		APIKey:          "not-used",
		BaseURL:         "https://example.invalid",
		MaxTokens:       1024,
		ReasoningEffort: "low",
		ServiceTier:     "flex",
		BetaFeatures:    []string{"example-beta"},
	}
}

// TestToolsSurfaceCompiles builds a registry the way a consumer would.
func TestToolsSurfaceCompiles(t *testing.T) {
	dir := t.TempDir()
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(dir))
	registry.Register(tools.NewWriteFileTool(dir))
	registry.Register(tools.NewListDirTool(dir))
	registry.Register(tools.NewGlobTool(dir))

	policy := tools.SingleDirPolicy(dir)
	registry.Register(tools.NewReadFileToolWithPolicy(policy))
	registry.Register(tools.NewExecToolWithManager(dir, tools.NewProcessManager()))

	if len(registry.Schemas()) == 0 {
		t.Fatal("registry produced no schemas")
	}
	scoped := tools.NewRegistryWithScope(tools.RegistryScopeUser)
	if scoped == nil {
		t.Fatal("scoped registry is nil")
	}
	_ = tools.JSONTextResult(map[string]string{"ok": "yes"}, false)
	_ = tools.KnownToolGroupNames()
	_ = tools.CanonicalToolName("read_file")
	_ = tools.IsHighRiskToolName("exec")
}

// TestAgentLoopSurfaceCompiles is the documented "minimal agent shape".
func TestAgentLoopSurfaceCompiles(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(t.TempDir()))

	loop := agentloop.New(&llm.FakeClient{}, registry)
	if loop == nil {
		t.Fatal("agentloop.New returned nil")
	}
	_, err := loop.Run(context.Background(),
		[]llm.ChatMessage{{Role: "user", Content: "hi"}},
		agentloop.RunOptions{
			Tools:         registry.Schemas(),
			MaxIterations: agentloop.DefaultMaxLoopIters,
		},
	)
	if err != nil {
		t.Fatalf("loop run: %v", err)
	}
}

// TestSessionMemorySkillSurfacesCompile covers the remaining packages an
// embedding consumer touches.
func TestSessionMemorySkillSurfacesCompile(t *testing.T) {
	dir := t.TempDir()

	store := session.NewStore(dir)
	if store == nil {
		t.Fatal("session.NewStore returned nil")
	}

	// nil semantic service: the file backend works without embeddings, and
	// this pins both the constructor signature and Backend conformance.
	var _ memory.Backend = memory.NewFileBackend(dir, nil)
	_ = memory.NewService

	// skill.Load reads SKILL.md sources; a source dir with no skills yields
	// an empty result, not an error, which is what this pins alongside the
	// LoadOptions shape.
	if _, err := skill.Load(skill.LoadOptions{
		Sources: []skill.SourceDir{{Dir: dir}},
	}); err != nil {
		t.Fatalf("skill.Load: %v", err)
	}
	_ = skill.FormatAvailableSkills
	_ = skill.ParseFrontmatter
}

// TestClientAndMCPSurfacesCompile references the HTTP client and MCP wrapper
// without opening a connection.
func TestClientAndMCPSurfacesCompile(t *testing.T) {
	_ = tarsclient.New
	_ = mcp.NewClient
	_ = mcp.MCPToolName
	var (
		_ mcp.ToolInfo
		_ mcp.ServerStatus
	)
	_ = time.Second
}
