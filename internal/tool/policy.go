package tool

import (
	"errors"
	"sort"
	"strings"
)

type Policy struct {
	AllowTools     []string
	DenyTools      []string
	AllowGroups    []string
	DenyGroups     []string
	UseAllowTools  bool
	UseAllowGroups bool
}

type BlockedToolError struct {
	Tool   string `json:"tool"`
	Rule   string `json:"rule"`
	Group  string `json:"group,omitempty"`
	Source string `json:"source"`
}

func (e BlockedToolError) Error() string {
	toolName := strings.TrimSpace(e.Tool)
	rule := strings.TrimSpace(e.Rule)
	group := strings.TrimSpace(e.Group)
	source := strings.TrimSpace(e.Source)
	if toolName == "" {
		toolName = "unknown"
	}
	if rule == "" {
		rule = "blocked"
	}
	if source == "" {
		source = "unknown"
	}
	b := strings.Builder{}
	b.WriteString("tool not injected for this request: ")
	b.WriteString(toolName)
	b.WriteString(" [rule=")
	b.WriteString(rule)
	b.WriteString(" source=")
	b.WriteString(source)
	if group != "" {
		b.WriteString(" group=")
		b.WriteString(group)
	}
	b.WriteString("]")
	return b.String()
}

type PolicyResolution struct {
	Allowed []string
	Blocked map[string]BlockedToolError
}

func ParseBlockedToolError(err error) (BlockedToolError, bool) {
	if err == nil {
		return BlockedToolError{}, false
	}
	var blocked BlockedToolError
	if !errors.As(err, &blocked) {
		return BlockedToolError{}, false
	}
	return blocked, true
}

func (p Policy) Resolve(all []string, source string) PolicyResolution {
	known := normalizeToolNameSet(all)
	allowedTools := normalizeToolNameSet(p.AllowTools)
	denyTools := normalizeToolNameSet(p.DenyTools)
	allowGroups := normalizeToolGroupSet(p.AllowGroups)
	denyGroups := normalizeToolGroupSet(p.DenyGroups)

	allowed := make([]string, 0, len(known))
	blocked := map[string]BlockedToolError{}
	for _, name := range sortedToolNames(known) {
		group := ToolGroupForName(name)
		switch {
		case group != "" && hasStringSetEntry(denyGroups, group):
			blocked[name] = BlockedToolError{Tool: name, Rule: "group_deny", Group: group, Source: source}
		case hasStringSetEntry(denyTools, name):
			blocked[name] = BlockedToolError{Tool: name, Rule: "tool_deny", Group: group, Source: source}
		case p.UseAllowGroups && !hasStringSetEntry(allowGroups, group):
			blocked[name] = BlockedToolError{Tool: name, Rule: "group_allow", Group: group, Source: source}
		case p.UseAllowTools && !hasStringSetEntry(allowedTools, name):
			blocked[name] = BlockedToolError{Tool: name, Rule: "tool_allow", Group: group, Source: source}
		default:
			allowed = append(allowed, name)
		}
	}
	return PolicyResolution{Allowed: allowed, Blocked: blocked}
}

func normalizeToolNameSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		canonical := CanonicalToolName(value)
		if canonical == "" {
			continue
		}
		out[canonical] = struct{}{}
	}
	return out
}

func normalizeToolGroupSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		canonical := NormalizeToolGroupName(value)
		if canonical == "" {
			continue
		}
		out[canonical] = struct{}{}
	}
	return out
}

func hasStringSetEntry(set map[string]struct{}, value string) bool {
	if len(set) == 0 {
		return false
	}
	_, ok := set[strings.TrimSpace(value)]
	return ok
}

func sortedToolNames(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
