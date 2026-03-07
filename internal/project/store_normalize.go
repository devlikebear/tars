package project

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func applyUpdateInput(item *Project, input UpdateInput) error {
	if input.Name != nil {
		v := strings.TrimSpace(*input.Name)
		if v == "" {
			return fmt.Errorf("name is required")
		}
		item.Name = v
	}
	if input.Type != nil {
		item.Type = normalizeType(*input.Type)
	}
	if input.Status != nil {
		item.Status = normalizeStatus(*input.Status)
	}
	if input.GitRepo != nil {
		item.GitRepo = strings.TrimSpace(*input.GitRepo)
	}
	if input.Objective != nil {
		item.Objective = strings.TrimSpace(*input.Objective)
	}
	if input.Instructions != nil {
		item.Body = strings.TrimSpace(*input.Instructions)
	}
	if len(input.ToolsAllow) > 0 {
		item.ToolsAllow = normalizeList(input.ToolsAllow)
	}
	if len(input.ToolsAllowGroups) > 0 {
		item.ToolsAllowGroups = normalizeList(input.ToolsAllowGroups)
	}
	if len(input.ToolsAllowPatterns) > 0 {
		item.ToolsAllowPatterns = normalizeList(input.ToolsAllowPatterns)
	}
	if len(input.ToolsDeny) > 0 {
		item.ToolsDeny = normalizeList(input.ToolsDeny)
	}
	if input.ToolsRiskMax != nil {
		item.ToolsRiskMax = strings.TrimSpace(strings.ToLower(*input.ToolsRiskMax))
	}
	if len(input.SkillsAllow) > 0 {
		item.SkillsAllow = normalizeList(input.SkillsAllow)
	}
	if len(input.MCPServers) > 0 {
		item.MCPServers = normalizeList(input.MCPServers)
	}
	if len(input.SecretsRefs) > 0 {
		item.SecretsRefs = normalizeList(input.SecretsRefs)
	}
	return nil
}

func normalizeProjectForWrite(project Project, nowFn func() time.Time) (Project, error) {
	project.ID = strings.TrimSpace(project.ID)
	if project.ID == "" {
		return Project{}, fmt.Errorf("project id is required")
	}
	project.Name = strings.TrimSpace(project.Name)
	if project.Name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	project.Type = normalizeType(project.Type)
	project.Status = normalizeStatus(project.Status)
	if project.CreatedAt == "" {
		project.CreatedAt = nowFn().UTC().Format(time.RFC3339)
	}
	if project.UpdatedAt == "" {
		project.UpdatedAt = nowFn().UTC().Format(time.RFC3339)
	}
	project.ToolsAllow = normalizeList(project.ToolsAllow)
	project.ToolsAllowGroups = normalizeList(project.ToolsAllowGroups)
	project.ToolsAllowPatterns = normalizeList(project.ToolsAllowPatterns)
	project.ToolsDeny = normalizeList(project.ToolsDeny)
	project.SkillsAllow = normalizeList(project.SkillsAllow)
	project.MCPServers = normalizeList(project.MCPServers)
	project.SecretsRefs = normalizeList(project.SecretsRefs)
	project.ToolsRiskMax = strings.TrimSpace(strings.ToLower(project.ToolsRiskMax))
	return project, nil
}

func normalizeType(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "development", "research", "operations", "general":
		return v
	default:
		return "general"
	}
}

func normalizeStatus(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "active", "paused", "completed", "archived":
		return v
	default:
		return "active"
	}
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func quoteYAML(v string) string {
	trimmed := strings.TrimSpace(v)
	trimmed = strings.ReplaceAll(trimmed, "\"", "\\\"")
	return "\"" + trimmed + "\""
}

func newProjectID(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.ReplaceAll(slug, " ", "-")
	var b strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	base := strings.Trim(b.String(), "-_")
	if base == "" {
		base = "project"
	}
	randPart := make([]byte, 3)
	if _, err := rand.Read(randPart); err != nil {
		return base + "-" + fmt.Sprint(time.Now().UTC().UnixNano())
	}
	return base + "-" + hex.EncodeToString(randPart)
}

func parseTime(raw string) time.Time {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}
