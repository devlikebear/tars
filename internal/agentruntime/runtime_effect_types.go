package agentruntime

type RuntimeToolPhase string

const (
	RuntimeToolPhaseBefore   RuntimeToolPhase = "before_tool"
	RuntimeToolPhaseAfter    RuntimeToolPhase = "after_tool"
	RuntimeToolPhaseProvider RuntimeToolPhase = "provider_tool"
	RuntimeToolPhaseAfterLLM RuntimeToolPhase = "after_llm"
)

type ToolRequestStatus string

const (
	ToolRequestStatusPending   ToolRequestStatus = "pending"
	ToolRequestStatusCommitted ToolRequestStatus = "committed"
	ToolRequestStatusReplayed  ToolRequestStatus = "replayed"
)

type EffectReceiptStatus string

const (
	EffectReceiptStatusPending   EffectReceiptStatus = "pending"
	EffectReceiptStatusCommitted EffectReceiptStatus = "committed"
)

type ToolRequestRecord struct {
	ID                       string            `json:"id"`
	RunID                    string            `json:"run_id"`
	Iteration                int               `json:"iteration"`
	ToolName                 string            `json:"tool_name"`
	ToolCallID               string            `json:"tool_call_id,omitempty"`
	ArgsDigest               string            `json:"args_digest"`
	Signature                string            `json:"signature"`
	EffectClass              string            `json:"effect_class"`
	IdempotencyKey           string            `json:"idempotency_key"`
	DownstreamIdempotencyKey string            `json:"downstream_idempotency_key,omitempty"`
	IdempotencyKeyArgument   string            `json:"idempotency_key_argument,omitempty"`
	SafeToRetryPending       bool              `json:"safe_to_retry_pending"`
	Status                   ToolRequestStatus `json:"status"`
	ResultID                 string            `json:"result_id,omitempty"`
	EffectReceiptID          string            `json:"effect_receipt_id,omitempty"`
	RequestedAt              string            `json:"requested_at"`
	CompletedAt              string            `json:"completed_at,omitempty"`
}

type ToolResultRecord struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	RequestID string `json:"request_id"`
	Digest    string `json:"digest"`
	Result    string `json:"result,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	Replayed  bool   `json:"replayed,omitempty"`
	ReceiptID string `json:"receipt_id,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	CreatedAt string `json:"created_at"`
}

type EffectReceipt struct {
	ID                string              `json:"id"`
	LedgerReceiptID   string              `json:"ledger_receipt_id,omitempty"`
	RunID             string              `json:"run_id"`
	RequestID         string              `json:"request_id"`
	IdempotencyKey    string              `json:"idempotency_key"`
	RequestDigest     string              `json:"request_digest"`
	EffectType        string              `json:"effect_type"`
	Status            EffectReceiptStatus `json:"status"`
	ResultID          string              `json:"result_id,omitempty"`
	ExternalReference string              `json:"external_reference,omitempty"`
	CreatedAt         string              `json:"created_at"`
	CommittedAt       string              `json:"committed_at,omitempty"`
}
