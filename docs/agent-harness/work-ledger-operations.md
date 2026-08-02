# Work Ledger operations

The Work Ledger is a local SQLite database opened through `internal/workstore`.
It uses WAL mode, foreign-key enforcement, full synchronous writes, versioned
migrations, and `0600` database permissions.

## Safety rules

- Keep the active database and every backup on a trusted local filesystem.
- Backup, restore, JSONL export, and quarantine operations never overwrite an
  existing destination. Choose a new path or move the existing file yourself.
- A restore target must not be the active database path.
- Large outputs remain external artifacts referenced by URI and digest; they
  are not copied into SQLite unless an operator explicitly archives them.
- A quarantined source is copied, not moved. The original remains in place for
  forensic review and manual recovery.

## Create and verify a backup

Call `Store.Backup(ctx, destination)`. The operation serializes local writers
and uses SQLite `VACUUM INTO`, so committed records still present in the WAL are
included. It validates `quick_check`, foreign keys, and migration checksums
before publishing the backup atomically with `0600` permissions.

Persist the returned SHA-256 digest with the backup. A destination collision is
reported as `ErrDestinationExists`; it is never treated as permission to
replace the existing backup.

## Restore a backup

Call `workstore.RestoreBackup(ctx, source, newTarget, options)` while the target
path does not exist. Restore performs these checks before publishing the new
database:

1. SQLite can open the source read-only.
2. `PRAGMA quick_check` reports `ok`.
3. `PRAGMA foreign_key_check` reports no violations.
4. Known migration checksums match and no unknown future migration exists.

The restored copy is opened once through the normal Work Ledger migration path
and then closed. Start TARS with the new target only after comparing the
reported digest and running `Store.Doctor`.

## Export audit data as JSONL

Call `Store.ExportJSONL(ctx, workspaceID, destination)`. Records are written in
a deterministic order as a header followed by Work, Step, dependency, Attempt,
Event, Approval, Artifact, Proof, and import-marker envelopes. The final line
contains a SHA-256 digest over every preceding line plus per-type record counts.

The export is a portable audit artifact, not an in-place database replacement.
Use a SQLite backup for operational restore. Verify a JSONL file by hashing all
bytes before the `checksum` footer and comparing the hexadecimal digest.

## Run doctor

Call `Store.Doctor(ctx, workspaceID)` before and after migrations, restores, or
manual incident recovery. A healthy report requires:

- SQLite quick-check and foreign-key checks to pass;
- every applied migration checksum to match the binary;
- stored JSON documents to decode;
- terminal Work, Step, and Attempt records to have terminal timestamps;
- the Step dependency graph to be acyclic; and
- every Work ID referenced by an import marker to exist in that workspace.

`Doctor` is read-only. It reports issue codes and record IDs but does not repair
or delete data automatically.

## Quarantine a corrupt legacy source

Call `Store.QuarantineSource` with the source kind, stable source ID, original
path, quarantine directory, reason, actor, and workspace. The operation writes:

- an exact `0600` copy named with timestamp and SHA-256 prefix;
- a JSON manifest containing provenance, digest, size, reason, and marker; and
- a `quarantined` import marker in the ledger.

Repeating the operation for the same source bytes returns the existing marker
and paths. If the source changes, its checksum changes and a separate quarantine
record is created. Never delete the original automatically; decide retention
only after the copy, manifest, and digest have been independently verified.
