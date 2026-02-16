package toolpolicy

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tarsncase/internal/tool"
)

type ProviderPolicy struct {
	Profile string   `json:"profile"`
	Allow   []string `json:"allow"`
	Deny    []string `json:"deny"`
}

type Policy struct {
	Profile    string                    `json:"profile"`
	Allow      []string                  `json:"allow"`
	Deny       []string                  `json:"deny"`
	ByProvider map[string]ProviderPolicy `json:"by_provider"`
}

type SelectorConfig struct {
	Mode       string
	MaxTools   int
	AutoExpand bool
}

type Selector struct {
	policy Policy
	cfg    SelectorConfig
}

func NewSelector(policy Policy, cfg SelectorConfig) Selector {
	return Selector{policy: policy, cfg: cfg}
}

func (s Selector) FilteredTools(tools []tool.Tool, provider, model string) []tool.Tool {
	return FilterTools(tools, s.policy, provider, model)
}

func (s Selector) Select(tools []tool.Tool, provider, model, userMessage string) []string {
	filtered := FilterTools(tools, s.policy, provider, model)
	if len(filtered) == 0 {
		return nil
	}
	if strings.TrimSpace(strings.ToLower(s.cfg.Mode)) != "heuristic" {
		out := make([]string, 0, len(filtered))
		for _, t := range filtered {
			out = append(out, t.Name)
		}
		return out
	}
	maxTools := s.cfg.MaxTools
	if maxTools <= 0 {
		maxTools = 12
	}
	type scored struct {
		name  string
		score int
	}
	items := make([]scored, 0, len(filtered))
	for _, t := range filtered {
		score := heuristicScore(t, userMessage)
		if strings.EqualFold(t.Name, "session_status") {
			score += 4
		}
		items = append(items, scored{name: t.Name, score: score})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].name < items[j].name
		}
		return items[i].score > items[j].score
	})
	out := make([]string, 0, minInt(maxTools, len(items)))
	seen := map[string]struct{}{}
	for _, it := range items {
		if len(out) >= maxTools {
			break
		}
		if it.score <= 0 && len(out) > 0 {
			continue
		}
		if _, ok := seen[it.name]; ok {
			continue
		}
		seen[it.name] = struct{}{}
		out = append(out, it.name)
	}
	if len(out) == 0 {
		for i := 0; i < len(items) && len(out) < maxTools; i++ {
			out = append(out, items[i].name)
		}
	}
	if len(out) < maxTools {
		if _, ok := seen["session_status"]; !ok {
			for _, t := range filtered {
				if strings.EqualFold(t.Name, "session_status") {
					out = append(out, t.Name)
					break
				}
			}
		}
	}
	return out
}

func heuristicScore(t tool.Tool, userMessage string) int {
	msg := normalize(userMessage)
	if msg == "" {
		return 0
	}
	name := normalize(t.Name)
	desc := normalize(t.Description)
	score := tokenOverlapScore(name+" "+desc, msg)

	switch {
	case containsAny(msg, "read", "file", "path", "directory", "dir", "write", "edit", "patch", "glob") && containsAny(name, "read", "write", "edit", "patch", "glob", "list_dir"):
		score += 3
	case containsAny(msg, "command", "shell", "execute", "run", "process") && containsAny(name, "exec", "process"):
		score += 3
	case containsAny(msg, "cron", "schedule", "job") && containsAny(name, "cron"):
		score += 4
	case containsAny(msg, "heartbeat") && containsAny(name, "heartbeat"):
		score += 4
	case containsAny(msg, "web", "search", "url", "http") && containsAny(name, "web"):
		score += 3
	case containsAny(msg, "remember", "memory", "기억", "기억해") && containsAny(name, "memory"):
		score += 3
	}

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.Name)), "mcp.") {
		if score > 0 {
			score += 2
		}
	}
	return score
}

func tokenOverlapScore(text, query string) int {
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return 0
	}
	score := 0
	for _, tok := range tokens {
		if len(tok) < 2 {
			continue
		}
		if strings.Contains(text, tok) {
			score++
		}
	}
	return score
}

func containsAny(text string, keys ...string) bool {
	for _, k := range keys {
		if strings.Contains(text, k) {
			return true
		}
	}
	return false
}

func normalize(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	repl := strings.NewReplacer(".", " ", "_", " ", "-", " ", "/", " ", ":", " ", "\n", " ", "\t", " ")
	return strings.Join(strings.Fields(repl.Replace(v)), " ")
}

