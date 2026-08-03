package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/workstore"
)

const maxCheckpointToolResultBytes = 64 * 1024

func (r *Runtime) recordRuntimeExecutionEvent(runID string, call RuntimeToolCall) error {
	switch call.Phase {
	case RuntimeToolPhaseAfterLLM:
		return r.recordRuntimeContinuation(runID, call)
	case RuntimeToolPhaseBefore, RuntimeToolPhaseProvider:
		return r.recordRuntimeToolRequest(runID, call)
	case RuntimeToolPhaseAfter, "":
		return r.recordRuntimeToolResult(runID, call)
	default:
		return fmt.Errorf("agent runtime: unsupported execution event phase %q", call.Phase)
	}
}

func (r *Runtime) recordRuntimeContinuation(runID string, call RuntimeToolCall) error {
	continuationID := strings.TrimSpace(call.ContinuationID)
	if continuationID == "" {
		return nil
	}
	now := r.nowFn().UTC().Format(time.RFC3339)
	r.mu.Lock()
	state := r.runs[strings.TrimSpace(runID)]
	if state == nil {
		r.mu.Unlock()
		return fmt.Errorf("agent runtime: run not found: %s", strings.TrimSpace(runID))
	}
	state.run.LatestContinuation = &CheckpointContinuation{
		Kind: "provider_session", ID: continuationID,
		Executor: agentRuntimeAgentInfo(state.executor).Kind, RecordedAt: now,
	}
	state.run.UpdatedAt = now
	r.appendRunCheckpointLocked(state, "continuation", "Provider continuation", "", resolveRunAllowedTools(
		r.opts.WorkspaceDir, agentRuntimeAgentInfo(state.executor).ToolsAllow,
	))
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return nil
}

func (r *Runtime) recordRuntimeToolRequest(runID string, call RuntimeToolCall) error {
	request := newToolRequestRecord(strings.TrimSpace(runID), call, r.nowFn().UTC())
	if request.ToolName == "" {
		return fmt.Errorf("agent runtime: tool name is required")
	}

	var (
		store     *workstore.Store
		workID    string
		stepID    string
		workspace string
		receipt   *EffectReceipt
	)
	r.mu.Lock()
	state := r.runs[request.RunID]
	if state == nil {
		r.mu.Unlock()
		return fmt.Errorf("agent runtime: run not found: %s", request.RunID)
	}
	if existing := findToolRequest(state.run.ToolRequests, request.ID); existing >= 0 {
		request = state.run.ToolRequests[existing]
	} else {
		state.run.ToolRequests = append(state.run.ToolRequests, request)
	}
	if request.EffectClass != "read_only" && !call.ToolReplayed {
		candidate := EffectReceipt{
			ID: request.ID + ":effect", RunID: request.RunID, RequestID: request.ID,
			IdempotencyKey: request.IdempotencyKey, RequestDigest: request.ArgsDigest,
			EffectType: request.ToolName, Status: EffectReceiptStatusPending,
			CreatedAt: request.RequestedAt,
		}
		if idx := findRuntimeEffectReceipt(state.run.EffectReceipts, candidate.ID); idx >= 0 {
			candidate = state.run.EffectReceipts[idx]
		} else {
			state.run.EffectReceipts = append(state.run.EffectReceipts, candidate)
		}
		receipt = &candidate
		for i := range state.run.ToolRequests {
			if state.run.ToolRequests[i].ID == request.ID {
				state.run.ToolRequests[i].EffectReceiptID = candidate.ID
				break
			}
		}
	}
	state.run.UpdatedAt = request.RequestedAt
	label := "Tool request: " + request.ToolName
	if call.Phase == RuntimeToolPhaseProvider {
		label = "Provider tool observed: " + request.ToolName
	}
	r.appendRunCheckpointLocked(state, "tool_request", label, "", resolveRunAllowedTools(
		r.opts.WorkspaceDir, agentRuntimeAgentInfo(state.executor).ToolsAllow,
	))
	r.stateVersion++
	store = r.effectReceiptStore
	workID = state.run.WorkID
	stepID = state.run.StepID
	workspace = state.run.WorkspaceID
	r.mu.Unlock()
	r.persistSnapshot()

	if receipt == nil || store == nil || strings.TrimSpace(workID) == "" {
		return nil
	}
	stored, err := store.BeginEffectReceipt(context.Background(), workstore.BeginEffectReceiptInput{
		WorkspaceID: workspace, WorkID: workID, StepID: stepID,
		IdempotencyKey: receipt.IdempotencyKey, CausationID: request.RunID,
		EffectType: request.ToolName, Target: request.ToolName,
		RequestDigest: request.ArgsDigest, ActorID: "agent-runtime",
	})
	if err != nil {
		return fmt.Errorf("agent runtime: persist pending effect receipt: %w", err)
	}
	r.mu.Lock()
	if state := r.runs[request.RunID]; state != nil {
		if idx := findRuntimeEffectReceipt(state.run.EffectReceipts, receipt.ID); idx >= 0 {
			state.run.EffectReceipts[idx].LedgerReceiptID = stored.ID
			state.run.UpdatedAt = r.nowFn().UTC().Format(time.RFC3339)
			r.stateVersion++
		}
	}
	r.mu.Unlock()
	r.persistSnapshot()
	return nil
}

