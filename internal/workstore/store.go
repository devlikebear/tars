package workstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// Register the pure-Go SQLite driver used by database/sql.
	_ "modernc.org/sqlite"
)

const schemaVersion = 6

type Options struct {
	Now   func() time.Time
	NewID func(prefix string) (string, error)
}

type Store struct {
	db       *sql.DB
	now      func() time.Time
	newID    func(prefix string) (string, error)
	writeMu  sync.Mutex
	importMu sync.Mutex
}

const migrationV1 = `
CREATE TABLE works (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    parent_work_id TEXT REFERENCES works(id),
    title TEXT NOT NULL,
    objective TEXT NOT NULL DEFAULT '',
    contract_json BLOB NOT NULL DEFAULT '{}',
    metadata_json BLOB NOT NULL DEFAULT '{}',
    state TEXT NOT NULL CHECK (state IN ('triage','backlog','todo','ready','running','review','blocked','done','cancelled')),
    priority INTEGER NOT NULL DEFAULT 0,
    actor_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX idx_works_workspace_state_updated
    ON works (workspace_id, state, updated_at DESC);
CREATE INDEX idx_works_workspace_source
    ON works (workspace_id, source, source_id);

CREATE TABLE steps (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    parent_step_id TEXT REFERENCES steps(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('triage','backlog','todo','ready','running','review','blocked','done','cancelled')),
    position INTEGER NOT NULL DEFAULT 0,
    actor_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER,
    UNIQUE (workspace_id, work_id, idempotency_key)
);
CREATE INDEX idx_steps_workspace_work_position
    ON steps (workspace_id, work_id, position, created_at);
CREATE INDEX idx_steps_workspace_state_updated
    ON steps (workspace_id, state, updated_at DESC);

CREATE TABLE step_dependencies (
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT NOT NULL REFERENCES steps(id),
    depends_on_step_id TEXT NOT NULL REFERENCES steps(id),
    actor_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (step_id, depends_on_step_id),
    CHECK (step_id <> depends_on_step_id)
);
CREATE INDEX idx_step_dependencies_workspace_work
    ON step_dependencies (workspace_id, work_id, step_id);

CREATE TABLE attempts (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    attempt_number INTEGER NOT NULL,
    adapter TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','succeeded','failed','cancelled')),
    actor_id TEXT NOT NULL,
    input_json BLOB NOT NULL DEFAULT '{}',
    output_json BLOB NOT NULL DEFAULT '{}',
    error_text TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    finished_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, work_id, idempotency_key),
    UNIQUE (step_id, attempt_number)
);
CREATE INDEX idx_attempts_workspace_work_created
    ON attempts (workspace_id, work_id, created_at);
CREATE INDEX idx_attempts_workspace_status_updated
    ON attempts (workspace_id, status, updated_at DESC);

CREATE TABLE events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    schema_version INTEGER NOT NULL,
    id TEXT NOT NULL UNIQUE,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    attempt_id TEXT REFERENCES attempts(id),
    event_type TEXT NOT NULL,
    from_state TEXT NOT NULL DEFAULT '',
    to_state TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    payload_json BLOB NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_events_workspace_work_sequence
    ON events (workspace_id, work_id, sequence);
CREATE INDEX idx_events_workspace_type_created
    ON events (workspace_id, event_type, created_at);
CREATE UNIQUE INDEX idx_events_idempotency
    ON events (workspace_id, work_id, event_type, idempotency_key)
    WHERE idempotency_key <> '';

CREATE TABLE artifacts (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    attempt_id TEXT REFERENCES attempts(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    uri TEXT NOT NULL,
    digest TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    actor_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (workspace_id, work_id, idempotency_key)
);
CREATE INDEX idx_artifacts_workspace_work_created
    ON artifacts (workspace_id, work_id, created_at);
CREATE INDEX idx_artifacts_digest ON artifacts (digest);

CREATE TABLE approvals (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    attempt_id TEXT REFERENCES attempts(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    authority TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','approved','denied','expired')),
    request TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL,
    reviewer_id TEXT NOT NULL DEFAULT '',
    expires_at INTEGER,
    decided_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, work_id, idempotency_key)
);
CREATE INDEX idx_approvals_workspace_status_created
    ON approvals (workspace_id, status, created_at);
CREATE INDEX idx_approvals_workspace_work_created
    ON approvals (workspace_id, work_id, created_at);

CREATE TABLE proofs (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    attempt_id TEXT REFERENCES attempts(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('passed','failed','inconclusive')),
    summary TEXT NOT NULL,
    verifier TEXT NOT NULL DEFAULT '',
    command TEXT NOT NULL DEFAULT '',
    artifact_id TEXT REFERENCES artifacts(id),
    actor_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (workspace_id, work_id, idempotency_key)
);
CREATE INDEX idx_proofs_workspace_work_created
    ON proofs (workspace_id, work_id, created_at);
CREATE INDEX idx_proofs_workspace_status_created
    ON proofs (workspace_id, status, created_at);
`

const migrationV2 = `
CREATE TABLE import_markers (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_path TEXT NOT NULL DEFAULT '',
    checksum TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('completed','failed','quarantined')),
    work_ids_json BLOB NOT NULL DEFAULT '[]',
    actor_id TEXT NOT NULL,
    error_text TEXT NOT NULL DEFAULT '',
    imported_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, source_kind, source_id, checksum)
);
CREATE INDEX idx_import_markers_workspace_kind_imported
    ON import_markers (workspace_id, source_kind, imported_at DESC);
CREATE INDEX idx_import_markers_workspace_source
    ON import_markers (workspace_id, source_kind, source_id, imported_at DESC);
`

const migrationV3 = `
CREATE TABLE step_schedules (
    schema_version INTEGER NOT NULL,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT PRIMARY KEY REFERENCES steps(id),
    policy_json BLOB NOT NULL DEFAULT '{}',
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at INTEGER,
    last_heartbeat_at INTEGER,
    active_attempt_id TEXT REFERENCES attempts(id),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    consumed_iterations INTEGER NOT NULL DEFAULT 0 CHECK (consumed_iterations >= 0),
    consumed_tokens INTEGER NOT NULL DEFAULT 0 CHECK (consumed_tokens >= 0),
    consumed_cost_usd REAL NOT NULL DEFAULT 0 CHECK (consumed_cost_usd >= 0),
    next_action TEXT NOT NULL DEFAULT 'execute' CHECK (next_action IN ('execute','retry','replan','decompose')),
    last_disposition TEXT NOT NULL DEFAULT '' CHECK (last_disposition IN ('','done','retry','replan','decompose','review','blocked')),
    blocked_reason TEXT NOT NULL DEFAULT '',
    human_resume_required INTEGER NOT NULL DEFAULT 0 CHECK (human_resume_required IN (0,1)),
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, work_id, step_id)
);
CREATE INDEX idx_step_schedules_workspace_lease
    ON step_schedules (workspace_id, lease_expires_at, step_id);
CREATE INDEX idx_step_schedules_workspace_work_action
    ON step_schedules (workspace_id, work_id, next_action, step_id);
`

const migrationV4 = `
ALTER TABLE step_schedules
    ADD COLUMN cycle_attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (cycle_attempt_count >= 0);
`

const migrationV5 = `
CREATE TABLE effect_receipts (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    attempt_id TEXT REFERENCES attempts(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    effect_type TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    request_digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','committed')),
    outcome_json BLOB NOT NULL DEFAULT '{}',
    external_reference TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    committed_at INTEGER,
    UNIQUE (workspace_id, idempotency_key)
);
CREATE INDEX idx_effect_receipts_workspace_work_created
    ON effect_receipts (workspace_id, work_id, created_at);
CREATE INDEX idx_effect_receipts_workspace_status_updated
    ON effect_receipts (workspace_id, status, updated_at DESC);
`

