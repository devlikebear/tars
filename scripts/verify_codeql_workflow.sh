#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
WORKFLOW="$ROOT/.github/workflows/codeql.yml"

status=0

require_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if [ ! -f "$file" ] || ! rg -q -- "$pattern" "$file"; then
    printf '[codeql-workflow] missing: %s\n' "$description" >&2
    status=1
  fi
}

require_pattern "$WORKFLOW" 'security-events: write' 'code scanning upload permission'
require_pattern "$WORKFLOW" 'language: go' 'Go CodeQL matrix entry'
require_pattern "$WORKFLOW" 'language: javascript-typescript' 'JavaScript/TypeScript CodeQL matrix entry'
require_pattern "$WORKFLOW" 'language: actions' 'GitHub Actions CodeQL matrix entry'
require_pattern "$WORKFLOW" 'github/codeql-action/init@v4' 'CodeQL init action'
require_pattern "$WORKFLOW" 'github/codeql-action/analyze@v4' 'CodeQL analyze action'

if [ "$status" -ne 0 ]; then
  exit "$status"
fi

printf '[codeql-workflow] passed\n'
