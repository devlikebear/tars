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
EXCLUDED_PACKAGES=(
  github.com/devlikebear/tars/internal/agentruntime    # command executor timeouts and effect receipt file modes
  github.com/devlikebear/tars/pkg/llm                  # claude-code-cli tests drive POSIX shell script stubs (was internal/llm before #928)
  github.com/devlikebear/tars/internal/tarsserver      # ~21 tests: sandboxes and notifiers that shell out, owner-only config files, symlinks, sqlite past Close; focus PR poll tests flake
)

# Individual tests in packages that otherwise pass. Verified causes:
#   * mode 0600 assertions — Windows Chmod only toggles the read-only bit, so
#     a file's mode always reads back as 0666.
#   * read-only directory negative tests — Windows still permits creating
#     files inside a directory marked read-only.
#   * symlink creation — needs Developer Mode or elevation on Windows.
#   * POSIX shell script stubs — a `#!/bin/sh` file without an .exe
#     extension is not an executable on Windows.
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
  TestCheckDoctorLLMRuntime_ClaudeCodeCLI                                      # POSIX shell stub
  TestRemoteAccessCLIRendersLiveStatusURLAndOwnedMutations                     # POSIX shell stub
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
