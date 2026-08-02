package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

const workLedgerBootstrapActor = "tars-bootstrap"

type workLedgerBootstrapOptions struct {
	WorkspaceDir               string
	AgentRuntimePersistenceDir string
	ActorID                    string
	Logger                     zerolog.Logger
}

type workLedgerBootstrapReport struct {
	DatabasePath                 string                 `json:"database_path"`
	LegacySessionsImported       int                    `json:"legacy_sessions_imported"`
	LegacySessionsReplayed       int                    `json:"legacy_sessions_replayed"`
	AgentRuntimeWorksImported    int                    `json:"agentruntime_works_imported"`
	AgentRuntimeSnapshotReplayed bool                   `json:"agentruntime_snapshot_replayed"`
	QuarantinedSources           int                    `json:"quarantined_sources"`
	Doctor                       workstore.DoctorReport `json:"doctor"`
}

func workLedgerDatabasePath(workspaceDir string) string {
	return filepath.Join(strings.TrimSpace(workspaceDir), "_shared", "work-ledger", "work-ledger.db")
}

func workLedgerQuarantineDir(workspaceDir string) string {
	return filepath.Join(strings.TrimSpace(workspaceDir), "_shared", "work-ledger", "quarantine")
}

func bootstrapWorkLedgerIfEnabled(ctx context.Context, enabled bool, opts workLedgerBootstrapOptions) (*workstore.Store, workLedgerBootstrapReport, error) {
	if !enabled {
		opts.Logger.Info().Msg("work ledger disabled; using legacy session and agent runtime state")
		return nil, workLedgerBootstrapReport{}, nil
	}
	return bootstrapWorkLedger(ctx, opts)
}

func bootstrapWorkLedger(ctx context.Context, opts workLedgerBootstrapOptions) (*workstore.Store, workLedgerBootstrapReport, error) {
	workspaceDir := strings.TrimSpace(opts.WorkspaceDir)
	if workspaceDir == "" {
		return nil, workLedgerBootstrapReport{}, fmt.Errorf("work ledger bootstrap: workspace directory is required")
	}
	actorID := strings.TrimSpace(opts.ActorID)
	if actorID == "" {
		actorID = workLedgerBootstrapActor
	}
	report := workLedgerBootstrapReport{DatabasePath: workLedgerDatabasePath(workspaceDir)}
	store, err := workstore.Open(ctx, report.DatabasePath, workstore.Options{})
	if err != nil {
		return nil, report, fmt.Errorf("work ledger bootstrap: open database: %w", err)
	}
	fail := func(err error) (*workstore.Store, workLedgerBootstrapReport, error) {
		_ = store.Close()
		return nil, report, err
	}

	if err := importLegacySessionSources(ctx, store, workspaceDir, actorID, opts.Logger, &report); err != nil {
		return fail(err)
	}
	runtimeDir := strings.TrimSpace(opts.AgentRuntimePersistenceDir)
	if runtimeDir == "" {
		runtimeDir = filepath.Join(workspaceDir, "_shared", "agentruntime")
	}
	if err := importAgentRuntimeSource(ctx, store, workspaceDir, runtimeDir, actorID, opts.Logger, &report); err != nil {
		return fail(err)
	}
	report.Doctor, err = store.Doctor(ctx, defaultWorkspaceID)
	if err != nil {
		return fail(fmt.Errorf("work ledger bootstrap: doctor failed: %w", err))
	}
	if !report.Doctor.Healthy {
		return fail(fmt.Errorf("work ledger bootstrap: doctor reported %d issue(s)", len(report.Doctor.Issues)))
	}
	opts.Logger.Info().
		Int("legacy_sessions_imported", report.LegacySessionsImported).
		Int("legacy_sessions_replayed", report.LegacySessionsReplayed).
		Int("agentruntime_works_imported", report.AgentRuntimeWorksImported).
		Bool("agentruntime_snapshot_replayed", report.AgentRuntimeSnapshotReplayed).
		Int("quarantined_sources", report.QuarantinedSources).
		Msg("work ledger bootstrap completed")
	return store, report, nil
}

