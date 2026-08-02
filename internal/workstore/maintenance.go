package workstore

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrDestinationExists = errors.New("workstore: destination already exists")

type BackupReport struct {
	Path          string    `json:"path"`
	Digest        string    `json:"digest"`
	SizeBytes     int64     `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`
	SchemaVersion int       `json:"schema_version"`
}

type RestoreReport struct {
	SourcePath string    `json:"source_path"`
	TargetPath string    `json:"target_path"`
	Digest     string    `json:"digest"`
	SizeBytes  int64     `json:"size_bytes"`
	RestoredAt time.Time `json:"restored_at"`
}

type ExportReport struct {
	Path         string         `json:"path"`
	Digest       string         `json:"digest"`
	SizeBytes    int64          `json:"size_bytes"`
	RecordCount  int            `json:"record_count"`
	RecordCounts map[string]int `json:"record_counts"`
	ExportedAt   time.Time      `json:"exported_at"`
}

type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type DoctorIssue struct {
	Code       string `json:"code"`
	RecordType string `json:"record_type,omitempty"`
	RecordID   string `json:"record_id,omitempty"`
	Field      string `json:"field,omitempty"`
	Detail     string `json:"detail"`
}

type DoctorReport struct {
	WorkspaceID   string        `json:"workspace_id"`
	Healthy       bool          `json:"healthy"`
	SchemaVersion int           `json:"schema_version"`
	CheckedAt     time.Time     `json:"checked_at"`
	Checks        []DoctorCheck `json:"checks"`
	Issues        []DoctorIssue `json:"issues"`
}

type QuarantineInput struct {
	WorkspaceID   string
	SourceKind    ImportSourceKind
	SourceID      string
	SourcePath    string
	QuarantineDir string
	Reason        string
	ActorID       string
}

type QuarantineRecord struct {
	WorkspaceID        string           `json:"workspace_id"`
	SourceKind         ImportSourceKind `json:"source_kind"`
	SourceID           string           `json:"source_id"`
	SourcePath         string           `json:"source_path"`
	QuarantinePath     string           `json:"quarantine_path"`
	ManifestPath       string           `json:"manifest_path"`
	Digest             string           `json:"digest"`
	SizeBytes          int64            `json:"size_bytes"`
	Reason             string           `json:"reason"`
	ActorID            string           `json:"actor_id"`
	QuarantinedAt      time.Time        `json:"quarantined_at"`
	AlreadyQuarantined bool             `json:"already_quarantined"`
	Marker             ImportMarker     `json:"marker"`
}

type jsonlHeader struct {
	Type          string    `json:"type"`
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	ExportedAt    time.Time `json:"exported_at"`
}

type jsonlRecord struct {
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	Record        any    `json:"record"`
}

type jsonlChecksum struct {
	Type         string         `json:"type"`
	Algorithm    string         `json:"algorithm"`
	Digest       string         `json:"digest"`
	RecordCount  int            `json:"record_count"`
	RecordCounts map[string]int `json:"record_counts"`
}

type jsonlExporter struct {
	writer       *bufio.Writer
	digest       hash.Hash
	recordCount  int
	recordCounts map[string]int
}

func newJSONLExporter(writer *bufio.Writer) *jsonlExporter {
	return &jsonlExporter{
		writer: writer, digest: sha256.New(), recordCounts: make(map[string]int),
	}
}

func (exporter *jsonlExporter) writeHeader(workspaceID string, exportedAt time.Time) error {
	return writeHashedJSONLine(exporter.writer, exporter.digest, jsonlHeader{
		Type: "header", SchemaVersion: schemaVersion,
		WorkspaceID: workspaceID, ExportedAt: exportedAt,
	})
}

func (exporter *jsonlExporter) writeRecord(recordType string, record any) error {
	if err := writeHashedJSONLine(exporter.writer, exporter.digest, jsonlRecord{
		Type: recordType, SchemaVersion: recordSchemaVersion, Record: record,
	}); err != nil {
		return err
	}
	exporter.recordCounts[recordType]++
	exporter.recordCount++
	return nil
}