func (r *Runtime) recordRuntimeToolResult(runID string, call RuntimeToolCall) error {
	now := r.nowFn().UTC()
	request := newToolRequestRecord(strings.TrimSpace(runID), call, now)
	resultText, truncated := boundedCheckpointResult(secrets.RedactText(call.ToolResult))
	result := ToolResultRecord{
		ID: request.ID + ":result", RunID: request.RunID, RequestID: request.ID,
		Digest: digestText(resultText), Result: resultText, IsError: call.ToolIsError,
		Replayed: call.ToolReplayed, ReceiptID: strings.TrimSpace(call.ToolReceiptID),
		Truncated: truncated, CreatedAt: now.Format(time.RFC3339),
	}

	var (
		store      *workstore.Store
		workspace  string
		workID     string
		receiptKey string
	)
	r.mu.RLock()
	state := r.runs[request.RunID]
	if state != nil {
		if idx := findToolRequest(state.run.ToolRequests, request.ID); idx >= 0 {
			request = state.run.ToolRequests[idx]
		}
		if request.EffectReceiptID != "" {
			if idx := findRuntimeEffectReceipt(state.run.EffectReceipts, request.EffectReceiptID); idx >= 0 {
				receiptKey = state.run.EffectReceipts[idx].IdempotencyKey
			}
		}
		store = r.effectReceiptStore
		workspace = state.run.WorkspaceID
		workID = state.run.WorkID
	}
	r.mu.RUnlock()
	if state == nil {
		return fmt.Errorf("agent runtime: run not found: %s", request.RunID)
	}

	if !call.ToolReplayed && store != nil && workID != "" && receiptKey != "" {
		outcome, _ := json.Marshal(map[string]any{
			"result": result.Result, "result_digest": result.Digest,
			"is_error": result.IsError, "truncated": result.Truncated,
		})
		if _, err := store.CommitEffectReceipt(context.Background(), workstore.CommitEffectReceiptInput{
			WorkspaceID: workspace, WorkID: workID, IdempotencyKey: receiptKey,
			RequestDigest: request.ArgsDigest, OutcomeJSON: outcome,
			ExternalReference: externalReferenceFromToolResult(result.Result), ActorID: "agent-runtime",
		}); err != nil {
			return fmt.Errorf("agent runtime: commit effect receipt: %w", err)
		}
	}

	r.mu.Lock()
	state = r.runs[request.RunID]
	if state == nil {
		r.mu.Unlock()
		return fmt.Errorf("agent runtime: run not found: %s", request.RunID)
	}
	requestIdx := findToolRequest(state.run.ToolRequests, request.ID)
	if requestIdx < 0 {
		state.run.ToolRequests = append(state.run.ToolRequests, request)
		requestIdx = len(state.run.ToolRequests) - 1
	}
	state.run.ToolResults = upsertToolResult(state.run.ToolResults, result)
	state.run.ToolRequests[requestIdx].ResultID = result.ID
	state.run.ToolRequests[requestIdx].CompletedAt = result.CreatedAt
	if call.ToolReplayed {
		state.run.ToolRequests[requestIdx].Status = ToolRequestStatusReplayed
	} else {
		state.run.ToolRequests[requestIdx].Status = ToolRequestStatusCommitted
	}
	if receiptID := state.run.ToolRequests[requestIdx].EffectReceiptID; receiptID != "" && !call.ToolReplayed {
		if idx := findRuntimeEffectReceipt(state.run.EffectReceipts, receiptID); idx >= 0 {
			state.run.EffectReceipts[idx].Status = EffectReceiptStatusCommitted
			state.run.EffectReceipts[idx].ResultID = result.ID
			state.run.EffectReceipts[idx].ExternalReference = externalReferenceFromToolResult(result.Result)
			state.run.EffectReceipts[idx].CommittedAt = result.CreatedAt
			result.ReceiptID = receiptID
			state.run.ToolResults = upsertToolResult(state.run.ToolResults, result)
		}
	}
	state.run.UpdatedAt = result.CreatedAt
	r.appendRunCheckpointLocked(state, "tool_result", "Tool result: "+request.ToolName, "", resolveRunAllowedTools(
		r.opts.WorkspaceDir, agentRuntimeAgentInfo(state.executor).ToolsAllow,
	))
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return nil
}