const migrationV6 = `
CREATE TABLE proofs_v6 (
    schema_version INTEGER NOT NULL,
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    work_id TEXT NOT NULL REFERENCES works(id),
    step_id TEXT REFERENCES steps(id),
    attempt_id TEXT REFERENCES attempts(id),
    idempotency_key TEXT NOT NULL,
    causation_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('reported','pending','passed','failed','stale')),
    origin TEXT NOT NULL CHECK (origin IN ('worker_report','independent_verifier','legacy')),
    summary TEXT NOT NULL,
    reporter_id TEXT NOT NULL DEFAULT '',
    verifier_id TEXT NOT NULL DEFAULT '',
    verifier TEXT NOT NULL DEFAULT '',
    command TEXT NOT NULL DEFAULT '',
    artifact_id TEXT REFERENCES artifacts(id),
    environment_json BLOB NOT NULL DEFAULT '{}',
    input_json BLOB NOT NULL DEFAULT '{}',
    artifact_digests_json BLOB NOT NULL DEFAULT '[]',
    subject_digest TEXT NOT NULL DEFAULT '',
    rationale TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL,
    observed_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, work_id, idempotency_key)
);
INSERT INTO proofs_v6 (
    schema_version, id, workspace_id, work_id, step_id, attempt_id,
    idempotency_key, causation_id, kind, status, origin, summary,
    reporter_id, verifier_id, verifier, command, artifact_id,
    environment_json, input_json, artifact_digests_json, subject_digest,
    rationale, actor_id, observed_at, created_at, updated_at
)
SELECT
    schema_version, id, workspace_id, work_id, step_id, attempt_id,
    idempotency_key, causation_id, kind, 'reported', 'legacy', summary,
    actor_id, verifier, verifier, command, artifact_id,
    '{}', '{}', '[]', '', 'migrated from legacy status: ' || status,
    actor_id, created_at, created_at, created_at
FROM proofs;
DROP TABLE proofs;
ALTER TABLE proofs_v6 RENAME TO proofs;
CREATE INDEX idx_proofs_workspace_work_created
    ON proofs (workspace_id, work_id, created_at);
CREATE INDEX idx_proofs_workspace_status_created
    ON proofs (workspace_id, status, created_at);
CREATE INDEX idx_proofs_workspace_step_status
    ON proofs (workspace_id, work_id, step_id, status, updated_at DESC);
`

var schemaMigrations = []struct {
	version int
	sql     string
}{
	{version: 1, sql: migrationV1},
	{version: 2, sql: migrationV2},
	{version: 3, sql: migrationV3},
	{version: 4, sql: migrationV4},
	{version: 5, sql: migrationV5},
	{version: 6, sql: migrationV6},
}