func importLegacySessionSources(ctx context.Context, store *workstore.Store, workspaceDir, actorID string, logger zerolog.Logger, report *workLedgerBootstrapReport) error {
	sessionsDir := filepath.Join(workspaceDir, "sessions")
	indexPath := filepath.Join(sessionsDir, "sessions.json")
	indexJSON, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("work ledger bootstrap: read session index: %w", err)
	}
	var sessions map[string]json.RawMessage
	if len(indexJSON) == 0 || json.Unmarshal(indexJSON, &sessions) != nil || sessions == nil {
		return quarantineBootstrapSource(ctx, store, workstore.QuarantineInput{
			WorkspaceID: defaultWorkspaceID, SourceKind: workstore.ImportSourceLegacySession,
			SourceID: "sessions-index", SourcePath: indexPath, QuarantineDir: workLedgerQuarantineDir(workspaceDir),
			Reason: "session index is not valid JSON", ActorID: actorID,
		}, logger, report)
	}
	ids := make([]string, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, indexID := range ids {
		sessionJSON := sessions[indexID]
		var identity struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(sessionJSON, &identity); err != nil || !safeLegacySessionID(indexID) || strings.TrimSpace(identity.ID) != indexID {
			reason := "session entry has invalid or mismatched id"
			if err != nil {
				reason = "session entry is invalid: " + err.Error()
			}
			if err := quarantineBootstrapSource(ctx, store, workstore.QuarantineInput{
				WorkspaceID: defaultWorkspaceID, SourceKind: workstore.ImportSourceLegacySession,
				SourceID: indexID, SourcePath: indexPath, QuarantineDir: workLedgerQuarantineDir(workspaceDir),
				Reason: reason, ActorID: actorID,
			}, logger, report); err != nil {
				return err
			}
			continue
		}

		tasksPath := filepath.Join(sessionsDir, indexID+".tasks.json")
		tasksJSON, err := os.ReadFile(tasksPath)
		sourcePath := indexPath
		switch {
		case err == nil:
			sourcePath = tasksPath
			if !json.Valid(tasksJSON) {
				if err := quarantineBootstrapSource(ctx, store, workstore.QuarantineInput{
					WorkspaceID: defaultWorkspaceID, SourceKind: workstore.ImportSourceLegacySession,
					SourceID: indexID, SourcePath: tasksPath, QuarantineDir: workLedgerQuarantineDir(workspaceDir),
					Reason: "session tasks document is not valid JSON", ActorID: actorID,
				}, logger, report); err != nil {
					return err
				}
				tasksJSON = []byte(`{"tasks":[]}`)
				sourcePath = indexPath
			}
		case os.IsNotExist(err):
			tasksJSON = []byte(`{"tasks":[]}`)
		default:
			return fmt.Errorf("work ledger bootstrap: read session tasks %q: %w", indexID, err)
		}

		result, err := store.ImportLegacySession(ctx, workstore.LegacySessionImportInput{
			WorkspaceID: defaultWorkspaceID,
			SessionJSON: sessionJSON,
			TasksJSON:   tasksJSON,
			SourcePath:  sourcePath,
			ActorID:     actorID,
		})
		if err != nil {
			quarantinePath := sourcePath
			if quarantinePath == indexPath && tasksPath != "" {
				if _, statErr := os.Stat(tasksPath); statErr == nil {
					quarantinePath = tasksPath
				}
			}
			if err := quarantineBootstrapSource(ctx, store, workstore.QuarantineInput{
				WorkspaceID: defaultWorkspaceID, SourceKind: workstore.ImportSourceLegacySession,
				SourceID: indexID, SourcePath: quarantinePath, QuarantineDir: workLedgerQuarantineDir(workspaceDir),
				Reason: err.Error(), ActorID: actorID,
			}, logger, report); err != nil {
				return err
			}
			continue
		}
		if result.AlreadyImported {
			report.LegacySessionsReplayed++
		} else {
			report.LegacySessionsImported++
		}
	}
	return nil
}

func importAgentRuntimeSource(ctx context.Context, store *workstore.Store, workspaceDir, runtimeDir, actorID string, logger zerolog.Logger, report *workLedgerBootstrapReport) error {
	runsPath := filepath.Join(runtimeDir, "runs.json")
	runsJSON, err := os.ReadFile(runsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("work ledger bootstrap: read agent runtime snapshot: %w", err)
	}
	if len(runsJSON) == 0 || !json.Valid(runsJSON) {
		return quarantineBootstrapSource(ctx, store, workstore.QuarantineInput{
			WorkspaceID: defaultWorkspaceID, SourceKind: workstore.ImportSourceAgentRuntime,
			SourceID: "runs", SourcePath: runsPath, QuarantineDir: workLedgerQuarantineDir(workspaceDir),
			Reason: "agent runtime snapshot is not valid JSON", ActorID: actorID,
		}, logger, report)
	}
	result, err := store.ImportAgentRuntimeSnapshot(ctx, workstore.AgentRuntimeImportInput{
		WorkspaceID:  defaultWorkspaceID,
		SourceID:     "runs",
		SourcePath:   runsPath,
		SnapshotJSON: runsJSON,
		ActorID:      actorID,
	})
	if err != nil {
		return quarantineBootstrapSource(ctx, store, workstore.QuarantineInput{
			WorkspaceID: defaultWorkspaceID, SourceKind: workstore.ImportSourceAgentRuntime,
			SourceID: "runs", SourcePath: runsPath, QuarantineDir: workLedgerQuarantineDir(workspaceDir),
			Reason: err.Error(), ActorID: actorID,
		}, logger, report)
	}
	if result.AlreadyImported {
		report.AgentRuntimeSnapshotReplayed = true
	} else {
		report.AgentRuntimeWorksImported = len(result.WorkIDs)
	}
	return nil
}

func quarantineBootstrapSource(ctx context.Context, store *workstore.Store, input workstore.QuarantineInput, logger zerolog.Logger, report *workLedgerBootstrapReport) error {
	record, err := store.QuarantineSource(ctx, input)
	if err != nil {
		return fmt.Errorf("work ledger bootstrap: quarantine %q: %w", input.SourceID, err)
	}
	if !record.AlreadyQuarantined {
		report.QuarantinedSources++
	}
	logger.Warn().
		Str("source_kind", string(input.SourceKind)).
		Str("source_id", input.SourceID).
		Str("digest", record.Digest).
		Msg("work ledger quarantined legacy source")
	return nil
}

func safeLegacySessionID(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\\`) && filepath.Base(id) == id
}