func (exporter *jsonlExporter) writeProjection(projection WorkProjection) error {
	if err := exporter.writeRecord("work", projection.Work); err != nil {
		return err
	}
	writers := []func() error{
		func() error { return writeExportRecords(exporter, "step", projection.Steps) },
		func() error { return writeExportRecords(exporter, "schedule", projection.Schedules) },
		func() error { return writeExportRecords(exporter, "dependency", projection.Dependencies) },
		func() error { return writeExportRecords(exporter, "attempt", projection.Attempts) },
		func() error { return writeExportRecords(exporter, "event", projection.Events) },
		func() error { return writeExportRecords(exporter, "approval", projection.Approvals) },
		func() error { return writeExportRecords(exporter, "artifact", projection.Artifacts) },
		func() error { return writeExportRecords(exporter, "proof", projection.Proofs) },
		func() error { return writeExportRecords(exporter, "effect_receipt", projection.EffectReceipts) },
	}
	for _, write := range writers {
		if err := write(); err != nil {
			return err
		}
	}
	return nil
}

func writeExportRecords[T any](exporter *jsonlExporter, recordType string, records []T) error {
	for _, record := range records {
		if err := exporter.writeRecord(recordType, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Backup(ctx context.Context, destination string) (BackupReport, error) {
	if strings.TrimSpace(destination) == "" {
		return BackupReport{}, fmt.Errorf("workstore: backup destination is required")
	}
	if err := requireMissingDestination(destination); err != nil {
		return BackupReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return BackupReport{}, fmt.Errorf("workstore: create backup directory: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	temporary, err := vacantTemporaryPath(filepath.Dir(destination), ".workstore-backup-*")
	if err != nil {
		return BackupReport{}, err
	}
	defer removeFile(temporary)
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", temporary); err != nil {
		return BackupReport{}, fmt.Errorf("workstore: create SQLite backup: %w", err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return BackupReport{}, fmt.Errorf("workstore: secure backup permissions: %w", err)
	}
	if err := validateSQLiteFile(ctx, temporary); err != nil {
		return BackupReport{}, fmt.Errorf("workstore: validate backup: %w", err)
	}
	digest, size, err := fileDigest(temporary)
	if err != nil {
		return BackupReport{}, err
	}
	if err := publishTemporaryFile(temporary, destination); err != nil {
		return BackupReport{}, fmt.Errorf("workstore: publish backup: %w", err)
	}
	return BackupReport{
		Path: destination, Digest: digest, SizeBytes: size,
		CreatedAt: s.now().UTC(), SchemaVersion: schemaVersion,
	}, nil
}

func RestoreBackup(ctx context.Context, source, target string, opts Options) (RestoreReport, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
		return RestoreReport{}, fmt.Errorf("workstore: restore source and target are required")
	}
	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: resolve restore source: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: resolve restore target: %w", err)
	}
	if filepath.Clean(sourceAbs) == filepath.Clean(targetAbs) {
		return RestoreReport{}, fmt.Errorf("workstore: restore source and target must differ")
	}
	if err := requireMissingDestination(target); err != nil {
		return RestoreReport{}, err
	}
	if err := validateSQLiteFile(ctx, source); err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: invalid backup: %w", err)
	}
	digest, size, err := fileDigest(source)
	if err != nil {
		return RestoreReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: create restore directory: %w", err)
	}
	temporary, err := copyToTemporaryFile(source, filepath.Dir(target), ".workstore-restore-*")
	if err != nil {
		return RestoreReport{}, err
	}
	defer removeFile(temporary)
	if err := publishTemporaryFile(temporary, target); err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: publish restored database: %w", err)
	}
	restoredStore, err := Open(ctx, target, opts)
	if err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: open restored database: %w", err)
	}
	if err := restoredStore.Close(); err != nil {
		return RestoreReport{}, fmt.Errorf("workstore: close restored database: %w", err)
	}
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	return RestoreReport{
		SourcePath: source, TargetPath: target, Digest: digest,
		SizeBytes: size, RestoredAt: now,
	}, nil
}

