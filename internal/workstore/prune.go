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
	pruned, err := s.pruneSupersededAgentRunRevisions(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	if pruned >= supersededRevisionVacuumThreshold {
		// Best effort, as after migration 8: without it the file keeps its
		// size and the freed pages are reused.
		_, _ = s.db.ExecContext(ctx, "VACUUM")
	}
	return pruned, nil
}

func (s *Store) pruneSupersededAgentRunRevisions(ctx context.Context, workspaceID string) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("workstore: begin prune run revisions: %w", err)
	}
	defer rollback(tx)

	selectSuperseded := `
		INSERT INTO pruned_revisions (id)
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY source_id ORDER BY created_at DESC, rowid DESC
			) AS revision_rank
			FROM works
			WHERE workspace_id = ? AND source = ?
		) AS revision
		WHERE revision_rank > 1
			AND NOT EXISTS (SELECT 1 FROM works child WHERE child.parent_work_id = revision.id)`
	for _, table := range supersededRevisionGuards {
		selectSuperseded += "\n\t\t\tAND NOT EXISTS (SELECT 1 FROM " + table + " guard WHERE guard.work_id = revision.id)"
	}
	type statement struct {
		sql  string
		args []any
	}
	statements := []statement{
		{sql: "CREATE TEMP TABLE IF NOT EXISTS pruned_revisions (id TEXT PRIMARY KEY)"},
		{sql: "DELETE FROM pruned_revisions"},
		{sql: selectSuperseded, args: []any{workspaceID, ImportSourceAgentRuntime}},
		// Children before parents: events point at steps and attempts,
		// attempts at steps, steps at works.
		{sql: "DELETE FROM events WHERE work_id IN (SELECT id FROM pruned_revisions)"},
		{sql: "DELETE FROM attempts WHERE work_id IN (SELECT id FROM pruned_revisions)"},
		{sql: "DELETE FROM steps WHERE work_id IN (SELECT id FROM pruned_revisions)"},
		{sql: "DELETE FROM works WHERE id IN (SELECT id FROM pruned_revisions)"},
		{sql: `
			DELETE FROM import_markers
			WHERE workspace_id = ? AND source_kind = ?
				AND EXISTS (
					SELECT 1 FROM json_each(CAST(import_markers.work_ids_json AS TEXT)) AS named
					WHERE named.value IN (SELECT id FROM pruned_revisions)
				)`, args: []any{workspaceID, ImportSourceAgentRuntime}},
	}
	for _, step := range statements {
		if _, err := tx.ExecContext(ctx, step.sql, step.args...); err != nil {
			return 0, fmt.Errorf("workstore: prune superseded run revisions: %w", err)
		}
	}
	var pruned int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM pruned_revisions").Scan(&pruned); err != nil {
		return 0, fmt.Errorf("workstore: count pruned run revisions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("workstore: commit prune run revisions: %w", err)
	}
	return pruned, nil
}
