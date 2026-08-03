package workerprotocol

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestEnvelopeValidationRequiresVersionedScopedIdentity(t *testing.T) {
	t.Parallel()

	valid := Envelope{
		ProtocolVersion: ProtocolVersionV1,
		MessageID:       "msg-register-1",
		IdempotencyKey:  "worker-a:register",
		Type:            MessageRegister,
		WorkerID:        "worker-a",
		Sequence:        1,
		SentAt:          time.Now().UTC(),
		Payload:         json.RawMessage(`{"transport":"in-process"}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid envelope: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*Envelope)
		want   error
	}{
		{name: "version", mutate: func(input *Envelope) { input.ProtocolVersion = "2.0" }, want: ErrVersionUnsupported},
		{name: "message id", mutate: func(input *Envelope) { input.MessageID = "" }, want: ErrInvalidEnvelope},
		{name: "idempotency", mutate: func(input *Envelope) { input.IdempotencyKey = "" }, want: ErrInvalidEnvelope},
		{name: "worker", mutate: func(input *Envelope) { input.WorkerID = "" }, want: ErrInvalidEnvelope},
		{name: "sequence", mutate: func(input *Envelope) { input.Sequence = 0 }, want: ErrInvalidEnvelope},
		{name: "placement", mutate: func(input *Envelope) { input.Type = MessageExecute }, want: ErrInvalidEnvelope},
		{name: "unknown type", mutate: func(input *Envelope) { input.Type = MessageType("surprise") }, want: ErrInvalidEnvelope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := valid
			tc.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() error=%v want %v", err, tc.want)
			}
		})
	}
}

func TestExecutionPolicyDefaultsToDenyAndRequiresBounds(t *testing.T) {
	t.Parallel()

	policy := DefaultExecutionPolicy()
	if policy.Egress.Mode != EgressDeny || len(policy.Egress.AllowHosts) != 0 {
		t.Fatalf("default egress policy = %+v", policy.Egress)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("default execution policy: %v", err)
	}
	policy.Egress.Mode = EgressAllowlist
	if err := policy.Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("empty egress allowlist error=%v want ErrInvalidPolicy", err)
	}
	policy = DefaultExecutionPolicy()
	policy.Limits.MemoryMB = 0
	if err := policy.Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("unbounded memory error=%v want ErrInvalidPolicy", err)
	}
}