func (s *Store) ExportJSONL(ctx context.Context, workspaceID, destination string) (ExportReport, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(destination) == "" {
		return ExportReport{}, fmt.Errorf("workstore: workspace id and export destination are required")
	}
	if err := requireMissingDestination(destination); err != nil {
		return ExportReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return ExportReport{}, fmt.Errorf("workstore: create export directory: %w", err)
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	temporaryFile, err := os.CreateTemp(filepath.Dir(destination), ".workstore-export-*")
	if err != nil {
		return ExportReport{}, fmt.Errorf("workstore: create export temporary file: %w", err)
	}
	temporary := temporaryFile.Name()
	defer removeFile(temporary)
	if err := temporaryFile.Chmod(0o600); err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, fmt.Errorf("workstore: secure export file: %w", err)
	}
	writer := bufio.NewWriterSize(temporaryFile, 64*1024)
	exporter := newJSONLExporter(writer)
	exportedAt := s.now().UTC()
	if err := exporter.writeHeader(workspaceID, exportedAt); err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, err
	}
	works, err := s.listAllWorks(ctx, workspaceID)
	if err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, err
	}
	for _, work := range works {
		projection, err := s.GetWorkProjection(ctx, workspaceID, work.ID)
		if err != nil {
			_ = temporaryFile.Close()
			return ExportReport{}, err
		}
		if err := exporter.writeProjection(projection); err != nil {
			_ = temporaryFile.Close()
			return ExportReport{}, err
		}
	}
	markers, err := s.ListImportMarkers(ctx, workspaceID)
	if err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, err
	}
	for _, marker := range markers {
		if err := exporter.writeRecord("import_marker", marker); err != nil {
			_ = temporaryFile.Close()
			return ExportReport{}, err
		}
	}
	digest := hex.EncodeToString(exporter.digest.Sum(nil))
	if err := writeJSONLine(writer, jsonlChecksum{
		Type: "checksum", Algorithm: "sha256", Digest: digest,
		RecordCount: exporter.recordCount, RecordCounts: exporter.recordCounts,
	}); err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, err
	}
	if err := writer.Flush(); err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, fmt.Errorf("workstore: flush JSONL export: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		_ = temporaryFile.Close()
		return ExportReport{}, fmt.Errorf("workstore: sync JSONL export: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return ExportReport{}, fmt.Errorf("workstore: close JSONL export: %w", err)
	}
	info, err := os.Stat(temporary)
	if err != nil {
		return ExportReport{}, fmt.Errorf("workstore: stat JSONL export: %w", err)
	}
	if err := publishTemporaryFile(temporary, destination); err != nil {
		return ExportReport{}, fmt.Errorf("workstore: publish JSONL export: %w", err)
	}
	return ExportReport{
		Path: destination, Digest: digest, SizeBytes: info.Size(),
		RecordCount: exporter.recordCount, RecordCounts: exporter.recordCounts, ExportedAt: exportedAt,
	}, nil
}

func (s *Store) Doctor(ctx context.Context, workspaceID string) (DoctorReport, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return DoctorReport{}, fmt.Errorf("workstore: workspace id is required")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	report := DoctorReport{
		WorkspaceID: workspaceID, SchemaVersion: schemaVersion,
		CheckedAt: s.now().UTC(), Healthy: true,
		Checks: make([]DoctorCheck, 0, 9), Issues: make([]DoctorIssue, 0),
	}
	addCheck := func(name string, err error) {
		check := DoctorCheck{Name: name, OK: err == nil}
		if err != nil {
			check.Detail = err.Error()
			report.Healthy = false
		}
		report.Checks = append(report.Checks, check)
	}

	addCheck("quick_check", quickCheckDB(ctx, s.db))
	addCheck("foreign_keys", foreignKeyCheckDB(ctx, s.db))
	addCheck("migrations", verifyMigrationsDB(ctx, s.db, true))
	jsonIssues, err := s.doctorJSONIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, jsonIssues...)
	addCheck("json", err)
	invariantIssues, err := s.doctorTerminalIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, invariantIssues...)
	addCheck("terminal_timestamps", err)
	dependencyIssues, err := s.doctorDependencyIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, dependencyIssues...)
	addCheck("dependency_graph", err)
	scheduleIssues, err := s.doctorScheduleIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, scheduleIssues...)
	addCheck("step_schedules", err)
	effectIssues, err := s.doctorEffectReceiptIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, effectIssues...)
	addCheck("effect_receipts", err)
	proofIssues, err := s.doctorProofIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, proofIssues...)
	addCheck("proofs", err)
	importIssues, err := s.doctorImportIssues(ctx, workspaceID)
	report.Issues = append(report.Issues, importIssues...)
	addCheck("import_references", err)
	if len(report.Issues) > 0 {
		report.Healthy = false
	}
	return report, nil
}

