package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/workstore"
)

func syncAgentRuntimeRunsToWorkLedger(
	ctx context.Context,
	ledger *workstore.Store,
	workspaceID string,
	sourcePath string,
	runs []agentruntime.Run,
	actorID string,
) error {
	if ledger == nil {
		return nil
	}
	payload, err := json.Marshal(struct {
		Runs []agentruntime.Run `json:"runs"`
	}{Runs: append([]agentruntime.Run(nil), runs...)})
	if err != nil {
		return fmt.Errorf("encode agent runtime snapshot for work ledger: %w", err)
	}
	_, err = ledger.ImportAgentRuntimeSnapshot(ctx, workstore.AgentRuntimeImportInput{
		WorkspaceID:  normalizeWorkspaceID(workspaceID),
		SourceID:     "runs",
		SourcePath:   strings.TrimSpace(sourcePath),
		SnapshotJSON: payload,
		ActorID:      strings.TrimSpace(actorID),
	})
	if err != nil {
		return fmt.Errorf("import agent runtime work ledger revision: %w", err)
	}
	return nil
}