func Open(ctx context.Context, path string, opts Options) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("workstore: database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("workstore: create database directory: %w", err)
	}
	databaseFile, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("workstore: create database file: %w", err)
	}
	if err := databaseFile.Close(); err != nil {
		return nil, fmt.Errorf("workstore: close database file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("workstore: secure database permissions: %w", err)
	}

	dsnURL := &url.URL{Scheme: "file", Path: path}
	query := dsnURL.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(FULL)")
	dsnURL.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("workstore: open database: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)

	store := &Store{db: db, now: opts.Now, newID: opts.NewID}
	if store.now == nil {
		store.now = time.Now
	}
	if store.newID == nil {
		store.newID = randomID
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workstore: ping database: %w", err)
	}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workstore: secure database permissions: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("workstore: create migration table: %w", err)
	}

	for _, migration := range schemaMigrations {
		if err := s.applyMigration(ctx, migration.version, migration.sql); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, version int, migrationSQL string) error {
	checksum := checksumText(migrationSQL)
	var existing string
	err := s.db.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = ?", version).Scan(&existing)
	switch {
	case err == nil:
		if existing != checksum {
			return fmt.Errorf("workstore: migration %d checksum mismatch", version)
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("workstore: read migration state: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workstore: begin migration %d: %w", version, err)
	}
	defer rollback(tx)
	if _, err := tx.ExecContext(ctx, migrationSQL); err != nil {
		return fmt.Errorf("workstore: apply migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, checksum, applied_at) VALUES (?, ?, ?)",
		version, checksum, s.now().UTC().UnixMilli(),
	); err != nil {
		return fmt.Errorf("workstore: record migration %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workstore: commit migration %d: %w", version, err)
	}
	return nil
}

func checksumText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}

func (s *Store) CreateWork(ctx context.Context, input CreateWorkInput) (Work, error) {
	if err := validateCreateWork(input); err != nil {
		return Work{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	state := input.InitialState
	if state == "" {
		state = WorkStateTriage
	}
	contract, err := normalizedJSON(input.ContractJSON)
	if err != nil {
		return Work{}, fmt.Errorf("workstore: contract json: %w", err)
	}
	metadata, err := normalizedJSON(input.MetadataJSON)
	if err != nil {
		return Work{}, fmt.Errorf("workstore: metadata json: %w", err)
	}
	id, err := s.newID("wrk")
	if err != nil {
		return Work{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Work{}, fmt.Errorf("workstore: begin create work: %w", err)
	}
	defer rollback(tx)
	if input.ParentWorkID != "" {
		if err := ensureWorkTx(ctx, tx, input.WorkspaceID, input.ParentWorkID); err != nil {
			return Work{}, fmt.Errorf("workstore: parent work: %w", err)
		}
	}
	var completedAt any
	if state == WorkStateDone || state == WorkStateCancelled {
		completedAt = now.UnixMilli()
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO works (
			schema_version, id, workspace_id, kind, source, source_id,
			idempotency_key, causation_id, parent_work_id, title, objective,
			contract_json, metadata_json, state, priority, actor_id, version,
			created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT (workspace_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.Kind, input.Source, input.SourceID,
		input.IdempotencyKey, input.CausationID, nullableString(input.ParentWorkID), input.Title,
		input.Objective, contract, metadata, state, input.Priority, input.ActorID,
		now.UnixMilli(), now.UnixMilli(), completedAt)
	if err != nil {
		return Work{}, fmt.Errorf("workstore: insert work: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Work{}, fmt.Errorf("workstore: inspect work insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      id,
			Type:        EventTypeWorkCreated,
			ToState:     state,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"kind": input.Kind, "source": input.Source, "source_id": input.SourceID}),
			CreatedAt:   now,
		}); err != nil {
			return Work{}, err
		}
	}
	work, err := getWorkTx(ctx, tx, input.WorkspaceID, id, input.IdempotencyKey)
	if err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("workstore: commit create work: %w", err)
	}
	return work, nil
}

func (s *Store) GetWork(ctx context.Context, workspaceID, workID string) (Work, error) {
	work, err := scanWork(s.db.QueryRowContext(ctx, workSelect+" WHERE workspace_id = ? AND id = ?", workspaceID, workID))
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, ErrNotFound
	}
	if err != nil {
		return Work{}, fmt.Errorf("workstore: get work: %w", err)
	}
	return work, nil
}

func (s *Store) ListWorks(ctx context.Context, filter ListWorksFilter) ([]Work, error) {
	if strings.TrimSpace(filter.WorkspaceID) == "" {
		return nil, fmt.Errorf("workstore: workspace id is required")
	}
	query := workSelect + " WHERE workspace_id = ?"
	args := []any{filter.WorkspaceID}
	source := strings.TrimSpace(filter.Source)
	sourceID := strings.TrimSpace(filter.SourceID)
	if sourceID != "" && source == "" {
		return nil, fmt.Errorf("workstore: source is required when source id is set")
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
		if sourceID != "" {
			query += " AND source_id = ?"
			args = append(args, sourceID)
		}
	}
	if len(filter.States) > 0 {
		placeholders := make([]string, 0, len(filter.States))
		for _, state := range filter.States {
			if !validWorkState(state) {
				return nil, fmt.Errorf("workstore: invalid state %q", state)
			}
			placeholders = append(placeholders, "?")
			args = append(args, state)
		}
		query += " AND state IN (" + strings.Join(placeholders, ",") + ")"
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query += " ORDER BY priority DESC, updated_at DESC, id LIMIT ? OFFSET ?"
	args = append(args, limit, max(filter.Offset, 0))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workstore: list works: %w", err)
	}
	defer closeRows(rows)
	var works []Work
	for rows.Next() {
		work, err := scanWork(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan work: %w", err)
		}
		works = append(works, work)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate works: %w", err)
	}
	return works, nil
}

func (s *Store) TransitionWork(ctx context.Context, input TransitionWorkInput) (Work, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return Work{}, fmt.Errorf("workstore: workspace id, work id, and actor id are required")
	}
	if !validWorkState(input.ToState) {
		return Work{}, fmt.Errorf("workstore: invalid target state %q", input.ToState)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Work{}, fmt.Errorf("workstore: begin transition: %w", err)
	}
	defer rollback(tx)
	if input.IdempotencyKey != "" {
		var existing int
		err := tx.QueryRowContext(ctx, `
			SELECT 1 FROM events
			WHERE workspace_id = ? AND work_id = ? AND event_type = ? AND idempotency_key = ?
		`, input.WorkspaceID, input.WorkID, EventTypeWorkTransitioned, input.IdempotencyKey).Scan(&existing)
		if err == nil {
			work, err := getWorkTx(ctx, tx, input.WorkspaceID, input.WorkID, "")
			if err != nil {
				return Work{}, err
			}
			if err := tx.Commit(); err != nil {
				return Work{}, fmt.Errorf("workstore: commit idempotent transition: %w", err)
			}
			return work, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Work{}, fmt.Errorf("workstore: query transition idempotency: %w", err)
		}
	}
	current, err := getWorkTx(ctx, tx, input.WorkspaceID, input.WorkID, "")
	if err != nil {
		return Work{}, err
	}
	if current.Version != input.ExpectedVersion {
		return Work{}, fmt.Errorf("%w: work %s is version %d, expected %d", ErrConflict, input.WorkID, current.Version, input.ExpectedVersion)
	}
	if !canTransition(current.State, input.ToState) {
		return Work{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, current.State, input.ToState)
	}
	now := s.now().UTC()
	var completedAt any
	if input.ToState == WorkStateDone || input.ToState == WorkStateCancelled {
		completedAt = now.UnixMilli()
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE works
		SET state = ?, version = version + 1, updated_at = ?, completed_at = ?
		WHERE workspace_id = ? AND id = ? AND version = ?
	`, input.ToState, now.UnixMilli(), completedAt, input.WorkspaceID, input.WorkID, input.ExpectedVersion)
	if err != nil {
		return Work{}, fmt.Errorf("workstore: update work state: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return Work{}, fmt.Errorf("workstore: inspect transition: %w", err)
	}
	if updated != 1 {
		return Work{}, ErrConflict
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID:    input.WorkspaceID,
		WorkID:         input.WorkID,
		Type:           EventTypeWorkTransitioned,
		FromState:      current.State,
		ToState:        input.ToState,
		ActorID:        input.ActorID,
		CausationID:    input.CausationID,
		IdempotencyKey: input.IdempotencyKey,
		PayloadJSON:    mustJSON(map[string]any{"reason": input.Reason, "previous_version": current.Version}),
		CreatedAt:      now,
	}); err != nil {
		return Work{}, err
	}
	work, err := getWorkTx(ctx, tx, input.WorkspaceID, input.WorkID, "")
	if err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("workstore: commit transition: %w", err)
	}
	return work, nil
}

func (s *Store) CreateStep(ctx context.Context, input CreateStepInput) (Step, error) {
	if err := validateCreateStep(input); err != nil {
		return Step{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	state := input.State
	if state == "" {
		state = WorkStateTodo
	}
	id, err := s.newID("stp")
	if err != nil {
		return Step{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Step{}, fmt.Errorf("workstore: begin create step: %w", err)
	}
	defer rollback(tx)
	if err := ensureWorkTx(ctx, tx, input.WorkspaceID, input.WorkID); err != nil {
		return Step{}, err
	}
	if input.ParentStepID != "" {
		if err := ensureStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.ParentStepID); err != nil {
			return Step{}, fmt.Errorf("workstore: parent step: %w", err)
		}
	}
	var completedAt any
	if state == WorkStateDone || state == WorkStateCancelled {
		completedAt = now.UnixMilli()
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO steps (
			schema_version, id, workspace_id, work_id, parent_step_id,
			idempotency_key, causation_id, title, description, state, position,
			actor_id, version, created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT (workspace_id, work_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, nullableString(input.ParentStepID),
		input.IdempotencyKey, input.CausationID, input.Title, input.Description, state,
		input.Position, input.ActorID, now.UnixMilli(), now.UnixMilli(), completedAt)
	if err != nil {
		return Step{}, fmt.Errorf("workstore: insert step: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Step{}, fmt.Errorf("workstore: inspect step insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			StepID:      id,
			Type:        EventTypeStepCreated,
			ToState:     state,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"title": input.Title, "position": input.Position}),
			CreatedAt:   now,
		}); err != nil {
			return Step{}, err
		}
	}
	step, err := getStepTx(ctx, tx, input.WorkspaceID, input.WorkID, id, input.IdempotencyKey)
	if err != nil {
		return Step{}, err
	}
	if err := tx.Commit(); err != nil {
		return Step{}, fmt.Errorf("workstore: commit create step: %w", err)
	}
	return step, nil
}

func (s *Store) AddStepDependency(ctx context.Context, input AddStepDependencyInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.DependsOnID) == "" || strings.TrimSpace(input.ActorID) == "" {
		return fmt.Errorf("workstore: workspace, work, step, dependency, and actor ids are required")
	}
	if input.StepID == input.DependsOnID {
		return ErrDependencyCycle
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("workstore: begin add dependency: %w", err)
	}
	defer rollback(tx)
	if err := ensureStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID); err != nil {
		return fmt.Errorf("%w: dependent step: %v", ErrInvalidDependency, err)
	}
	if err := ensureStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.DependsOnID); err != nil {
		return fmt.Errorf("%w: prerequisite step: %v", ErrInvalidDependency, err)
	}
	var cycle int
	err = tx.QueryRowContext(ctx, `
		WITH RECURSIVE ancestors(id) AS (
			SELECT depends_on_step_id FROM step_dependencies WHERE step_id = ?
			UNION
			SELECT d.depends_on_step_id
			FROM step_dependencies d
			JOIN ancestors a ON d.step_id = a.id
		)
		SELECT 1 FROM ancestors WHERE id = ? LIMIT 1
	`, input.DependsOnID, input.StepID).Scan(&cycle)
	if err == nil {
		return ErrDependencyCycle
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("workstore: inspect dependency graph: %w", err)
	}
	now := s.now().UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO step_dependencies (
			workspace_id, work_id, step_id, depends_on_step_id, actor_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (step_id, depends_on_step_id) DO NOTHING
	`, input.WorkspaceID, input.WorkID, input.StepID, input.DependsOnID, input.ActorID, now.UnixMilli())
	if err != nil {
		return fmt.Errorf("workstore: insert dependency: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("workstore: inspect dependency insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			StepID:      input.StepID,
			Type:        EventTypeStepDependencyAdded,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"depends_on_step_id": input.DependsOnID}),
			CreatedAt:   now,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("workstore: commit dependency: %w", err)
	}
	return nil
}

