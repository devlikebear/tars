# ADR: Durable Work Ledger storage

- Status: Accepted for Phase 1 implementation
- Decision date: 2026-08-02
- Scope: [OH-002 / #905](https://github.com/devlikebear/tars/issues/905) and the recovery/effect phases that follow it

## Context

TARS currently persists related state in several filesystem shapes: session task JSON, Agent Runtime `runs.json` / `channels.json` snapshots, cron state, approval records, and delivery-specific data. Atomic replacement protects an individual file from partial writes, but the system cannot atomically claim work, transition several related records, enforce an idempotency key, or query one lifecycle across triggers and runtimes.

The control-plane roadmap needs a durable unit of work with:

- transactional state transitions;
- dependency and attempt history;
- leases and heartbeats for restart recovery;
- approval and proof references;
- unique effect/idempotency receipts;
- indexed operator queries; and
- schema migration and backup behavior that remains compatible with a single Go binary.

Two candidate storage designs were evaluated.

## Options

### A. Append-only file journal plus snapshots

Advantages:

- no new database dependency;
- easy manual inspection with ordinary text tools;
- naturally preserves an event history; and
- fits the existing filesystem-first mental model.

Costs and risks:

- multi-process writer coordination and compare-and-swap claims require a custom lock protocol;
- uniqueness constraints for work keys and effect receipts must be reimplemented;
- every query needs replay, secondary indexes, or additional snapshots that can drift;
- compaction, torn-tail handling, schema evolution, and snapshot/journal reconciliation become bespoke database work; and
- atomicity across work, dependency, attempt, approval, and receipt records is difficult to prove.

### B. Pure-Go SQLite in WAL mode

Advantages:

- transactions, uniqueness constraints, foreign keys, indexed queries, and migrations are built-in primitives;
- WAL supports concurrent readers while a single scheduler writer claims or updates work;
- lease claims and effect receipts can use conditional updates and unique indexes instead of file locks;
- one database can expose a coherent lifecycle while still allowing export to JSONL/Markdown for inspection; and
- a pure-Go driver preserves cross-compilation and the no-CGO release path.

Costs and risks:

- a pure-Go SQLite driver increases dependency and binary size;
- WAL files require correct backup/checkpoint handling rather than copying only the main database file;
- write contention needs a bounded busy timeout and short transactions; and
- migrations become a release-critical compatibility surface.

## Decision

Use **pure-Go SQLite in WAL mode** for the Durable Work Ledger. The intended driver is `modernc.org/sqlite` unless an implementation spike demonstrates a blocking compatibility or size problem.

The database belongs under the shared workspace control-plane directory, not inside a chat session. The Phase 1 implementation must set and verify:

- `journal_mode=WAL`;
- `foreign_keys=ON`;
- a bounded `busy_timeout`;
- explicit transaction isolation for claim/update paths; and
- file permissions consistent with other local TARS state.

SQLite is the authoritative materialized state. An append-only `work_events` table records lifecycle events inside the same transaction as the state transition. Human-readable JSONL/Markdown is an export, not a second source of truth.

## Initial schema boundary

The first migration should keep the schema narrow:

- `work_items`: stable ID, idempotency key, kind, source, state, priority, policy/budget references, timestamps, lease owner/expiry, terminal reason;
- `work_dependencies`: prerequisite edges;
- `work_attempts`: execution adapter, attempt number, start/finish, result/error, usage;
- `work_events`: ordered state-transition/audit records;
- `work_approvals`: requested authority, decision, reviewer, expiry;
- `effect_receipts`: idempotency key, effect type/target, request digest, outcome, external reference; and
- `proof_records`: verifier, command/artifact digest, outcome, captured evidence reference.

Large transcripts, artifacts, diffs, and tool output remain in existing bounded files or artifact storage. The ledger stores references and digests rather than becoming a blob store.

## Recovery semantics

- A scheduler claims queued work with a conditional transaction that sets a lease owner and expiry.
- Heartbeats extend only the current attempt's lease.
- On startup, an expired `running` lease becomes recoverable work; the scheduler consults the execution adapter and effect receipts before retrying.
- A unique work idempotency key prevents duplicate enqueue.
- A unique effect idempotency key prevents a replay from repeating an already committed external effect.
- Terminal state and its final event/proof reference commit in the same transaction.
- If SQLite reports corruption or a migration would lose columns/data, startup fails closed and preserves the original database for recovery.

## Migration and rollback

1. Create the database and migrations without deleting or rewriting existing task/runtime files.
2. Dual-read existing session Tasks and Agent Runtime history into the operator view; new durable work writes only to SQLite.
3. Provide an idempotent importer that records source path plus source record ID so re-running migration cannot duplicate work.
4. Keep legacy files untouched for at least one minor release and export a pre-migration backup manifest.
5. Rollback disables new ledger writes and returns to legacy readers; it never down-migrates or destructively rewrites the database.
6. Remove legacy write paths only in a later, separately reviewed migration after usage and recovery evidence is stable.

## Backup and operational requirements

- Use SQLite's online backup API or a checkpointed snapshot, never copy only the main `.db` while WAL writes are active.
- Validate a restored backup in CI with schema and row-count checks.
- Bound event retention/compaction without deleting current work, latest terminal state, effect receipts, or proof records.
- Expose health fields for schema version, WAL mode, last successful checkpoint, migration status, and recovery counts.

## Consequences

Phase 1 accepts a larger binary and a new critical dependency in exchange for substantially less custom durability code and enforceable recovery/idempotency invariants. TARS remains locally inspectable through CLI/API queries and deterministic exports. The existing atomic file writer remains the right tool for independent Markdown, config, transcript, and report artifacts; it is no longer stretched into a transactional work database.
