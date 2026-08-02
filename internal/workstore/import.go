package workstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ImportSourceKind string

const (
	ImportSourceLegacySession ImportSourceKind = "legacy-session"
	ImportSourceAgentRuntime  ImportSourceKind = "agentruntime"
)

type ImportStatus string

const (
	ImportStatusCompleted   ImportStatus = "completed"
	ImportStatusFailed      ImportStatus = "failed"
	ImportStatusQuarantined ImportStatus = "quarantined"
)

type ImportMarker struct {
	SchemaVersion int
	ID            string
	WorkspaceID   string
	SourceKind    ImportSourceKind
	SourceID      string
	SourcePath    string
	Checksum      string
	Status        ImportStatus
	WorkIDs       []string
	ActorID       string
	ErrorText     string
	ImportedAt    time.Time
	UpdatedAt     time.Time
}

type ImportResult struct {
	Marker          ImportMarker
	WorkIDs         []string
	AlreadyImported bool
}

type LegacySessionImportInput struct {
	WorkspaceID string
	SessionJSON []byte
	TasksJSON   []byte
	SourcePath  string
	ActorID     string
}

type AgentRuntimeImportInput struct {
	WorkspaceID  string
	SourceID     string
	SourcePath   string
	SnapshotJSON []byte
	ActorID      string
}

type legacySessionDocument struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Kind      string             `json:"kind"`
	Goal      *legacySessionGoal `json:"goal"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
}

type legacySessionGoal struct {
	Description string `json:"description"`
	Status      string `json:"status"`
}

type legacyTasksDocument struct {
	Plan     *legacyPlan     `json:"plan"`
	Contract json.RawMessage `json:"contract"`
	Tasks    []legacyTask    `json:"tasks"`
}

type legacyPlan struct {
	Goal        string `json:"goal"`
	Constraints string `json:"constraints"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type legacyTask struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Status      string           `json:"status"`
	Description string           `json:"description"`
	RunID       string           `json:"run_id"`
	Evidence    []legacyEvidence `json:"evidence"`
}