func (s *Store) CreateAttempt(ctx context.Context, input CreateAttemptInput) (Attempt, error) {
	if err := validateCreateAttempt(input); err != nil {
		return Attempt{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	inputJSON, err := normalizedJSON(input.InputJSON)
	if err != nil {
		return Attempt{}, fmt.Errorf("workstore: attempt input json: %w", err)
	}
	outputJSON, err := normalizedJSON(input.OutputJSON)
	if err != nil {
		return Attempt{}, fmt.Errorf("workstore: attempt output json: %w", err)
	}
	id, err := s.newID("att")
	if err != nil {
		return Attempt{}, err
	}
	now := s.now().UTC()
	startedAt := now
	if input.StartedAt != nil {
		startedAt = input.StartedAt.UTC()
	}
	finishedAt := input.FinishedAt
	if finishedAt == nil && (input.Status == AttemptStatusSucceeded || input.Status == AttemptStatusFailed || input.Status == AttemptStatusCancelled) {
		finishedAt = &now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, fmt.Errorf("workstore: begin create attempt: %w", err)
	}
	defer rollback(tx)
	if err := ensureStepTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID); err != nil {
		return Attempt{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO attempts (
			schema_version, id, workspace_id, work_id, step_id, idempotency_key,
			causation_id, attempt_number, adapter, status, actor_id, input_json,
			output_json, error_text, started_at, finished_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, work_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, nullableString(input.StepID),
		input.IdempotencyKey, input.CausationID, input.Number, input.Adapter, input.Status,
		input.ActorID, inputJSON, outputJSON, input.ErrorText, startedAt.UnixMilli(),
		nullableTime(finishedAt), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Attempt{}, fmt.Errorf("workstore: insert attempt: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Attempt{}, fmt.Errorf("workstore: inspect attempt insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			StepID:      input.StepID,
			AttemptID:   id,
			Type:        EventTypeAttemptCreated,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"number": input.Number, "adapter": input.Adapter, "status": input.Status}),
			CreatedAt:   now,
		}); err != nil {
			return Attempt{}, err
		}
	}
	attempt, err := getAttemptTx(ctx, tx, input.WorkspaceID, input.WorkID, id, input.IdempotencyKey)
	if err != nil {
		return Attempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attempt{}, fmt.Errorf("workstore: commit create attempt: %w", err)
	}
	return attempt, nil
}

func (s *Store) CreateApproval(ctx context.Context, input CreateApprovalInput) (Approval, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Authority) == "" || strings.TrimSpace(input.Request) == "" || strings.TrimSpace(input.ActorID) == "" || !validApprovalStatus(input.Status) {
		return Approval{}, fmt.Errorf("workstore: invalid approval input")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	id, err := s.newID("apr")
	if err != nil {
		return Approval{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Approval{}, fmt.Errorf("workstore: begin create approval: %w", err)
	}
	defer rollback(tx)
	if err := ensureReferencesTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, input.AttemptID); err != nil {
		return Approval{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO approvals (
			schema_version, id, workspace_id, work_id, step_id, attempt_id,
			idempotency_key, causation_id, authority, status, request, reason,
			actor_id, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, work_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, nullableString(input.StepID),
		nullableString(input.AttemptID), input.IdempotencyKey, input.CausationID, input.Authority,
		input.Status, input.Request, input.Reason, input.ActorID, nullableTime(input.ExpiresAt),
		now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Approval{}, fmt.Errorf("workstore: insert approval: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Approval{}, fmt.Errorf("workstore: inspect approval insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			StepID:      input.StepID,
			AttemptID:   input.AttemptID,
			Type:        EventTypeApprovalCreated,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"authority": input.Authority, "status": input.Status}),
			CreatedAt:   now,
		}); err != nil {
			return Approval{}, err
		}
	}
	approval, err := getApprovalTx(ctx, tx, input.WorkspaceID, input.WorkID, id, input.IdempotencyKey)
	if err != nil {
		return Approval{}, err
	}
	if err := tx.Commit(); err != nil {
		return Approval{}, fmt.Errorf("workstore: commit create approval: %w", err)
	}
	return approval, nil
}

func (s *Store) CreateArtifact(ctx context.Context, input CreateArtifactInput) (Artifact, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.URI) == "" || strings.TrimSpace(input.Digest) == "" || strings.TrimSpace(input.ActorID) == "" || input.SizeBytes < 0 {
		return Artifact{}, fmt.Errorf("workstore: invalid artifact input")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	id, err := s.newID("art")
	if err != nil {
		return Artifact{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Artifact{}, fmt.Errorf("workstore: begin create artifact: %w", err)
	}
	defer rollback(tx)
	if err := ensureReferencesTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, input.AttemptID); err != nil {
		return Artifact{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO artifacts (
			schema_version, id, workspace_id, work_id, step_id, attempt_id,
			idempotency_key, causation_id, kind, name, uri, digest, media_type,
			size_bytes, actor_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, work_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, nullableString(input.StepID),
		nullableString(input.AttemptID), input.IdempotencyKey, input.CausationID, input.Kind,
		input.Name, input.URI, input.Digest, input.MediaType, input.SizeBytes, input.ActorID,
		now.UnixMilli())
	if err != nil {
		return Artifact{}, fmt.Errorf("workstore: insert artifact: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Artifact{}, fmt.Errorf("workstore: inspect artifact insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			StepID:      input.StepID,
			AttemptID:   input.AttemptID,
			Type:        EventTypeArtifactCreated,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"artifact_id": id, "kind": input.Kind, "digest": input.Digest}),
			CreatedAt:   now,
		}); err != nil {
			return Artifact{}, err
		}
	}
	artifact, err := getArtifactTx(ctx, tx, input.WorkspaceID, input.WorkID, id, input.IdempotencyKey)
	if err != nil {
		return Artifact{}, err
	}
	if err := tx.Commit(); err != nil {
		return Artifact{}, fmt.Errorf("workstore: commit create artifact: %w", err)
	}
	return artifact, nil
}