func (s *Store) QuarantineSource(ctx context.Context, input QuarantineInput) (QuarantineRecord, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(string(input.SourceKind)) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.SourcePath) == "" || strings.TrimSpace(input.QuarantineDir) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.ActorID) == "" {
		return QuarantineRecord{}, fmt.Errorf("workstore: incomplete quarantine input")
	}
	raw, err := os.ReadFile(input.SourcePath)
	if err != nil {
		return QuarantineRecord{}, fmt.Errorf("workstore: read quarantine source: %w", err)
	}
	digest := digestBytes(raw)
	marker, found, err := s.findImportMarkerStatus(ctx, input.WorkspaceID, input.SourceKind, input.SourceID, digest, ImportStatusQuarantined)
	if err != nil {
		return QuarantineRecord{}, err
	}
	quarantinedAt := time.UnixMilli(s.now().UTC().UnixMilli()).UTC()
	canonicalSourcePath := input.SourcePath
	if found {
		quarantinedAt = marker.ImportedAt
		if strings.TrimSpace(marker.SourcePath) != "" {
			canonicalSourcePath = marker.SourcePath
		}
	}
	copyPath, manifestPath := quarantinePaths(input.QuarantineDir, canonicalSourcePath, digest, quarantinedAt)
	if err := os.MkdirAll(input.QuarantineDir, 0o700); err != nil {
		return QuarantineRecord{}, fmt.Errorf("workstore: create quarantine directory: %w", err)
	}
	if err := writeExactFile(copyPath, raw, 0o600); err != nil {
		return QuarantineRecord{}, fmt.Errorf("workstore: copy quarantine source: %w", err)
	}
	if !found {
		var existed bool
		marker, existed, err = s.recordImportMarker(ctx, importMarkerInput{
			WorkspaceID: input.WorkspaceID,
			SourceKind:  input.SourceKind,
			SourceID:    input.SourceID,
			SourcePath:  input.SourcePath,
			Checksum:    digest,
			Status:      ImportStatusQuarantined,
			ActorID:     input.ActorID,
			ErrorText:   input.Reason,
			ImportedAt:  &quarantinedAt,
		})
		if err != nil {
			return QuarantineRecord{}, err
		}
		if existed {
			if marker.Status != ImportStatusQuarantined {
				return QuarantineRecord{}, fmt.Errorf("workstore: source checksum already has %s import marker", marker.Status)
			}
			found = true
			quarantinedAt = marker.ImportedAt
			if strings.TrimSpace(marker.SourcePath) != "" {
				canonicalSourcePath = marker.SourcePath
			}
			canonicalCopy, canonicalManifest := quarantinePaths(input.QuarantineDir, canonicalSourcePath, digest, quarantinedAt)
			if canonicalCopy != copyPath {
				removeFile(copyPath)
				copyPath, manifestPath = canonicalCopy, canonicalManifest
				if err := writeExactFile(copyPath, raw, 0o600); err != nil {
					return QuarantineRecord{}, fmt.Errorf("workstore: restore canonical quarantine copy: %w", err)
				}
			}
		}
	}
	recordReason := input.Reason
	recordActorID := input.ActorID
	if found {
		if strings.TrimSpace(marker.ErrorText) != "" {
			recordReason = marker.ErrorText
		}
		if strings.TrimSpace(marker.ActorID) != "" {
			recordActorID = marker.ActorID
		}
	}
	record := QuarantineRecord{
		WorkspaceID: input.WorkspaceID, SourceKind: input.SourceKind,
		SourceID: input.SourceID, SourcePath: canonicalSourcePath,
		QuarantinePath: copyPath, ManifestPath: manifestPath, Digest: digest,
		SizeBytes: int64(len(raw)), Reason: recordReason, ActorID: recordActorID,
		QuarantinedAt: quarantinedAt, AlreadyQuarantined: found, Marker: marker,
	}
	manifestRecord := record
	manifestRecord.AlreadyQuarantined = false
	manifestJSON, err := json.MarshalIndent(manifestRecord, "", "  ")
	if err != nil {
		return QuarantineRecord{}, fmt.Errorf("workstore: encode quarantine manifest: %w", err)
	}
	if err := writeExactFile(manifestPath, append(manifestJSON, '\n'), 0o600); err != nil {
		return QuarantineRecord{}, fmt.Errorf("workstore: write quarantine manifest: %w", err)
	}
	return record, nil
}

func (s *Store) listAllWorks(ctx context.Context, workspaceID string) ([]Work, error) {
	rows, err := s.db.QueryContext(ctx, workSelect+" WHERE workspace_id = ? ORDER BY id", workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workstore: list all works: %w", err)
	}
	defer closeRows(rows)
	var works []Work
	for rows.Next() {
		work, err := scanWork(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan export work: %w", err)
		}
		works = append(works, work)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate export works: %w", err)
	}
	return works, nil
}

