package workstore

import (
	"context"
	"fmt"
	"strings"
)

// supersededRevisionVacuumThreshold is how many revisions one prune must
// remove before the file is compacted afterwards: the first prune of a
// ledger that kept every revision, not the one or two of a normal import.
const supersededRevisionVacuumThreshold = 100

// supersededRevisionGuards are the tables whose rows keep a superseded
// revision alive. An imported agent-run revision owns one step, one attempt
// and their events and nothing else; a revision something else hangs off
// (a proof, an artifact, a receipt, a schedule, a capability record) is not
// plain history and is left alone.
var supersededRevisionGuards = []string{
	"artifacts", "approvals", "step_schedules", "step_dependencies", "effect_receipts",
	"proofs", "capability_versions", "evaluation_runs", "capability_outcomes",
}

// PruneSupersededAgentRunRevisions deletes the ledger records of agent runs
// that a newer record of the same run has replaced. Every change of a run is
// imported as a new work, so without this a run that changed two hundred
// times keeps two hundred works, each frozen in the state it had: old
// revisions pile up as megabytes of run documents and as hundreds of works
// that still read "running". The newest revision of each run is kept, with
// everything attached to it.
//
// Import markers that name a deleted work are removed with it, so the ledger
// doctor still finds every work a marker refers to.
func (s *Store) PruneSupersededAgentRunRevisions(ctx context.Context, workspaceID string) (int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workstore: workspace id is required to prune run revisions")
	}
	return s.pruneAndCompact(ctx, workspaceID, agentRunRevisions)
}

// PruneSupersededSessionRevisions deletes the ledger records of session task
// documents that a newer import of the same session has replaced. Every save
// of a session's tasks is imported as a new work holding the whole document
// and one proof per evidence record, so a session saved a hundred times kept
// a hundred copies: 851 revisions of 99 sessions made an 11.8 GB ledger, and
// checking it took the server minutes to start. The session's task file is
// the source of these records and the newest revision is what readers are
// served (GetLegacySessionTasksProjection), so the older ones are copies of
// documents that no longer exist.
//
// A revision's imported steps, evidence artifacts, proofs and events go with
// it. A revision anything else was attached to afterwards (an attempt, an
// approval, a schedule, a receipt, a capability record, an evaluation of one
// of its proofs, a child work) is kept.
func (s *Store) PruneSupersededSessionRevisions(ctx context.Context, workspaceID string) (int, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return 0, fmt.Errorf("workstore: workspace id is required to prune session revisions")
	}
	return s.pruneAndCompact(ctx, workspaceID, sessionRevisions)
}

// DeleteSessionRevisions deletes every ledger record imported from one
// session's task file, the newest included: the session was deleted, so no
// reader will ask for it and its source file is gone. The guards of
// PruneSupersededSessionRevisions apply here too — a revision something was
// attached to afterwards stays.
func (s *Store) DeleteSessionRevisions(ctx context.Context, workspaceID, sessionID string) (int, error) {
	workspaceID, sessionID = strings.TrimSpace(workspaceID), strings.TrimSpace(sessionID)
	if workspaceID == "" || sessionID == "" {
		return 0, fmt.Errorf("workstore: workspace id and session id are required to delete session revisions")
	}
	return s.pruneRevisions(ctx, workspaceID, sessionID, sessionRevisions, 1)
}

func (s *Store) pruneAndCompact(ctx context.Context, workspaceID string, revisions supersededRevisions) (int, error) {
	pruned, err := s.pruneSupersededRevisions(ctx, workspaceID, "", revisions)
	if err != nil {
		return 0, err
	}
	if pruned >= supersededRevisionVacuumThreshold {
		// Best effort, as after migration 8: without it the file keeps its
		// size and the freed pages are reused.
		s.compact(ctx)
	}
	return pruned, nil
}

// supersededRevisions describes one imported source whose works are
// revisions of the same record.
type supersededRevisions struct {
	source ImportSourceKind
	// guards are the tables whose rows keep a superseded revision alive.
	guards []string
	// owned are the tables an import fills for a revision, children before
	// parents: events point at steps and attempts, proofs at artifacts,
	// attempts at steps, steps at works.
	owned []string
}

