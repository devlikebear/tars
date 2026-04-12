package tarsserver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/gateway"
	"gopkg.in/yaml.v3"
)

func parseWorkspaceGatewayAgentDocument(raw string) (workspaceGatewayAgentFrontmatter, string, error) {
	metaBlock, body, hasFrontmatter, err := splitYAMLFrontmatter(raw)
	if err != nil {
		return workspaceGatewayAgentFrontmatter{}, "", err
	}
	if !hasFrontmatter {
		return workspaceGatewayAgentFrontmatter{}, body, nil
	}
	meta, err := parseWorkspaceGatewayAgentFrontmatter(metaBlock)
	if err != nil {
		return workspaceGatewayAgentFrontmatter{}, "", err
	}
	return meta, body, nil
}

func splitYAMLFrontmatter(raw string) (meta string, body string, hasFrontmatter bool, err error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", raw, false, nil
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
			return rest[:end], "", true, nil
		}
		return "", "", false, fmt.Errorf("unterminated frontmatter block")
	}
	return rest[:end], rest[end+len("\n---\n"):], true, nil
}

func parseWorkspaceGatewayAgentFrontmatter(raw string) (workspaceGatewayAgentFrontmatter, error) {
	meta := workspaceGatewayAgentFrontmatter{}
	parsed := map[string]any{}
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		return workspaceGatewayAgentFrontmatter{}, err
	}

	if value, ok := frontmatterValue(parsed, "name"); ok {
		meta.Name = frontmatterString(value)
	}
	if value, ok := frontmatterValue(parsed, "description"); ok {
		meta.Description = frontmatterString(value)
	}
	if value, ok := frontmatterValue(parsed, "tools_allow", "tools-allow"); ok {
		meta.ToolsAllowExists = true
		meta.ToolsAllow = frontmatterStringList(value)
	}
	if value, ok := frontmatterValue(parsed, "tools_deny", "tools-deny"); ok {
		meta.ToolsDenyExists = true
		meta.ToolsDeny = frontmatterStringList(value)
	}
	if value, ok := frontmatterValue(parsed, "tools_risk_max", "tools-risk-max"); ok {
		meta.ToolsRiskMaxExists = true
		meta.ToolsRiskMax = frontmatterString(value)
	}
	if value, ok := frontmatterValue(parsed, "tools_allow_groups", "tools-allow-groups"); ok {
		meta.ToolsAllowGroupsExists = true
		meta.ToolsAllowGroups = frontmatterStringList(value)
	}
	if value, ok := frontmatterValue(parsed, "tools_deny_groups", "tools-deny-groups"); ok {
		meta.ToolsDenyGroupsExists = true
		meta.ToolsDenyGroups = frontmatterStringList(value)
	}
	if value, ok := frontmatterValue(parsed, "tools_allow_patterns", "tools-allow-patterns"); ok {
		meta.ToolsAllowPatternsExists = true
		meta.ToolsAllowPatterns = frontmatterStringList(value)
	}
	if value, ok := frontmatterValue(parsed, "session_routing_mode", "session-routing-mode"); ok {
		meta.SessionRoutingMode = frontmatterString(value)
	}
	if value, ok := frontmatterValue(parsed, "session_fixed_id", "session-fixed-id"); ok {
		meta.SessionFixedID = frontmatterString(value)
	}
	if value, ok := frontmatterValue(parsed, "tier"); ok {
		meta.Tier = frontmatterString(value)
	}
	if value, ok := frontmatterValue(parsed, "provider_override", "provider-override"); ok {
		meta.ProviderOverride = frontmatterProviderOverride(value)
	}
	return meta, nil
}

func frontmatterValue(values map[string]any, keys ...string) (any, bool) {
	if len(values) == 0 || len(keys) == 0 {
		return nil, false
	}
	normalized := make(map[string]any, len(values))
	for key, value := range values {
		normalized[strings.ToLower(strings.TrimSpace(key))] = value
	}
	for _, key := range keys {
		v, ok := normalized[strings.ToLower(strings.TrimSpace(key))]
		if ok {
			return v, true
		}
	}
	return nil, false
}

func frontmatterString(value any) string {
	switch item := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(item)
	case bool:
		if item {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(item)
	case int8:
		return strconv.FormatInt(int64(item), 10)
	case int16:
		return strconv.FormatInt(int64(item), 10)
	case int32:
		return strconv.FormatInt(int64(item), 10)
	case int64:
		return strconv.FormatInt(item, 10)
	case uint:
		return strconv.FormatUint(uint64(item), 10)
	case uint8:
		return strconv.FormatUint(uint64(item), 10)
	case uint16:
		return strconv.FormatUint(uint64(item), 10)
	case uint32:
		return strconv.FormatUint(uint64(item), 10)
	case uint64:
		return strconv.FormatUint(item, 10)
	case float32:
		return strconv.FormatFloat(float64(item), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(item))
	}
}

func frontmatterStringList(value any) []string {
	switch item := value.(type) {
	case nil:
		return []string{}
	case []any:
		out := make([]string, 0, len(item))
		for _, entry := range item {
			trimmed := frontmatterString(entry)
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
		return out
	default:
		trimmed := frontmatterString(item)
		if trimmed == "" {
			return []string{}
		}
		return []string{trimmed}
	}
}

func frontmatterProviderOverride(value any) *gateway.ProviderOverride {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	override := &gateway.ProviderOverride{
		Alias: strings.TrimSpace(frontmatterString(object["alias"])),
		Model: strings.TrimSpace(frontmatterString(object["model"])),
	}
	if override.Alias == "" && override.Model == "" {
		return nil
	}
	return override
}
