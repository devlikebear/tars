package gateway

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (r *Runtime) initExecutors() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.applyExecutorsLocked(r.opts.Executors, r.opts.DefaultAgent)
	r.markAgentsReloadLocked()
	r.stateVersion++
}

func (r *Runtime) SetExecutors(executors []AgentExecutor, defaultAgent string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.opts.Executors = append([]AgentExecutor(nil), executors...)
	r.opts.DefaultAgent = strings.TrimSpace(defaultAgent)
	r.applyExecutorsLocked(r.opts.Executors, r.opts.DefaultAgent)
	r.markAgentsReloadLocked()
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
}

func (r *Runtime) SetAgentsWatchEnabled(enabled bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.agentsWatchEnabled = enabled
	r.mu.Unlock()
}

func (r *Runtime) applyExecutorsLocked(executors []AgentExecutor, requestedDefault string) {
	r.executors = map[string]AgentExecutor{}
	r.defaultAgent = ""

	registered := false
	for _, executor := range executors {
		if r.registerExecutorLocked(executor) {
			registered = true
		}
	}
	if r.opts.RunPrompt != nil {
		if _, exists := r.executors["default"]; !exists {
			if ex, err := NewPromptExecutorWithOptions(PromptExecutorOptions{
				Name:        "default",
				Description: "Default in-process agent loop",
				Source:      "in-process",
				Entry:       "llm-loop",
				RunPrompt: func(ctx context.Context, runLabel string, prompt string, _ []string, _ string, _ *ProviderOverride) (string, error) {
					return r.opts.RunPrompt(ctx, runLabel, prompt)
				},
			}); err == nil && ex != nil {
				r.executors["default"] = ex
				registered = true
			}
		}
	}

	requested := strings.TrimSpace(requestedDefault)
	if requested != "" {
		if _, ok := r.executors[requested]; ok {
			r.defaultAgent = requested
		}
	}
	if r.defaultAgent == "" {
		if _, ok := r.executors["default"]; ok {
			r.defaultAgent = "default"
		}
	}
	if r.defaultAgent == "" && registered {
		names := r.executorNamesLocked()
		if len(names) > 0 {
			r.defaultAgent = names[0]
		}
	}
}

func (r *Runtime) markAgentsReloadLocked() {
	r.agentsReloadVersion++
	r.agentsLastReload = r.nowFn().UTC()
}

func (r *Runtime) registerExecutorLocked(executor AgentExecutor) bool {
	if executor == nil {
		return false
	}
	info := executor.Info()
	name := strings.TrimSpace(info.Name)
	if name == "" {
		return false
	}
	r.executors[name] = executor
	return true
}

func (r *Runtime) executorNamesLocked() []string {
	names := make([]string, 0, len(r.executors))
	for name := range r.executors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Runtime) resolveExecutor(agentName string) (string, AgentExecutor, error) {
	if err := r.requireEnabled(); err != nil {
		return "", nil, err
	}
	requested := strings.TrimSpace(agentName)

	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.executors) == 0 {
		return "", nil, fmt.Errorf("no agent executors are configured")
	}
	if requested == "" {
		requested = strings.TrimSpace(r.defaultAgent)
	}
	if requested == "" {
		return "", nil, fmt.Errorf("default agent is not configured")
	}
	executor, ok := r.executors[requested]
	if ok {
		return requested, executor, nil
	}
	names := r.executorNamesLocked()
	return "", nil, fmt.Errorf("unknown agent %q (available: %s)", requested, strings.Join(names, ", "))
}

func (r *Runtime) Agents() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := r.executorNamesLocked()
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		executor := r.executors[name]
		if executor == nil {
			continue
		}
		info := executor.Info()
		toolsAllow := append([]string{}, info.ToolsAllow...)
		out = append(out, map[string]any{
			"name":                 info.Name,
			"description":          info.Description,
			"enabled":              r.opts.Enabled && info.Enabled,
			"kind":                 info.Kind,
			"source":               info.Source,
			"entry":                info.Entry,
			"default":              info.Name == r.defaultAgent,
			"policy_mode":          info.PolicyMode,
			"tools_allow":          toolsAllow,
			"tools_allow_count":    info.ToolsAllowCount,
			"tools_deny":           append([]string{}, info.ToolsDeny...),
			"tools_deny_count":     info.ToolsDenyCount,
			"tools_risk_max":       info.ToolsRiskMax,
			"tools_allow_groups":   append([]string{}, info.ToolsAllowGroups...),
			"tools_deny_groups":    append([]string{}, info.ToolsDenyGroups...),
			"tools_allow_patterns": append([]string{}, info.ToolsAllowPatterns...),
			"session_routing_mode": normalizeSessionRoutingMode(info.SessionRoutingMode),
			"session_fixed_id":     strings.TrimSpace(info.SessionFixedID),
			"tier":                 strings.TrimSpace(info.Tier),
			"provider_override":    CloneProviderOverride(info.ProviderOverride),
		})
	}
	return out
}

func (r *Runtime) LookupAgent(name string) (AgentInfo, bool) {
	if r == nil {
		return AgentInfo{}, false
	}
	key := strings.TrimSpace(name)
	if key == "" {
		return AgentInfo{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[key]
	if !ok || executor == nil {
		return AgentInfo{}, false
	}
	return executor.Info(), true
}

func (r *Runtime) SubagentLimits() (maxThreads int, maxDepth int) {
	if r == nil {
		return 0, 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.opts.GatewaySubagentsMaxThreads, r.opts.GatewaySubagentsMaxDepth
}

func gatewayAgentInfo(executor AgentExecutor) AgentInfo {
	if executor == nil {
		return AgentInfo{}
	}
	return executor.Info()
}