type legacyEvidence struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	URL       string `json:"url"`
	Command   string `json:"command"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type agentRuntimeSnapshot struct {
	Runs []json.RawMessage `json:"runs"`
}

type legacyRuntimeRun struct {
	ID               string `json:"run_id"`
	SessionID        string `json:"session_id"`
	TaskID           string `json:"task_id"`
	Agent            string `json:"agent"`
	Prompt           string `json:"prompt"`
	ParentRunID      string `json:"parent_run_id"`
	Status           string `json:"status"`
	Response         string `json:"response"`
	Error            string `json:"error"`
	DiagnosticCode   string `json:"diagnostic_code"`
	DiagnosticReason string `json:"diagnostic_reason"`
	ResolvedAlias    string `json:"resolved_alias"`
	ResolvedKind     string `json:"resolved_kind"`
	ResolvedModel    string `json:"resolved_model"`
	CreatedAt        string `json:"created_at"`
	StartedAt        string `json:"started_at"`
	CompletedAt      string `json:"completed_at"`
	UpdatedAt        string `json:"updated_at"`
}

type parsedRuntimeRun struct {
	raw json.RawMessage
	run legacyRuntimeRun
}

func (s *Store) ImportLegacySession(ctx context.Context, input LegacySessionImportInput) (ImportResult, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return ImportResult{}, fmt.Errorf("workstore: workspace id and actor id are required for session import")
	}
	var session legacySessionDocument
	if err := json.Unmarshal(input.SessionJSON, &session); err != nil {
		return ImportResult{}, fmt.Errorf("workstore: decode legacy session: %w", err)
	}
	session.ID = strings.TrimSpace(session.ID)
	if session.ID == "" {
		return ImportResult{}, fmt.Errorf("workstore: legacy session id is required")
	}
	tasksJSON := input.TasksJSON
	if len(tasksJSON) == 0 {
		tasksJSON = []byte(`{"tasks":[]}`)
	}
	var tasks legacyTasksDocument
	if err := json.Unmarshal(tasksJSON, &tasks); err != nil {
		return ImportResult{}, fmt.Errorf("workstore: decode legacy session tasks: %w", err)
	}
	checksum := importChecksum(input.SessionJSON, tasksJSON)
	marker, found, err := s.findImportMarker(ctx, input.WorkspaceID, ImportSourceLegacySession, session.ID, checksum)
	if err != nil {
		return ImportResult{}, err
	}
	if found {
		return importResult(marker, true), nil
	}

	contractJSON := tasks.Contract
	if len(contractJSON) == 0 || string(contractJSON) == "null" {
		contractJSON = json.RawMessage(`{}`)
	}
	metadataJSON, err := json.Marshal(map[string]any{
		"import_checksum": checksum,
		"source_path":     input.SourcePath,
		"legacy_session":  json.RawMessage(input.SessionJSON),
		"legacy_tasks":    json.RawMessage(tasksJSON),
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("workstore: encode legacy session metadata: %w", err)
	}
	objective := ""
	goalStatus := ""
	if session.Goal != nil {
		objective = strings.TrimSpace(session.Goal.Description)
		goalStatus = session.Goal.Status
	}
	planStatus := ""
	if tasks.Plan != nil {
		if objective == "" {
			objective = strings.TrimSpace(tasks.Plan.Goal)
		}
		planStatus = tasks.Plan.Status
	}
	title := strings.TrimSpace(session.Title)
	if title == "" {
		title = "Imported session " + session.ID
	}
	idempotencyPrefix := "import:legacy-session:" + session.ID + ":" + checksum
	work, err := s.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    input.WorkspaceID,
		Kind:           "session",
		Source:         string(ImportSourceLegacySession),
		SourceID:       session.ID,
		IdempotencyKey: idempotencyPrefix,
		CausationID:    "import:" + checksum,
		Title:          title,
		Objective:      objective,
		ContractJSON:   contractJSON,
		MetadataJSON:   metadataJSON,
		InitialState:   legacySessionWorkState(goalStatus, planStatus),
		ActorID:        input.ActorID,
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("workstore: import legacy session work: %w", err)
	}
	for taskIndex, task := range tasks.Tasks {
		taskID := strings.TrimSpace(task.ID)
		if taskID == "" {
			taskID = fmt.Sprintf("index-%d", taskIndex)
		}
		taskTitle := strings.TrimSpace(task.Title)
		if taskTitle == "" {
			taskTitle = "Imported task " + taskID
		}
		stepKey := fmt.Sprintf("%s:task:%d:%s", idempotencyPrefix, taskIndex, taskID)
		step, err := s.CreateStep(ctx, CreateStepInput{
			WorkspaceID:    input.WorkspaceID,
			WorkID:         work.ID,
			IdempotencyKey: stepKey,
			CausationID:    "import:" + checksum,
			Title:          taskTitle,
			Description:    task.Description,
			State:          legacyTaskWorkState(task.Status),
			Position:       taskIndex + 1,
			ActorID:        input.ActorID,
		})
		if err != nil {
			return ImportResult{}, fmt.Errorf("workstore: import legacy task %q: %w", taskID, err)
		}
		for evidenceIndex, evidence := range task.Evidence {
			if err := s.importLegacyEvidence(ctx, input, checksum, stepKey, work, step, taskID, evidenceIndex, evidence); err != nil {
				return ImportResult{}, err
			}
		}
	}

	marker, existed, err := s.recordCompletedImport(ctx, completedImportInput{
		WorkspaceID: input.WorkspaceID,
		SourceKind:  ImportSourceLegacySession,
		SourceID:    session.ID,
		SourcePath:  input.SourcePath,
		Checksum:    checksum,
		WorkIDs:     []string{work.ID},
		ActorID:     input.ActorID,
	})
	if err != nil {
		return ImportResult{}, err
	}
	return importResult(marker, existed), nil
}

// GetLegacySessionTasksProjection returns the most recent imported Tasks
// document for a legacy session. The source document is preserved as JSON so
// compatibility readers can serve the existing API contract without deriving
// lossy fields from the normalized ledger records.
func (s *Store) GetLegacySessionTasksProjection(ctx context.Context, workspaceID, sessionID string) (json.RawMessage, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	sessionID = strings.TrimSpace(sessionID)
	if workspaceID == "" || sessionID == "" {
		return nil, false, fmt.Errorf("workstore: workspace id and session id are required")
	}
	work, found, err := s.findLatestSourceWork(ctx, workspaceID, ImportSourceLegacySession, sessionID)
	if err != nil || !found {
		return nil, found, err
	}
	var metadata struct {
		LegacyTasks json.RawMessage `json:"legacy_tasks"`
	}
	if err := json.Unmarshal(work.MetadataJSON, &metadata); err != nil {
		return nil, false, fmt.Errorf("workstore: decode legacy tasks projection metadata: %w", err)
	}
	if len(metadata.LegacyTasks) == 0 || string(metadata.LegacyTasks) == "null" || !json.Valid(metadata.LegacyTasks) {
		return nil, false, fmt.Errorf("workstore: legacy tasks projection is missing or invalid")
	}
	return append(json.RawMessage(nil), metadata.LegacyTasks...), true, nil
}

func (s *Store) importLegacyEvidence(
	ctx context.Context,
	input LegacySessionImportInput,
	checksum string,
	stepKey string,
	work Work,
	step Step,
	taskID string,
	evidenceIndex int,
	evidence legacyEvidence,
) error {
	evidenceID := strings.TrimSpace(evidence.ID)
	if evidenceID == "" {
		evidenceID = fmt.Sprintf("index-%d", evidenceIndex)
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("workstore: encode legacy evidence %q: %w", evidenceID, err)
	}
	kind := strings.TrimSpace(evidence.Type)
	if kind == "" {
		kind = "legacy-evidence"
	}
	name := strings.TrimSpace(evidence.Title)
	if name == "" {
		name = evidenceID
	}
	artifact, err := s.CreateArtifact(ctx, CreateArtifactInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		IdempotencyKey: fmt.Sprintf("%s:evidence:%d:%s:artifact", stepKey, evidenceIndex, evidenceID),
		CausationID:    "import:" + checksum,
		Kind:           kind,
		Name:           name,
		URI:            legacyEvidenceURI(work.SourceID, taskID, evidenceID, evidence),
		Digest:         "sha256:" + digestBytes(raw),
		MediaType:      "application/json",
		SizeBytes:      int64(len(raw)),
		ActorID:        input.ActorID,
	})
	if err != nil {
		return fmt.Errorf("workstore: import legacy evidence artifact %q: %w", evidenceID, err)
	}
	summary := strings.TrimSpace(evidence.Summary)
	if summary == "" {
		summary = strings.TrimSpace(evidence.Title)
	}
	if summary == "" {
		summary = "Imported legacy evidence " + evidenceID
	}
	if _, err := s.CreateProof(ctx, CreateProofInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		IdempotencyKey: fmt.Sprintf("%s:evidence:%d:%s:proof", stepKey, evidenceIndex, evidenceID),
		CausationID:    "import:" + checksum,
		Kind:           kind,
		Status:         legacyProofStatus(evidence.Status),
		Summary:        summary,
		Command:        evidence.Command,
		ArtifactID:     artifact.ID,
		ActorID:        input.ActorID,
	}); err != nil {
		return fmt.Errorf("workstore: import legacy evidence proof %q: %w", evidenceID, err)
	}
	return nil
}

func (s *Store) ImportAgentRuntimeSnapshot(ctx context.Context, input AgentRuntimeImportInput) (ImportResult, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return ImportResult{}, fmt.Errorf("workstore: workspace id, source id, and actor id are required for runtime import")
	}
	var snapshot agentRuntimeSnapshot
	if err := json.Unmarshal(input.SnapshotJSON, &snapshot); err != nil {
		return ImportResult{}, fmt.Errorf("workstore: decode agent runtime snapshot: %w", err)
	}
	parsedRuns, err := parseRuntimeRuns(snapshot.Runs)
	if err != nil {
		return ImportResult{}, err
	}
	checksum := importChecksum(input.SnapshotJSON)
	marker, found, err := s.findImportMarker(ctx, input.WorkspaceID, ImportSourceAgentRuntime, input.SourceID, checksum)
	if err != nil {
		return ImportResult{}, err
	}
	if found {
		return importResult(marker, true), nil
	}

	runsByID := make(map[string]parsedRuntimeRun, len(parsedRuns))
	for _, parsed := range parsedRuns {
		runsByID[parsed.run.ID] = parsed
	}
	workByRunID := make(map[string]Work, len(parsedRuns))
	workIDs := make([]string, 0, len(parsedRuns))
	pending := append([]parsedRuntimeRun(nil), parsedRuns...)
	for len(pending) > 0 {
		remaining := pending[:0]
		progressed := false
		for _, parsed := range pending {
			parentID := strings.TrimSpace(parsed.run.ParentRunID)
			if _, belongsToSnapshot := runsByID[parentID]; parentID != "" && belongsToSnapshot {
				if _, imported := workByRunID[parentID]; !imported {
					remaining = append(remaining, parsed)
					continue
				}
			}
			parentWorkID := ""
			if parentID != "" {
				if parent, ok := workByRunID[parentID]; ok {
					parentWorkID = parent.ID
				} else if parent, found, err := s.findLatestSourceWork(ctx, input.WorkspaceID, ImportSourceAgentRuntime, parentID); err != nil {
					return ImportResult{}, err
				} else if found {
					parentWorkID = parent.ID
				}
			}
			work, err := s.importRuntimeRun(ctx, input, checksum, parentWorkID, parsed)
			if err != nil {
				return ImportResult{}, err
			}
			workByRunID[parsed.run.ID] = work
			workIDs = append(workIDs, work.ID)
			progressed = true
		}
		if !progressed {
			return ImportResult{}, fmt.Errorf("workstore: agent runtime snapshot contains a parent cycle")
		}
		pending = append([]parsedRuntimeRun(nil), remaining...)
	}

	marker, existed, err := s.recordCompletedImport(ctx, completedImportInput{
		WorkspaceID: input.WorkspaceID,
		SourceKind:  ImportSourceAgentRuntime,
		SourceID:    input.SourceID,
		SourcePath:  input.SourcePath,
		Checksum:    checksum,
		WorkIDs:     workIDs,
		ActorID:     input.ActorID,
	})
	if err != nil {
		return ImportResult{}, err
	}
	return importResult(marker, existed), nil
}

func (s *Store) importRuntimeRun(ctx context.Context, input AgentRuntimeImportInput, checksum, parentWorkID string, parsed parsedRuntimeRun) (Work, error) {
	run := parsed.run
	metadataJSON, err := json.Marshal(map[string]any{
		"import_checksum": checksum,
		"source_path":     input.SourcePath,
		"legacy_run":      parsed.raw,
		"legacy_snapshot": json.RawMessage(input.SnapshotJSON),
	})
	if err != nil {
		return Work{}, fmt.Errorf("workstore: encode runtime run %q metadata: %w", run.ID, err)
	}
	prefix := "import:agentruntime:" + input.SourceID + ":" + checksum + ":run:" + run.ID
	work, err := s.CreateWork(ctx, CreateWorkInput{
		WorkspaceID:    input.WorkspaceID,
		Kind:           "agent-run",
		Source:         string(ImportSourceAgentRuntime),
		SourceID:       run.ID,
		IdempotencyKey: prefix,
		CausationID:    "import:" + checksum,
		ParentWorkID:   parentWorkID,
		Title:          "Agent run " + run.ID,
		Objective:      run.Prompt,
		MetadataJSON:   metadataJSON,
		InitialState:   runtimeWorkState(run.Status),
		ActorID:        input.ActorID,
	})
	if err != nil {
		return Work{}, fmt.Errorf("workstore: import runtime work %q: %w", run.ID, err)
	}
	agent := strings.TrimSpace(run.Agent)
	if agent == "" {
		agent = "legacy-agent"
	}
	step, err := s.CreateStep(ctx, CreateStepInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         work.ID,
		IdempotencyKey: prefix + ":step",
		CausationID:    "import:" + checksum,
		Title:          "Execute " + agent,
		Description:    run.Prompt,
		State:          runtimeWorkState(run.Status),
		Position:       1,
		ActorID:        input.ActorID,
	})
	if err != nil {
		return Work{}, fmt.Errorf("workstore: import runtime step %q: %w", run.ID, err)
	}
	outputJSON, err := json.Marshal(map[string]string{
		"response":          run.Response,
		"diagnostic_code":   run.DiagnosticCode,
		"diagnostic_reason": run.DiagnosticReason,
	})
	if err != nil {
		return Work{}, fmt.Errorf("workstore: encode runtime output %q: %w", run.ID, err)
	}
	adapter := firstImportValue(run.ResolvedKind, run.ResolvedAlias, run.Agent, "legacy")
	if _, err := s.CreateAttempt(ctx, CreateAttemptInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         work.ID,
		StepID:         step.ID,
		IdempotencyKey: prefix + ":attempt",
		CausationID:    "import:" + checksum,
		Number:         1,
		Adapter:        adapter,
		Status:         runtimeAttemptStatus(run.Status),
		ActorID:        input.ActorID,
		InputJSON:      parsed.raw,
		OutputJSON:     outputJSON,
		ErrorText:      run.Error,
		StartedAt:      parseImportTime(run.StartedAt),
		FinishedAt:     parseImportTime(run.CompletedAt),
	}); err != nil {
		return Work{}, fmt.Errorf("workstore: import runtime attempt %q: %w", run.ID, err)
	}
	return work, nil
}

func parseRuntimeRuns(rawRuns []json.RawMessage) ([]parsedRuntimeRun, error) {
	parsed := make([]parsedRuntimeRun, 0, len(rawRuns))
	seen := make(map[string]struct{}, len(rawRuns))
	for index, raw := range rawRuns {
		var run legacyRuntimeRun
		if err := json.Unmarshal(raw, &run); err != nil {
			return nil, fmt.Errorf("workstore: decode agent runtime run %d: %w", index, err)
		}
		run.ID = strings.TrimSpace(run.ID)
		if run.ID == "" {
			return nil, fmt.Errorf("workstore: agent runtime run %d has no id", index)
		}
		if _, exists := seen[run.ID]; exists {
			return nil, fmt.Errorf("workstore: duplicate agent runtime run id %q", run.ID)
		}
		seen[run.ID] = struct{}{}
		parsed = append(parsed, parsedRuntimeRun{raw: append(json.RawMessage(nil), raw...), run: run})
	}
	if err := validateRuntimeParentGraph(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateRuntimeParentGraph(runs []parsedRuntimeRun) error {
	parents := make(map[string]string, len(runs))
	for _, parsed := range runs {
		parents[parsed.run.ID] = strings.TrimSpace(parsed.run.ParentRunID)
	}
	states := make(map[string]uint8, len(runs))
	var visit func(string) error
	visit = func(runID string) error {
		switch states[runID] {
		case 1:
			return fmt.Errorf("workstore: agent runtime snapshot contains a parent cycle at %q", runID)
		case 2:
			return nil
		}
		states[runID] = 1
		if parentID := parents[runID]; parentID != "" {
			if _, belongsToSnapshot := parents[parentID]; belongsToSnapshot {
				if err := visit(parentID); err != nil {
					return err
				}
			}
		}
		states[runID] = 2
		return nil
	}
	for runID := range parents {
		if err := visit(runID); err != nil {
			return err
		}
	}
	return nil
}

type completedImportInput struct {
	WorkspaceID string
	SourceKind  ImportSourceKind
	SourceID    string
	SourcePath  string
	Checksum    string
	WorkIDs     []string
	ActorID     string
}

func (s *Store) recordCompletedImport(ctx context.Context, input completedImportInput) (ImportMarker, bool, error) {
	return s.recordImportMarker(ctx, importMarkerInput{
		WorkspaceID: input.WorkspaceID,
		SourceKind:  input.SourceKind,
		SourceID:    input.SourceID,
		SourcePath:  input.SourcePath,
		Checksum:    input.Checksum,
		Status:      ImportStatusCompleted,
		WorkIDs:     input.WorkIDs,
		ActorID:     input.ActorID,
	})
}

type importMarkerInput struct {
	WorkspaceID string
	SourceKind  ImportSourceKind
	SourceID    string
	SourcePath  string
	Checksum    string
	Status      ImportStatus
	WorkIDs     []string
	ActorID     string
	ErrorText   string
	ImportedAt  *time.Time
}

func (s *Store) recordImportMarker(ctx context.Context, input importMarkerInput) (ImportMarker, bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	id, err := s.newID("imp")
	if err != nil {
		return ImportMarker{}, false, err
	}
	workIDsJSON, err := json.Marshal(input.WorkIDs)
	if err != nil {
		return ImportMarker{}, false, fmt.Errorf("workstore: encode imported work ids: %w", err)
	}
	now := s.now().UTC()
	if input.ImportedAt != nil {
		now = input.ImportedAt.UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportMarker{}, false, fmt.Errorf("workstore: begin import marker: %w", err)
	}
	defer rollback(tx)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO import_markers (
			schema_version, id, workspace_id, source_kind, source_id, source_path,
			checksum, status, work_ids_json, actor_id, error_text, imported_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, source_kind, source_id, checksum) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.SourceKind, input.SourceID,
		input.SourcePath, input.Checksum, input.Status, workIDsJSON, input.ActorID,
		input.ErrorText, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return ImportMarker{}, false, fmt.Errorf("workstore: insert import marker: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return ImportMarker{}, false, fmt.Errorf("workstore: inspect import marker insert: %w", err)
	}
	marker, err := getImportMarkerTx(ctx, tx, input.WorkspaceID, input.SourceKind, input.SourceID, input.Checksum)
	if err != nil {
		return ImportMarker{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ImportMarker{}, false, fmt.Errorf("workstore: commit import marker: %w", err)
	}
	return marker, inserted == 0, nil
}

func (s *Store) findImportMarker(ctx context.Context, workspaceID string, sourceKind ImportSourceKind, sourceID, checksum string) (ImportMarker, bool, error) {
	return s.findImportMarkerStatus(ctx, workspaceID, sourceKind, sourceID, checksum, ImportStatusCompleted)
}

func (s *Store) findImportMarkerStatus(ctx context.Context, workspaceID string, sourceKind ImportSourceKind, sourceID, checksum string, status ImportStatus) (ImportMarker, bool, error) {
	marker, err := scanImportMarker(s.db.QueryRowContext(ctx, importMarkerSelect+`
		WHERE workspace_id = ? AND source_kind = ? AND source_id = ? AND checksum = ? AND status = ?
	`, workspaceID, sourceKind, sourceID, checksum, status))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportMarker{}, false, nil
	}
	if err != nil {
		return ImportMarker{}, false, fmt.Errorf("workstore: find import marker: %w", err)
	}
	return marker, true, nil
}

func (s *Store) ListImportMarkers(ctx context.Context, workspaceID string) ([]ImportMarker, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workstore: workspace id is required")
	}
	rows, err := s.db.QueryContext(ctx, importMarkerSelect+`
		WHERE workspace_id = ? ORDER BY imported_at, id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workstore: list import markers: %w", err)
	}
	defer closeRows(rows)
	var markers []ImportMarker
	for rows.Next() {
		marker, err := scanImportMarker(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan import marker: %w", err)
		}
		markers = append(markers, marker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate import markers: %w", err)
	}
	return markers, nil
}

