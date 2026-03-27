package project

import (
	"strings"
	"time"
)

type WorkflowRuntimePolicy struct {
	PlanningBlockTimeout time.Duration
	RunRetention         time.Duration
}

func ResolveWorkflowRuntimePolicy(project Project) WorkflowRuntimePolicy {
	policy := WorkflowRuntimePolicy{
		PlanningBlockTimeout: defaultPlanningBlockTimeout,
		RunRetention:         defaultAutopilotRunRetention,
	}
	applyWorkflowRuntimeRuleOverrides(&policy, project.WorkflowRules)
	return policy
}

func applyWorkflowRuntimeRuleOverrides(policy *WorkflowRuntimePolicy, rules []WorkflowRule) {
	if policy == nil || len(rules) == 0 {
		return
	}
	for _, rule := range rules {
		duration, ok := workflowRuleDuration(rule.Params)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(rule.Name)) {
		case "planning_block_timeout":
			policy.PlanningBlockTimeout = duration
		case "run_retention":
			policy.RunRetention = duration
		}
	}
}

func workflowRuleDuration(params map[string]string) (time.Duration, bool) {
	if len(params) == 0 {
		return 0, false
	}
	raw := strings.TrimSpace(params["duration"])
	if raw == "" {
		return 0, false
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, false
	}
	return duration, true
}
