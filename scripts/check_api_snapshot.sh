#!/usr/bin/env bash
set -euo pipefail

# Fail when the checked-in public API snapshot is stale.
#
# The point is not to prevent a change — semver v0 permits one — but to make
# it visible in review. A PR that alters what an external consumer can rely on
# must carry the regenerated file, so the diff shows exactly which identifiers
# moved.

GO="${GO:-go}"
snapshot="docs/public-api-surface.txt"

if [[ ! -f "${snapshot}" ]]; then
  echo "[api-snapshot] ${snapshot} is missing; run: make api-snapshot" >&2
  exit 1
fi

actual="$(mktemp)"
trap 'rm -f "${actual}"' EXIT

"${GO}" run ./cmd/apisnapshot > "${actual}"

if diff -u "${snapshot}" "${actual}" > /dev/null 2>&1; then
  echo "[api-snapshot] public API surface matches ($(grep -cvE '^#|^$' "${snapshot}") identifiers)"
  exit 0
fi

echo "[api-snapshot] the public API surface of pkg/* changed." >&2
echo >&2
echo "  Lines prefixed '-' were removed or renamed — those break external consumers." >&2
echo "  Lines prefixed '+' are additions, which are safe to add." >&2
echo >&2
diff -u "${snapshot}" "${actual}" | tail -n +3 >&2
echo >&2
echo "  If the change is intended, run 'make api-snapshot' and commit the result" >&2
echo "  in the same PR. See docs/public-agent-packages.md for the policy." >&2
exit 1