const importMarkerSelect = `SELECT
    schema_version, id, workspace_id, source_kind, source_id, source_path,
    checksum, status, work_ids_json, actor_id, error_text, imported_at, updated_at
FROM import_markers`

func scanImportMarker(row scanner) (ImportMarker, error) {
	var marker ImportMarker
	var workIDsJSON jsonValue
	var importedAt, updatedAt int64
	if err := row.Scan(&marker.SchemaVersion, &marker.ID, &marker.WorkspaceID,
		&marker.SourceKind, &marker.SourceID, &marker.SourcePath, &marker.Checksum,
		&marker.Status, &workIDsJSON, &marker.ActorID, &marker.ErrorText,
		&importedAt, &updatedAt); err != nil {
		return ImportMarker{}, err
	}
	if err := json.Unmarshal(workIDsJSON, &marker.WorkIDs); err != nil {
		return ImportMarker{}, fmt.Errorf("workstore: decode import marker work ids: %w", err)
	}
	marker.ImportedAt = time.UnixMilli(importedAt).UTC()
	marker.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return marker, nil
}

func getImportMarkerTx(ctx context.Context, tx *sql.Tx, workspaceID string, sourceKind ImportSourceKind, sourceID, checksum string) (ImportMarker, error) {
	marker, err := scanImportMarker(tx.QueryRowContext(ctx, importMarkerSelect+`
		WHERE workspace_id = ? AND source_kind = ? AND source_id = ? AND checksum = ?
	`, workspaceID, sourceKind, sourceID, checksum))
	if errors.Is(err, sql.ErrNoRows) {
		return ImportMarker{}, ErrNotFound
	}
	if err != nil {
		return ImportMarker{}, fmt.Errorf("workstore: query import marker: %w", err)
	}
	return marker, nil
}