func (s *Store) doctorJSONIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	checks := []struct {
		recordType string
		field      string
		query      string
	}{
		{"work", "contract_json", "SELECT id, contract_json FROM works WHERE workspace_id = ?"},
		{"work", "metadata_json", "SELECT id, metadata_json FROM works WHERE workspace_id = ?"},
		{"attempt", "input_json", "SELECT id, input_json FROM attempts WHERE workspace_id = ?"},
		{"attempt", "output_json", "SELECT id, output_json FROM attempts WHERE workspace_id = ?"},
		{"event", "payload_json", "SELECT id, payload_json FROM events WHERE workspace_id = ?"},
		{"step_schedule", "policy_json", "SELECT step_id, policy_json FROM step_schedules WHERE workspace_id = ?"},
		{"effect_receipt", "outcome_json", "SELECT id, outcome_json FROM effect_receipts WHERE workspace_id = ?"},
		{"proof", "environment_json", "SELECT id, environment_json FROM proofs WHERE workspace_id = ?"},
		{"proof", "input_json", "SELECT id, input_json FROM proofs WHERE workspace_id = ?"},
		{"proof", "artifact_digests_json", "SELECT id, artifact_digests_json FROM proofs WHERE workspace_id = ?"},
		{"import_marker", "work_ids_json", "SELECT id, work_ids_json FROM import_markers WHERE workspace_id = ?"},
	}
	var issues []DoctorIssue
	for _, check := range checks {
		rows, err := s.db.QueryContext(ctx, check.query, workspaceID)
		if err != nil {
			return issues, fmt.Errorf("workstore: doctor query %s.%s: %w", check.recordType, check.field, err)
		}
		for rows.Next() {
			var recordID string
			var raw jsonValue
			if err := rows.Scan(&recordID, &raw); err != nil {
				closeRows(rows)
				return issues, fmt.Errorf("workstore: doctor scan %s.%s: %w", check.recordType, check.field, err)
			}
			if !json.Valid(raw) {
				issues = append(issues, DoctorIssue{
					Code: "invalid_json", RecordType: check.recordType,
					RecordID: recordID, Field: check.field, Detail: "stored value is not valid JSON",
				})
			}
		}
		if err := rows.Err(); err != nil {
			closeRows(rows)
			return issues, fmt.Errorf("workstore: doctor iterate %s.%s: %w", check.recordType, check.field, err)
		}
		closeRows(rows)
	}
	return issues, nil
}

func (s *Store) doctorTerminalIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	queries := []struct {
		recordType string
		query      string
	}{
		{"work", "SELECT id FROM works WHERE workspace_id = ? AND state IN ('done','cancelled') AND completed_at IS NULL"},
		{"step", "SELECT id FROM steps WHERE workspace_id = ? AND state IN ('done','cancelled') AND completed_at IS NULL"},
		{"attempt", "SELECT id FROM attempts WHERE workspace_id = ? AND status IN ('succeeded','failed','cancelled') AND finished_at IS NULL"},
	}
	var issues []DoctorIssue
	for _, check := range queries {
		rows, err := s.db.QueryContext(ctx, check.query, workspaceID)
		if err != nil {
			return issues, fmt.Errorf("workstore: doctor terminal query %s: %w", check.recordType, err)
		}
		for rows.Next() {
			var recordID string
			if err := rows.Scan(&recordID); err != nil {
				closeRows(rows)
				return issues, fmt.Errorf("workstore: doctor terminal scan %s: %w", check.recordType, err)
			}
			issues = append(issues, DoctorIssue{
				Code: "missing_terminal_timestamp", RecordType: check.recordType,
				RecordID: recordID, Detail: "terminal record has no completion timestamp",
			})
		}
		if err := rows.Err(); err != nil {
			closeRows(rows)
			return issues, fmt.Errorf("workstore: doctor terminal iterate %s: %w", check.recordType, err)
		}
		closeRows(rows)
	}
	return issues, nil
}