func (s *Store) CreateProof(ctx context.Context, input CreateProofInput) (Proof, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Summary) == "" || strings.TrimSpace(input.ActorID) == "" || !validProofStatus(input.Status) {
		return Proof{}, fmt.Errorf("workstore: invalid proof input")
	}
	origin := input.Origin
	if origin == "" {
		if input.Status == ProofStatusReported {
			origin = ProofOriginWorkerReport
		} else {
			origin = ProofOriginIndependentVerifier
		}
	}
	if !validProofOrigin(origin) {
		return Proof{}, fmt.Errorf("workstore: invalid proof origin")
	}
	environmentJSON, err := normalizedJSON(input.EnvironmentJSON)
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: proof environment json: %w", err)
	}
	inputJSON, err := normalizedJSON(input.InputJSON)
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: proof input json: %w", err)
	}
	artifactDigestsJSON := input.ArtifactDigestsJSON
	if len(artifactDigestsJSON) == 0 {
		artifactDigestsJSON = json.RawMessage(`[]`)
	}
	artifactDigestsJSON, err = normalizedJSON(artifactDigestsJSON)
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: proof artifact digests json: %w", err)
	}
	reporterID := strings.TrimSpace(input.ReporterID)
	verifierID := strings.TrimSpace(input.VerifierID)
	if reporterID == "" && origin != ProofOriginIndependentVerifier {
		reporterID = strings.TrimSpace(input.ActorID)
	}
	if verifierID == "" && origin == ProofOriginIndependentVerifier {
		verifierID = strings.TrimSpace(input.Verifier)
		if verifierID == "" {
			verifierID = strings.TrimSpace(input.ActorID)
		}
	}
	rationale := strings.TrimSpace(input.Rationale)
	if rationale == "" && (input.Status == ProofStatusPassed || input.Status == ProofStatusFailed) {
		rationale = strings.TrimSpace(input.Summary)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	id, err := s.newID("prf")
	if err != nil {
		return Proof{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: begin create proof: %w", err)
	}
	defer rollback(tx)
	if err := ensureReferencesTx(ctx, tx, input.WorkspaceID, input.WorkID, input.StepID, input.AttemptID); err != nil {
		return Proof{}, err
	}
	if input.ArtifactID != "" {
		var found int
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM artifacts WHERE workspace_id = ? AND work_id = ? AND id = ?", input.WorkspaceID, input.WorkID, input.ArtifactID).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return Proof{}, fmt.Errorf("workstore: artifact reference: %w", ErrNotFound)
		}
		if err != nil {
			return Proof{}, fmt.Errorf("workstore: validate artifact reference: %w", err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO proofs (
			schema_version, id, workspace_id, work_id, step_id, attempt_id,
			idempotency_key, causation_id, kind, status, origin, summary,
			reporter_id, verifier_id, verifier, command, artifact_id,
			environment_json, input_json, artifact_digests_json, subject_digest,
			rationale, actor_id, observed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id, work_id, idempotency_key) DO NOTHING
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, nullableString(input.StepID),
		nullableString(input.AttemptID), input.IdempotencyKey, input.CausationID, input.Kind,
		input.Status, origin, input.Summary, reporterID, verifierID, input.Verifier,
		input.Command, nullableString(input.ArtifactID), environmentJSON, inputJSON,
		artifactDigestsJSON, strings.TrimSpace(input.SubjectDigest), rationale, input.ActorID,
		nullableTime(proofObservedAt(input.Status, input.ObservedAt, now)), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: insert proof: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: inspect proof insert: %w", err)
	}
	if inserted == 1 {
		if err := s.insertEventTx(ctx, tx, eventInput{
			WorkspaceID: input.WorkspaceID,
			WorkID:      input.WorkID,
			StepID:      input.StepID,
			AttemptID:   input.AttemptID,
			Type:        EventTypeProofCreated,
			ActorID:     input.ActorID,
			CausationID: input.CausationID,
			PayloadJSON: mustJSON(map[string]any{"proof_id": id, "kind": input.Kind, "status": input.Status}),
			CreatedAt:   now,
		}); err != nil {
			return Proof{}, err
		}
	}
	proof, err := getProofTx(ctx, tx, input.WorkspaceID, input.WorkID, id, input.IdempotencyKey)
	if err != nil {
		return Proof{}, err
	}
	if err := tx.Commit(); err != nil {
		return Proof{}, fmt.Errorf("workstore: commit create proof: %w", err)
	}
	return proof, nil
}

func (s *Store) GetProof(ctx context.Context, workspaceID, workID, proofID string) (Proof, error) {
	proof, err := scanProof(s.db.QueryRowContext(ctx, proofSelect+" WHERE workspace_id = ? AND work_id = ? AND id = ?", workspaceID, workID, proofID))
	if errors.Is(err, sql.ErrNoRows) {
		return Proof{}, ErrNotFound
	}
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: get proof: %w", err)
	}
	return proof, nil
}

func (s *Store) TransitionProof(ctx context.Context, input TransitionProofInput) (Proof, error) {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.ProofID) == "" || strings.TrimSpace(input.ActorID) == "" || !validProofStatus(input.ExpectedStatus) || !validProofStatus(input.ToStatus) {
		return Proof{}, fmt.Errorf("workstore: invalid proof transition input")
	}
	if input.ExpectedStatus != input.ToStatus && !canTransitionProof(input.ExpectedStatus, input.ToStatus) {
		return Proof{}, ErrInvalidTransition
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: begin proof transition: %w", err)
	}
	defer rollback(tx)
	proof, err := getProofTx(ctx, tx, input.WorkspaceID, input.WorkID, input.ProofID, "")
	if err != nil {
		return Proof{}, err
	}
	if proof.Status != input.ExpectedStatus {
		return Proof{}, ErrConflict
	}
	if input.ExpectedStatus == input.ToStatus {
		if err := tx.Commit(); err != nil {
			return Proof{}, fmt.Errorf("workstore: commit idempotent proof transition: %w", err)
		}
		return proof, nil
	}
	now := s.now().UTC()
	rationale := strings.TrimSpace(input.Rationale)
	if rationale == "" {
		rationale = proof.Rationale
	}
	subjectDigest := strings.TrimSpace(input.SubjectDigest)
	if subjectDigest == "" {
		subjectDigest = proof.SubjectDigest
	}
	observedAt := input.ObservedAt
	if observedAt == nil {
		observedAt = proof.ObservedAt
	}
	if input.ToStatus == ProofStatusPassed || input.ToStatus == ProofStatusFailed {
		if observedAt == nil {
			observedAt = &now
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE proofs
		SET status = ?, subject_digest = ?, rationale = ?, actor_id = ?, observed_at = ?, updated_at = ?
		WHERE workspace_id = ? AND work_id = ? AND id = ? AND status = ?
	`, input.ToStatus, subjectDigest, rationale, input.ActorID, nullableTime(observedAt), now.UnixMilli(),
		input.WorkspaceID, input.WorkID, input.ProofID, input.ExpectedStatus); err != nil {
		return Proof{}, fmt.Errorf("workstore: transition proof: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, eventInput{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID,
		StepID: proof.StepID, AttemptID: proof.AttemptID,
		Type: EventTypeProofTransitioned, FromState: WorkState(input.ExpectedStatus), ToState: WorkState(input.ToStatus),
		ActorID: input.ActorID, CausationID: proof.ID,
		PayloadJSON: mustJSON(map[string]any{
			"proof_id": proof.ID, "from_status": input.ExpectedStatus,
			"to_status": input.ToStatus, "rationale": rationale,
		}), CreatedAt: now,
	}); err != nil {
		return Proof{}, err
	}
	proof, err = getProofTx(ctx, tx, input.WorkspaceID, input.WorkID, input.ProofID, "")
	if err != nil {
		return Proof{}, err
	}
	if err := tx.Commit(); err != nil {
		return Proof{}, fmt.Errorf("workstore: commit proof transition: %w", err)
	}
	return proof, nil
}

func (s *Store) DetectStaleProof(ctx context.Context, input DetectStaleProofInput) (Proof, bool, error) {
	if strings.TrimSpace(input.CurrentSubjectDigest) == "" {
		return Proof{}, false, fmt.Errorf("workstore: current proof subject digest is required")
	}
	proof, err := s.GetProof(ctx, input.WorkspaceID, input.WorkID, input.ProofID)
	if err != nil {
		return Proof{}, false, err
	}
	if proof.Status == ProofStatusStale || proof.SubjectDigest == input.CurrentSubjectDigest {
		return proof, false, nil
	}
	stale, err := s.TransitionProof(ctx, TransitionProofInput{
		WorkspaceID: input.WorkspaceID, WorkID: input.WorkID, ProofID: input.ProofID,
		ExpectedStatus: proof.Status, ToStatus: ProofStatusStale,
		ActorID: input.ActorID, Rationale: input.Rationale,
	})
	if err != nil {
		return Proof{}, false, err
	}
	return stale, true, nil
}

func (s *Store) ListEvents(ctx context.Context, workspaceID, workID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, eventSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY sequence", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: list events: %w", err)
	}
	defer closeRows(rows)
	return scanEvents(rows)
}

func (s *Store) GetWorkProjection(ctx context.Context, workspaceID, workID string) (WorkProjection, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return WorkProjection{}, fmt.Errorf("workstore: begin projection: %w", err)
	}
	defer rollback(tx)
	work, err := getWorkTx(ctx, tx, workspaceID, workID, "")
	if err != nil {
		return WorkProjection{}, err
	}
	projection := WorkProjection{Work: work}
	if projection.Steps, err = querySteps(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Schedules, err = queryStepSchedules(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Dependencies, err = queryDependencies(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Attempts, err = queryAttempts(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Events, err = queryEvents(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Approvals, err = queryApprovals(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Artifacts, err = queryArtifacts(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.Proofs, err = queryProofs(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if projection.EffectReceipts, err = queryEffectReceipts(ctx, tx, workspaceID, workID); err != nil {
		return WorkProjection{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkProjection{}, fmt.Errorf("workstore: commit projection read: %w", err)
	}
	return projection, nil
}

type eventInput struct {
	WorkspaceID    string
	WorkID         string
	StepID         string
	AttemptID      string
	Type           EventType
	FromState      WorkState
	ToState        WorkState
	ActorID        string
	CausationID    string
	IdempotencyKey string
	PayloadJSON    json.RawMessage
	CreatedAt      time.Time
}

func (s *Store) insertEventTx(ctx context.Context, tx *sql.Tx, input eventInput) error {
	id, err := s.newID("evt")
	if err != nil {
		return err
	}
	payload, err := normalizedJSON(input.PayloadJSON)
	if err != nil {
		return fmt.Errorf("workstore: event payload: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO events (
			schema_version, id, workspace_id, work_id, step_id, attempt_id,
			event_type, from_state, to_state, actor_id, causation_id,
			idempotency_key, payload_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, recordSchemaVersion, id, input.WorkspaceID, input.WorkID, nullableString(input.StepID),
		nullableString(input.AttemptID), input.Type, input.FromState, input.ToState,
		input.ActorID, input.CausationID, input.IdempotencyKey, payload, input.CreatedAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("workstore: insert %s event: %w", input.Type, err)
	}
	return nil
}

const workSelect = `SELECT
    schema_version, id, workspace_id, kind, source, source_id,
    idempotency_key, causation_id, COALESCE(parent_work_id, ''), title, objective,
    contract_json, metadata_json, state, priority, actor_id, version,
    created_at, updated_at, completed_at
FROM works`

type scanner interface {
	Scan(dest ...any) error
}

func scanWork(row scanner) (Work, error) {
	var work Work
	var contractJSON, metadataJSON jsonValue
	var createdAt, updatedAt int64
	var completedAt sql.NullInt64
	err := row.Scan(
		&work.SchemaVersion, &work.ID, &work.WorkspaceID, &work.Kind, &work.Source,
		&work.SourceID, &work.IdempotencyKey, &work.CausationID, &work.ParentWorkID,
		&work.Title, &work.Objective, &contractJSON, &metadataJSON, &work.State,
		&work.Priority, &work.ActorID, &work.Version, &createdAt, &updatedAt, &completedAt,
	)
	if err != nil {
		return Work{}, err
	}
	work.CreatedAt = time.UnixMilli(createdAt).UTC()
	work.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	work.CompletedAt = timeFromNull(completedAt)
	work.ContractJSON = json.RawMessage(contractJSON)
	work.MetadataJSON = json.RawMessage(metadataJSON)
	return work, nil
}

func getWorkTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, idempotencyKey string) (Work, error) {
	query := workSelect + " WHERE workspace_id = ? AND id = ?"
	args := []any{workspaceID, workID}
	if idempotencyKey != "" {
		query = workSelect + " WHERE workspace_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, idempotencyKey}
	}
	work, err := scanWork(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, ErrNotFound
	}
	if err != nil {
		return Work{}, fmt.Errorf("workstore: query work: %w", err)
	}
	return work, nil
}

const stepSelect = `SELECT
    schema_version, id, workspace_id, work_id, COALESCE(parent_step_id, ''),
    idempotency_key, causation_id, title, description, state, position, actor_id,
    version, created_at, updated_at, completed_at
FROM steps`

func scanStep(row scanner) (Step, error) {
	var step Step
	var createdAt, updatedAt int64
	var completedAt sql.NullInt64
	err := row.Scan(&step.SchemaVersion, &step.ID, &step.WorkspaceID, &step.WorkID,
		&step.ParentStepID, &step.IdempotencyKey, &step.CausationID, &step.Title,
		&step.Description, &step.State, &step.Position, &step.ActorID, &step.Version,
		&createdAt, &updatedAt, &completedAt)
	if err != nil {
		return Step{}, err
	}
	step.CreatedAt = time.UnixMilli(createdAt).UTC()
	step.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	step.CompletedAt = timeFromNull(completedAt)
	return step, nil
}

func getStepTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, stepID, idempotencyKey string) (Step, error) {
	query := stepSelect + " WHERE workspace_id = ? AND work_id = ? AND id = ?"
	args := []any{workspaceID, workID, stepID}
	if idempotencyKey != "" {
		query = stepSelect + " WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, workID, idempotencyKey}
	}
	step, err := scanStep(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Step{}, ErrNotFound
	}
	if err != nil {
		return Step{}, fmt.Errorf("workstore: query step: %w", err)
	}
	return step, nil
}

const attemptSelect = `SELECT
    schema_version, id, workspace_id, work_id, COALESCE(step_id, ''),
    idempotency_key, causation_id, attempt_number, adapter, status, actor_id,
    input_json, output_json, error_text, started_at, finished_at, created_at, updated_at
FROM attempts`

func scanAttempt(row scanner) (Attempt, error) {
	var attempt Attempt
	var inputJSON, outputJSON jsonValue
	var startedAt, createdAt, updatedAt int64
	var finishedAt sql.NullInt64
	err := row.Scan(&attempt.SchemaVersion, &attempt.ID, &attempt.WorkspaceID, &attempt.WorkID,
		&attempt.StepID, &attempt.IdempotencyKey, &attempt.CausationID, &attempt.Number,
		&attempt.Adapter, &attempt.Status, &attempt.ActorID, &inputJSON,
		&outputJSON, &attempt.ErrorText, &startedAt, &finishedAt, &createdAt, &updatedAt)
	if err != nil {
		return Attempt{}, err
	}
	attempt.StartedAt = time.UnixMilli(startedAt).UTC()
	attempt.FinishedAt = timeFromNull(finishedAt)
	attempt.CreatedAt = time.UnixMilli(createdAt).UTC()
	attempt.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	attempt.InputJSON = json.RawMessage(inputJSON)
	attempt.OutputJSON = json.RawMessage(outputJSON)
	return attempt, nil
}

func getAttemptTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, attemptID, idempotencyKey string) (Attempt, error) {
	query := attemptSelect + " WHERE workspace_id = ? AND work_id = ? AND id = ?"
	args := []any{workspaceID, workID, attemptID}
	if idempotencyKey != "" {
		query = attemptSelect + " WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, workID, idempotencyKey}
	}
	attempt, err := scanAttempt(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, ErrNotFound
	}
	if err != nil {
		return Attempt{}, fmt.Errorf("workstore: query attempt: %w", err)
	}
	return attempt, nil
}

const approvalSelect = `SELECT
    schema_version, id, workspace_id, work_id, COALESCE(step_id, ''),
    COALESCE(attempt_id, ''), idempotency_key, causation_id, authority, status,
    request, reason, actor_id, reviewer_id, expires_at, decided_at, created_at, updated_at
FROM approvals`

func scanApproval(row scanner) (Approval, error) {
	var approval Approval
	var expiresAt, decidedAt sql.NullInt64
	var createdAt, updatedAt int64
	err := row.Scan(&approval.SchemaVersion, &approval.ID, &approval.WorkspaceID,
		&approval.WorkID, &approval.StepID, &approval.AttemptID, &approval.IdempotencyKey,
		&approval.CausationID, &approval.Authority, &approval.Status, &approval.Request,
		&approval.Reason, &approval.ActorID, &approval.ReviewerID, &expiresAt, &decidedAt,
		&createdAt, &updatedAt)
	if err != nil {
		return Approval{}, err
	}
	approval.ExpiresAt = timeFromNull(expiresAt)
	approval.DecidedAt = timeFromNull(decidedAt)
	approval.CreatedAt = time.UnixMilli(createdAt).UTC()
	approval.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return approval, nil
}

func getApprovalTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, approvalID, idempotencyKey string) (Approval, error) {
	query := approvalSelect + " WHERE workspace_id = ? AND work_id = ? AND id = ?"
	args := []any{workspaceID, workID, approvalID}
	if idempotencyKey != "" {
		query = approvalSelect + " WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, workID, idempotencyKey}
	}
	approval, err := scanApproval(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Approval{}, ErrNotFound
	}
	if err != nil {
		return Approval{}, fmt.Errorf("workstore: query approval: %w", err)
	}
	return approval, nil
}

const artifactSelect = `SELECT
    schema_version, id, workspace_id, work_id, COALESCE(step_id, ''),
    COALESCE(attempt_id, ''), idempotency_key, causation_id, kind, name, uri,
    digest, media_type, size_bytes, actor_id, created_at
FROM artifacts`

func scanArtifact(row scanner) (Artifact, error) {
	var artifact Artifact
	var createdAt int64
	err := row.Scan(&artifact.SchemaVersion, &artifact.ID, &artifact.WorkspaceID,
		&artifact.WorkID, &artifact.StepID, &artifact.AttemptID, &artifact.IdempotencyKey,
		&artifact.CausationID, &artifact.Kind, &artifact.Name, &artifact.URI,
		&artifact.Digest, &artifact.MediaType, &artifact.SizeBytes, &artifact.ActorID, &createdAt)
	if err != nil {
		return Artifact{}, err
	}
	artifact.CreatedAt = time.UnixMilli(createdAt).UTC()
	return artifact, nil
}

func getArtifactTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, artifactID, idempotencyKey string) (Artifact, error) {
	query := artifactSelect + " WHERE workspace_id = ? AND work_id = ? AND id = ?"
	args := []any{workspaceID, workID, artifactID}
	if idempotencyKey != "" {
		query = artifactSelect + " WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, workID, idempotencyKey}
	}
	artifact, err := scanArtifact(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, ErrNotFound
	}
	if err != nil {
		return Artifact{}, fmt.Errorf("workstore: query artifact: %w", err)
	}
	return artifact, nil
}

const proofSelect = `SELECT
    schema_version, id, workspace_id, work_id, COALESCE(step_id, ''),
    COALESCE(attempt_id, ''), idempotency_key, causation_id, kind, status,
    origin, summary, reporter_id, verifier_id, verifier, command,
    COALESCE(artifact_id, ''), environment_json, input_json,
    artifact_digests_json, subject_digest, rationale, actor_id, observed_at,
    created_at, updated_at
FROM proofs`

func scanProof(row scanner) (Proof, error) {
	var proof Proof
	var environmentJSON, inputJSON, artifactDigestsJSON jsonValue
	var observedAt sql.NullInt64
	var createdAt, updatedAt int64
	err := row.Scan(&proof.SchemaVersion, &proof.ID, &proof.WorkspaceID, &proof.WorkID,
		&proof.StepID, &proof.AttemptID, &proof.IdempotencyKey, &proof.CausationID,
		&proof.Kind, &proof.Status, &proof.Origin, &proof.Summary, &proof.ReporterID,
		&proof.VerifierID, &proof.Verifier, &proof.Command, &proof.ArtifactID,
		&environmentJSON, &inputJSON, &artifactDigestsJSON, &proof.SubjectDigest,
		&proof.Rationale, &proof.ActorID, &observedAt, &createdAt, &updatedAt)
	if err != nil {
		return Proof{}, err
	}
	proof.EnvironmentJSON = append(json.RawMessage(nil), environmentJSON...)
	proof.InputJSON = append(json.RawMessage(nil), inputJSON...)
	proof.ArtifactDigestsJSON = append(json.RawMessage(nil), artifactDigestsJSON...)
	proof.ObservedAt = timeFromNull(observedAt)
	proof.CreatedAt = time.UnixMilli(createdAt).UTC()
	proof.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return proof, nil
}

func getProofTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, proofID, idempotencyKey string) (Proof, error) {
	query := proofSelect + " WHERE workspace_id = ? AND work_id = ? AND id = ?"
	args := []any{workspaceID, workID, proofID}
	if idempotencyKey != "" {
		query = proofSelect + " WHERE workspace_id = ? AND work_id = ? AND idempotency_key = ?"
		args = []any{workspaceID, workID, idempotencyKey}
	}
	proof, err := scanProof(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Proof{}, ErrNotFound
	}
	if err != nil {
		return Proof{}, fmt.Errorf("workstore: query proof: %w", err)
	}
	return proof, nil
}

const eventSelect = `SELECT
    schema_version, sequence, id, workspace_id, work_id, COALESCE(step_id, ''),
    COALESCE(attempt_id, ''), event_type, from_state, to_state, actor_id,
    causation_id, idempotency_key, payload_json, created_at
FROM events`

func scanEvent(row scanner) (Event, error) {
	var event Event
	var payloadJSON jsonValue
	var createdAt int64
	err := row.Scan(&event.SchemaVersion, &event.Sequence, &event.ID, &event.WorkspaceID,
		&event.WorkID, &event.StepID, &event.AttemptID, &event.Type, &event.FromState,
		&event.ToState, &event.ActorID, &event.CausationID, &event.IdempotencyKey,
		&payloadJSON, &createdAt)
	if err != nil {
		return Event{}, err
	}
	event.CreatedAt = time.UnixMilli(createdAt).UTC()
	event.PayloadJSON = json.RawMessage(payloadJSON)
	return event, nil
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workstore: iterate events: %w", err)
	}
	return events, nil
}

func querySteps(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]Step, error) {
	rows, err := tx.QueryContext(ctx, stepSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY position, created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection steps: %w", err)
	}
	defer closeRows(rows)
	var steps []Step
	for rows.Next() {
		step, err := scanStep(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection step: %w", err)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func queryDependencies(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]StepDependency, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT workspace_id, work_id, step_id, depends_on_step_id, actor_id, created_at
		FROM step_dependencies
		WHERE workspace_id = ? AND work_id = ?
		ORDER BY step_id, depends_on_step_id
	`, workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection dependencies: %w", err)
	}
	defer closeRows(rows)
	var dependencies []StepDependency
	for rows.Next() {
		var dependency StepDependency
		var createdAt int64
		if err := rows.Scan(&dependency.WorkspaceID, &dependency.WorkID, &dependency.StepID,
			&dependency.DependsOnID, &dependency.ActorID, &createdAt); err != nil {
			return nil, fmt.Errorf("workstore: scan projection dependency: %w", err)
		}
		dependency.CreatedAt = time.UnixMilli(createdAt).UTC()
		dependencies = append(dependencies, dependency)
	}
	return dependencies, rows.Err()
}

func queryAttempts(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]Attempt, error) {
	rows, err := tx.QueryContext(ctx, attemptSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection attempts: %w", err)
	}
	defer closeRows(rows)
	var attempts []Attempt
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection attempt: %w", err)
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func queryEvents(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]Event, error) {
	rows, err := tx.QueryContext(ctx, eventSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY sequence", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection events: %w", err)
	}
	defer closeRows(rows)
	return scanEvents(rows)
}

