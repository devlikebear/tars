package project

type UpdatePayload struct {
	ProjectID          string         `json:"project_id,omitempty"`
	Name               *string        `json:"name,omitempty"`
	Type               *string        `json:"type,omitempty"`
	Status             *string        `json:"status,omitempty"`
	GitRepo            *string        `json:"git_repo,omitempty"`
	Objective          *string        `json:"objective,omitempty"`
	Instructions       *string        `json:"instructions,omitempty"`
	ExecutionMode      *string        `json:"execution_mode,omitempty"`
	MaxPhases          *int           `json:"max_phases,omitempty"`
	SubAgents          []string       `json:"sub_agents,omitempty"`
	ToolsAllow         []string       `json:"tools_allow,omitempty"`
	ToolsAllowGroups   []string       `json:"tools_allow_groups,omitempty"`
	ToolsAllowPatterns []string       `json:"tools_allow_patterns,omitempty"`
	ToolsDeny          []string       `json:"tools_deny,omitempty"`
	ToolsRiskMax       *string        `json:"tools_risk_max,omitempty"`
	SkillsAllow        []string       `json:"skills_allow,omitempty"`
	WorkflowProfile    *string        `json:"workflow_profile,omitempty"`
	WorkflowRules      []WorkflowRule `json:"workflow_rules,omitempty"`
	MCPServers         []string       `json:"mcp_servers,omitempty"`
	SecretsRefs        []string       `json:"secrets_refs,omitempty"`
}

func (p UpdatePayload) ToUpdateInput() UpdateInput {
	return UpdateInput{
		Name:               p.Name,
		Type:               p.Type,
		Status:             p.Status,
		GitRepo:            p.GitRepo,
		Objective:          p.Objective,
		Instructions:       p.Instructions,
		ToolsAllow:         p.ToolsAllow,
		ToolsAllowGroups:   p.ToolsAllowGroups,
		ToolsAllowPatterns: p.ToolsAllowPatterns,
		ToolsDeny:          p.ToolsDeny,
		ToolsRiskMax:       p.ToolsRiskMax,
		SkillsAllow:        p.SkillsAllow,
		WorkflowProfile:    p.WorkflowProfile,
		WorkflowRules:      p.WorkflowRules,
		ExecutionMode:      p.ExecutionMode,
		MaxPhases:          p.MaxPhases,
		SubAgents:          p.SubAgents,
		MCPServers:         p.MCPServers,
		SecretsRefs:        p.SecretsRefs,
	}
}