func (s *Store) doctorDependencyIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT step_id, depends_on_step_id FROM step_dependencies WHERE workspace_id = ?", workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workstore: doctor dependency query: %w", err)
	}
	defer closeRows(rows)
	graph := make(map[string][]string)
	for rows.Next() {
		var stepID, dependencyID string
		if err := rows.Scan(&stepID, &dependencyID); err != nil {
			return nil, fmt.Errorf("workstore: doctor dependency scan: %w", err)
		}
		graph[stepID] = append(graph[stepID], dependencyID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: doctor dependency iterate: %w", err)
	}
	states := make(map[string]uint8, len(graph))
	var cycleAt string
	var visit func(string) bool
	visit = func(stepID string) bool {
		if states[stepID] == 1 {
			cycleAt = stepID
			return true
		}
		if states[stepID] == 2 {
			return false
		}
		states[stepID] = 1
		for _, dependencyID := range graph[stepID] {
			if visit(dependencyID) {
				return true
			}
		}
		states[stepID] = 2
		return false
	}
	keys := make([]string, 0, len(graph))
	for stepID := range graph {
		keys = append(keys, stepID)
	}
	sort.Strings(keys)
	for _, stepID := range keys {
		if visit(stepID) {
			return []DoctorIssue{{
				Code: "dependency_cycle", RecordType: "step", RecordID: cycleAt,
				Detail: "step dependency graph contains a cycle",
			}}, nil
		}
	}
	return nil, nil
}

func (s *Store) doctorScheduleIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT schedule.step_id, step.state, schedule.lease_owner,
			COALESCE(schedule.active_attempt_id, ''), schedule.lease_expires_at,
			schedule.human_resume_required, schedule.cycle_attempt_count,
			schedule.attempt_count, COALESCE(attempt.status, '')
		FROM step_schedules schedule
		JOIN steps step ON step.id = schedule.step_id
		LEFT JOIN attempts attempt ON attempt.id = schedule.active_attempt_id
		WHERE schedule.workspace_id = ?
		ORDER BY schedule.step_id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workstore: doctor schedule query: %w", err)
	}
	defer closeRows(rows)
	var issues []DoctorIssue
	for rows.Next() {
		var stepID, state, leaseOwner, attemptID, attemptStatus string
		var leaseExpiresAt sql.NullInt64
		var humanResume, cycleAttempts, attempts int
		if err := rows.Scan(&stepID, &state, &leaseOwner, &attemptID, &leaseExpiresAt,
			&humanResume, &cycleAttempts, &attempts, &attemptStatus); err != nil {
			return issues, fmt.Errorf("workstore: doctor schedule scan: %w", err)
		}
		claimBroken := leaseOwner == "" && (attemptID != "" || leaseExpiresAt.Valid) ||
			leaseOwner != "" && (attemptID == "" || !leaseExpiresAt.Valid || WorkState(state) != WorkStateRunning || AttemptStatus(attemptStatus) != AttemptStatusRunning)
		if claimBroken {
			issues = append(issues, DoctorIssue{
				Code: "invalid_schedule_claim", RecordType: "step_schedule", RecordID: stepID,
				Detail: "lease, active attempt, and running state are inconsistent",
			})
		}
		if humanResume != 0 && (leaseOwner != "" || WorkState(state) != WorkStateReview && WorkState(state) != WorkStateBlocked) {
			issues = append(issues, DoctorIssue{
				Code: "invalid_schedule_resume", RecordType: "step_schedule", RecordID: stepID,
				Detail: "human resume is required outside a lease-free review or blocked state",
			})
		}
		if cycleAttempts > attempts {
			issues = append(issues, DoctorIssue{
				Code: "invalid_schedule_attempt_count", RecordType: "step_schedule", RecordID: stepID,
				Detail: "cycle attempt count exceeds lifetime attempt count",
			})
		}
	}
	if err := rows.Err(); err != nil {
		return issues, fmt.Errorf("workstore: doctor schedule iterate: %w", err)
	}
	return issues, nil
}

