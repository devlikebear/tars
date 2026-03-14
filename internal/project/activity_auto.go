package project

import (
	"fmt"
	"strings"
)

func (s *Store) appendSystemActivity(projectID string, input ActivityAppendInput) error {
	if s == nil {
		return fmt.Errorf("project store is nil")
	}
	if strings.TrimSpace(input.Source) == "" {
		input.Source = ActivitySourceSystem
	}
	_, err := s.AppendActivity(projectID, input)
	return err
}

func projectActivityChanged(before, after Project) bool {
	return before.Name != after.Name ||
		before.Type != after.Type ||
		before.Status != after.Status ||
		before.GitRepo != after.GitRepo ||
		before.Objective != after.Objective ||
		before.Body != after.Body ||
		!stringSlicesEqual(before.ToolsAllow, after.ToolsAllow) ||
		!stringSlicesEqual(before.ToolsAllowGroups, after.ToolsAllowGroups) ||
		!stringSlicesEqual(before.ToolsAllowPatterns, after.ToolsAllowPatterns) ||
		!stringSlicesEqual(before.ToolsDeny, after.ToolsDeny) ||
		before.ToolsRiskMax != after.ToolsRiskMax ||
		!stringSlicesEqual(before.SkillsAllow, after.SkillsAllow) ||
		!stringSlicesEqual(before.MCPServers, after.MCPServers) ||
		!stringSlicesEqual(before.SecretsRefs, after.SecretsRefs)
}

func projectStateActivityChanged(before, after ProjectState) bool {
	return before.Goal != after.Goal ||
		before.Phase != after.Phase ||
		before.Status != after.Status ||
		before.NextAction != after.NextAction ||
		!stringSlicesEqual(before.RemainingTasks, after.RemainingTasks) ||
		before.CompletionSummary != after.CompletionSummary ||
		before.LastRunSummary != after.LastRunSummary ||
		before.LastRunAt != after.LastRunAt ||
		before.StopReason != after.StopReason ||
		before.Body != after.Body
}

func boardTaskActivityChanged(before, after BoardTask) bool {
	return before.Title != after.Title ||
		before.Status != after.Status ||
		before.Assignee != after.Assignee ||
		before.Role != after.Role ||
		before.ReviewRequired != after.ReviewRequired ||
		before.TestCommand != after.TestCommand ||
		before.BuildCommand != after.BuildCommand
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