func queryApprovals(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]Approval, error) {
	rows, err := tx.QueryContext(ctx, approvalSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection approvals: %w", err)
	}
	defer closeRows(rows)
	var approvals []Approval
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection approval: %w", err)
		}
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func queryArtifacts(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]Artifact, error) {
	rows, err := tx.QueryContext(ctx, artifactSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection artifacts: %w", err)
	}
	defer closeRows(rows)
	var artifacts []Artifact
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

func queryProofs(ctx context.Context, tx *sql.Tx, workspaceID, workID string) ([]Proof, error) {
	rows, err := tx.QueryContext(ctx, proofSelect+" WHERE workspace_id = ? AND work_id = ? ORDER BY created_at, id", workspaceID, workID)
	if err != nil {
		return nil, fmt.Errorf("workstore: query projection proofs: %w", err)
	}
	defer closeRows(rows)
	var proofs []Proof
	for rows.Next() {
		proof, err := scanProof(rows)
		if err != nil {
			return nil, fmt.Errorf("workstore: scan projection proof: %w", err)
		}
		proofs = append(proofs, proof)
	}
	return proofs, rows.Err()
}

func ensureWorkTx(ctx context.Context, tx *sql.Tx, workspaceID, workID string) error {
	var found int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM works WHERE workspace_id = ? AND id = ?", workspaceID, workID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("workstore: validate work reference: %w", err)
	}
	return nil
}

func ensureStepTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, stepID string) error {
	var found int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM steps WHERE workspace_id = ? AND work_id = ? AND id = ?", workspaceID, workID, stepID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("workstore: validate step reference: %w", err)
	}
	return nil
}

func ensureReferencesTx(ctx context.Context, tx *sql.Tx, workspaceID, workID, stepID, attemptID string) error {
	if err := ensureWorkTx(ctx, tx, workspaceID, workID); err != nil {
		return err
	}
	if stepID != "" {
		if err := ensureStepTx(ctx, tx, workspaceID, workID, stepID); err != nil {
			return err
		}
	}
	if attemptID != "" {
		var foundStep sql.NullString
		err := tx.QueryRowContext(ctx, `
			SELECT step_id FROM attempts WHERE workspace_id = ? AND work_id = ? AND id = ?
		`, workspaceID, workID, attemptID).Scan(&foundStep)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("workstore: validate attempt reference: %w", err)
		}
		if stepID != "" && foundStep.String != stepID {
			return fmt.Errorf("workstore: attempt does not belong to step: %w", ErrInvalidDependency)
		}
	}
	return nil
}

func validateCreateWork(input CreateWorkInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.ActorID) == "" {
		return fmt.Errorf("workstore: workspace, kind, idempotency key, title, and actor are required")
	}
	if input.InitialState != "" && !validWorkState(input.InitialState) {
		return fmt.Errorf("workstore: invalid initial state %q", input.InitialState)
	}
	return nil
}

