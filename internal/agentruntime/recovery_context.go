package agentruntime

import (
	"context"
	"strings"
)

type RecoveryToolResult struct {
	RequestID string `json:"request_id"`
	Signature string `json:"signature"`
	Result    string `json:"result"`
	IsError   bool   `json:"is_error,omitempty"`
	ReceiptID string `json:"receipt_id,omitempty"`
}

type RecoveryExecutionPlan struct {
	Mode            RecoveryMode         `json:"mode"`
	SourceRunID     string               `json:"source_run_id"`
	CheckpointID    string               `json:"checkpoint_id"`
	ContinuationID  string               `json:"continuation_id,omitempty"`
	ToolResults     []RecoveryToolResult `json:"tool_results,omitempty"`
	Requests        []ToolRequestRecord  `json:"requests,omitempty"`
	RecordedResults []ToolResultRecord   `json:"recorded_results,omitempty"`
	EffectReceipts  []EffectReceipt      `json:"effect_receipts,omitempty"`
}

type recoveryExecutionContextKey struct{}

func WithRecoveryExecution(ctx context.Context, plan *RecoveryExecutionPlan) context.Context {
	if plan == nil {
		return ctx
	}
	cloned := cloneRecoveryExecutionPlan(plan)
	return context.WithValue(ctx, recoveryExecutionContextKey{}, cloned)
}

func RecoveryExecutionFromContext(ctx context.Context) *RecoveryExecutionPlan {
	if ctx == nil {
		return nil
	}
	plan, _ := ctx.Value(recoveryExecutionContextKey{}).(*RecoveryExecutionPlan)
	return cloneRecoveryExecutionPlan(plan)
}

func MatchRecoveryToolResult(plan *RecoveryExecutionPlan, toolName, toolArgs string) (RecoveryToolResult, bool) {
	if plan == nil || plan.Mode != RecoveryModeReplayFromCheckpoint {
		return RecoveryToolResult{}, false
	}
	signature := ToolCallSignature(toolName, toolArgs)
	for _, result := range plan.ToolResults {
		if result.Signature == signature {
			return result, true
		}
	}
	return RecoveryToolResult{}, false
}

// ConsumeRecoveryToolResult returns and removes the earliest recorded result
// matching a tool call. Replay callers must consume results in order so two
// identical calls do not both reuse the first effect receipt.
func ConsumeRecoveryToolResult(plan *RecoveryExecutionPlan, toolName, toolArgs string) (RecoveryToolResult, bool) {
	if plan == nil || plan.Mode != RecoveryModeReplayFromCheckpoint {
		return RecoveryToolResult{}, false
	}
	signature := ToolCallSignature(toolName, toolArgs)
	for index, result := range plan.ToolResults {
		if result.Signature != signature {
			continue
		}
		plan.ToolResults = append(plan.ToolResults[:index], plan.ToolResults[index+1:]...)
		return result, true
	}
	return RecoveryToolResult{}, false
}

func ToolCallSignature(toolName, toolArgs string) string {
	return strings.ToLower(strings.TrimSpace(toolName)) + ":" + digestText(canonicalToolArguments(toolArgs))
}

func cloneRecoveryExecutionPlan(plan *RecoveryExecutionPlan) *RecoveryExecutionPlan {
	if plan == nil {
		return nil
	}
	cloned := *plan
	cloned.ToolResults = append([]RecoveryToolResult(nil), plan.ToolResults...)
	cloned.Requests = append([]ToolRequestRecord(nil), plan.Requests...)
	cloned.RecordedResults = append([]ToolResultRecord(nil), plan.RecordedResults...)
	cloned.EffectReceipts = append([]EffectReceipt(nil), plan.EffectReceipts...)
	return &cloned
}

func inheritedRecoveryRecords(plan *RecoveryExecutionPlan) ([]ToolRequestRecord, []ToolResultRecord, []EffectReceipt) {
	if plan == nil {
		return nil, nil, nil
	}
	return append([]ToolRequestRecord(nil), plan.Requests...),
		append([]ToolResultRecord(nil), plan.RecordedResults...),
		append([]EffectReceipt(nil), plan.EffectReceipts...)
}
