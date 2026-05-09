#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
CI_FILE="$ROOT/.github/workflows/ci.yml"
RELEASE_FILE="$ROOT/.github/workflows/release-on-version-bump.yml"
SONAR_FILE="$ROOT/.github/workflows/sonarcloud.yml"
MAKEFILE="$ROOT/Makefile"
SECURITY_SCRIPT="$ROOT/scripts/security_scan.sh"

status=0

require_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if ! rg -q -- "$pattern" "$file"; then
    printf '[github-actions-hardening] missing: %s\n' "$description" >&2
    status=1
  fi
}

forbid_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if rg -q -- "$pattern" "$file"; then
    printf '[github-actions-hardening] unexpected: %s\n' "$description" >&2
    status=1
  fi
}

forbid_top_level_permissions() {
  local file="$1"
  local description="$2"

  if awk 'NR < 20 && $0 == "permissions:" { found = 1 } END { exit found ? 0 : 1 }' "$file"; then
    printf '[github-actions-hardening] unexpected: %s\n' "$description" >&2
    status=1
  fi
}

require_pattern "$CI_FILE" '^permissions:$' 'CI workflow has explicit default permissions'
require_pattern "$CI_FILE" '^  contents: read$' 'CI workflow defaults to read-only repository contents'
require_pattern "$SECURITY_SCRIPT" 'go -C tools tool github.com/zricethezav/gitleaks/v8 detect' 'gitleaks runs through the go.mod tool lock'
forbid_pattern "$CI_FILE" 'go install github.com/zricethezav/gitleaks' 'CI workflow installs gitleaks outside go.mod'
forbid_pattern "$CI_FILE" '@latest' 'CI workflow installs tools with @latest'
require_pattern "$CI_FILE" 'npm ci --ignore-scripts' 'CI npm installs ignore package lifecycle scripts'
require_pattern "$MAKEFILE" 'npm ci --ignore-scripts' 'Makefile npm installs are lockfile-enforcing and ignore lifecycle scripts'
forbid_pattern "$MAKEFILE" 'npm install' 'Makefile uses npm install instead of npm ci'

forbid_top_level_permissions "$RELEASE_FILE" 'release workflow uses broad top-level permissions'
require_pattern "$RELEASE_FILE" '^    permissions:$' 'release workflow declares permissions at job scope'
require_pattern "$RELEASE_FILE" '^      contents: write$' 'release publish job has write contents permission'
require_pattern "$RELEASE_FILE" '^      contents: read$' 'release read-only jobs have read contents permission'

forbid_top_level_permissions "$SONAR_FILE" 'SonarCloud workflow uses top-level permissions'
require_pattern "$SONAR_FILE" '^    permissions:$' 'SonarCloud workflow declares permissions at job scope'
require_pattern "$SONAR_FILE" '^      pull-requests: read$' 'SonarCloud job can read pull request metadata'

if [[ "$status" -ne 0 ]]; then
  exit "$status"
fi

printf '[github-actions-hardening] passed\n'