func (s *Store) doctorEffectReceiptIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, created_at, updated_at, committed_at,
			idempotency_key, effect_type, request_digest, actor_id
		FROM effect_receipts
		WHERE workspace_id = ?
		ORDER BY id
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workstore: doctor effect receipt query: %w", err)
	}
	defer closeRows(rows)
	var issues []DoctorIssue
	for rows.Next() {
		var id, status, idempotencyKey, effectType, requestDigest, actorID string
		var createdAt, updatedAt int64
		var committedAt sql.NullInt64
		if err := rows.Scan(&id, &status, &createdAt, &updatedAt, &committedAt,
			&idempotencyKey, &effectType, &requestDigest, &actorID); err != nil {
			return issues, fmt.Errorf("workstore: doctor effect receipt scan: %w", err)
		}
		invalidState := status == string(EffectReceiptStatusPending) && committedAt.Valid ||
			status == string(EffectReceiptStatusCommitted) && !committedAt.Valid ||
			updatedAt < createdAt || committedAt.Valid && (committedAt.Int64 < createdAt || committedAt.Int64 > updatedAt)
		if invalidState {
			issues = append(issues, DoctorIssue{
				Code: "invalid_effect_receipt_state", RecordType: "effect_receipt", RecordID: id,
				Field: "committed_at", Detail: "effect status and lifecycle timestamps are inconsistent",
			})
		}
		contracts := []struct{ field, value string }{
			{field: "idempotency_key", value: idempotencyKey},
			{field: "effect_type", value: effectType},
			{field: "request_digest", value: requestDigest},
			{field: "actor_id", value: actorID},
		}
		for _, contract := range contracts {
			if strings.TrimSpace(contract.value) == "" {
				issues = append(issues, DoctorIssue{
					Code: "invalid_effect_receipt_contract", RecordType: "effect_receipt", RecordID: id,
					Field: contract.field, Detail: "required effect receipt contract field is empty",
				})
			}
		}
	}
	if err := rows.Err(); err != nil {
		return issues, fmt.Errorf("workstore: doctor effect receipt iterate: %w", err)
	}
	return issues, nil
}

func (s *Store) doctorProofIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	rows, err := s.db.QueryContext(ctx, proofSelect+" WHERE workspace_id = ? ORDER BY id", workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workstore: doctor proof query: %w", err)
	}
	defer closeRows(rows)
	var issues []DoctorIssue
	for rows.Next() {
		proof, err := scanProof(rows)
		if err != nil {
			return issues, fmt.Errorf("workstore: doctor proof scan: %w", err)
		}
		if proof.UpdatedAt.Before(proof.CreatedAt) ||
			(proof.Status == ProofStatusPassed || proof.Status == ProofStatusFailed) && proof.ObservedAt == nil {
			issues = append(issues, DoctorIssue{
				Code: "invalid_proof_lifecycle", RecordType: "proof", RecordID: proof.ID,
				Detail: "proof status and lifecycle timestamps are inconsistent",
			})
		}
		if proof.Status == ProofStatusPassed && !proofHasIndependentProvenance(proof) {
			issues = append(issues, DoctorIssue{
				Code: "invalid_proof_provenance", RecordType: "proof", RecordID: proof.ID,
				Detail: "passed proof lacks an independent verifier identity, environment, subject digest, or rationale",
			})
		}
		if (proof.Origin == ProofOriginWorkerReport || proof.Origin == ProofOriginLegacy) &&
			proof.Status != ProofStatusReported && proof.Status != ProofStatusStale {
			issues = append(issues, DoctorIssue{
				Code: "invalid_proof_authority", RecordType: "proof", RecordID: proof.ID,
				Detail: "reported or legacy evidence cannot hold a verifier terminal state",
			})
		}
	}
	if err := rows.Err(); err != nil {
		return issues, fmt.Errorf("workstore: doctor proof iterate: %w", err)
	}
	return issues, nil
}

func (s *Store) doctorImportIssues(ctx context.Context, workspaceID string) ([]DoctorIssue, error) {
	markers, err := s.ListImportMarkers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var issues []DoctorIssue
	for _, marker := range markers {
		for _, workID := range marker.WorkIDs {
			if _, err := s.GetWork(ctx, workspaceID, workID); errors.Is(err, ErrNotFound) {
				issues = append(issues, DoctorIssue{
					Code: "missing_import_work", RecordType: "import_marker",
					RecordID: marker.ID, Field: "work_ids_json",
					Detail: "referenced work does not exist: " + workID,
				})
			} else if err != nil {
				return issues, err
			}
		}
	}
	return issues, nil
}

func quickCheckDB(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA quick_check")
	if err != nil {
		return err
	}
	defer closeRows(rows)
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(result), "ok") {
			return fmt.Errorf("SQLite quick_check: %s", result)
		}
	}
	return rows.Err()
}

func foreignKeyCheckDB(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer closeRows(rows)
	if rows.Next() {
		return fmt.Errorf("SQLite foreign_key_check reported a violation")
	}
	return rows.Err()
}

