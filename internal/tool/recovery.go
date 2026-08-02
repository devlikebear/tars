package tool

import "strings"

type ToolEffectClass string

const (
	ToolEffectReadOnly   ToolEffectClass = "read_only"
	ToolEffectIdempotent ToolEffectClass = "idempotent"
	ToolEffectUnsafe     ToolEffectClass = "unsafe"
)

type ToolRecoveryPolicy struct {
	EffectClass            ToolEffectClass `json:"effect_class"`
	IdempotencyKeyArgument string          `json:"idempotency_key_argument,omitempty"`
}

func (p ToolRecoveryPolicy) HasIdempotencyContract() bool {
	return p.EffectClass == ToolEffectIdempotent && strings.TrimSpace(p.IdempotencyKeyArgument) != ""
}

func (p ToolRecoveryPolicy) SafeToRetryPending() bool {
	return p.EffectClass == ToolEffectReadOnly || p.HasIdempotencyContract()
}

func RecoveryPolicyForTool(value Tool) ToolRecoveryPolicy {
	policy := value.Recovery
	policy.IdempotencyKeyArgument = strings.TrimSpace(policy.IdempotencyKeyArgument)
	switch policy.EffectClass {
	case ToolEffectReadOnly:
		policy.IdempotencyKeyArgument = ""
		return policy
	case ToolEffectIdempotent:
		if policy.IdempotencyKeyArgument != "" {
			return policy
		}
		return ToolRecoveryPolicy{EffectClass: ToolEffectUnsafe}
	case ToolEffectUnsafe:
		return policy
	}

	raw := strings.ToLower(strings.TrimSpace(value.Name))
	canonical := CanonicalToolName(raw)
	switch raw {
	case "read", "read_file", "list_dir", "glob", "memory_get", "memory_search", "usage_report", "session_status", "web_search", "web_fetch":
		return ToolRecoveryPolicy{EffectClass: ToolEffectReadOnly}
	}
	switch canonical {
	case "read_file", "list_dir", "glob":
		return ToolRecoveryPolicy{EffectClass: ToolEffectReadOnly}
	default:
		return ToolRecoveryPolicy{EffectClass: ToolEffectUnsafe}
	}
}
