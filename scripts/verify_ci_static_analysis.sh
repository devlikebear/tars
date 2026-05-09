#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
CI_FILE="$ROOT/.github/workflows/ci.yml"
MAKEFILE="$ROOT/Makefile"

status=0

require_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if ! rg -q -- "$pattern" "$file"; then
    printf '[ci-static-analysis] missing: %s\n' "$description" >&2
    status=1
  fi
}

require_pattern "$CI_FILE" 'name: Check frontend console' 'PR CI frontend console check step'
require_pattern "$CI_FILE" 'cd frontend/console && npm ci --ignore-scripts' 'frontend dependency install in PR CI'
require_pattern "$CI_FILE" 'cd frontend/console && npm run check' 'frontend type/Svelte check in PR CI'
require_pattern "$CI_FILE" 'cd frontend/console && npm run test:ci' 'frontend CI test run in PR CI'
require_pattern "$MAKEFILE" '--enable=staticcheck' 'staticcheck enabled for PR diff lint'
require_pattern "$MAKEFILE" '--enable=errcheck' 'errcheck enabled for PR diff lint'

if [ "$status" -ne 0 ]; then
  exit "$status"
fi

printf '[ci-static-analysis] passed\n'
