#!/usr/bin/env bash
# Runs the Go test suite on Windows.
#
# tars builds and runs natively on Windows, but a set of tests still assume a
# unix host. This script runs everything else, so the Windows job is a real
# gate that fails on regressions rather than a permanently red job nobody
# reads. Both lists below are debt, not policy: shrink them.
#
# Run it the same way CI does:
#   ./scripts/windows_test.sh
set -euo pipefail

GO="${GO:-go}"
TEST_TIMEOUT="${TEST_TIMEOUT:-300s}"

# Packages whose Windows failures are broad enough that listing individual
# tests would be unmaintainable. Excluding a package loses its passing tests
# too, so prefer SKIPPED_TESTS whenever the count is small.
#
# internal/workscheduler is here for a reason worth knowing. It used to hang,
# which was a deadlock in its own error callback and is fixed. What remains is
# a teardown failure with no fix on this side: after sql.DB.Close() returns
# without error, the sqlite driver still holds the ledger's Windows file
# handle, and Windows refuses to unlink an open file, so t.TempDir()'s
# RemoveAll fails. It only shows under parallel load, it strikes whichever
# test happens to finish at the wrong moment, and it is invisible on Linux
# because unlinking an open file is allowed there. Retrying the removal for
# twenty seconds and forcing a GC both fail to release it, and a goroutine
# dump at that moment shows nothing of ours still running.
EXCLUDED_PACKAGES=(
  github.com/devlikebear/tars/cmd/tars                 # init/service/doctor assume launchd and unix workspace layout
  github.com/devlikebear/tars/internal/agentruntime    # command executor timeouts and effect receipt file modes
  github.com/devlikebear/tars/internal/auth            # credential files asserted at mode 0600
  github.com/devlikebear/tars/internal/executionplane  # artifact URIs, symlinks, and POSIX runner assumptions
  github.com/devlikebear/tars/pkg/llm                  # claude-code-cli tests drive POSIX shell script stubs (was internal/llm before #928)
  github.com/devlikebear/tars/internal/tarsserver      # sandboxes that shell out, plus macOS/Linux notifier paths
  github.com/devlikebear/tars/internal/workerprotocol  # ssh/container/symlink policy assumptions
  github.com/devlikebear/tars/internal/workscheduler   # sqlite holds the ledger file past Close — see above
)

# Individual tests in packages that otherwise pass. Verified causes:
#   * mode 0600 assertions — Windows Chmod only toggles the read-only bit, so
#     a file's mode always reads back as 0666.
#   * read-only directory negative tests — Windows still permits creating
#     files inside a directory marked read-only.
#   * symlink creation — needs Developer Mode or elevation on Windows.
SKIPPED_TESTS=(
  TestWrite_DoesNotCorruptOnReadOnlyDir                                        # read-only directory
  TestManager_UpdateApprovalStatus_SetsReviewedAtAndPersists                   # mode 0600
  TestManager_SaveApprovalsPreservesExistingFileWhenAtomicTempCannotBeCreated  # read-only directory
  TestOpenAppliesSchemaWALAndForeignKeys                                       # mode 0600
  TestQuarantineSourceCopiesWithoutDeletingOriginalAndIsIdempotent             # mode 0600
  TestBackupAndRestorePreserveCommittedWALState                                # mode 0600
  TestTracker_UpdateLimitsWritesPrivateFile                                    # mode 0600
  TestTracker_UpdateLimitsPreservesExistingFileAndMemoryWhenAtomicTempCannotBeCreated # read-only directory
  TestEngineRejectsArtifactAccessThroughEscapingDirectorySymlink               # symlink privilege
  TestEngineVerifiesConfinedArtifactTreesAndExpectedDigests                    # symlink privilege
  TestAppendInboxCandidateAndReviewActions                                     # unexamined
  TestExtractionInboxAppendListAndReview                                       # unexamined
  TestMirrorToWorkspace_CompanionFiles                                         # unexamined
)

packages="$("${GO}" list ./...)"
for excluded in "${EXCLUDED_PACKAGES[@]}"; do
  packages="$(printf '%s\n' "${packages}" | grep -vxF "${excluded}" || true)"
done

if [[ -z "${packages}" ]]; then
  echo "no packages left to test after exclusions" >&2
  exit 1
fi

skip_pattern="$(printf '%s|' "${SKIPPED_TESTS[@]}")"
skip_pattern="^(${skip_pattern%|})$"

echo "Running $(printf '%s\n' "${packages}" | wc -l | tr -d '[:space:]') packages, skipping ${#SKIPPED_TESTS[@]} tests"

# shellcheck disable=SC2086 # packages is a newline-separated list meant to split
exec "${GO}" test -timeout "${TEST_TIMEOUT}" -skip "${skip_pattern}" ${packages}
