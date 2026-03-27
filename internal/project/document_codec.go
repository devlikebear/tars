package project

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func parseDocument(raw string) (Project, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	metaRaw, body, hasMeta, err := splitFrontmatter(normalized)
	if err != nil {
		return Project{}, err
	}
	if !hasMeta {
		return Project{Body: strings.TrimSpace(normalized)}, nil
	}
	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(metaRaw), &parsed); err != nil {
		return Project{}, fmt.Errorf("parse project frontmatter: %w", err)
	}
	item := Project{
		ID:                 mapString(parsed, "id"),
		Name:               mapString(parsed, "name"),
		Type:               normalizeType(mapString(parsed, "type")),
		Status:             normalizeStatus(mapString(parsed, "status")),
		GitRepo:            mapString(parsed, "git_repo", "git-repo"),
		CreatedAt:          mapString(parsed, "created_at", "created-at"),
		UpdatedAt:          mapString(parsed, "updated_at", "updated-at"),
		Objective:          mapString(parsed, "objective"),
		ToolsAllow:         mapStringList(parsed, "tools_allow", "tools-allow"),
		ToolsAllowGroups:   mapStringList(parsed, "tools_allow_groups", "tools-allow-groups"),
		ToolsAllowPatterns: mapStringList(parsed, "tools_allow_patterns", "tools-allow-patterns"),
		ToolsDeny:          mapStringList(parsed, "tools_deny", "tools-deny"),
		ToolsRiskMax:       mapString(parsed, "tools_risk_max", "tools-risk-max"),
		SkillsAllow:        mapStringList(parsed, "skills_allow", "skills-allow"),
		WorkflowProfile:    normalizeWorkflowProfile(mapString(parsed, "workflow_profile", "workflow-profile")),
		WorkflowRules:      mapWorkflowRuleList(parsed, "workflow_rules", "workflow-rules"),
		MCPServers:         mapStringList(parsed, "mcp_servers", "mcp-servers"),
		SecretsRefs:        mapStringList(parsed, "secrets_refs", "secrets-refs"),
		Body:               strings.TrimSpace(body),
	}
	if item.CreatedAt == "" {
		item.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if item.UpdatedAt == "" {
		item.UpdatedAt = item.CreatedAt
	}
	return item, nil
}

func buildDocument(project Project) string {
	project.Type = normalizeType(project.Type)
	project.Status = normalizeStatus(project.Status)

	var b strings.Builder
	b.WriteString("---\n")
	_, _ = fmt.Fprintf(&b, "id: %s\n", strings.TrimSpace(project.ID))
	_, _ = fmt.Fprintf(&b, "name: %s\n", quoteYAML(project.Name))
	_, _ = fmt.Fprintf(&b, "type: %s\n", strings.TrimSpace(project.Type))
	_, _ = fmt.Fprintf(&b, "status: %s\n", strings.TrimSpace(project.Status))
	if v := strings.TrimSpace(project.GitRepo); v != "" {
		_, _ = fmt.Fprintf(&b, "git_repo: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(project.CreatedAt); v != "" {
		_, _ = fmt.Fprintf(&b, "created_at: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(project.UpdatedAt); v != "" {
		_, _ = fmt.Fprintf(&b, "updated_at: %s\n", quoteYAML(v))
	}
	if v := strings.TrimSpace(project.Objective); v != "" {
		_, _ = fmt.Fprintf(&b, "objective: %s\n", quoteYAML(v))
	}
	writeDocumentList(&b, "tools_allow", project.ToolsAllow)
	writeDocumentList(&b, "tools_allow_groups", project.ToolsAllowGroups)
	writeDocumentList(&b, "tools_allow_patterns", project.ToolsAllowPatterns)
	writeDocumentList(&b, "tools_deny", project.ToolsDeny)
	if v := strings.TrimSpace(project.ToolsRiskMax); v != "" {
		_, _ = fmt.Fprintf(&b, "tools_risk_max: %s\n", quoteYAML(v))
	}
	writeDocumentList(&b, "skills_allow", project.SkillsAllow)
	if v := strings.TrimSpace(project.WorkflowProfile); v != "" {
		_, _ = fmt.Fprintf(&b, "workflow_profile: %s\n", quoteYAML(v))
	}
	writeWorkflowRuleList(&b, "workflow_rules", project.WorkflowRules)
	writeDocumentList(&b, "mcp_servers", project.MCPServers)
	writeDocumentList(&b, "secrets_refs", project.SecretsRefs)
	b.WriteString("---\n")
	if body := strings.TrimSpace(project.Body); body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func writeDocumentList(b *strings.Builder, key string, values []string) {
	vals := normalizeList(values)
	if len(vals) == 0 {
		return
	}
	_, _ = fmt.Fprintf(b, "%s:\n", key)
	for _, item := range vals {
		_, _ = fmt.Fprintf(b, "  - %s\n", quoteYAML(item))
	}
}

func writeWorkflowRuleList(b *strings.Builder, key string, rules []WorkflowRule) {
	items := normalizeWorkflowRules(rules)
	if len(items) == 0 {
		return
	}
	_, _ = fmt.Fprintf(b, "%s:\n", key)
	for _, rule := range items {
		_, _ = fmt.Fprintf(b, "  - name: %s\n", quoteYAML(rule.Name))
		if len(rule.Params) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(b, "    params:\n")
		keys := make([]string, 0, len(rule.Params))
		for key := range rule.Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, paramKey := range keys {
			_, _ = fmt.Fprintf(b, "      %s: %s\n", quoteYAML(paramKey), quoteYAML(rule.Params[paramKey]))
		}
	}
}

func splitFrontmatter(raw string) (string, string, bool, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", normalized, false, nil
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			return rest[:len(rest)-len("\n---")], "", true, nil
		}
		return "", "", false, fmt.Errorf("unterminated frontmatter")
	}
	meta := rest[:end]
	body := rest[end+len("\n---\n"):]
	return meta, body, true, nil
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func mapStringList(values map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case []any:
			items := make([]string, 0, len(v))
			for _, entry := range v {
				items = append(items, fmt.Sprint(entry))
			}
			return normalizeList(items)
		case []string:
			return normalizeList(v)
		case string:
			return normalizeList([]string{v})
		}
	}
	return nil
}

func mapWorkflowRuleList(values map[string]any, keys ...string) []WorkflowRule {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		data, err := yaml.Marshal(raw)
		if err != nil {
			continue
		}
		var rules []WorkflowRule
		if err := yaml.Unmarshal(data, &rules); err != nil {
			continue
		}
		return normalizeWorkflowRules(rules)
	}
	return nil
}