func newToolRequestRecord(runID string, call RuntimeToolCall, now time.Time) ToolRequestRecord {
	toolName := strings.ToLower(strings.TrimSpace(call.ToolName))
	args := canonicalToolArguments(call.ToolArgs)
	argsDigest := digestText(args)
	iteration := call.Iteration
	identity := fmt.Sprintf("%s\n%d\n%s\n%s\n%s", runID, iteration, strings.TrimSpace(call.ToolCallID), toolName, argsDigest)
	requestID := "toolreq_" + digestText(identity)[:24]
	effectClass := strings.ToLower(strings.TrimSpace(call.ToolEffectClass))
	if effectClass == "" {
		effectClass = defaultRuntimeToolEffectClass(toolName)
	}
	keyArg := strings.TrimSpace(call.ToolIdempotencyKeyArgument)
	downstreamKey := toolArgumentString(args, keyArg)
	safePending := effectClass == "read_only" || (effectClass == "idempotent" && downstreamKey != "")
	return ToolRequestRecord{
		ID: requestID, RunID: runID, Iteration: iteration, ToolName: toolName,
		ToolCallID: strings.TrimSpace(call.ToolCallID), ArgsDigest: argsDigest,
		Signature: toolName + ":" + argsDigest, EffectClass: effectClass,
		IdempotencyKey: "agentruntime:" + requestID, DownstreamIdempotencyKey: downstreamKey,
		IdempotencyKeyArgument: keyArg, SafeToRetryPending: safePending,
		Status: ToolRequestStatusPending, RequestedAt: now.UTC().Format(time.RFC3339),
	}
}

func canonicalToolArguments(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return trimmed
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return trimmed
	}
	return string(normalized)
}

func toolArgumentString(canonicalArgs, key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	var args map[string]any
	if json.Unmarshal([]byte(canonicalArgs), &args) != nil {
		return ""
	}
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func defaultRuntimeToolEffectClass(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "read", "read_file", "list_dir", "glob", "memory_get", "memory_search", "usage_report", "session_status", "web_search", "web_fetch":
		return "read_only"
	default:
		return "unsafe"
	}
}

func digestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func boundedCheckpointResult(value string) (string, bool) {
	if len(value) <= maxCheckpointToolResultBytes {
		return value, false
	}
	return value[:maxCheckpointToolResultBytes], true
}

func externalReferenceFromToolResult(result string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(result)), &payload) != nil {
		return ""
	}
	for _, key := range []string{"external_reference", "message_id", "id"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findToolRequest(requests []ToolRequestRecord, id string) int {
	for i := range requests {
		if requests[i].ID == id {
			return i
		}
	}
	return -1
}

func findRuntimeEffectReceipt(receipts []EffectReceipt, id string) int {
	for i := range receipts {
		if receipts[i].ID == id {
			return i
		}
	}
	return -1
}

func upsertToolResult(results []ToolResultRecord, result ToolResultRecord) []ToolResultRecord {
	for i := range results {
		if results[i].ID == result.ID {
			results[i] = result
			return results
		}
	}
	return append(results, result)
}