func verifyMigrationsDB(ctx context.Context, db *sql.DB, requireCurrent bool) error {
	rows, err := db.QueryContext(ctx, "SELECT version, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	defer closeRows(rows)
	seen := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return err
		}
		seen[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, migration := range schemaMigrations {
		checksum, found := seen[migration.version]
		if !found {
			if requireCurrent {
				return fmt.Errorf("migration %d is missing", migration.version)
			}
			continue
		}
		if checksum != checksumText(migration.sql) {
			return fmt.Errorf("migration %d checksum mismatch", migration.version)
		}
		delete(seen, migration.version)
	}
	if len(seen) > 0 {
		return fmt.Errorf("database contains unsupported migrations")
	}
	return nil
}

func validateSQLiteFile(ctx context.Context, path string) error {
	dsnURL := &url.URL{Scheme: "file", Path: path}
	query := dsnURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "foreign_keys(1)")
	dsnURL.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if err := quickCheckDB(ctx, db); err != nil {
		return err
	}
	if err := foreignKeyCheckDB(ctx, db); err != nil {
		return err
	}
	return verifyMigrationsDB(ctx, db, false)
}

func writeHashedJSONLine(writer *bufio.Writer, digest hash.Hash, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("workstore: encode JSONL record: %w", err)
	}
	raw = append(raw, '\n')
	if _, err := writer.Write(raw); err != nil {
		return fmt.Errorf("workstore: write JSONL record: %w", err)
	}
	if _, err := digest.Write(raw); err != nil {
		return fmt.Errorf("workstore: checksum JSONL record: %w", err)
	}
	return nil
}

func writeJSONLine(writer *bufio.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("workstore: encode JSONL footer: %w", err)
	}
	if _, err := writer.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("workstore: write JSONL footer: %w", err)
	}
	return nil
}

func requireMissingDestination(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrDestinationExists, path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("workstore: inspect destination %q: %w", path, err)
	}
	return nil
}

func vacantTemporaryPath(dir, pattern string) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("workstore: create temporary path: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("workstore: close temporary path: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("workstore: release temporary path: %w", err)
	}
	return path, nil
}

func publishTemporaryFile(temporary, destination string) error {
	if err := os.Link(temporary, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrDestinationExists, destination)
		}
		return err
	}
	if err := os.Remove(temporary); err != nil {
		return fmt.Errorf("remove published temporary file: %w", err)
	}
	return syncDirectory(filepath.Dir(destination))
}

func copyToTemporaryFile(source, destinationDir, pattern string) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("workstore: open backup source: %w", err)
	}
	defer func() { _ = input.Close() }()
	output, err := os.CreateTemp(destinationDir, pattern)
	if err != nil {
		return "", fmt.Errorf("workstore: create restore temporary file: %w", err)
	}
	path := output.Name()
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			removeFile(path)
		}
	}()
	if err := output.Chmod(0o600); err != nil {
		return "", fmt.Errorf("workstore: secure restore temporary file: %w", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		return "", fmt.Errorf("workstore: copy backup: %w", err)
	}
	if err := output.Sync(); err != nil {
		return "", fmt.Errorf("workstore: sync restored database: %w", err)
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("workstore: close restored database: %w", err)
	}
	ok = true
	return path, nil
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("workstore: open file for checksum: %w", err)
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	if err != nil {
		return "", 0, fmt.Errorf("workstore: checksum file: %w", err)
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}

func writeExactFile(path string, raw []byte, mode os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil {
		if bytesEqual(existing, raw) {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrDestinationExists, path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".workstore-write-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer removeFile(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := publishTemporaryFile(temporaryPath, path); err != nil {
		if errors.Is(err, ErrDestinationExists) {
			existing, readErr := os.ReadFile(path)
			if readErr == nil && bytesEqual(existing, raw) {
				return nil
			}
		}
		return err
	}
	return nil
}

func quarantinePaths(dir, sourcePath, digest string, timestamp time.Time) (string, string) {
	base := filepath.Base(sourcePath)
	base = strings.Map(func(value rune) rune {
		switch {
		case value >= 'a' && value <= 'z':
			return value
		case value >= 'A' && value <= 'Z':
			return value
		case value >= '0' && value <= '9':
			return value
		case value == '.', value == '-', value == '_':
			return value
		default:
			return '_'
		}
	}, base)
	if base == "" || base == "." {
		base = "source.bin"
	}
	name := timestamp.UTC().Format("20060102T150405.000000000Z") + "-" + digest[:12] + "-" + base
	copyPath := filepath.Join(dir, name)
	return copyPath, copyPath + ".manifest.json"
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	return directory.Sync()
}

func removeFile(path string) {
	_ = os.Remove(path)
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