func validateCreateStep(input CreateStepInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.ActorID) == "" {
		return fmt.Errorf("workstore: workspace, work, idempotency key, title, and actor are required")
	}
	if input.State != "" && !validWorkState(input.State) {
		return fmt.Errorf("workstore: invalid step state %q", input.State)
	}
	return nil
}

func validateCreateAttempt(input CreateAttemptInput) error {
	if strings.TrimSpace(input.WorkspaceID) == "" || strings.TrimSpace(input.WorkID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.Adapter) == "" || strings.TrimSpace(input.ActorID) == "" || input.Number <= 0 || !validAttemptStatus(input.Status) {
		return fmt.Errorf("workstore: invalid attempt input")
	}
	return nil
}

func validWorkState(state WorkState) bool {
	switch state {
	case WorkStateTriage, WorkStateBacklog, WorkStateTodo, WorkStateReady,
		WorkStateRunning, WorkStateReview, WorkStateBlocked, WorkStateDone,
		WorkStateCancelled:
		return true
	default:
		return false
	}
}

func validAttemptStatus(status AttemptStatus) bool {
	switch status {
	case AttemptStatusPending, AttemptStatusRunning, AttemptStatusSucceeded,
		AttemptStatusFailed, AttemptStatusCancelled:
		return true
	default:
		return false
	}
}

func validApprovalStatus(status ApprovalStatus) bool {
	switch status {
	case ApprovalStatusPending, ApprovalStatusApproved, ApprovalStatusDenied, ApprovalStatusExpired:
		return true
	default:
		return false
	}
}

func validProofStatus(status ProofStatus) bool {
	switch status {
	case ProofStatusReported, ProofStatusPending, ProofStatusPassed, ProofStatusFailed, ProofStatusStale:
		return true
	default:
		return false
	}
}

func validProofOrigin(origin ProofOrigin) bool {
	switch origin {
	case ProofOriginWorkerReport, ProofOriginIndependentVerifier, ProofOriginLegacy:
		return true
	default:
		return false
	}
}

func canTransitionProof(from, to ProofStatus) bool {
	switch from {
	case ProofStatusReported:
		return to == ProofStatusPending || to == ProofStatusStale
	case ProofStatusPending:
		return to == ProofStatusPassed || to == ProofStatusFailed || to == ProofStatusStale
	case ProofStatusPassed, ProofStatusFailed:
		return to == ProofStatusStale
	case ProofStatusStale:
		return to == ProofStatusPending
	default:
		return false
	}
}

func proofObservedAt(status ProofStatus, observedAt *time.Time, now time.Time) *time.Time {
	if observedAt != nil {
		value := observedAt.UTC()
		return &value
	}
	if status == ProofStatusPending {
		return nil
	}
	value := now.UTC()
	return &value
}

func canTransition(from, to WorkState) bool {
	allowed := map[WorkState]map[WorkState]bool{
		WorkStateTriage: {
			WorkStateBacklog: true, WorkStateTodo: true, WorkStateCancelled: true,
		},
		WorkStateBacklog: {
			WorkStateTodo: true, WorkStateCancelled: true,
		},
		WorkStateTodo: {
			WorkStateReady: true, WorkStateRunning: true, WorkStateBlocked: true, WorkStateCancelled: true,
		},
		WorkStateReady: {
			WorkStateRunning: true, WorkStateBlocked: true, WorkStateCancelled: true,
		},
		WorkStateRunning: {
			WorkStateReview: true, WorkStateBlocked: true, WorkStateTodo: true, WorkStateDone: true, WorkStateCancelled: true,
		},
		WorkStateReview: {
			WorkStateDone: true, WorkStateRunning: true, WorkStateBlocked: true, WorkStateCancelled: true,
		},
		WorkStateBlocked: {
			WorkStateTodo: true, WorkStateReady: true, WorkStateCancelled: true,
		},
	}
	return allowed[from][to]
}

func normalizedJSON(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(value) {
		return nil, fmt.Errorf("invalid json")
	}
	return value, nil
}

type jsonValue []byte

func (value *jsonValue) Scan(src any) error {
	switch typed := src.(type) {
	case nil:
		*value = nil
	case []byte:
		*value = append((*value)[:0], typed...)
	case string:
		*value = append((*value)[:0], typed...)
	default:
		return fmt.Errorf("workstore: unsupported json value %T", src)
	}
	return nil
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().UnixMilli()
}

func timeFromNull(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed := time.UnixMilli(value.Int64).UTC()
	return &parsed
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}
