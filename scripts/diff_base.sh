#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${DIFF_BASE:-}" ]]; then
  printf '%s\n' "${DIFF_BASE}"
  exit 0
fi

diff_branch="${DIFF_BRANCH:-origin/main}"
if git rev-parse --verify --quiet "${diff_branch}^{commit}" >/dev/null; then
  git merge-base HEAD "${diff_branch}"
  exit 0
fi

if git rev-parse --verify --quiet "main^{commit}" >/dev/null; then
  git merge-base HEAD main
  exit 0
fi

cat >&2 <<'EOF'
Unable to resolve a diff base.
Set DIFF_BASE=<sha> or fetch origin/main before running this target.
EOF
exit 1