func (s *Store) findLatestSourceWork(ctx context.Context, workspaceID string, sourceKind ImportSourceKind, sourceID string) (Work, bool, error) {
	work, err := scanWork(s.db.QueryRowContext(ctx, workSelect+`
		WHERE workspace_id = ? AND source = ? AND source_id = ?
		ORDER BY created_at DESC, rowid DESC LIMIT 1
	`, workspaceID, sourceKind, sourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, false, nil
	}
	if err != nil {
		return Work{}, false, fmt.Errorf("workstore: find source work: %w", err)
	}
	return work, true, nil
}

func importResult(marker ImportMarker, alreadyImported bool) ImportResult {
	return ImportResult{
		Marker:          marker,
		WorkIDs:         append([]string(nil), marker.WorkIDs...),
		AlreadyImported: alreadyImported,
	}
}

func importChecksum(parts ...[]byte) string {
	hash := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func legacySessionWorkState(goalStatus, planStatus string) WorkState {
	switch strings.ToLower(strings.TrimSpace(goalStatus)) {
	case "satisfied":
		return WorkStateDone
	case "exhausted":
		return WorkStateCancelled
	}
	switch strings.ToLower(strings.TrimSpace(planStatus)) {
	case "completed":
		return WorkStateDone
	case "aborted":
		return WorkStateCancelled
	case "paused":
		return WorkStateBlocked
	case "executing":
		return WorkStateRunning
	case "drafting", "proposed":
		return WorkStateTodo
	default:
		return WorkStateBacklog
	}
}

func legacyTaskWorkState(status string) WorkState {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return WorkStateDone
	case "cancelled", "canceled":
		return WorkStateCancelled
	case "in_progress", "running":
		return WorkStateRunning
	default:
		return WorkStateTodo
	}
}

func legacyProofStatus(status string) ProofStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "passed", "pass", "success", "succeeded", "completed", "ok":
		return ProofStatusPassed
	case "failed", "failure", "error":
		return ProofStatusFailed
	default:
		return ProofStatusInconclusive
	}
}

func runtimeWorkState(status string) WorkState {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted":
		return WorkStateReady
	case "running":
		return WorkStateRunning
	case "completed", "succeeded":
		return WorkStateDone
	case "failed":
		return WorkStateBlocked
	case "cancelled", "canceled":
		return WorkStateCancelled
	default:
		return WorkStateTodo
	}
}

func runtimeAttemptStatus(status string) AttemptStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted", "pending":
		return AttemptStatusPending
	case "running":
		return AttemptStatusRunning
	case "completed", "succeeded":
		return AttemptStatusSucceeded
	case "failed":
		return AttemptStatusFailed
	case "cancelled", "canceled":
		return AttemptStatusCancelled
	default:
		return AttemptStatusPending
	}
}

func legacyEvidenceURI(sessionID, taskID, evidenceID string, evidence legacyEvidence) string {
	if value := strings.TrimSpace(evidence.URL); value != "" {
		return value
	}
	if value := strings.TrimSpace(evidence.Path); value != "" {
		return "file:" + value
	}
	return "legacy://session/" + url.PathEscape(sessionID) + "/task/" + url.PathEscape(taskID) + "/evidence/" + url.PathEscape(evidenceID)
}

func parseImportTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func firstImportValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