func FilterTools(tools []tool.Tool, policy Policy, provider, model string) []tool.Tool {
	if len(tools) == 0 {
		return nil
	}
	effective := mergeEffectivePolicy(policy, provider, model)
	allNames := make([]string, 0, len(tools))
	for _, t := range tools {
		allNames = append(allNames, t.Name)
	}
	base := profileBaseSet(effective.Profile, allNames)
	allowed := base
	if len(effective.Allow) > 0 {
		allowSet := expandEntries(effective.Allow, allNames)
		if len(allowSet) > 0 {
			for n := range allowSet {
				allowed[n] = struct{}{}
			}
		}
	}
	if len(effective.Deny) > 0 {
		denySet := expandEntries(effective.Deny, allNames)
		for n := range denySet {
			delete(allowed, n)
		}
	}
	out := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(t.Name))]; ok {
			out = append(out, t)
		}
	}
	return out
}

func mergeEffectivePolicy(base Policy, provider, model string) Policy {
	effective := Policy{
		Profile: strings.TrimSpace(base.Profile),
		Allow:   append([]string(nil), base.Allow...),
		Deny:    append([]string(nil), base.Deny...),
	}
	if len(base.ByProvider) == 0 {
		return effective
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	keys := []string{}
	if provider != "" && model != "" {
		keys = append(keys, provider+"/"+model)
	}
	if provider != "" {
		keys = append(keys, provider)
	}
	for _, key := range keys {
		for raw, p := range base.ByProvider {
			if strings.EqualFold(strings.TrimSpace(raw), key) {
				if strings.TrimSpace(p.Profile) != "" {
					effective.Profile = strings.TrimSpace(p.Profile)
				}
				effective.Allow = append(effective.Allow, p.Allow...)
				effective.Deny = append(effective.Deny, p.Deny...)
			}
		}
	}
	return effective
}

func profileBaseSet(profile string, allNames []string) map[string]struct{} {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" || profile == "full" {
		return setFromNames(allNames)
	}
	entries := []string{}
	switch profile {
	case "minimal":
		entries = []string{"session_status"}
	case "coding":
		entries = []string{"group:fs", "group:runtime", "group:sessions", "group:memory", "image"}
	case "messaging":
		entries = []string{"group:messaging", "sessions_list", "sessions_history", "sessions_send", "session_status"}
	default:
		return setFromNames(allNames)
	}
	set := expandEntries(entries, allNames)
	if len(set) == 0 {
		return setFromNames(allNames)
	}
	return set
}

func expandEntries(entries []string, allNames []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(entry), "group:") {
			group := strings.ToLower(entry)
			for _, name := range expandGroup(group, allNames) {
				out[strings.ToLower(name)] = struct{}{}
			}
			continue
		}
		for _, n := range allNames {
			if wildcardMatch(n, entry) {
				out[strings.ToLower(n)] = struct{}{}
			}
		}
	}
	return out
}

func expandGroup(group string, allNames []string) []string {
	groups := map[string][]string{
		"group:runtime":    {"exec", "bash", "process"},
		"group:fs":         {"read", "read_file", "write", "write_file", "edit", "edit_file", "apply_patch", "glob", "list_dir"},
		"group:sessions":   {"sessions_list", "sessions_history", "sessions_send", "sessions_spawn", "session_status"},
		"group:memory":     {"memory_search", "memory_get"},
		"group:web":        {"web_search", "web_fetch"},
		"group:ui":         {"browser", "canvas"},
		"group:automation": {"cron", "cron_list", "cron_create", "cron_update", "cron_delete", "cron_run", "heartbeat", "heartbeat_status", "heartbeat_run_once", "gateway"},
		"group:messaging":  {"message"},
		"group:nodes":      {"nodes"},
	}
	if group == "group:openclaw" {
		out := make([]string, 0, len(allNames))
		for _, n := range allNames {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(n)), "mcp.") {
				continue
			}
			out = append(out, n)
		}
		return out
	}
	patterns := groups[group]
	out := make([]string, 0, len(patterns))
	for _, n := range allNames {
		for _, p := range patterns {
			if strings.EqualFold(n, p) {
				out = append(out, n)
				break
			}
		}
	}
	return out
}

func wildcardMatch(value, pattern string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	ok, err := filepath.Match(pattern, value)
	if err != nil {
		return value == pattern
	}
	return ok
}

func setFromNames(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	return out
}

func intersectSets(a, b map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for k := range a {
		if _, ok := b[k]; ok {
			out[k] = struct{}{}
		}
	}
	return out
}

func ExpandEntriesForTest(entries []string, allNames []string) []string {
	set := expandEntries(entries, allNames)
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
