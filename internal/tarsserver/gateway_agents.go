package tarsserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/gateway"
)

type workspaceGatewayAgent struct {
	Name               string
	Description        string
	Prompt             string
	FilePath           string
	PolicyMode         string
	ToolsAllow         []string
	ToolsDeny          []string
	ToolsRiskMax       string
	ToolsAllowGroups   []string
	ToolsAllowPatterns []string
	SessionRoutingMode string
	SessionFixedID     string
}

type workspaceGatewayAgentFrontmatter struct {
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
	ToolsAllowPatterns       []string
	ToolsAllowPatternsExists bool
	SessionRoutingMode       string
	SessionFixedID           string
}

func newWorkspacePromptExecutor(
	def workspaceGatewayAgent,
	runPrompt func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error),
) (gateway.AgentExecutor, error) {
	if runPrompt == nil {
		return nil, fmt.Errorf("run prompt is required")
	}
	name := strings.TrimSpace(def.Name)
	description := strings.TrimSpace(def.Description)
	if description == "" {
		description = "Workspace markdown sub-agent"
	}
	instructions := strings.TrimSpace(def.Prompt)
	return gateway.NewPromptExecutorWithOptions(gateway.PromptExecutorOptions{
		Name:               name,
		Description:        description,
		Source:             "workspace",
		Entry:              strings.TrimSpace(def.FilePath),
		PolicyMode:         normalizeGatewayPolicyMode(def.PolicyMode),
		ToolsAllow:         append([]string(nil), def.ToolsAllow...),
		ToolsDeny:          append([]string(nil), def.ToolsDeny...),
		ToolsRiskMax:       strings.TrimSpace(def.ToolsRiskMax),
		ToolsAllowGroups:   append([]string(nil), def.ToolsAllowGroups...),
		ToolsAllowPatterns: append([]string(nil), def.ToolsAllowPatterns...),
		SessionRoutingMode: normalizeGatewaySessionRoutingMode(def.SessionRoutingMode),
		SessionFixedID:     strings.TrimSpace(def.SessionFixedID),
		RunPrompt: func(ctx context.Context, runLabel string, prompt string, allowedTools []string) (string, error) {
			label := strings.TrimSpace(runLabel)
			if label == "" {
				label = "spawn"
			}
			label += ":" + name
			return runPrompt(ctx, label, composeWorkspaceAgentPrompt(name, instructions, prompt), allowedTools)
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
