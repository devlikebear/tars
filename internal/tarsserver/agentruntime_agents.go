package tarsserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
)

type workspaceAgentRuntimeAgent struct {
	Name               string
	Description        string
	Prompt             string
	FilePath           string
	PolicyMode         string
	ToolsAllow         []string
	ToolsDeny          []string
	ToolsRiskMax       string
	ToolsAllowGroups   []string
	ToolsDenyGroups    []string
	ToolsAllowPatterns []string
	SessionRoutingMode string
	SessionFixedID     string
	Tier               string
	ProviderOverride   *agentruntime.ProviderOverride
}

type workspaceAgentRuntimeAgentFrontmatter struct {
	Name                     string
	Description              string
	ToolsAllow               []string
	ToolsAllowExists         bool
	ToolsDeny                []string
	ToolsDenyExists          bool
	ToolsRiskMax             string
	ToolsRiskMaxExists       bool
	ToolsAllowGroups         []string
	ToolsAllowGroupsExists   bool
	ToolsDenyGroups          []string
	ToolsDenyGroupsExists    bool
	ToolsAllowPatterns       []string
	ToolsAllowPatternsExists bool
	SessionRoutingMode       string
	SessionFixedID           string
	Tier                     string
	ProviderOverride         *agentruntime.ProviderOverride
}

func newWorkspacePromptExecutor(
	def workspaceAgentRuntimeAgent,
	runPrompt func(ctx context.Context, runLabel string, prompt string, allowedTools []string, tier string, providerOverride *agentruntime.ProviderOverride) (string, error),
) (agentruntime.AgentExecutor, error) {
	if runPrompt == nil {
		return nil, fmt.Errorf("run prompt is required")
	}
	name := strings.TrimSpace(def.Name)
	description := strings.TrimSpace(def.Description)
	if description == "" {
		description = "Workspace markdown sub-agent"
	}
	instructions := strings.TrimSpace(def.Prompt)
	return agentruntime.NewPromptExecutorWithOptions(agentruntime.PromptExecutorOptions{
		Name:               name,
		Description:        description,
		Source:             "workspace",
		Entry:              strings.TrimSpace(def.FilePath),
		PolicyMode:         normalizeAgentRuntimePolicyMode(def.PolicyMode),
		ToolsAllow:         append([]string(nil), def.ToolsAllow...),
		ToolsDeny:          append([]string(nil), def.ToolsDeny...),
		ToolsRiskMax:       strings.TrimSpace(def.ToolsRiskMax),
		ToolsAllowGroups:   append([]string(nil), def.ToolsAllowGroups...),
		ToolsDenyGroups:    append([]string(nil), def.ToolsDenyGroups...),
		ToolsAllowPatterns: append([]string(nil), def.ToolsAllowPatterns...),
		SessionRoutingMode: normalizeAgentRuntimeSessionRoutingMode(def.SessionRoutingMode),
		SessionFixedID:     strings.TrimSpace(def.SessionFixedID),
		Tier:               strings.TrimSpace(def.Tier),
		ProviderOverride:   agentruntime.CloneProviderOverride(def.ProviderOverride),
		RunPrompt: func(ctx context.Context, runLabel string, prompt string, allowedTools []string, tier string, providerOverride *agentruntime.ProviderOverride) (string, error) {
			label := strings.TrimSpace(runLabel)
			if label == "" {
				label = "spawn"
			}
			label += ":" + name
			return runPrompt(ctx, label, composeWorkspaceAgentPrompt(name, instructions, prompt), allowedTools, tier, providerOverride)
		},
	})
}

func composeWorkspaceAgentPrompt(name, instructions, userPrompt string) string {
	task := strings.TrimSpace(userPrompt)
	profile := strings.TrimSpace(instructions)
	if profile == "" {
		return task
	}
	var b strings.Builder
	b.WriteString("Sub-agent profile: ")
	b.WriteString(strings.TrimSpace(name))
	b.WriteString("\n\n")
	b.WriteString(profile)
	if task != "" {
		b.WriteString("\n\nUser task:\n")
		b.WriteString(task)
	}
	return b.String()
}