var agentRunRevisions = supersededRevisions{
	source: ImportSourceAgentRuntime,
	guards: supersededRevisionGuards,
	owned:  []string{"events", "attempts", "steps"},
}

var sessionRevisions = supersededRevisions{
	source: ImportSourceLegacySession,
	guards: []string{
		"attempts", "approvals", "step_schedules", "step_dependencies", "effect_receipts",
		"capability_versions", "evaluation_runs", "capability_outcomes",
	},
	owned: []string{"events", "proofs", "artifacts", "steps"},
}

// pruneSupersededRevisions deletes every revision but the newest of each
// record of one source, or of the single record sourceID when it is set.
func (s *Store) pruneSupersededRevisions(ctx context.Context, workspaceID, sourceID string, revisions supersededRevisions) (int, error) {
	return s.pruneRevisions(ctx, workspaceID, sourceID, revisions, 2)
}

// pruneRevisions deletes the revisions ranked fromRank and older, newest
// first: 2 keeps the newest, 1 keeps none.
func (s *Store) pruneRevisions(ctx context.Context, workspaceID, sourceID string, revisions supersededRevisions, fromRank int) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("workstore: begin prune %s revisions: %w", revisions.source, err)
	}
	defer rollback(tx)

	selectArgs := []any{workspaceID, revisions.source}
	onlySource := ""
	if sourceID != "" {
		onlySource = " AND source_id = ?"
		selectArgs = append(selectArgs, sourceID)
	}
	selectArgs = append(selectArgs, fromRank)
	selectSuperseded := `
		INSERT INTO pruned_revisions (id)
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY source_id ORDER BY created_at DESC, rowid DESC
			) AS revision_rank
			FROM works
			WHERE workspace_id = ? AND source = ?` + onlySource + `
		) AS revision
		WHERE revision_rank >= ?
			AND NOT EXISTS (SELECT 1 FROM works child WHERE child.parent_work_id = revision.id)
			AND NOT EXISTS (
				SELECT 1 FROM evaluation_runs evaluation
				JOIN proofs evaluated ON evaluated.id = evaluation.proof_id
				WHERE evaluated.work_id = revision.id
			)`
	for _, table := range revisions.guards {
		selectSuperseded += "\n\t\t\tAND NOT EXISTS (SELECT 1 FROM " + table + " guard WHERE guard.work_id = revision.id)"
	}
	type statement struct {
		sql  string
		args []any
	}
	statements := []statement{
		{sql: "CREATE TEMP TABLE IF NOT EXISTS pruned_revisions (id TEXT PRIMARY KEY)"},
		{sql: "DELETE FROM pruned_revisions"},
		{sql: selectSuperseded, args: selectArgs},
	}
	for _, table := range revisions.owned {
		statements = append(statements, statement{sql: "DELETE FROM " + table + " WHERE work_id IN (SELECT id FROM pruned_revisions)"})
	}
	statements = append(statements,
		statement{sql: "DELETE FROM works WHERE id IN (SELECT id FROM pruned_revisions)"},
		// Import markers that name a deleted work go with it, so the doctor
		// still finds every work a marker refers to.
		statement{sql: `
			DELETE FROM import_markers
			WHERE workspace_id = ? AND source_kind = ?
				AND EXISTS (
					SELECT 1 FROM json_each(CAST(import_markers.work_ids_json AS TEXT)) AS named
					WHERE named.value IN (SELECT id FROM pruned_revisions)
				)`, args: []any{workspaceID, revisions.source}},
	)
	for _, step := range statements {
		if _, err := tx.ExecContext(ctx, step.sql, step.args...); err != nil {
			return 0, fmt.Errorf("workstore: prune superseded %s revisions: %w", revisions.source, err)
		}
	}
	var pruned int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM pruned_revisions").Scan(&pruned); err != nil {
		return 0, fmt.Errorf("workstore: count pruned %s revisions: %w", revisions.source, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("workstore: commit prune %s revisions: %w", revisions.source, err)
	}
	return pruned, nil
}
