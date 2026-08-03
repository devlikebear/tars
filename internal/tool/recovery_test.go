package tool

import "testing"

func TestToolRecoveryPolicyFailsClosedAndRecognizesReadOnlyTools(t *testing.T) {
	unknown := RecoveryPolicyForTool(Tool{Name: "send_money"})
	if unknown.EffectClass != ToolEffectUnsafe || unknown.HasIdempotencyContract() {
		t.Fatalf("unknown tool policy must fail closed: %+v", unknown)
	}

	read := RecoveryPolicyForTool(Tool{Name: "read_file"})
	if read.EffectClass != ToolEffectReadOnly || !read.SafeToRetryPending() {
		t.Fatalf("read_file policy: %+v", read)
	}

	idempotent := RecoveryPolicyForTool(Tool{
		Name: "send_message",
		Recovery: ToolRecoveryPolicy{
			EffectClass:            ToolEffectIdempotent,
			IdempotencyKeyArgument: "idempotency_key",
		},
	})
	if !idempotent.HasIdempotencyContract() || !idempotent.SafeToRetryPending() {
		t.Fatalf("explicit idempotent policy: %+v", idempotent)
	}
}
